//go:build windows && amd64

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"golang.org/x/sys/windows"
)

func creationStorageIntent(t *testing.T) DirectoryOperation {
	t.Helper()
	const target = `E:\synthetic\new-workspace`
	h, err := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(testInstallation), PrincipalID: protocol.UUID(testPrincipal), OperationID: protocol.UUID(testOperation), Kind: "create", WorkspaceID: protocol.UUID(testCandidate), CanonicalLocator: target})
	if err != nil {
		t.Fatal(err)
	}
	id, err := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID(testCandidate), TenantID: protocol.UUID(testTenant), DatabaseRelativePath: "data/workspace.sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	return DirectoryOperation{InstallationOperation: InstallationOperation{InstallationID: testInstallation, OperationID: testOperation, PrincipalID: testPrincipal, WorkspaceID: testCandidate, Kind: "create", TargetLocator: target, InputDigest: string(h), Phase: string(IntentRecorded), Outcome: string(InstallationAccepted)}, TenantID: testTenant, DisplayName: "synthetic workspace", AllowedLocation: `E:\synthetic`, IdentityDigest: string(id)}
}
func creationStorageReady(t *testing.T, d *Database, hash string) DirectoryOperation {
	t.Helper()
	ctx := context.Background()
	want := creationStorageIntent(t)
	got, existing, err := d.RecordCreationIntent(ctx, hash, 1, want)
	if err != nil || existing {
		t.Fatalf("intent: %v", err)
	}
	got.RootVolume = "synthetic-evidence-only"
	got.RootFileID = make([]byte, 16)
	got.RootFileID[0] = 1
	got.DatabaseFileID = make([]byte, 16)
	got.DatabaseFileID[0] = 2
	if err = d.CheckpointCreation(ctx, hash, 1, got, DirectoryPrepared); err != nil {
		t.Fatal(err)
	}
	got.Phase = string(DirectoryPrepared)
	if err = d.CheckpointCreation(ctx, hash, 1, got, GenesisConfirmed); err != nil {
		t.Fatal(err)
	}
	got.Phase = string(GenesisConfirmed)
	got.DescriptorFileID = make([]byte, 16)
	got.DescriptorFileID[0] = 3
	if err = d.CheckpointCreation(ctx, hash, 1, got, DescriptorPublished); err != nil {
		t.Fatal(err)
	}
	got.Phase = string(DescriptorPublished)
	return got
}
func TestInstallationCreationFrozenPayloadAndKey(t *testing.T) {
	ctx := context.Background()
	d, hash := authStorageFixture(t)
	want := creationStorageIntent(t)
	got, existing, err := d.RecordCreationIntent(ctx, hash, 1, want)
	if err != nil || existing || !sameCreationIntent(got, want) {
		t.Fatalf("first intent=%+v %v", got, err)
	}
	renamed := want
	renamed.DisplayName = "renamed without hash change"
	if _, _, err = d.RecordCreationIntent(ctx, hash, 1, renamed); !errors.Is(err, ErrInstallationOperationConflict) {
		t.Fatal("hash omission allowed changed displayName", err)
	}
	if _, existing, err = d.RecordCreationIntent(ctx, hash, 1, want); err != nil || !existing {
		t.Fatal("same original intent lost", err)
	}
	if _, err = d.ReadDirectoryOperation(ctx, hash, 1, testInstallation, testResource, testOperation); err == nil {
		t.Fatal("different actor read operation")
	}
	if _, err = d.ReadDirectoryOperation(ctx, hash, 2, testInstallation, testPrincipal, testOperation); err == nil {
		t.Fatal("stale version read operation")
	}
	if n := scalar(t, d, `SELECT count(*) FROM directory_registration WHERE workspace_id=?`, want.WorkspaceID); n != 0 {
		t.Fatal("intent registered target")
	}
	if err = d.FinalizeCreation(ctx, hash, 1, want, true); err == nil {
		t.Fatal("reconcile guessed intent-only evidence")
	}
}
func TestInstallationCreationFinalizeAtomicAndReconcileExactEvidence(t *testing.T) {
	ctx := context.Background()
	d, hash := authStorageFixture(t)
	want := creationStorageReady(t, d, hash)
	if err := d.InterruptCreation(ctx, hash, 1, want, true); err != nil {
		t.Fatal(err)
	}
	if err := d.FinalizeCreation(ctx, hash, 1, want, false); err == nil {
		t.Fatal("unknown implicitly resumed")
	}
	changed := want
	changed.DatabaseFileID = append([]byte{}, want.DatabaseFileID...)
	changed.DatabaseFileID[0]++
	if err := d.FinalizeCreation(ctx, hash, 1, changed, true); err == nil {
		t.Fatal("different physical DB accepted")
	}
	if err := d.FinalizeCreation(ctx, hash, 1, want, true); err != nil {
		t.Fatal(err)
	}
	if scalar(t, d, `SELECT count(*) FROM directory_registration WHERE workspace_id=? AND state='registered'`, want.WorkspaceID) != 1 || scalar(t, d, `SELECT count(*) FROM grant_entry WHERE workspace_id=?`, want.WorkspaceID) != 5 {
		t.Fatal("registration+initial grants incomplete")
	}
	got, err := d.ReadDirectoryOperation(ctx, hash, 1, testInstallation, testPrincipal, testOperation)
	if err != nil || got.Phase != string(RegistrationConfirmed) || got.Outcome != string(InstallationCommitted) {
		t.Fatalf("final row=%+v %v", got, err)
	}
	if err = d.FinalizeCreation(ctx, hash, 1, want, true); err != nil {
		t.Fatal("exact committed investigation failed", err)
	}
	if _, err = d.h.Conn.ExecContext(ctx, `UPDATE directory_registration SET registration_version=2 WHERE workspace_id=?`, want.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err = d.FinalizeCreation(ctx, hash, 1, want, true); err == nil {
		t.Fatal("changed current registration accepted")
	}
}
func TestInstallationCreationFinalizeRealFlushUnknown(t *testing.T) {
	ctx := context.Background()
	count := int32(0)
	for ordinal := int32(0); ordinal == 0 || ordinal <= count; ordinal++ {
		t.Run(fmt.Sprintf("flush-%d", ordinal), func(t *testing.T) {
			d, hash := authStorageFixture(t)
			want := creationStorageReady(t, d, hash)
			path := filepath.Join(filepath.Dir(d.path), "readonly-stimulus")
			if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
				t.Fatal(err)
			}
			stimulus, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer stimulus.Close()
			if err = windows.FlushFileBuffers(windows.Handle(stimulus.Fd())); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				t.Fatal("native stimulus missing", err)
			}
			flushFault.handle = stimulus.Fd()
			flushFault.at = ordinal
			flushFault.calls.Store(0)
			flushFault.failed.Store(0)
			restore := replaceFlushForGate(t)
			err = d.FinalizeCreation(ctx, hash, 1, want, false)
			calls, failed := flushFault.calls.Load(), flushFault.failed.Load()
			restore()
			if ordinal == 0 {
				if err != nil || calls < 1 || calls > 8 {
					t.Fatal("flush baseline missing", err, calls)
				}
				count = calls
			} else if !errors.Is(err, ErrUnknown) || failed != 1 {
				t.Fatal("commit flush failure claimed known", err, failed)
			}
			path = d.path
			if err = d.Close(); err != nil {
				t.Fatal(err)
			}
			fresh, err := OpenDatabase(ctx, path, ReadWriteExisting, Genesis{Kind: InstallationDatabase})
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			registered := scalar(t, fresh, `SELECT count(*) FROM directory_registration WHERE workspace_id=?`, want.WorkspaceID)
			grants := scalar(t, fresh, `SELECT count(*) FROM grant_entry WHERE workspace_id=?`, want.WorkspaceID)
			committed := scalar(t, fresh, `SELECT count(*) FROM installation_operation WHERE operation_id=? AND phase='registrationConfirmed' AND outcome='committed'`, want.OperationID)
			if (registered != 0 && registered != 1) || grants != registered*5 || committed != registered {
				t.Fatalf("partial installation transaction: registrations=%d grants=%d committed=%d", registered, grants, committed)
			}
			t.Logf("ordinal=%d native_failures=%d flush_calls=%d atomic_registered=%d grants=%d", ordinal, failed, calls, registered, grants)
		})
	}
}
