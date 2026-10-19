//go:build windows && amd64

package installation

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/abworks-dev/aboard-host/internal/digest"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

var ErrDirectoryBinding = errors.New("directory binding incomplete; investigate original installation operation")

type RegisterDirectoryRequest struct{ OperationID, RootLocator, ExpectedWorkspaceID, ExpectedIdentityDigest string }
type RelinkDirectoryRequest struct {
	OperationID, RootLocator    string
	ExpectedRegistrationVersion int64
}

// DirectoryCoordinator is the business dispatch owner. It shares original-key
// resources and the installation uncertainty fence with WorkspaceCreator.
type DirectoryCoordinator struct {
	creator     *WorkspaceCreator
	afterIntent func() error
}

func NewDirectoryCoordinator(auth *Authenticator) (*DirectoryCoordinator, error) {
	c, e := NewWorkspaceCreator(auth)
	if e != nil {
		return nil, e
	}
	return &DirectoryCoordinator{creator: c}, nil
}
func bindingView(o sqlite.BindingOperation) InstallationOperationView {
	return operationView(sqlite.DirectoryOperation{InstallationOperation: o.InstallationOperation})
}
func (d *DirectoryCoordinator) checkLocked(ctx context.Context, a *Authorization, kind, id, path string) error {
	if d == nil || a == nil || a.auth != d.creator.auth {
		return ErrUnauthorized
	}
	if kind == "register" {
		if a.workspaceID != "" || a.action != "installation.operator" {
			return ErrUnauthorized
		}
	} else if kind == "relink" {
		if (a.workspaceID != id || a.action != "workspace.operator") && (a.workspaceID != "" || a.action != "installation.operator") {
			return ErrUnauthorized
		}
	} else {
		return ErrUnauthorized
	}
	if path != "" && !a.auth.allows(path) {
		return ErrUnauthorized
	}
	return a.checkLocked(ctx)
}
func (d *DirectoryCoordinator) owner(ctx context.Context, a *Authorization, key, kind, id string) (*workspaceCreation, error) {
	if a == nil {
		return nil, ErrUnauthorized
	}
	if _, e := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(a.installationID), PrincipalID: protocol.UUID(a.principalID), OperationID: protocol.UUID(key), Kind: kind, WorkspaceID: protocol.UUID(id), CanonicalLocator: "operation-key-validation"}); e != nil {
		return nil, ErrDirectoryBinding
	}
	s := d.creator.auth.store
	s.mu.Lock()
	if e := d.checkLocked(ctx, a, kind, id, ""); e != nil {
		s.mu.Unlock()
		return nil, e
	}
	if s.workspaceCreations == nil {
		s.workspaceCreations = make(map[string]*workspaceCreation)
	}
	w := s.workspaceCreations[key]
	if w == nil {
		w = &workspaceCreation{}
		s.workspaceCreations[key] = w
	}
	s.mu.Unlock()
	w.mu.Lock()
	s.mu.Lock()
	if e := d.checkLocked(ctx, a, kind, id, ""); e != nil {
		s.mu.Unlock()
		w.mu.Unlock()
		return nil, e
	}
	if !w.active {
		w.active = true
		s.pendingWorkspaceCreations++
	}
	s.mu.Unlock()
	return w, nil
}
func (s *Store) lifecycleLocked(scope workspace.Scope) (*workspace.Lifecycle, error) {
	if scope.InstallationID != s.identity.InstallationID || scope.TenantID != s.identity.TenantID || scope.Validate() != nil || s.closed {
		return nil, ErrUnauthorized
	}
	if s.workspaceLifecycles == nil {
		s.workspaceLifecycles = make(map[string]*workspace.Lifecycle)
	}
	l := s.workspaceLifecycles[scope.WorkspaceID]
	if l == nil {
		var err error
		l, err = workspace.NewLifecycle(scope)
		if err != nil {
			return nil, err
		}
		s.workspaceLifecycles[scope.WorkspaceID] = l
	}
	return l, nil
}

// WorkspaceLifecycle returns the SAME scoped owner used for binding admission.
// Future open/close composition must use it, not construct an alternative owner.
func (s *Store) WorkspaceLifecycle(ctx context.Context, a *Authorization) (*workspace.Lifecycle, error) {
	if a == nil || a.auth == nil || a.auth.store != s {
		return nil, ErrUnauthorized
	}
	scope, e := a.Scope()
	if e != nil {
		return nil, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e = a.checkLocked(ctx); e != nil {
		return nil, e
	}
	return s.lifecycleLocked(scope)
}
func (d *DirectoryCoordinator) registration(ctx context.Context, a *Authorization, id string) (sqlite.RegisteredDirectory, error) {
	s := d.creator.auth.store
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.ReadRegisteredDirectory(ctx, a.hash, a.version, a.installationID, a.principalID, id)
}
func (d *DirectoryCoordinator) freeze(ctx context.Context, a *Authorization, w *workspaceCreation, kind, id, root string) (sqlite.BindingOperation, error) {
	var o sqlite.BindingOperation
	s := d.creator.auth.store
	s.mu.Lock()
	e := d.checkLocked(ctx, a, kind, id, root)
	s.mu.Unlock()
	if e != nil {
		return o, e
	}
	w.lease, e = platform.Acquire(root, platform.Workspace, false, false)
	if e != nil {
		return o, e
	}
	dbPin, e := w.lease.PinFile(`data\workspace.sqlite`, true)
	if e != nil {
		return o, e
	}
	w.pins = append(w.pins, dbPin)
	manifest, e := w.lease.PinFile("aboard.workspace.json", true)
	if e != nil {
		return o, e
	}
	w.pins = append(w.pins, manifest)
	raw, e := platform.ReadPinnedWorkspaceFile(manifest, 4096)
	if e != nil {
		return o, e
	}
	desc, e := workspace.ParseDirectoryDescriptor(raw)
	if e != nil {
		return o, e
	}
	if desc.TenantID != a.tenantID {
		return o, ErrUnauthorized
	}
	if desc.WorkspaceID != id {
		return o, sqlite.ErrInstallationOperationConflict
	}
	if e = w.lease.WorkspaceDataHasOnlyDatabase(); e != nil {
		return o, e
	}
	w.db, e = sqlite.OpenDatabase(ctx, filepath.Join(root, "data", "workspace.sqlite"), sqlite.ReadOnlyScratch, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
	if errors.Is(e, sqlite.ErrPhysicalCloseUnknown) && w.db == nil {
		w.openUnknown = true
	}
	if e != nil {
		return o, e
	}
	if e = w.db.ValidateBindingReadiness(ctx, desc); e != nil {
		return o, e
	}
	if e = w.db.Close(); e != nil {
		return o, e
	}
	w.db = nil
	if e = w.lease.Validate(); e != nil {
		return o, e
	}
	r, b, m := w.lease.RootIdentity(), dbPin.Identity(), manifest.Identity()
	if r.Volume != b.Volume || r.Volume != m.Volume {
		return o, ErrDirectoryBinding
	}
	o.TenantID, o.IdentityDigest, o.RootVolume = a.tenantID, desc.IdentityDigest, strconv.FormatUint(r.Volume, 10)
	o.RootFileID = append([]byte{}, r.FileID[:]...)
	o.DatabaseFileID = append([]byte{}, b.FileID[:]...)
	o.DescriptorFileID = append([]byte{}, m.FileID[:]...)
	for _, allowed := range a.auth.roots {
		if underLocation(allowed, root) {
			o.AllowedLocation = allowed
			break
		}
	}
	if o.AllowedLocation == "" {
		return o, ErrUnauthorized
	}
	return o, nil
}
func samePhysicalBinding(a, b sqlite.BindingOperation) bool {
	return a.TenantID == b.TenantID && a.IdentityDigest == b.IdentityDigest && a.RootVolume == b.RootVolume && bytes.Equal(a.RootFileID, b.RootFileID) && bytes.Equal(a.DatabaseFileID, b.DatabaseFileID) && bytes.Equal(a.DescriptorFileID, b.DescriptorFileID)
}
func (d *DirectoryCoordinator) Register(ctx context.Context, a *Authorization, r RegisterDirectoryRequest) (InstallationOperationView, error) {
	return d.bind(ctx, a, "register", r.ExpectedWorkspaceID, r.OperationID, r.RootLocator, r.ExpectedIdentityDigest, 0)
}
func (d *DirectoryCoordinator) Relink(ctx context.Context, a *Authorization, id string, r RelinkDirectoryRequest) (InstallationOperationView, error) {
	if a == nil || a.workspaceID != id || a.action != "workspace.operator" {
		return InstallationOperationView{}, ErrUnauthorized
	}
	if r.ExpectedRegistrationVersion <= 0 {
		return InstallationOperationView{}, ErrDirectoryBinding
	}
	return d.bind(ctx, a, "relink", id, r.OperationID, r.RootLocator, "", r.ExpectedRegistrationVersion)
}
func (d *DirectoryCoordinator) bind(ctx context.Context, a *Authorization, kind, id, key, root, identity string, version int64) (view InstallationOperationView, err error) {
	if _, e := canonicalLocation(root); e != nil {
		return view, ErrDirectoryBinding
	}
	w, e := d.owner(ctx, a, key, kind, id)
	if e != nil {
		return view, e
	}
	defer w.mu.Unlock()
	if w.creation != nil || w.lease != nil || w.db != nil || w.openUnknown {
		return view, sqlite.ErrPhysicalCloseUnknown
	}
	closedInside := false
	defer func() {
		if !closedInside {
			err = errors.Join(err, d.creator.drain(w))
		}
	}()
	s := d.creator.auth.store
	s.mu.Lock()
	e = d.checkLocked(ctx, a, kind, id, root)
	var original sqlite.InstallationOperation
	if e == nil {
		original, e = s.db.ReadInstallationOperation(ctx, a.installationID, key)
	}
	s.mu.Unlock()
	if e == nil {
		if original.Kind != kind || original.WorkspaceID != id || original.PrincipalID != a.principalID || original.TargetLocator != root {
			return view, sqlite.ErrInstallationOperationConflict
		}
		old, e := d.readBinding(ctx, a, key)
		if e != nil {
			return view, e
		}
		if (kind == "register" && old.IdentityDigest != identity) || (kind == "relink" && old.ExpectedRegistrationVersion != version) {
			return view, sqlite.ErrInstallationOperationConflict
		}
		return bindingView(old), nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return view, e
	}
	scope := workspace.Scope{InstallationID: a.installationID, TenantID: a.tenantID, WorkspaceID: id}
	s.mu.Lock()
	l, e := s.lifecycleLocked(scope)
	s.mu.Unlock()
	if e != nil {
		return view, e
	}
	err = l.WithClosed(ctx, scope, func() (actionErr error) {
		defer func() {
			closedInside = true
			if closeErr := d.creator.drain(w); closeErr != nil {
				actionErr = errors.Join(actionErr, closeErr, workspace.ErrLifecycleUncertain)
			}
		}()
		observed, e := d.freeze(ctx, a, w, kind, id, root)
		if e != nil {
			return e
		}
		if kind == "register" && observed.IdentityDigest != identity {
			return sqlite.ErrInstallationOperationConflict
		}
		reg, e := d.registration(ctx, a, id)
		if kind == "relink" {
			if e != nil {
				return e
			}
			if reg.RegistrationVersion != version || reg.RootLocator == root || reg.VolumeIdentity != observed.RootVolume || !bytes.Equal(reg.RootFileID, observed.RootFileID) {
				return sqlite.ErrInstallationOperationConflict
			}
			if !a.auth.allows(reg.RootLocator) {
				return ErrUnauthorized
			}
			if _, e = os.Lstat(reg.RootLocator); !os.IsNotExist(e) {
				return ErrDirectoryBinding
			}
			observed.PreviousLocator = reg.RootLocator
			observed.ExpectedRegistrationVersion = version
		} else if e == nil {
			observed.ExpectedRegistrationVersion = reg.RegistrationVersion
		} else if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		h, e := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(a.installationID), PrincipalID: protocol.UUID(a.principalID), OperationID: protocol.UUID(key), Kind: kind, WorkspaceID: protocol.UUID(id), CanonicalLocator: root})
		if e != nil {
			return e
		}
		observed.InstallationOperation = sqlite.InstallationOperation{InstallationID: a.installationID, OperationID: key, PrincipalID: a.principalID, WorkspaceID: id, Kind: kind, TargetLocator: root, InputDigest: string(h), Phase: string(sqlite.IntentRecorded), Outcome: string(sqlite.InstallationAccepted)}
		s.mu.Lock()
		e = d.checkLocked(ctx, a, kind, id, root)
		var old bool
		if e == nil {
			observed, old, e = s.db.RecordBindingIntent(ctx, a.hash, a.version, observed)
		}
		if errors.Is(e, sqlite.ErrUnknown) {
			s.authUncertain = true
		}
		s.mu.Unlock()
		if e != nil {
			return e
		}
		view = bindingView(observed)
		if old {
			return nil
		}
		if d.afterIntent != nil {
			if e = d.afterIntent(); e != nil {
				return e
			}
		}
		if e = w.lease.Validate(); e != nil {
			return e
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if e = d.checkLocked(ctx, a, kind, id, root); e != nil {
			return e
		}
		e = s.db.FinalizeBinding(ctx, a.hash, a.version, observed, false)
		if errors.Is(e, sqlite.ErrUnknown) {
			s.authUncertain = true
			view.Outcome = sqlite.InstallationUnknown
		}
		if e != nil {
			return e
		}
		view.Phase, view.Outcome = sqlite.RegistrationConfirmed, sqlite.InstallationCommitted
		return nil
	})
	return view, err
}
func (d *DirectoryCoordinator) readBinding(ctx context.Context, a *Authorization, key string) (sqlite.BindingOperation, error) {
	s := d.creator.auth.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.workspaceID == "" && a.action == "installation.operator" {
		return s.db.ReadBindingInvestigation(ctx, a.hash, a.version, a.installationID, a.principalID, key)
	}
	return s.db.ReadBindingOperation(ctx, a.hash, a.version, a.installationID, a.principalID, key)
}

// Query/Reconcile dispatch by the persisted kind, never by a client claim.
func (d *DirectoryCoordinator) GetInstallationOperation(ctx context.Context, a *Authorization, key string) (InstallationOperationView, error) {
	o, e := d.dispatch(ctx, a, key)
	if e != nil {
		return InstallationOperationView{}, e
	}
	if o.Kind == "create" {
		return d.creator.GetInstallationOperation(ctx, a, key)
	}
	b, e := d.readBinding(ctx, a, key)
	if e != nil {
		return InstallationOperationView{}, e
	}
	if !a.auth.allows(b.TargetLocator) || !underLocation(b.AllowedLocation, b.TargetLocator) {
		return InstallationOperationView{}, ErrUnauthorized
	}
	return bindingView(b), nil
}
func (d *DirectoryCoordinator) dispatch(ctx context.Context, a *Authorization, key string) (sqlite.InstallationOperation, error) {
	if a == nil || a.auth != d.creator.auth || (a.action != "installation.operator" || a.workspaceID != "") {
		return sqlite.InstallationOperation{}, ErrUnauthorized
	}
	s := a.auth.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := a.checkLocked(ctx); e != nil {
		return sqlite.InstallationOperation{}, e
	}
	o, e := s.db.ReadInstallationOperation(ctx, a.installationID, key)
	if errors.Is(e, sql.ErrNoRows) {
		return o, ErrWorkspaceCreationNotFound
	}
	if e != nil {
		return o, e
	}
	if o.PrincipalID != a.principalID || (a.workspaceID != "" && o.WorkspaceID != a.workspaceID) {
		return sqlite.InstallationOperation{}, ErrUnauthorized
	}
	return o, nil
}
func (d *DirectoryCoordinator) ReconcileInstallationOperation(ctx context.Context, a *Authorization, key string, phase sqlite.InstallationPhase) (view InstallationOperationView, err error) {
	raw, e := d.dispatch(ctx, a, key)
	if e != nil {
		return view, e
	}
	if raw.Kind == "create" {
		return d.creator.ReconcileInstallationOperation(ctx, a, key, phase)
	}
	o, e := d.readBinding(ctx, a, key)
	if e != nil {
		return view, e
	}
	view = bindingView(o)
	if o.Phase != string(phase) {
		return view, sqlite.ErrInstallationOperationConflict
	}
	w, e := d.owner(ctx, a, key, o.Kind, o.WorkspaceID)
	if e != nil {
		return view, e
	}
	defer w.mu.Unlock()
	if w.creation != nil || w.lease != nil || w.db != nil || w.openUnknown {
		return view, sqlite.ErrPhysicalCloseUnknown
	}
	closedInside := false
	defer func() {
		if !closedInside {
			err = errors.Join(err, d.creator.drain(w))
		}
	}()
	s := a.auth.store
	scope := workspace.Scope{InstallationID: a.installationID, TenantID: a.tenantID, WorkspaceID: o.WorkspaceID}
	s.mu.Lock()
	l, e := s.lifecycleLocked(scope)
	s.mu.Unlock()
	if e != nil {
		return view, e
	}
	err = l.WithClosed(ctx, scope, func() (actionErr error) {
		defer func() {
			closedInside = true
			if closeErr := d.creator.drain(w); closeErr != nil {
				actionErr = errors.Join(actionErr, closeErr, workspace.ErrLifecycleUncertain)
			}
		}()
		if !underLocation(o.AllowedLocation, o.TargetLocator) {
			return ErrUnauthorized
		}
		observed, e := d.freeze(ctx, a, w, o.Kind, o.WorkspaceID, o.TargetLocator)
		if e != nil {
			return e
		}
		if !samePhysicalBinding(o, observed) {
			return sqlite.ErrInstallationOperationConflict
		}
		if o.Kind == "relink" {
			if !a.auth.allows(o.PreviousLocator) {
				return ErrUnauthorized
			}
			if _, e = os.Lstat(o.PreviousLocator); !os.IsNotExist(e) {
				return ErrDirectoryBinding
			}
		}
		if e = w.lease.Validate(); e != nil {
			return e
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if e = d.checkLocked(ctx, a, o.Kind, o.WorkspaceID, o.TargetLocator); e != nil {
			return e
		}
		e = s.db.FinalizeBinding(ctx, a.hash, a.version, o, true)
		if errors.Is(e, sqlite.ErrUnknown) {
			s.authUncertain = true
			view.Outcome = sqlite.InstallationUnknown
		}
		if e != nil {
			return e
		}
		view.Phase, view.Outcome = sqlite.RegistrationConfirmed, sqlite.InstallationCommitted
		return nil
	})
	return view, err
}
