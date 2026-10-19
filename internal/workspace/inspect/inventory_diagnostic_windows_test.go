//go:build windows && amd64

package inspect

import (
	"errors"
	win "golang.org/x/sys/windows"
	"testing"
)

// This diagnostic intentionally performs ONLY the pre-inspection inventory
// inside the observation window. It never calls Inspect, and does not require
// that a different NTFS version produce a read-related metadata notification.
// It does not weaken any required zero-change assertion around actual Inspect.
func TestInspectionInventoryNotificationDiagnostic(t *testing.T) {
	root, _ := inspectionFixture(t)
	watch := startWatch(t, root)
	defer win.CloseHandle(watch.handle)
	defer win.CloseHandle(watch.event)
	before := ioSnapshot(t)
	_ = inventory(t, root)
	after := ioSnapshot(t)
	state, err := win.WaitForSingleObject(watch.event, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if state == uint32(win.WAIT_TIMEOUT) {
		if err = win.CancelIoEx(watch.handle, &watch.overlapped); err != nil && !errors.Is(err, win.ERROR_NOT_FOUND) {
			t.Fatal(err)
		}
	}
	var n uint32
	err = win.GetOverlappedResult(watch.handle, &watch.overlapped, &n, true)
	if err != nil && !errors.Is(err, win.ERROR_OPERATION_ABORTED) {
		t.Fatal(err)
	}
	t.Logf("inventory ONLY, no Inspect called: actual writes/bytes=%d/%d; notifications=%d error=%v %s", after.Writes-before.Writes, after.WriteBytes-before.WriteBytes, n, err, decodeNotifications(watch.buffer[:n]))
	if after.Writes-before.Writes != 0 || after.WriteBytes-before.WriteBytes != 0 {
		t.Fatal("inventory unexpectedly performed counted data writes")
	}
}
