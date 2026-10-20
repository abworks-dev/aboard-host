//go:build windows && amd64

package installation

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

var errRecoveryShape = errors.New("workspace rollback journal is not a supported recovery input")
var rollbackMagic = []byte{0xd9, 0xd5, 0x05, 0xf9, 0x20, 0xa1, 0x63, 0xd7}

// This conservative shape gate admits only a nonempty hot rollback journal of
// the fixed DELETE profile, not cold/partial headers, WAL or multi-DB recovery.
// SQLite, not this parser, owns page recovery. A recovered private copy must
// additionally pass the full descriptor/schema/history checks before source IO.
func validateRollbackJournal(path string) error {
	return validateRollbackJournalContext(context.Background(), path)
}
func validateRollbackJournalContext(ctx context.Context, path string) (err error) {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, platform.ErrRecoveryInputCloseUnknown, closeErr)
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	var header [28]byte
	if _, err = io.ReadFull(f, header[:]); err != nil {
		return errors.Join(errRecoveryShape, err)
	}
	page := binary.BigEndian.Uint32(header[24:28])
	sector := binary.BigEndian.Uint32(header[20:24])
	if !bytes.Equal(header[:8], rollbackMagic) || binary.BigEndian.Uint32(header[8:12]) == 0 || binary.BigEndian.Uint32(header[16:20]) == 0 || page < 512 || page > 65536 || page&(page-1) != 0 || sector < 512 || sector > 65536 || sector&(sector-1) != 0 || info.Size() < int64(sector)+int64(page)+8 {
		return errRecoveryShape
	}
	// Playback may truncate/grow the database to the header's original size
	// BEFORE schema validation. Bound that expansion as well as copied inputs;
	// a tiny malicious journal must not turn private scratch into a huge file.
	// uint32 original * page<=65536 is safely representable in int64.
	recoveredBytes := int64(binary.BigEndian.Uint32(header[16:20])) * int64(page)
	if info.Size() > maxRecoveryScratchBytes || recoveredBytes > maxRecoveryScratchBytes-info.Size() {
		return errRecoveryBudget
	}
	var tail [8]byte
	if _, err = f.Seek(-8, io.SeekEnd); err != nil {
		return err
	}
	if _, err = io.ReadFull(f, tail[:]); err != nil {
		return err
	}
	// The master-journal trailer ends in the same magic. ATTACH/multi-DB
	// transactions are not this slice's protocol; never follow a supplied name.
	if bytes.Equal(tail[:], rollbackMagic) {
		return errRecoveryShape
	}
	return validateRollbackRecords(ctx, f, info.Size(), header)
}

// SQLite may silently stop journal playback at a damaged record checksum. A
// valid recovered schema alone would then be insufficient evidence (for example
// mutable display_name is not an immutable content hash). Verify every record in
// our supported synced DELETE journal before permitting source recovery.
func validateRollbackRecords(ctx context.Context, f *os.File, size int64, first [28]byte) error {
	header := first
	original := binary.BigEndian.Uint32(first[16:20])
	sector := int64(binary.BigEndian.Uint32(first[20:24]))
	page := int64(binary.BigEndian.Uint32(first[24:28]))
	offset := int64(0)
	record := make([]byte, int(page)+8)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !bytes.Equal(header[:8], rollbackMagic) || binary.BigEndian.Uint32(header[16:20]) != original || int64(binary.BigEndian.Uint32(header[20:24])) != sector || int64(binary.BigEndian.Uint32(header[24:28])) != page {
			return errRecoveryShape
		}
		count := int64(binary.BigEndian.Uint32(header[8:12]))
		// 0xffffffff is the no-sync profile, not the frozen EXTRA profile. A zero
		// header is an unsynced segment; do not guess which partial pages are safe.
		if count == 0 || count == 0xffffffff || count > (size-offset-sector)/(page+8) {
			return errRecoveryShape
		}
		offset += sector
		nonce := binary.BigEndian.Uint32(header[12:16])
		for i := int64(0); i < count; i++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, err := f.ReadAt(record, offset); err != nil {
				return errors.Join(errRecoveryShape, err)
			}
			number := binary.BigEndian.Uint32(record[:4])
			if number == 0 || number > original || uint64(number) == (uint64(1)<<30)/uint64(page)+1 {
				return errRecoveryShape
			}
			checksum := nonce
			for j := int(page) - 200; j > 0; j -= 200 {
				checksum += uint32(record[4+j])
			}
			if checksum != binary.BigEndian.Uint32(record[4+int(page):]) {
				return errRecoveryShape
			}
			offset += page + 8
		}
		if offset == size {
			return nil
		}
		aligned := ((offset + sector - 1) / sector) * sector
		if aligned >= size {
			// Unconsumed bytes are not proof of harmless sector padding. They
			// may be a damaged/partial trailing segment; refuse conservatively.
			return errRecoveryShape
		}
		if size-aligned < 28 {
			return errRecoveryShape
		}
		offset = aligned
		if _, err := f.ReadAt(header[:], offset); err != nil {
			return errors.Join(errRecoveryShape, err)
		}
	}
}

// A local recovery-adapter resource ceiling, not a workspace/database size
// limit. Larger ordinary databases still open; oversized recovery is unavailable
// without changing the source, rather than consuming unbounded shared scratch.
const maxRecoveryScratchBytes int64 = 256 << 20
const maxRecoveryPreflightTime = 30 * time.Second

var errRecoveryBudget = errors.New("workspace explicit recovery exceeds local scratch budget")

type recoveryWriter struct {
	ctx       context.Context
	out       io.Writer
	remaining *int64
}

func (w recoveryWriter) Write(b []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if w.remaining == nil || int64(len(b)) > *w.remaining {
		return 0, errRecoveryBudget
	}
	n, err := w.out.Write(b)
	*w.remaining -= int64(n)
	return n, err
}

func (r *openResources) preflightRecovery(ctx context.Context, descriptor workspace.DirectoryDescriptor, authorize func(context.Context) error) (needsRecovery bool, err error) {
	defer func() {
		if errors.Is(err, platform.ErrPhysicalCloseUnknown) {
			r.quarantined = true
			r.nativeFailure = err
		}
	}()
	r.recoveryInputs, err = r.lease.FreezeWorkspaceOpenInputs()
	if err != nil {
		return false, err
	}
	if r.recoveryInputs.Journal == nil {
		err = r.recoveryInputs.Close()
		if err == nil {
			r.recoveryInputs = nil
		}
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, maxRecoveryPreflightTime)
	defer cancel()
	total := int64(0)
	for _, input := range []*platform.File{r.recoveryInputs.Journal, r.recoveryInputs.Database} {
		size, sizeErr := input.FrozenInputSize()
		if sizeErr != nil {
			return false, sizeErr
		}
		if size <= 0 || size > maxRecoveryScratchBytes-total {
			return false, errRecoveryBudget
		}
		total += size
	}
	remaining := maxRecoveryScratchBytes
	var nonce [24]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return false, err
	}
	// The installation root is already owner/SYSTEM-private and is never request
	// input. This fresh child is disposable storage, not another source of truth.
	r.recoveryScratch, err = platform.NewScratch(r.store.root, "inspect-"+hex.EncodeToString(nonce[:]))
	if err != nil {
		return false, err
	}
	for _, input := range []struct {
		source *platform.File
		name   string
	}{{r.recoveryInputs.Journal, "workspace.sqlite-journal"}, {r.recoveryInputs.Database, "workspace.sqlite"}} {
		out, createErr := r.recoveryScratch.CreateFile(input.name)
		if createErr != nil {
			return false, createErr
		}
		copyErr := input.source.CopyFrozenWorkspaceInput(recoveryWriter{ctx, out, &remaining})
		closeErr := out.Close()
		if closeErr != nil || errors.Is(copyErr, platform.ErrRecoveryInputCloseUnknown) {
			r.quarantined = true
		}
		if err = errors.Join(copyErr, closeErr); err != nil {
			return false, err
		}
		if input.name == "workspace.sqlite-journal" {
			if err = validateRollbackJournalContext(ctx, filepath.Join(r.recoveryScratch.Path(), input.name)); err != nil {
				r.quarantined = errors.Is(err, platform.ErrRecoveryInputCloseUnknown)
				return false, err
			}
		}
	}
	scratchDB := filepath.Join(r.recoveryScratch.Path(), "workspace.sqlite")
	r.recoveryDatabase, err = sqlite.OpenDatabase(ctx, scratchDB, sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
	if err != nil {
		r.quarantined = errors.Is(err, sqlite.ErrPhysicalCloseUnknown)
		return false, err
	}
	if _, err = r.recoveryDatabase.ValidateOpenHistory(ctx, descriptor); err != nil {
		return false, err
	}
	if err = r.closeRecoveryScratch(); err != nil {
		return false, err
	}
	if err = authorize(ctx); err != nil {
		return false, err
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	if err = r.recoveryInputs.Close(); err != nil {
		return false, err
	}
	r.recoveryInputs = nil
	return true, r.lease.Validate()
}

func (r *openResources) closeRecoveryScratch() error {
	if r.recoveryDatabase != nil {
		if err := r.recoveryDatabase.Close(); err != nil {
			r.quarantined = true
			return err
		}
		r.recoveryDatabase = nil
	}
	if r.recoveryScratch != nil {
		// Known absolute freshly created child only. Keep the installation anchor
		// pinned during deletion; never delete the source or a request-supplied path.
		path := r.recoveryScratch.Path()
		if filepath.Dir(path) != r.store.root {
			return errRecoveryShape
		}
		if !r.recoveryScratchDeleted {
			if err := r.recoveryScratch.PrepareCleanup(); err != nil {
				return err
			}
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			r.recoveryScratchDeleted = true
		}
		if err := r.recoveryScratch.Close(); err != nil {
			return err
		}
		r.recoveryScratch = nil
		r.recoveryScratchDeleted = false
	}
	return nil
}
