package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

var (
	ErrInstallationOperation         = errors.New("installation operation unavailable or inconsistent")
	ErrInstallationOperationConflict = errors.New("installation operation key conflicts with original intent")
)

type InstallationPhase string
type InstallationOutcome string

const (
	IntentRecorded        InstallationPhase   = "intentRecorded"
	DirectoryPrepared     InstallationPhase   = "directoryPrepared"
	GenesisConfirmed      InstallationPhase   = "genesisConfirmed"
	DescriptorPublished   InstallationPhase   = "descriptorPublished"
	RegistrationConfirmed InstallationPhase   = "registrationConfirmed"
	InstallationAccepted  InstallationOutcome = "accepted"
	InstallationCommitted InstallationOutcome = "committed"
	InstallationRejected  InstallationOutcome = "rejected"
	InstallationUnknown   InstallationOutcome = "unknown"
)

// DirectoryOperation composes the existing minimal typed dispatch record with
// create-only frozen intent/evidence. No register/relink implementation lives here.
// This is private storage data, NEVER a public result: it contains locators.
type DirectoryOperation struct {
	InstallationOperation
	TenantID, DisplayName, AllowedLocation, IdentityDigest string
	RootVolume                                             string
	RootFileID, DatabaseFileID, DescriptorFileID           []byte
}

func ValidCreationDisplayName(name string) bool {
	return utf8.ValidString(name) && utf8.RuneCountInString(name) >= 1 && utf8.RuneCountInString(name) <= 256 && !strings.ContainsRune(name, 0)
}
func validCreationIntent(o DirectoryOperation) bool {
	h, err := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(o.InstallationID), PrincipalID: protocol.UUID(o.PrincipalID), OperationID: protocol.UUID(o.OperationID), Kind: "create", WorkspaceID: protocol.UUID(o.WorkspaceID), CanonicalLocator: o.TargetLocator})
	id, idErr := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID(o.WorkspaceID), TenantID: protocol.UUID(o.TenantID), DatabaseRelativePath: "data/workspace.sqlite"})
	return err == nil && idErr == nil && o.Kind == "create" && o.InputDigest == string(h) && o.IdentityDigest == string(id) && ValidCreationDisplayName(o.DisplayName) && o.AllowedLocation != ""
}
func sameCreationIntent(a, b DirectoryOperation) bool {
	return a.InstallationID == b.InstallationID && a.OperationID == b.OperationID && a.PrincipalID == b.PrincipalID && a.WorkspaceID == b.WorkspaceID && a.Kind == b.Kind && a.TargetLocator == b.TargetLocator && a.InputDigest == b.InputDigest && a.TenantID == b.TenantID && a.DisplayName == b.DisplayName && a.AllowedLocation == b.AllowedLocation && a.IdentityDigest == b.IdentityDigest
}
func sameCreationEvidence(a, b DirectoryOperation) bool {
	return a.RootVolume == b.RootVolume && reflect.DeepEqual(a.RootFileID, b.RootFileID) && reflect.DeepEqual(a.DatabaseFileID, b.DatabaseFileID) && reflect.DeepEqual(a.DescriptorFileID, b.DescriptorFileID)
}
func creationOperator(ctx context.Context, tx *sql.Tx, hash string, version int64, installationID, principalID string) error {
	a, err := currentSession(ctx, tx, hash, "", "")
	if err != nil {
		return ErrAuthorization
	}
	if a.InstallationID != installationID || a.PrincipalID != principalID || a.PrincipalID != a.OwnerPrincipalID || a.GrantVersion != version {
		return ErrAuthorization
	}
	return nil
}

const directoryOperationSelect = `SELECT o.installation_id,o.operation_id,o.principal_id,o.workspace_id,o.kind,o.target_locator,o.input_digest,o.phase,o.outcome,c.tenant_id,c.display_name,c.allowed_location,c.identity_digest,c.root_volume,c.root_file_id,c.database_file_id,c.descriptor_file_id FROM installation_operation o JOIN installation_creation_intent c USING(installation_id,operation_id) WHERE o.installation_id=? AND o.operation_id=?`

func readDirectoryOperation(ctx context.Context, q schemaReader, installationID, operationID string) (DirectoryOperation, error) {
	var o DirectoryOperation
	err := q.QueryRowContext(ctx, directoryOperationSelect, installationID, operationID).Scan(&o.InstallationID, &o.OperationID, &o.PrincipalID, &o.WorkspaceID, &o.Kind, &o.TargetLocator, &o.InputDigest, &o.Phase, &o.Outcome, &o.TenantID, &o.DisplayName, &o.AllowedLocation, &o.IdentityDigest, &o.RootVolume, &o.RootFileID, &o.DatabaseFileID, &o.DescriptorFileID)
	if err != nil {
		return DirectoryOperation{}, err
	}
	if !validCreationIntent(o) {
		return DirectoryOperation{}, ErrInstallationOperation
	}
	return o, nil
}

// ReadDirectoryOperation checks CURRENT operator and original actor in the same
// installation read transaction. It never inspects a directory or replays work.
func (d *Database) ReadDirectoryOperation(ctx context.Context, hash string, version int64, installationID, principalID, operationID string) (DirectoryOperation, error) {
	if d == nil || d.kind != InstallationDatabase || !validUUID(operationID) {
		return DirectoryOperation{}, ErrInstallationOperation
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return DirectoryOperation{}, err
	}
	var o DirectoryOperation
	if err = creationOperator(ctx, tx, hash, version, installationID, principalID); err == nil {
		o, err = readDirectoryOperation(ctx, tx, installationID, operationID)
		if err == nil && o.PrincipalID != principalID {
			err = ErrAuthorization
		}
	}
	err = errors.Join(err, tx.Rollback())
	if err != nil {
		return DirectoryOperation{}, err
	}
	return o, nil
}

// RecordCreationIntent freezes hash-covered fields AND displayName (which the
// unchanged INSTALL-OP/1 hash intentionally does not include). Known old keys
// are investigation-only; existing=true never authorizes a new FS execution.
func (d *Database) RecordCreationIntent(ctx context.Context, hash string, version int64, want DirectoryOperation) (out DirectoryOperation, existing bool, err error) {
	if !validCreationIntent(want) || want.Phase != string(IntentRecorded) || want.Outcome != string(InstallationAccepted) || want.RootVolume != "" || len(want.RootFileID) != 0 || len(want.DatabaseFileID) != 0 || len(want.DescriptorFileID) != 0 {
		return out, false, ErrInstallationOperation
	}
	err = d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		if err := creationOperator(ctx, tx, hash, version, want.InstallationID, want.PrincipalID); err != nil {
			return err
		}
		got, err := readDirectoryOperation(ctx, tx, want.InstallationID, want.OperationID)
		if err == nil {
			if !sameCreationIntent(got, want) {
				return ErrInstallationOperationConflict
			}
			out, existing = got, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var n int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM installation_operation WHERE installation_id=? AND operation_id=?`, want.InstallationID, want.OperationID).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			return ErrInstallationOperationConflict
		}
		// A workspace UUID or locator belongs to one original creation, not a retry
		// under a fresh operation ID. Residual rejected intents are not first use.
		if err = tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM installation_operation WHERE installation_id=? AND (workspace_id=? OR target_locator=?))+(SELECT count(*) FROM directory_registration WHERE workspace_id=? OR root_locator=?)`, want.InstallationID, want.WorkspaceID, want.TargetLocator, want.WorkspaceID, want.TargetLocator).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			return ErrInstallationOperationConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO installation_operation VALUES(?,?,?,?,?,?,?,?,?)`, want.InstallationID, want.OperationID, want.PrincipalID, want.WorkspaceID, want.Kind, want.TargetLocator, want.InputDigest, want.Phase, want.Outcome)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO installation_creation_intent(installation_id,operation_id,tenant_id,display_name,allowed_location,identity_digest) VALUES(?,?,?,?,?,?)`, want.InstallationID, want.OperationID, want.TenantID, want.DisplayName, want.AllowedLocation, want.IdentityDigest)
		out = want
		return err
	})
	return
}

func (d *Database) CheckpointCreation(ctx context.Context, hash string, version int64, want DirectoryOperation, next InstallationPhase) error {
	return d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		if err := creationOperator(ctx, tx, hash, version, want.InstallationID, want.PrincipalID); err != nil {
			return err
		}
		got, err := readDirectoryOperation(ctx, tx, want.InstallationID, want.OperationID)
		if err != nil {
			return err
		}
		if !sameCreationIntent(got, want) || got.Outcome != string(InstallationAccepted) {
			return ErrInstallationOperationConflict
		}
		switch next {
		case DirectoryPrepared:
			if got.Phase != string(IntentRecorded) || got.RootVolume != "" || want.RootVolume == "" || len(want.RootFileID) != 16 || len(want.DatabaseFileID) != 16 || len(want.DescriptorFileID) != 0 {
				return ErrInstallationOperation
			}
		case GenesisConfirmed:
			if got.Phase != string(DirectoryPrepared) || !sameCreationEvidence(got, want) {
				return ErrInstallationOperation
			}
		case DescriptorPublished:
			copyWant := want
			copyWant.DescriptorFileID = nil
			if got.Phase != string(GenesisConfirmed) || !sameCreationEvidence(got, copyWant) || len(want.DescriptorFileID) != 16 {
				return ErrInstallationOperation
			}
		default:
			return ErrInstallationOperation
		}
		if _, err = tx.ExecContext(ctx, `UPDATE installation_creation_intent SET root_volume=?,root_file_id=?,database_file_id=?,descriptor_file_id=? WHERE installation_id=? AND operation_id=?`, want.RootVolume, want.RootFileID, want.DatabaseFileID, want.DescriptorFileID, want.InstallationID, want.OperationID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE installation_operation SET phase=? WHERE installation_id=? AND operation_id=?`, string(next), want.InstallationID, want.OperationID)
		return err
	})
}

// InterruptCreation records only a known original operation. Its failure never
// turns a partial or unknown cross-directory operation into success or repair.
func (d *Database) InterruptCreation(ctx context.Context, hash string, version int64, want DirectoryOperation, unknown bool) error {
	return d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		if err := creationOperator(ctx, tx, hash, version, want.InstallationID, want.PrincipalID); err != nil {
			return err
		}
		got, err := readDirectoryOperation(ctx, tx, want.InstallationID, want.OperationID)
		if err != nil {
			return err
		}
		if !sameCreationIntent(got, want) || got.Outcome == string(InstallationCommitted) {
			return ErrInstallationOperationConflict
		}
		outcome := InstallationRejected
		if unknown || got.Outcome == string(InstallationUnknown) {
			outcome = InstallationUnknown
		}
		_, err = tx.ExecContext(ctx, `UPDATE installation_operation SET outcome=? WHERE installation_id=? AND operation_id=?`, string(outcome), want.InstallationID, want.OperationID)
		return err
	})
}

// FinalizeCreation is one short installation transaction AFTER the business
// owner proved exact original physical root/DB/descriptor/genesis under lease.
// It never touches files, creates missing data, or changes an existing binding.
// COMMIT uncertainty affects grants: callers MUST set shared authUncertain.
func (d *Database) FinalizeCreation(ctx context.Context, hash string, version int64, want DirectoryOperation, reconcile bool) error {
	return d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		if err := creationOperator(ctx, tx, hash, version, want.InstallationID, want.PrincipalID); err != nil {
			return err
		}
		got, err := readDirectoryOperation(ctx, tx, want.InstallationID, want.OperationID)
		if err != nil {
			return err
		}
		if !sameCreationIntent(got, want) || !sameCreationEvidence(got, want) || len(got.RootFileID) != 16 || len(got.DatabaseFileID) != 16 || len(got.DescriptorFileID) != 16 || got.RootVolume == "" {
			return ErrInstallationOperationConflict
		}
		if got.Outcome == string(InstallationCommitted) {
			if got.Phase != string(RegistrationConfirmed) {
				return ErrInstallationOperation
			}
			return verifyCreationRegistration(ctx, tx, got)
		}
		if got.Phase != string(DescriptorPublished) || (!reconcile && got.Outcome != string(InstallationAccepted)) {
			return ErrInstallationOperation
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO directory_registration VALUES(?,?,?,?,?,?,1,'registered')`, got.WorkspaceID, got.TenantID, got.TargetLocator, got.AllowedLocation, got.RootVolume, got.RootFileID); err != nil {
			return err
		}
		for _, action := range []string{"workspace.read", "workspace.write", "workspace.open", "workspace.close", "workspace.operator"} {
			if _, err = tx.ExecContext(ctx, `INSERT INTO grant_entry VALUES(?,?,?,?)`, got.PrincipalID, got.TenantID, got.WorkspaceID, action); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE installation_operation SET phase='registrationConfirmed',outcome='committed' WHERE installation_id=? AND operation_id=?`, got.InstallationID, got.OperationID)
		return err
	})
}
func verifyCreationRegistration(ctx context.Context, tx *sql.Tx, o DirectoryOperation) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM directory_registration WHERE workspace_id=? AND tenant_id=? AND root_locator=? AND allowed_location=? AND volume_identity=? AND root_file_id=? AND registration_version=1 AND state='registered'`, o.WorkspaceID, o.TenantID, o.TargetLocator, o.AllowedLocation, o.RootVolume, o.RootFileID).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return ErrInstallationOperation
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM grant_entry WHERE principal_id=? AND tenant_id=? AND workspace_id=? AND action IN ('workspace.read','workspace.write','workspace.open','workspace.close','workspace.operator')`, o.PrincipalID, o.TenantID, o.WorkspaceID).Scan(&n); err != nil {
		return err
	}
	if n != 5 {
		return ErrInstallationOperation
	}
	return nil
}
