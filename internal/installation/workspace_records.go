//go:build windows && amd64

package installation

import (
	"context"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// readOwner only selects the retained workspace-local owner after current
// authorization. It does not open/recover a directory or hold Store.mu over IO.
func (o *WorkspaceOpener) readOwner(ctx context.Context, a *Authorization, action string) (*workspace.OpenOwner, workspace.Scope, func(context.Context) error, error) {
	if o == nil || a == nil || a.auth != o.auth {
		return nil, workspace.Scope{}, nil, ErrUnauthorized
	}
	scope, err := a.Scope()
	if err != nil {
		return nil, scope, nil, err
	}
	check := func(ctx context.Context) error { return a.CheckAction(ctx, scope, action) }
	if err = check(ctx); err != nil {
		return nil, scope, nil, err
	}
	owner, _, err := o.owner(ctx, a)
	return owner, scope, check, err
}
func (o *WorkspaceOpener) ReadSnapshot(ctx context.Context, a *Authorization) (workspace.Snapshot, error) {
	owner, scope, check, err := o.readOwner(ctx, a, "workspace.read")
	if err != nil {
		return workspace.Snapshot{}, err
	}
	return owner.ReadSnapshot(ctx, scope, check)
}
func (o *WorkspaceOpener) ListRecords(ctx context.Context, a *Authorization, cursor string, limit int) (workspace.RecordPage, error) {
	owner, scope, check, err := o.readOwner(ctx, a, "workspace.read")
	if err != nil {
		return workspace.RecordPage{}, err
	}
	return owner.ListRecords(ctx, scope, check, cursor, limit)
}
func (o *WorkspaceOpener) ReadRecord(ctx context.Context, a *Authorization, id string) (workspace.TextRecord, error) {
	owner, scope, check, err := o.readOwner(ctx, a, "workspace.read")
	if err != nil {
		return workspace.TextRecord{}, err
	}
	return owner.ReadRecord(ctx, scope, check, id)
}
