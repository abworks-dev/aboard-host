//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestDirectoryCoordinatorRegistersUnregisteredSourceWithoutGrantImport(t *testing.T) {
	f := newAuthFixture(t)
	defer f.store.Close()
	ctx := context.Background()
	authFixtureSQL(t, f, `DELETE FROM grant_entry WHERE workspace_id=?`, authA)
	authFixtureSQL(t, f, `DELETE FROM directory_registration WHERE workspace_id=?`, authA)
	operator, token := authOperator(t, f)
	root := filepath.Join(f.allowed, "external-source")
	descriptor := inspectionSource(t, root, f.store.Identity().TenantID)
	before := inspectionTree(t, root)
	d, _ := NewDirectoryCoordinator(f.auth)
	got, err := d.Register(ctx, operator, RegisterDirectoryRequest{bindingOperation, root, authA, descriptor.IdentityDigest})
	if err != nil || got.Outcome != sqlite.InstallationCommitted {
		t.Fatalf("new registration %+v %v", got, err)
	}
	if !reflect.DeepEqual(before, inspectionTree(t, root)) {
		t.Fatal("registration modified source")
	}
	if _, err = f.auth.Authorize(ctx, token, authA, "workspace.read"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("registration imported workspace grants", err)
	}
}

func TestDirectoryCoordinatorRejectsPersistedUncertainSaves(t *testing.T) {
	for _, state := range []string{"accepted", "unknown"} {
		t.Run(state, func(t *testing.T) {
			f, c, operator, token, r := creationFixture(t)
			defer f.store.Close()
			ctx := context.Background()
			created, err := c.CreateWorkspace(ctx, operator, r)
			if err != nil {
				t.Fatal(err)
			}
			inspectionSQL(t, r.RootLocator, `INSERT INTO candidate SELECT workspace_id,'synthetic-candidate','synthetic-principal','synthetic-resource','create-text',0,NULL,'synthetic','body',printf('%064d',0),printf('%064d',0),1,'synthetic','2099-01-01T00:00:00Z','previewed' FROM workspace_identity;
 INSERT INTO save_operation SELECT workspace_id,'synthetic-operation','synthetic-principal','synthetic-candidate',printf('%064d',0),'synthetic','`+state+`',NULL FROM workspace_identity;`)
			a, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.operator")
			if err != nil {
				t.Fatal(err)
			}
			moved := r.RootLocator + "-moved"
			if err = os.Rename(r.RootLocator, moved); err != nil {
				t.Fatal(err)
			}
			before := inspectionTree(t, moved)
			d, _ := NewDirectoryCoordinator(f.auth)
			if _, err = d.Relink(ctx, a, created.WorkspaceID, RelinkDirectoryRequest{relinkOperation, moved, 1}); !errors.Is(err, sqlite.ErrUnknown) {
				t.Fatalf("uncertain save accepted: %v", err)
			}
			if !reflect.DeepEqual(before, inspectionTree(t, moved)) {
				t.Fatal("uncertain source modified")
			}
			if _, err = d.GetInstallationOperation(ctx, operator, relinkOperation); !errors.Is(err, ErrWorkspaceCreationNotFound) {
				t.Fatal("uncertain source admitted intent", err)
			}
		})
	}
}

const bindingOperation = "62000000-0000-4000-8000-000000000001"
const relinkOperation = "62000000-0000-4000-8000-000000000002"

func TestDirectoryCoordinatorPhysicalRegisterAndRelink(t *testing.T) {
	f, c, operator, token, r := creationFixture(t)
	defer f.store.Close()
	ctx := context.Background()
	created, err := c.CreateWorkspace(ctx, operator, r)
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDirectoryCoordinator(f.auth)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := workspace.NewDirectoryDescriptor(created.WorkspaceID, f.store.Identity().TenantID)
	if err != nil {
		t.Fatal(err)
	}
	before := inspectionTree(t, r.RootLocator)
	req := RegisterDirectoryRequest{bindingOperation, r.RootLocator, created.WorkspaceID, descriptor.IdentityDigest}
	got, err := d.Register(ctx, operator, req)
	if err != nil || got.Outcome != sqlite.InstallationCommitted {
		t.Fatalf("register: %+v %v", got, err)
	}
	if !reflect.DeepEqual(before, inspectionTree(t, r.RootLocator)) {
		t.Fatal("register changed business source")
	}
	again, err := d.Register(ctx, operator, req)
	if err != nil || again != got {
		t.Fatalf("repeat register: %+v %v", again, err)
	}
	for _, key := range []string{r.OperationID, bindingOperation} {
		if _, err = d.GetInstallationOperation(ctx, operator, key); err != nil {
			t.Fatal("shared query dispatch", err)
		}
	}
	if _, err = d.ReconcileInstallationOperation(ctx, operator, bindingOperation, sqlite.RegistrationConfirmed); err != nil {
		t.Fatal(err)
	}
	a, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.operator")
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := f.store.WorkspaceLifecycle(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	same, err := f.store.WorkspaceLifecycle(ctx, a)
	if err != nil || same != lifecycle {
		t.Fatal("second lifecycle owner")
	}
	scope, _ := a.Scope()
	if err = lifecycle.BeginOpen(scope); err != nil {
		t.Fatal(err)
	}
	moved := r.RootLocator + "-moved"
	if err = os.Rename(r.RootLocator, moved); err != nil {
		t.Fatal(err)
	}
	renameBefore := inspectionTree(t, moved)
	move := RelinkDirectoryRequest{relinkOperation, moved, 1}
	if _, err = d.Relink(ctx, a, created.WorkspaceID, move); !errors.Is(err, workspace.ErrLifecycle) {
		t.Fatalf("opening relink accepted: %v", err)
	}
	if err = lifecycle.BeginDrain(scope); err != nil {
		t.Fatal(err)
	}
	if err = lifecycle.FinishDrain(scope, workspace.DrainUnknown); err == nil {
		t.Fatal("unknown drain claimed success")
	}
	if _, err = d.Relink(ctx, a, created.WorkspaceID, move); !errors.Is(err, workspace.ErrLifecycle) {
		t.Fatalf("unknown relink accepted: %v", err)
	}
	if err = lifecycle.BeginDrain(scope); err != nil {
		t.Fatal(err)
	}
	if err = lifecycle.FinishDrain(scope, workspace.DrainSucceeded); err != nil {
		t.Fatal(err)
	}
	got, err = d.Relink(ctx, a, created.WorkspaceID, move)
	if err != nil || got.Outcome != sqlite.InstallationCommitted {
		t.Fatalf("rename relink: %+v %v", got, err)
	}
	if !reflect.DeepEqual(renameBefore, inspectionTree(t, moved)) {
		t.Fatal("relink changed business source")
	}
	if err = a.Check(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old registration handle remained current", err)
	}
	fresh, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.operator")
	if err != nil || fresh.Location().RootLocator != moved || fresh.Location().RegistrationVersion != 2 {
		t.Fatalf("new binding: %+v %v", fresh, err)
	}
	if _, err = d.GetInstallationOperation(ctx, fresh, relinkOperation); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("workspace-only query accepted", err)
	}
	if _, err = d.ReconcileInstallationOperation(ctx, fresh, relinkOperation, sqlite.RegistrationConfirmed); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("workspace-only reconcile accepted", err)
	}
	if _, err = d.GetInstallationOperation(ctx, operator, relinkOperation); err != nil {
		t.Fatal(err)
	}
	if _, err = d.ReconcileInstallationOperation(ctx, operator, relinkOperation, sqlite.RegistrationConfirmed); err != nil {
		t.Fatal(err)
	}
	if _, err = d.GetInstallationOperation(ctx, operator, r.OperationID); err != nil {
		t.Fatal("create query after relink", err)
	}
}
func TestDirectoryCoordinatorRejectsCopyAndRetainsOriginalIntent(t *testing.T) {
	f, c, operator, token, r := creationFixture(t)
	defer f.store.Close()
	ctx := context.Background()
	created, err := c.CreateWorkspace(ctx, operator, r)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := NewDirectoryCoordinator(f.auth)
	descriptor, _ := workspace.NewDirectoryDescriptor(created.WorkspaceID, f.store.Identity().TenantID)
	copyRoot := r.RootLocator + "-copy"
	if err = os.CopyFS(copyRoot, os.DirFS(r.RootLocator)); err != nil {
		t.Fatal(err)
	}
	before := inspectionTree(t, copyRoot)
	if _, err = d.Register(ctx, operator, RegisterDirectoryRequest{bindingOperation, copyRoot, created.WorkspaceID, descriptor.IdentityDigest}); err == nil {
		t.Fatal("copied UUID registered")
	}
	a, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.Relink(ctx, a, created.WorkspaceID, RelinkDirectoryRequest{relinkOperation, copyRoot, 1}); err == nil {
		t.Fatal("copy relinked")
	}
	if !reflect.DeepEqual(before, inspectionTree(t, copyRoot)) {
		t.Fatal("rejected copy changed")
	}
	if _, err = d.GetInstallationOperation(ctx, operator, bindingOperation); !errors.Is(err, ErrWorkspaceCreationNotFound) {
		t.Fatalf("invalid copy admitted durable intent: %v", err)
	}
	req := RegisterDirectoryRequest{bindingOperation, r.RootLocator, created.WorkspaceID, descriptor.IdentityDigest}
	d.afterIntent = func() error { return errors.New("synthetic lost waiter before finalize") }
	pending, err := d.Register(ctx, operator, req)
	if err == nil || pending.Outcome != sqlite.InstallationAccepted {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	d.afterIntent = nil
	again, err := d.Register(ctx, operator, req)
	if err != nil || again != pending {
		t.Fatal("same key replayed filesystem/finalize", err)
	}
	held, err := platform.Acquire(r.RootLocator, platform.Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.ReconcileInstallationOperation(ctx, operator, bindingOperation, sqlite.IntentRecorded); err == nil {
		t.Fatal("live writer reconciled")
	}
	if err = held.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(r.RootLocator, "data", "workspace.sqlite"), filepath.Join(r.RootLocator, "data", "workspace.sqlite.original")); err != nil {
		t.Fatal(err)
	}
	if _, err = d.ReconcileInstallationOperation(ctx, operator, bindingOperation, sqlite.IntentRecorded); err == nil {
		t.Fatal("missing DB repaired")
	}
	if _, err = os.Stat(filepath.Join(r.RootLocator, "data", "workspace.sqlite")); !os.IsNotExist(err) {
		t.Fatal("missing DB created")
	}
}
