package workspace

import (
	"context"
	"errors"
)

var (
	ErrOperationHistoryUnavailable = errors.New("original operation history is unavailable; absence is not rollback evidence")
	ErrOperationAccess             = errors.New("operation is not accessible to this principal")
)

// OperationEvidence reports one operation, not permission to reuse its key or a
// claim that the directory is safe to move. RequiresReopen revokes ordinary reads
// and writes after an explicit fresh investigation; physical Close is still due.
type OperationEvidence struct {
	Result         SaveResult
	PhysicalEnded  bool
	RequiresReopen bool
}
type OperationFacts interface {
	QuerySaveOperation(context.Context, Scope, string, string, bool) (OperationEvidence, error)
	ReconcileSaveOperation(context.Context, Scope, string, string, func(context.Context) error) (OperationEvidence, error)
	RetainSaveUncertainty()
}

func (o *OpenOwner) GetSaveOperation(ctx context.Context, scope Scope, principal, operationID string, operator bool, authorize func(context.Context) error) (OperationEvidence, error) {
	return o.investigateSave(ctx, scope, principal, operationID, operator, false, authorize)
}
func (o *OpenOwner) ReconcileSaveOperation(ctx context.Context, scope Scope, principal, operationID string, authorize func(context.Context) error) (OperationEvidence, error) {
	return o.investigateSave(ctx, scope, principal, operationID, true, true, authorize)
}
func (o *OpenOwner) investigateSave(ctx context.Context, scope Scope, principal, operationID string, operator, reconcile bool, authorize func(context.Context) error) (OperationEvidence, error) {
	if err := o.checkInvestigation(ctx, operationID); err != nil {
		return OperationEvidence{}, err
	}
	ticket, admissionErr := o.beginOperation(ctx, scope, authorize, true, false)
	if admissionErr != nil {
		return OperationEvidence{}, admissionErr
	}
	defer ticket.finish()
	o.mu.Lock()
	defer o.mu.Unlock()
	if scope != o.scope {
		return OperationEvidence{}, ErrScope
	}
	if authorize == nil || !recordID(principal) || !recordID(operationID) {
		return OperationEvidence{}, ErrSaveInput
	}
	if err := authorize(ctx); err != nil {
		return OperationEvidence{}, err
	}
	if err := ctx.Err(); err != nil {
		return OperationEvidence{}, err
	}
	state, err := o.lifecycle.State(scope)
	if err != nil {
		return OperationEvidence{}, err
	}
	if (state != LifecycleOpen && state != LifecycleUnknown) || o.resources == nil {
		return OperationEvidence{}, ErrLifecycle
	}
	facts, ok := o.resources.(OperationFacts)
	if !ok {
		return OperationEvidence{}, ErrOperationHistoryUnavailable
	}
	fence := func() {
		facts.RetainSaveUncertainty()
		o.view.NeedsInvestigation = true
		_ = o.lifecycle.transition(scope, LifecycleUnknown, LifecycleOpen, LifecycleUnknown)
	}
	if err := o.resources.Validate(); err != nil {
		fence()
		return OperationEvidence{}, errors.Join(ErrOpenUncertain, err)
	}
	var evidence OperationEvidence
	if reconcile {
		// An explicit owner investigation may physically end the original connection
		// and durably record proven non-commit. A disconnected waiter cannot undo it.
		evidence, err = facts.ReconcileSaveOperation(context.WithoutCancel(ctx), scope, operationID, principal, authorize)
	} else {
		evidence, err = facts.QuerySaveOperation(context.WithoutCancel(ctx), scope, operationID, principal, operator)
	}
	// An arbitrary missing key must not fence an otherwise healthy workspace.
	// A known accepted/unknown record, an unavailable physical view or an explicit
	// fresh-view transition must retain exclusion instead.
	if evidence.Result.Status == SaveUnknown && !errors.Is(err, ErrOperationHistoryUnavailable) && !errors.Is(err, ErrOperationAccess) {
		fence()
	} else if evidence.RequiresReopen {
		o.view.NeedsInvestigation = true
		_ = o.lifecycle.transition(scope, LifecycleUnknown, LifecycleOpen, LifecycleUnknown)
	}
	if evidence.PhysicalEnded && (evidence.Result.Status == SaveConfirmed || evidence.Result.Status == SaveRejected || evidence.Result.Status == SaveNotCommitted) {
		o.queue.resolve("save:" + operationID)
	}
	if reconcile && err == nil && o.view.Epoch == "" && evidence.RequiresReopen && evidence.PhysicalEnded &&
		(evidence.Result.Status == SaveConfirmed || evidence.Result.Status == SaveRejected || evidence.Result.Status == SaveNotCommitted) {
		// A cold unknown open published no epoch, so public close cannot be
		// fabricated to dispose of the fresh investigation connection. Finish
		// only this operator-requested, proven original history. The adapter's
		// Close still refuses any other pending save/quarantine and all physical
		// cleanup must actually finish before lifecycle/capacity can be released.
		if end := authorize(context.WithoutCancel(ctx)); end != nil {
			err = end
		} else if end = o.drainOwnedResources(scope); end != nil {
			fence()
			err = end
		}
	}
	var physicalErr error
	if o.resources != nil {
		physicalErr = o.resources.Validate()
	}
	if physicalErr != nil {
		fence()
	}
	authErr := authorize(context.WithoutCancel(ctx))
	if authErr != nil {
		evidence.Result.Receipt = nil
	}
	deliveryErr := errors.Join(physicalErr, authErr, ctx.Err())
	if deliveryErr != nil {
		evidence.Result.ViewDeliveryFailed = true
		return evidence, errors.Join(err, ErrSaveViewDelivery, deliveryErr)
	}
	return evidence, err
}
