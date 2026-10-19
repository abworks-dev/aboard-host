//go:build windows && amd64

package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func factsScope() workspace.Scope {
	return workspace.Scope{InstallationID: testInstallation, TenantID: testTenant, WorkspaceID: testWorkspace}
}

func TestWorkspaceFactsFormalOwnerAndReopen(t *testing.T) {
	ctx := context.Background()
	d, g := newTestGate(t)
	defer g.EndPhysical()
	scope := factsScope()
	f, err := NewWorkspaceFacts(g, scope)
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.Snapshot(ctx, scope)
	if err != nil || before.Revision != 0 || before.Identity.WorkspaceID != scope.WorkspaceID || before.Identity.TenantID != scope.TenantID {
		t.Fatalf("genesis %+v %v", before, err)
	}
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	// A saved preview is NOT a formal resource.
	if err := g.PutCandidate(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Current(ctx, scope, testResource); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("preview became formal: %v", err)
	}
	res, a, err := g.Admit(ctx, r, c.ExpiresAt.Add(-1))
	if err != nil || a == nil || res.Status != Accepted {
		t.Fatalf("admit %+v %v", res, err)
	}
	committed, err := g.CommitAccepted(ctx, a)
	if err != nil || committed.Status != Confirmed {
		t.Fatalf("commit %+v %v", committed, err)
	}
	record, err := f.Current(ctx, scope, testResource)
	if err != nil || record.Body != c.Body || record.Title != c.Title || record.ContentDigest != c.ContentDigest || record.Revision != 1 || record.Version != 1 || record.CreatedBy != testPrincipal || record.OperationID != r.OperationID {
		t.Fatalf("formal %+v %v", record, err)
	}
	snapshot, err := f.Snapshot(ctx, scope)
	if err != nil || snapshot.Revision != 1 {
		t.Fatalf("head %+v %v", snapshot, err)
	}
	list, err := f.List(ctx, scope, "", 100)
	if err != nil || len(list) != 1 || !reflect.DeepEqual(list[0], record) {
		t.Fatalf("list %+v %v", list, err)
	}
	list, err = f.List(ctx, scope, testResource, 1)
	if err != nil || len(list) != 0 {
		t.Fatalf("exclusive cursor %+v %v", list, err)
	}
	if err := g.EndPhysical(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Snapshot(ctx, scope); !errors.Is(err, ErrFenced) {
		t.Fatalf("old view used after close %v", err)
	}
	reopened, err := OpenDatabase(ctx, d.path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
	if err != nil {
		t.Fatal(err)
	}
	next, err := NewCommitGate(reopened, "new-facts-epoch")
	if err != nil {
		t.Fatal(err)
	}
	defer next.EndPhysical()
	fresh, err := NewWorkspaceFacts(next, scope)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fresh.Current(ctx, scope, testResource)
	if err != nil || !reflect.DeepEqual(got, record) {
		t.Fatalf("fresh physical connection %+v %v", got, err)
	}
	t.Log("same real SQLite owner: preview not formal; receipt-backed bytes/head; physical close/new connection read matches (not a Host-process acceptance)")
}

func TestWorkspaceFactsScopeAndIsolation(t *testing.T) {
	ctx := context.Background()
	d, g := newTestGate(t)
	defer g.EndPhysical()
	scope := factsScope()
	f, err := NewWorkspaceFacts(g, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []workspace.Scope{{}, {InstallationID: testTenant, TenantID: testTenant, WorkspaceID: testWorkspace}, {InstallationID: testInstallation, TenantID: testInstallation, WorkspaceID: testWorkspace}, {InstallationID: testInstallation, TenantID: testTenant, WorkspaceID: testPrincipal}} {
		if _, err := f.Snapshot(ctx, bad); !errors.Is(err, workspace.ErrScope) {
			t.Fatalf("wrong scope snapshot %v", err)
		}
		if _, err := f.Current(ctx, bad, testResource); !errors.Is(err, workspace.ErrScope) {
			t.Fatalf("wrong scope current %v", err)
		}
		if _, err := f.List(ctx, bad, "", 1); !errors.Is(err, workspace.ErrScope) {
			t.Fatalf("wrong scope list %v", err)
		}
	}
	if _, err := NewWorkspaceFacts(nil, scope); !errors.Is(err, workspace.ErrScope) {
		t.Fatal(err)
	}
	wrong := scope
	wrong.TenantID = testInstallation
	if _, err := NewWorkspaceFacts(g, wrong); !errors.Is(err, workspace.ErrScope) {
		t.Fatalf("foreign tenant bound %v", err)
	}
	if _, err := f.Current(ctx, scope, "not-uuid"); err == nil {
		t.Fatal("invalid resource key")
	}
	for _, n := range []int{-1, 0, 101} {
		if _, err := f.List(ctx, scope, "", n); err == nil {
			t.Fatal("invalid limit", n)
		}
	}
	if _, err := f.List(ctx, scope, "forged cursor", 1); err == nil {
		t.Fatal("invalid cursor")
	}
	// Separate actual database B has the same resource key but different bytes.
	b, err := OpenDatabase(ctx, filepath.Join(filepath.Dir(d.path), "second.sqlite"), CreateNew, Genesis{Kind: WorkspaceDatabase, WorkspaceID: testPrincipal, TenantID: testTenant, DisplayName: "B"})
	if err != nil {
		t.Fatal(err)
	}
	bg, err := NewCommitGate(b, "epoch-b")
	if err != nil {
		t.Fatal(err)
	}
	defer bg.EndPhysical()
	bs := scope
	bs.WorkspaceID = testPrincipal
	bf, err := NewWorkspaceFacts(bg, bs)
	if err != nil {
		t.Fatal(err)
	}
	bc := Candidate{WorkspaceID: bs.WorkspaceID, CandidateID: testCandidate, PrincipalID: testPrincipal, ResourceID: testResource, Kind: "create-text", Title: "B", Body: "B only", PreviewVersion: 1, PreviewEpoch: bg.epoch, ExpiresAt: time.Now().Add(time.Hour)}
	bc.ContentDigest, bc.CandidateDigest, err = candidateHashes(bc)
	if err != nil {
		t.Fatal(err)
	}
	br := SaveRequest{InstallationID: testInstallation, TenantID: testTenant, WorkspaceID: bs.WorkspaceID, PrincipalID: testPrincipal, OperationID: testOperation, CandidateID: bc.CandidateID, CandidateDigest: bc.CandidateDigest, Epoch: bg.epoch, PreviewVersion: bc.PreviewVersion}
	ba := prepareAdmission(t, bg, bc, br)
	if saved, err := bg.CommitAccepted(ctx, ba); err != nil || saved.Status != Confirmed {
		t.Fatalf("B %+v %v", saved, err)
	}
	if _, err := f.Current(ctx, scope, testResource); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("B leaked into A %v", err)
	}
	if err := g.EndPhysical(); err != nil {
		t.Fatal(err)
	}
	got, err := bf.Current(ctx, bs, testResource)
	if err != nil || got.Body != "B only" {
		t.Fatalf("close A broke B %+v %v", got, err)
	}
	bg.mu.Lock()
	bg.fenced = true
	bg.mu.Unlock()
	if _, err := bf.Current(ctx, bs, testResource); !errors.Is(err, ErrFenced) {
		t.Fatalf("unknown ordinary read trusted %v", err)
	}
	t.Log("separate real databases, same resource key, explicit scopes; closing A leaves B readable; no authorization or Host-lifecycle claim")
}
