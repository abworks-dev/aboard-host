package sqlite

import (
	"context"
	"database/sql"
	"errors"
)

// InstallationIdentity is private adapter data, never a transport DTO. The
// installation database is its sole authority; DPAPI material must match it.
type InstallationIdentity struct {
	InstallationID, TenantID, OwnerPrincipalID string
	WindowsSID, SecretReference                string
	SchemaVersion                              int
}

func (d *Database) ReadInstallationIdentity(ctx context.Context) (InstallationIdentity, error) {
	var id InstallationIdentity
	if d == nil || d.kind != InstallationDatabase {
		return id, ErrSchema
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return id, err
	}
	if err = verifySchema(ctx, tx, InstallationDatabase); err == nil {
		err = tx.QueryRowContext(ctx, "SELECT installation_id,tenant_id,owner_principal_id,windows_sid,secret_reference,schema_version FROM installation_identity WHERE singleton=1").Scan(&id.InstallationID, &id.TenantID, &id.OwnerPrincipalID, &id.WindowsSID, &id.SecretReference, &id.SchemaVersion)
	}
	err = errors.Join(err, tx.Rollback())
	if err != nil {
		return InstallationIdentity{}, err
	}
	return id, nil
}

// InstallationAuthorization samples the current database facts in one read
// transaction. It is not an enduring permission, session, or execution lease.
// Callers must check a session's version and re-read at the business boundary.
type InstallationAuthorization struct {
	TenantID, PrincipalID     string
	GrantVersion              int64
	Revoked, WorkspaceAllowed bool
}

func (d *Database) CurrentInstallationAuthorization(ctx context.Context, principalID, workspaceID, action string) (InstallationAuthorization, error) {
	var result InstallationAuthorization
	if d == nil || d.kind != InstallationDatabase || !validUUID(principalID) || (workspaceID != "" && !validUUID(workspaceID)) || (workspaceID == "") != (action == "") {
		return result, ErrSchema
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	var revoked int
	err = tx.QueryRowContext(ctx, "SELECT p.tenant_id,p.principal_id,p.grant_version,p.revoked FROM principal p JOIN installation_identity i ON i.tenant_id=p.tenant_id WHERE i.singleton=1 AND p.principal_id=?", principalID).Scan(&result.TenantID, &result.PrincipalID, &result.GrantVersion, &revoked)
	result.Revoked = revoked != 0
	if err == nil && workspaceID != "" {
		var count int
		err = tx.QueryRowContext(ctx, "SELECT count(*) FROM grant_entry g JOIN directory_registration r ON r.workspace_id=g.workspace_id AND r.tenant_id=g.tenant_id WHERE g.principal_id=? AND g.tenant_id=? AND g.workspace_id=? AND g.action=? AND r.state='registered'", principalID, result.TenantID, workspaceID, action).Scan(&count)
		result.WorkspaceAllowed = count == 1 && !result.Revoked
	}
	err = errors.Join(err, tx.Rollback())
	if err != nil {
		return InstallationAuthorization{}, err
	}
	return result, nil
}

// InstallationOperation retains installation-scoped coordination facts only.
// Reading an old operation never grants current authorization or replays it.
type InstallationOperation struct {
	InstallationID, OperationID, PrincipalID, WorkspaceID string
	Kind, TargetLocator, InputDigest, Phase, Outcome      string
}

func (d *Database) ReadInstallationOperation(ctx context.Context, installationID, operationID string) (InstallationOperation, error) {
	var result InstallationOperation
	if d == nil || d.kind != InstallationDatabase || !validUUID(installationID) || !validUUID(operationID) {
		return result, ErrSchema
	}
	return readInstallationOperation(ctx, d.h.Conn, installationID, operationID)
}

func readInstallationOperation(ctx context.Context, q schemaReader, installationID, operationID string) (InstallationOperation, error) {
	var result InstallationOperation
	err := q.QueryRowContext(ctx, "SELECT o.installation_id,o.operation_id,o.principal_id,o.workspace_id,o.kind,o.target_locator,o.input_digest,o.phase,o.outcome FROM installation_operation o JOIN installation_identity i ON i.installation_id=o.installation_id WHERE i.singleton=1 AND o.installation_id=? AND o.operation_id=?", installationID, operationID).Scan(&result.InstallationID, &result.OperationID, &result.PrincipalID, &result.WorkspaceID, &result.Kind, &result.TargetLocator, &result.InputDigest, &result.Phase, &result.Outcome)
	if err != nil {
		return InstallationOperation{}, err
	}
	return result, nil
}
