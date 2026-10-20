package workspace

import (
	"context"
	"sync"
)

// OperationCapacity is the ONE installation quota owner. Queue serialization,
// request delivery, and retained native ownership hold references to one counted
// allocation; they do not independently charge the installation or workspace.
// Lock order is queue.mu -> capacity.mu; capacity never acquires a queue mutex.
type OperationCapacity struct {
	mu             sync.Mutex
	used, ordinary int
	local          map[*operationQueue]capacityUse
	changed        chan struct{}
}
type capacityUse struct{ used, ordinary int }
type operationAllocation struct {
	capacity                     *OperationCapacity
	queue                        *operationQueue
	refs                         int
	control, reserved, executing bool
	requestClaim                 bool // at most one actual HTTP/body/delivery lifetime
	generation                   uint64
}

// OperationReservation is one owned reference, not authority or a save result.
// Retain creates a distinct reference for physical ownership. Release is
// idempotent for THIS reference, never a release of somebody else's reference.
type OperationReservation struct{ state *reservationReference }
type reservationReference struct {
	allocation        *operationAllocation
	released          bool // guarded by allocation.capacity.mu; copies share this state
	requestClaim      bool
	investigationKey  string // inherited by derived refs; never authority
	investigationKind uint8
}
type reservationContextKey struct{}
type reservationContext struct{ reference *OperationReservation }

func NewOperationCapacity() *OperationCapacity {
	return &OperationCapacity{local: make(map[*operationQueue]capacityUse), changed: make(chan struct{})}
}
func (c *OperationCapacity) signalLocked() { close(c.changed); c.changed = make(chan struct{}) }
func (c *OperationCapacity) watch() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.changed
}
func (c *OperationCapacity) Usage() int { c.mu.Lock(); defer c.mu.Unlock(); return c.used }
func (c *OperationCapacity) localUsage(q *operationQueue) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.local[q].used
}

// ReserveOperation counts installation-only work before its workspace is known.
// It is always ordinary-backed; only an owner with a runnable control turn can
// allocate the existing reserved position. Binding a later workspace never
// releases/reacquires this global allocation.
func (c *OperationCapacity) ReserveOperation() (*OperationReservation, error) {
	return c.reserve(nil, false, false)
}

// reserve is called with q.mu held when q != nil.
func (c *OperationCapacity) reserve(q *operationQueue, control, reserved bool) (*OperationReservation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	u := c.local[q]
	if c.used >= InstallationOperationLimit || (!reserved && c.ordinary >= InstallationOperationLimit-1) || (q != nil && (u.used >= WorkspaceOperationLimit || (!control && u.ordinary >= WorkspaceOperationLimit-1))) {
		return nil, ErrCapacity
	}
	a := &operationAllocation{capacity: c, queue: q, refs: 1, control: control, reserved: reserved}
	if q != nil {
		a.generation = q.generation
		u.used++
		if !control {
			u.ordinary++
		}
		c.local[q] = u
	}
	c.used++
	if !reserved {
		c.ordinary++
	}
	c.signalLocked()
	return &OperationReservation{state: &reservationReference{allocation: a}}, nil
}

// RetainOperation is used by a trusted resource owner. A request context shares
// its existing allocation; direct internal calls allocate one ordinary position.
// It never grants authority, binds a different queue, or borrows a control reserve
// for newly started physical work.
func (c *OperationCapacity) RetainOperation(ctx context.Context) (*OperationReservation, error) {
	value, ok := ctx.Value(reservationContextKey{}).(reservationContext)
	if !ok {
		return c.ReserveOperation()
	}
	r := value.reference
	c.mu.Lock()
	defer c.mu.Unlock()
	if r == nil || r.state == nil || r.state.allocation == nil || r.state.allocation.capacity != c {
		return nil, ErrScope
	}
	a := r.state.allocation
	if r.state.released || a.refs == 0 {
		return nil, ErrLifecycle
	}
	if r.state.investigationKey != "" {
		return nil, ErrScope
	}
	if err := c.ordinaryBackingLocked(a); err != nil {
		return nil, err
	}
	a.refs++
	return &OperationReservation{state: &reservationReference{allocation: a}}, nil
}
func (r *OperationReservation) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, reservationContextKey{}, reservationContext{r})
}
func (r *OperationReservation) Retain() (*OperationReservation, error) {
	if r == nil || r.state == nil || r.state.allocation == nil {
		return nil, ErrScope
	}
	c := r.state.allocation.capacity
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.state.released || r.state.allocation.refs == 0 {
		return nil, ErrLifecycle
	}
	r.state.allocation.refs++
	return &OperationReservation{state: &reservationReference{allocation: r.state.allocation, investigationKey: r.state.investigationKey, investigationKind: r.state.investigationKind}}, nil
}
func (r *OperationReservation) Release() {
	if r == nil || r.state == nil || r.state.allocation == nil {
		return
	}
	c := r.state.allocation.capacity
	c.mu.Lock()
	defer c.mu.Unlock()
	r.releaseLocked()
}
func (r *OperationReservation) releaseLocked() {
	if r.state.released {
		return
	}
	r.state.released = true
	a := r.state.allocation
	if r.state.requestClaim {
		a.requestClaim = false
	}
	a.refs--
	if a.refs != 0 {
		return
	}
	c := a.capacity
	c.used--
	if !a.reserved {
		c.ordinary--
	}
	if a.queue != nil {
		u := c.local[a.queue]
		u.used--
		if !a.control {
			u.ordinary--
		}
		if u.used == 0 {
			delete(c.local, a.queue)
		} else {
			c.local[a.queue] = u
		}
	}
	c.signalLocked()
}

// bindLocked adds local accounting once. q.mu and capacity.mu are already held.
func (c *OperationCapacity) bindLocked(r *OperationReservation, q *operationQueue, control bool) error {
	if r == nil || r.state == nil || r.state.allocation == nil || r.state.allocation.capacity != c {
		return ErrScope
	}
	a := r.state.allocation
	if r.state.released || a.refs == 0 {
		return ErrLifecycle
	}
	if a.queue != nil && a.queue != q {
		return ErrScope
	}
	if a.queue == nil {
		u := c.local[q]
		if u.used >= WorkspaceOperationLimit || (!control && u.ordinary >= WorkspaceOperationLimit-1) {
			return ErrCapacity
		}
		a.queue = q
		a.control = control
		a.generation = q.generation
		u.used++
		if !control {
			u.ordinary++
		}
		c.local[q] = u
		c.signalLocked()
	}
	return nil
}
func (c *OperationCapacity) ordinaryBackingLocked(a *operationAllocation) error {
	if !a.control && !a.reserved {
		return nil
	}
	u := c.local[a.queue]
	if (a.reserved && c.ordinary >= InstallationOperationLimit-1) || (a.queue != nil && a.control && u.ordinary >= WorkspaceOperationLimit-1) {
		return ErrCapacity
	}
	if a.reserved {
		c.ordinary++
		a.reserved = false
	}
	if a.control {
		a.control = false
		if a.queue != nil {
			u.ordinary++
			c.local[a.queue] = u
		}
	}
	return nil
}

// beginTurn borrows a distinct execution reference, never a second count.
func (c *OperationCapacity) beginTurn(r *OperationReservation, q *operationQueue, control bool) (*OperationReservation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r != nil && r.state != nil && r.state.investigationKey != "" && (r.state.investigationKind != investigationWorkspace || r.state.allocation == nil || r.state.allocation.queue != q) {
		return nil, ErrScope
	}
	if err := c.bindLocked(r, q, control); err != nil {
		return nil, err
	}
	a := r.state.allocation
	if !control && r.state.investigationKey != "" {
		return nil, ErrScope
	}
	if a.executing || (!control && a.generation != q.generation) {
		return nil, ErrLifecycle
	}
	// A reserved query cannot smuggle a later ordinary mutation into the reserve.
	if !control {
		if err := c.ordinaryBackingLocked(a); err != nil {
			return nil, err
		}
	}
	a.refs++
	a.executing = true
	return &OperationReservation{state: &reservationReference{allocation: a, investigationKey: r.state.investigationKey, investigationKind: r.state.investigationKind}}, nil
}
func (r *OperationReservation) endTurn() {
	c := r.state.allocation.capacity
	c.mu.Lock()
	defer c.mu.Unlock()
	r.state.allocation.executing = false
}

// reserveTurn uses the identical reference protocol for non-HTTP business work.
func (q *operationQueue) reserveTurn(ctx context.Context, control bool) (*OperationReservation, error) {
	if value, ok := ctx.Value(reservationContextKey{}).(reservationContext); ok {
		return q.capacity.beginTurn(value.reference, q, control)
	}
	r, err := q.capacity.reserve(q, control, control && q.active == nil)
	if err != nil {
		return nil, err
	}
	t, err := q.capacity.beginTurn(r, q, control)
	r.Release()
	return t, err
}

// retainCloseReference is called with q.mu held before the cutoff. The accepted
// close owns an ordinary-backed reference even after its HTTP waiter is gone.
func (q *operationQueue) retainCloseReference(ctx context.Context) (*OperationReservation, error) {
	if err := CheckInvestigationReservation(ctx, ""); err != nil {
		return nil, err
	}
	value, ok := ctx.Value(reservationContextKey{}).(reservationContext)
	if !ok {
		return nil, nil
	}
	c := q.capacity
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.bindLocked(value.reference, q, false); err != nil {
		return nil, err
	}
	a := value.reference.state.allocation
	if a.executing || a.generation != q.generation {
		return nil, ErrLifecycle
	}
	if err := c.ordinaryBackingLocked(a); err != nil {
		return nil, err
	}
	a.refs++
	return &OperationReservation{state: &reservationReference{allocation: a}}, nil
}

// ReserveOperation binds an external request to this same scope before reading
// its body, but does not take a serialization turn or open physical resources.
// HTTP composition must still retain it through delivery and native ownership.
func (o *OpenOwner) ReserveOperation(ctx context.Context, scope Scope, authorize func(context.Context) error, control, opening bool) (*OperationReservation, error) {
	if scope != o.scope {
		return nil, ErrScope
	}
	if authorize == nil {
		return nil, ErrOpen
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := authorize(ctx); err != nil {
		return nil, err
	}
	if err := CheckInvestigationReservation(ctx, ""); err != nil {
		return nil, err
	}
	q := o.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.drainWaiting || (q.stopping && !control) || (q.closing && !control && !(opening && q.closed)) {
		return nil, ErrLifecycle
	}
	if value, ok := ctx.Value(reservationContextKey{}).(reservationContext); ok {
		q.capacity.mu.Lock()
		defer q.capacity.mu.Unlock()
		if err := q.capacity.bindLocked(value.reference, q, false); err != nil {
			return nil, err
		}
		a := value.reference.state.allocation
		if !control && a.generation != q.generation {
			return nil, ErrLifecycle
		}
		if err := q.capacity.ordinaryBackingLocked(a); err != nil {
			return nil, err
		}
		a.refs++
		return &OperationReservation{state: &reservationReference{allocation: a}}, nil
	}
	// A prebody control has no runnable turn and cannot occupy either reserve.
	return q.capacity.reserve(q, false, false)
}
