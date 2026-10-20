package workspace

import "context"

// ClaimRequest owns the ONE unfinished request/delivery claim on an allocation.
// Resource references and execution turns are independent of this claim. A
// resource Retain is not permission to create unbounded duplicate HTTP requests.
func (r *OperationReservation) ClaimRequest() (*OperationReservation, error) {
	return r.claimRequest("", 0)
}

// ClaimInvestigation is only for an authenticated original-resource lookup.
// The returned reference is restricted to control investigation of exactly key;
// neither this method nor the reference establishes actor/operator permission.
func (r *OperationReservation) ClaimInvestigation(key string) (*OperationReservation, error) {
	if !recordID(key) {
		return nil, ErrSaveInput
	}
	return r.claimRequest(key, investigationWorkspace)
}

func (r *OperationReservation) ClaimInstallationInvestigation(key string) (*OperationReservation, error) {
	if !recordID(key) {
		return nil, ErrSaveInput
	}
	return r.claimRequest(key, investigationInstallation)
}

const (
	investigationWorkspace    uint8 = 1
	investigationInstallation uint8 = 2
)

func (r *OperationReservation) claimRequest(key string, kind uint8) (*OperationReservation, error) {
	if r == nil || r.state == nil || r.state.allocation == nil {
		return nil, ErrCapacity
	}
	c := r.state.allocation.capacity
	c.mu.Lock()
	defer c.mu.Unlock()
	a := r.state.allocation
	if r.state.released || a.refs == 0 {
		return nil, ErrLifecycle
	}
	if a.requestClaim || r.state.investigationKey != "" {
		return nil, ErrCapacity
	}
	if kind == investigationWorkspace && a.queue == nil {
		return nil, ErrScope
	}
	// A slow body/delivery must never pin the shared runnable-control reserve.
	if err := c.ordinaryBackingLocked(a); err != nil {
		return nil, err
	}
	a.requestClaim = true
	a.refs++
	return &OperationReservation{state: &reservationReference{allocation: a, requestClaim: true, investigationKey: key, investigationKind: kind}}, nil
}

// CheckInvestigationReservation is a restriction, NOT an authorization check.
// Unrestricted internal contexts remain valid; borrowed contexts cannot change
// operation keys, create new work, or cross installation/workspace owners.
func CheckInvestigationReservation(ctx context.Context, key string) error {
	return checkInvestigationOwner(ctx, key, nil, nil, 0)
}
func (o *OpenOwner) checkInvestigation(ctx context.Context, key string) error {
	return checkInvestigationOwner(ctx, key, o.queue, o.queue.capacity, investigationWorkspace)
}
func (c *OperationCapacity) CheckInstallationInvestigation(ctx context.Context, key string) error {
	return checkInvestigationOwner(ctx, key, nil, c, investigationInstallation)
}
func checkInvestigationOwner(ctx context.Context, key string, queue *operationQueue, capacity *OperationCapacity, kind uint8) error {
	value, ok := ctx.Value(reservationContextKey{}).(reservationContext)
	if !ok {
		return nil
	}
	r := value.reference
	if r == nil || r.state == nil || r.state.allocation == nil {
		return ErrScope
	}
	c := r.state.allocation.capacity
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.state.released {
		return ErrLifecycle
	}
	if capacity != nil && c != capacity {
		return ErrScope
	}
	if r.state.investigationKey != "" && kind != 0 && (r.state.investigationKind != kind || (kind == investigationWorkspace && r.state.allocation.queue != queue)) {
		return ErrScope
	}
	if r.state.investigationKey != "" && (key == "" || r.state.investigationKey != key) {
		return ErrScope
	}
	return nil
}
