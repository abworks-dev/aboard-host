package workspace

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func waitQueueUsage(t *testing.T, q *operationQueue, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		q.mu.Lock()
		got := q.capacity.localUsage(q)
		changed := q.changed
		q.mu.Unlock()
		if got == want {
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatalf("queue usage %d, want %d", got, want)
		}
	}
}
func holdQueueTickets(t *testing.T, q *operationQueue, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		ticket, err := q.enter(context.Background(), false, false)
		if err != nil {
			t.Fatal(err)
		}
		ticket.retain(fmt.Sprint(i))
		ticket.finish()
	}
}
func TestOperationQueueHardBoundsRetainedUnknownAndRunnableReserve(t *testing.T) {
	capacity := NewOperationCapacity()
	a := newOperationQueue(capacity)
	b := newOperationQueue(capacity)
	c := newOperationQueue(capacity)
	holdQueueTickets(t, a, 30)
	active, err := a.enter(context.Background(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	holdQueueTickets(t, b, 31)
	holdQueueTickets(t, c, 1)
	if capacity.Usage() != 63 {
		t.Fatal(capacity.Usage())
	}
	if _, err = a.enter(context.Background(), false, false); !errors.Is(err, ErrCapacity) {
		t.Fatal("A ordinary limit", err)
	}
	if _, err = a.enter(context.Background(), true, false); !errors.Is(err, ErrCapacity) {
		t.Fatal("blocked A control monopolized reserve", err)
	}
	control, err := c.enter(context.Background(), true, false)
	if err != nil || capacity.Usage() != 64 {
		t.Fatal("idle C cannot investigate", err, capacity.Usage())
	}
	if _, err = b.enter(context.Background(), true, false); !errors.Is(err, ErrCapacity) {
		t.Fatal("global hard bound", err)
	}
	c.resolve("0")
	if capacity.Usage() != 63 {
		t.Fatal("known result did not release original held slot", capacity.Usage())
	}
	control.finish()
	// A runnable control may use its reserved 32nd position even when 31 unknown
	// operations remain. A duplicate key inquiry never allocates another hold.
	active.retain("active")
	active.finish()
	control, err = a.enter(context.Background(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	used := a.capacity.localUsage(a)
	a.mu.Unlock()
	if used != 32 {
		t.Fatal(used)
	}
	control.finish()
	a.ended()
	b.ended()
	c.ended()
	if capacity.Usage() != 0 {
		t.Fatal("capacity leak", capacity.Usage())
	}
}
func TestOperationQueueFIFOControlAndCanceledWaiter(t *testing.T) {
	q := newOperationQueue(NewOperationCapacity())
	active, _ := q.enter(context.Background(), false, false)
	ordinary := make(chan *queueTicket, 1)
	go func() { v, _ := q.enter(context.Background(), false, false); ordinary <- v }()
	waitQueueUsage(t, q, 2)
	control := make(chan *queueTicket, 1)
	go func() { v, _ := q.enter(context.Background(), true, false); control <- v }()
	waitQueueUsage(t, q, 3)
	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() { _, err := q.enter(ctx, false, false); canceled <- err }()
	waitQueueUsage(t, q, 4)
	cancel()
	if err := <-canceled; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	waitQueueUsage(t, q, 3)
	active.finish()
	ticket := <-control
	select {
	case <-ordinary:
		t.Fatal("ordinary bypassed queued control")
	default:
	}
	ticket.finish()
	(<-ordinary).finish()
	if q.capacity.Usage() != 0 {
		t.Fatal(q.capacity.Usage())
	}
}

func TestOperationQueuePendingDrainCannotBeStarvedByControls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	q := newOperationQueue(NewOperationCapacity())
	active, err := q.enter(ctx, false, false)
	if err != nil {
		t.Fatal(err)
	}
	rejected := make(chan error, 1)
	go func() {
		v, err := q.enter(ctx, true, false)
		if v != nil {
			v.finish()
		}
		rejected <- err
	}()
	waitQueueUsage(t, q, 2)
	drained := make(chan *queueTicket, 1)
	go func() {
		v, err := q.enterDrain(ctx)
		if err == nil {
			drained <- v
		}
	}()
	for {
		q.mu.Lock()
		pending, changed := q.drainWaiting, q.changed
		q.mu.Unlock()
		if pending {
			break
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if _, err = q.enter(ctx, true, false); !errors.Is(err, ErrLifecycle) {
		t.Fatal("replacement control bypassed pending drain", err)
	}
	if err = <-rejected; !errors.Is(err, ErrLifecycle) {
		t.Fatal("queued pre-IO control not rejected", err)
	}
	active.finish()
	var drain *queueTicket
	select {
	case drain = <-drained:
	case <-ctx.Done():
		t.Fatal("drain did not gain priority", ctx.Err())
	}
	drain.finish()
	control, err := q.enter(ctx, true, false)
	if err != nil {
		t.Fatal("failed drain would block explicit investigation", err)
	}
	control.finish()
	if q.capacity.Usage() != 0 {
		t.Fatal(q.capacity.Usage())
	}
}

func TestOperationQueueLocalControlReserveBehindActiveIO(t *testing.T) {
	q := newOperationQueue(NewOperationCapacity())
	holdQueueTickets(t, q, 30)
	active, err := q.enter(context.Background(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan *queueTicket, 1)
	fail := make(chan error, 1)
	go func() {
		v, err := q.enter(context.Background(), true, false)
		if err != nil {
			fail <- err
			return
		}
		got <- v
	}()
	waitQueueUsage(t, q, 32)
	select {
	case err := <-fail:
		t.Fatal(err)
	case <-got:
		t.Fatal("control bypassed live IO")
	default:
	}
	active.finish()
	control := <-got
	control.finish()
	q.ended()
	if q.capacity.Usage() != 0 {
		t.Fatal(q.capacity.Usage())
	}
}
