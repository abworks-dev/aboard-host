//go:build windows && amd64

package acceptance

import (
	"os"
	"path/filepath"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

func TestHarnessSyntheticScopeGuards(t *testing.T) {
	for _, bad := range []string{logRoot(), filepath.Join(projectRoot(), "installation"), filepath.Join(logRoot()+"-outside", "t023-bad"), filepath.Join(logRoot(), "ordinary"), "t023-relative"} {
		if requireSyntheticRoot(bad) == nil {
			t.Fatal("unsafe synthetic fixture accepted")
		}
	}
	for _, bad := range []string{filepath.Join(projectRoot(), "host.exe"), filepath.Join(logRoot()+"-outside", "host.exe"), "host.exe"} {
		if _, err := checkedExecutable(bad); err == nil {
			t.Fatal("unowned executable accepted")
		}
	}
}

// A harness smoke test, NOT the full V011/T027 matrix: one actual external Host
// creates/saves, gracefully exits through the production interrupt path, then a
// distinct production process reopens and reads the original durable receipt.
func TestIndependentHostHarnessSmoke(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	first := f.start(t)
	c := first.connect(t)
	var identity protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &identity, 200)
	root := filepath.Join(f.config.AllowedRoots[0], "A")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	operation := newID(t)
	var created protocol.WorkspaceView
	c.request(t, "POST", "/v1/workspace-creations", protocol.CreateWorkspace{OperationId: operation, Path: protocol.Path(root), DisplayName: "external synthetic A"}, &created, 201)
	opened := c.open(t, created.WorkspaceId)
	if opened.OpenEpoch == nil {
		t.Fatal("open did not publish epoch")
	}
	var candidate protocol.CandidateView
	const body = "synthetic bytes saved by the separate production executable"
	c.request(t, "POST", "/v1/workspaces/"+string(created.WorkspaceId)+"/record-previews", protocol.PreviewRecord{OpenEpoch: *opened.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "external title", Body: body}, &candidate, 201)
	saveID := newID(t)
	commit := protocol.CommitRecord{OperationId: saveID, OpenEpoch: *opened.OpenEpoch, CandidateId: candidate.CandidateId, CandidateDigest: candidate.CandidateDigest, PreviewVersion: candidate.PreviewVersion}
	var receipt protocol.Receipt
	c.request(t, "POST", "/v1/workspaces/"+string(created.WorkspaceId)+"/record-commits", commit, &receipt, 200)
	first.stop(t)
	lease, err := platform.Acquire(f.config.InstallationRoot, platform.Installation, false, false)
	if err != nil {
		t.Fatal("actual installation exclusion did not end")
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	lease, err = platform.Acquire(root, platform.Workspace, false, false)
	if err != nil {
		t.Fatal("actual writer exclusion did not end")
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	second := f.start(t)
	d := second.connect(t)
	if first.cmd.Process.Pid == second.cmd.Process.Pid && first.birth == second.birth {
		t.Fatal("fresh process identity not distinct")
	}
	var identityAgain protocol.PrincipalView
	d.request(t, "GET", "/v1/context", nil, &identityAgain, 200)
	if identityAgain.InstallationId != identity.InstallationId || identityAgain.TenantId != identity.TenantId || identityAgain.PrincipalId != identity.PrincipalId {
		t.Fatal("durable installation identity changed")
	}
	reopened := d.open(t, created.WorkspaceId)
	if reopened.OpenEpoch == nil || *reopened.OpenEpoch == *opened.OpenEpoch {
		t.Fatal("fresh process did not publish a new epoch")
	}
	var record protocol.TextRecordView
	d.request(t, "GET", "/v1/workspaces/"+string(created.WorkspaceId)+"/records/"+string(receipt.ResourceId), nil, &record, 200)
	if record.Body != body || record.OperationId != saveID || record.ContentDigest != receipt.ContentDigest || record.WorkspaceRevision != receipt.NewRevision {
		t.Fatal("new process did not read the identical formal history")
	}
	var original protocol.Receipt
	d.request(t, "POST", "/v1/workspaces/"+string(created.WorkspaceId)+"/record-commits", commit, &original, 200)
	if original != receipt {
		t.Fatal("original key did not return original receipt")
	}
	var state protocol.WorkspaceView
	d.request(t, "GET", "/v1/workspaces/"+string(created.WorkspaceId), nil, &state, 200)
	if state.Revision == nil || *state.Revision != "1" {
		t.Fatal("same-key inquiry replayed business effects")
	}
	second.stop(t)
	t.Log("two distinct production processes; native protected session/preconditions; durable save, actual interrupt/drain/exit, new epoch and identical original receipt; only smoke scope, not full V008-V015")
}
