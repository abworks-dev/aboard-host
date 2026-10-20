//go:build windows && amd64

package acceptance

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/abworks-dev/aboard-host/internal/host"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

const hotJournalEnvironment = "ABOARD_ACCEPTANCE_HOT_JOURNAL"
const hotJournalPrefix = "ABOARD_SYNCED_HOT_JOURNAL "

type hotJournalInput struct{ Fixture, Root string }
type hotJournalEvidence struct {
	PID                                  int
	Birth                                uint64
	DatabaseFlush                        bool
	Flushes                              int32
	Bytes                                int64
	Records, OriginalPages, Sector, Page uint32
}

var hotJournalState struct {
	root    string
	flushes atomic.Int32
}

// A permanently rooted Go function value, installed only in this dedicated
// helper process. No runtime restore/swap or production fault endpoint exists.
var hotJournalCallback = hotJournalFlush

func hotJournalFlush(tls *libc.TLS, actual uintptr) int32 {
	result := libc.XFlushFileBuffers(tls, actual)
	if result == 0 {
		return result
	}
	n := hotJournalState.flushes.Add(1)
	if n != 3 {
		return result
	}
	var path [32768]uint16
	size, pathErr := win.GetFinalPathNameByHandle(win.Handle(actual), &path[0], uint32(len(path)), 0)
	if pathErr != nil || size == 0 || size >= uint32(len(path)) || !strings.EqualFold(strings.TrimPrefix(win.UTF16ToString(path[:size]), `\\?\`), filepath.Join(hotJournalState.root, "data", "workspace.sqlite")) {
		os.Exit(2)
	}
	evidence, err := readHotJournal(hotJournalState.root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "synced journal shape was not established")
		os.Exit(2)
	}
	evidence.PID = os.Getpid()
	evidence.DatabaseFlush = true
	evidence.Flushes = n
	raw, err := json.Marshal(evidence)
	if err != nil {
		os.Exit(2)
	}
	if _, err = os.Stdout.Write(append(append([]byte(hotJournalPrefix), raw...), '\n')); err != nil {
		os.Exit(2)
	}
	// The dirty DB and its synced rollback journal are real SQLite outputs. This
	// exit deliberately does NOT run SQL rollback/Close or release the live lease.
	os.Exit(0)
	return result
}
func readHotJournal(root string) (hotJournalEvidence, error) {
	path := filepath.Join(root, "data", "workspace.sqlite-journal")
	f, err := os.Open(path)
	if err != nil {
		return hotJournalEvidence{}, err
	}
	var head [28]byte
	n, readErr := f.ReadAt(head[:], 0)
	info, statErr := f.Stat()
	closeErr := f.Close()
	if readErr != nil || n != len(head) || statErr != nil || closeErr != nil {
		return hotJournalEvidence{}, fmt.Errorf("journal evidence unavailable")
	}
	if !bytes.Equal(head[:8], []byte{0xd9, 0xd5, 0x05, 0xf9, 0x20, 0xa1, 0x63, 0xd7}) {
		return hotJournalEvidence{}, fmt.Errorf("journal not synced")
	}
	e := hotJournalEvidence{Bytes: info.Size(), Records: binary.BigEndian.Uint32(head[8:12]), OriginalPages: binary.BigEndian.Uint32(head[16:20]), Sector: binary.BigEndian.Uint32(head[20:24]), Page: binary.BigEndian.Uint32(head[24:28])}
	power := func(n uint32) bool { return n != 0 && n&(n-1) == 0 }
	if e.Records == 0 || e.Records == 0xffffffff || e.OriginalPages == 0 || !power(e.Sector) || e.Sector < 512 || e.Sector > 65536 || !power(e.Page) || e.Page < 512 || e.Page > 65536 || uint64(e.Sector)+uint64(e.Records)*(uint64(e.Page)+8) > uint64(e.Bytes) {
		return hotJournalEvidence{}, fmt.Errorf("journal records incomplete")
	}
	return e, nil
}
func TestAcceptanceHotJournalHelper(t *testing.T) {
	encoded := os.Getenv(hotJournalEnvironment)
	if encoded == "" {
		t.Skip("genuine hot-journal child only")
	}
	if len(encoded) > 8192 {
		t.Fatal("oversized fixture")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	var in hotJournalInput
	if err != nil || json.Unmarshal(raw, &in) != nil || requireSyntheticRoot(in.Fixture) != nil {
		t.Fatal("unowned hot fixture")
	}
	config, err := host.LoadConfig(filepath.Join(in.Fixture, "config.json"))
	if err != nil || len(config.AllowedRoots) != 1 || config.AllowedRoots[0] != filepath.Join(in.Fixture, "allowed") || config.InstallationRoot != filepath.Join(in.Fixture, "installation") || in.Root != filepath.Join(config.AllowedRoots[0], "hot-source") || !within(in.Fixture, in.Root) || !unreparsed(in.Root) {
		t.Fatal("hot target mismatch")
	}
	var permit [1]byte
	if _, err = io.ReadFull(os.Stdin, permit[:]); err != nil || permit[0] != 1 {
		t.Fatal("launcher identity permit absent")
	}
	lease, err := platform.Acquire(in.Root, platform.Workspace, false, false)
	if err != nil {
		t.Fatal("hot fixture requires native exclusion")
	}
	_, err = lease.PinFile(filepath.Join("data", "workspace.sqlite"), false)
	if err != nil {
		t.Fatal("hot DB pin refused")
	}
	db, err := sqlite.Open(context.Background(), filepath.Join(in.Root, "data", "workspace.sqlite"), sqlite.ReadWriteExisting)
	if err != nil {
		t.Fatal("existing hot database refused")
	}
	tls := libc.NewTLS()
	name, err := libc.CString("win32")
	if err != nil {
		t.Fatal(err)
	}
	vfs := sqlite3.Xsqlite3_vfs_find(tls, name)
	if vfs == 0 {
		t.Fatal("fixed VFS missing")
	}
	var view sqlite3.Tsqlite3_vfs
	libc.Xmemcpy(tls, uintptr(unsafe.Pointer(&view)), vfs, libc.Tsize_t(unsafe.Sizeof(view)))
	if view.FiVersion < 3 || view.FxGetSystemCall == 0 || view.FxSetSystemCall == 0 {
		t.Fatal("fixed VFS ABI unavailable")
	}
	get := *(*func(*libc.TLS, uintptr, uintptr) uintptr)(unsafe.Pointer(&view.FxGetSystemCall))
	set := *(*func(*libc.TLS, uintptr, uintptr, uintptr) int32)(unsafe.Pointer(&view.FxSetSystemCall))
	syscallName, err := libc.CString("FlushFileBuffers")
	if err != nil {
		t.Fatal(err)
	}
	if get(tls, vfs, syscallName) == 0 {
		t.Fatal("native flush missing")
	}
	hotJournalState.root = in.Root
	hotJournalState.flushes.Store(0)
	ptr := *(*uintptr)(unsafe.Pointer(&hotJournalCallback))
	if set(tls, vfs, syscallName, ptr) != sqlite3.SQLITE_OK || get(tls, vfs, syscallName) != ptr {
		t.Fatal("child flush delegate refused")
	}
	// No spilling/unsynced trailing journal segment. The third actual successful
	// flush is before DELETE's commit point; no synthesized/truncated sidecar.
	_, err = db.Conn.ExecContext(context.Background(), `PRAGMA cache_spill=OFF; BEGIN IMMEDIATE; UPDATE workspace_identity SET display_name=hex(zeroblob(1500000)); COMMIT;`)
	t.Fatalf("real crash checkpoint not reached: flushes=%d failed=%t", hotJournalState.flushes.Load(), err != nil)
}
func makeExternalHotJournal(t *testing.T, f *fixture, root string) hotJournalEvidence {
	t.Helper()
	input, _ := json.Marshal(hotJournalInput{f.root, root})
	image, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, image, "-test.run=^TestAcceptanceHotJournalHelper$", "-test.count=1", "-test.v")
	cmd.Dir = projectRoot()
	cmd.Env = append(os.Environ(), hotJournalEnvironment+"="+base64.RawURLEncoding.EncodeToString(input))
	var out boundedCapture
	cmd.Stdout = &out
	cmd.Stderr = &out
	permit, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("helper permit pipe unavailable")
	}
	defer permit.Close()
	if cmd.Start() != nil {
		t.Fatal("hot helper did not start")
	}
	handle, birth, actual, identityErr := processIdentity(cmd.Process.Pid)
	if identityErr == nil && birth != 0 && strings.EqualFold(actual, image) {
		_, err = permit.Write([]byte{1})
	}
	_ = permit.Close()
	err = cmd.Wait()
	if handle != 0 {
		if e := win.CloseHandle(handle); e != nil {
			t.Error("helper process pin close failed")
		}
	}
	raw, overflow := out.snapshot()
	if err != nil || ctx.Err() != nil || overflow || identityErr != nil || birth == 0 || !strings.EqualFold(actual, image) {
		t.Fatalf("hot helper failed (output withheld): exitFailure=%t identityFailure=%t overflow=%t", err != nil, identityErr != nil, overflow)
	}
	var evidence hotJournalEvidence
	found := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, hotJournalPrefix) {
			if found || json.Unmarshal([]byte(strings.TrimPrefix(line, hotJournalPrefix)), &evidence) != nil {
				t.Fatal("invalid hot evidence")
			}
			found = true
		}
	}
	disk, e := readHotJournal(root)
	if !found || e != nil || evidence.PID != cmd.Process.Pid || !evidence.DatabaseFlush || evidence.Flushes != 3 || evidence.Bytes != disk.Bytes || evidence.Records != disk.Records || evidence.OriginalPages != disk.OriginalPages || evidence.Sector != disk.Sector || evidence.Page != disk.Page {
		t.Fatal("actual hot journal evidence absent")
	}
	evidence.Birth = birth
	return evidence
}

func TestV009IndependentGenuineHotJournal(t *testing.T) {
	for _, variant := range []string{"recoverable", "invalid-recovered-history"} {
		t.Run(variant, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			first := f.start(t)
			c := first.connect(t)
			root, a := newInspectionWorkspace(t, c, "hot-source")
			_, b := newInspectionWorkspace(t, c, "hot-independent-B")
			opened := c.open(t, a.WorkspaceId)
			prefix := "/v1/workspaces/" + string(a.WorkspaceId)
			var candidate protocol.CandidateView
			c.request(t, "POST", prefix+"/record-previews", protocol.PreviewRecord{OpenEpoch: *opened.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "before real journal", Body: "exact formal bytes before genuine uncommitted SQL"}, &candidate, 201)
			commit := protocol.CommitRecord{OperationId: newID(t), OpenEpoch: *opened.OpenEpoch, CandidateId: candidate.CandidateId, CandidateDigest: candidate.CandidateDigest, PreviewVersion: candidate.PreviewVersion}
			var receipt protocol.Receipt
			c.request(t, "POST", prefix+"/record-commits", commit, &receipt, 200)
			var expected protocol.TextRecordView
			c.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil, &expected, 200)
			first.stop(t)
			if variant == "invalid-recovered-history" {
				mutateBadSQL(t, f, root, "workspace", "immutable-content-corrupt")
			}
			clean := tree(t, root)
			hot := makeExternalHotJournal(t, f, root)
			before := tree(t, root)
			if reflect.DeepEqual(clean, before) || clean[filepath.Join("data", "workspace.sqlite")] == before[filepath.Join("data", "workspace.sqlite")] {
				t.Fatal("native dirty database bytes were not observed")
			}
			second := f.start(t)
			d := second.connect(t)
			watch := startInspectionWatch(t, root)
			var rejected protocol.ErrorView
			d.request(t, "POST", "/v1/workspace-inspections", protocol.InspectWorkspace{Path: protocol.Path(root)}, &rejected, 409)
			if rejected.Code != protocol.ErrorCodeRECOVERYREQUIRED || !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("inspect recovered or rewrote actual hot source")
			}
			watch.finish(t, false)
			other := d.open(t, b.WorkspaceId)
			bPrefix := "/v1/workspaces/" + string(b.WorkspaceId)
			var bp protocol.CandidateView
			d.request(t, "POST", bPrefix+"/record-previews", protocol.PreviewRecord{OpenEpoch: *other.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "independent hot B", Body: protocol.Body(variant)}, &bp, 201)
			var br protocol.Receipt
			d.request(t, "POST", bPrefix+"/record-commits", protocol.CommitRecord{OperationId: newID(t), OpenEpoch: *other.OpenEpoch, CandidateId: bp.CandidateId, CandidateDigest: bp.CandidateDigest, PreviewVersion: bp.PreviewVersion}, &br, 200)
			var body protocol.TextRecordView
			d.request(t, "GET", bPrefix+"/records/"+string(br.ResourceId), nil, &body, 200)
			if body.Body != protocol.Body(variant) || body.OperationId != br.OperationId || br.NewRevision != "1" {
				t.Fatal("B could not genuinely save while A hot")
			}
			if variant == "recoverable" {
				recovered := d.open(t, a.WorkspaceId)
				if recovered.OpenEpoch == nil || *recovered.OpenEpoch == *opened.OpenEpoch || recovered.Revision == nil || *recovered.Revision != "1" {
					t.Fatal("authorized recovery invented identity/head/name")
				}
				var actualRecord protocol.TextRecordView
				d.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil, &actualRecord, 200)
				var same protocol.Receipt
				d.request(t, "POST", prefix+"/record-commits", commit, &same, 200)
				var records protocol.RecordList
				d.request(t, "GET", prefix+"/records", nil, &records, 200)
				if !reflect.DeepEqual(expected, actualRecord) || same != receipt || len(records.Items) != 1 {
					t.Fatal("recovery replayed or changed confirmed formal data")
				}
				if _, e := os.Lstat(filepath.Join(root, "data", "workspace.sqlite-journal")); !os.IsNotExist(e) {
					t.Fatal("explicit recovery did not finish journal")
				}
				closePublic(t, d, recovered)
			} else {
				watch = startInspectionWatch(t, root)
				pre := registrationPrecondition(t, d, a.WorkspaceId, "open")
				d.request(t, "POST", prefix+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, &rejected, 409)
				if rejected.Code != protocol.ErrorCodeUNSUPPORTEDSCHEMA || !reflect.DeepEqual(before, tree(t, root)) {
					t.Fatal("bad recovered history allowed source recovery", rejected.Code)
				}
				watch.finish(t, false)
				lease, e := platform.Acquire(root, platform.Workspace, false, false)
				if e != nil {
					t.Fatal("refused recovery lost cleanup ownership")
				}
				if lease.Close() != nil {
					t.Fatal("native proof lease did not close")
				}
			}
			second.stop(t)
			// Rollback under this frozen DELETE profile restores the complete original
			// pages and file length, including unexposed display_name; no public DTO
			// field or direct post-recovery SQL observer is invented for that oracle.
			if variant == "recoverable" && !reflect.DeepEqual(clean, tree(t, root)) {
				t.Fatal("rollback did not restore the exact pre-transaction source bytes")
			}
			if variant != "recoverable" && !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("shutdown repaired bad hot history")
			}
			matrixEvidence(t, f, "actual-hot-"+variant, struct {
				Native        hotJournalEvidence
				Before, After map[string]string
				Original      protocol.Receipt
			}{hot, before, tree(t, root), receipt})
		})
	}
}
