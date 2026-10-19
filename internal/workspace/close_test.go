package workspace

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type closeTestResources struct {
	commitTestResources
	started, release chan struct{}
	reads            atomic.Int32
}

func (r *closeTestResources) Snapshot(_ context.Context, s Scope) (Snapshot, error) {
	if r.reads.Add(1) == 1 {
		close(r.started)
		<-r.release
	}
	return Snapshot{Identity: Identity{WorkspaceID: s.WorkspaceID, TenantID: s.TenantID, BusinessState: "ready"}}, nil
}
func TestCloseOwnerRejectsQueuedWritesAndWaiterCannotReleaseIO(t *testing.T) {
	owner, _, scope := openTestOwner(t)
	auth := func(context.Context) error { return nil }
	r := &closeTestResources{started: make(chan struct{}), release: make(chan struct{})}
	var writes atomic.Int32
	r.admit = func(context.Context, Scope, SaveRequest, bool) (SaveResult, AcceptedSave, error) {
		writes.Add(1)
		return SaveResult{Status: SaveRejected}, nil, nil
	}
	view, err := owner.Open(context.Background(), scope, auth, func(context.Context, string) (OpenEvidence, error) {
		return OpenEvidence{Resources: r, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reading := make(chan error, 1)
	go func() { _, err := owner.ReadSnapshot(context.Background(), scope, auth); reading <- err }()
	<-r.started
	queued := make(chan error, 1)
	go func() {
		_, err := owner.CommitRecord(context.Background(), scope, scope.InstallationID, auth, SaveInput{})
		queued <- err
	}()
	waitQueueUsage(t, owner.queue, 2)
	closeView, err := owner.CloseWorkspace(context.Background(), scope, scope.InstallationID, scope.WorkspaceID, view.Epoch, auth)
	if err != nil || closeView.Closed || closeView.State != LifecycleDraining {
		t.Fatal(closeView, err)
	}
	if err = <-queued; !errors.Is(err, ErrLifecycle) || writes.Load() != 0 {
		t.Fatal("queued write crossed close cutoff", err, writes.Load())
	}
	if _, err = owner.ReadSnapshot(context.Background(), scope, auth); !errors.Is(err, ErrLifecycle) {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	waiting, err := owner.WaitClose(canceled, scope, scope.InstallationID, scope.WorkspaceID, false, auth)
	// A canceled waiter cannot begin a query, but the accepted owner remains active.
	if !errors.Is(err, context.Canceled) || waiting.Closed || r.closed != 0 {
		t.Fatal(waiting, err, r.closed)
	}
	close(r.release)
	if err = <-reading; err != nil {
		t.Fatal(err)
	}
	timeout, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	closed, err := owner.WaitClose(timeout, scope, scope.InstallationID, scope.WorkspaceID, false, auth)
	if err != nil || !closed.Closed || r.closed != 1 || owner.queue.capacity.Usage() != 0 {
		t.Fatal(closed, err, r.closed, owner.queue.capacity.Usage())
	}
	duplicate, err := owner.CloseWorkspace(context.Background(), scope, scope.InstallationID, scope.WorkspaceID, view.Epoch, auth)
	if err != nil || duplicate != closed {
		t.Fatal(duplicate, err)
	}
	if _, err = owner.CloseWorkspace(context.Background(), scope, scope.TenantID, scope.WorkspaceID, view.Epoch, auth); !errors.Is(err, ErrSaveKeyReuse) {
		t.Fatal(err)
	}
	fresh, _, _ := openTestOwner(t)
	if _, err = fresh.GetCloseOperation(context.Background(), scope, scope.InstallationID, scope.WorkspaceID, false, auth); !errors.Is(err, ErrOperationHistoryUnavailable) {
		t.Fatal("close invented durable history", err)
	}
}
func TestCloseOwnerAcceptedSaveFinishesAndShutdownUsesCurrentGeneration(t *testing.T) {
	owner, _, scope, r, input := commitOwnerFixture(t)
	auth := func(context.Context) error { return nil }
	started, release := make(chan struct{}), make(chan struct{})
	r.admit = func(context.Context, Scope, SaveRequest, bool) (SaveResult, AcceptedSave, error) {
		return SaveResult{Status: SaveAccepted}, commitCapability(func(ctx context.Context) (SaveResult, error) {
			close(started)
			<-release
			if ctx.Err() != nil {
				return SaveResult{}, ctx.Err()
			}
			return SaveResult{Status: SaveConfirmed, Receipt: &Receipt{OperationID: input.OperationID}}, nil
		}), nil
	}
	waiter, cancel := context.WithCancel(context.Background())
	result := make(chan SaveResult, 1)
	go func() { v, _ := owner.CommitRecord(waiter, scope, scope.InstallationID, auth, input); result <- v }()
	<-started
	cancel()
	closeView, err := owner.CloseWorkspace(context.Background(), scope, scope.InstallationID, scope.TenantID, input.Epoch, auth)
	if err != nil || closeView.Closed {
		t.Fatal(closeView, err)
	}
	shutdown := owner.BeginShutdown()
	select {
	case <-shutdown:
		t.Fatal("shutdown called accepted IO drained")
	default:
	}
	if r.closed != 0 {
		t.Fatal("physical resources released before commit ended")
	}
	close(release)
	got := <-result
	if got.Status != SaveConfirmed || !got.ViewDeliveryFailed {
		t.Fatal(got)
	}
	select {
	case <-shutdown:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	if r.closed != 1 {
		t.Fatal(r.closed)
	}
	if _, err = owner.Open(context.Background(), scope, auth, func(context.Context, string) (OpenEvidence, error) {
		t.Fatal("shutdown permitted reopen")
		return OpenEvidence{}, nil
	}); !errors.Is(err, ErrLifecycle) {
		t.Fatal(err)
	}
}

func TestCloseOwnerCutsOffDispatchedButPreAdmissionCommit(t *testing.T) {
	owner, _, scope, r, in := commitOwnerFixture(t)
	admissions := 0
	r.admit = func(context.Context, Scope, SaveRequest, bool) (SaveResult, AcceptedSave, error) {
		admissions++
		return SaveResult{Status: SaveRejected}, nil, nil
	}
	paused, release := make(chan struct{}), make(chan struct{})
	calls := 0
	auth := func(context.Context) error {
		calls++
		if calls == 2 {
			close(paused)
			<-release
		}
		return nil
	}
	result := make(chan error, 1)
	go func() {
		_, err := owner.CommitRecord(context.Background(), scope, scope.TenantID, auth, in)
		result <- err
	}()
	<-paused
	closing, err := owner.CloseWorkspace(context.Background(), scope, scope.TenantID, scope.TenantID, in.Epoch, func(context.Context) error { return nil })
	if err != nil || closing.Closed {
		close(release)
		t.Fatal(closing, err)
	}
	close(release)
	if err := <-result; !errors.Is(err, ErrLifecycle) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	closed, err := owner.WaitClose(ctx, scope, scope.TenantID, scope.TenantID, false, func(context.Context) error { return nil })
	if err != nil || !closed.Closed || admissions != 0 {
		t.Fatal(closed, err, admissions)
	}
}

type closeBoundaryResources struct {
	openTestResources
	beforeClose func()
}

func (r *closeBoundaryResources) Close() error {
	if r.beforeClose != nil {
		r.beforeClose()
	}
	return r.openTestResources.Close()
}
func TestCloseOwnerPublishesGenerationBeforeReopenAndShutdown(t *testing.T) {
	o, _, s := openTestOwner(t)
	ctx := context.Background()
	auth := func(context.Context) error { return nil }
	physical, release := make(chan struct{}), make(chan struct{})
	old := &closeBoundaryResources{beforeClose: func() { close(physical); <-release }}
	evidence := func(r OpenResources) OpenEvidence {
		return OpenEvidence{Resources: r, Snapshot: Snapshot{Identity: Identity{WorkspaceID: s.WorkspaceID, TenantID: s.TenantID, BusinessState: "ready"}}}
	}
	first, err := o.Open(ctx, s, auth, func(context.Context, string) (OpenEvidence, error) { return evidence(old), nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = o.CloseWorkspace(ctx, s, s.InstallationID, s.WorkspaceID, first.Epoch, auth); err != nil {
		t.Fatal(err)
	}
	<-physical
	o.closeMu.Lock()
	locked := true
	defer func() {
		if locked {
			o.closeMu.Unlock()
		}
	}()
	close(release)
	for {
		o.queue.mu.Lock()
		closed, changed := o.queue.closed, o.queue.changed
		o.queue.mu.Unlock()
		if closed {
			break
		}
		select {
		case <-changed:
		case <-time.After(3 * time.Second):
			t.Fatal("physical close did not end")
		}
	}
	factory, proceed := make(chan struct{}), make(chan struct{})
	fresh := &openTestResources{}
	opened := make(chan error, 1)
	go func() {
		_, err := o.Open(ctx, s, auth, func(context.Context, string) (OpenEvidence, error) {
			close(factory)
			<-proceed
			return evidence(fresh), nil
		})
		opened <- err
	}()
	waitQueueUsage(t, o.queue, 2)
	select {
	case <-factory:
		t.Fatal("reopen dispatched before predecessor terminal history")
	default:
	}
	o.closeMu.Unlock()
	locked = false
	<-factory
	done := o.BeginShutdown()
	close(proceed)
	if err := <-opened; err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown joined retired close generation")
	}
	if old.closed != 1 || fresh.closed != 1 || o.queue.capacity.Usage() != 0 {
		t.Fatal(old.closed, fresh.closed, o.queue.capacity.Usage())
	}
}
