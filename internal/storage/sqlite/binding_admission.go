package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// RegisteredDirectory is installation-owned location evidence, not a grant.
type RegisteredDirectory struct {
	WorkspaceID, TenantID, RootLocator, AllowedLocation, VolumeIdentity string
	RootFileID                                                          []byte
	RegistrationVersion                                                 int64
}

func (d *Database) ReadRegisteredDirectory(ctx context.Context, hash string, version int64, installationID, principalID, workspaceID string) (out RegisteredDirectory, err error) {
	if d == nil || d.kind != InstallationDatabase || !validUUID(workspaceID) {
		return out, ErrAuthorization
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer func() {
		err = errors.Join(err, tx.Rollback())
		if err != nil {
			out = RegisteredDirectory{}
		}
	}()
	a, e := currentSession(ctx, tx, hash, "", "")
	if e != nil || a.InstallationID != installationID || a.PrincipalID != principalID || a.GrantVersion != version {
		return out, ErrAuthorization
	}
	if a.PrincipalID != a.OwnerPrincipalID {
		if _, e = currentSession(ctx, tx, hash, workspaceID, "workspace.operator"); e != nil {
			return out, ErrAuthorization
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT workspace_id,tenant_id,root_locator,allowed_location,volume_identity,root_file_id,registration_version FROM directory_registration WHERE workspace_id=? AND tenant_id=? AND state='registered'`, workspaceID, a.TenantID).Scan(&out.WorkspaceID, &out.TenantID, &out.RootLocator, &out.AllowedLocation, &out.VolumeIdentity, &out.RootFileID, &out.RegistrationVersion)
	return
}

// ValidateBindingReadiness reads an exclusively frozen source. Accepted/unknown
// saves after a cold start are NOT proof of drained/known and cannot be relinked.
func (d *Database) ValidateBindingReadiness(ctx context.Context, descriptor workspace.DirectoryDescriptor) error {
	if err := d.ValidateDirectoryDescriptor(ctx, descriptor); err != nil {
		return err
	}
	var state string
	var pending int
	if err := d.h.Conn.QueryRowContext(ctx, `SELECT business_state FROM workspace_identity WHERE singleton=1`).Scan(&state); err != nil {
		return err
	}
	if state != "ready" {
		return ErrSchema
	}
	if err := d.h.Conn.QueryRowContext(ctx, `SELECT count(*) FROM save_operation WHERE state IN ('accepted','unknown')`).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 {
		return ErrUnknown
	}
	return nil
}
