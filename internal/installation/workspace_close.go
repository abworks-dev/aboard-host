//go:build windows && amd64

package installation

import (
	"context"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func (o *WorkspaceOpener) CloseWorkspace(ctx context.Context, a *Authorization, operationID, epoch string) (workspace.CloseView, error) {
	owner, scope, check, err := o.readOwner(ctx, a, "workspace.close")
	if err != nil {
		return workspace.CloseView{}, err
	}
	return owner.CloseWorkspace(ctx, scope, a.principalID, operationID, epoch, check)
}
func (o *WorkspaceOpener) GetCloseOperation(ctx context.Context, a *Authorization, operationID string) (workspace.CloseView, error) {
	if a == nil || (a.action != "workspace.read" && a.action != "workspace.operator") {
		return workspace.CloseView{}, ErrUnauthorized
	}
	owner, scope, check, err := o.readOwner(ctx, a, a.action)
	if err != nil {
		return workspace.CloseView{}, err
	}
	operator := a.action == "workspace.operator"
	return owner.GetCloseOperation(ctx, scope, a.principalID, operationID, operator, check)
}
func (o *WorkspaceOpener) WaitClose(ctx context.Context, a *Authorization, operationID string) (workspace.CloseView, error) {
	owner, scope, check, err := o.readOwner(ctx, a, "workspace.close")
	if err != nil {
		return workspace.CloseView{}, err
	}
	return owner.WaitClose(ctx, scope, a.principalID, operationID, false, check)
}

// BeginShutdownWorkspaces is a trusted Host teardown path, not an HTTP command.
// It synchronously freezes the owner set and stops every scope before returning
// the actual physical-completion join. No caller cancellation releases ownership.
func (o *WorkspaceOpener) BeginShutdownWorkspaces() <-chan struct{} {
	s := o.auth.store
	s.mu.Lock()
	s.workspaceStopping = true
	owners := make([]*workspace.OpenOwner, 0, len(s.workspaceOpenOwners))
	for _, owner := range s.workspaceOpenOwners {
		owners = append(owners, owner)
	}
	s.mu.Unlock()
	waits := make([]<-chan struct{}, 0, len(owners))
	for _, owner := range owners {
		waits = append(waits, owner.BeginShutdown())
	}
	joined := make(chan struct{})
	go func() {
		for _, done := range waits {
			<-done
		}
		close(joined)
	}()
	return joined
}

// ShutdownWorkspaces only waits on the already-frozen owners. Cancellation ends
// this waiter, not queue cutoff, accepted work, or physical cleanup.
func (o *WorkspaceOpener) ShutdownWorkspaces(ctx context.Context) error {
	select {
	case <-o.BeginShutdownWorkspaces():
		return nil
	case <-ctx.Done():
		return errors.Join(ErrWorkspaceOpenPending, ctx.Err())
	}
}
