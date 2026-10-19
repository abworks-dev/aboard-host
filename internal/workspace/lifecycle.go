package workspace

import (
	"context"
	"errors"
	"sync"
)

var ErrLifecycle = errors.New("workspace lifecycle does not admit the operation")

// ErrLifecycleUncertain is returned or wrapped by a trusted binding action when
// physical cleanup or its result cannot be established. WithClosed fences the
// owner before releasing admission; ordinary known rejections do not fence it.
var ErrLifecycleUncertain = errors.New("workspace lifecycle physical completion is uncertain")

// LifecycleState is local admission state, not business state or an IO receipt.
// The zero value is unknown: only construction establishes an initially closed owner.
type LifecycleState string

const (
	LifecycleClosed   LifecycleState = "closed"
	LifecycleOpening  LifecycleState = "opening"
	LifecycleOpen     LifecycleState = "open"
	LifecycleDraining LifecycleState = "draining"
	LifecycleUnknown  LifecycleState = "unknown"
)

// DrainOutcome is supplied by the trusted physical resource owner, not a client.
type DrainOutcome uint8

const (
	DrainUnknown DrainOutcome = iota
	DrainFailed
	// DrainSucceeded asserts that physical work ended, all persistence results
	// are known, and connections/locks were successfully closed/released.
	// A timeout, canceled waiter, or logical completion is not this evidence.
	DrainSucceeded
)

// Lifecycle is the sole local lifecycle admission owner for one stable scope.
// Retain one pointer per workspace; do not copy it or replace it to clear a fence.
// It does not authorize principals, perform IO, or prove cross-process exclusion.
type Lifecycle struct {
	mu    sync.Mutex
	scope Scope
	state LifecycleState
}

// NewLifecycle is only for fresh/cold workspaces with no live resource owner.
// The installation coordinator must ensure that precondition and retain the
// singleton across registration, open, and drain; construction is not recovery.
func NewLifecycle(scope Scope) (*Lifecycle, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	return &Lifecycle{scope: scope, state: LifecycleClosed}, nil
}

func (l *Lifecycle) check(scope Scope) error {
	if err := scope.Validate(); err != nil || scope != l.scope {
		return ErrScope
	}
	return nil
}

// WithClosed serializes a binding change with opening for the entire action.
// The action must not call back into this Lifecycle. Cancellation is checked
// after acquiring the mutex; cancellation after admission does not undo action.
func (l *Lifecycle) WithClosed(ctx context.Context, scope Scope, action func() error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.check(scope); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if l.state != LifecycleClosed || action == nil {
		return ErrLifecycle
	}
	err := action()
	if errors.Is(err, ErrLifecycleUncertain) {
		l.state = LifecycleUnknown
	}
	return err
}

func (l *Lifecycle) BeginOpen(scope Scope) error {
	return l.transition(scope, LifecycleOpening, LifecycleClosed)
}

func (l *Lifecycle) MarkOpen(scope Scope) error {
	return l.transition(scope, LifecycleOpen, LifecycleOpening)
}

// BeginDrain also admits failed opens and explicit investigation of unknown
// resources. It does not itself establish physical completion or known results.
func (l *Lifecycle) BeginDrain(scope Scope) error {
	return l.transition(scope, LifecycleDraining, LifecycleOpening, LifecycleOpen, LifecycleUnknown)
}

// FinishDrain never infers success from an absent error or an elapsed context.
// Unrecognized outcomes fail closed to unknown. A fenced owner may be drained
// again only by an explicit BeginDrain followed by trusted physical evidence.
func (l *Lifecycle) FinishDrain(scope Scope, outcome DrainOutcome) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.check(scope); err != nil {
		return err
	}
	if l.state != LifecycleDraining {
		return ErrLifecycle
	}
	if outcome != DrainSucceeded {
		l.state = LifecycleUnknown
		return ErrLifecycle
	}
	l.state = LifecycleClosed
	return nil
}

func (l *Lifecycle) State(scope Scope) (LifecycleState, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.check(scope); err != nil {
		return LifecycleUnknown, err
	}
	if l.state == "" {
		return LifecycleUnknown, nil
	}
	return l.state, nil
}

func (l *Lifecycle) transition(scope Scope, next LifecycleState, allowed ...LifecycleState) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.check(scope); err != nil {
		return err
	}
	for _, state := range allowed {
		if l.state == state {
			l.state = next
			return nil
		}
	}
	return ErrLifecycle
}
