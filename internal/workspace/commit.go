package workspace

import (
	"context"
	"errors"
	"strconv"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

var (
	ErrSaveInput        = errors.New("workspace save input is invalid")
	ErrSaveKeyReuse     = errors.New("operation key already binds different principal or input")
	ErrSaveViewDelivery = errors.New("save result delivery failed")
)

// Trusted persistence context, constructed by the owner, never decoded from HTTP.
// Epoch and PreviewVersion govern NEW admission only, not the SAVE/1 identity.
// CandidateDigest already binds its immutable previewVersion.
type SaveRequest struct {
	InstallationID, TenantID, WorkspaceID, PrincipalID string
	OperationID, CandidateID, CandidateDigest, Epoch   string
	PreviewVersion                                     int64
}
type Receipt struct {
	WorkspaceID, OperationID, InputDigest, PrincipalID, CandidateID, ResourceID string
	NewVersion, NewRevision                                                     int64
	ContentDigest                                                               string
}
type PersistenceStatus string

const (
	SaveConfirmed    PersistenceStatus = "confirmed"
	SaveAccepted     PersistenceStatus = "accepted"
	SaveUnknown      PersistenceStatus = "unknown"
	SaveRejected     PersistenceStatus = "rejected"
	SaveNotCommitted PersistenceStatus = "notCommitted"
)

type SaveResult struct {
	Status      PersistenceStatus
	Receipt     *Receipt
	FailureCode string
	// Delivery failure never changes a positive durable result into rollback.
	// A late authorization failure suppresses Receipt, even for confirmed state.
	ViewDeliveryFailed bool
}
type SaveInput struct {
	OperationID, Epoch, CandidateID, CandidateDigest string
	PreviewVersion                                   int64
}

func (in SaveInput) validate() error {
	_, err := (protocol.CommitRecord{OperationId: protocol.UUID(in.OperationID), OpenEpoch: protocol.UUID(in.Epoch), CandidateId: protocol.UUID(in.CandidateID), CandidateDigest: protocol.Digest(in.CandidateDigest), PreviewVersion: protocol.PositiveRevision(strconv.FormatInt(in.PreviewVersion, 10))}).MarshalJSON()
	if err != nil {
		return ErrSaveInput
	}
	return nil
}

// AcceptedSave is an internal single-use capability, minted only by confirmed
// NEW durable admission. Existing/unknown keys must never mint another attempt.
type AcceptedSave interface {
	Commit(context.Context) (SaveResult, error)
}
type SaveFacts interface {
	AdmitSave(context.Context, Scope, SaveRequest, bool) (SaveResult, AcceptedSave, error)
	// Keep the original physical resources; no timeout/authorization error can
	// launder a durable accepted/unknown operation into a safe physical close.
	RetainSaveUncertainty()
}

// CommitRecord is synchronous owner execution, not the HTTP waiting/queue API.
// T021 wraps this with bounded independent admission/waiters. Once admission is
// durable, finishing uses an owner context, never a canceled request's context.
func (o *OpenOwner) CommitRecord(ctx context.Context, scope Scope, principal string, authorize func(context.Context) error, in SaveInput) (SaveResult, error) {
	ticket, admissionErr := o.beginOperation(ctx, scope, authorize, false, false)
	if admissionErr != nil {
		return SaveResult{Status: SaveRejected}, admissionErr
	}
	defer ticket.finish()
	o.mu.Lock()
	defer o.mu.Unlock()
	if scope != o.scope {
		return SaveResult{Status: SaveRejected}, ErrScope
	}
	if authorize == nil {
		return SaveResult{Status: SaveRejected}, ErrSaveInput
	}
	if err := authorize(ctx); err != nil {
		return SaveResult{Status: SaveRejected}, err
	}
	if err := ctx.Err(); err != nil {
		return SaveResult{Status: SaveRejected}, err
	}
	if err := in.validate(); err != nil {
		return SaveResult{Status: SaveRejected}, err
	}
	if !recordID(principal) {
		return SaveResult{Status: SaveRejected}, ErrSaveInput
	}
	state, err := o.lifecycle.State(scope)
	if err != nil {
		return SaveResult{Status: SaveRejected}, err
	}
	if (state != LifecycleOpen && state != LifecycleUnknown) || o.resources == nil {
		return SaveResult{Status: SaveRejected}, ErrLifecycle
	}
	store, ok := o.resources.(SaveFacts)
	if !ok {
		return SaveResult{Status: SaveRejected}, ErrSaveInput
	}
	investigationOnly := state != LifecycleOpen || o.view.NeedsInvestigation || o.view.Epoch == ""
	insertedKey, err := o.reserveSaveKey(in.OperationID, investigationOnly)
	if err != nil {
		return SaveResult{Status: SaveRejected}, err
	}
	keepKey := false
	defer func() {
		if insertedKey && !keepKey {
			o.forgetUnadmittedSaveKey(in.OperationID)
		}
	}()
	uncertain := func(cause error) (SaveResult, error) {
		keepKey = true
		// A fenced same-key inquiry cannot start persistence IO. Its unknown
		// result must not allocate another unfinished physical-operation slot.
		if !investigationOnly {
			ticket.retainFor("save:"+in.OperationID, principal)
		}
		store.RetainSaveUncertainty()
		o.view.NeedsInvestigation = true
		return SaveResult{Status: SaveUnknown}, errors.Join(ErrOpenUncertain, cause, o.lifecycle.transition(scope, LifecycleUnknown, LifecycleOpen, LifecycleUnknown))
	}
	if err = o.resources.Validate(); err != nil {
		return uncertain(err)
	}
	if err = authorize(ctx); err != nil {
		return SaveResult{Status: SaveRejected}, err
	}
	r := SaveRequest{InstallationID: scope.InstallationID, TenantID: scope.TenantID, WorkspaceID: scope.WorkspaceID, PrincipalID: principal, OperationID: in.OperationID, CandidateID: in.CandidateID, CandidateDigest: in.CandidateDigest, Epoch: in.Epoch, PreviewVersion: in.PreviewVersion}
	// The port looks up this scoped key BEFORE new-write epoch/expiry/base checks,
	// including in a fenced view. An old epoch can retrieve, not acquire write rights.
	if err = o.queue.beforeMutation(); err != nil {
		return SaveResult{Status: SaveRejected}, err
	}
	result, admitted, saveErr := store.AdmitSave(ctx, scope, r, investigationOnly)
	// Classify the admission result, NOT a later commit/delivery error. Only
	// this call's NEW reservation can be forgotten after proven non-admission.
	keepKey = admitted != nil || result.Status != SaveRejected || result.FailureCode != "" || errors.Is(saveErr, ErrSaveKeyReuse)
	if result.Status == SaveUnknown {
		return uncertain(saveErr)
	}
	finishCtx := ctx
	if admitted != nil {
		if saveErr != nil || result.Status != SaveAccepted {
			return uncertain(errors.Join(ErrSaveInput, saveErr))
		}
		finishCtx = context.WithoutCancel(ctx)
		if err = o.resources.Validate(); err != nil {
			return uncertain(err)
		}
		if err = authorize(finishCtx); err != nil {
			return uncertain(err)
		}
		result, saveErr = admitted.Commit(finishCtx)
	} else if result.Status == SaveAccepted {
		return uncertain(saveErr)
	}
	if result.Status != SaveConfirmed && result.Status != SaveRejected && result.Status != SaveNotCommitted {
		return uncertain(saveErr)
	}
	if result.Status == SaveConfirmed && result.Receipt == nil {
		return uncertain(ErrSaveInput)
	}
	// Check delivery separately after IO. Keep durable truth even when the caller
	// can no longer receive it; never leak a receipt after authorization revocation.
	physicalErr := o.resources.Validate()
	if physicalErr != nil {
		_, _ = uncertain(physicalErr)
	}
	authErr := authorize(finishCtx)
	if authErr != nil {
		result.Receipt = nil
	}
	deliveryErr := errors.Join(physicalErr, authErr, ctx.Err())
	if deliveryErr != nil {
		result.ViewDeliveryFailed = true
		return result, errors.Join(saveErr, ErrSaveViewDelivery, deliveryErr)
	}
	return result, saveErr
}
