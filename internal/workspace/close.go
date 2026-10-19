package workspace

import "context"

// CloseView is process-local coordination history, never a durable save receipt.
// A fresh process has no evidence for a previous process's close operation.
type CloseView struct {
	Scope                           Scope
	OperationID, PrincipalID, Epoch string
	State                           LifecycleState
	Closed                          bool
}
type closeOperation struct {
	view CloseView
	done chan struct{}
}

func (o *OpenOwner) CloseWorkspace(ctx context.Context, scope Scope, principal, operationID, epoch string, authorize func(context.Context) error) (CloseView, error) {
	if scope != o.scope {
		return CloseView{}, ErrScope
	}
	if authorize == nil || !recordID(principal) || !recordID(operationID) || !recordID(epoch) {
		return CloseView{}, ErrSaveInput
	}
	if err := ctx.Err(); err != nil {
		return CloseView{}, err
	}
	if err := authorize(ctx); err != nil {
		return CloseView{}, err
	}
	o.closeMu.Lock()
	defer o.closeMu.Unlock()
	if previous := o.closes[operationID]; previous != nil {
		if previous.view.PrincipalID != principal || previous.view.Epoch != epoch {
			return CloseView{}, ErrSaveKeyReuse
		}
		return previous.view, nil
	}
	q := o.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.epoch != epoch || q.closed {
		return CloseView{}, ErrStaleHandle
	}
	if q.closing {
		return CloseView{}, ErrLifecycle
	}
	// This short critical section rejects new ordinary work without waiting for
	// the workspace's active physical IO. Only already executing work keeps its
	// epoch; queued requests have no durable admission and are rejected.
	q.stopLocked()
	operation := &closeOperation{view: CloseView{Scope: scope, PrincipalID: principal, OperationID: operationID, Epoch: epoch, State: LifecycleDraining}, done: make(chan struct{})}
	o.closes[operationID] = operation
	o.closing = operation
	go o.finishClose(context.WithoutCancel(ctx), scope, operation)
	return operation.view, nil
}
func (o *OpenOwner) GetCloseOperation(ctx context.Context, scope Scope, principal, operationID string, operator bool, authorize func(context.Context) error) (CloseView, error) {
	if scope != o.scope {
		return CloseView{}, ErrScope
	}
	if authorize == nil {
		return CloseView{}, ErrSaveInput
	}
	if err := ctx.Err(); err != nil {
		return CloseView{}, err
	}
	if err := authorize(ctx); err != nil {
		return CloseView{}, err
	}
	o.closeMu.Lock()
	defer o.closeMu.Unlock()
	op := o.closes[operationID]
	if op == nil {
		return CloseView{}, ErrOperationHistoryUnavailable
	}
	if !operator && op.view.PrincipalID != principal {
		return CloseView{}, ErrOperationAccess
	}
	return op.view, nil
}
func (o *OpenOwner) WaitClose(ctx context.Context, scope Scope, principal, operationID string, operator bool, authorize func(context.Context) error) (CloseView, error) {
	if _, err := o.GetCloseOperation(ctx, scope, principal, operationID, operator, authorize); err != nil {
		return CloseView{}, err
	}
	o.closeMu.Lock()
	done := o.closes[operationID].done
	o.closeMu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
	}
	view, err := o.GetCloseOperation(context.WithoutCancel(ctx), scope, principal, operationID, operator, authorize)
	if err != nil {
		return CloseView{}, err
	}
	if ctx.Err() != nil {
		return view, ctx.Err()
	}
	return view, nil
}
func (o *OpenOwner) finishClose(ctx context.Context, scope Scope, operation *closeOperation) {
	for {
		ticket, err := o.queue.enterDrain(ctx)
		if err != nil {
			return
		} // detached owner context has no waiter deadline
		o.mu.Lock()
		state, _ := o.lifecycle.State(scope)
		emptyClosed := o.resources == nil && state == LifecycleClosed
		o.mu.Unlock()
		if emptyClosed {
			o.queue.ended()
		} else {
			err = o.DrainOpen(ctx, scope, func(context.Context) error { return nil })
		}
		o.closeMu.Lock()
		if err == nil {
			operation.view.State = LifecycleClosed
			operation.view.Closed = true
			if o.closing == operation {
				o.closing = nil
			}
			// Publish terminal history before dispatching a queued reopen, and
			// release its capacity before notifying waiters. closeMu prevents
			// shutdown from joining the retired generation during this handoff.
			ticket.finish()
			close(operation.done)
			o.closeMu.Unlock()
			return
		}
		operation.view.State = LifecycleUnknown
		o.closeMu.Unlock()
		changed := ticket.finish()
		// An unknown close retains resources/capacity. Only a subsequent explicit
		// investigation or real work completion can justify another physical attempt.
		<-changed
	}
}

// BeginShutdown is a trusted process teardown path. It never borrows a client
// session or request deadline, and cannot clear an unknown foreign lifecycle.
func (o *OpenOwner) BeginShutdown() <-chan struct{} {
	o.closeMu.Lock()
	defer o.closeMu.Unlock()
	o.queue.mu.Lock()
	o.queue.stopping = true
	o.queue.stopLocked()
	o.queue.mu.Unlock()
	if o.closing != nil {
		return o.closing.done
	}
	operation := &closeOperation{view: CloseView{Scope: o.scope, State: LifecycleDraining}, done: make(chan struct{})}
	o.closing = operation
	go o.finishClose(context.Background(), o.scope, operation)
	return operation.done
}
func (q *operationQueue) enterDrain(ctx context.Context) (*queueTicket, error) {
	// Drain intent is metadata, not a waiting HTTP allocation. Already active
	// physical work finishes; not-yet-started controls are rejected so a stream
	// of polling/reconcile requests cannot starve actual close indefinitely.
	q.mu.Lock()
	q.drainWaiting = true
	for _, ticket := range q.waiting {
		select {
		case <-ticket.rejected:
		default:
			close(ticket.rejected)
		}
	}
	q.signalLocked()
	q.mu.Unlock()
	acquired := false
	defer func() {
		if !acquired {
			q.mu.Lock()
			q.drainWaiting = false
			q.dispatchLocked()
			q.signalLocked()
			q.mu.Unlock()
		}
	}()
	for {
		q.mu.Lock()
		globalChanged := q.capacity.watch()
		if q.active == nil && len(q.waiting) == 0 {
			if q.used < WorkspaceOperationLimit && q.capacity.acquire(true) {
				t := &queueTicket{queue: q, control: true, reserved: true, ready: make(chan struct{})}
				q.used++
				q.active = t
				q.drainWaiting = false
				acquired = true
				close(t.ready)
				q.mu.Unlock()
				return t, nil
			}
		}
		changed := q.changed
		q.mu.Unlock()
		select {
		case <-changed:
		case <-globalChanged:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
