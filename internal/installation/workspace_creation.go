//go:build windows && amd64

package installation

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/abworks-dev/aboard-host/internal/digest"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

var (
	ErrWorkspaceCreation         = errors.New("workspace creation incomplete; investigate original installation operation")
	ErrWorkspaceCreationNotFound = errors.New("installation operation not found")
)

// Workspace identity is generated here only for a genuinely new durable intent,
// never in HTTP or from caller input. Original keys reuse their frozen identity.
type CreateWorkspaceRequest struct{ OperationID, RootLocator, DisplayName string }

func newWorkspaceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// InstallationOperationView deliberately excludes locators, physical IDs, SQL,
// secrets and live control handles. InstallationOperation is not a save receipt.
type InstallationOperationView struct {
	InstallationID, OperationID, PrincipalID, WorkspaceID, Kind, InputDigest string
	Phase                                                                    sqlite.InstallationPhase
	Outcome                                                                  sqlite.InstallationOutcome
}

func operationView(o sqlite.DirectoryOperation) InstallationOperationView {
	return InstallationOperationView{o.InstallationID, o.OperationID, o.PrincipalID, o.WorkspaceID, o.Kind, o.InputDigest, sqlite.InstallationPhase(o.Phase), sqlite.InstallationOutcome(o.Outcome)}
}

// WorkspaceCreator has no private owner map: all wrappers share Store's unique
// durable-key owner map. HTTP can call these business cases, never an adapter.
type WorkspaceCreator struct {
	auth       *Authenticator
	afterPhase func(sqlite.InstallationPhase) error // internal failure seam; no public control API
}

func NewWorkspaceCreator(auth *Authenticator) (*WorkspaceCreator, error) {
	if auth == nil || auth.store == nil {
		return nil, ErrUnauthorized
	}
	return &WorkspaceCreator{auth: auth}, nil
}

// workspaceCreation is retained in Store through every physical-close unknown,
// even when the database factory returned nil. Store.Close cannot outrun it.
type workspaceCreation struct {
	mu          sync.Mutex
	active      bool // changed under Store.mu only; all resource fields under mu
	creation    *platform.WorkspaceCreation
	lease       *platform.Lease
	pins        []*platform.File
	db          *sqlite.Database
	openUnknown bool
}

func (c *WorkspaceCreator) checkLocked(ctx context.Context, operator *Authorization, path string) error {
	if operator == nil || operator.auth != c.auth || operator.workspaceID != "" || operator.action != "installation.operator" {
		return ErrUnauthorized
	}
	if path != "" && !c.auth.allows(path) {
		return ErrUnauthorized
	}
	return operator.checkLocked(ctx)
}
func (c *WorkspaceCreator) owner(ctx context.Context, operator *Authorization, key string) (*workspaceCreation, error) {
	if operator == nil {
		return nil, ErrUnauthorized
	}
	if _, err := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(operator.installationID), PrincipalID: protocol.UUID(operator.principalID), OperationID: protocol.UUID(key), Kind: "create", WorkspaceID: protocol.UUID(operator.tenantID), CanonicalLocator: "operation-key-validation"}); err != nil {
		return nil, ErrWorkspaceCreation
	}
	s := c.auth.store
	s.mu.Lock()
	if err := c.checkLocked(ctx, operator, ""); err != nil {
		s.mu.Unlock()
		return nil, err
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
	w.mu.Lock() // never wait for workspace IO with the installation mutex held
	s.mu.Lock()
	if err := c.checkLocked(ctx, operator, ""); err != nil {
		s.mu.Unlock()
		w.mu.Unlock()
		return nil, err
	}
	if !w.active {
		w.active = true
		s.pendingWorkspaceCreations++
	}
	s.mu.Unlock()
	return w, nil
}
func (c *WorkspaceCreator) closePhysical(w *workspaceCreation) error {
	if w.openUnknown {
		return errors.Join(ErrWorkspaceCreation, sqlite.ErrPhysicalCloseUnknown)
	}
	if w.db != nil {
		if err := w.db.Close(); err != nil {
			return err
		}
		w.db = nil
	}
	if w.creation != nil {
		if err := w.creation.Close(); err != nil {
			return err
		}
		w.creation = nil
	}
	for _, p := range w.pins {
		if err := p.Close(); err != nil {
			return err
		}
	}
	if w.lease != nil {
		if err := w.lease.Close(); err != nil {
			return err
		}
		w.lease = nil
	}
	w.pins = nil
	return nil
}
func (c *WorkspaceCreator) releaseOwner(w *workspaceCreation) {
	s := c.auth.store
	s.mu.Lock()
	if w.active {
		w.active = false
		s.pendingWorkspaceCreations--
	}
	s.mu.Unlock()
}
func (c *WorkspaceCreator) drain(w *workspaceCreation) error {
	if err := c.closePhysical(w); err != nil {
		return err
	}
	c.releaseOwner(w)
	return nil
}
func (c *WorkspaceCreator) readLocked(ctx context.Context, operator *Authorization, key string) (sqlite.DirectoryOperation, error) {
	if err := c.checkLocked(ctx, operator, ""); err != nil {
		return sqlite.DirectoryOperation{}, err
	}
	return c.auth.store.db.ReadDirectoryOperation(ctx, operator.hash, operator.version, operator.installationID, operator.principalID, key)
}
func (c *WorkspaceCreator) GetInstallationOperation(ctx context.Context, operator *Authorization, key string) (InstallationOperationView, error) {
	s := c.auth.store
	s.mu.Lock()
	defer s.mu.Unlock()
	o, err := c.readLocked(ctx, operator, key)
	if errors.Is(err, sql.ErrNoRows) {
		return InstallationOperationView{}, ErrWorkspaceCreationNotFound
	}
	if err != nil {
		return InstallationOperationView{}, err
	}
	// Location policy is current, not a historical authorization in the row.
	if !c.auth.allows(o.TargetLocator) || !underLocation(o.AllowedLocation, o.TargetLocator) {
		return InstallationOperationView{}, ErrUnauthorized
	}
	return operationView(o), nil
}
func (c *WorkspaceCreator) intent(operator *Authorization, r CreateWorkspaceRequest, workspaceID string) (sqlite.DirectoryOperation, error) {
	var o sqlite.DirectoryOperation
	if _, err := canonicalLocation(r.RootLocator); err != nil || !sqlite.ValidCreationDisplayName(r.DisplayName) {
		return o, ErrWorkspaceCreation
	}
	var allowed string
	for _, root := range c.auth.roots {
		if underLocation(root, r.RootLocator) && len(root) > len(allowed) {
			allowed = root
		}
	}
	if allowed == "" {
		return o, ErrUnauthorized
	}
	h, err := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(operator.installationID), PrincipalID: protocol.UUID(operator.principalID), OperationID: protocol.UUID(r.OperationID), Kind: "create", WorkspaceID: protocol.UUID(workspaceID), CanonicalLocator: r.RootLocator})
	if err != nil {
		return o, ErrWorkspaceCreation
	}
	id, err := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID(workspaceID), TenantID: protocol.UUID(operator.tenantID), DatabaseRelativePath: "data/workspace.sqlite"})
	if err != nil {
		return o, ErrWorkspaceCreation
	}
	o.InstallationOperation = sqlite.InstallationOperation{InstallationID: operator.installationID, OperationID: r.OperationID, PrincipalID: operator.principalID, WorkspaceID: workspaceID, Kind: "create", TargetLocator: r.RootLocator, InputDigest: string(h), Phase: string(sqlite.IntentRecorded), Outcome: string(sqlite.InstallationAccepted)}
	o.TenantID, o.DisplayName, o.AllowedLocation, o.IdentityDigest = operator.tenantID, r.DisplayName, allowed, string(id)
	return o, nil
}
func (c *WorkspaceCreator) checkpoint(ctx context.Context, operator *Authorization, o *sqlite.DirectoryOperation, next sqlite.InstallationPhase) error {
	s := c.auth.store
	s.mu.Lock()
	err := c.checkLocked(ctx, operator, o.TargetLocator)
	if err == nil {
		err = s.db.CheckpointCreation(ctx, operator.hash, operator.version, *o, next)
		if errors.Is(err, sqlite.ErrUnknown) {
			s.authUncertain = true
		}
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	o.Phase = string(next)
	if c.afterPhase != nil {
		return c.afterPhase(next)
	}
	return nil
}
func (c *WorkspaceCreator) interrupted(ctx context.Context, operator *Authorization, o sqlite.DirectoryOperation, cause error) error {
	s := c.auth.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := c.checkLocked(ctx, operator, o.TargetLocator); err != nil {
		return err
	}
	err := s.db.InterruptCreation(ctx, operator.hash, operator.version, o, errors.Is(cause, sqlite.ErrUnknown) || errors.Is(cause, sqlite.ErrPhysicalCloseUnknown))
	if errors.Is(err, sqlite.ErrUnknown) {
		s.authUncertain = true
	}
	return err
}
func (c *WorkspaceCreator) finalize(ctx context.Context, operator *Authorization, o sqlite.DirectoryOperation, reconcile bool) error {
	s := c.auth.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := c.checkLocked(ctx, operator, o.TargetLocator); err != nil {
		return err
	}
	err := s.db.FinalizeCreation(ctx, operator.hash, operator.version, o, reconcile)
	// The internal seam also models loss of confirmation after a real COMMIT;
	// it cannot grant anything or suppress an adapter error.
	if err == nil && c.afterPhase != nil {
		err = c.afterPhase(sqlite.RegistrationConfirmed)
	}
	if errors.Is(err, sqlite.ErrUnknown) {
		s.authUncertain = true
	}
	return err
}

// CreateWorkspace is staged across two databases and files, NOT atomic. Every
// DURABLY admitted original key is inquiry-only, never repeated filesystem IO.
// Actual intent COMMIT uncertainty fences the current Store before any target
// FS operation. A cold missing row is not proof of the old RPC result and grants
// no recovery permission: Get/Reconcile only report missing evidence. No recovery
// path may convert that absence into Create or promise eternal RPC deduplication.
func (c *WorkspaceCreator) CreateWorkspace(ctx context.Context, operator *Authorization, r CreateWorkspaceRequest) (view InstallationOperationView, err error) {
	if operator == nil {
		return view, ErrUnauthorized
	}
	var o sqlite.DirectoryOperation
	admitted := false
	if _, err := canonicalLocation(r.RootLocator); err != nil || !sqlite.ValidCreationDisplayName(r.DisplayName) {
		return view, ErrWorkspaceCreation
	}
	w, err := c.owner(ctx, operator, r.OperationID)
	if err != nil {
		return view, err
	}
	defer w.mu.Unlock()
	// A live unknown physical owner cannot be overwritten by a new call/wrapper.
	if w.creation != nil || w.lease != nil || w.db != nil || w.openUnknown {
		return view, errors.Join(ErrWorkspaceCreation, sqlite.ErrPhysicalCloseUnknown)
	}
	defer func() {
		closeErr := c.closePhysical(w)
		err = errors.Join(err, closeErr)
		if err != nil && admitted && o.Phase != "" && o.Outcome != string(sqlite.InstallationCommitted) {
			if errors.Is(err, sqlite.ErrUnknown) || errors.Is(err, sqlite.ErrPhysicalCloseUnknown) {
				view.Outcome = sqlite.InstallationUnknown
			} else {
				view.Outcome = sqlite.InstallationRejected
			}
			// A canceled request must not suppress best-effort recording, but this fresh
			// transaction still checks that the current session has not been revoked.
			err = errors.Join(ErrWorkspaceCreation, err, c.interrupted(context.WithoutCancel(ctx), operator, o, err))
		}
		// Keep the installation alive through final interruption bookkeeping too.
		if closeErr == nil {
			c.releaseOwner(w)
		}
	}()
	s := c.auth.store
	s.mu.Lock()
	if err = c.checkLocked(ctx, operator, r.RootLocator); err != nil {
		s.mu.Unlock()
		return view, err
	}
	// Admission and identity generation belong to the shared original-key owner.
	// A present row is inquiry-only, including accepted/unknown/rejected phases.
	o, err = c.readLocked(ctx, operator, r.OperationID)
	if err == nil {
		if o.PrincipalID != operator.principalID || o.TargetLocator != r.RootLocator || o.DisplayName != r.DisplayName {
			s.mu.Unlock()
			return view, sqlite.ErrInstallationOperationConflict
		}
		if !underLocation(o.AllowedLocation, o.TargetLocator) {
			s.mu.Unlock()
			return view, ErrUnauthorized
		}
		s.mu.Unlock()
		return operationView(o), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		s.mu.Unlock()
		return view, err
	}
	var workspaceID string
	workspaceID, err = newWorkspaceID()
	if err == nil {
		o, err = c.intent(operator, r, workspaceID)
	}
	var existing bool
	if err == nil {
		o, existing, err = s.db.RecordCreationIntent(ctx, operator.hash, operator.version, o)
	}
	// An actual adapter transaction unknown fences this SAME installation conn,
	// not merely final grants. Do not write Interrupt or infer safe first use.
	if errors.Is(err, sqlite.ErrUnknown) {
		s.authUncertain = true
	}
	s.mu.Unlock()
	if err != nil {
		return view, err
	}
	if existing {
		return operationView(o), nil
	}
	admitted = true
	if c.afterPhase != nil {
		if err = c.afterPhase(sqlite.IntentRecorded); err != nil {
			return operationView(o), err
		}
	}
	if err = operator.CheckLocation(ctx, o.TargetLocator); err != nil {
		return operationView(o), err
	}
	w.creation, err = platform.InspectWorkspaceCreation(o.TargetLocator)
	if err != nil {
		return operationView(o), err
	}
	if err = w.creation.Prepare(); err != nil {
		return operationView(o), err
	}
	root, dbID := w.creation.RootIdentity(), w.creation.DatabaseIdentity()
	if root.Volume != dbID.Volume {
		return operationView(o), ErrWorkspaceCreation
	}
	o.RootVolume = strconv.FormatUint(root.Volume, 10)
	o.RootFileID = append([]byte{}, root.FileID[:]...)
	o.DatabaseFileID = append([]byte{}, dbID.FileID[:]...)
	if err = c.checkpoint(ctx, operator, &o, sqlite.DirectoryPrepared); err != nil {
		return operationView(o), err
	}
	if err = operator.CheckLocation(ctx, o.TargetLocator); err != nil {
		return operationView(o), err
	}
	w.db, err = sqlite.InitializePinnedWorkspace(ctx, w.creation.FreshDatabase(), sqlite.Genesis{Kind: sqlite.WorkspaceDatabase, WorkspaceID: o.WorkspaceID, TenantID: o.TenantID, DisplayName: o.DisplayName})
	if errors.Is(err, sqlite.ErrPhysicalCloseUnknown) && w.db == nil {
		w.openUnknown = true
	}
	if err != nil {
		return operationView(o), err
	}
	if err = w.creation.Validate(); err != nil {
		return operationView(o), err
	}
	if err = c.checkpoint(ctx, operator, &o, sqlite.GenesisConfirmed); err != nil {
		return operationView(o), err
	}
	descriptor, err := workspace.NewDirectoryDescriptor(o.WorkspaceID, o.TenantID)
	if err != nil {
		return operationView(o), err
	}
	encoded, err := workspace.EncodeDirectoryDescriptor(descriptor)
	if err != nil {
		return operationView(o), err
	}
	if err = operator.CheckLocation(ctx, o.TargetLocator); err != nil {
		return operationView(o), err
	}
	if err = w.creation.PublishDescriptor(encoded); err != nil {
		return operationView(o), err
	}
	descriptorID := w.creation.DescriptorIdentity()
	if descriptorID.Volume != root.Volume {
		return operationView(o), ErrWorkspaceCreation
	}
	o.DescriptorFileID = append([]byte{}, descriptorID.FileID[:]...)
	if err = c.checkpoint(ctx, operator, &o, sqlite.DescriptorPublished); err != nil {
		return operationView(o), err
	}
	// Close SQLite before final registration: successful return proves physical
	// writer drain as well as metadata persistence. Pins/lease are still retained.
	if err = w.db.Close(); err != nil {
		return operationView(o), err
	}
	w.db = nil
	if err = w.creation.Validate(); err != nil {
		return operationView(o), err
	}
	if err = c.finalize(ctx, operator, o, false); err != nil {
		return operationView(o), err
	}
	o.Phase, o.Outcome = string(sqlite.RegistrationConfirmed), string(sqlite.InstallationCommitted)
	return operationView(o), nil
}

// DrainWorkspaceCreation is authenticated physical close ONLY. It never retries
// creation/genesis/registration or clears a durable authorization-unknown fence.
func (c *WorkspaceCreator) DrainWorkspaceCreation(ctx context.Context, operator *Authorization, key string) error {
	w, err := c.owner(ctx, operator, key)
	if err != nil {
		return err
	}
	defer w.mu.Unlock()
	return c.drain(w)
}

// ReconcileInstallationOperation may complete ONLY the original registration,
// after proving all original evidence. It cannot seed a DB, publish a manifest,
// replay create, relink a root, reset identity, or infer an unknown missing row.
func (c *WorkspaceCreator) ReconcileInstallationOperation(ctx context.Context, operator *Authorization, key string, expectedPhase sqlite.InstallationPhase) (view InstallationOperationView, err error) {
	w, err := c.owner(ctx, operator, key)
	if err != nil {
		return view, err
	}
	defer w.mu.Unlock()
	if w.creation != nil || w.lease != nil || w.db != nil || w.openUnknown {
		return view, errors.Join(ErrWorkspaceCreation, sqlite.ErrPhysicalCloseUnknown)
	}
	defer func() { err = errors.Join(err, c.drain(w)) }()
	s := c.auth.store
	s.mu.Lock()
	o, err := c.readLocked(ctx, operator, key)
	s.mu.Unlock()
	if errors.Is(err, sql.ErrNoRows) {
		return view, ErrWorkspaceCreationNotFound
	}
	if err != nil {
		return view, err
	}
	view = operationView(o)
	if o.Phase != string(expectedPhase) {
		return view, sqlite.ErrInstallationOperationConflict
	}
	if o.Kind != "create" || (o.Phase != string(sqlite.DescriptorPublished) && o.Phase != string(sqlite.RegistrationConfirmed)) || len(o.RootFileID) != 16 || len(o.DatabaseFileID) != 16 || len(o.DescriptorFileID) != 16 {
		return view, ErrWorkspaceCreation
	}
	if !underLocation(o.AllowedLocation, o.TargetLocator) {
		return view, ErrUnauthorized
	}
	if err = operator.CheckLocation(ctx, o.TargetLocator); err != nil {
		return view, err
	}
	w.lease, err = platform.Acquire(o.TargetLocator, platform.Workspace, false, false)
	if err != nil {
		return view, err
	}
	root := w.lease.RootIdentity()
	if strconv.FormatUint(root.Volume, 10) != o.RootVolume || !bytes.Equal(root.FileID[:], o.RootFileID) {
		return view, ErrWorkspaceCreation
	}
	dbPin, err := w.lease.PinFile(`data\workspace.sqlite`, true)
	if err != nil {
		return view, err
	}
	w.pins = append(w.pins, dbPin)
	descriptorPin, err := w.lease.PinFile("aboard.workspace.json", true)
	if err != nil {
		return view, err
	}
	w.pins = append(w.pins, descriptorPin)
	dbID, descriptorID := dbPin.Identity(), descriptorPin.Identity()
	if dbID.Volume != root.Volume || descriptorID.Volume != root.Volume || !bytes.Equal(dbID.FileID[:], o.DatabaseFileID) || !bytes.Equal(descriptorID.FileID[:], o.DescriptorFileID) {
		return view, ErrWorkspaceCreation
	}
	b, err := platform.ReadPinnedWorkspaceFile(descriptorPin, 4096)
	if err != nil {
		return view, err
	}
	descriptor, err := workspace.ParseDirectoryDescriptor(b)
	if err != nil || string(descriptor.WorkspaceID) != o.WorkspaceID || string(descriptor.TenantID) != o.TenantID || string(descriptor.IdentityDigest) != o.IdentityDigest {
		return view, ErrWorkspaceCreation
	}
	if err = w.lease.WorkspaceDataHasOnlyDatabase(); err != nil {
		return view, err
	}
	// No sidecars means readonly URI cannot recover an interrupted journal. A
	// frozen original DB and exact ledger are required; never rw/recover a source.
	w.db, err = sqlite.OpenDatabase(ctx, filepath.Join(o.TargetLocator, "data", "workspace.sqlite"), sqlite.ReadOnlyScratch, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
	if errors.Is(err, sqlite.ErrPhysicalCloseUnknown) && w.db == nil {
		w.openUnknown = true
	}
	if err != nil {
		return view, err
	}
	if o.Outcome == string(sqlite.InstallationCommitted) {
		// A committed creation may already have legitimate business history.
		// Verify its current identity/schema, not an obsolete empty-genesis state.
		// Exact original physical objects and current authorization remain required;
		// finalize's committed branch only verifies the existing registration/grants.
		err = w.db.ValidateDirectoryDescriptor(ctx, descriptor)
	} else {
		// Pending finalization still needs the original pristine genesis proof.
		err = w.db.VerifyCreationGenesis(ctx, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase, WorkspaceID: o.WorkspaceID, TenantID: o.TenantID, DisplayName: o.DisplayName})
	}
	if err != nil {
		return view, err
	}
	if err = w.db.Close(); err != nil {
		return view, err
	}
	w.db = nil
	if err = w.lease.Validate(); err != nil {
		return view, err
	}
	if err = c.finalize(ctx, operator, o, true); err != nil {
		return view, err
	}
	o.Phase, o.Outcome = string(sqlite.RegistrationConfirmed), string(sqlite.InstallationCommitted)
	return operationView(o), nil
}
