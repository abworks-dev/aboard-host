//go:build windows && amd64

package windows

import (
	"os"
	"path/filepath"
	"testing"

	win "golang.org/x/sys/windows"
)

// Authorization deliberately requires exact root spelling. A native pin must
// not turn that conservative policy into a Unicode/case-insensitive lookup.
// This proves rejection on the actual NTFS profile, not an OS sandbox or a test
// that enables case-sensitive directories/system features.
func TestDirectoryGateDOSSpelling(t *testing.T) {
	for _, pair := range [][2]string{{`E:\root\Name`, `e:\root\Name`}, {`E:\root\中文`, `E:\root\中文`}} {
		if !sameDOSSpelling(pair[0], pair[1]) {
			t.Fatalf("exact spelling rejected: %q", pair)
		}
	}
	for _, pair := range [][2]string{{`E:\root\Name`, `E:\ROOT\Name`}, {`E:\root\Name`, `E:\root\name`}, {`E:\root\K`, `E:\root\K`}, {`E:\root\Ä`, `E:\root\ä`}, {`E:\root`, `X:\root`}, {`root`, `root`}} {
		if sameDOSSpelling(pair[0], pair[1]) {
			t.Fatalf("unproven spelling equivalence: %q", pair)
		}
	}
}

func TestDirectoryGateConservativeSpelling(t *testing.T) {
	root := filepath.Join(syntheticDir(t), "CanonicalMixedCase")
	if err := os.MkdirAll(filepath.Join(root, ".aboard", "locks"), 0700); err != nil {
		t.Fatal(err)
	}
	lease, err := Acquire(root, Workspace, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(filepath.Dir(root), "canonicalmixedcase")
	other, err := Acquire(alias, Workspace, false, false)
	if other != nil {
		defer other.Close()
	}
	if err == nil {
		t.Fatal("unproven case alias accepted by native pins")
	}
	// Original exact spelling still resolves and owns the same existing lock.
	lease, err = Acquire(root, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	var rootPin *File
	for _, pin := range lease.pins {
		if pin.path == root {
			rootPin = pin
			break
		}
	}
	if rootPin == nil {
		t.Fatal("missing retained root")
	}
	child, err := openRelativePinned(rootPin, ".ABOARD", win.FILE_READ_ATTRIBUTES|win.FILE_LIST_DIRECTORY, win.FILE_SHARE_READ, win.OPEN_EXISTING, true)
	if child != nil {
		defer child.Close()
	}
	if err == nil {
		t.Fatal("unproven child spelling alias accepted by native pins")
	}
}
