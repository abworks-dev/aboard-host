package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"time"
)

// ErrAuthorization deliberately contains no target, token, SQL or secret.
var ErrAuthorization = errors.New("current installation authorization denied")

// WorkspaceGrant is a current explicit permission, never an installation role.
type WorkspaceGrant struct{ WorkspaceID, Action string }

func ValidWorkspaceAction(action string) bool {
	switch action {
	case "workspace.read", "workspace.write", "workspace.open", "workspace.close", "workspace.operator":
		return true
	}
	return false
}

// SessionAuthorization is a single fresh installation snapshot, not a permission
// cache. Registration facts are returned only for a granted, registered target.
type SessionAuthorization struct {
	InstallationID, TenantID, PrincipalID, OwnerPrincipalID, SessionID string
	GrantVersion                                                       int64
	RootLocator, AllowedLocation, VolumeIdentity                       string
	RootFileID                                                         []byte
	RegistrationVersion                                                int64
}

func validTokenHash(hash string) bool {
	b, err := hex.DecodeString(hash)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == hash
}

func currentSession(ctx context.Context, tx *sql.Tx, hash, workspaceID, action string) (SessionAuthorization, error) {
	var out SessionAuthorization
	if !validTokenHash(hash) || (workspaceID == "") != (action == "") || (workspaceID != "" && (!validUUID(workspaceID) || !ValidWorkspaceAction(action))) {
		return out, ErrAuthorization
	}
	var expiry string
	var sessionVersion int64
	var revoked, principalRevoked int
	err := tx.QueryRowContext(ctx, `SELECT i.installation_id,i.tenant_id,p.principal_id,i.owner_principal_id,s.session_id,s.grant_version,p.grant_version,s.expires_at,s.revoked,p.revoked FROM client_session s JOIN principal p ON p.principal_id=s.principal_id JOIN installation_identity i ON i.tenant_id=p.tenant_id WHERE i.singleton=1 AND s.token_hash=?`, hash).Scan(&out.InstallationID, &out.TenantID, &out.PrincipalID, &out.OwnerPrincipalID, &out.SessionID, &sessionVersion, &out.GrantVersion, &expiry, &revoked, &principalRevoked)
	if err != nil {
		return SessionAuthorization{}, err
	}
	expires, err := time.Parse(time.RFC3339Nano, expiry)
	if err != nil || revoked != 0 || principalRevoked != 0 || sessionVersion != out.GrantVersion || out.GrantVersion <= 0 || !time.Now().Before(expires) {
		return SessionAuthorization{}, ErrAuthorization
	}
	if workspaceID != "" {
		err = tx.QueryRowContext(ctx, `SELECT r.root_locator,r.allowed_location,r.volume_identity,r.root_file_id,r.registration_version FROM grant_entry g JOIN directory_registration r ON r.workspace_id=g.workspace_id AND r.tenant_id=g.tenant_id WHERE g.principal_id=? AND g.tenant_id=? AND g.workspace_id=? AND g.action=? AND r.state='registered'`, out.PrincipalID, out.TenantID, workspaceID, action).Scan(&out.RootLocator, &out.AllowedLocation, &out.VolumeIdentity, &out.RootFileID, &out.RegistrationVersion)
		if err != nil {
			return SessionAuthorization{}, err
		}
	}
	return out, nil
}

func (d *Database) CurrentSessionAuthorization(ctx context.Context, hash, workspaceID, action string) (SessionAuthorization, error) {
	if d == nil || d.kind != InstallationDatabase {
		return SessionAuthorization{}, ErrAuthorization
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SessionAuthorization{}, err
	}
	out, err := currentSession(ctx, tx, hash, workspaceID, action)
	err = errors.Join(err, tx.Rollback())
	if err != nil {
		return SessionAuthorization{}, err
	}
	return out, nil
}

// installationAuthWrite never retries. A proved pre-COMMIT rollback is known;
// every COMMIT error (including driver rollback) is unknown, not replayable.
func (d *Database) installationAuthWrite(ctx context.Context, work func(*sql.Tx) error) error {
	if d == nil || d.kind != InstallationDatabase {
		return ErrAuthorization
	}
	tx, err := d.h.Conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err = work(tx); err != nil {
		if endErr := tx.Rollback(); endErr != nil {
			return errors.Join(ErrUnknown, err, endErr)
		}
		return err
	}
	if err = tx.Commit(); err != nil {
		return errors.Join(ErrUnknown, err)
	}
	return nil
}

// SessionDelivery is the exact pending row issued by one live authenticated pipe.
// Only its private issuer may retain it for ACK activation; it is not a bearer,
// public DTO, or an authorization capability for arbitrary session unrevocation.
type SessionDelivery struct {
	SessionID, TokenHash, PrincipalID, WorkspaceID, Action, ExpiresAt string
	GrantVersion                                                      int64
}

// CreateInstallationSession persists an unusable pending session AFTER the pipe
// authenticated its peer. A crash before ACK leaves revoked=1 on cold reopen.
func (d *Database) CreateInstallationSession(ctx context.Context, ownerID, principalID, sessionID, hash string, expires time.Time, workspaceID, action string) (SessionDelivery, error) {
	if !validUUID(ownerID) || !validUUID(principalID) || !validUUID(sessionID) || !validTokenHash(hash) || !expires.After(time.Now()) || expires.After(time.Now().Add(5*time.Minute)) {
		return SessionDelivery{}, ErrAuthorization
	}
	out := SessionDelivery{SessionID: sessionID, TokenHash: hash, PrincipalID: principalID, WorkspaceID: workspaceID, Action: action, ExpiresAt: expires.UTC().Format(time.RFC3339Nano)}
	err := d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		version, err := deliveryPermission(ctx, tx, ownerID, principalID, workspaceID, action)
		if err != nil {
			return err
		}
		out.GrantVersion = version
		_, err = tx.ExecContext(ctx, `INSERT INTO client_session(session_id,token_hash,principal_id,expires_at,revoked,grant_version) VALUES(?,?,?,?,1,?)`, sessionID, hash, principalID, out.ExpiresAt, version)
		return err
	})
	if err != nil {
		return SessionDelivery{}, err
	}
	return out, nil
}

func deliveryPermission(ctx context.Context, tx *sql.Tx, ownerID, principalID, workspaceID, action string) (int64, error) {
	var tenant string
	var ownerCount int
	if err := tx.QueryRowContext(ctx, `SELECT i.tenant_id,count(*) FROM installation_identity i JOIN principal p ON p.principal_id=i.owner_principal_id AND p.tenant_id=i.tenant_id WHERE i.singleton=1 AND i.owner_principal_id=? AND p.revoked=0`, ownerID).Scan(&tenant, &ownerCount); err != nil {
		return 0, err
	}
	if ownerCount != 1 {
		return 0, ErrAuthorization
	}
	var version int64
	if err := tx.QueryRowContext(ctx, `SELECT grant_version FROM principal WHERE principal_id=? AND tenant_id=? AND revoked=0`, principalID, tenant).Scan(&version); err != nil {
		return 0, err
	}
	if version <= 0 {
		return 0, ErrAuthorization
	}
	if workspaceID == "" {
		if action != "installation.operator" || principalID != ownerID {
			return 0, ErrAuthorization
		}
	} else {
		if !validUUID(workspaceID) || !ValidWorkspaceAction(action) {
			return 0, ErrAuthorization
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM grant_entry g JOIN directory_registration r ON r.workspace_id=g.workspace_id AND r.tenant_id=g.tenant_id WHERE g.principal_id=? AND g.tenant_id=? AND g.workspace_id=? AND g.action=? AND r.state='registered'`, principalID, tenant, workspaceID, action).Scan(&count); err != nil {
			return 0, err
		}
		if count != 1 {
			return 0, ErrAuthorization
		}
	}
	return version, nil
}

// ActivateInstallationSession is ACK activation of precisely one retained pending
// delivery. It is not a general session-unrevoke operation. Terminal revocation
// also expires the row, so a delayed ACK can never resurrect that row.
func (d *Database) ActivateInstallationSession(ctx context.Context, ownerID string, pending SessionDelivery) error {
	expires, err := time.Parse(time.RFC3339Nano, pending.ExpiresAt)
	if err != nil || !expires.After(time.Now()) || !validUUID(ownerID) || !validUUID(pending.PrincipalID) || !validUUID(pending.SessionID) || !validTokenHash(pending.TokenHash) || pending.GrantVersion <= 0 {
		return ErrAuthorization
	}
	return d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		version, err := deliveryPermission(ctx, tx, ownerID, pending.PrincipalID, pending.WorkspaceID, pending.Action)
		if err != nil {
			return err
		}
		if version != pending.GrantVersion {
			return ErrAuthorization
		}
		result, err := tx.ExecContext(ctx, `UPDATE client_session SET revoked=0 WHERE session_id=? AND token_hash=? AND principal_id=? AND expires_at=? AND grant_version=? AND revoked=1`, pending.SessionID, pending.TokenHash, pending.PrincipalID, pending.ExpiresAt, pending.GrantVersion)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrAuthorization
		}
		return nil
	})
}

// ReplacePrincipalAuthorization is a typed, atomic replacement. Incrementing the
// live version invalidates every older session, including a formerly valid one.
func (d *Database) ReplacePrincipalAuthorization(ctx context.Context, actorHash string, actorVersion int64, principalID string, revoked bool, grants []WorkspaceGrant) error {
	if !validUUID(principalID) {
		return ErrAuthorization
	}
	for _, g := range grants {
		if !validUUID(g.WorkspaceID) || !ValidWorkspaceAction(g.Action) {
			return ErrAuthorization
		}
	}
	return d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		actor, err := currentSession(ctx, tx, actorHash, "", "")
		if err != nil {
			return err
		}
		if actor.PrincipalID != actor.OwnerPrincipalID || actor.GrantVersion != actorVersion {
			return ErrAuthorization
		}
		var version int64
		err = tx.QueryRowContext(ctx, `SELECT grant_version FROM principal WHERE principal_id=? AND tenant_id=?`, principalID, actor.TenantID).Scan(&version)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, `INSERT INTO principal(principal_id,tenant_id,grant_version,revoked) VALUES(?,?,1,?)`, principalID, actor.TenantID, revoked)
		} else if err == nil {
			if version == math.MaxInt64 {
				return ErrAuthorization
			}
			_, err = tx.ExecContext(ctx, `UPDATE principal SET grant_version=grant_version+1,revoked=? WHERE principal_id=? AND tenant_id=?`, revoked, principalID, actor.TenantID)
		}
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM grant_entry WHERE principal_id=? AND tenant_id=?`, principalID, actor.TenantID); err != nil {
			return err
		}
		for _, g := range grants {
			var count int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM directory_registration WHERE workspace_id=? AND tenant_id=? AND state='registered'`, g.WorkspaceID, actor.TenantID).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return ErrAuthorization
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO grant_entry(principal_id,tenant_id,workspace_id,action) VALUES(?,?,?,?)`, principalID, actor.TenantID, g.WorkspaceID, g.Action); err != nil {
				return err
			}
		}
		return nil
	})
}

// RevokeInstallationSession can only revoke the current subject's own session or
// an owner's explicitly selected session. Delivery-failure revocation uses the
// separate hash-only path below, not a forged owner action.
func (d *Database) RevokeInstallationSession(ctx context.Context, actorHash string, actorVersion int64, targetHash string) error {
	if !validTokenHash(targetHash) {
		return ErrAuthorization
	}
	return d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		actor, err := currentSession(ctx, tx, actorHash, "", "")
		if err != nil {
			return err
		}
		if actor.GrantVersion != actorVersion || (actorHash != targetHash && actor.PrincipalID != actor.OwnerPrincipalID) {
			return ErrAuthorization
		}
		_, err = tx.ExecContext(ctx, `UPDATE client_session SET revoked=1,expires_at='1970-01-01T00:00:00Z' WHERE token_hash=?`, targetHash)
		return err
	})
}

// RevokeUndeliveredSession is only for the authenticated pipe's failed ACK path.
func (d *Database) RevokeUndeliveredSession(ctx context.Context, hash string) error {
	if !validTokenHash(hash) {
		return ErrAuthorization
	}
	return d.installationAuthWrite(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE client_session SET revoked=1,expires_at='1970-01-01T00:00:00Z' WHERE token_hash=?`, hash)
		return err
	})
}
