package workspace

import "context"

// Status reports retained state without implicitly opening a directory. Only a
// currently validated open view contributes formal facts or a published epoch.
type Status struct {
	Scope              Scope
	State              LifecycleState
	Epoch              string
	Snapshot           *Snapshot
	NeedsInvestigation bool
}

func (o *OpenOwner) Status(ctx context.Context, scope Scope, authorize func(context.Context) error) (Status, error) {
	ticket, err := o.beginOperation(ctx, scope, authorize, true, false)
	if err != nil {
		return Status{}, err
	}
	defer ticket.finish()
	o.mu.Lock()
	defer o.mu.Unlock()
	if scope != o.scope {
		return Status{}, ErrScope
	}
	if authorize == nil {
		return Status{}, ErrOpen
	}
	if err = authorize(ctx); err != nil {
		return Status{}, err
	}
	state, err := o.lifecycle.State(scope)
	if err != nil {
		return Status{}, err
	}
	result := Status{Scope: scope, State: state, Epoch: o.view.Epoch, NeedsInvestigation: o.view.NeedsInvestigation || state == LifecycleUnknown}
	if state == LifecycleOpen && !result.NeedsInvestigation && o.resources != nil {
		if err = o.resources.Validate(); err != nil {
			return Status{}, err
		}
		snapshot, e := o.resources.Snapshot(ctx, scope)
		if e != nil {
			return Status{}, e
		}
		result.Snapshot = &snapshot
		if err = o.resources.Validate(); err != nil {
			return Status{}, err
		}
	}
	if err = authorize(ctx); err != nil {
		return Status{}, err
	}
	if err = ctx.Err(); err != nil {
		return Status{}, err
	}
	return result, nil
}
