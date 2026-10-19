package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// QuerySaveOperation uses the existing retained gate; it cannot open a directory
// or recover storage. Original-actor and operator inquiries are distinct trusted
// permissions, neither is a client-supplied principal override.
func (g *CommitGate) QuerySaveOperation(ctx context.Context, scope workspace.Scope, operationID, principal string, operator bool) (SaveResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return SaveResult{Status: Unknown}, ErrFenced
	}
	result, err := g.db.QuerySaveOperation(ctx, scope, operationID, principal, operator)
	if result.Status == Unknown && !errors.Is(err, workspace.ErrOperationHistoryUnavailable) && !errors.Is(err, workspace.ErrOperationAccess) {
		g.fenced = true
	}
	return result, err
}
func (d *Database) checkOperationScope(scope workspace.Scope) error {
	if d == nil || d.kind != WorkspaceDatabase || d.h == nil || d.h.Conn == nil || scope.Validate() != nil || scope.WorkspaceID != d.workspaceID || scope.TenantID != d.tenantID {
		return workspace.ErrScope
	}
	return nil
}
func readSaveOperation(ctx context.Context, q schemaReader, scope workspace.Scope, operationID, principal string, operator bool) (SaveResult, error) {
	r := SaveRequest{InstallationID: scope.InstallationID, TenantID: scope.TenantID, WorkspaceID: scope.WorkspaceID, OperationID: operationID}
	var stored string
	err := q.QueryRowContext(ctx, `SELECT o.principal_id,o.candidate_id,o.input_digest,o.admission_epoch,c.candidate_digest,c.preview_version FROM save_operation o JOIN candidate c ON c.workspace_id=o.workspace_id AND c.candidate_id=o.candidate_id WHERE o.workspace_id=? AND o.operation_id=?`, scope.WorkspaceID, operationID).Scan(&r.PrincipalID, &r.CandidateID, &stored, &r.Epoch, &r.CandidateDigest, &r.PreviewVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return SaveResult{Status: Unknown}, workspace.ErrOperationHistoryUnavailable
	}
	if err != nil {
		return SaveResult{Status: Unknown}, err
	}
	if !operator && r.PrincipalID != principal {
		return SaveResult{}, workspace.ErrOperationAccess
	}
	// Recompute SAVE/1 with the current installation scope and original immutable
	// candidate digest, not merely trust relationally equal stored digest strings.
	input, err := digest.Save(digest.SaveInput{InstallationID: protocol.UUID(scope.InstallationID), TenantID: protocol.UUID(scope.TenantID), WorkspaceID: protocol.UUID(scope.WorkspaceID), PrincipalID: protocol.UUID(r.PrincipalID), OperationID: protocol.UUID(operationID), CandidateID: protocol.UUID(r.CandidateID), CandidateDigest: protocol.Digest(r.CandidateDigest)})
	if err != nil || string(input) != stored {
		return SaveResult{Status: Unknown}, errors.Join(ErrSchema, err)
	}
	result, _, err := lookup(ctx, q, r, string(input))
	return result, err
}
func (d *Database) QuerySaveOperation(ctx context.Context, scope workspace.Scope, operationID, principal string, operator bool) (SaveResult, error) {
	if err := d.checkOperationScope(scope); err != nil {
		return SaveResult{}, err
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SaveResult{Status: Unknown}, errors.Join(ErrUnknown, err)
	}
	result, err := readSaveOperation(ctx, tx, scope, operationID, principal, operator)
	// Positive results must come from a structurally consistent formal history.
	if err == nil {
		err = verifySchema(ctx, tx, WorkspaceDatabase)
	}
	end := tx.Rollback()
	if end != nil || (err != nil && !errors.Is(err, workspace.ErrOperationHistoryUnavailable) && !errors.Is(err, workspace.ErrOperationAccess)) {
		return SaveResult{Status: Unknown}, errors.Join(ErrUnknown, err, end)
	}
	return result, err
}

// ReconcileSaveOperation is only for a FRESH, fully validated existing database
// whose caller retains exclusion, unchanged physical identity/same history, and
// proof that every original adapter call has physically ended. It never executes
// a candidate. A positive original admission plus absent formal receipt in this
// view may become a durable terminal rejection, so later explicit reopen can
// accept NEW work without forgetting or replaying this key. Missing admission is
// NEVER terminalized. The callback is mandatory, internal owner evidence.
func (d *Database) ReconcileSaveOperation(ctx context.Context, scope workspace.Scope, operationID, principal string, verifyExclusiveHistory func() error) (SaveResult, bool, error) {
	unknown := SaveResult{Status: Unknown}
	if err := d.checkOperationScope(scope); err != nil {
		return unknown, true, err
	}
	if verifyExclusiveHistory == nil {
		return unknown, true, ErrFenced
	}
	if err := verifyExclusiveHistory(); err != nil {
		return unknown, true, err
	}
	tx, err := d.h.Conn.BeginTx(ctx, nil)
	if err != nil {
		return unknown, true, errors.Join(ErrUnknown, err)
	}
	abort := func(cause error) (SaveResult, bool, error) { return unknown, true, errors.Join(cause, tx.Rollback()) }
	if err = verifySchema(ctx, tx, WorkspaceDatabase); err != nil {
		return abort(err)
	}
	result, err := readSaveOperation(ctx, tx, scope, operationID, principal, true)
	if err != nil {
		return abort(err)
	}
	changed := false
	if result.Status == Unknown {
		// This exact old admission has positive evidence, unlike a missing key. No
		// receipt exists and the original transaction cannot still complete later.
		if err = verifyExclusiveHistory(); err != nil {
			return abort(err)
		}
		update, e := tx.ExecContext(ctx, `UPDATE save_operation SET state='rejected',failure_code='NOT_COMMITTED' WHERE workspace_id=? AND operation_id=? AND state IN ('accepted','unknown') AND NOT EXISTS(SELECT 1 FROM commit_receipt r WHERE r.workspace_id=save_operation.workspace_id AND r.operation_id=save_operation.operation_id)`, scope.WorkspaceID, operationID)
		if e != nil {
			return abort(e)
		}
		count, e := update.RowsAffected()
		if e != nil || count != 1 {
			return abort(errors.Join(ErrSchema, e))
		}
		changed = true
		result = SaveResult{Status: NotCommitted, FailureCode: "NOT_COMMITTED"}
	}
	if changed {
		if err = tx.Commit(); err != nil {
			return unknown, true, errors.Join(ErrUnknown, err)
		}
	} else if err = tx.Rollback(); err != nil {
		return unknown, true, errors.Join(ErrUnknown, err)
	}
	if err = verifyExclusiveHistory(); err != nil {
		return unknown, true, err
	}
	var pending int
	if err = d.h.Conn.QueryRowContext(ctx, `SELECT count(*) FROM save_operation WHERE state IN ('accepted','unknown')`).Scan(&pending); err != nil {
		return unknown, true, errors.Join(ErrUnknown, err)
	}
	return result, pending != 0, nil
}
