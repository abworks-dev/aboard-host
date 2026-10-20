//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	inspection "github.com/abworks-dev/aboard-host/internal/workspace/inspect"
	win "golang.org/x/sys/windows"
)

func TestInspectionOriginalOwnerRetainsQuotaThroughCleanupAndRevocation(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup", true: "late-revocation"}[revoke], func(t *testing.T) {
			ctx := context.Background()
			f := newAuthFixture(t)
			operator, token := authOperator(t, f)
			root := filepath.Join(f.allowed, "inspection-source")
			inspectionSource(t, root, f.store.Identity().TenantID)
			before := inspectionTree(t, root)
			scratch := filepath.Join(f.root, "inspection-private")
			if err := platform.CreatePrivateDirectory(scratch); err != nil {
				t.Fatal(err)
			}
			inspector, err := NewDirectoryInspector(f.store, scratch)
			if err != nil {
				t.Fatal(err)
			}
			var blocker win.Handle
			inspector.run = func(ctx context.Context, root, parent, tenant string, check func(context.Context) error) (inspection.DirectoryReport, *inspection.Resources, error) {
				report, resources, err := inspection.InspectDirectoryOwned(ctx, root, parent, tenant, check)
				if err != nil {
					return report, resources, err
				}
				entries, e := os.ReadDir(parent)
				if e != nil || len(entries) != 1 {
					t.Fatalf("private child missing %v %v", entries, e)
				}
				path := filepath.Join(parent, entries[0].Name(), "workspace.sqlite")
				name, e := win.UTF16PtrFromString(path)
				if e != nil {
					t.Fatal(e)
				}
				blocker, e = win.CreateFile(name, win.GENERIC_READ, win.FILE_SHARE_READ, nil, win.OPEN_EXISTING, 0, 0)
				if e != nil {
					t.Fatal(e)
				}
				if revoke {
					if e = f.auth.RevokeSession(ctx, operator, token); e != nil {
						t.Fatal(e)
					}
				}
				return report, resources, nil
			}
			request, err := f.store.ReserveOperation(ctx, operator)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := request.ClaimRequest()
			request.Release()
			if err != nil {
				t.Fatal(err)
			}
			view, inspectErr := inspector.Inspect(claim.Context(ctx), operator, root)
			claim.Release()
			if revoke {
				if !errors.Is(inspectErr, ErrUnauthorized) || view.WorkspaceID != "" || view.ScratchCleanupFailed {
					t.Fatalf("revoked projection leaked %+v %v", view, inspectErr)
				}
			} else {
				var failure *InspectionFailure
				if !errors.As(inspectErr, &failure) || failure.Code != protocol.ErrorCodeINSPECTIONUNAVAILABLE || !view.ScratchCleanupFailed {
					t.Fatal(view, inspectErr)
				}
			}
			if f.store.workspaceCapacity.Usage() != 1 || len(f.store.inspections) != 1 {
				t.Fatal("request completion discarded original inspection")
			}
			if err = f.store.Close(); !errors.Is(err, ErrInspectionPending) {
				t.Fatal(err)
			}
			if err = f.store.CheckShutdownReady(ctx); !errors.Is(err, ErrShutdownPending) {
				t.Fatal(err)
			}
			if lease, e := platform.Acquire(root, platform.Workspace, false, false); e == nil {
				_ = lease.Close()
				t.Fatal("source exclusion released on failed deletion")
			}
			if err = f.store.DrainInspections(); !errors.Is(err, inspection.ErrScratchCleanupPending) {
				t.Fatal("original cleanup did not remain pending", err)
			}
			if !revoke {
				creator, e := NewWorkspaceCreator(f.auth)
				if e != nil {
					t.Fatal(e)
				}
				b, e := creator.CreateWorkspace(ctx, operator, CreateWorkspaceRequest{OperationID: authSameResource, RootLocator: filepath.Join(f.allowed, "independent-B"), DisplayName: "independent B"})
				if e != nil {
					t.Fatal(e)
				}
				opener, e := NewWorkspaceOpener(f.auth)
				if e != nil {
					t.Fatal(e)
				}
				openAuth, e := f.auth.Authorize(ctx, token, b.WorkspaceID, "workspace.open")
				if e != nil {
					t.Fatal(e)
				}
				opened, e := opener.OpenWorkspace(ctx, openAuth, openAuth.Location().RegistrationVersion)
				if e != nil {
					t.Fatal(e)
				}
				readAuth, e := f.auth.Authorize(ctx, token, b.WorkspaceID, "workspace.read")
				if e != nil {
					t.Fatal(e)
				}
				if snap, e := opener.ReadSnapshot(ctx, readAuth); e != nil || snap.Identity.WorkspaceID != b.WorkspaceID || opened.Epoch == "" {
					t.Fatal("inspection A blocked actual B", snap, e)
				}
				closeAuth, e := f.auth.Authorize(ctx, token, b.WorkspaceID, "workspace.close")
				if e != nil {
					t.Fatal(e)
				}
				if e = opener.DrainWorkspace(ctx, closeAuth); e != nil {
					t.Fatal(e)
				}
				if f.store.workspaceCapacity.Usage() != 1 {
					t.Fatal("independent B disturbed A retained count")
				}
			}
			if blocker == 0 {
				t.Fatal("native delete-refusal stimulus absent")
			}
			if err = win.CloseHandle(blocker); err != nil {
				t.Fatal(err)
			}
			blocker = 0
			if err = f.store.DrainInspections(); err != nil {
				t.Fatal(err)
			}
			if f.store.workspaceCapacity.Usage() != 0 || len(f.store.inspections) != 0 {
				t.Fatal("proven cleanup did not release original count")
			}
			if !reflect.DeepEqual(before, inspectionTree(t, root)) {
				t.Fatal("inspection cleanup changed source")
			}
			if entries, e := os.ReadDir(scratch); e != nil || len(entries) != 0 {
				t.Fatal("sensitive scratch retained after known drain", entries, e)
			}
		})
	}
}

func TestInspectionFactoryUnknownBeforeScopeStillFencesInstallation(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	operator, _ := authOperator(t, f)
	scratch := filepath.Join(f.root, "inspection-private")
	if err := platform.CreatePrivateDirectory(scratch); err != nil {
		t.Fatal(err)
	}
	inspector, err := NewDirectoryInspector(f.store, scratch)
	if err != nil {
		t.Fatal(err)
	}
	inspector.run = func(context.Context, string, string, string, func(context.Context) error) (inspection.DirectoryReport, *inspection.Resources, error) {
		return inspection.DirectoryReport{}, nil, sqlite.ErrPhysicalCloseUnknown
	}
	view, err := inspector.Inspect(ctx, operator, filepath.Join(f.allowed, "not-opened"))
	if err == nil || view.WorkspaceID != "" || view.ScratchCleanupFailed {
		t.Fatal("unproved scope leaked", view, err)
	}
	if f.store.workspaceCapacity.Usage() != 1 || len(f.store.inspections) != 1 {
		t.Fatal("nil factory marker dropped owner")
	}
	if err = f.store.DrainInspections(); !errors.Is(err, ErrInspectionPending) {
		t.Fatal(err)
	}
	if err = f.store.Close(); !errors.Is(err, ErrInspectionPending) {
		t.Fatal(err)
	}
	// Controlled factory seam has no physical resource. Test-only cleanup does
	// not claim an unknown native driver was repaired or safe to move.
	for owner := range f.store.inspections {
		owner.failure = nil
	}
	if err = f.store.DrainInspections(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreStartupUnknownAdoptsOriginalOwningError(t *testing.T) {
	f := newAuthFixture(t)
	cause := errors.Join(sqlite.ErrPhysicalCloseUnknown, errors.New("controlled factory owner"))
	retained, err := f.store.fail(cause)
	if retained != f.store || err == nil || !f.store.openFailure || f.store.nativeFailure != cause {
		t.Fatal("startup unknown lost owner", err)
	}
	if lease, e := platform.Acquire(f.install, platform.Installation, false, false); e == nil {
		_ = lease.Close()
		t.Fatal("startup unknown released installation")
	}
	// Synthetic owning-error policy test; no unknown native driver is repaired.
	f.store.openFailure = false
	f.store.nativeFailure = nil
}

func TestStoreNativeCloseFailureDoesNotRetryOrReleaseInstallation(t *testing.T) {
	f := newAuthFixture(t)
	handle := f.store.databasePin.Handle()
	if err := win.SetHandleInformation(handle, 2, 2); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = win.SetHandleInformation(handle, 2, 0)
		// Exact protected native pin remained live; explicit TEST teardown only.
		_ = f.store.databasePin.Close()
		f.store.openFailure = false
		f.store.nativeFailure = nil
	}()
	first := f.store.Close()
	if !errors.Is(first, platform.ErrPhysicalCloseUnknown) || f.store.closed {
		t.Fatal("native close did not fence", first)
	}
	if err := win.SetHandleInformation(handle, 2, 0); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Close(); err == nil || f.store.closed {
		t.Fatal("second close laundered native uncertainty", err)
	}
	var info win.ByHandleFileInformation
	if err := win.GetFileInformationByHandle(handle, &info); err != nil {
		t.Fatal("second close retried original native pin", err)
	}
	if lease, err := platform.Acquire(f.install, platform.Installation, false, false); err == nil {
		_ = lease.Close()
		t.Fatal("installation exclusion released")
	}
	if !strings.Contains(first.Error(), platform.ErrPhysicalCloseUnknown.Error()) {
		t.Fatal(first)
	}
}
