package workspace

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestReservationCopiesReleaseOneReferenceNotRetainedOwnership(t *testing.T) {
	c := NewOperationCapacity()
	r, err := c.ReserveOperation()
	if err != nil {
		t.Fatal(err)
	}
	retained, err := r.Retain()
	if err != nil {
		t.Fatal(err)
	}
	copyOfReference := *r
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); copyOfReference.Release(); r.Release() }()
	}
	wg.Wait()
	if c.Usage() != 1 {
		t.Fatal("value copy freed retained physical owner", c.Usage())
	}
	if _, err := r.Retain(); !errors.Is(err, ErrLifecycle) {
		t.Fatal(err)
	}
	retained.Release()
	retained.Release()
	if c.Usage() != 0 {
		t.Fatal(c.Usage())
	}
	if _, err := newOperationQueue(c).enter(r.Context(context.Background()), false, false); !errors.Is(err, ErrLifecycle) {
		t.Fatal("released reference reused", err)
	}
}

func TestReservationSequentialTurnsDeliveryAndUnknownShareOneCount(t *testing.T) {
	c := NewOperationCapacity()
	q := newOperationQueue(c)
	r, err := c.ReserveOperation()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release()
	ctx := r.Context(context.Background())
	for i := 0; i < 3; i++ {
		ticket, err := q.enter(ctx, i == 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if c.Usage() != 1 || c.localUsage(q) != 1 {
			t.Fatal("double charged composition", c.Usage(), c.localUsage(q))
		}
		ticket.finish()
		if c.Usage() != 1 {
			t.Fatal("business return released delivery", c.Usage())
		}
	}
	ticket, err := q.enter(ctx, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.enter(ctx, false, false); !errors.Is(err, ErrLifecycle) {
		t.Fatal("concurrent reservation reuse", err)
	}
	ticket.retain("save:original")
	ticket.finish()
	r.Release()
	if c.Usage() != 1 {
		t.Fatal("delivery ended unknown ownership", c.Usage())
	}
	inquiry, err := q.enter(context.Background(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	inquiry.retain("save:original")
	inquiry.finish()
	if c.Usage() != 1 {
		t.Fatal("duplicate inquiry retained second quota", c.Usage())
	}
	q.resolve("save:original")
	q.resolve("save:original")
	if c.Usage() != 0 || c.localUsage(q) != 0 {
		t.Fatal(c.Usage(), c.localUsage(q))
	}
}

func TestReservationGlobalHandoffScopeAndActiveTurnOutliveCaller(t *testing.T) {
	c := NewOperationCapacity()
	a, b := newOperationQueue(c), newOperationQueue(c)
	r, err := c.ReserveOperation()
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := a.enter(r.Context(context.Background()), false, false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Usage() != 1 || c.localUsage(a) != 1 {
		t.Fatal(c.Usage(), c.localUsage(a))
	}
	if _, err := b.enter(r.Context(context.Background()), false, false); !errors.Is(err, ErrScope) {
		t.Fatal("workspace handoff repeated", err)
	}
	if _, err := newOperationQueue(NewOperationCapacity()).enter(r.Context(context.Background()), false, false); !errors.Is(err, ErrScope) {
		t.Fatal("foreign installation borrowed quota", err)
	}
	r.Release()
	if c.Usage() != 1 {
		t.Fatal("caller release ended active IO", c.Usage())
	}
	ticket.finish()
	if c.Usage() != 0 {
		t.Fatal(c.Usage())
	}
}

func TestReservationPrebodyControlCannotPinEitherReserve(t *testing.T) {
	ctx := context.Background()
	allow := func(context.Context) error { return nil }
	c := NewOperationCapacity()
	o, l, scope := openTestOwner(t)
	_ = l
	o.queue = newOperationQueue(c)
	var refs []*OperationReservation
	defer func() {
		for _, r := range refs {
			r.Release()
		}
	}()
	for i := 0; i < 31; i++ {
		r, err := o.ReserveOperation(ctx, scope, allow, false, false)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, r)
	}
	if _, err := o.ReserveOperation(ctx, scope, allow, true, false); !errors.Is(err, ErrCapacity) {
		t.Fatal("slow control used local reserve", err)
	}
	for i := 31; i < 63; i++ {
		r, err := c.ReserveOperation()
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, r)
	}
	if _, err := o.ReserveOperation(ctx, scope, allow, true, false); !errors.Is(err, ErrCapacity) {
		t.Fatal("slow control used global reserve", err)
	}
	control, err := o.queue.enter(ctx, true, false)
	if err != nil {
		t.Fatal("actual runnable control starved", err)
	}
	if c.Usage() != 64 || c.localUsage(o.queue) != 32 {
		t.Fatal(c.Usage(), c.localUsage(o.queue))
	}
	control.finish()
}

func TestReservationPrebodyOpenCannotCrossLaterCloseCutoff(t *testing.T) {
	o, _, scope, _, input := commitOwnerFixture(t)
	ctx := context.Background()
	allow := func(context.Context) error { return nil }
	r, err := o.ReserveOperation(ctx, scope, allow, false, true)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release()
	if _, err := o.CloseWorkspace(ctx, scope, scope.InstallationID, scope.TenantID, input.Epoch, allow); err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if view, err := o.WaitClose(wait, scope, scope.InstallationID, scope.TenantID, false, allow); err != nil || !view.Closed {
		t.Fatal(view, err)
	}
	called := false
	if _, err := o.Open(r.Context(ctx), scope, allow, func(context.Context, string) (OpenEvidence, error) { called = true; return OpenEvidence{}, nil }); !errors.Is(err, ErrLifecycle) || called {
		t.Fatal("late prebody open bypassed cutoff", called, err)
	}
	r.Release()
	if o.queue.capacity.Usage() != 0 {
		t.Fatal(o.queue.capacity.Usage())
	}
}

type reservationCloseResources struct {
	openTestResources
	started, release chan struct{}
}

func (r *reservationCloseResources) Close() error {
	close(r.started)
	<-r.release
	return r.openTestResources.Close()
}
func TestReservationAcceptedCloseAdoptsCountUntilActualCleanup(t *testing.T) {
	o, _, scope := openTestOwner(t)
	ctx := context.Background()
	allow := func(context.Context) error { return nil }
	resources := &reservationCloseResources{started: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	unblock := func() { once.Do(func() { close(resources.release) }) }
	defer unblock()
	view, err := o.Open(ctx, scope, allow, func(context.Context, string) (OpenEvidence, error) {
		return OpenEvidence{Resources: resources, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := o.ReserveOperation(ctx, scope, allow, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.CloseWorkspace(r.Context(ctx), scope, scope.InstallationID, scope.WorkspaceID, view.Epoch, allow); err != nil {
		t.Fatal(err)
	}
	r.Release()
	select {
	case <-resources.started:
	case <-time.After(3 * time.Second):
		t.Fatal("accepted close lost caller reservation")
	}
	if o.queue.capacity.Usage() != 1 {
		t.Fatal("close double charged or released active cleanup", o.queue.capacity.Usage())
	}
	unblock()
	wait, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if result, err := o.WaitClose(wait, scope, scope.InstallationID, scope.WorkspaceID, false, allow); err != nil || !result.Closed {
		t.Fatal(result, err)
	}
	if o.queue.capacity.Usage() != 0 || resources.closed != 1 {
		t.Fatal(o.queue.capacity.Usage(), resources.closed)
	}
}
