package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// ValidateDirectoryTenant reads identity metadata only, before callers diagnose
// a schema or business history. It never grants permission from a descriptor.
func ValidateDirectoryTenant(ctx context.Context, conn *sql.Conn, expectedTenant string) error {
	if conn == nil {
		return ErrSchema
	}
	var tenant string
	if err := conn.QueryRowContext(ctx, "SELECT tenant_id FROM workspace_identity WHERE singleton=1").Scan(&tenant); err != nil {
		return errors.Join(ErrSchema, err)
	}
	if tenant != expectedTenant {
		return errors.Join(ErrSchema, workspace.ErrDirectoryTenantMismatch)
	}
	return nil
}

// ValidateDirectorySnapshot checks an already frozen, read-only scratch view or
// an exclusively owned database. It grants no filesystem/tenant permission and
// never opens, initializes, migrates or repairs a path.
func ValidateDirectorySnapshot(ctx context.Context, conn *sql.Conn, descriptor workspace.DirectoryDescriptor) (err error) {
	if conn == nil || descriptor.Validate() != nil {
		return ErrSchema
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return errors.Join(ErrSchema, err)
	}
	defer func() { err = errors.Join(err, tx.Rollback()) }()
	var workspaceID, tenantID, identityDigest string
	var formatVersion int
	err = tx.QueryRowContext(ctx, "SELECT workspace_id,tenant_id,format_version,identity_digest FROM workspace_identity WHERE singleton=1 AND schema_version=1").Scan(&workspaceID, &tenantID, &formatVersion, &identityDigest)
	if err != nil {
		return errors.Join(ErrSchema, err)
	}
	if tenantID != descriptor.TenantID {
		return errors.Join(ErrSchema, workspace.ErrDirectoryTenantMismatch)
	}
	if workspaceID != descriptor.WorkspaceID || formatVersion != descriptor.FormatVersion {
		return errors.Join(ErrSchema, workspace.ErrDirectoryIdentityMismatch)
	}
	if identityDigest != descriptor.IdentityDigest {
		return errors.Join(ErrSchema, workspace.ErrDirectoryDigestMismatch)
	}
	return verifySchema(ctx, tx, WorkspaceDatabase)
}

func (d *Database) ValidateDirectoryDescriptor(ctx context.Context, descriptor workspace.DirectoryDescriptor) error {
	if d == nil || d.kind != WorkspaceDatabase {
		return ErrSchema
	}
	return ValidateDirectorySnapshot(ctx, d.h.Conn, descriptor)
}
