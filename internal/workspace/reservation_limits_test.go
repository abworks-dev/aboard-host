package workspace

import (
	"context"
	"errors"
	"testing"
)

func TestReservationCanceledQueueTurnDoesNotReleaseRequest(t *testing.T) {
	c := NewOperationCapacity()
	q := newOperationQueue(c)
	active, err := q.enter(context.Background(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.ReserveOperation()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(r.Context(context.Background()))
	finished := make(chan error, 1)
	go func() {
		ticket, err := q.enter(ctx, false, false)
		if ticket != nil {
			ticket.finish()
		}
		finished <- err
	}()
	waitQueueUsage(t, q, 2)
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if c.Usage() != 2 {
		t.Fatal("canceled queue waiter released request reference", c.Usage())
	}
	r.Release()
	if c.Usage() != 1 {
		t.Fatal(c.Usage())
	}
	active.finish()
	if c.Usage() != 0 {
		t.Fatal(c.Usage())
	}
}

func TestReservationControlCannotPromoteIntoFullOrdinaryBudget(t *testing.T) {
	c := NewOperationCapacity()
	q := newOperationQueue(c)
	var ordinary []*OperationReservation
	defer func() {
		for _, r := range ordinary {
			r.Release()
		}
	}()
	for i := 0; i < 63; i++ {
		r, err := c.ReserveOperation()
		if err != nil {
			t.Fatal(err)
		}
		ordinary = append(ordinary, r)
	}
	control, err := q.enter(context.Background(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := control.reservation.Retain()
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	control.finish()
	if _, err := q.enter(ref.Context(context.Background()), false, false); !errors.Is(err, ErrCapacity) {
		t.Fatal("ordinary work entered shared control reserve", err)
	}
	if c.Usage() != 64 {
		t.Fatal(c.Usage())
	}
	ordinary[0].Release()
	ticket, err := q.enter(ref.Context(context.Background()), false, false)
	if err != nil {
		t.Fatal("ordinary backing did not become available", err)
	}
	ticket.finish()
	c.mu.Lock()
	used, backing := c.used, c.ordinary
	c.mu.Unlock()
	if used != 63 || backing != 63 {
		t.Fatal("promotion double-counted or kept reserved ownership", used, backing)
	}
}
