package workspace

import (
	"context"
	"errors"
	"testing"
)

type operationTestResources struct {
	openTestResources
	query     func(context.Context) (OperationEvidence, error)
	reconcile func(context.Context) (OperationEvidence, error)
	retained  int
}

func (r *operationTestResources) QuerySaveOperation(ctx context.Context, _ Scope, _, _ string, _ bool) (OperationEvidence, error) {
	return r.query(ctx)
}
func (r *operationTestResources) ReconcileSaveOperation(ctx context.Context, _ Scope, _, _ string, _ func(context.Context) error) (OperationEvidence, error) {
	return r.reconcile(ctx)
}
func (r *operationTestResources) RetainSaveUncertainty() { r.retained++ }

func TestSaveOperationCanceledReadIsDeliveryNotUncertainty(t *testing.T) {
	owner, l, scope := openTestOwner(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &operationTestResources{}
	r.query = func(physical context.Context) (OperationEvidence, error) {
		cancel()
		if physical.Err() != nil || physical.Done() != nil {
			t.Fatal("read used canceled waiter as physical authority")
		}
		return OperationEvidence{Result: SaveResult{Status: SaveUnknown}}, ErrOperationHistoryUnavailable
	}
	allow := func(context.Context) error { return nil }
	if _, err := owner.Open(context.Background(), scope, allow, func(context.Context, string) (OpenEvidence, error) {
		return OpenEvidence{Resources: r, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	got, err := owner.GetSaveOperation(ctx, scope, scope.TenantID, scope.WorkspaceID, false, allow)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrSaveViewDelivery) || !errors.Is(err, ErrOperationHistoryUnavailable) || !got.Result.ViewDeliveryFailed {
		t.Fatal(got, err)
	}
	state, _ := l.State(scope)
	if state != LifecycleOpen || r.retained != 0 || owner.view.NeedsInvestigation {
		t.Fatal("canceled arbitrary GET poisoned healthy owner", state, r.retained)
	}
	if err = owner.DrainOpen(context.Background(), scope, allow); err != nil {
		t.Fatal(err)
	}
}

func TestSaveOperationKnownUnknownAndLateAuthorization(t *testing.T) {
	for _, mode := range []string{"late-denied", "known-unknown", "fresh-confirmed"} {
		t.Run(mode, func(t *testing.T) {
			owner, l, scope := openTestOwner(t)
			r := &operationTestResources{}
			receipt := &Receipt{OperationID: scope.WorkspaceID, WorkspaceID: scope.WorkspaceID}
			queryRan := false
			r.query = func(context.Context) (OperationEvidence, error) {
				queryRan = true
				if mode == "known-unknown" {
					return OperationEvidence{Result: SaveResult{Status: SaveUnknown}}, nil
				}
				return OperationEvidence{Result: SaveResult{Status: SaveConfirmed, Receipt: receipt}}, nil
			}
			r.reconcile = func(context.Context) (OperationEvidence, error) {
				return OperationEvidence{Result: SaveResult{Status: SaveConfirmed, Receipt: receipt}, PhysicalEnded: true, RequiresReopen: true}, nil
			}
			if _, err := owner.Open(context.Background(), scope, func(context.Context) error { return nil }, func(context.Context, string) (OpenEvidence, error) {
				return OpenEvidence{Resources: r, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}}, nil
			}); err != nil {
				t.Fatal(err)
			}
			denied := errors.New("synthetic current authorization revoked")
			auth := func(context.Context) error {
				if mode == "late-denied" && queryRan {
					return denied
				}
				return nil
			}
			var got OperationEvidence
			var err error
			if mode == "fresh-confirmed" {
				got, err = owner.ReconcileSaveOperation(context.Background(), scope, scope.TenantID, scope.WorkspaceID, auth)
			} else {
				got, err = owner.GetSaveOperation(context.Background(), scope, scope.TenantID, scope.WorkspaceID, false, auth)
			}
			if mode == "late-denied" {
				if !errors.Is(err, denied) || got.Result.Receipt != nil || got.Result.Status != SaveConfirmed || !got.Result.ViewDeliveryFailed {
					t.Fatal(got, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			state, _ := l.State(scope)
			switch mode {
			case "known-unknown":
				if state != LifecycleUnknown || r.retained != 1 {
					t.Fatal(state, r.retained)
				}
			case "fresh-confirmed":
				if state != LifecycleUnknown || r.retained != 0 {
					t.Fatal("positive fresh proof re-poisoned resource", state, r.retained)
				}
			case "late-denied":
				if state != LifecycleOpen || r.retained != 0 {
					t.Fatal("delivery denial rewrote durable state", state, r.retained)
				}
			}
		})
	}
}
