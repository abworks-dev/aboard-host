//go:build windows && amd64

package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

func bindingFixture(t *testing.T) BindingOperation {
	t.Helper()
	o := creationStorageIntent(t)
	b := BindingOperation{InstallationOperation: o.InstallationOperation, TenantID: o.TenantID, AllowedLocation: o.AllowedLocation, IdentityDigest: o.IdentityDigest, RootVolume: "synthetic-volume", RootFileID: make([]byte, 16), DatabaseFileID: make([]byte, 16), DescriptorFileID: make([]byte, 16)}
	b.Kind = "register"
	bindingHash(t, &b)
	return b
}
func bindingHash(t *testing.T, b *BindingOperation) {
	t.Helper()
	h, err := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(b.InstallationID), PrincipalID: protocol.UUID(b.PrincipalID), OperationID: protocol.UUID(b.OperationID), Kind: b.Kind, WorkspaceID: protocol.UUID(b.WorkspaceID), CanonicalLocator: b.TargetLocator})
	if err != nil {
		t.Fatal(err)
	}
	b.InputDigest = string(h)
}
func TestBindingRejectsContradictoryCommittedJournal(t *testing.T) {
	d, hash := authStorageFixture(t)
	ctx := context.Background()
	want := bindingFixture(t)
	if _, _, err := d.RecordBindingIntent(ctx, hash, 1, want); err != nil {
		t.Fatal(err)
	}
	if err := d.FinalizeBinding(ctx, hash, 1, want, false); err != nil {
		t.Fatal(err)
	}
	if _, err := d.h.Conn.ExecContext(ctx, `UPDATE installation_operation SET phase='intentRecorded' WHERE operation_id=?`, want.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := d.FinalizeBinding(ctx, hash, 1, want, true); !errors.Is(err, ErrInstallationOperation) {
		t.Fatalf("contradictory completion accepted: %v", err)
	}
	got, err := d.ReadBindingOperation(ctx, hash, 1, want.InstallationID, want.PrincipalID, want.OperationID)
	if !errors.Is(err, ErrInstallationOperation) || got.InstallationID != "" {
		t.Fatalf("inconsistent fact exposed: %+v %v", got, err)
	}
}
func TestBindingRejectsRejectedCreationResidualNewKey(t *testing.T) {
	d, hash := authStorageFixture(t)
	ctx := context.Background()
	original := creationStorageReady(t, d, hash)
	if err := d.InterruptCreation(ctx, hash, 1, original, false); err != nil {
		t.Fatal(err)
	}
	want := bindingFixture(t)
	want.OperationID = testResource
	bindingHash(t, &want)
	if _, _, err := d.RecordBindingIntent(ctx, hash, 1, want); !errors.Is(err, ErrInstallationOperationConflict) {
		t.Fatalf("new key adopted rejected original: %v", err)
	}
	if scalar(t, d, `SELECT count(*) FROM installation_binding_intent`) != 0 || scalar(t, d, `SELECT count(*) FROM directory_registration WHERE workspace_id=?`, want.WorkspaceID) != 0 {
		t.Fatal("residual bypass created new fact")
	}
}

func TestBindingRegisterFrozenIntentAndAtomicFinalize(t *testing.T) {
	d, hash := authStorageFixture(t)
	ctx := context.Background()
	want := bindingFixture(t)
	got, old, err := d.RecordBindingIntent(ctx, hash, 1, want)
	if err != nil || old || !sameBinding(got, want) {
		t.Fatalf("record: %+v %v", got, err)
	}
	changed := want
	changed.ExpectedRegistrationVersion = 1
	if _, _, err = d.RecordBindingIntent(ctx, hash, 1, changed); !errors.Is(err, ErrInstallationOperationConflict) {
		t.Fatalf("omitted version not frozen: %v", err)
	}
	if _, old, err = d.RecordBindingIntent(ctx, hash, 1, want); err != nil || !old {
		t.Fatalf("repeat not inquiry: %v", err)
	}
	if scalar(t, d, `SELECT count(*) FROM directory_registration WHERE workspace_id=?`, want.WorkspaceID) != 0 {
		t.Fatal("intent registered")
	}
	if _, err = d.h.Conn.ExecContext(ctx, `UPDATE installation_operation SET outcome='unknown' WHERE operation_id=?`, want.OperationID); err != nil {
		t.Fatal(err)
	}
	if err = d.FinalizeBinding(ctx, hash, 1, want, false); err == nil {
		t.Fatal("unknown auto-finalized")
	}
	if err = d.FinalizeBinding(ctx, hash, 1, want, true); err != nil {
		t.Fatal(err)
	}
	if scalar(t, d, `SELECT count(*) FROM grant_entry WHERE workspace_id=?`, want.WorkspaceID) != 0 {
		t.Fatal("directory imported grants")
	}
	got, err = d.ReadBindingOperation(ctx, hash, 1, want.InstallationID, want.PrincipalID, want.OperationID)
	if err != nil || got.Outcome != string(InstallationCommitted) {
		t.Fatalf("read committed: %+v %v", got, err)
	}
	if err = d.FinalizeBinding(ctx, hash, 1, want, true); err != nil {
		t.Fatalf("committed verify: %v", err)
	}
	copy := want
	copy.OperationID = testResource
	copy.RootFileID = append([]byte{}, want.RootFileID...)
	copy.RootFileID[0] = 1
	copy.ExpectedRegistrationVersion = 1
	bindingHash(t, &copy)
	if _, _, err = d.RecordBindingIntent(ctx, hash, 1, copy); err == nil {
		t.Fatal("same UUID copied root admitted")
	}
}
func TestBindingRelinkCompareAndSwapPreservesGrants(t *testing.T) {
	d, hash := authStorageFixture(t)
	ctx := context.Background()
	first := bindingFixture(t)
	if _, _, err := d.RecordBindingIntent(ctx, hash, 1, first); err != nil {
		t.Fatal(err)
	}
	if err := d.FinalizeBinding(ctx, hash, 1, first, false); err != nil {
		t.Fatal(err)
	}
	if _, err := d.h.Conn.ExecContext(ctx, `INSERT INTO grant_entry VALUES(?,?,?,'workspace.operator')`, testPrincipal, testTenant, first.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	moved := first
	moved.Kind = "relink"
	moved.OperationID = testResource
	moved.PreviousLocator = first.TargetLocator
	moved.TargetLocator += "-moved"
	moved.ExpectedRegistrationVersion = 1
	bindingHash(t, &moved)
	if _, _, err := d.RecordBindingIntent(ctx, hash, 1, moved); err != nil {
		t.Fatal(err)
	}
	changed := moved
	changed.PreviousLocator += "-forged"
	if err := d.FinalizeBinding(ctx, hash, 1, changed, true); !errors.Is(err, ErrInstallationOperationConflict) {
		t.Fatalf("changed original locator: %v", err)
	}
	if _, err := d.h.Conn.ExecContext(ctx, `UPDATE directory_registration SET registration_version=2 WHERE workspace_id=?`, first.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err := d.FinalizeBinding(ctx, hash, 1, moved, true); err == nil {
		t.Fatal("stale relink overwrote current binding")
	}
	if _, err := d.h.Conn.ExecContext(ctx, `UPDATE directory_registration SET registration_version=1 WHERE workspace_id=?`, first.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err := d.FinalizeBinding(ctx, hash, 1, moved, true); err != nil {
		t.Fatal(err)
	}
	if scalar(t, d, `SELECT count(*) FROM directory_registration WHERE workspace_id=? AND root_locator=? AND registration_version=2`, first.WorkspaceID, moved.TargetLocator) != 1 {
		t.Fatal("relink CAS not committed")
	}
	if scalar(t, d, `SELECT count(*) FROM grant_entry WHERE workspace_id=?`, first.WorkspaceID) != 1 {
		t.Fatal("relink changed grants")
	}
	if _, err := d.h.Conn.ExecContext(ctx, `DELETE FROM grant_entry WHERE workspace_id=?`, first.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err := d.FinalizeBinding(ctx, hash, 1, moved, true); !errors.Is(err, ErrAuthorization) {
		t.Fatalf("stale permission authorized reconcile: %v", err)
	}
}
