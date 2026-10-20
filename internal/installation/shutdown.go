//go:build windows && amd64

package installation

import (
	"context"
	"errors"
)

var ErrShutdownPending = errors.New("installation shutdown lacks known physical and persistent completion")

// CheckShutdownReady is nonmutating. Host must quiesce HTTP and native control
// handlers, then recheck this before Close; a successful probe is not a lease or
// a promise against concurrent issuance. Close remains the sole release path.
func (s *Store) CheckShutdownReady(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if s.openFailure || s.authUncertain || s.pendingDeliveries != 0 || len(s.inspections) != 0 || s.pendingWorkspaceCreations != 0 || s.liveWorkspaceHandles != 0 || s.db == nil {
		return ErrShutdownPending
	}
	pending, err := s.db.HasUnresolvedInstallationOperations(ctx, s.identity.InstallationID)
	if err != nil || pending {
		return errors.Join(ErrShutdownPending, err)
	}
	return ctx.Err()
}
