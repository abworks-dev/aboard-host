//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

var ErrWorkspaceOpenPending = errors.New("workspace physical resources remain; installation lease retained")

type WorkspaceOpener struct{ auth *Authenticator }

func NewWorkspaceOpener(auth *Authenticator) (*WorkspaceOpener, error) {
	if auth == nil || auth.store == nil {
		return nil, ErrUnauthorized
	}
	return &WorkspaceOpener{auth: auth}, nil
}
func (o *WorkspaceOpener) owner(ctx context.Context, a *Authorization) (*workspace.OpenOwner, workspace.Scope, error) {
	if o == nil || a == nil || a.auth != o.auth {
		return nil, workspace.Scope{}, ErrUnauthorized
	}
	scope, err := a.Scope()
	if err != nil {
		return nil, scope, err
	}
	s := o.auth.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = a.checkLocked(ctx); err != nil {
		return nil, scope, err
	}
	lifecycle, err := s.lifecycleLocked(scope)
	if err != nil {
		return nil, scope, err
	}
	if s.workspaceOpenOwners == nil {
		s.workspaceOpenOwners = make(map[string]*workspace.OpenOwner)
	}
	owner := s.workspaceOpenOwners[scope.WorkspaceID]
	if owner == nil {
		if s.workspaceStopping {
			return nil, scope, workspace.ErrLifecycle
		}
		owner, err = workspace.NewOpenOwnerWithCapacity(scope, lifecycle, s.capacityLocked())
		if err != nil {
			return nil, scope, err
		}
		s.workspaceOpenOwners[scope.WorkspaceID] = owner
	}
	return owner, scope, nil
}
func (o *WorkspaceOpener) OpenWorkspace(ctx context.Context, a *Authorization, expectedRegistrationVersion int64) (workspace.OpenView, error) {
	if o == nil || a == nil || a.auth != o.auth {
		return workspace.OpenView{}, ErrUnauthorized
	}
	scope, err := a.Scope()
	if err != nil {
		return workspace.OpenView{}, err
	}
	check := func(ctx context.Context) error {
		if err := a.CheckAction(ctx, scope, "workspace.open"); err != nil {
			return err
		}
		if expectedRegistrationVersion <= 0 || a.location.RegistrationVersion != expectedRegistrationVersion {
			return workspace.ErrOpen
		}
		return nil
	}
	if err = check(ctx); err != nil {
		return workspace.OpenView{}, err
	}
	owner, _, err := o.owner(ctx, a)
	if err != nil {
		return workspace.OpenView{}, err
	}
	return owner.Open(ctx, scope, check, func(ctx context.Context, epoch string) (result workspace.OpenEvidence, err error) {
		s := o.auth.store
		s.mu.Lock()
		if err := a.checkLocked(ctx); err != nil {
			s.mu.Unlock()
			return workspace.OpenEvidence{}, err
		}
		if s.workspaceStopping {
			s.mu.Unlock()
			return workspace.OpenEvidence{}, workspace.ErrLifecycle
		}
		s.liveWorkspaceHandles++
		s.mu.Unlock()
		r := &openResources{store: s, scope: scope, active: true}
		defer func() {
			if errors.Is(err, platform.ErrPhysicalCloseUnknown) {
				r.quarantined = true
				r.nativeFailure = err
			}
		}()
		evidence := workspace.OpenEvidence{Resources: r}
		r.lease, err = platform.Acquire(a.location.RootLocator, platform.Workspace, false, false)
		if err != nil {
			return evidence, err
		}
		root := r.lease.RootIdentity()
		if strconv.FormatUint(root.Volume, 10) != a.location.VolumeIdentity || root.FileID != a.location.RootFileID {
			return evidence, workspace.ErrOpen
		}
		r.descriptorPin, err = r.lease.PinFile("aboard.workspace.json", true)
		if err != nil {
			return evidence, err
		}
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
		r.databasePin, err = r.lease.PinFile(`data\workspace.sqlite`, false)
		if err != nil {
			return evidence, err
		}
		// Only explicit open may recover storage. First recover a frozen private
		// copy and validate the full existing history; unsupported/corrupt inputs
		// never reach writable source SQLite. Inspect/register remain non-recovering.
		recovering, recoveryErr := r.preflightRecovery(ctx, descriptor, check)
		if recoveryErr != nil {
			return evidence, recoveryErr
		}
		if err = r.lease.Validate(); err != nil {
			return evidence, err
		}
		r.database, err = sqlite.OpenDatabase(ctx, filepath.Join(a.location.RootLocator, "data", "workspace.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
		if err != nil {
			r.quarantined = recovering || errors.Is(err, sqlite.ErrPhysicalCloseUnknown)
			return evidence, err
		}
		r.pending, err = r.database.ValidateOpenHistory(ctx, descriptor)
		if err != nil {
			r.quarantined = recovering
			return evidence, err
		}
		r.gate, err = sqlite.NewCommitGate(r.database, epoch)
		if err != nil {
			return evidence, err
		}
		if r.pending {
			// A valid historical view is not writable admission. Keep the original
			// gate/lease for explicit investigation; publish neither epoch nor an
			// ordinary business-read snapshot while persistence remains unknown.
			return evidence, errors.Join(workspace.ErrOpenUncertain, sqlite.ErrUnknown)
		}
		r.facts, err = sqlite.NewWorkspaceFacts(r.gate, scope)
		if err != nil {
			return evidence, err
		}
		evidence.Snapshot, err = r.facts.Snapshot(ctx, scope)
		if err == nil {
			evidence.SaveOperationIDs, err = r.gate.SaveOperationIDs(ctx, scope)
		}
		evidence.NeedsInvestigation = r.pending
		return evidence, err
	})
}

// DrainWorkspace is a trusted physical primitive for composition, not the T021
// public close protocol/operation history. It cannot clear unknown persistence.
func (o *WorkspaceOpener) DrainWorkspace(ctx context.Context, a *Authorization) error {
	if o == nil || a == nil || a.auth != o.auth {
		return ErrUnauthorized
	}
	scope, err := a.Scope()
	if err != nil {
		return err
	}
	check := func(ctx context.Context) error { return a.CheckAction(ctx, scope, "workspace.close") }
	if err = check(ctx); err != nil {
		return err
	}
	owner, _, err := o.owner(ctx, a)
	if err != nil {
		return err
	}
	return owner.DrainOpen(ctx, scope, check)
}

type openResources struct {
	store                        *Store
	scope                        workspace.Scope
	active, pending, quarantined bool
	nativeFailure                error // retain failed native factory resources, even nil+error
	lease                        *platform.Lease
	descriptorPin, databasePin   *platform.File
	database                     *sqlite.Database
	gate                         *sqlite.CommitGate
	facts                        *sqlite.WorkspaceFacts
	recoveryInputs               *platform.FrozenWorkspaceInputs
	recoveryScratch              *platform.Scratch
	recoveryDatabase             *sqlite.Database
	recoveryScratchDeleted       bool
	investigationView            bool
}

func (r *openResources) Validate() error {
	if r.quarantined || r.lease == nil {
		return workspace.ErrOpenUncertain
	}
	return r.lease.Validate()
}
func (r *openResources) Close() (err error) {
	defer func() {
		if errors.Is(err, platform.ErrPhysicalCloseUnknown) {
			r.quarantined = true
			r.nativeFailure = err
		}
	}()
	if r.quarantined || r.pending {
		return workspace.ErrOpenUncertain
	}
	if err := r.closeRecoveryScratch(); err != nil {
		return err
	}
	if r.recoveryInputs != nil {
		if err := r.recoveryInputs.Close(); err != nil {
			return err
		}
		r.recoveryInputs = nil
	}
	if r.gate != nil {
		if err := r.gate.EndPhysical(); err != nil {
			return err
		}
		r.gate = nil
		r.database = nil
	} else if r.database != nil {
		if err := r.database.Close(); err != nil {
			return err
		}
		r.database = nil
	}
	for _, pin := range []*platform.File{r.databasePin, r.descriptorPin} {
		if pin != nil {
			if err := pin.Close(); err != nil {
				return err
			}
		}
	}
	if r.lease != nil {
		if err := r.lease.Close(); err != nil {
			return err
		}
		r.lease = nil
	}
	if r.active {
		r.store.mu.Lock()
		r.store.liveWorkspaceHandles--
		r.active = false
		r.store.mu.Unlock()
	}
	return nil
}
func (r *openResources) Snapshot(ctx context.Context, scope workspace.Scope) (workspace.Snapshot, error) {
	if r.pending || r.quarantined || r.facts == nil {
		return workspace.Snapshot{}, workspace.ErrOpenUncertain
	}
	return r.facts.Snapshot(ctx, scope)
}
func (r *openResources) Current(ctx context.Context, scope workspace.Scope, id string) (workspace.TextRecord, error) {
	if r.pending || r.quarantined || r.facts == nil {
		return workspace.TextRecord{}, workspace.ErrOpenUncertain
	}
	return r.facts.Current(ctx, scope, id)
}
func (r *openResources) List(ctx context.Context, scope workspace.Scope, after string, limit int) ([]workspace.TextRecord, error) {
	if r.pending || r.quarantined || r.facts == nil {
		return nil, workspace.ErrOpenUncertain
	}
	return r.facts.List(ctx, scope, after, limit)
}
