//go:build windows && amd64

package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

func savePreview(t *testing.T, c *client, opened protocol.WorkspaceView, in protocol.PreviewRecord) protocol.CandidateView {
	t.Helper()
	if opened.OpenEpoch == nil {
		t.Fatal("save fixture has no published epoch")
	}
	in.OpenEpoch = *opened.OpenEpoch
	var out protocol.CandidateView
	c.request(t, "POST", "/v1/workspaces/"+string(opened.WorkspaceId)+"/record-previews", in, &out, 201)
	if out.WorkspaceId != opened.WorkspaceId || out.Body != in.Body || out.Title != in.Title || out.Validation.State != "valid" {
		t.Fatal("preview changed scope/text/validation")
	}
	return out
}
func saveInput(key protocol.UUID, opened protocol.WorkspaceView, p protocol.CandidateView) protocol.CommitRecord {
	return protocol.CommitRecord{OperationId: key, OpenEpoch: *opened.OpenEpoch, CandidateId: p.CandidateId, CandidateDigest: p.CandidateDigest, PreviewVersion: p.PreviewVersion}
}
func saveFacts(t *testing.T, c *client, id protocol.UUID, revision protocol.Revision, rows int) []protocol.TextRecordView {
	t.Helper()
	prefix := "/v1/workspaces/" + string(id)
	var view protocol.WorkspaceView
	c.request(t, "GET", prefix, nil, &view, 200)
	if view.WorkspaceId != id || view.Revision == nil || *view.Revision != revision {
		t.Fatal("unexpected actual formal head")
	}
	var list protocol.RecordList
	c.request(t, "GET", prefix+"/records", nil, &list, 200)
	if list.WorkspaceId != id || len(list.Items) != rows || list.NextCursor != nil {
		t.Fatal("unexpected formal record set")
	}
	for _, row := range list.Items {
		if row.WorkspaceId != id {
			t.Fatal("foreign formal record")
		}
	}
	return list.Items
}
func saveMatches(t *testing.T, row protocol.TextRecordView, receipt protocol.Receipt, p protocol.CandidateView) {
	t.Helper()
	if receipt.WorkspaceId != p.WorkspaceId || receipt.ResourceId != p.ResourceId || row.WorkspaceId != receipt.WorkspaceId || row.ResourceId != receipt.ResourceId || row.OperationId != receipt.OperationId || row.Version != receipt.NewVersion || row.WorkspaceRevision != receipt.NewRevision || row.ContentDigest != receipt.ContentDigest || row.Title != p.Title || row.Body != p.Body || receipt.CandidateId != p.CandidateId || receipt.ContentDigest != p.ContentDigest {
		t.Fatal("actual formal record/receipt/candidate differ")
	}
}

// This is the normal production executable and public HTTP/native authentication,
// not a storage-clock/fault seam, SQL observer, or the full V010 fault matrix.
func TestV010IndependentSaveOriginalKeysAndScopes(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	first := f.start(t)
	c := first.connect(t)
	var identity protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &identity, 200)
	_, a := newInspectionWorkspace(t, c, "save-A")
	_, b := newInspectionWorkspace(t, c, "save-B")
	a = c.open(t, a.WorkspaceId)
	b = c.open(t, b.WorkspaceId)
	pa := "/v1/workspaces/" + string(a.WorkspaceId)
	pb := "/v1/workspaces/" + string(b.WorkspaceId)
	ca := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "same title", Body: "A: 原始正文\nnot B"})
	cb := savePreview(t, c, b, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "same title", Body: "B: 独立正文\nnot A"})
	abandoned := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "unconfirmed", Body: "must never become formal"})
	if ca.ResourceId == abandoned.ResourceId {
		t.Fatal("Host reused a resource identity within A")
	}
	saveFacts(t, c, a.WorkspaceId, "0", 0)
	saveFacts(t, c, b.WorkspaceId, "0", 0)
	var missing protocol.ErrorView
	c.request(t, "GET", pa+"/records/"+string(ca.ResourceId), nil, &missing, 404)
	if missing.Code != protocol.ErrorCodeNOTFOUND {
		t.Fatal("unconfirmed preview published a formal record")
	}
	key := newID(t)
	ia := saveInput(key, a, ca)
	ib := saveInput(key, b, cb)
	var ra, rb protocol.Receipt
	c.request(t, "POST", pa+"/record-commits", ia, &ra, 200)
	c.request(t, "POST", pb+"/record-commits", ib, &rb, 200)
	if ra.WorkspaceId != a.WorkspaceId || rb.WorkspaceId != b.WorkspaceId || ra.OperationId != key || rb.OperationId != key || ra.NewRevision != "1" || rb.NewRevision != "1" {
		t.Fatal("same scoped operation key crossed workspaces")
	}
	saveMatches(t, saveFacts(t, c, a.WorkspaceId, "1", 1)[0], ra, ca)
	saveMatches(t, saveFacts(t, c, b.WorkspaceId, "1", 1)[0], rb, cb)
	var prior protocol.Receipt
	versionInquiry := ia
	versionInquiry.PreviewVersion = "999"
	c.request(t, "POST", pa+"/record-commits", versionInquiry, &prior, 200)
	if prior != ra {
		t.Fatal("caller previewVersion overrode existing semantic key")
	}
	var conflict protocol.ErrorView
	c.request(t, "POST", pa+"/record-commits", saveInput(key, a, abandoned), &conflict, 409)
	if conflict.Code != protocol.ErrorCodeOPERATIONKEYREUSE {
		t.Fatal("changed candidate did not reject original key reuse")
	}
	foreign := ia
	foreign.OpenEpoch = *b.OpenEpoch
	c.request(t, "POST", pb+"/record-commits", foreign, &conflict, 409)
	if conflict.Code != protocol.ErrorCodeOPERATIONKEYREUSE {
		t.Fatal("foreign candidate leaked through B's existing scoped key")
	}
	foreign.OperationId = newID(t)
	c.request(t, "POST", pb+"/record-commits", foreign, &conflict, 409)
	applied := ia
	applied.OperationId = newID(t)
	c.request(t, "POST", pa+"/record-commits", applied, &conflict, 409)
	for _, item := range []struct {
		prefix  string
		receipt protocol.Receipt
	}{{pa, ra}, {pb, rb}} {
		var op protocol.OperationView
		c.request(t, "GET", item.prefix+"/operations/"+string(key), nil, &op, 200)
		if op.Scope != "workspace" || op.WorkspaceId == nil || *op.WorkspaceId != item.receipt.WorkspaceId || op.LogicalState != "committed" || op.PersistenceState != "known" || op.Receipt == nil || *op.Receipt != item.receipt {
			t.Fatal("original operation inquiry crossed scope/result")
		}
	}
	closePublic(t, c, a)
	closePublic(t, c, b)
	first.stop(t)
	second := f.start(t)
	d := second.connect(t)
	if first.cmd.Process.Pid == second.cmd.Process.Pid && first.birth == second.birth {
		t.Fatal("restart did not change kernel process identity")
	}
	var identityAgain protocol.PrincipalView
	d.request(t, "GET", "/v1/context", nil, &identityAgain, 200)
	if identity.InstallationId != identityAgain.InstallationId || identity.TenantId != identityAgain.TenantId || identity.PrincipalId != identityAgain.PrincipalId {
		t.Fatal("restart changed durable actor/scope")
	}
	aa := d.open(t, a.WorkspaceId)
	bb := d.open(t, b.WorkspaceId)
	if aa.OpenEpoch == nil || bb.OpenEpoch == nil || *aa.OpenEpoch == *a.OpenEpoch || *bb.OpenEpoch == *b.OpenEpoch {
		t.Fatal("restart reused a retired epoch")
	}
	for _, item := range []struct {
		prefix  string
		input   protocol.CommitRecord
		receipt protocol.Receipt
	}{{pa, versionInquiry, ra}, {pb, ib, rb}} {
		d.request(t, "POST", item.prefix+"/record-commits", item.input, &prior, 200)
		if prior != item.receipt {
			t.Fatal("old epoch/base/applied inquiry replayed or lost original receipt")
		}
	}
	stale := saveInput(newID(t), a, abandoned)
	d.request(t, "POST", pa+"/record-commits", stale, &conflict, 409)
	if conflict.Code != protocol.ErrorCodeSTALEHANDLE {
		t.Fatal("new operation accepted a retired epoch")
	}
	d.request(t, "GET", pa+"/records/"+string(abandoned.ResourceId), nil, &missing, 404)
	saveMatches(t, saveFacts(t, d, a.WorkspaceId, "1", 1)[0], ra, ca)
	saveMatches(t, saveFacts(t, d, b.WorkspaceId, "1", 1)[0], rb, cb)
	second.stop(t)
	matrixEvidence(t, f, "save-original-key-scopes", struct {
		Identity                             protocol.PrincipalView
		A, B                                 protocol.UUID
		SharedKey                            protocol.UUID
		AReceipt, BReceipt                   protocol.Receipt
		Abandoned                            protocol.UUID
		EpochA, EpochB, NewEpochA, NewEpochB protocol.UUID
	}{identity, a.WorkspaceId, b.WorkspaceId, key, ra, rb, abandoned.CandidateId, *a.OpenEpoch, *b.OpenEpoch, *aa.OpenEpoch, *bb.OpenEpoch})
}

type saveWireResult struct {
	Status  int
	Receipt protocol.Receipt
	Failure protocol.ErrorView
	Err     error
}

// No testing.Fatal in worker goroutines. Always join both actual HTTP exchanges
// before inspecting results, closing the Host, or releasing the client lifetime.
func saveWire(ctx context.Context, c *client, prefix string, in protocol.CommitRecord) saveWireResult {
	var result saveWireResult
	raw, err := json.Marshal(in)
	if err != nil {
		result.Err = errors.New("generated commit invalid")
		return result
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://"+c.authority+prefix+"/record-commits", bytes.NewReader(raw))
	if err != nil {
		result.Err = errors.New("save request unavailable")
		return result
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	reply, err := c.http.Do(req)
	if err != nil {
		result.Err = errors.New("save exchange failed; credential withheld")
		return result
	}
	result.Status = reply.StatusCode
	body, readErr := io.ReadAll(io.LimitReader(reply.Body, 64*protocol.RequestMaxBytes+1))
	endErr := reply.Body.Close()
	if readErr != nil || endErr != nil || len(body) > 64*protocol.RequestMaxBytes {
		result.Err = errors.New("save response did not end")
		return result
	}
	if result.Status == 200 {
		err = json.Unmarshal(body, &result.Receipt)
	} else {
		err = json.Unmarshal(body, &result.Failure)
	}
	if err != nil {
		result.Err = errors.New("generated save response invalid")
	}
	return result
}
func TestV010IndependentSameOldBaseHasOneWinner(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	p := f.start(t)
	c := p.connect(t)
	_, a := newInspectionWorkspace(t, c, "same-base")
	a = c.open(t, a.WorkspaceId)
	prefix := "/v1/workspaces/" + string(a.WorkspaceId)
	original := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "r1", Body: "original before concurrent updates"})
	var first protocol.Receipt
	c.request(t, "POST", prefix+"/record-commits", saveInput(newID(t), a, original), &first, 200)
	left := savePreview(t, c, a, protocol.PreviewRecord{Kind: "update-text", ResourceId: &first.ResourceId, BaseRevision: "1", BaseDigest: &first.ContentDigest, Title: "left", Body: "left old-base candidate"})
	right := savePreview(t, c, a, protocol.PreviewRecord{Kind: "update-text", ResourceId: &first.ResourceId, BaseRevision: "1", BaseDigest: &first.ContentDigest, Title: "right", Body: "right old-base candidate"})
	for _, preview := range []protocol.CandidateView{left, right} {
		if preview.Diff.BeforeBody == nil || *preview.Diff.BeforeBody != original.Body {
			t.Fatal("concurrent previews did not share the original formal baseline")
		}
	}
	saveMatches(t, saveFacts(t, c, a.WorkspaceId, "1", 1)[0], first, original)
	inputs := []protocol.CommitRecord{saveInput(newID(t), a, left), saveInput(newID(t), a, right)}
	candidates := []protocol.CandidateView{left, right}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	start := make(chan struct{})
	ready := make(chan struct{}, 2)
	type indexed struct {
		Index  int
		Result saveWireResult
	}
	ended := make(chan indexed, 2)
	for index, in := range inputs {
		peer := *c
		transport := &http.Transport{DisableKeepAlives: true}
		peer.http = &http.Client{Transport: transport, Timeout: 15 * time.Second}
		defer transport.CloseIdleConnections()
		go func(index int, in protocol.CommitRecord, peer client) {
			ready <- struct{}{}
			<-start
			ended <- indexed{index, saveWire(ctx, &peer, prefix, in)}
		}(index, in, peer)
	}
	<-ready
	<-ready
	close(start)
	results := make([]saveWireResult, 2)
	for range inputs {
		one := <-ended
		results[one.Index] = one.Result
	}
	winner, loser := -1, -1
	for i, result := range results {
		if result.Err != nil {
			t.Fatal("concurrent HTTP exchange failed (details withheld)")
		}
		switch result.Status {
		case 200:
			if winner != -1 {
				t.Fatal("same old base committed twice")
			}
			winner = i
		case 409:
			if loser != -1 {
				t.Fatal("same old base had no successful control")
			}
			loser = i
		default:
			t.Fatalf("unexpected concurrent save status %d", result.Status)
		}
	}
	if winner < 0 || loser < 0 || results[loser].Failure.Code != protocol.ErrorCodeSTALEBASELINE {
		t.Fatal("same-base race lacked one confirmed and one stale rejection")
	}
	receipt := results[winner].Receipt
	if receipt.ResourceId != first.ResourceId || receipt.NewVersion != "2" || receipt.NewRevision != "2" || receipt.OperationId != inputs[winner].OperationId {
		t.Fatal("winner advanced formal history incorrectly")
	}
	saveMatches(t, saveFacts(t, c, a.WorkspaceId, "2", 1)[0], receipt, candidates[winner])
	var history protocol.ErrorView
	c.request(t, "GET", prefix+"/operations/"+string(inputs[loser].OperationId), nil, &history, 409)
	// The ordinary owner serializes full requests: the loser is rejected by
	// Admit's baseline check BEFORE an admission row exists. Missing history
	// is unavailable/unknown, not a fabricated rejection or rollback receipt.
	if history.Code != protocol.ErrorCodeOPERATIONHISTORYUNAVAILABLE || history.OperationId == nil || *history.OperationId != inputs[loser].OperationId {
		t.Fatal("unadmitted loser acquired fabricated durable history")
	}
	var rejection protocol.ErrorView
	c.request(t, "POST", prefix+"/record-commits", inputs[loser], &rejection, 409)
	if rejection.Code != protocol.ErrorCodeSTALEBASELINE {
		t.Fatal("same loser key did not preserve rejection")
	}
	var prior protocol.Receipt
	c.request(t, "POST", prefix+"/record-commits", inputs[winner], &prior, 200)
	if prior != receipt {
		t.Fatal("same winner key changed receipt")
	}
	closePublic(t, c, a)
	p.stop(t)
	next := f.start(t)
	d := next.connect(t)
	reopened := d.open(t, a.WorkspaceId)
	if reopened.OpenEpoch == nil || *reopened.OpenEpoch == *a.OpenEpoch {
		t.Fatal("fresh process did not replace epoch")
	}
	d.request(t, "POST", prefix+"/record-commits", inputs[winner], &prior, 200)
	if prior != receipt {
		t.Fatal("restart replayed winner")
	}
	d.request(t, "POST", prefix+"/record-commits", inputs[loser], &rejection, 409)
	if rejection.Code != protocol.ErrorCodeSTALEHANDLE {
		t.Fatal("unadmitted request crossed a retired epoch")
	}
	currentLoser := inputs[loser]
	currentLoser.OpenEpoch = *reopened.OpenEpoch
	d.request(t, "POST", prefix+"/record-commits", currentLoser, &rejection, 409)
	if rejection.Code != protocol.ErrorCodeSTALEHANDLE {
		// The immutable candidate itself also belongs to the retired epoch;
		// changing only the command epoch cannot silently rebase that candidate.
		t.Fatal("old preview was rebound to a fresh epoch")
	}
	saveMatches(t, saveFacts(t, d, a.WorkspaceId, "2", 1)[0], receipt, candidates[winner])
	next.stop(t)
	matrixEvidence(t, f, "save-same-old-base", struct {
		Workspace     protocol.UUID
		Inputs        []protocol.CommitRecord
		Winner, Loser int
		Receipt       protocol.Receipt
		Rejected      protocol.ErrorCode
	}{a.WorkspaceId, inputs, winner, loser, receipt, results[loser].Failure.Code})
}
