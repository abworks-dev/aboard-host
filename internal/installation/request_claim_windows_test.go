//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestInstallationRequestClaimOriginalAtCapacityAndNamespace(t *testing.T) {
	ctx := context.Background()
	f, c, a, token, input := creationFixture(t)
	c.afterPhase = func(p sqlite.InstallationPhase) error {
		if p == sqlite.DescriptorPublished {
			return sqlite.ErrUnknown
		}
		return nil
	}
	if view, err := c.CreateWorkspace(ctx, a, input); err == nil || view.Outcome != sqlite.InstallationUnknown {
		t.Fatal(view, err)
	}
	var refs []*workspace.OperationReservation
	defer func() {
		for _, r := range refs {
			r.Release()
		}
	}()
	for i := 0; i < 62; i++ {
		r, err := f.store.ReserveOperation(ctx, a)
		if err != nil {
			t.Fatal(i, err)
		}
		refs = append(refs, r)
	}
	if _, err := f.store.ReserveOperation(ctx, a); !errors.Is(err, workspace.ErrCapacity) {
		t.Fatal(err)
	}
	readAuth, err := f.auth.Authorize(ctx, token, authA, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ClaimInstallationInvestigation(ctx, readAuth, input.OperationID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("workspace grant became installation permission", err)
	}
	borrowed, err := f.store.ClaimInstallationInvestigation(ctx, a, input.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	defer borrowed.Release()
	if _, err := f.store.ClaimInstallationInvestigation(ctx, a, input.OperationID); !errors.Is(err, workspace.ErrCapacity) {
		t.Fatal("second body/delivery claim entered", err)
	}
	if _, err := c.GetInstallationOperation(borrowed.Context(ctx), a, authSameResource); !errors.Is(err, workspace.ErrScope) {
		t.Fatal("borrow changed original key", err)
	}
	if _, err := c.CreateWorkspace(borrowed.Context(ctx), a, input); !errors.Is(err, workspace.ErrScope) {
		t.Fatal("borrow became new creation", err)
	}
	final, err := c.ReconcileInstallationOperation(borrowed.Context(ctx), a, input.OperationID, sqlite.DescriptorPublished)
	if err != nil || final.Outcome != sqlite.InstallationCommitted {
		t.Fatal(final, err)
	}
	if f.store.workspaceCapacity.Usage() != 63 {
		t.Fatal("known physical cleanup cleared unfinished delivery", f.store.workspaceCapacity.Usage())
	}
	borrowed.Release()
	if f.store.workspaceCapacity.Usage() != 62 {
		t.Fatal("finished claim did not retire original allocation")
	}
	for _, r := range refs {
		r.Release()
	}
	opener, _ := NewWorkspaceOpener(f.auth)
	scoped, err := opener.ReserveWorkspaceOperation(ctx, readAuth, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer scoped.Release()
	wrongKind, err := scoped.ClaimInvestigation(input.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	defer wrongKind.Release()
	if _, err := c.GetInstallationOperation(wrongKind.Context(ctx), a, input.OperationID); !errors.Is(err, workspace.ErrScope) {
		t.Fatal("same-UUID workspace claim crossed installation namespace", err)
	}
	t.Log("real native staged creation and original authorized reconciliation at 63 held quota positions; extra positions are accounting fixtures, not 62 slow HTTP clients")
}
