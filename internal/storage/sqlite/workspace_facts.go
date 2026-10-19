package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// WorkspaceFacts is a typed adapter over the SAME CommitGate/connection. It
// cannot open another database or create another writer. The composition owner
// retains the OS lease and performs current authorization/lifecycle admission.
type WorkspaceFacts struct {
	gate  *CommitGate
	scope workspace.Scope
}

var _ workspace.TextFacts = (*WorkspaceFacts)(nil)

func NewWorkspaceFacts(g *CommitGate, scope workspace.Scope) (*WorkspaceFacts, error) {
	if g == nil || scope.Validate() != nil {
		return nil, workspace.ErrScope
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.db == nil || g.db.kind != WorkspaceDatabase || g.db.workspaceID != scope.WorkspaceID || g.db.tenantID != scope.TenantID {
		return nil, workspace.ErrScope
	}
	if g.closed {
		return nil, ErrFenced
	}
	return &WorkspaceFacts{gate: g, scope: scope}, nil
}

func (f *WorkspaceFacts) check(scope workspace.Scope) error {
	if f.scope != scope {
		return workspace.ErrScope
	}
	// Unknown transaction/physical state is not a trustworthy ordinary read
	// view. Investigation uses the gate's separate, explicit read protocol.
	if f.gate.closed || f.gate.fenced {
		return ErrFenced
	}
	return nil
}
func (f *WorkspaceFacts) Snapshot(ctx context.Context, scope workspace.Scope) (workspace.Snapshot, error) {
	f.gate.mu.Lock()
	defer f.gate.mu.Unlock()
	if err := f.check(scope); err != nil {
		return workspace.Snapshot{}, err
	}
	// Identity and revision come from a single trusted database snapshot.
	var out workspace.Snapshot
	err := f.gate.db.h.Conn.QueryRowContext(ctx, `SELECT i.workspace_id,i.tenant_id,i.display_name,i.identity_digest,i.format_version,i.schema_version,i.business_state,h.revision FROM workspace_identity i JOIN workspace_head h ON h.workspace_id=i.workspace_id WHERE i.singleton=1 AND i.workspace_id=? AND i.tenant_id=?`, scope.WorkspaceID, scope.TenantID).Scan(&out.Identity.WorkspaceID, &out.Identity.TenantID, &out.Identity.DisplayName, &out.Identity.IdentityDigest, &out.Identity.FormatVersion, &out.Identity.SchemaVersion, &out.Identity.BusinessState, &out.Revision)
	if err != nil {
		return workspace.Snapshot{}, err
	}
	return out, nil
}

const currentRecordSQL = `SELECT r.workspace_id,r.tenant_id,r.resource_id,v.version,v.workspace_revision,v.title,v.body,v.content_digest,v.created_by,v.operation_id FROM text_record r JOIN text_version v ON v.workspace_id=r.workspace_id AND v.resource_id=r.resource_id AND v.version=r.current_version WHERE r.workspace_id=? AND r.tenant_id=?`

type rowScanner interface{ Scan(...any) error }

func scanTextRecord(row rowScanner) (workspace.TextRecord, error) {
	var out workspace.TextRecord
	err := row.Scan(&out.WorkspaceID, &out.TenantID, &out.ResourceID, &out.Version, &out.Revision, &out.Title, &out.Body, &out.ContentDigest, &out.CreatedBy, &out.OperationID)
	if errors.Is(err, sql.ErrNoRows) {
		return workspace.TextRecord{}, workspace.ErrNotFound
	}
	return out, err
}
func (f *WorkspaceFacts) Current(ctx context.Context, scope workspace.Scope, resourceID string) (workspace.TextRecord, error) {
	f.gate.mu.Lock()
	defer f.gate.mu.Unlock()
	if err := f.check(scope); err != nil {
		return workspace.TextRecord{}, err
	}
	if !validUUID(resourceID) {
		return workspace.TextRecord{}, ErrConflict
	}
	return scanTextRecord(f.gate.db.h.Conn.QueryRowContext(ctx, currentRecordSQL+` AND r.resource_id=?`, scope.WorkspaceID, scope.TenantID, resourceID))
}
func (f *WorkspaceFacts) List(ctx context.Context, scope workspace.Scope, afterResourceID string, limit int) ([]workspace.TextRecord, error) {
	f.gate.mu.Lock()
	defer f.gate.mu.Unlock()
	if err := f.check(scope); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 || (afterResourceID != "" && !validUUID(afterResourceID)) {
		return nil, ErrConflict
	}
	rows, err := f.gate.db.h.Conn.QueryContext(ctx, currentRecordSQL+` AND r.resource_id>? ORDER BY r.resource_id LIMIT ?`, scope.WorkspaceID, scope.TenantID, afterResourceID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]workspace.TextRecord, 0)
	for rows.Next() {
		record, err := scanTextRecord(rows)
		if err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		out = append(out, record)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	return out, nil
}
