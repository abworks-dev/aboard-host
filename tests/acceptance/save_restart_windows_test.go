//go:build windows && amd64

package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

type restartSaved struct {
	Candidate protocol.CandidateView
	Input     protocol.CommitRecord
	Receipt   protocol.Receipt
	Row       protocol.TextRecordView
}

type restartWorkspace struct {
	Root              string
	Opened            protocol.WorkspaceView
	Reopened          protocol.WorkspaceView
	History           []restartSaved
	Current           map[protocol.UUID]restartSaved
	UnconfirmedCreate protocol.CandidateView
	UnconfirmedUpdate protocol.CandidateView
	UnconfirmedInput  protocol.CommitRecord
	EditedFile        string
	EditedBytes       []byte
	ReopenedRows      []protocol.TextRecordView
	ReopenedClose     protocol.OperationView
}

// Read only the SQLite header, not SQL or an internal Host/store API. The file
// identity also proves A/B are distinct actual files and survive the restart.
func restartDatabase(t *testing.T, root string) os.FileInfo {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "data", "workspace.sqlite"))
	if err != nil {
		t.Fatal("formal SQLite file unavailable")
	}
	info, statErr := f.Stat()
	var header [16]byte
	_, readErr := io.ReadFull(f, header[:])
	closeErr := f.Close()
	if statErr != nil || readErr != nil || closeErr != nil || !info.Mode().IsRegular() || info.Size() <= 16 || string(header[:]) != "SQLite format 3\x00" {
		t.Fatal("formal file is not a physically readable SQLite database")
	}
	return info
}

func restartVerify(t *testing.T, c *client, view protocol.WorkspaceView, expected map[protocol.UUID]restartSaved) []protocol.TextRecordView {
	t.Helper()
	rows := saveFacts(t, c, view.WorkspaceId, "11", 10)
	seen := map[protocol.UUID]bool{}
	for _, row := range rows {
		want, ok := expected[row.ResourceId]
		if !ok || seen[row.ResourceId] {
			t.Fatal("unexpected, duplicate or foreign resource after formal save")
		}
		seen[row.ResourceId] = true
		saveMatches(t, row, want.Receipt, want.Candidate)
		if row != want.Row {
			t.Fatal("formal content/identity/version changed across a non-save boundary")
		}
		var individual protocol.TextRecordView
		c.request(t, "GET", "/v1/workspaces/"+string(view.WorkspaceId)+"/records/"+string(row.ResourceId), nil, &individual, 200)
		if individual != row {
			t.Fatal("fresh individual read differs from the actual complete list")
		}
	}
	if len(seen) != len(expected) {
		t.Fatal("confirmed resource missing")
	}
	return rows
}

// V011 uses only the normal independent production CLI/native authentication/
// public HTTP. Expectations are retained client-side for comparison, but every
// observation after restart is fetched over the NEW process/client connection.
// An ordinary edited JSON file is an inert candidate-shaped stimulus, not a
// supported file-import format or an invented generic Apply endpoint.
func TestV011IndependentCompleteSaveRestart(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	first := f.start(t)
	c := first.connect(t)
	var identity protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &identity, 200)
	if identity.InstallationId == "" || identity.TenantId == "" || identity.PrincipalId == "" {
		t.Fatal("actual context omitted a durable actor/scope identity")
	}
	workspaces := make([]restartWorkspace, 2)
	for i := range workspaces {
		w := &workspaces[i]
		w.Root, w.Opened = newInspectionWorkspace(t, c, fmt.Sprintf("v011-%d", i))
		w.Opened = c.open(t, w.Opened.WorkspaceId)
		w.Current = map[protocol.UUID]restartSaved{}
		if w.Opened.TenantId != identity.TenantId || w.Opened.OpenEpoch == nil {
			t.Fatal("created workspace/actor identity differs")
		}
	}
	if workspaces[0].Opened.WorkspaceId == workspaces[1].Opened.WorkspaceId {
		t.Fatal("A/B share workspace identity")
	}
	commit := func(w *restartWorkspace, candidate protocol.CandidateView) restartSaved {
		input := saveInput(newID(t), w.Opened, candidate)
		var receipt protocol.Receipt
		prefix := "/v1/workspaces/" + string(w.Opened.WorkspaceId)
		c.request(t, "POST", prefix+"/record-commits", input, &receipt, 200)
		if receipt.OperationId != input.OperationId {
			t.Fatal("confirmation is not associated with the submitted original key")
		}
		var row protocol.TextRecordView
		c.request(t, "GET", prefix+"/records/"+string(candidate.ResourceId), nil, &row, 200)
		saveMatches(t, row, receipt, candidate)
		saved := restartSaved{candidate, input, receipt, row}
		w.History = append(w.History, saved)
		w.Current[row.ResourceId] = saved
		return saved
	}
	// Interleave A/B creates with individually distinct Unicode/newline bodies.
	bodies := map[protocol.Body]bool{}
	for n := 0; n < 10; n++ {
		for i := range workspaces {
			w := &workspaces[i]
			body := protocol.Body(fmt.Sprintf("工作区%d / record %02d\n正文唯一 %d-%d — UTF-8\r\nlast line", i, n, i, n))
			if bodies[body] {
				t.Fatal("fixture bodies are not independent")
			}
			bodies[body] = true
			candidate := savePreview(t, c, w.Opened, protocol.PreviewRecord{Kind: "create-text", BaseRevision: protocol.Revision(strconv.Itoa(n)), Title: protocol.Title(fmt.Sprintf("record %02d", n)), Body: body})
			saveFacts(t, c, w.Opened.WorkspaceId, protocol.Revision(strconv.Itoa(n)), n)
			saved := commit(w, candidate)
			if saved.Receipt.NewRevision != protocol.PositiveRevision(strconv.Itoa(n+1)) || saved.Receipt.NewVersion != "1" {
				t.Fatal("create advanced the wrong workspace revision/version")
			}
		}
	}
	for i := range workspaces {
		w := &workspaces[i]
		original := w.History[0]
		candidate := savePreview(t, c, w.Opened, protocol.PreviewRecord{Kind: "update-text", ResourceId: &original.Receipt.ResourceId, BaseRevision: "10", BaseDigest: &original.Receipt.ContentDigest, Title: protocol.Title(fmt.Sprintf("updated %d", i)), Body: protocol.Body(fmt.Sprintf("updated workspace%d first record\n新正文：%d", i, i))})
		if candidate.Diff.BeforeBody == nil || *candidate.Diff.BeforeBody != original.Row.Body || candidate.Diff.BeforeVersion == nil || *candidate.Diff.BeforeVersion != "1" {
			t.Fatal("update preview did not bind the actual original formal content/version")
		}
		saveFacts(t, c, w.Opened.WorkspaceId, "10", 10)
		updated := commit(w, candidate)
		if updated.Receipt.ResourceId != original.Receipt.ResourceId || updated.Receipt.NewRevision != "11" || updated.Receipt.NewVersion != "2" {
			t.Fatal("update created another resource or wrong revision/version")
		}
		restartVerify(t, c, w.Opened, w.Current)
		w.UnconfirmedCreate = savePreview(t, c, w.Opened, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "11", Title: "unconfirmed create", Body: protocol.Body(fmt.Sprintf("unconfirmed A/B %d never formal", i))})
		w.UnconfirmedUpdate = savePreview(t, c, w.Opened, protocol.PreviewRecord{Kind: "update-text", ResourceId: &updated.Receipt.ResourceId, BaseRevision: "11", BaseDigest: &updated.Receipt.ContentDigest, Title: "unconfirmed update", Body: "unconfirmed replacement must not become formal"})
		w.UnconfirmedInput = saveInput(newID(t), w.Opened, w.UnconfirmedCreate)
		var missing protocol.ErrorView
		c.request(t, "GET", "/v1/workspaces/"+string(w.Opened.WorkspaceId)+"/records/"+string(w.UnconfirmedCreate.ResourceId), nil, &missing, 404)
		if missing.Code != protocol.ErrorCodeNOTFOUND {
			t.Fatal("unconfirmed creation became formal")
		}
		// Deliberately edit only an ordinary own-fixture candidate-shaped file.
		w.EditedFile = filepath.Join(w.Root, "record-candidate.json")
		raw, err := json.Marshal(protocol.PreviewRecord{OpenEpoch: *w.Opened.OpenEpoch, Kind: "update-text", ResourceId: &updated.Receipt.ResourceId, BaseRevision: "11", BaseDigest: &updated.Receipt.ContentDigest, Title: "file candidate", Body: "first file candidate"})
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(w.EditedFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal("synthetic candidate file creation")
		}
		_, writeErr := file.Write(raw)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal("candidate file was not physically closed")
		}
		before, err := os.ReadFile(w.EditedFile)
		if err != nil || !bytes.Equal(before, raw) {
			t.Fatal("candidate file baseline unavailable")
		}
		w.EditedBytes = bytes.Replace(before, []byte("first file candidate"), []byte("manually changed file candidate; no formal Apply"), 1)
		if bytes.Equal(before, w.EditedBytes) || os.WriteFile(w.EditedFile, w.EditedBytes, 0600) != nil {
			t.Fatal("synthetic file edit did not occur")
		}
		restartVerify(t, c, w.Opened, w.Current)
	}
	oldFiles := []os.FileInfo{restartDatabase(t, workspaces[0].Root), restartDatabase(t, workspaces[1].Root)}
	if os.SameFile(oldFiles[0], oldFiles[1]) {
		t.Fatal("two workspaces alias one SQLite file")
	}
	closeResults := make([]protocol.OperationView, 2)
	for i := range workspaces {
		closeResults[i] = closePublic(t, c, workspaces[i].Opened)
		if closeResults[i].Scope != "workspace" || closeResults[i].WorkspaceId == nil || *closeResults[i].WorkspaceId != workspaces[i].Opened.WorkspaceId {
			t.Fatal("close result crossed workspace scope")
		}
	}
	first.stop(t) // actual exit0, scanner join and stopped; never forced cleanup.
	second := f.start(t)
	d := second.connect(t)
	if first.cmd.Process.Pid == second.cmd.Process.Pid && first.birth == second.birth {
		t.Fatal("same kernel process reused as the restart oracle")
	}
	var identityAgain protocol.PrincipalView
	d.request(t, "GET", "/v1/context", nil, &identityAgain, 200)
	if identity.InstallationId != identityAgain.InstallationId || identity.TenantId != identityAgain.TenantId || identity.PrincipalId != identityAgain.PrincipalId {
		t.Fatal("restart changed installation/tenant/principal identity")
	}
	for i := range workspaces {
		w := &workspaces[i]
		w.Reopened = d.open(t, w.Opened.WorkspaceId)
		if w.Reopened.WorkspaceId != w.Opened.WorkspaceId || w.Reopened.TenantId != identityAgain.TenantId || w.Reopened.OpenEpoch == nil || *w.Reopened.OpenEpoch == *w.Opened.OpenEpoch {
			t.Fatal("restart changed workspace identity or reused old epoch")
		}
		if !os.SameFile(oldFiles[i], restartDatabase(t, w.Root)) {
			t.Fatal("restart silently replaced the actual formal database")
		}
		prefix := "/v1/workspaces/" + string(w.Opened.WorkspaceId)
		restartVerify(t, d, w.Reopened, w.Current)
		for _, saved := range w.History {
			var op protocol.OperationView
			d.request(t, "GET", prefix+"/operations/"+string(saved.Input.OperationId), nil, &op, 200)
			if op.Kind != "save" || op.PhysicalState != "drained" || op.Scope != "workspace" || op.WorkspaceId == nil || *op.WorkspaceId != w.Opened.WorkspaceId || op.OperationId != saved.Input.OperationId || op.LogicalState != "committed" || op.PersistenceState != "known" || op.Receipt == nil || *op.Receipt != saved.Receipt {
				t.Fatal("confirmed original history was lost or crossed scope")
			}
			var prior protocol.Receipt
			d.request(t, "POST", prefix+"/record-commits", saved.Input, &prior, 200)
			if prior != saved.Receipt {
				t.Fatal("old epoch original-key inquiry lost/replayed a confirmed save")
			}
		}
		var missing protocol.ErrorView
		d.request(t, "GET", prefix+"/records/"+string(w.UnconfirmedCreate.ResourceId), nil, &missing, 404)
		if missing.Code != protocol.ErrorCodeNOTFOUND {
			t.Fatal("restart applied an unconfirmed creation")
		}
		d.request(t, "POST", prefix+"/record-commits", w.UnconfirmedInput, &missing, 409)
		if missing.Code != protocol.ErrorCodeSTALEHANDLE {
			t.Fatal("new unconfirmed operation survived its old epoch")
		}
		actual, err := os.ReadFile(w.EditedFile)
		if err != nil || !bytes.Equal(actual, w.EditedBytes) {
			t.Fatal("restart consumed, rewrote or lost the ordinary edited candidate file")
		}
		w.ReopenedRows = restartVerify(t, d, w.Reopened, w.Current) // 22 old-key queries/negative stimuli caused zero new formal effects.
		w.ReopenedClose = closePublic(t, d, w.Reopened)
		if w.ReopenedClose.Scope != "workspace" || w.ReopenedClose.WorkspaceId == nil || *w.ReopenedClose.WorkspaceId != w.Reopened.WorkspaceId {
			t.Fatal("reopened close result crossed workspace scope")
		}
	}
	second.stop(t)
	matrixEvidence(t, f, "save-complete-restart", struct {
		Identity, IdentityAgain    protocol.PrincipalView
		Workspaces                 []restartWorkspace
		OldPID, NewPID             int
		OldBirth, NewBirth         uint64
		BinarySHA256               string
		CloseResults               []protocol.OperationView
		OldHostEnded, NewHostEnded bool
	}{identity, identityAgain, workspaces, first.cmd.Process.Pid, second.cmd.Process.Pid, first.birth, second.birth, f.sha, closeResults, true, true})
}
