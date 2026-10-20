//go:build windows && amd64

package inspect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	store "github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
)

func assertInspectionExcluded(t *testing.T, root string) {
	t.Helper()
	lease, err := platform.Acquire(root, platform.Workspace, false, false)
	if err == nil {
		_ = lease.Close()
		t.Fatal("inspection released source exclusion before cleanup")
	}
}

func TestInspectionOwnedNativeStreamCloseFailure(t *testing.T) {
	root, parent := inspectionFixture(t)
	before := inventory(t, root)
	r := &Resources{cleanup: os.RemoveAll}
	var protected win.Handle
	attempts := 0
	r.closeStream = func(f *os.File) error {
		if f.Name() == "frozen-source" && protected == 0 {
			protected = win.Handle(f.Fd())
			if err := win.SetHandleInformation(protected, 2, 2); err != nil {
				return err
			} // PROTECT_FROM_CLOSE
			attempts++
			return f.Close() // real native refusal, not a simulated errno
		}
		return f.Close()
	}
	// The exact protected handle cannot have been closed/reused. Test teardown
	// removes the protection and closes it directly; product code never does this.
	defer func() {
		if protected != 0 {
			if err := win.SetHandleInformation(protected, 2, 0); err != nil {
				t.Error(err)
				return
			}
			if err := win.CloseHandle(protected); err != nil {
				t.Error(err)
				return
			}
		}
		for _, s := range r.streams {
			if s.result != nil {
				s.result = nil
			}
		}
		r.poison = nil // controlled test-only teardown AFTER exact native handle ended
		if err := r.Close(); err != nil {
			t.Error(err)
		}
		if !equalInventory(before, inventory(t, root)) {
			t.Error("source changed")
		}
	}()
	report, err := inspectUsing(context.Background(), root, parent, r, nil, nil)
	if err == nil || protected == 0 || attempts != 1 {
		t.Fatalf("native close stimulus absent: %+v %v", report, err)
	}
	first := r.Close()
	if !errors.Is(first, ErrCleanupPending) || r.scratch == nil || r.ended {
		t.Fatal("native unknown lost original resources", first)
	}
	assertInspectionExcluded(t, root)
	if err := os.Rename(parent, parent+"-moved"); err == nil {
		t.Fatal("unknown released scratch anchor")
	}
	if second := r.Close(); !errors.Is(second, ErrCleanupPending) || attempts != 1 {
		t.Fatal("native close retried/laundered", second, attempts)
	}
}

func TestInspectionOwnedDriverFailureIsSticky(t *testing.T) {
	root, parent := inspectionFixture(t)
	r := &Resources{cleanup: os.RemoveAll}
	report, err := inspectUsing(context.Background(), root, parent, r, nil, nil)
	if err != nil || !report.IntegrityOK {
		t.Fatal(err)
	}
	calls := 0
	r.closeDatabase = func(h *store.Handle) error { calls++; return errors.Join(h.Close(), store.ErrPhysicalCloseUnknown) }
	if err = r.Close(); !errors.Is(err, ErrCleanupPending) {
		t.Fatal(err)
	}
	assertInspectionExcluded(t, root)
	if _, err = os.Stat(filepath.Join(r.scratch.Path(), "workspace.sqlite")); err != nil {
		t.Fatal("driver uncertainty deleted scratch", err)
	}
	if err = r.Close(); !errors.Is(err, ErrCleanupPending) || calls != 1 {
		t.Fatal("driver unknown retried", err, calls)
	}
	// Synthetic driver's failure is policy evidence, NOT a native SQLite fault.
	r.closeDatabase = nil
	r.database = nil
	r.poison = nil
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInspectionNilFactoryCloseUnknownRetainsOwner(t *testing.T) {
	root, parent := inspectionFixture(t)
	r := &Resources{cleanup: os.RemoveAll}
	var err error
	r.lease, err = platform.Acquire(root, platform.Workspace, false, true)
	if err != nil {
		t.Fatal(err)
	}
	r.scratch, err = platform.NewScratch(parent, "inspect-"+"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	// Models the documented nil-handle factory contract, not an injected native
	// SQLite close failure. No nil handle is interpreted as physical success.
	r.poison = store.ErrPhysicalCloseUnknown
	if err = r.Close(); !errors.Is(err, ErrCleanupPending) {
		t.Fatal(err)
	}
	assertInspectionExcluded(t, root)
	r.poison = nil
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
}
