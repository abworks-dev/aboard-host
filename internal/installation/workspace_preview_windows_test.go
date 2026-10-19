//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestWorkspacePreviewRealOwnerAuthAndReopen(t *testing.T) {
	f, creator, operator, token, request := creationFixture(t)
	ctx := context.Background()
	created, err := creator.CreateWorkspace(ctx, operator, request)
	if err != nil {
		t.Fatal(err)
	}
	second := request
	second.OperationID = authSameResource
	second.RootLocator = filepath.Join(f.allowed, "preview-b")
	b, err := creator.CreateWorkspace(ctx, operator, second)
	if err != nil {
		t.Fatal(err)
	}
	seedRecordReadFacts(t, request.RootLocator, created.WorkspaceID, operator, "A unchanged")
	opener, err := NewWorkspaceOpener(f.auth)
	if err != nil {
		t.Fatal(err)
	}
	auth := func(ws, action string) *Authorization {
		t.Helper()
		a, err := f.auth.Authorize(ctx, token, ws, action)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	openA, closeA, readA, writeA := auth(created.WorkspaceID, "workspace.open"), auth(created.WorkspaceID, "workspace.close"), auth(created.WorkspaceID, "workspace.read"), auth(created.WorkspaceID, "workspace.write")
	openB, closeB, readB, writeB := auth(b.WorkspaceID, "workspace.open"), auth(b.WorkspaceID, "workspace.close"), auth(b.WorkspaceID, "workspace.read"), auth(b.WorkspaceID, "workspace.write")
	view, err := opener.OpenWorkspace(ctx, openA, 1)
	if err != nil {
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
	original, err := opener.ListRecords(ctx, readA, "", 100)
	if err != nil || len(original.Items) != 2 {
		t.Fatal(original, err)
	}
	in := workspace.PreviewInput{Epoch: view.Epoch, Kind: "create-text", BaseRevision: 2, Title: "same title😀", Body: "exact\r\né inline text"}
	first, err := opener.PreviewRecord(ctx, writeA, in)
	if err != nil || first.Before != nil || first.Candidate.PrincipalID != operator.principalID || first.Candidate.WorkspaceID != created.WorkspaceID || first.Candidate.PreviewEpoch != view.Epoch || first.Candidate.PreviewVersion != 1 || !time.Now().Before(first.Candidate.ExpiresAt) {
		t.Fatal(first, err)
	}
	next, err := opener.PreviewRecord(ctx, writeA, in)
	if err != nil || next.Candidate.ResourceID == first.Candidate.ResourceID || next.Candidate.CandidateID == first.Candidate.CandidateID {
		t.Fatal("same name merged", next, err)
	}
	in.Body = "changed text needs its own preview"
	changed, err := opener.PreviewRecord(ctx, writeA, in)
	if err != nil || changed.Candidate.CandidateDigest == first.Candidate.CandidateDigest || changed.Candidate.CandidateID == first.Candidate.CandidateID {
		t.Fatal(changed, err)
	}
	base := original.Items[0].ContentDigest
	updateIn := workspace.PreviewInput{Epoch: view.Epoch, Kind: "update-text", ResourceID: original.Items[0].ResourceID, BaseRevision: 2, BaseDigest: &base, Title: "updated", Body: "only a candidate, never applied"}
	updated, err := opener.PreviewRecord(ctx, writeA, updateIn)
	if err != nil || updated.Before == nil || *updated.Before != original.Items[0] || updated.Candidate.ResourceID != original.Items[0].ResourceID {
		t.Fatal(updated, err)
	}
	bad := updateIn
	stale := "0000000000000000000000000000000000000000000000000000000000000000"
	bad.BaseDigest = &stale
	if _, err = opener.PreviewRecord(ctx, writeA, bad); !errors.Is(err, workspace.ErrBaseline) {
		t.Fatal("stale content", err)
	}
	bad = updateIn
	bad.ResourceID = authSameResource
	if _, err = opener.PreviewRecord(ctx, writeA, bad); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatal("foreign/missing resource", err)
	}
	bad = in
	bad.BaseRevision = 1
	if _, err = opener.PreviewRecord(ctx, writeA, bad); !errors.Is(err, workspace.ErrBaseline) {
		t.Fatal("stale head", err)
	}
	if _, err = opener.PreviewRecord(ctx, readA, in); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("read auth writes", err)
	}
	if _, err = opener.PreviewRecord(ctx, writeB, in); !errors.Is(err, workspace.ErrStaleHandle) {
		t.Fatal("A epoch selects B", err)
	}
	if snapshot, err := opener.ReadSnapshot(ctx, readA); err != nil || snapshot.Revision != 2 {
		t.Fatal("preview changed Head", snapshot, err)
	}
	got, err := opener.ListRecords(ctx, readA, "", 100)
	if err != nil || len(got.Items) != 2 || got.Items[0] != original.Items[0] || got.Items[1] != original.Items[1] {
		t.Fatal("preview changed formal", got, err)
	}
	if got, err := opener.ListRecords(ctx, readB, "", 100); err != nil || len(got.Items) != 0 {
		t.Fatal("A preview crossed B", got, err)
	}
	// Exact actor is taken from current authorization, never from caller input.
	if err = f.auth.SetPrincipalAuthorization(ctx, operator, authRestricted, false, []sqlite.WorkspaceGrant{{WorkspaceID: created.WorkspaceID, Action: "workspace.write"}}); err != nil {
		t.Fatal(err)
	}
	restrictedToken := authIssue(t, f, authRestricted, created.WorkspaceID, "workspace.write", time.Minute)
	restricted, err := f.auth.Authorize(ctx, restrictedToken, created.WorkspaceID, "workspace.write")
	if err != nil {
		t.Fatal(err)
	}
	restrictedPreview, err := opener.PreviewRecord(ctx, restricted, in)
	if err != nil || restrictedPreview.Candidate.PrincipalID != authRestricted {
		t.Fatal(restrictedPreview, err)
	}
	if err = f.auth.RevokeSession(ctx, operator, restrictedToken); err != nil {
		t.Fatal(err)
	}
	if got, err := opener.PreviewRecord(ctx, restricted, in); !errors.Is(err, ErrUnauthorized) || got.Candidate.CandidateID != "" {
		t.Fatal("revoked", got, err)
	}
	if err = opener.DrainWorkspace(ctx, closeA); err != nil {
		t.Fatal(err)
	}
	if _, err = opener.PreviewRecord(ctx, writeA, in); !errors.Is(err, workspace.ErrLifecycle) {
		t.Fatal("preview reopened", err)
	}
	// Inspect persistent candidates only after physical drain and actual exclusion.
	func() {
		lease, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false)
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()
		h, err := sqlite.Open(ctx, filepath.Join(request.RootLocator, "data", "workspace.sqlite"), sqlite.ReadWriteExisting)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()
		for _, preview := range []workspace.Preview{first, next, changed, updated, restrictedPreview} {
			c := preview.Candidate
			var actor, resource, kind, epoch, title, body, hash, content, expiry, state string
			var revision, version int64
			if err = h.Conn.QueryRowContext(ctx, `SELECT principal_id,resource_id,kind,preview_epoch,title,body,candidate_digest,content_digest,expires_at,state,base_revision,preview_version FROM candidate WHERE workspace_id=? AND candidate_id=?`, created.WorkspaceID, c.CandidateID).Scan(&actor, &resource, &kind, &epoch, &title, &body, &hash, &content, &expiry, &state, &revision, &version); err != nil {
				t.Fatal(err)
			}
			if actor != c.PrincipalID || resource != c.ResourceID || kind != c.Kind || epoch != c.PreviewEpoch || title != c.Title || body != c.Body || hash != c.CandidateDigest || content != c.ContentDigest || expiry != c.ExpiresAt.UTC().Format(time.RFC3339Nano) || state != "previewed" || revision != c.BaseRevision || version != c.PreviewVersion {
				t.Fatal("persisted candidate differs", c.CandidateID)
			}
		}
		for _, sample := range []struct {
			table string
			count int
		}{{"candidate", 8}, {"text_record", 2}, {"text_version", 2}, {"save_operation", 2}, {"commit_receipt", 2}, {"audit_entry", 2}} {
			var n int
			if err = h.Conn.QueryRowContext(ctx, "SELECT count(*) FROM "+sample.table).Scan(&n); err != nil || n != sample.count {
				t.Fatal(sample.table, n, sample.count, err)
			}
		}
	}()
	fresh, err := opener.OpenWorkspace(ctx, openA, 1)
	if err != nil || fresh.Epoch == view.Epoch || fresh.Snapshot.Revision != 2 {
		t.Fatal(fresh, err)
	}
	if _, err = opener.PreviewRecord(ctx, writeA, in); !errors.Is(err, workspace.ErrStaleHandle) {
		t.Fatal("old epoch writes after reopen", err)
	}
	got, err = opener.ListRecords(ctx, readA, "", 100)
	if err != nil || len(got.Items) != 2 || got.Items[0] != original.Items[0] || got.Items[1] != original.Items[1] {
		t.Fatal("reopen applied previews", got, err)
	}
	t.Log("actual native leases + business previews + persistent candidates + physical close/reopen: formal Head/versions/receipts unchanged; no T019 save or full Host-process acceptance claim")
}
