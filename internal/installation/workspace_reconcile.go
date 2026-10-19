//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"path/filepath"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func (o *WorkspaceOpener) GetWorkspaceOperation(ctx context.Context, a *Authorization, operationID string) (workspace.OperationEvidence, error) {
	if a == nil || (a.action != "workspace.read" && a.action != "workspace.operator") {
		return workspace.OperationEvidence{}, ErrUnauthorized
	}
	owner, scope, check, err := o.readOwner(ctx, a, a.action)
	if err != nil {
		return workspace.OperationEvidence{}, err
	}
	return owner.GetSaveOperation(ctx, scope, a.principalID, operationID, a.action == "workspace.operator", check)
}
func (o *WorkspaceOpener) ReconcileWorkspaceOperation(ctx context.Context, a *Authorization, operationID string) (workspace.OperationEvidence, error) {
	owner, scope, check, err := o.readOwner(ctx, a, "workspace.operator")
	if err != nil {
		return workspace.OperationEvidence{}, err
	}
	return owner.ReconcileSaveOperation(ctx, scope, a.principalID, operationID, check)
}
func (r *openResources) QuerySaveOperation(ctx context.Context, scope workspace.Scope, operationID, principal string, operator bool) (workspace.OperationEvidence, error) {
	if scope != r.scope {
		return workspace.OperationEvidence{}, workspace.ErrScope
	}
	evidence := workspace.OperationEvidence{Result: workspace.SaveResult{Status: workspace.SaveUnknown}, RequiresReopen: r.investigationView, PhysicalEnded: r.investigationView}
	if r.quarantined {
		return evidence, workspace.ErrOpenUncertain
	}
	var err error
	if r.gate != nil {
		evidence.Result, err = r.gate.QuerySaveOperation(ctx, scope, operationID, principal, operator)
	} else if r.investigationView && r.database != nil {
		evidence.Result, err = r.database.QuerySaveOperation(ctx, scope, operationID, principal, operator)
	} else {
		return evidence, workspace.ErrOpenUncertain
	}
	if evidence.Result.Status == workspace.SaveConfirmed || evidence.Result.Status == workspace.SaveRejected || evidence.Result.Status == workspace.SaveNotCommitted {
		evidence.PhysicalEnded = true
	}
	if evidence.Result.Status == workspace.SaveUnknown && !errors.Is(err, workspace.ErrOperationHistoryUnavailable) && !errors.Is(err, workspace.ErrOperationAccess) {
		r.pending = true
	}
	return evidence, err
}
func (r *openResources) ReconcileSaveOperation(ctx context.Context, scope workspace.Scope, operationID, principal string, authorize func(context.Context) error) (workspace.OperationEvidence, error) {
	evidence, queryErr := r.QuerySaveOperation(ctx, scope, operationID, principal, true)
	if !r.pending && queryErr == nil && evidence.Result.Status != workspace.SaveUnknown {
		return evidence, nil
	}
	// A random absent key is not an excuse to close a healthy handle. Once this
	// owner is already uncertain, an absent admission still cannot prove rollback.
	if !r.pending && errors.Is(queryErr, workspace.ErrOperationHistoryUnavailable) {
		return evidence, queryErr
	}
	r.pending = true
	evidence = workspace.OperationEvidence{Result: workspace.SaveResult{Status: workspace.SaveUnknown}, RequiresReopen: true}
	proof := func() error {
		if err := authorize(ctx); err != nil {
			return err
		}
		return r.Validate()
	}
	if err := proof(); err != nil {
		return evidence, err
	}
	// Pins and the same writer lease remain alive across physical connection end,
	// conservative journal preflight and the fresh checked view. No new lease/path
	// UUID can substitute for this unchanged physical-history ownership chain.
	if r.gate != nil {
		if err := r.gate.EndPhysical(); err != nil {
			return evidence, err
		}
		r.gate = nil
		r.database = nil
		r.facts = nil
	} else if r.database != nil {
		if err := r.database.Close(); err != nil {
			return evidence, err
		}
		r.database = nil
	}
	r.investigationView = false
	evidence.PhysicalEnded = true
	raw, err := platform.ReadPinnedWorkspaceFile(r.descriptorPin, 4096)
	if err != nil {
		return evidence, err
	}
	descriptor, err := workspace.ParseDirectoryDescriptor(raw)
	if err != nil {
		return evidence, err
	}
	if descriptor.WorkspaceID != scope.WorkspaceID || descriptor.TenantID != scope.TenantID {
		return evidence, workspace.ErrOpen
	}
	// A prior failed preflight can retain a frozen source, scratch and its DB.
	// Close those exact objects before assigning any replacement; close-unknown
	// stops this attempt while all original namespace/exclusion pins remain held.
	if err = r.closeRecoveryScratch(); err != nil {
		return evidence, err
	}
	if r.recoveryInputs != nil {
		if err = r.recoveryInputs.Close(); err != nil {
			return evidence, err
		}
		r.recoveryInputs = nil
	}
	recovering, err := r.preflightRecovery(ctx, descriptor, authorize)
	if err != nil {
		return evidence, err
	}
	if err = proof(); err != nil {
		return evidence, err
	}
	r.database, err = sqlite.OpenDatabase(ctx, filepath.Join(r.lease.RootPath(), "data", "workspace.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
	if err != nil {
		r.quarantined = recovering || errors.Is(err, sqlite.ErrPhysicalCloseUnknown)
		return evidence, err
	}
	if _, err = r.database.ValidateOpenHistory(ctx, descriptor); err != nil {
		r.quarantined = recovering
		return evidence, err
	}
	r.investigationView = true
	result, pending, err := r.database.ReconcileSaveOperation(ctx, scope, operationID, principal, proof)
	if err != nil {
		return evidence, err
	}
	// Validate the resulting FULL same history, not just the requested receipt.
	actualPending, err := r.database.ValidateOpenHistory(ctx, descriptor)
	if err != nil {
		return evidence, err
	}
	if err = proof(); err != nil {
		return evidence, err
	}
	r.pending = pending || actualPending
	evidence.Result = result
	return evidence, nil
}

var _ workspace.OperationFacts = (*openResources)(nil)
