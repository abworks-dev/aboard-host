package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// BindingOperation freezes original registration/relink input and physical
// evidence. It is private storage data, not authority to access a directory.
// Filesystem/lifecycle proofs belong to the business owner and MUST remain held
// until this transaction and physical cleanup complete. No method here does IO.
type BindingOperation struct {
	InstallationOperation
	TenantID, AllowedLocation, IdentityDigest, RootVolume string
	RootFileID, DatabaseFileID, DescriptorFileID          []byte
	PreviousLocator                                       string
	ExpectedRegistrationVersion                           int64
}

func validBinding(o BindingOperation) bool {
	if o.Kind != "register" && o.Kind != "relink" {
		return false
	}
	// A contradictory journal is not evidence of a completed registration.
	if o.Outcome == string(InstallationCommitted) {
		if o.Phase != string(RegistrationConfirmed) {
			return false
		}
	} else if o.Phase != string(IntentRecorded) || (o.Outcome != string(InstallationAccepted) && o.Outcome != string(InstallationUnknown) && o.Outcome != string(InstallationRejected)) {
		return false
	}
	h, err := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(o.InstallationID), PrincipalID: protocol.UUID(o.PrincipalID), OperationID: protocol.UUID(o.OperationID), Kind: o.Kind, WorkspaceID: protocol.UUID(o.WorkspaceID), CanonicalLocator: o.TargetLocator})
	identity, idErr := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID(o.WorkspaceID), TenantID: protocol.UUID(o.TenantID), DatabaseRelativePath: "data/workspace.sqlite"})
	return err == nil && idErr == nil && o.InputDigest == string(h) && o.IdentityDigest == string(identity) && o.AllowedLocation != "" && o.RootVolume != "" && len(o.RootFileID) == 16 && len(o.DatabaseFileID) == 16 && len(o.DescriptorFileID) == 16 && o.ExpectedRegistrationVersion >= 0 && o.ExpectedRegistrationVersion < math.MaxInt64 && ((o.Kind == "register" && o.PreviousLocator == "") || (o.Kind == "relink" && o.PreviousLocator != "" && o.PreviousLocator != o.TargetLocator && o.ExpectedRegistrationVersion > 0))
}
func sameBinding(a, b BindingOperation) bool {
	return a.InstallationID == b.InstallationID && a.OperationID == b.OperationID && a.PrincipalID == b.PrincipalID && a.WorkspaceID == b.WorkspaceID && a.Kind == b.Kind && a.TargetLocator == b.TargetLocator && a.InputDigest == b.InputDigest && a.TenantID == b.TenantID && a.AllowedLocation == b.AllowedLocation && a.IdentityDigest == b.IdentityDigest && a.RootVolume == b.RootVolume && bytes.Equal(a.RootFileID, b.RootFileID) && bytes.Equal(a.DatabaseFileID, b.DatabaseFileID) && bytes.Equal(a.DescriptorFileID, b.DescriptorFileID) && a.PreviousLocator == b.PreviousLocator && a.ExpectedRegistrationVersion == b.ExpectedRegistrationVersion
}
func bindingActor(ctx context.Context, tx *sql.Tx, hash string, version int64, o BindingOperation) error {
	workspaceID, action := "", ""
	if o.Kind == "relink" {
		workspaceID, action = o.WorkspaceID, "workspace.operator"
	}
	a, err := currentSession(ctx, tx, hash, workspaceID, action)
	if err != nil || a.InstallationID != o.InstallationID || a.TenantID != o.TenantID || a.PrincipalID != o.PrincipalID || a.GrantVersion != version || (o.Kind == "register" && a.PrincipalID != a.OwnerPrincipalID) {
		return ErrAuthorization
	}
	return nil
}
func readBinding(ctx context.Context, q schemaReader, installationID, operationID string) (BindingOperation, error) {
	var o BindingOperation
	err := q.QueryRowContext(ctx, `SELECT o.installation_id,o.operation_id,o.principal_id,o.workspace_id,o.kind,o.target_locator,o.input_digest,o.phase,o.outcome,b.tenant_id,b.allowed_location,b.identity_digest,b.root_volume,b.root_file_id,b.database_file_id,b.descriptor_file_id,b.previous_locator,b.expected_registration_version FROM installation_operation o JOIN installation_binding_intent b USING(installation_id,operation_id) WHERE o.installation_id=? AND o.operation_id=?`, installationID, operationID).Scan(&o.InstallationID, &o.OperationID, &o.PrincipalID, &o.WorkspaceID, &o.Kind, &o.TargetLocator, &o.InputDigest, &o.Phase, &o.Outcome, &o.TenantID, &o.AllowedLocation, &o.IdentityDigest, &o.RootVolume, &o.RootFileID, &o.DatabaseFileID, &o.DescriptorFileID, &o.PreviousLocator, &o.ExpectedRegistrationVersion)
	if err != nil {
		return BindingOperation{}, err
	}
	if !validBinding(o) {
		return BindingOperation{}, ErrInstallationOperation
	}
	return o, nil
}

// ReadBindingOperation checks the current original actor in the same snapshot.
func (d *Database) ReadBindingOperation(ctx context.Context, hash string, version int64, installationID, principalID, operationID string) (BindingOperation, error) {
	return d.readAuthorizedBinding(ctx, hash, version, installationID, principalID, operationID, false)
}

// ReadBindingInvestigation requires the endpoint's installation.operator, not a workspace action.
func (d *Database) ReadBindingInvestigation(ctx context.Context, hash string, version int64, installationID, principalID, operationID string) (BindingOperation, error) {
	return d.readAuthorizedBinding(ctx, hash, version, installationID, principalID, operationID, true)
}
func (d *Database) readAuthorizedBinding(ctx context.Context, hash string, version int64, installationID, principalID, operationID string, investigation bool) (out BindingOperation, err error) {
	if d == nil || d.kind != InstallationDatabase || !validUUID(installationID) || !validUUID(principalID) || !validUUID(operationID) {
		return out, ErrInstallationOperation
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer func() {
		err = errors.Join(err, tx.Rollback())
		if err != nil {
			out = BindingOperation{}
		}
	}()
	// Authenticate before looking up the requested fact; relink needs its per-zone
	// action checked again after lookup, rather than an owner-role substitution.
	a, e := currentSession(ctx, tx, hash, "", "")
	if e != nil || a.InstallationID != installationID || a.PrincipalID != principalID || a.GrantVersion != version {
		return out, ErrAuthorization
	}
	if investigation && a.PrincipalID != a.OwnerPrincipalID {
		return out, ErrAuthorization
	}
	out, err = readBinding(ctx, tx, installationID, operationID)
	if err != nil {
		return out, err
	}
	if out.PrincipalID != principalID {
		return out, ErrAuthorization
	}
	if out.TenantID != a.TenantID {
		return BindingOperation{}, ErrAuthorization
	}
	if !investigation {
		err = bindingActor(ctx, tx, hash, version, out)
	}
	return
}

// RecordBindingIntent never replays an old key. Non-hash-covered expected
// version, physical evidence and old locator are also frozen and compared.
func (d *Database) RecordBindingIntent(ctx context.Context, hash string, version int64, want BindingOperation) (out BindingOperation, existing bool, err error) {
	if !validBinding(want) || want.Phase != string(IntentRecorded) || want.Outcome != string(InstallationAccepted) {
		return out, false, ErrInstallationOperation
	}
	err = d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		if err := bindingActor(ctx, tx, hash, version, want); err != nil {
			return err
		}
		old, err := readBinding(ctx, tx, want.InstallationID, want.OperationID)
		if err == nil {
			if !sameBinding(old, want) {
				return ErrInstallationOperationConflict
			}
			out, existing = old, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM installation_operation WHERE installation_id=? AND (operation_id=? OR ((workspace_id=? OR target_locator=?) AND outcome!='committed'))`, want.InstallationID, want.OperationID, want.WorkspaceID, want.TargetLocator).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return ErrInstallationOperationConflict
		}
		if err = checkBindingBase(ctx, tx, want); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO installation_operation VALUES(?,?,?,?,?,?,?,?,?)`, want.InstallationID, want.OperationID, want.PrincipalID, want.WorkspaceID, want.Kind, want.TargetLocator, want.InputDigest, want.Phase, want.Outcome); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO installation_binding_intent VALUES(?,?,?,?,?,?,?,?,?,?,?)`, want.InstallationID, want.OperationID, want.TenantID, want.AllowedLocation, want.IdentityDigest, want.RootVolume, want.RootFileID, want.DatabaseFileID, want.DescriptorFileID, want.PreviousLocator, want.ExpectedRegistrationVersion)
		out = want
		return err
	})
	return
}
func checkBindingBase(ctx context.Context, tx *sql.Tx, o BindingOperation) error {
	var tenant, locator, volume string
	var root []byte
	var version int64
	err := tx.QueryRowContext(ctx, `SELECT tenant_id,root_locator,volume_identity,root_file_id,registration_version FROM directory_registration WHERE workspace_id=? AND state='registered'`, o.WorkspaceID).Scan(&tenant, &locator, &volume, &root, &version)
	if errors.Is(err, sql.ErrNoRows) && o.Kind == "register" && o.ExpectedRegistrationVersion == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	expectedLocator := o.TargetLocator
	if o.Kind == "relink" {
		expectedLocator = o.PreviousLocator
	}
	if tenant != o.TenantID || locator != expectedLocator || volume != o.RootVolume || !bytes.Equal(root, o.RootFileID) || version != o.ExpectedRegistrationVersion {
		return ErrInstallationOperationConflict
	}
	return nil
}

// FinalizeBinding atomically changes only locator facts and the original
// operation outcome. It never imports grants from a directory or changes IDs.
// reconcile=true is an explicit business command with fresh physical proofs,
// never a retry triggered by absence, timeout or an unknown COMMIT.
func (d *Database) FinalizeBinding(ctx context.Context, hash string, version int64, want BindingOperation, reconcile bool) error {
	if !validBinding(want) {
		return ErrInstallationOperation
	}
	return d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		if reconcile {
			a, e := currentSession(ctx, tx, hash, "", "")
			if e != nil || a.PrincipalID != a.OwnerPrincipalID || a.PrincipalID != want.PrincipalID || a.InstallationID != want.InstallationID || a.TenantID != want.TenantID || a.GrantVersion != version {
				return ErrAuthorization
			}
		}
		if err := bindingActor(ctx, tx, hash, version, want); err != nil {
			return err
		}
		got, err := readBinding(ctx, tx, want.InstallationID, want.OperationID)
		if err != nil {
			return err
		}
		if !sameBinding(got, want) {
			return ErrInstallationOperationConflict
		}
		if got.Outcome == string(InstallationCommitted) {
			return verifyBindingFinal(ctx, tx, got)
		}
		if got.Phase != string(IntentRecorded) || (!reconcile && got.Outcome != string(InstallationAccepted)) {
			return ErrInstallationOperation
		}
		if err = checkBindingBase(ctx, tx, got); err != nil {
			return err
		}
		if got.Kind == "register" && got.ExpectedRegistrationVersion == 0 {
			_, err = tx.ExecContext(ctx, `INSERT INTO directory_registration VALUES(?,?,?,?,?,?,1,'registered')`, got.WorkspaceID, got.TenantID, got.TargetLocator, got.AllowedLocation, got.RootVolume, got.RootFileID)
		} else if got.Kind == "relink" {
			var result sql.Result
			result, err = tx.ExecContext(ctx, `UPDATE directory_registration SET root_locator=?,allowed_location=?,registration_version=registration_version+1 WHERE workspace_id=? AND tenant_id=? AND root_locator=? AND registration_version=? AND state='registered'`, got.TargetLocator, got.AllowedLocation, got.WorkspaceID, got.TenantID, got.PreviousLocator, got.ExpectedRegistrationVersion)
			if err == nil {
				n, e := result.RowsAffected()
				if e != nil {
					return e
				}
				if n != 1 {
					return ErrInstallationOperationConflict
				}
			}
		}
		if err != nil {
			return err
		}
		if err = verifyBindingFinal(ctx, tx, got); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE installation_operation SET phase='registrationConfirmed',outcome='committed' WHERE installation_id=? AND operation_id=?`, got.InstallationID, got.OperationID)
		return err
	})
}
func verifyBindingFinal(ctx context.Context, tx *sql.Tx, o BindingOperation) error {
	v := o.ExpectedRegistrationVersion
	if o.Kind == "relink" {
		v++
	} else if v == 0 {
		v = 1
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM directory_registration WHERE workspace_id=? AND tenant_id=? AND root_locator=? AND allowed_location=? AND volume_identity=? AND root_file_id=? AND registration_version=? AND state='registered'`, o.WorkspaceID, o.TenantID, o.TargetLocator, o.AllowedLocation, o.RootVolume, o.RootFileID, v).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return ErrInstallationOperationConflict
	}
	return nil
}
