//go:build windows && amd64

package installation

import (
	"context"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func (o *WorkspaceOpener) PreviewRecord(ctx context.Context, a *Authorization, in workspace.PreviewInput) (workspace.Preview, error) {
	owner, scope, check, err := o.readOwner(ctx, a, "workspace.write")
	if err != nil {
		return workspace.Preview{}, err
	}
	return owner.PreviewRecord(ctx, scope, a.Principal().PrincipalID, check, in)
}

func (r *openResources) PutPreview(ctx context.Context, scope workspace.Scope, c workspace.Candidate) error {
	if r.pending || r.quarantined || r.facts == nil {
		return workspace.ErrOpenUncertain
	}
	err := r.facts.PutPreview(ctx, scope, c)
	if errors.Is(err, sqlite.ErrUnknown) || errors.Is(err, sqlite.ErrFenced) || errors.Is(err, workspace.ErrOpenUncertain) {
		// A candidate is never a formal save; nevertheless uncertain physical
		// completion cannot release the lease or admit further writes/close.
		r.pending = true
		return errors.Join(workspace.ErrOpenUncertain, err)
	}
	return err
}
