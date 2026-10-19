//go:build windows && amd64

package installation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
)

// The child exits without closing an actual SQLite transaction, leaving a hot
// DELETE journal. No fault shim fabricates SQLite recovery or a successful drain.
func TestWorkspaceHotRollbackChild(t *testing.T) {
	root := os.Getenv("ABOARD_T016_ROLLBACK_ROOT")
	if root == "" {
		return
	}
	base, err := filepath.Abs(filepath.Join("..", "..", ".data", "logs", "001-validation"))
	if err != nil || !underLocation(base, root) {
		t.Fatal("not synthetic root")
	}
	ctx := context.Background()
	lease, err := platform.Acquire(root, platform.Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = lease.PinFile(`data\workspace.sqlite`, false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(ctx, filepath.Join(root, "data", "workspace.sqlite"), sqlite.ReadWriteExisting)
	if err != nil {
		t.Fatal(err)
	}
	// Crash after both synced journal and dirty database have actually flushed,
	// but before SQLite deletes the rollback journal (the commit point). Avoid
	// cache spilling: its trailing unsynced segment is intentionally unsupported
	// by this conservative recovery adapter. Never fabricate/truncate the source.
	creationFlushFault.at = 0
	creationFlushFault.calls.Store(0)
	creationFlushFault.afterFlush = func(n int32) {
		if n != 3 {
			return
		}
		if err := validateRollbackJournal(filepath.Join(root, "data", "workspace.sqlite-journal")); err != nil {
			fmt.Fprintln(os.Stderr, "real synced journal refused:", err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	restore := replaceCreationFlush(t)
	_, err = db.Conn.ExecContext(ctx, `PRAGMA cache_spill=OFF; BEGIN IMMEDIATE; UPDATE workspace_identity SET display_name=hex(zeroblob(1500000)); COMMIT;`)
	restore()
	creationFlushFault.afterFlush = nil
	t.Fatalf("crash point not reached: flushes=%d err=%v", creationFlushFault.calls.Load(), err)
}

func makeHotRollback(t *testing.T, root string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWorkspaceHotRollbackChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), "ABOARD_T016_ROLLBACK_ROOT="+root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rollback child: %v %s", err, out)
	}
	if err = validateRollbackJournal(filepath.Join(root, "data", "workspace.sqlite-journal")); err != nil {
		t.Fatal("no hot rollback evidence", err)
	}
}

func TestWorkspaceOpenExplicitHotRollback(t *testing.T) {
	f, c, operator, token, r := creationFixture(t)
	defer f.store.Close()
	ctx := context.Background()
	created, err := c.CreateWorkspace(ctx, operator, r)
	if err != nil {
		t.Fatal(err)
	}
	original := inspectionTree(t, r.RootLocator)
	makeHotRollback(t, r.RootLocator)
	hot := inspectionTree(t, r.RootLocator)
	if reflect.DeepEqual(original, hot) {
		t.Fatal("fixture did not dirty source")
	}
	a, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	closeA, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.close")
	if err != nil {
		t.Fatal(err)
	}
	opener, _ := NewWorkspaceOpener(f.auth)
	view, err := opener.OpenWorkspace(ctx, a, 1)
	if err != nil {
		t.Fatal("explicit recovery", err)
	}
	if view.Epoch == "" || view.Snapshot.Identity.DisplayName != r.DisplayName || view.Snapshot.Revision != 0 {
		t.Fatal("not original committed history", view)
	}
	if err = opener.DrainWorkspace(ctx, closeA); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(r.RootLocator, "data", "workspace.sqlite-journal")); !os.IsNotExist(err) {
		t.Fatal("journal was not recovered", err)
	}
	// Verify recovery did not invent a save or receipt, and persists after re-open.
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(r.RootLocator, "data", "workspace.sqlite"))+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"save_operation", "commit_receipt", "text_record", "candidate", "text_version", "audit_entry"} {
		var n int
		if err = db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s replayed n=%d %v", table, n, err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	view, err = opener.OpenWorkspace(ctx, a, 1)
	if err != nil {
		t.Fatal(err)
	}
	if view.Snapshot.Identity.DisplayName != r.DisplayName {
		t.Fatal(view)
	}
	if err = opener.DrainWorkspace(ctx, closeA); err != nil {
		t.Fatal(err)
	}
	// No disposable scratch survives a successful preflight.
	matches, _ := filepath.Glob(filepath.Join(f.store.root, "inspect-*"))
	if len(matches) != 0 {
		t.Fatal("scratch retained", matches)
	}
}

func TestWorkspaceRecoveryRejectsInvalidHistoryWithoutSourceWrites(t *testing.T) {
	f, c, operator, token, r := creationFixture(t)
	defer f.store.Close()
	ctx := context.Background()
	created, err := c.CreateWorkspace(ctx, operator, r)
	if err != nil {
		t.Fatal(err)
	}
	// Commit corruption BEFORE creating a genuine interrupted transaction. A
	// valid journal alone is never sufficient permission to touch the source.
	inspectionSQL(t, r.RootLocator, `UPDATE workspace_identity SET identity_digest=printf('%064d',0)`)
	makeHotRollback(t, r.RootLocator)
	before := inspectionTree(t, r.RootLocator)
	a, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	opener, _ := NewWorkspaceOpener(f.auth)
	view, err := opener.OpenWorkspace(ctx, a, 1)
	if err == nil || view.Epoch != "" {
		t.Fatal("corrupt recovered history accepted", view, err)
	}
	if !reflect.DeepEqual(before, inspectionTree(t, r.RootLocator)) {
		t.Fatal("invalid recovery changed source")
	}
	lease, err := platform.Acquire(r.RootLocator, platform.Workspace, false, false)
	if err != nil {
		t.Fatal("known invalid source incorrectly retained", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
}

// Structurally plausible headers are not evidence that SQLite may safely
// recover a source: its tolerant playback can silently discard bad records.
func TestWorkspaceRecoveryRejectsMalformedRecordsWithoutSourceWrites(t *testing.T) {
	for _, kind := range []string{"zero-page", "checksum", "truncated", "no-sync", "growth-budget", "trailing-byte"} {
		t.Run(kind, func(t *testing.T) {
			f, c, operator, token, r := creationFixture(t)
			defer f.store.Close()
			ctx := context.Background()
			created, err := c.CreateWorkspace(ctx, operator, r)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(r.RootLocator, "data", "workspace.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			page := int(binary.BigEndian.Uint16(raw[16:18]))
			if page == 1 {
				page = 65536
			}
			payload := make([]byte, 512+page+8)
			copy(payload, rollbackMagic)
			binary.BigEndian.PutUint32(payload[8:12], 1)
			binary.BigEndian.PutUint32(payload[12:16], 73)
			binary.BigEndian.PutUint32(payload[16:20], uint32(len(raw)/page))
			binary.BigEndian.PutUint32(payload[20:24], 512)
			binary.BigEndian.PutUint32(payload[24:28], uint32(page))
			binary.BigEndian.PutUint32(payload[512:516], 1)
			binary.BigEndian.PutUint32(payload[len(payload)-4:], 73)
			switch kind {
			case "zero-page":
				binary.BigEndian.PutUint32(payload[512:516], 0)
			case "checksum":
				payload[len(payload)-1]++
			case "truncated":
				payload = payload[:len(payload)-1]
			case "no-sync":
				binary.BigEndian.PutUint32(payload[8:12], 0xffffffff)
			case "trailing-byte":
				payload = append(payload, 0x42)
			case "growth-budget":
				// Valid page/checksum but a claimed pretransaction size of many GiB.
				// Refuse before SQLite can grow even the disposable copy.
				binary.BigEndian.PutUint32(payload[16:20], 0x7fffffff)
			}
			if err = os.WriteFile(filepath.Join(r.RootLocator, "data", "workspace.sqlite-journal"), payload, 0600); err != nil {
				t.Fatal(err)
			}
			before := inspectionTree(t, r.RootLocator)
			a, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.open")
			if err != nil {
				t.Fatal(err)
			}
			opener, _ := NewWorkspaceOpener(f.auth)
			wantErr := errRecoveryShape
			if kind == "growth-budget" {
				wantErr = errRecoveryBudget
			}
			if v, e := opener.OpenWorkspace(ctx, a, 1); !errors.Is(e, wantErr) || v.Epoch != "" {
				t.Fatalf("malformed admitted: %+v %v", v, e)
			}
			lease, leaseErr := platform.Acquire(r.RootLocator, platform.Workspace, false, false)
			if leaseErr != nil {
				t.Fatal("known refusal retained lease", leaseErr)
			}
			if leaseErr = lease.Close(); leaseErr != nil {
				t.Fatal(leaseErr)
			}
			if !reflect.DeepEqual(before, inspectionTree(t, r.RootLocator)) {
				t.Fatal("malformed recovery mutated source")
			}
			matches, _ := filepath.Glob(filepath.Join(f.store.root, "inspect-*"))
			if len(matches) != 0 {
				t.Fatal("scratch leaked", matches)
			}
		})
	}
}

func TestWorkspaceRecoveryOversizeRefusalKeepsOtherWorkspaceUsable(t *testing.T) {
	f, c, operator, token, r := creationFixture(t)
	defer f.store.Close()
	ctx := context.Background()
	aCreated, err := c.CreateWorkspace(ctx, operator, r)
	if err != nil {
		t.Fatal(err)
	}
	bReq := r
	bReq.OperationID = "61000000-0000-4000-8000-000000000002"
	bReq.RootLocator = filepath.Join(f.allowed, "other")
	operator, token = authOperator(t, f)
	bCreated, err := c.CreateWorkspace(ctx, operator, bReq)
	if err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(r.RootLocator, "data", "workspace.sqlite-journal")
	out, err := os.Create(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err = out.Truncate(maxRecoveryScratchBytes + 1); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
	before := inspectionTree(t, r.RootLocator)
	a, err := f.auth.Authorize(ctx, token, aCreated.WorkspaceID, "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	opener, _ := NewWorkspaceOpener(f.auth)
	if v, e := opener.OpenWorkspace(ctx, a, 1); !errors.Is(e, errRecoveryBudget) || v.Epoch != "" {
		t.Fatalf("budget refusal %+v %v", v, e)
	}
	if !reflect.DeepEqual(before, inspectionTree(t, r.RootLocator)) {
		t.Fatal("budget refusal mutated source")
	}
	matches, _ := filepath.Glob(filepath.Join(f.store.root, "inspect-*"))
	if len(matches) != 0 {
		t.Fatal("oversize allocated scratch", matches)
	}
	b, err := f.auth.Authorize(ctx, token, bCreated.WorkspaceID, "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	if v, e := opener.OpenWorkspace(ctx, b, 1); e != nil || v.Epoch == "" {
		t.Fatalf("B blocked %+v %v", v, e)
	}
	closer, err := f.auth.Authorize(ctx, token, bCreated.WorkspaceID, "workspace.close")
	if err != nil {
		t.Fatal(err)
	}
	if err = opener.DrainWorkspace(ctx, closer); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryWriterBudgetAndCancellation(t *testing.T) {
	var out bytes.Buffer
	remaining := int64(3)
	w := recoveryWriter{context.Background(), &out, &remaining}
	if n, err := w.Write([]byte("ab")); n != 2 || err != nil || remaining != 1 {
		t.Fatal(n, err, remaining)
	}
	if n, err := w.Write([]byte("cd")); n != 0 || !errors.Is(err, errRecoveryBudget) || out.String() != "ab" {
		t.Fatal(n, err, out.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.ctx = ctx
	if n, err := w.Write([]byte("e")); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
	var header [28]byte
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = validateRollbackRecords(ctx, f, 100, header); !errors.Is(err, context.Canceled) {
		t.Fatal("parser ignored cancellation", err)
	}
}

func TestRecoveryScratchCleanupRetriesTransientDelete(t *testing.T) {
	f, _, _, _, _ := creationFixture(t)
	defer f.store.Close()
	scratch, err := platform.NewScratch(f.store.root, "inspect-"+string(bytes.Repeat([]byte("a"), 48)))
	if err != nil {
		t.Fatal(err)
	}
	out, err := scratch.CreateFile("workspace.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
	name, err := win.UTF16PtrFromString(filepath.Join(scratch.Path(), "workspace.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := win.CreateFile(name, win.GENERIC_READ, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE, nil, win.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	r := &openResources{store: f.store, recoveryScratch: scratch}
	if err = r.closeRecoveryScratch(); err == nil {
		win.CloseHandle(blocker)
		t.Fatal("delete-denying handle ignored")
	}
	if r.recoveryScratch == nil || r.recoveryScratchDeleted {
		win.CloseHandle(blocker)
		t.Fatal("cleanup falsely completed")
	}
	if _, err = scratch.CreateFile("descriptor.json"); err == nil {
		win.CloseHandle(blocker)
		t.Fatal("create allowed after cleanup began")
	}
	if err = win.CloseHandle(blocker); err != nil {
		t.Fatal(err)
	}
	if err = r.closeRecoveryScratch(); err != nil {
		t.Fatal("cleanup not retryable", err)
	}
	if r.recoveryScratch != nil {
		t.Fatal("scratch still retained")
	}
	if err = r.closeRecoveryScratch(); err != nil {
		t.Fatal("cleanup not idempotent", err)
	}
}

// Retained in the output so each run carries concrete source-byte evidence.
func TestRollbackHeaderRejectsJunk(t *testing.T) {
	dir := filepath.Join("..", "..", ".data", "logs", "001-validation", fmt.Sprintf("journal-header-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "workspace.sqlite-journal")
	payload := []byte("not an SQLite journal")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateRollbackJournal(path); err == nil {
		t.Fatal("junk accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a, b := sha256.Sum256(payload), sha256.Sum256(got)
	if a != b {
		t.Fatal("shape gate mutated journal")
	}
	evidence, _ := json.Marshal(map[string]string{"sha256": hex.EncodeToString(a[:])})
	t.Log(string(evidence))
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
