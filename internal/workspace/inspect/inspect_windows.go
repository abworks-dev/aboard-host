//go:build windows && amd64

// Package inspect copies an already quiescent, frozen source into private
// scratch. It is not an authorization boundary, repair path, or live snapshot.
package inspect

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	store "github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
)

var (
	ErrUnavailable         = errors.New("directory cannot be inspected without source writes")
	ErrIncompleteDirectory = errors.New("directory descriptor or existing lock missing")
	ErrMissingDatabase     = errors.New("workspace database missing")
	ErrRecoveryRequired    = errors.New("workspace contains storage recovery sidecars")
	ErrCorruptDatabase     = errors.New("workspace database invalid")
)

func absentSource(err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	var status win.NTStatus
	return errors.As(err, &status) && (status == win.STATUS_OBJECT_NAME_NOT_FOUND || status == win.STATUS_OBJECT_PATH_NOT_FOUND)
}

// Report contains only storage observations, not installation readiness or
// grants. Descriptor/ledger/application identity validation belongs to T015.
type Report struct {
	DatabaseUserVersion  int
	IntegrityOK          bool
	ScratchCleanupFailed bool
	scratchWrites        uint64
	scratchBytes         uint64
}

// Inspect requires the caller to authorize this root before any target access.
// scratchParent must already be an owner/SYSTEM-only protected directory outside
// the source. No missing source object is created and no source is opened by SQL.
func Inspect(ctx context.Context, root, scratchParent string) (Report, error) {
	return inspect(ctx, root, scratchParent, os.RemoveAll)
}

func inspect(ctx context.Context, root, scratchParent string, cleanup func(string) error) (Report, error) {
	return inspectValidated(ctx, root, scratchParent, cleanup, nil, nil)
}

type snapshotValidator func(context.Context, string, *sql.Conn) error
type descriptorGate func(context.Context, *platform.File) error

func inspectValidated(ctx context.Context, root, scratchParent string, cleanup func(string) error, metadata descriptorGate, validate snapshotValidator) (report Report, err error) {
	if err = ctx.Err(); err != nil {
		return report, err
	}
	root, scratchParent = filepath.Clean(root), filepath.Clean(scratchParent)
	if !filepath.IsAbs(root) || !filepath.IsAbs(scratchParent) || within(root, scratchParent) || within(scratchParent, root) {
		return report, ErrUnavailable
	}
	if err = platform.CheckPrivateDirectory(scratchParent); err != nil {
		return report, errors.Join(ErrUnavailable, err)
	}
	lease, err := platform.Acquire(root, platform.Workspace, false, true)
	if err != nil {
		if absentSource(err) {
			err = errors.Join(ErrIncompleteDirectory, err)
		}
		return report, errors.Join(ErrUnavailable, err)
	}
	var files []*platform.File
	defer func() {
		for i := len(files) - 1; i >= 0; i-- {
			err = errors.Join(err, files[i].Close())
		}
		err = errors.Join(err, lease.Close())
	}()
	descriptor, err := lease.PinFile("aboard.workspace.json", true)
	if err != nil {
		if absentSource(err) {
			err = errors.Join(ErrIncompleteDirectory, err)
		}
		return report, errors.Join(ErrUnavailable, err)
	}
	files = append(files, descriptor)
	// Current tenant must be established from frozen metadata before even
	// pinning or diagnosing the database. Unknown scope exposes no detail.
	if metadata != nil {
		if err = metadata(ctx, descriptor); err != nil {
			return report, err
		}
	}
	database, err := lease.PinFile(`data\workspace.sqlite`, true)
	if err != nil {
		if absentSource(err) {
			err = errors.Join(ErrMissingDatabase, err)
		}
		return report, errors.Join(ErrUnavailable, err)
	}
	files = append(files, database)
	// The source DB and data directory are now retained. Reject every SQLite
	// sidecar including hot rollback, WAL/SHM and super-journal candidates; never
	// ask SQLite to inspect/recover the live original or delete its sidecars.
	entries, err := os.ReadDir(filepath.Join(root, "data"))
	if err != nil {
		return report, errors.Join(ErrUnavailable, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(strings.ToLower(entry.Name()), "workspace.sqlite-") {
			return report, errors.Join(ErrUnavailable, ErrRecoveryRequired)
		}
	}
	if err = lease.Validate(); err != nil {
		return report, errors.Join(ErrUnavailable, err)
	}
	var nonce [24]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return report, err
	}
	scratch, err := platform.NewScratch(scratchParent, "inspect-"+hex.EncodeToString(nonce[:]))
	if err != nil {
		return report, err
	}
	defer func() {
		// Delete only this freshly created, explicitly known private child, never a
		// caller-supplied root. Cleanup failures do not turn into source repair.
		defer func() {
			if closeErr := scratch.Close(); closeErr != nil {
				report.ScratchCleanupFailed = true
				err = errors.Join(err, closeErr)
			}
		}()
		if cleanupErr := scratch.PrepareCleanup(); cleanupErr != nil {
			report.ScratchCleanupFailed = true
			err = errors.Join(err, fmt.Errorf("sensitive scratch release failed: %w", cleanupErr))
			return
		}
		if cleanupErr := cleanup(scratch.Path()); cleanupErr != nil {
			report.ScratchCleanupFailed = true
			err = errors.Join(err, fmt.Errorf("sensitive scratch cleanup failed: %w", cleanupErr))
		}
	}()
	for _, item := range []struct {
		source *platform.File
		name   string
	}{{descriptor, "descriptor.json"}, {database, "workspace.sqlite"}} {
		if err = copyFrozen(ctx, item.source, scratch, item.name, &report); err != nil {
			return report, err
		}
	}
	if err = lease.Validate(); err != nil {
		return report, errors.Join(ErrUnavailable, err)
	}
	handle, err := store.Open(ctx, filepath.Join(scratch.Path(), "workspace.sqlite"), store.ReadOnlyScratch)
	if err != nil {
		return report, errors.Join(ErrUnavailable, err)
	}
	defer func() { err = errors.Join(err, handle.Close()) }()
	if validate != nil {
		if err = validate(ctx, scratch.Path(), handle.Conn); err != nil {
			return report, errors.Join(ErrUnavailable, err)
		}
	}
	var result string
	if err = handle.Conn.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return report, errors.Join(ErrUnavailable, err)
	}
	if result != "ok" {
		return report, errors.Join(ErrUnavailable, ErrCorruptDatabase)
	}
	if err = handle.Conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&report.DatabaseUserVersion); err != nil {
		return report, err
	}
	if err = lease.Validate(); err != nil {
		return report, errors.Join(ErrUnavailable, err)
	}
	report.IntegrityOK = true
	return report, nil
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyFrozen(ctx context.Context, source *platform.File, scratch *platform.Scratch, name string, report *Report) (err error) {
	if err = source.Validate(); err != nil {
		return err
	}
	var duplicate win.Handle
	process := win.CurrentProcess()
	if err = win.DuplicateHandle(process, source.Handle(), process, &duplicate, 0, false, win.DUPLICATE_SAME_ACCESS); err != nil {
		return err
	}
	in := os.NewFile(uintptr(duplicate), "frozen-source")
	defer func() { err = errors.Join(err, in.Close()) }()
	// DuplicateHandle shares the source pointer with metadata preflight. Reset
	// explicitly; no two reads of a source are performed concurrently.
	if _, err = in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	out, err := scratch.CreateFile(name)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	buf := make([]byte, 64*1024)
	var copied int64
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := in.Read(buf)
		if n > 0 {
			copied += int64(n)
			if name == "descriptor.json" && copied > int64(protocol.RequestMaxBytes) {
				return ErrUnavailable
			}
			written, writeErr := out.Write(buf[:n])
			// One successful nonempty <=64KiB os.File.Write requires at least
			// one native data write; retries/short writes only increase it.
			// This independent lower bound is checked against OS counters in
			// tests, never inferred from those counters or used as authority.
			if written > 0 {
				report.scratchWrites++
				report.scratchBytes += uint64(written)
			}
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if name == "workspace.sqlite" && copied < 100 {
		return errors.Join(ErrUnavailable, ErrCorruptDatabase)
	}
	return source.Validate()
}
