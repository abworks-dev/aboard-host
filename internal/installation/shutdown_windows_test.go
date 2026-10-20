//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
)

func TestShutdownReadinessRetainsInstallationAndAllOwnerFences(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	if err := f.store.CheckShutdownReady(ctx); err != nil {
		t.Fatal(err)
	}
	if competing, err := platform.Acquire(f.install, platform.Installation, false, false); err == nil {
		competing.Close()
		t.Fatal("readiness released installation lease")
	}
	// These controlled owner-state cases prove the preflight does not mistake
	// counters/authorization uncertainty for readiness; they are not IO faults.
	for _, field := range []string{"delivery", "creation", "workspace", "openFailure", "authorization"} {
		f.store.mu.Lock()
		switch field {
		case "delivery":
			f.store.pendingDeliveries = 1
		case "creation":
			f.store.pendingWorkspaceCreations = 1
		case "workspace":
			f.store.liveWorkspaceHandles = 1
		case "openFailure":
			f.store.openFailure = true
		case "authorization":
			f.store.authUncertain = true
		}
		f.store.mu.Unlock()
		err := f.store.CheckShutdownReady(ctx)
		f.store.mu.Lock()
		f.store.pendingDeliveries = 0
		f.store.pendingWorkspaceCreations = 0
		f.store.liveWorkspaceHandles = 0
		f.store.openFailure = false
		f.store.authUncertain = false
		f.store.mu.Unlock()
		if !errors.Is(err, ErrShutdownPending) {
			t.Fatal("owner fence ignored", field, err)
		}
	}
	if err := f.store.CheckShutdownReady(ctx); err != nil {
		t.Fatal(err)
	}
}
