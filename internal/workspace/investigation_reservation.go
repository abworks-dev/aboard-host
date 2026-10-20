package workspace

import "context"

// ClaimOperationInvestigation looks only at retained resource metadata, never
// waits behind workspace IO. Current actor/operator authorization is mandatory
// here and again in the actual business method and response delivery.
func (o *OpenOwner) ClaimOperationInvestigation(ctx context.Context, scope Scope, principal, key string, operator bool, authorize func(context.Context) error) (*OperationReservation, error) {
	if err := CheckInvestigationReservation(ctx, ""); err != nil {
		return nil, err
	}
	if scope != o.scope {
		return nil, ErrScope
	}
	if authorize == nil || !recordID(principal) || !recordID(key) {
		return nil, ErrSaveInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := authorize(ctx); err != nil {
		return nil, err
	}
	o.closeMu.Lock()
	defer o.closeMu.Unlock()
	q := o.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	// An exact close lookup reads only closeMu-protected metadata and never
	// enters the runnable queue. Borrow its original bounded request claim even
	// while drain waits; unrelated/save controls must still yield to physical drain.
	if op := o.closes[key]; op != nil {
		if !operator && op.view.PrincipalID != principal {
			return nil, ErrCapacity
		}
		if !op.view.Closed && op.reservation != nil {
			return op.reservation.ClaimInvestigation(key)
		}
	}
	if q.drainWaiting {
		return nil, ErrCapacity
	}
	if ref := q.held["save:"+key]; ref != nil {
		if !operator && q.heldPrincipals["save:"+key] != principal {
			return nil, ErrCapacity
		}
		return ref.ClaimInvestigation(key)
	}
	// A cold unknown open (or unknown preview) has no published save admission
	// actor. Only an explicit current operator may investigate its retained view;
	// the queried key is still checked against original history by the adapter.
	if operator {
		for _, marker := range []string{"open", "preview"} {
			if ref := q.held[marker]; ref != nil {
				return ref.ClaimInvestigation(key)
			}
		}
	}
	return nil, ErrCapacity
}
