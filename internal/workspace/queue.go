package workspace

import (
	"context"
	"errors"
	"sync"
)

var ErrCapacity = errors.New("workspace or installation operation capacity reached")

const WorkspaceOperationLimit = 32
const InstallationOperationLimit = 64

// OperationCapacity belongs to ONE installation. A reserved control position is
// included in (not added to) each limit, so full ordinary queues cannot prevent
// investigation. No installation mutex is held while a workspace performs IO.
type OperationCapacity struct {
	mu             sync.Mutex
	used, ordinary int
	changed        chan struct{}
}

func NewOperationCapacity() *OperationCapacity {
	return &OperationCapacity{changed: make(chan struct{})}
}
func (c *OperationCapacity) watch() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.changed
}
func (c *OperationCapacity) acquire(control bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used >= InstallationOperationLimit || (!control && c.ordinary >= InstallationOperationLimit-1) {
		return false
	}
	c.used++
	if !control {
		c.ordinary++
	}
	return true
}
func (c *OperationCapacity) release(control bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.used--
	if !control {
		c.ordinary--
	}
	close(c.changed)
	c.changed = make(chan struct{})
}
func (c *OperationCapacity) Usage() int { c.mu.Lock(); defer c.mu.Unlock(); return c.used }

type operationQueue struct {
	mu                        sync.Mutex
	capacity                  *OperationCapacity
	active                    *queueTicket
	waiting                   []*queueTicket
	held                      map[string]*queueTicket
	used, ordinary            int
	closing, closed, stopping bool
	drainWaiting              bool
	epoch                     string
	changed                   chan struct{}
}
type queueTicket struct {
	queue    *operationQueue
	control  bool
	reserved bool
	ready    chan struct{}
	rejected chan struct{}
	once     sync.Once
	hold     string
}

func newOperationQueue(c *OperationCapacity) *operationQueue {
	return &operationQueue{capacity: c, held: make(map[string]*queueTicket), changed: make(chan struct{})}
}
func (q *operationQueue) signalLocked() { close(q.changed); q.changed = make(chan struct{}) }
func (q *operationQueue) dispatchLocked() {
	if q.active != nil || q.drainWaiting || len(q.waiting) == 0 {
		return
	}
	index := 0
	for i, t := range q.waiting {
		if t.control {
			index = i
			break
		}
	}
	t := q.waiting[index]
	q.waiting = append(q.waiting[:index], q.waiting[index+1:]...)
	q.active = t
	close(t.ready)
}
func (q *operationQueue) enter(ctx context.Context, control, opening bool) (*queueTicket, error) {
	q.mu.Lock()
	if q.drainWaiting {
		q.mu.Unlock()
		return nil, ErrLifecycle
	}
	if q.stopping && !control {
		q.mu.Unlock()
		return nil, ErrLifecycle
	}
	if opening && q.closed {
		q.closed = false
		q.closing = false
	}
	if q.closing && !control {
		q.mu.Unlock()
		return nil, ErrLifecycle
	}
	if err := ctx.Err(); err != nil {
		q.mu.Unlock()
		return nil, err
	}
	// A control waiting behind unrelated physical IO must not pin the shared
	// reserve. Such a queued control uses ordinary capacity; only runnable
	// investigation can use the reserved position.
	reserved := control && q.active == nil
	if q.used >= WorkspaceOperationLimit || (!control && q.ordinary >= WorkspaceOperationLimit-1) || !q.capacity.acquire(reserved) {
		q.mu.Unlock()
		return nil, ErrCapacity
	}
	t := &queueTicket{queue: q, control: control, reserved: reserved, ready: make(chan struct{}), rejected: make(chan struct{})}
	q.used++
	if !control {
		q.ordinary++
	}
	q.waiting = append(q.waiting, t)
	q.dispatchLocked()
	q.signalLocked()
	q.mu.Unlock()
	select {
	case <-t.rejected:
		t.finish()
		return nil, ErrLifecycle
	case <-t.ready:
		select {
		case <-t.rejected:
			t.finish()
			return nil, ErrLifecycle
		default:
		}
		if err := ctx.Err(); err != nil {
			t.finish()
			return nil, err
		}
		return t, nil
	case <-ctx.Done():
		t.finish()
		return nil, ctx.Err()
	}
}
func (t *queueTicket) retain(key string) { t.queue.mu.Lock(); t.hold = key; t.queue.mu.Unlock() }
func (t *queueTicket) finish() <-chan struct{} {
	var changed <-chan struct{}
	t.once.Do(func() {
		q := t.queue
		q.mu.Lock()
		defer q.mu.Unlock()
		if q.active == t {
			q.active = nil
		} else {
			for i, v := range q.waiting {
				if v == t {
					q.waiting = append(q.waiting[:i], q.waiting[i+1:]...)
					break
				}
			}
		}
		if t.hold != "" && q.held[t.hold] == nil {
			q.held[t.hold] = t
		} else {
			q.releaseLocked(t)
		}
		q.dispatchLocked()
		q.signalLocked()
		changed = q.changed
	})
	return changed
}
func (q *operationQueue) releaseLocked(t *queueTicket) {
	q.used--
	if !t.control {
		q.ordinary--
	}
	q.capacity.release(t.reserved)
}
func (q *operationQueue) resolve(key string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if t := q.held[key]; t != nil {
		delete(q.held, key)
		q.releaseLocked(t)
		q.signalLocked()
	}
}

// beforeMutation is the close cutoff for a dispatched request that has not yet
// started persistence IO. No queue mutex is retained across that IO.
func (q *operationQueue) beforeMutation() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closing || q.active == nil {
		return ErrLifecycle
	}
	return nil
}
func (q *operationQueue) stopLocked() {
	if !q.closing {
		q.closing = true
		for _, ticket := range q.waiting {
			if !ticket.control {
				select {
				case <-ticket.rejected:
				default:
					close(ticket.rejected)
				}
			}
		}
	}
	q.signalLocked()
}
func (q *operationQueue) ended() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for key, t := range q.held {
		q.releaseLocked(t)
		delete(q.held, key)
	}
	q.closed = true
	q.signalLocked()
}
func (o *OpenOwner) beginOperation(ctx context.Context, scope Scope, authorize func(context.Context) error, control, opening bool) (*queueTicket, error) {
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
	return o.queue.enter(ctx, control, opening)
}
