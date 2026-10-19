package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func TestInstallationTypedIdentityAndCurrentAuthorization(t *testing.T) {
	ctx := context.Background()
	d := newStorageDB(t, InstallationDatabase)
	defer closeStorage(t, d)
	id, err := d.ReadInstallationIdentity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id.InstallationID != testInstallation || id.TenantID != testTenant || id.OwnerPrincipalID != testPrincipal || id.WindowsSID != "S-1-5-21-synthetic" || id.SecretReference != "synthetic-reference-only" || id.SchemaVersion != 1 {
		t.Fatal("typed identity differs from DB genesis")
	}
	auth, err := d.CurrentInstallationAuthorization(ctx, testPrincipal, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if auth.GrantVersion != 1 || auth.Revoked || auth.WorkspaceAllowed || auth.TenantID != testTenant {
		t.Fatal("genesis current principal invalid")
	}
	for _, query := range []string{
		"INSERT INTO directory_registration VALUES('" + testWorkspace + "','" + testTenant + "','synthetic-root','synthetic-parent','synthetic-volume',zeroblob(16),1,'pending')",
		"INSERT INTO grant_entry VALUES('" + testPrincipal + "','" + testTenant + "','" + testWorkspace + "','save')",
	} {
		if _, err = d.h.Conn.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	auth, err = d.CurrentInstallationAuthorization(ctx, testPrincipal, testWorkspace, "save")
	if err != nil || auth.WorkspaceAllowed {
		t.Fatal("pending registration grants authority", err)
	}
	if _, err = d.h.Conn.ExecContext(ctx, "UPDATE directory_registration SET state='registered'"); err != nil {
		t.Fatal(err)
	}
	auth, err = d.CurrentInstallationAuthorization(ctx, testPrincipal, testWorkspace, "save")
	if err != nil || !auth.WorkspaceAllowed {
		t.Fatal("explicit grant refused", err)
	}
	auth, err = d.CurrentInstallationAuthorization(ctx, testPrincipal, testWorkspace, "read")
	if err != nil || auth.WorkspaceAllowed {
		t.Fatal("action expanded", err)
	}
	auth, err = d.CurrentInstallationAuthorization(ctx, testPrincipal, testResource, "save")
	if err != nil || auth.WorkspaceAllowed {
		t.Fatal("cross-workspace authority", err)
	}
	if _, err = d.h.Conn.ExecContext(ctx, "UPDATE principal SET grant_version=2,revoked=1 WHERE principal_id=?", testPrincipal); err != nil {
		t.Fatal(err)
	}
	auth, err = d.CurrentInstallationAuthorization(ctx, testPrincipal, testWorkspace, "save")
	if err != nil || !auth.Revoked || auth.GrantVersion != 2 || auth.WorkspaceAllowed {
		t.Fatal("revocation/version cached", err)
	}
	if _, err = d.CurrentInstallationAuthorization(ctx, testResource, testWorkspace, "save"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("missing principal accepted", err)
	}
	if _, err = d.CurrentInstallationAuthorization(ctx, "not-a-uuid", testWorkspace, "save"); !errors.Is(err, ErrSchema) {
		t.Fatal("invalid principal accepted", err)
	}
}

func TestInstallationTypedOperationScopeAndNoInitialization(t *testing.T) {
	ctx := context.Background()
	d := newStorageDB(t, InstallationDatabase)
	defer closeStorage(t, d)
	if _, err := d.ReadInstallationOperation(ctx, testInstallation, testOperation); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("missing operation was synthesized", err)
	}
	if _, err := d.h.Conn.ExecContext(ctx, "INSERT INTO installation_operation VALUES(?,?,?,?,'create','synthetic-root',?,'intentRecorded','unknown')", testInstallation, testOperation, testPrincipal, testWorkspace, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	op, err := d.ReadInstallationOperation(ctx, testInstallation, testOperation)
	if err != nil || op.Phase != "intentRecorded" || op.Outcome != "unknown" || op.PrincipalID != testPrincipal {
		t.Fatal("operation lost original association", err)
	}
	if _, err = d.ReadInstallationOperation(ctx, testResource, testOperation); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("cross-installation operation leaked", err)
	}
	if _, err = d.h.Conn.ExecContext(ctx, "UPDATE migration_ledger SET checksum=?", strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = d.ReadInstallationIdentity(ctx); !errors.Is(err, ErrSchema) {
		t.Fatal("bad ledger accepted", err)
	}
	if got := scalar(t, d, "SELECT count(*) FROM migration_ledger WHERE checksum=?", strings.Repeat("0", 64)); got != 1 {
		t.Fatal("typed identity repaired ledger")
	}
	w := newStorageDB(t, WorkspaceDatabase)
	defer closeStorage(t, w)
	if _, err = w.ReadInstallationIdentity(ctx); !errors.Is(err, ErrSchema) {
		t.Fatal("workspace became installation authority", err)
	}
	if _, err = w.CurrentInstallationAuthorization(ctx, testPrincipal, "", ""); !errors.Is(err, ErrSchema) {
		t.Fatal("workspace authority accepted", err)
	}
}
