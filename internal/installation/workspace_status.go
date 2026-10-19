//go:build windows && amd64

package installation

import (
	"context"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func (o *WorkspaceOpener) WorkspaceStatus(ctx context.Context, a *Authorization) (workspace.Status, error) {
	owner, scope, check, err := o.readOwner(ctx, a, "workspace.read")
	if err != nil {
		return workspace.Status{}, err
	}
	return owner.Status(ctx, scope, check)
}
