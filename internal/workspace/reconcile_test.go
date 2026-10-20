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

func TestColdReconcileDrainsBeforeQueuedOpenPublishesUsableGeneration(t *testing.T) {
	o, l, scope := openTestOwner(t)
	ctx := context.Background()
	allow := func(context.Context) error { return nil }
	r := &operationTestResources{openTestResources: openTestResources{closeErr: ErrOpenUncertain}}
	if v, err := o.Open(ctx, scope, allow, func(context.Context, string) (OpenEvidence, error) {
		return OpenEvidence{Resources: r}, ErrOpenUncertain
	}); !errors.Is(err, ErrOpenUncertain) || v.Epoch != "" {
		t.Fatal(v, err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	r.reconcile = func(context.Context) (OperationEvidence, error) {
		close(entered)
		<-release
		r.closeErr = nil
		return OperationEvidence{Result: SaveResult{Status: SaveNotCommitted}, PhysicalEnded: true, RequiresReopen: true}, nil
	}
	reconciled := make(chan error, 1)
	go func() {
		_, err := o.ReconcileSaveOperation(ctx, scope, scope.TenantID, scope.WorkspaceID, allow)
		reconciled <- err
	}()
	<-entered
	fresh := &openTestResources{}
	opened := make(chan OpenView, 1)
	openErr := make(chan error, 1)
	go func() {
		v, err := o.Open(ctx, scope, allow, func(context.Context, string) (OpenEvidence, error) {
			return OpenEvidence{Resources: fresh, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}}, nil
		})
		opened <- v
		openErr <- err
	}()
	waitQueueUsage(t, o.queue, 3)
	close(release)
	if err := <-reconciled; err != nil {
		t.Fatal(err)
	}
	v := <-opened
	if err := <-openErr; err != nil || v.Epoch == "" {
		t.Fatal(v, err)
	}
	if state, _ := l.State(scope); state != LifecycleOpen || r.closed != 2 || o.queue.capacity.Usage() != 0 {
		t.Fatal(state, r.closed, o.queue.capacity.Usage())
	}
	key := "10000000-0000-4000-8000-000000000004"
	if _, err := o.CloseWorkspace(ctx, scope, scope.TenantID, key, v.Epoch, allow); err != nil {
		t.Fatal("queued reopen published a retired queue generation", err)
	}
	if closed, err := o.WaitClose(ctx, scope, scope.TenantID, key, false, allow); err != nil || !closed.Closed || fresh.closed != 1 {
		t.Fatal(closed, err, fresh.closed)
	}
	if _, err := o.GetSaveOperation(ctx, scope, scope.TenantID, scope.WorkspaceID, false, allow); !errors.Is(err, ErrLifecycle) {
		t.Fatal("closed save inquiry must require explicit authorized reopen", err)
	}
}

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
