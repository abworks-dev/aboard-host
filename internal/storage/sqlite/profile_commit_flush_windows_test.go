//go:build windows && amd64

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"modernc.org/libc"
	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Only this test binary can replace the VFS syscall. No production fault hook,
// environment switch, second driver or dependency source modification exists.
// Tests are serial: xSetSystemCall changes the process-wide win32 VFS table.
var flushFault struct {
	handle uintptr
	at     int32
	calls  atomic.Int32
	failed atomic.Int32
}

func commitFlushStimulus(tls *libc.TLS, actualHandle uintptr) int32 {
	n := flushFault.calls.Add(1)
	if n == flushFault.at {
		flushFault.failed.Add(1)
		// A real Windows flush attempt on a real, read-only synthetic file.
		// This exercises the corrected bridge, not a fabricated SQLite error.
		return libc.XFlushFileBuffers(tls, flushFault.handle)
	}
	return libc.XFlushFileBuffers(tls, actualHandle)
}

func replaceFlushForGate(t *testing.T) func() {
	t.Helper()
	tls := libc.NewTLS()
	name, err := libc.CString("win32")
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, name)
	vfs := sqlite3.Xsqlite3_vfs_find(tls, name)
	if vfs == 0 {
		t.Fatal("required win32 VFS absent")
	}
	var view sqlite3.Tsqlite3_vfs
	// Copy a C allocation into a pointer-free Go layout. All ABI-dependent
	// operations stay in this test and use the exact frozen library's types.
	libc.Xmemcpy(tls, uintptr(unsafe.Pointer(&view)), vfs, libc.Tsize_t(unsafe.Sizeof(view)))
	if view.FiVersion < 3 || view.FxSetSystemCall == 0 || view.FxGetSystemCall == 0 {
		t.Fatal("required VFS syscall inspection unavailable")
	}
	get := *(*func(*libc.TLS, uintptr, uintptr) uintptr)(unsafe.Pointer(&view.FxGetSystemCall))
	set := *(*func(*libc.TLS, uintptr, uintptr, uintptr) int32)(unsafe.Pointer(&view.FxSetSystemCall))
	syscallName, err := libc.CString("FlushFileBuffers")
	if err != nil {
		t.Fatal(err)
	}
	original := get(tls, vfs, syscallName)
	if original == 0 {
		t.Fatal("VFS flush syscall unavailable")
	}
	callback := commitFlushStimulus
	pointer := *(*uintptr)(unsafe.Pointer(&callback))
	if rc := set(tls, vfs, syscallName, pointer); rc != sqlite3.SQLITE_OK {
		t.Fatalf("cannot install isolated test stimulus: %d", rc)
	}
	var once sync.Once
	restore := func() {
		once.Do(func() {
			if rc := set(tls, vfs, syscallName, original); rc != sqlite3.SQLITE_OK {
				t.Errorf("cannot restore original flush syscall: %d", rc)
			}
			libc.Xfree(tls, syscallName)
			tls.Close()
		})
	}
	t.Cleanup(restore)
	return restore
}

func TestProfileGateCommitFlushFailure(t *testing.T) {
	root := t.TempDir()
	stimulusPath := filepath.Join(root, "synthetic-flush-error-input.bin")
	if err := os.WriteFile(stimulusPath, []byte("synthetic readonly flush"), 0600); err != nil {
		t.Fatal(err)
	}
	stimulus, err := os.Open(stimulusPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stimulus.Close()
	if err := windows.FlushFileBuffers(windows.Handle(stimulus.Fd())); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("real native error stimulus unavailable: %v", err)
	}
	// First measure all flushes in a genuine successful commit, then fail each
	// observed ordinal separately. This covers both journal and DB flush phases
	// without asserting undocumented fixed counts for future versions.
	flushCount := int32(0)
	for ordinal := int32(0); ordinal == 0 || ordinal <= flushCount; ordinal++ {
		t.Run(fmt.Sprintf("commit-flush-%d", ordinal), func(t *testing.T) {
			path := filepath.Join(root, fmt.Sprintf("synthetic-%d.sqlite", ordinal))
			h := openGate(t, path, CreateNew)
			execGate(t, h, "CREATE TABLE notes(id INTEGER PRIMARY KEY, body TEXT NOT NULL)")
			execGate(t, h, "CREATE TABLE receipts(id INTEGER PRIMARY KEY, result TEXT NOT NULL)")
			tx, err := h.Conn.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec("INSERT INTO notes VALUES(1,'synthetic pending')"); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec("INSERT INTO receipts VALUES(1,'synthetic result')"); err != nil {
				t.Fatal(err)
			}
			flushFault.handle = stimulus.Fd()
			flushFault.at = ordinal
			flushFault.calls.Store(0)
			flushFault.failed.Store(0)
			restore := replaceFlushForGate(t)
			err = tx.Commit() // real database/sql -> driver -> SQLite pager -> win32 VFS
			calls, failures := flushFault.calls.Load(), flushFault.failed.Load()
			restore() // restore before close, recovery, or any next connection
			if ordinal == 0 {
				if err != nil || calls < 1 || calls > 8 {
					t.Fatalf("baseline commit/flush evidence missing: calls=%d err=%v", calls, err)
				}
				flushCount = calls
			} else {
				var sqlErr *driver.Error
				if failures != 1 || !errors.As(err, &sqlErr) || sqlErr.Code() != sqlite3.SQLITE_IOERR_FSYNC {
					t.Fatalf("flush error falsely acknowledged/lost: ordinal=%d calls=%d failures=%d err=%v", ordinal, calls, failures, err)
				}
			}
			t.Logf("real COMMIT ordinal=%d calls=%d native failures=%d result=%v", ordinal, calls, failures, err)
			closeGate(t, h)
			// Only after physical connection closure and a fresh open investigate
			// both rows. An error is NOT treated as an automatic rollback/retry.
			h = openGate(t, path, ReadWriteExisting)
			defer closeGate(t, h)
			var notes, receipts int
			if err := h.Conn.QueryRowContext(context.Background(), "SELECT count(*) FROM notes").Scan(&notes); err != nil {
				t.Fatal(err)
			}
			if err := h.Conn.QueryRowContext(context.Background(), "SELECT count(*) FROM receipts").Scan(&receipts); err != nil {
				t.Fatal(err)
			}
			if notes != receipts || (notes != 0 && notes != 1) {
				t.Fatalf("torn business/receipt transaction: %d/%d", notes, receipts)
			}
			if ordinal == 0 && notes != 1 {
				t.Fatal("confirmed baseline lost")
			}
			t.Logf("fresh physical view after close: business=%d receipt=%d; no automatic replay", notes, receipts)
		})
	}
}
