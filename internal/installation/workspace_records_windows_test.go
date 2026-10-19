//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/digest"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// Fixture only: real receipt-backed saves while holding the actual native lease,
// then physical close before the business owner opens. This does NOT claim the
// T018/T019 use cases or full Host-process save/reopen acceptance already exist.
func seedRecordReadFacts(t *testing.T, root, ws string, a *Authorization, label string) {
	t.Helper()
	ctx := context.Background()
	lease, err := platform.Acquire(root, platform.Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	}()
	db, err := sqlite.OpenDatabase(ctx, filepath.Join(root, "data", "workspace.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	gate, err := sqlite.NewCommitGate(db, "synthetic-record-read-seed")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := gate.EndPhysical(); err != nil {
			t.Error(err)
		}
	}()
	for n := 1; n <= 3; n++ {
		candidate := sqlite.Candidate{WorkspaceID: ws, CandidateID: fmt.Sprintf("71000000-0000-4000-8000-%012d", n), PrincipalID: a.principalID, ResourceID: fmt.Sprintf("73000000-0000-4000-8000-%012d", n), Kind: "create-text", BaseRevision: int64(n - 1), Title: label, Body: fmt.Sprintf("%s body %d", label, n), PreviewVersion: 1, PreviewEpoch: "synthetic-record-read-seed", ExpiresAt: time.Now().Add(time.Hour)}
		content, err := digest.Content(digest.ContentInput{WorkspaceID: protocol.UUID(ws), ResourceID: protocol.UUID(candidate.ResourceID), Title: protocol.Title(candidate.Title), Body: protocol.Body(candidate.Body)})
		if err != nil {
			t.Fatal(err)
		}
		hash, err := digest.Candidate(digest.CandidateInput{WorkspaceID: protocol.UUID(ws), PrincipalID: protocol.UUID(a.principalID), CandidateID: protocol.UUID(candidate.CandidateID), ResourceID: protocol.UUID(candidate.ResourceID), Kind: "create-text", BaseRevision: protocol.Revision(fmt.Sprint(n - 1)), ContentDigest: content, PreviewVersion: "1"})
		if err != nil {
			t.Fatal(err)
		}
		candidate.ContentDigest = string(content)
		candidate.CandidateDigest = string(hash)
		if err = gate.PutCandidate(ctx, candidate); err != nil {
			t.Fatal(err)
		}
		if n == 3 {
			continue
		} // An unconfirmed preview must never enter the formal list.
		request := sqlite.SaveRequest{InstallationID: a.installationID, TenantID: a.tenantID, WorkspaceID: ws, PrincipalID: a.principalID, OperationID: fmt.Sprintf("72000000-0000-4000-8000-%012d", n), CandidateID: candidate.CandidateID, CandidateDigest: candidate.CandidateDigest, Epoch: candidate.PreviewEpoch, PreviewVersion: candidate.PreviewVersion}
		_, admission, err := gate.Admit(ctx, request, time.Now())
		if err != nil || admission == nil {
			t.Fatal("admission", err)
		}
		result, err := gate.CommitAccepted(ctx, admission)
		if err != nil || result.Status != sqlite.Confirmed {
			t.Fatal("commit", result, err)
		}
	}
}
func TestWorkspaceRecordsRealIsolationCurrentAuthAndClosed(t *testing.T) {
	f, creator, operator, token, r := creationFixture(t)
	ctx := context.Background()
	a, err := creator.CreateWorkspace(ctx, operator, r)
	if err != nil {
		t.Fatal(err)
	}
	rb := r
	rb.OperationID = authSameResource
	rb.RootLocator = filepath.Join(f.allowed, "created-b")
	b, err := creator.CreateWorkspace(ctx, operator, rb)
	if err != nil {
		t.Fatal(err)
	}
	seedRecordReadFacts(t, r.RootLocator, a.WorkspaceID, operator, "A")
	seedRecordReadFacts(t, rb.RootLocator, b.WorkspaceID, operator, "B")
	opener, err := NewWorkspaceOpener(f.auth)
	if err != nil {
		t.Fatal(err)
	}
	openA, err := f.auth.Authorize(ctx, token, a.WorkspaceID, "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	openB, err := f.auth.Authorize(ctx, token, b.WorkspaceID, "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	closeA, err := f.auth.Authorize(ctx, token, a.WorkspaceID, "workspace.close")
	if err != nil {
		t.Fatal(err)
	}
	closeB, err := f.auth.Authorize(ctx, token, b.WorkspaceID, "workspace.close")
	if err != nil {
		t.Fatal(err)
	}
	readA, err := f.auth.Authorize(ctx, token, a.WorkspaceID, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	readB, err := f.auth.Authorize(ctx, token, b.WorkspaceID, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = opener.ListRecords(ctx, readA, "", 1); !errors.Is(err, workspace.ErrLifecycle) {
		t.Fatal("closed read auto-opened", err)
	}
	if _, err = opener.OpenWorkspace(ctx, openA, 1); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opener.DrainWorkspace(ctx, closeA); err != nil {
			t.Error(err)
		}
	}()
	if _, err = opener.OpenWorkspace(ctx, openB, 1); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opener.DrainWorkspace(ctx, closeB); err != nil {
			t.Error(err)
		}
	}()
	first, err := opener.ListRecords(ctx, readA, "", 1)
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" || first.Items[0].Body != "A body 1" || first.Items[0].OperationID != "72000000-0000-4000-8000-000000000001" {
		t.Fatal(first, err)
	}
	if _, err = opener.ListRecords(ctx, readB, first.NextCursor, 1); !errors.Is(err, workspace.ErrRecordQuery) {
		t.Fatal("A cursor used in B", err)
	}
	next, err := opener.ListRecords(ctx, readA, first.NextCursor, 100)
	if err != nil || len(next.Items) != 1 || next.Items[0].Body != "A body 2" {
		t.Fatal(next, err)
	}
	same, err := opener.ReadRecord(ctx, readB, first.Items[0].ResourceID)
	if err != nil || same.Body != "B body 1" {
		t.Fatal("same key cross-scope", same, err)
	}
	snapshot, err := opener.ReadSnapshot(ctx, readA)
	if err != nil || snapshot.Revision != 2 {
		t.Fatal(snapshot, err)
	}
	if _, err = opener.ReadRecord(ctx, readA, "73000000-0000-4000-8000-000000000003"); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatal("preview became formal", err)
	}
	if _, err = opener.ReadRecord(ctx, openA, first.Items[0].ResourceID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("wrong action", err)
	}
	if err = f.auth.SetPrincipalAuthorization(ctx, operator, authRestricted, false, []sqlite.WorkspaceGrant{{WorkspaceID: a.WorkspaceID, Action: "workspace.read"}}); err != nil {
		t.Fatal(err)
	}
	restrictedToken := authIssue(t, f, authRestricted, a.WorkspaceID, "workspace.read", time.Minute)
	restricted, err := f.auth.Authorize(ctx, restrictedToken, a.WorkspaceID, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	got, err := opener.ReadRecord(ctx, restricted, first.Items[0].ResourceID)
	if err != nil || got.Body != "A body 1" {
		t.Fatal("workspace.read-only principal cannot read", got, err)
	}
	if _, err = f.auth.Authorize(ctx, restrictedToken, b.WorkspaceID, "workspace.read"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("restricted B", err)
	}
	if err = f.auth.RevokeSession(ctx, operator, restrictedToken); err != nil {
		t.Fatal(err)
	}
	got, err = opener.ReadRecord(ctx, restricted, first.Items[0].ResourceID)
	if !errors.Is(err, ErrUnauthorized) || got.Body != "" {
		t.Fatal("revoked read", got, err)
	}
	if err = opener.DrainWorkspace(ctx, closeA); err != nil {
		t.Fatal(err)
	}
	if _, err = opener.ReadRecord(ctx, readA, first.Items[0].ResourceID); !errors.Is(err, workspace.ErrLifecycle) {
		t.Fatal("closed stale snapshot", err)
	}
	if _, err = opener.ReadRecord(ctx, readB, first.Items[0].ResourceID); err != nil {
		t.Fatal("close A broke B", err)
	}
	// A read never reopens implicitly; only the explicit authorized open does.
	if _, err = opener.OpenWorkspace(ctx, openA, 1); err != nil {
		t.Fatal(err)
	}
	got, err = opener.ReadRecord(ctx, readA, first.Items[0].ResourceID)
	if err != nil || got != first.Items[0] {
		t.Fatal("fresh physical connection changed formal facts", got, err)
	}
	t.Log("real native leases + fixed SQLite facts, same IDs across A/B, scoped pagination, read-only and revoked principal, preview not formal, closed A does not open itself or break B; not full Host-process acceptance")
}
