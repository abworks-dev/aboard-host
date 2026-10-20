//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	inspection "github.com/abworks-dev/aboard-host/internal/workspace/inspect"
	win "golang.org/x/sys/windows"
)

type capturedInspectionFault struct {
	cause error
	lease *platform.Lease
}

func (e *capturedInspectionFault) Error() string { return e.cause.Error() }
func (e *capturedInspectionFault) Unwrap() error { return e.cause }

func TestInspectionNativeFactoryFailureBeforeScopeRetainsOriginalRequest(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown-scope", true: "revoked-after-IO"}[revoke], func(t *testing.T) {
			ctx := context.Background()
			f := newAuthFixture(t)
			operator, token := authOperator(t, f)
			root := filepath.Join(f.allowed, "native-inspection-source")
			inspectionSource(t, root, f.store.Identity().TenantID)
			scratch := filepath.Join(f.root, "native-inspection-private")
			if err := platform.CreatePrivateDirectory(scratch); err != nil {
				t.Fatal(err)
			}
			inspector, err := NewDirectoryInspector(f.store, scratch)
			if err != nil {
				t.Fatal(err)
			}
			var lease *platform.Lease
			var pin *platform.File
			var handle win.Handle
			defer func() {
				if handle != 0 {
					_ = win.SetHandleInformation(handle, 2, 0)
					_ = pin.Close()
					_ = lease.ClosePinnedFiles()
					_ = lease.Close()
				}
				// Native handle end was explicitly proven by TEST teardown, never inferred
				// by a retry through the product's permanently quarantined owner.
				for owner := range f.store.inspections {
					owner.failure = nil
				}
				if err := f.store.DrainInspections(); err != nil {
					t.Error(err)
				}
			}()
			inspector.run = func(ctx context.Context, root, parent, tenant string, check func(context.Context) error) (inspection.DirectoryReport, *inspection.Resources, error) {
				var e error
				lease, e = platform.Acquire(root, platform.Workspace, false, true)
				if e != nil {
					t.Fatal(e)
				}
				pin, e = lease.PinFile("aboard.workspace.json", true)
				if e != nil {
					t.Fatal(e)
				}
				handle = pin.Handle()
				if e = win.SetHandleInformation(handle, 2, 2); e != nil {
					t.Fatal(e)
				}
				closeErr := pin.Close()
				if !errors.Is(closeErr, platform.ErrPhysicalCloseUnknown) {
					t.Fatal("real native close did not fail", closeErr)
				}
				if revoke {
					if e = f.auth.RevokeSession(ctx, operator, token); e != nil {
						t.Fatal(e)
					}
				}
				return inspection.DirectoryReport{}, nil, &capturedInspectionFault{closeErr, lease}
			}
			outer, err := f.store.ReserveOperation(ctx, operator)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := outer.ClaimRequest()
			outer.Release()
			if err != nil {
				t.Fatal(err)
			}
			view, err := inspector.Inspect(claim.Context(ctx), operator, root)
			claim.Release()
			if err == nil || view.WorkspaceID != "" || view.TenantID != "" || view.ScratchCleanupFailed {
				t.Fatal("pre-scope failure leaked facts", view, err)
			}
			if revoke && !errors.Is(err, ErrUnauthorized) {
				t.Fatal("late revocation not enforced", err)
			}
			if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), token) {
				t.Fatal("private failure escaped projection")
			}
			if f.store.workspaceCapacity.Usage() != 1 || len(f.store.inspections) != 1 {
				t.Fatal("native unknown escaped original capacity")
			}
			// Removing TEST protection would let a retry close it, but product Drain
			// must keep the first unknown and perform no native retry.
			if e := win.SetHandleInformation(handle, 2, 0); e != nil {
				t.Fatal(e)
			}
			if e := f.store.DrainInspections(); !errors.Is(e, ErrInspectionPending) {
				t.Fatal(e)
			}
			var info win.ByHandleFileInformation
			if e := win.GetFileInformationByHandle(handle, &info); e != nil {
				t.Fatal("unknown was retried", e)
			}
			if e := f.store.Close(); !errors.Is(e, ErrInspectionPending) {
				t.Fatal(e)
			}
			if other, e := platform.Acquire(root, platform.Workspace, false, false); e == nil {
				_ = other.Close()
				t.Fatal("source lease released on redacted failure")
			}
		})
	}
}
