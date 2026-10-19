//go:build windows && amd64

package inspect

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/abworks-dev/aboard-host/internal/digest"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	store "github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
)

func inspectionFixture(t *testing.T) (root, parent string) {
	t.Helper()
	base := t.TempDir()
	normalized := strings.ToLower(filepath.ToSlash(base))
	if !strings.Contains(normalized, "/.data/logs/001-validation/") {
		t.Fatal("synthetic root only; use project runner")
	}
	root, parent = filepath.Join(base, "source #&中文"), filepath.Join(base, "private")
	if err := os.MkdirAll(filepath.Join(root, ".aboard", "locks"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".aboard", "locks", "writer.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	identityDigest, err := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID("11111111-1111-4111-8111-111111111111"), TenantID: protocol.UUID("22222222-2222-4222-8222-222222222222"), DatabaseRelativePath: "data/workspace.sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := fmt.Sprintf(`{"formatVersion":2,"workspaceId":"11111111-1111-4111-8111-111111111111","tenantId":"22222222-2222-4222-8222-222222222222","databaseRelativePath":"data/workspace.sqlite","identityDigestVersion":1,"identityDigest":"%s"}`, identityDigest)
	if err := os.WriteFile(filepath.Join(root, "aboard.workspace.json"), []byte(descriptor), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := store.Open(context.Background(), filepath.Join(root, "data", "workspace.sqlite"), store.CreateNew)
	if err != nil {
		t.Fatal(err)
	}
	_, sqlErr := h.Conn.ExecContext(context.Background(), "CREATE TABLE probe(body TEXT NOT NULL); INSERT INTO probe VALUES ('你好\r\nsynthetic'); PRAGMA user_version=1")
	if err = errors.Join(sqlErr, h.Close()); err != nil {
		t.Fatal(err)
	}
	if err = platform.CreatePrivateDirectory(parent); err != nil {
		t.Fatal(err)
	}
	return root, parent
}

func inventory(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			result[rel+"/"] = sha256.Sum256(nil)
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("unexpected symlink in synthetic inventory")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[rel] = sha256.Sum256(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

// Directory changes are delivered by NTFS, not a Go write mock. The controlled
// same-byte write below establishes sensitivity; this detects completed source
// mutations, not failed WriteFile attempts or a universal forensic IO trace.
type sourceWatch struct {
	handle     win.Handle
	event      win.Handle
	overlapped win.Overlapped
	buffer     [65536]byte
}

func startWatch(t *testing.T, root string) *sourceWatch {
	t.Helper()
	name, err := win.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	h, err := win.CreateFile(name, win.FILE_LIST_DIRECTORY, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE|win.FILE_SHARE_DELETE, nil, win.OPEN_EXISTING, win.FILE_FLAG_BACKUP_SEMANTICS|win.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		t.Fatal(err)
	}
	e, err := win.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = win.CloseHandle(h)
		t.Fatal(err)
	}
	w := &sourceWatch{handle: h, event: e}
	w.overlapped.HEvent = e
	mask := uint32(win.FILE_NOTIFY_CHANGE_FILE_NAME | win.FILE_NOTIFY_CHANGE_DIR_NAME | win.FILE_NOTIFY_CHANGE_SIZE | win.FILE_NOTIFY_CHANGE_LAST_WRITE | win.FILE_NOTIFY_CHANGE_ATTRIBUTES | win.FILE_NOTIFY_CHANGE_SECURITY)
	err = win.ReadDirectoryChanges(h, &w.buffer[0], uint32(len(w.buffer)), true, mask, nil, &w.overlapped, 0)
	if err != nil && !errors.Is(err, win.ERROR_IO_PENDING) {
		_ = win.CloseHandle(e)
		_ = win.CloseHandle(h)
		t.Fatal(err)
	}
	return w
}
func (w *sourceWatch) finish(t *testing.T, wantChange bool) {
	t.Helper()
	defer win.CloseHandle(w.handle)
	defer win.CloseHandle(w.event)
	// Equal delivery windows for positive and negative observations. This is
	// a bounded notification observation, not a universal latency guarantee.
	wait := uint32(2000)
	state, err := win.WaitForSingleObject(w.event, wait)
	if err != nil {
		t.Fatal(err)
	}
	if state == uint32(win.WAIT_TIMEOUT) {
		if err = win.CancelIoEx(w.handle, &w.overlapped); err != nil && !errors.Is(err, win.ERROR_NOT_FOUND) {
			t.Fatal(err)
		}
	}
	var n uint32
	err = win.GetOverlappedResult(w.handle, &w.overlapped, &n, true)
	if wantChange {
		if err != nil || n == 0 {
			t.Fatalf("NTFS observer failed positive control: bytes=%d error=%v", n, err)
		}
	} else if !errors.Is(err, win.ERROR_OPERATION_ABORTED) || n != 0 {
		t.Fatalf("source mutation or observer failure: bytes=%d error=%v events=%s", n, err, decodeNotifications(w.buffer[:n]))
	}
	t.Logf("NTFS change observer: expectedChange=%v bytes=%d completion=%v", wantChange, n, err)
}

// Diagnostic decoding only; never filters or excuses an unexpected event.
func decodeNotifications(buf []byte) string {
	var events []string
	for offset := 0; offset+12 <= len(buf); {
		next := int(binary.LittleEndian.Uint32(buf[offset:]))
		action := binary.LittleEndian.Uint32(buf[offset+4:])
		size := int(binary.LittleEndian.Uint32(buf[offset+8:]))
		if size%2 != 0 || size < 0 || size > len(buf)-offset-12 {
			return "invalid notification payload"
		}
		units := make([]uint16, size/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(buf[offset+12+i*2:])
		}
		events = append(events, fmt.Sprintf("action=%d name=%q", action, string(utf16.Decode(units))))
		if next == 0 {
			break
		}
		if next < 12 || next > len(buf)-offset {
			return "invalid notification offset"
		}
		offset += next
	}
	return strings.Join(events, "; ")
}

func TestInspectionGateFrozenCopy(t *testing.T) {
	root, parent := inspectionFixture(t)
	// Real same-byte overwrite/close must be visible before relying on a zero
	// notification result. Do not weaken this to an inventory-only assertion.
	control := startWatch(t, root)
	path := filepath.Join(root, "aboard.workspace.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0) // OPEN_EXISTING, never O_TRUNC
	if err != nil {
		t.Fatal(err)
	}
	n, writeErr := f.WriteAt(data, 0)
	if closeErr := f.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if writeErr != nil || n != len(data) {
		t.Fatal(n, writeErr)
	}
	control.finish(t, true)
	before := inventory(t, root)
	watch := startWatch(t, root)
	ioBefore := ioSnapshot(t)
	report, err := Inspect(context.Background(), root, parent)
	ioAfter := ioSnapshot(t) // before test output, includes every Inspect defer
	checkScratchConservation(t, ioBefore, ioAfter, report)
	watch.finish(t, false)
	if err != nil || !report.IntegrityOK || report.DatabaseUserVersion != 1 || report.ScratchCleanupFailed {
		t.Fatalf("storage inspect: %+v %v", report, err)
	}
	if after := inventory(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("source inventory/bytes changed")
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
		t.Fatalf("private cache not destroyed: count=%d error=%v", len(entries), err)
	}
}

func TestInspectionGateRejectedSources(t *testing.T) {
	for _, kind := range []string{"writer-held", "missing-lock", "missing-database", "rollback-journal", "wal", "shm", "corrupt-database", "missing-descriptor", "empty-database", "unsafe-scratch-acl", "nonexistent-source"} {
		t.Run(kind, func(t *testing.T) {
			root, parent := inspectionFixture(t)
			switch kind {
			case "nonexistent-source": // query an absent child; watch the existing source ancestor
			case "unsafe-scratch-acl":
				parent = filepath.Join(filepath.Dir(parent), "unsafe-parent")
				if err := os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
				if err := platform.CheckPrivateDirectory(parent); err == nil {
					t.Fatal("unsafe ACL stimulus unavailable")
				}
			case "empty-database":
				if err := os.WriteFile(filepath.Join(root, "data", "workspace.sqlite"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "writer-held":
				// Snapshot first; an honest share=0 lease must deny even the
				// test inventory's attempt to reopen its lock object.
			case "missing-lock":
				if err := os.Remove(filepath.Join(root, ".aboard", "locks", "writer.lock")); err != nil {
					t.Fatal(err)
				}
			case "missing-database":
				if err := os.Remove(filepath.Join(root, "data", "workspace.sqlite")); err != nil {
					t.Fatal(err)
				}
			case "missing-descriptor":
				if err := os.Remove(filepath.Join(root, "aboard.workspace.json")); err != nil {
					t.Fatal(err)
				}
			case "corrupt-database":
				if err := os.WriteFile(filepath.Join(root, "data", "workspace.sqlite"), []byte("synthetic corrupt SQLite"), 0600); err != nil {
					t.Fatal(err)
				}
			default:
				suffix := map[string]string{"rollback-journal": "-journal", "wal": "-wal", "shm": "-shm"}[kind]
				if err := os.WriteFile(filepath.Join(root, "data", "workspace.sqlite"+suffix), []byte("synthetic sidecar"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := inventory(t, root)
			var held *platform.Lease
			if kind == "writer-held" {
				var err error
				held, err = platform.Acquire(root, platform.Workspace, false, false)
				if err != nil {
					t.Fatal(err)
				}
				defer held.Close()
			}
			watch := startWatch(t, root)
			ioBefore := ioSnapshot(t)
			target := root
			if kind == "nonexistent-source" {
				target = filepath.Join(root, "never-created")
			}
			report, err := Inspect(context.Background(), target, parent)
			ioAfter := ioSnapshot(t)
			checkScratchConservation(t, ioBefore, ioAfter, report)
			watch.finish(t, false)
			if held != nil {
				if closeErr := held.Close(); closeErr != nil {
					t.Fatal(closeErr)
				}
			}
			if err == nil {
				t.Fatal("unsafe source accepted")
			}
			if !reflect.DeepEqual(before, inventory(t, root)) {
				t.Fatal("refusal modified source")
			}
		})
	}
}

func TestInspectionGateScratchAndCancellation(t *testing.T) {
	root, parent := inspectionFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Inspect(ctx, root, parent); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Inspect(context.Background(), root, root); err == nil {
		t.Fatal("source used as scratch")
	}
	var retained string
	var cleanupPinsProtected bool
	var blocker win.Handle
	before := inventory(t, root)
	watch := startWatch(t, root)
	ioBefore := ioSnapshot(t)
	report, err := inspect(context.Background(), root, parent, func(path string) error {
		// This is the exact vulnerable interval: root pin released, deletion
		// not started. The known synthetic cache anchor must stay immovable.
		if !filepath.IsAbs(parent) || !within(filepath.Dir(root), parent) {
			return errors.New("synthetic rename target not checked")
		}
		moveErr := os.Rename(parent, parent+"-moved")
		if moveErr == nil {
			return errors.New("scratch parent moved during cleanup")
		}
		if !errors.Is(moveErr, win.ERROR_SHARING_VIOLATION) && !errors.Is(moveErr, win.ERROR_ACCESS_DENIED) {
			return moveErr
		}
		cleanupPinsProtected = true
		retained = path
		name, nameErr := win.UTF16PtrFromString(filepath.Join(path, "workspace.sqlite"))
		if nameErr != nil {
			return nameErr
		}
		var openErr error
		blocker, openErr = win.CreateFile(name, win.GENERIC_READ, 0, nil, win.OPEN_EXISTING, 0, 0)
		if openErr != nil {
			return openErr
		}
		t.Cleanup(func() {
			if blocker != 0 {
				_ = win.CloseHandle(blocker)
			}
		})
		return os.RemoveAll(path) // actual sharing-violation refusal, not a fabricated cleanup error
	})
	ioAfter := ioSnapshot(t)
	checkScratchConservation(t, ioBefore, ioAfter, report)
	watch.finish(t, false)
	if !cleanupPinsProtected {
		t.Fatal("scratch anchor rename protection failed during cleanup")
	}
	if err == nil || !report.ScratchCleanupFailed || !report.IntegrityOK {
		t.Fatalf("cleanup must be independent: %+v %v", report, err)
	}
	if retained == "" || !within(parent, retained) {
		t.Fatal("bad retained cache path")
	}
	if err = platform.CheckPrivateDirectory(retained); err != nil {
		t.Fatal(err)
	}
	if blocker == 0 {
		t.Fatal("real cleanup fault stimulus unavailable")
	}
	if closeErr := win.CloseHandle(blocker); closeErr != nil {
		t.Fatal(closeErr)
	}
	blocker = 0
	if err = os.RemoveAll(retained); err != nil {
		t.Fatal(err)
	} // exact freshly returned synthetic path checked above
	if !reflect.DeepEqual(before, inventory(t, root)) {
		t.Fatal("cleanup failure modified source")
	}
	// Same checked absolute synthetic anchor/token: once all pins are released
	// the identical rename must succeed, proving the earlier stimulus was valid.
	if !filepath.IsAbs(parent) || !within(filepath.Dir(root), parent) {
		t.Fatal("unchecked rename target")
	}
	if err = os.Rename(parent, parent+"-moved"); err != nil {
		t.Fatal("post-release rename positive control", err)
	}
	if err = os.Rename(parent+"-moved", parent); err != nil {
		t.Fatal("restore known synthetic anchor", err)
	}
	t.Log(fmt.Sprint("cleanup refusal reported separately; private artifact explicitly removed; rename succeeds only after pin release"))
}
