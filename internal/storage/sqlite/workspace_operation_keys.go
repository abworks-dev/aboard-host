package sqlite

import (
	"context"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// SaveOperationIDs reads kind-denial metadata from the already validated SAME
// connection. No actor/state filter may hide an existing scoped save key; it is
// never an authorization check, receipt lookup or a second writer/connection.
func (g *CommitGate) SaveOperationIDs(ctx context.Context, scope workspace.Scope) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.db == nil {
		return nil, ErrFenced
	}
	if scope.Validate() != nil || g.db.kind != WorkspaceDatabase || g.db.workspaceID != scope.WorkspaceID || g.db.tenantID != scope.TenantID {
		return nil, workspace.ErrScope
	}
	rows, err := g.db.h.Conn.QueryContext(ctx, `SELECT operation_id FROM save_operation WHERE workspace_id=? ORDER BY operation_id`, scope.WorkspaceID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		if !validUUID(id) {
			return nil, errors.Join(ErrSchema, rows.Close())
		}
		ids = append(ids, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	return ids, nil
}
