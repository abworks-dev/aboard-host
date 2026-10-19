//go:build windows && amd64

package windows

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryFreezeRetriesReuseValidatedDataPin(t *testing.T) {
	for _, prePinned := range []bool{false, true} {
		t.Run(map[bool]string{false: "freeze-first", true: "ordinary-open-pin"}[prePinned], func(t *testing.T) {
			root := gateRoot(t)
			if err := os.Mkdir(filepath.Join(root, "data"), 0700); err != nil {
				t.Fatal(err)
			}
			readyFile(t, filepath.Join(root, "data", "workspace.sqlite"), []byte("synthetic frozen bytes, not SQLite validation"))
			lease, err := Acquire(root, Workspace, false, false)
			if err != nil {
				t.Fatal(err)
			}
			defer mustClose(t, lease)
			if prePinned {
				pin, err := lease.PinFile(`data\workspace.sqlite`, false)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := pin.Close(); err != nil {
						t.Error(err)
					}
				}()
			}
			for i := 0; i < 128; i++ {
				frozen, err := lease.FreezeWorkspaceOpenInputs()
				if err != nil {
					t.Fatal(i, err)
				}
				if err = frozen.Close(); err != nil {
					t.Fatal(err)
				}
				count := 0
				for _, pin := range lease.pins {
					if sameDOSSpelling(pin.path, filepath.Join(root, "data")) {
						count++
					}
				}
				if count != 1 {
					t.Fatal("retry accumulated namespace pins", i, count)
				}
				if err = lease.Validate(); err != nil {
					t.Fatal("closing temporary input invalidated retained namespace", err)
				}
			}
		})
	}
}
