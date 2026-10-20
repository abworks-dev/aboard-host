//go:build windows && amd64

package acceptance

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	win "golang.org/x/sys/windows"
)

// Recursive native NTFS notifications supplement exact inventories. This is a
// bounded metadata/change observation, not attribution of failed write attempts.
type inspectionWatch struct {
	handle, event win.Handle
	overlapped    win.Overlapped
	buffer        [65536]byte
	done          bool
	pin           runtime.Pinner
}

func startInspectionWatch(t *testing.T, root string) *inspectionWatch {
	t.Helper()
	if !within(logRoot(), root) || !unreparsed(root) {
		t.Fatal("unowned observer source")
	}
	path, err := win.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	h, err := win.CreateFile(path, win.FILE_LIST_DIRECTORY, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE|win.FILE_SHARE_DELETE, nil, win.OPEN_EXISTING, win.FILE_FLAG_BACKUP_SEMANTICS|win.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		t.Fatal("native observer open", err)
	}
	e, err := win.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = win.CloseHandle(h)
		t.Fatal(err)
	}
	w := &inspectionWatch{handle: h, event: e}
	w.overlapped.HEvent = e
	// Native asynchronous IO outlives the issuing syscall. Pin both buffers and
	// keep its issuing thread alive until cancellation/completion is joined.
	w.pin.Pin(&w.buffer[0])
	w.pin.Pin(&w.overlapped)
	runtime.LockOSThread()
	mask := uint32(win.FILE_NOTIFY_CHANGE_FILE_NAME | win.FILE_NOTIFY_CHANGE_DIR_NAME | win.FILE_NOTIFY_CHANGE_SIZE | win.FILE_NOTIFY_CHANGE_LAST_WRITE | win.FILE_NOTIFY_CHANGE_ATTRIBUTES | win.FILE_NOTIFY_CHANGE_SECURITY)
	err = win.ReadDirectoryChanges(h, &w.buffer[0], uint32(len(w.buffer)), true, mask, nil, &w.overlapped, 0)
	if err != nil && !errors.Is(err, win.ERROR_IO_PENDING) {
		_ = win.CloseHandle(e)
		_ = win.CloseHandle(h)
		w.pin.Unpin()
		runtime.UnlockOSThread()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !w.done {
			w.finish(t, false)
		}
	})
	return w
}
func (w *inspectionWatch) finish(t *testing.T, wantChange bool) {
	t.Helper()
	if w.done {
		return
	}
	w.done = true
	state, waitErr := win.WaitForSingleObject(w.event, 2000)
	var cancelErr error
	if waitErr != nil || state == uint32(win.WAIT_TIMEOUT) {
		cancelErr = win.CancelIoEx(w.handle, &w.overlapped)
	}
	var n uint32
	err := win.GetOverlappedResult(w.handle, &w.overlapped, &n, true)
	eventErr, handleErr := win.CloseHandle(w.event), win.CloseHandle(w.handle)
	runtime.KeepAlive(w)
	w.pin.Unpin()
	runtime.UnlockOSThread()
	if waitErr != nil || (cancelErr != nil && !errors.Is(cancelErr, win.ERROR_NOT_FOUND)) || eventErr != nil || handleErr != nil {
		t.Fatal("native observation did not physically join/close", waitErr, cancelErr, eventErr, handleErr)
	}
	if wantChange {
		if err != nil || n == 0 {
			t.Fatal("observer positive control failed", n, err)
		}
	} else if n != 0 || !errors.Is(err, win.ERROR_OPERATION_ABORTED) {
		t.Fatal("unexpected source notification or lost/overflowed observation", n, err)
	}
	t.Logf("recursive NTFS observation: expectedChange=%v bytes=%d; bounded 2s window, no failed-attempt/global call trace", wantChange, n)
}
func TestV009DirectoryObserverControls(t *testing.T) {
	f := newFixture(t)
	root := filepath.Join(f.config.AllowedRoots[0], "observation-controls")
	if os.Mkdir(root, 0700) != nil {
		t.Fatal("control root")
	}
	for _, kind := range []string{"same-bytes", "create-delete", "rename", "attributes", "nested-child"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(root, "control-"+kind)
			if kind == "nested-child" {
				dir := filepath.Join(root, "nested")
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(dir, "existing")
			}
			if err := os.WriteFile(path, []byte("same synthetic bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			watch := startInspectionWatch(t, root)
			var err error
			switch kind {
			case "same-bytes", "nested-child":
				var file *os.File
				file, err = os.OpenFile(path, os.O_WRONLY, 0)
				if err == nil {
					_, writeErr := file.WriteAt([]byte("same synthetic bytes"), 0)
					err = errors.Join(writeErr, file.Close())
				}
			case "create-delete":
				var file *os.File
				file, err = os.OpenFile(path+"-transient", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err == nil {
					err = file.Close()
				}
				if err == nil {
					err = os.Remove(path + "-transient")
				}
			case "rename":
				if !within(root, path) {
					t.Fatal("unowned control rename")
				}
				err = os.Rename(path, path+"-renamed")
			case "attributes":
				var p *uint16
				p, err = win.UTF16PtrFromString(path)
				if err == nil {
					err = win.SetFileAttributes(p, win.FILE_ATTRIBUTE_HIDDEN)
				}
			}
			if err != nil {
				t.Fatal("control mutation failed", err)
			}
			watch.finish(t, true)
		})
	}
}

func TestV009DirectoryObserverIdleCancellation(t *testing.T) {
	f := newFixture(t)
	root := filepath.Join(f.config.AllowedRoots[0], "idle-observer")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 4; n++ {
		w := startInspectionWatch(t, root)
		runtime.GC() // exercise asynchronous buffer lifetime before native completion
		w.finish(t, false)
	}
}
