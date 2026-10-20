//go:build windows && amd64

package windows

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	win "golang.org/x/sys/windows"
)

func TestNativeFactoryCloseUnknownRetainsAcquiredHandle(t *testing.T) {
	root := t.TempDir()
	name, err := win.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	h, err := win.CreateFile(name, win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ, nil, win.OPEN_EXISTING, win.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = win.SetHandleInformation(h, 2, 2); err != nil {
		_ = win.CloseHandle(h)
		t.Fatal(err)
	}
	defer func() { _ = win.SetHandleInformation(h, 2, 0); _ = win.CloseHandle(h) }()
	// A directory cannot satisfy the file contract; its failed native cleanup
	// must travel with the nil result, not disappear behind the validation error.
	file, err := adoptPinned(h, root, false)
	if file != nil || !errors.Is(err, ErrPhysicalCloseUnknown) {
		t.Fatal("factory lost failed-close ownership", file, err)
	}
	var retained *physicalCloseError
	if !errors.As(err, &retained) || len(retained.retained) == 0 {
		t.Fatal("marker does not strongly retain original handle")
	}
	var info win.ByHandleFileInformation
	if err = win.GetFileInformationByHandle(h, &info); err != nil {
		t.Fatal("protected native handle was not live", err)
	}
}

func TestNativeAncestorCloseFailureKeepsWriterExclusion(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".aboard", "locks"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".aboard", "locks", "writer.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	lease, err := Acquire(root, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	h := lease.pins[len(lease.pins)-1].handle
	if err = win.SetHandleInformation(h, 2, 2); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = win.SetHandleInformation(h, 2, 0)
		// Test controls the exact protected handle; this is not a Host recovery API.
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err = lease.Close(); !errors.Is(err, ErrPhysicalCloseUnknown) {
		t.Fatal(err)
	}
	var info win.ByHandleFileInformation
	if err = win.GetFileInformationByHandle(lease.lock.handle, &info); err != nil {
		t.Fatal("ancestor failure released writer lock", err)
	}
	if another, e := Acquire(root, Workspace, false, false); e == nil {
		_ = another.Close()
		t.Fatal("second writer admitted on unknown ancestor")
	}
}
