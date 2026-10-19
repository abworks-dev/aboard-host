//go:build windows && amd64

// Package installation owns the installation database and its lifetime lease.
// Its entities and material are internal facts, not public protocol DTOs.
package installation

import (
	"context"
	"errors"
	"path/filepath"
	"sync"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	win "golang.org/x/sys/windows"
)

var ErrInstallation = errors.New("installation unavailable or inconsistent; no automatic initialization or repair")

var ErrSessionDeliveryPending = errors.New("session delivery not drained; installation lease retained")
var ErrWorkspaceCreationPending = errors.New("workspace creation not drained; installation lease retained")

// Identity contains only stable identifiers, not Windows SID or secret binding.
type Identity struct{ InstallationID, TenantID, OwnerPrincipalID string }

// Store never exports a database handle, SQL, material, or OS lease. Close is the
// sole release path and must not be raced with work outside its serialized owner.
type Store struct {
	mu                       sync.Mutex
	root                     string
	identity                 Identity
	db                       *sqlite.Database
	material                 security.Material
	lease                    *platform.Lease
	materialPin, databasePin *platform.File
	closed                   bool
	openFailure              bool
	// Authorization persistence uncertainty fences shared authorization uses,
	// not physical IO: it must never masquerade as a failed database Close.
	authUncertain bool
	// Under mu: committed-but-unfinished control-plane deliveries must finish
	// activation/revocation before normal Close may release this database owner.
	pendingDeliveries int
	// Under mu: live directory creation/coordinated recovery owners retain their
	// installation dependency until all physical work and required cleanup end.
	// Separate from session delivery and durable authorization uncertainty.
	pendingWorkspaceCreations int
	workspaceCreations        map[string]*workspaceCreation   // under mu; durable intent remains authoritative
	workspaceLifecycles       map[string]*workspace.Lifecycle // sole scoped lifecycle owner, retained until Store close
	workspaceOpenOwners       map[string]*workspace.OpenOwner
	liveWorkspaceHandles      int // under mu; retain installation through physical open/drain
	workspaceCapacity         *workspace.OperationCapacity
	workspaceStopping         bool
}

func (s *Store) Identity() Identity { s.mu.Lock(); defer s.mu.Unlock(); return s.identity }

// Initialize is explicit and only accepts a nonexistent final root. Failed
// initialization leaves its artifacts for investigation, never deletes/reseeds.
// If cleanup fails, a nonnil Store accompanies the error and retains ownership.
func Initialize(ctx context.Context, root string) (*Store, error) {
	if err := platform.CreatePrivateDirectory(root); err != nil {
		return nil, errors.Join(ErrInstallation, err)
	}
	lease, err := platform.Acquire(root, platform.Installation, true, false)
	if err != nil {
		return nil, errors.Join(ErrInstallation, err)
	}
	s := &Store{root: root, lease: lease}
	s.material, s.materialPin, err = security.InitializeWithLease(root, lease)
	if err != nil {
		return s.fail(err)
	}
	sid, err := installationSID()
	if err != nil {
		return s.fail(err)
	}
	if err = lease.ValidateInstallation(root); err != nil {
		return s.fail(err)
	}
	// host.lock and bootstrap.dpapi keep the root nonempty. The native relative
	// pins prevent conversion/replacement while SQLite performs its own O_EXCL.
	s.db, err = sqlite.OpenDatabase(ctx, filepath.Join(root, "installation.sqlite"), sqlite.CreateNew, sqlite.Genesis{Kind: sqlite.InstallationDatabase, InstallationID: s.material.InstallationID, TenantID: s.material.TenantID, OwnerPrincipalID: s.material.PrincipalID, WindowsSID: sid, SecretReference: s.material.SecretReference()})
	if err != nil {
		if errors.Is(err, sqlite.ErrPhysicalCloseUnknown) {
			s.openFailure = true
			return s, errors.Join(ErrInstallation, err)
		}
		return s.fail(err)
	}
	s.databasePin, err = lease.PinInstallationFile("installation.sqlite")
	if err != nil {
		return s.fail(err)
	}
	if err = s.verify(ctx, sid); err != nil {
		return s.fail(err)
	}
	return s, nil
}

// Open only opens existing fixed files under an existing actual host.lock. Bad
// ACL/material/schema/history/identity never becomes first use or a new root.
func Open(ctx context.Context, root string) (*Store, error) {
	if err := platform.CheckPrivateDirectory(root); err != nil {
		return nil, errors.Join(ErrInstallation, err)
	}
	lease, err := platform.Acquire(root, platform.Installation, false, false)
	if err != nil {
		return nil, errors.Join(ErrInstallation, err)
	}
	s := &Store{root: root, lease: lease}
	s.materialPin, err = lease.PinInstallationFile("bootstrap.dpapi")
	if err != nil {
		return s.fail(err)
	}
	s.material, err = security.LoadWithLease(root, lease, s.materialPin)
	if err != nil {
		return s.fail(err)
	}
	// Pin BEFORE SQLite opens: the verified historical database object cannot be
	// deleted/replaced between schema/identity verification and subsequent work.
	s.databasePin, err = lease.PinInstallationFile("installation.sqlite")
	if err != nil {
		return s.fail(err)
	}
	s.db, err = sqlite.OpenDatabase(ctx, filepath.Join(root, "installation.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.InstallationDatabase})
	if err != nil {
		if errors.Is(err, sqlite.ErrPhysicalCloseUnknown) {
			s.openFailure = true
			return s, errors.Join(ErrInstallation, err)
		}
		return s.fail(err)
	}
	sid, err := installationSID()
	if err != nil {
		return s.fail(err)
	}
	if err = s.verify(ctx, sid); err != nil {
		return s.fail(err)
	}
	return s, nil
}

func installationSID() (string, error) {
	token, err := win.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	user, readErr := token.GetTokenUser()
	err = errors.Join(readErr, token.Close())
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}
func (s *Store) verify(ctx context.Context, sid string) error {
	if err := s.lease.ValidateInstallation(s.root); err != nil {
		return err
	}
	if err := platform.CheckPrivateDirectory(s.root); err != nil {
		return err
	}
	id, err := s.db.ReadInstallationIdentity(ctx)
	if err != nil {
		return err
	}
	if id.InstallationID != s.material.InstallationID || id.TenantID != s.material.TenantID || id.OwnerPrincipalID != s.material.PrincipalID || id.WindowsSID != sid || id.SecretReference != s.material.SecretReference() || id.SchemaVersion != 1 {
		return ErrInstallation
	}
	s.identity = Identity{id.InstallationID, id.TenantID, id.OwnerPrincipalID}
	return s.lease.ValidateInstallation(s.root)
}
func (s *Store) fail(cause error) (*Store, error) {
	if err := s.Close(); err != nil {
		return s, errors.Join(ErrInstallation, cause, err)
	}
	return nil, errors.Join(ErrInstallation, cause)
}

// Close physically closes the database before clearing material or releasing
// any fixed file/OS lease. An error never claims the installation is released.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if s.pendingDeliveries != 0 {
		return ErrSessionDeliveryPending
	}
	if s.pendingWorkspaceCreations != 0 {
		return ErrWorkspaceCreationPending
	}
	if s.liveWorkspaceHandles != 0 {
		return ErrWorkspaceOpenPending
	}
	if s.openFailure {
		return errors.New("database open failed without physical-close evidence; installation lease retained until process exit")
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return err
		}
		s.db = nil
	}
	s.material.Clear()
	for _, pin := range []*platform.File{s.databasePin, s.materialPin} {
		if pin != nil {
			if err := pin.Close(); err != nil {
				return err
			}
		}
	}
	if s.lease != nil {
		if err := s.lease.Close(); err != nil {
			return err
		}
	}
	s.closed = true
	return nil
}
