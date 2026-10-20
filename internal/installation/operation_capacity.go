//go:build windows && amd64

package installation

import (
	"context"
	"database/sql"
	"errors"
	"github.com/abworks-dev/aboard-host/internal/protocol"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// capacityLocked returns the ONE quota owner even before any workspace opens.
// Store.mu is held; neither acquisition nor reference release performs IO.
func (s *Store) capacityLocked() *workspace.OperationCapacity {
	if s.workspaceCapacity == nil {
		s.workspaceCapacity = workspace.NewOperationCapacity()
	}
	return s.workspaceCapacity
}

// ReserveOperation is current-authenticated installation request accounting,
// not permission to execute a command or access a workspace. The transport owns
// its reference through actual response delivery; a resource owner may retain it.
func (s *Store) ReserveOperation(ctx context.Context, a *Authorization) (*workspace.OperationReservation, error) {
	if a == nil || a.auth == nil || a.auth.store != s {
		return nil, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := a.checkLocked(ctx); err != nil {
		return nil, err
	}
	return s.capacityLocked().RetainOperation(ctx)
}

// ClaimInstallationInvestigation borrows only the authenticated original
// resource's ordinary allocation, with one request/delivery claim. No original
// actor fact or native owner is inferred from a client operation key.
func (s *Store) ClaimInstallationInvestigation(ctx context.Context, a *Authorization, key string) (*workspace.OperationReservation, error) {
	if err := workspace.CheckInvestigationReservation(ctx, ""); err != nil {
		return nil, err
	}
	if a == nil || a.auth == nil || a.auth.store != s || a.action != "installation.operator" || a.workspaceID != "" {
		return nil, ErrUnauthorized
	}
	if _, err := protocol.UUID(key).MarshalJSON(); err != nil {
		return nil, workspace.ErrSaveInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := a.checkLocked(ctx); err != nil {
		return nil, err
	}
	w := s.workspaceCreations[key]
	if w == nil || w.reservation == nil {
		return nil, workspace.ErrCapacity
	}
	original, err := s.db.ReadCurrentInstallationOperation(ctx, a.hash, a.version, a.installationID, a.principalID, key)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, sqlite.ErrAuthorization) {
		return nil, workspace.ErrCapacity
	}
	if err != nil {
		return nil, err
	}
	if !a.auth.allows(original.TargetLocator) {
		return nil, workspace.ErrCapacity
	}
	return w.reservation.ClaimInstallationInvestigation(key)
}

func (o *WorkspaceOpener) ClaimWorkspaceInvestigation(ctx context.Context, a *Authorization, key string) (*workspace.OperationReservation, error) {
	if a == nil || (a.action != "workspace.read" && a.action != "workspace.operator") {
		return nil, ErrUnauthorized
	}
	owner, scope, err := o.owner(ctx, a)
	if err != nil {
		return nil, err
	}
	return owner.ClaimOperationInvestigation(ctx, scope, a.principalID, key, a.action == "workspace.operator", a.Check)
}

func (s *Store) checkInstallationInvestigation(ctx context.Context, key string) error {
	s.mu.Lock()
	c := s.capacityLocked()
	s.mu.Unlock()
	return c.CheckInstallationInvestigation(ctx, key)
}

func terminalInstallationOperation(o sqlite.InstallationOperation) bool {
	return o.Outcome == string(sqlite.InstallationRejected) || (o.Outcome == string(sqlite.InstallationCommitted) && o.Phase == string(sqlite.RegistrationConfirmed))
}

// Caller holds w.mu. Only authenticated original facts or confirmed writes call
// this; neither an HTTP error nor a successful physical Close proves the result.
func (w *workspaceCreation) observeQuotaOutcome(o sqlite.InstallationOperation) {
	w.quotaPending = true
	w.quotaKnown = terminalInstallationOperation(o)
}

// Store.mu remains held from the authenticated terminal read to bookkeeping.
// TryLock never waits in the reverse order; live native IO is not joined here.
// A stale GET must not clear uncertainty from a later mutating reconciliation.
func (c *WorkspaceCreator) rememberKnownQuotaLocked(key string) {
	s := c.auth.store
	w := s.workspaceCreations[key]
	if w == nil || !w.mu.TryLock() {
		return
	}
	defer w.mu.Unlock()
	w.quotaKnown = true
	if !w.active {
		w.reservation.Release()
		w.reservation = nil
	}
}

// ReserveWorkspaceOperation binds accounting to the same retained owner before
// request-body IO, without opening a directory or taking an execution turn.
func (o *WorkspaceOpener) ReserveWorkspaceOperation(ctx context.Context, a *Authorization, control, opening bool) (*workspace.OperationReservation, error) {
	owner, scope, err := o.owner(ctx, a)
	if err != nil {
		return nil, err
	}
	return owner.ReserveOperation(ctx, scope, a.Check, control, opening)
}
