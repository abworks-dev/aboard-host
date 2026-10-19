package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// Only these ordered, compiled-in inputs can initialize a new database. This is
// not an existing-database upgrade engine. Changing SQL changes its ledger hash.
//
//go:embed migrations/workspace_001.sql migrations/installation_001.sql
var schemaFiles embed.FS

type DatabaseKind string

const (
	WorkspaceDatabase    DatabaseKind = "workspace"
	InstallationDatabase DatabaseKind = "installation"
)

var (
	ErrSchema  = errors.New("incomplete or incompatible database schema/history")
	ErrUnknown = errors.New("persistence result unknown; investigate original key without replay")
)

type migration struct {
	sequence, version   int
	name, sql, checksum string
}

func migrations(kind DatabaseKind) ([]migration, error) {
	var name string
	switch kind {
	case WorkspaceDatabase:
		name = "migrations/workspace_001.sql"
	case InstallationDatabase:
		name = "migrations/installation_001.sql"
	default:
		return nil, ErrSchema
	}
	b, err := schemaFiles.ReadFile(name)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	return []migration{{1, 1, name, string(b), hex.EncodeToString(sum[:])}}, nil
}

// Genesis is supplied by the installation/directory owner, never an HTTP body.
// Existing-open uses only Kind and verifies; it NEVER seeds supplied identities.
// Root/ACL/OS lease stability and current authorization remain caller duties.
type Genesis struct {
	Kind                                                    DatabaseKind
	InstallationID, WorkspaceID, TenantID, OwnerPrincipalID string
	WindowsSID, SecretReference, DisplayName                string
}
type Database struct {
	h                     *Handle
	path                  string
	kind                  DatabaseKind
	workspaceID, tenantID string
}

func (d *Database) Close() error { return d.h.Close() }

func OpenDatabase(ctx context.Context, path string, mode Mode, genesis Genesis) (*Database, error) {
	if _, err := migrations(genesis.Kind); err != nil {
		return nil, err
	}
	h, err := Open(ctx, path, mode)
	if err != nil {
		return nil, err
	}
	d := &Database{h: h, path: path, kind: genesis.Kind}
	if mode == CreateNew {
		err = initialize(ctx, h.Conn, genesis)
	}
	if err == nil {
		err = VerifySchema(ctx, h.Conn, genesis.Kind)
	}
	if err == nil && genesis.Kind == WorkspaceDatabase {
		err = h.Conn.QueryRowContext(ctx, "SELECT workspace_id,tenant_id FROM workspace_identity WHERE singleton=1").Scan(&d.workspaceID, &d.tenantID)
	}
	if err != nil {
		return nil, errors.Join(err, h.Close())
	}
	return d, nil
}

// FreshWorkspaceDatabase is a trusted adapter port, NOT a request capability.
// Its sole production producer is the Windows opaque, one-shot fresh object:
// relative CREATE_NEW, retained original file/ancestor pins and actual writer
// lease. No caller-supplied path, existing zero file or mode flag is a substitute.
type FreshWorkspaceDatabase interface {
	ClaimInitialization() (string, error)
	ValidateInitialization() error
}

// InitializePinnedWorkspace is the only precreated-file genesis entry. Existing
// OpenDatabase/Inspect still only verify. Any failed/unknown creation is left in
// place and the consumed fresh proof can never be replayed after restart.
func InitializePinnedWorkspace(ctx context.Context, fresh FreshWorkspaceDatabase, genesis Genesis) (*Database, error) {
	if fresh == nil || genesis.Kind != WorkspaceDatabase {
		return nil, ErrSchema
	}
	path, err := fresh.ClaimInitialization()
	if err != nil {
		return nil, err
	}
	if err = fresh.ValidateInitialization(); err != nil {
		return nil, err
	}
	h, err := Open(ctx, path, ReadWriteExisting)
	if err != nil {
		return nil, err
	}
	d := &Database{h: h, path: path, kind: WorkspaceDatabase}
	if err = fresh.ValidateInitialization(); err == nil {
		err = initialize(ctx, h.Conn, genesis)
	}
	if err == nil {
		err = d.VerifyCreationGenesis(ctx, genesis)
	}
	if err == nil {
		err = fresh.ValidateInitialization()
	}
	if err != nil {
		endErr := d.Close()
		if endErr != nil {
			return d, errors.Join(err, endErr)
		}
		return nil, err
	}
	d.workspaceID, d.tenantID = genesis.WorkspaceID, genesis.TenantID
	return d, nil
}

// VerifyCreationGenesis checks the entire schema/history plus the frozen original
// payload, identity and pristine genesis. A digest alone is not authorization.
func (d *Database) VerifyCreationGenesis(ctx context.Context, g Genesis) error {
	if d == nil || d.kind != WorkspaceDatabase || g.Kind != WorkspaceDatabase {
		return ErrSchema
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	check := func() error {
		if err := verifySchema(ctx, tx, WorkspaceDatabase); err != nil {
			return err
		}
		var w, t, name, state string
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id,tenant_id,display_name,business_state FROM workspace_identity WHERE singleton=1 AND format_version=2 AND schema_version=1`).Scan(&w, &t, &name, &state); err != nil {
			return err
		}
		if w != g.WorkspaceID || t != g.TenantID || name != g.DisplayName || state != "ready" {
			return ErrSchema
		}
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT revision FROM workspace_head WHERE workspace_id=?`, w).Scan(&revision); err != nil {
			return err
		}
		if revision != 0 {
			return ErrSchema
		}
		for _, table := range []string{"text_record", "text_version", "candidate", "save_operation", "commit_receipt", "audit_entry"} {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return ErrSchema
			}
		}
		return nil
	}
	return errors.Join(check(), tx.Rollback())
}

func validUUID(s string) bool {
	_, err := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID(s), TenantID: protocol.UUID(s), DatabaseRelativePath: "data/workspace.sqlite"})
	return err == nil
}
func initialize(ctx context.Context, conn *sql.Conn, g Genesis) error {
	if !validUUID(g.TenantID) {
		return ErrSchema
	}
	if g.Kind == WorkspaceDatabase && !validUUID(g.WorkspaceID) {
		return ErrSchema
	}
	if g.Kind == InstallationDatabase && (!validUUID(g.InstallationID) || !validUUID(g.OwnerPrincipalID) || g.WindowsSID == "" || g.SecretReference == "") {
		return ErrSchema
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return errors.Join(ErrUnknown, err)
	}
	// On pre-COMMIT failures explicit rollback is allowed, but its failure is
	// unknown. On COMMIT error database/sql's driver may itself try ROLLBACK;
	// neither behavior is used as a persistence classification here.
	abort := func(cause error) error {
		if endErr := tx.Rollback(); endErr != nil {
			return errors.Join(ErrUnknown, cause, endErr)
		}
		return cause
	}
	var count, version int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&count); err != nil {
		return abort(err)
	}
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return abort(err)
	}
	if count != 0 || version != 0 {
		return abort(ErrSchema)
	}
	steps, _ := migrations(g.Kind)
	for _, step := range steps {
		if _, err := tx.ExecContext(ctx, step.sql); err != nil {
			return abort(err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO migration_ledger(sequence,version,checksum,state) VALUES(?,?,?,'applied')", step.sequence, step.version, step.checksum); err != nil {
			return abort(err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_metadata VALUES(1,?,1)", string(g.Kind)); err != nil {
		return abort(err)
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
		return abort(err)
	}
	if g.Kind == WorkspaceDatabase {
		id, err := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID(g.WorkspaceID), TenantID: protocol.UUID(g.TenantID), DatabaseRelativePath: "data/workspace.sqlite"})
		if err != nil {
			return abort(err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO workspace_identity VALUES(1,?,?,2,1,?,?,'ready')", g.WorkspaceID, g.TenantID, string(id), g.DisplayName); err != nil {
			return abort(err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO workspace_head VALUES(?,0)", g.WorkspaceID); err != nil {
			return abort(err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, "INSERT INTO installation_identity VALUES(1,?,?,?,?,1,?)", g.InstallationID, g.TenantID, g.OwnerPrincipalID, g.WindowsSID, g.SecretReference); err != nil {
			return abort(err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO principal VALUES(?,?,1,0)", g.OwnerPrincipalID, g.TenantID); err != nil {
			return abort(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(ErrUnknown, err)
	}
	return nil
}

type schemaReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// VerifySchema issues queries only. Scratch inspectors can reuse it after their
// own frozen-copy protocol. No missing/old/future/partial state is ever repaired.
// One read transaction prevents mixing independently sampled history states.
func VerifySchema(ctx context.Context, conn *sql.Conn, kind DatabaseKind) error {
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return errors.Join(ErrSchema, err)
	}
	err = verifySchema(ctx, tx, kind)
	endErr := tx.Rollback()
	return errors.Join(err, endErr)
}
func verifySchema(ctx context.Context, q schemaReader, kind DatabaseKind) error {
	steps, err := migrations(kind)
	if err != nil {
		return err
	}
	var version, count int
	if err = q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		return errors.Join(ErrSchema, err)
	}
	var storedKind string
	if err = q.QueryRowContext(ctx, "SELECT kind,version FROM schema_metadata WHERE singleton=1").Scan(&storedKind, &version); err != nil || storedKind != string(kind) || version != 1 {
		return errors.Join(ErrSchema, err)
	}
	if err = q.QueryRowContext(ctx, "SELECT count(*) FROM schema_metadata").Scan(&count); err != nil || count != 1 {
		return errors.Join(ErrSchema, err)
	}
	rows, err := q.QueryContext(ctx, "SELECT sequence,version,checksum,state FROM migration_ledger ORDER BY sequence")
	if err != nil {
		return errors.Join(ErrSchema, err)
	}
	i := 0
	for rows.Next() {
		var seq, v int
		var checksum, state string
		if err = rows.Scan(&seq, &v, &checksum, &state); err != nil {
			break
		}
		if i >= len(steps) || seq != steps[i].sequence || v != steps[i].version || checksum != steps[i].checksum || state != "applied" {
			err = ErrSchema
			break
		}
		i++
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil || i != len(steps) {
		return errors.Join(ErrSchema, err)
	}
	// Check the actual DDL, not just a ledger claiming that DDL was applied. The
	// fixed SQL uses ordinary CREATE statements; SQLite preserves their text minus
	// the final semicolon. Internal autoindexes have NULL SQL and are excluded.
	script := strings.Join(strings.Fields(steps[0].sql), " ")
	rows, err = q.QueryContext(ctx, "SELECT sql FROM sqlite_schema WHERE sql IS NOT NULL ORDER BY name")
	if err != nil {
		return errors.Join(ErrSchema, err)
	}
	count = 0
	for rows.Next() {
		var ddl string
		if err = rows.Scan(&ddl); err != nil {
			break
		}
		if !strings.Contains(script, strings.Join(strings.Fields(ddl), " ")+";") {
			err = ErrSchema
			break
		}
		count++
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	expected := strings.Count(script, "CREATE TABLE ") + strings.Count(script, "CREATE TRIGGER ")
	if err != nil || count != expected {
		return errors.Join(ErrSchema, err)
	}
	var integrity string
	if err = q.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil || integrity != "ok" {
		return errors.Join(ErrSchema, err)
	}
	rows, err = q.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return errors.Join(ErrSchema, err)
	}
	badFK := rows.Next()
	err = errors.Join(rows.Err(), rows.Close())
	if badFK || err != nil {
		return errors.Join(ErrSchema, err)
	}
	if kind == InstallationDatabase {
		var id, tenant, owner, sid, secret string
		if err = q.QueryRowContext(ctx, "SELECT installation_id,tenant_id,owner_principal_id,windows_sid,secret_reference FROM installation_identity WHERE singleton=1 AND schema_version=1").Scan(&id, &tenant, &owner, &sid, &secret); err != nil || !validUUID(id) || !validUUID(tenant) || !validUUID(owner) || sid == "" || secret == "" {
			return errors.Join(ErrSchema, err)
		}
		if err = q.QueryRowContext(ctx, "SELECT count(*) FROM principal WHERE principal_id=? AND tenant_id=?", owner, tenant).Scan(&count); err != nil || count != 1 {
			return errors.Join(ErrSchema, err)
		}
		return nil
	}
	return verifyWorkspace(ctx, q)
}

func verifyWorkspace(ctx context.Context, q schemaReader) error {
	var workspace, tenant, identity string
	if err := q.QueryRowContext(ctx, "SELECT workspace_id,tenant_id,identity_digest FROM workspace_identity WHERE singleton=1 AND format_version=2 AND schema_version=1").Scan(&workspace, &tenant, &identity); err != nil {
		return errors.Join(ErrSchema, err)
	}
	expected, err := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID(workspace), TenantID: protocol.UUID(tenant), DatabaseRelativePath: "data/workspace.sqlite"})
	if err != nil || identity != string(expected) {
		return ErrSchema
	}
	var revision, versions, maxRevision int64
	if err = q.QueryRowContext(ctx, "SELECT revision FROM workspace_head WHERE workspace_id=?", workspace).Scan(&revision); err != nil {
		return errors.Join(ErrSchema, err)
	}
	if err = q.QueryRowContext(ctx, "SELECT count(*),coalesce(max(workspace_revision),0) FROM text_version WHERE workspace_id=?", workspace).Scan(&versions, &maxRevision); err != nil || revision != versions || revision != maxRevision {
		return errors.Join(ErrSchema, err)
	}
	for _, query := range []string{
		"SELECT count(*) FROM text_version v LEFT JOIN commit_receipt r ON r.workspace_id=v.workspace_id AND r.operation_id=v.operation_id WHERE r.operation_id IS NULL",
		"SELECT count(*) FROM save_operation o LEFT JOIN commit_receipt r ON r.workspace_id=o.workspace_id AND r.operation_id=o.operation_id WHERE (o.state='committed') != (r.operation_id IS NOT NULL)",
		"SELECT count(*) FROM commit_receipt r JOIN candidate c USING(workspace_id,candidate_id) WHERE c.state!='applied' OR c.resource_id!=r.resource_id OR c.content_digest!=r.content_digest",
		"SELECT count(*) FROM text_record r WHERE r.current_version!=(SELECT max(v.version) FROM text_version v WHERE v.workspace_id=r.workspace_id AND v.resource_id=r.resource_id) OR r.current_version!=(SELECT count(*) FROM text_version v WHERE v.workspace_id=r.workspace_id AND v.resource_id=r.resource_id)",
		"SELECT count(*) FROM candidate c WHERE c.state='applied' AND NOT EXISTS(SELECT 1 FROM commit_receipt r WHERE r.workspace_id=c.workspace_id AND r.candidate_id=c.candidate_id)",
		"SELECT count(*) FROM commit_receipt r WHERE (SELECT count(*) FROM audit_entry a WHERE a.workspace_id=r.workspace_id AND a.operation_id=r.operation_id AND a.principal_id=r.principal_id AND a.revision=r.new_revision AND a.action='save' AND a.outcome_code='committed')!=1",
		"SELECT count(*) FROM audit_entry a WHERE a.outcome_code='committed' AND NOT EXISTS(SELECT 1 FROM commit_receipt r WHERE r.workspace_id=a.workspace_id AND r.operation_id=a.operation_id AND r.new_revision=a.revision)",
	} {
		var bad int
		if err = q.QueryRowContext(ctx, query).Scan(&bad); err != nil || bad != 0 {
			return errors.Join(ErrSchema, err)
		}
	}
	rows, err := q.QueryContext(ctx, "SELECT workspace_id,resource_id,title,body,content_digest FROM text_version")
	if err != nil {
		return errors.Join(ErrSchema, err)
	}
	defer rows.Close()
	for rows.Next() {
		var w, r, title, body, got string
		if err := rows.Scan(&w, &r, &title, &body, &got); err != nil {
			return errors.Join(ErrSchema, err)
		}
		want, err := digest.Content(digest.ContentInput{WorkspaceID: protocol.UUID(w), ResourceID: protocol.UUID(r), Title: protocol.Title(title), Body: protocol.Body(body)})
		if err != nil || got != string(want) {
			return ErrSchema
		}
	}
	return errors.Join(rows.Err(), rows.Close())
}

func schemaError(detail string) error { return fmt.Errorf("%w: %s", ErrSchema, detail) }
