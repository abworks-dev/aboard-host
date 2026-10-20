//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestInstallationCapacityExistsBeforeAnyWorkspaceAndRejectsBeforeIntent(t *testing.T) {
	ctx := context.Background()
	f, c, a, _, request := creationFixture(t)
	var refs []*workspace.OperationReservation
	defer func() {
		for _, r := range refs {
			r.Release()
		}
	}()
	for i := 0; i < 63; i++ {
		r, err := f.store.ReserveOperation(ctx, a)
		if err != nil {
			t.Fatal(i, err)
		}
		refs = append(refs, r)
	}
	if len(f.store.workspaceOpenOwners) != 0 {
		t.Fatal("quota constructed a workspace owner")
	}
	if _, err := f.store.ReserveOperation(ctx, a); !errors.Is(err, workspace.ErrCapacity) {
		t.Fatal(err)
	}
	if _, err := c.CreateWorkspace(ctx, a, request); !errors.Is(err, workspace.ErrCapacity) {
		t.Fatal("installation-only operation bypassed common capacity", err)
	}
	if _, err := os.Stat(request.RootLocator); !os.IsNotExist(err) {
		t.Fatal("capacity rejection touched target", err)
	}
	if _, err := c.GetInstallationOperation(ctx, a, request.OperationID); !errors.Is(err, ErrWorkspaceCreationNotFound) {
		t.Fatal("capacity rejection recorded intent", err)
	}
	refs[0].Release()
	if view, err := c.CreateWorkspace(ctx, a, request); err != nil || view.Outcome != sqlite.InstallationCommitted {
		t.Fatal(view, err)
	}
	if used := f.store.workspaceCapacity.Usage(); used != 62 {
		t.Fatal("ordinary physical owner did not release", used)
	}
}

func TestInstallationCapacitySharesRequestProjectionAndWorkspaceTurns(t *testing.T) {
	ctx := context.Background()
	f, c, a, token, request := creationFixture(t)
	r, err := f.store.ReserveOperation(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release()
	view, err := c.CreateWorkspace(r.Context(ctx), a, request)
	if err != nil {
		t.Fatal(err)
	}
	if f.store.workspaceCapacity.Usage() != 1 {
		t.Fatal("creation double charged request")
	}
	opener, _ := NewWorkspaceOpener(f.auth)
	openAuth, err := f.auth.Authorize(ctx, token, view.WorkspaceID, "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	bound, err := opener.ReserveWorkspaceOperation(r.Context(ctx), openAuth, false, true)
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Release()
	r.Release()
	opened, err := opener.OpenWorkspace(bound.Context(ctx), openAuth, openAuth.Location().RegistrationVersion)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Epoch == "" || f.store.workspaceCapacity.Usage() != 1 {
		t.Fatal("open did not adopt one request count", opened)
	}
	readAuth, err := f.auth.Authorize(ctx, token, view.WorkspaceID, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opener.WorkspaceStatus(bound.Context(ctx), readAuth); err != nil {
		t.Fatal(err)
	}
	if f.store.workspaceCapacity.Usage() != 1 {
		t.Fatal("sequential projection double charged")
	}
	bound.Release()
	if f.store.workspaceCapacity.Usage() != 0 {
		t.Fatal("request completion did not release")
	}
	closeAuth, _ := f.auth.Authorize(ctx, token, view.WorkspaceID, "workspace.close")
	if err := opener.DrainWorkspace(ctx, closeAuth); err != nil {
		t.Fatal(err)
	}
}

func TestInstallationQuotaRetainsUnknownAfterPhysicalDrainUntilReconcile(t *testing.T) {
	ctx := context.Background()
	f, c, a, _, request := creationFixture(t)
	c.afterPhase = func(phase sqlite.InstallationPhase) error {
		if phase == sqlite.DescriptorPublished {
			return sqlite.ErrUnknown
		}
		return nil
	}
	requestRef, err := f.store.ReserveOperation(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	view, err := c.CreateWorkspace(requestRef.Context(ctx), a, request)
	requestRef.Release()
	if err == nil || view.Outcome != sqlite.InstallationUnknown {
		t.Fatal(view, err)
	}
	original := f.store.workspaceCreations[request.OperationID].reservation
	if original == nil || f.store.workspaceCapacity.Usage() != 1 || f.store.pendingWorkspaceCreations != 0 {
		t.Fatal("logical unknown lost quota or fabricated physical liveness")
	}
	lease, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false)
	if err != nil {
		t.Fatal("actual physical owner not drained", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	other, _ := NewWorkspaceCreator(f.auth)
	duplicate, err := other.CreateWorkspace(ctx, a, request)
	if err != nil || duplicate.Outcome != sqlite.InstallationUnknown {
		t.Fatal(duplicate, err)
	}
	if f.store.workspaceCapacity.Usage() != 1 || f.store.workspaceCreations[request.OperationID].reservation != original {
		t.Fatal("duplicate replaced unknown original count")
	}
	if err := other.DrainWorkspaceCreation(ctx, a, request.OperationID); err != nil {
		t.Fatal(err)
	}
	if f.store.workspaceCapacity.Usage() != 1 {
		t.Fatal("physical drain alone cleared unknown outcome")
	}
	final, err := other.ReconcileInstallationOperation(ctx, a, request.OperationID, sqlite.DescriptorPublished)
	if err != nil || final.Outcome != sqlite.InstallationCommitted {
		t.Fatal(final, err)
	}
	if f.store.workspaceCapacity.Usage() != 0 || f.store.pendingWorkspaceCreations != 0 {
		t.Fatal("known finalization and cleanup did not release original")
	}
	t.Log("actual staged creation/SQLite and native lease: phase seam models unknown confirmation, not an injected native FSYNC failure; physical drain and known outcome are independently required")
}

func TestInstallationQuotaRejectedReconcileCanBecomeUnknown(t *testing.T) {
	ctx := context.Background()
	f, c, a, _, request := creationFixture(t)
	c.afterPhase = func(p sqlite.InstallationPhase) error {
		if p == sqlite.DescriptorPublished {
			return errors.New("synthetic known interruption")
		}
		return nil
	}
	first, err := c.CreateWorkspace(ctx, a, request)
	if err == nil || first.Outcome != sqlite.InstallationRejected || f.store.workspaceCapacity.Usage() != 0 {
		t.Fatal(first, err, f.store.workspaceCapacity.Usage())
	}
	c.afterPhase = func(p sqlite.InstallationPhase) error {
		if p == sqlite.RegistrationConfirmed {
			return sqlite.ErrUnknown
		}
		return nil
	}
	_, err = c.ReconcileInstallationOperation(ctx, a, request.OperationID, sqlite.DescriptorPublished)
	if !errors.Is(err, sqlite.ErrUnknown) || f.store.workspaceCapacity.Usage() != 1 || f.store.pendingWorkspaceCreations != 0 {
		t.Fatal("old rejected evidence cleared newly unknown finalization", err, f.store.workspaceCapacity.Usage())
	}
	if err := f.store.Close(); err != nil {
		t.Fatal("quota changed physically drained fenced Store.Close semantics", err)
	}
	lease, err := platform.Acquire(f.install, platform.Installation, false, false)
	if err != nil {
		t.Fatal("fenced database could not actually close for cold investigation", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	t.Log("existing post-COMMIT confirmation seam; original quota remains unknown, while explicit physical installation close remains possible for cold investigation")
}

func TestInstallationTerminalQueryNeverWaitsOrOverwritesBusyOwner(t *testing.T) {
	ctx := context.Background()
	f, c, a, _, request := creationFixture(t)
	c.afterPhase = func(p sqlite.InstallationPhase) error {
		if p == sqlite.DescriptorPublished {
			return errors.New("synthetic known interruption")
		}
		return nil
	}
	if _, err := c.CreateWorkspace(ctx, a, request); err == nil {
		t.Fatal("missing rejected fixture")
	}
	w := f.store.workspaceCreations[request.OperationID]
	w.mu.Lock()
	ref, err := f.store.ReserveOperation(ctx, a)
	if err != nil {
		w.mu.Unlock()
		t.Fatal(err)
	}
	f.store.mu.Lock()
	w.reservation = ref
	w.quotaPending = true
	w.quotaKnown = false
	f.store.mu.Unlock()
	done := make(chan error, 1)
	go func() { _, err := c.GetInstallationOperation(ctx, a, request.OperationID); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			w.mu.Unlock()
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		w.mu.Unlock()
		t.Fatal("terminal GET waited behind physical owner")
	}
	if w.quotaKnown || f.store.workspaceCapacity.Usage() != 1 {
		w.mu.Unlock()
		t.Fatal("terminal query overwrote busy original mutation")
	}
	w.mu.Unlock()
	if _, err := c.GetInstallationOperation(ctx, a, request.OperationID); err != nil {
		t.Fatal(err)
	}
	if f.store.workspaceCapacity.Usage() != 0 {
		t.Fatal("fresh atomic terminal read did not settle idle quota")
	}
	t.Log("barrier models busy quota metadata, not a native I/O failure; authenticated terminal fact and bookkeeping stay within the same Store lock")
}

func TestInstallationBindingQuotaSurvivesPhysicalDrainAndDuplicate(t *testing.T) {
	ctx := context.Background()
	f, c, a, token, request := creationFixture(t)
	created, err := c.CreateWorkspace(ctx, a, request)
	if err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(f.allowed, "renamed-for-capacity")
	if err = os.Rename(request.RootLocator, next); err != nil {
		t.Fatal(err)
	}
	d, _ := NewDirectoryCoordinator(f.auth)
	d.afterIntent = func() error { return errors.New("synthetic stop after confirmed binding intent") }
	wa, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.operator")
	if err != nil {
		t.Fatal(err)
	}
	input := RelinkDirectoryRequest{OperationID: "62000000-0000-4000-8000-000000000001", RootLocator: next, ExpectedRegistrationVersion: wa.Location().RegistrationVersion}
	accepted, err := d.Relink(ctx, wa, created.WorkspaceID, input)
	if err == nil || accepted.Outcome != sqlite.InstallationAccepted || f.store.workspaceCapacity.Usage() != 1 || f.store.pendingWorkspaceCreations != 0 {
		t.Fatal(accepted, err)
	}
	original := f.store.workspaceCreations[input.OperationID].reservation
	lease, err := platform.Acquire(next, platform.Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	duplicate, err := d.Relink(ctx, wa, created.WorkspaceID, input)
	if err != nil || duplicate.Outcome != sqlite.InstallationAccepted || f.store.workspaceCapacity.Usage() != 1 || f.store.workspaceCreations[input.OperationID].reservation != original {
		t.Fatal(duplicate, err)
	}
	final, err := d.ReconcileInstallationOperation(ctx, a, input.OperationID, sqlite.IntentRecorded)
	if err != nil || final.Outcome != sqlite.InstallationCommitted || f.store.workspaceCapacity.Usage() != 0 {
		t.Fatal(final, err, f.store.workspaceCapacity.Usage())
	}
	current, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.read")
	if err != nil || current.Location().RegistrationVersion != input.ExpectedRegistrationVersion+1 {
		t.Fatal("reconcile did not preserve actual relink CAS", err)
	}
}

func TestInstallationKnownQuotaCannotReleaseBeforePhysicalOwner(t *testing.T) {
	ctx := context.Background()
	f, c, a, _, request := creationFixture(t)
	c.afterPhase = func(phase sqlite.InstallationPhase) error {
		if phase == sqlite.RegistrationConfirmed {
			f.store.workspaceCreations[request.OperationID].openUnknown = true
		}
		return nil
	}
	view, err := c.CreateWorkspace(ctx, a, request)
	if err == nil || view.Outcome != sqlite.InstallationCommitted {
		t.Fatal(view, err)
	}
	if f.store.workspaceCapacity.Usage() != 1 || f.store.pendingWorkspaceCreations != 1 {
		t.Fatal("known logical outcome released physical owner")
	}
	if lease, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false); err == nil {
		lease.Close()
		t.Fatal("original writer released")
	}
	if _, err := c.GetInstallationOperation(ctx, a, request.OperationID); err != nil {
		t.Fatal(err)
	}
	if f.store.workspaceCapacity.Usage() != 1 {
		t.Fatal("known query freed live owner")
	}
	w := f.store.workspaceCreations[request.OperationID]
	w.mu.Lock()
	w.openUnknown = false
	w.mu.Unlock()
	if err := c.DrainWorkspaceCreation(ctx, a, request.OperationID); err != nil {
		t.Fatal(err)
	}
	if f.store.workspaceCapacity.Usage() != 0 {
		t.Fatal("actual original drain did not release")
	}
	t.Log("controlled owner-fence injection over real native resources; not a native Close syscall fault")
}
