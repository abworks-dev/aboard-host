//go:build windows && amd64

package acceptance

import (
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// These assertions are derived from the frozen OpenAPI responses/parameters,
// independently of the dispatch constants and of the generated DTO validator.
func TestV015IndependentHTTPBindings(t *testing.T) {
	t.Run("preview-status", func(t *testing.T) {
		f := newFixture(t)
		f.initialize(t)
		p := f.start(t)
		c := isolationConnect(t, p)
		_, v := newInspectionWorkspace(t, c, "binding-preview")
		v = c.open(t, v.WorkspaceId)
		var candidate protocol.CandidateView
		c.request(t, "POST", "/v1/workspaces/"+string(v.WorkspaceId)+"/record-previews", protocol.PreviewRecord{OpenEpoch: *v.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "binding-only candidate", Body: "not confirmed"}, &candidate, 201)
		if candidate.WorkspaceId != v.WorkspaceId || candidate.Validation.State != "valid" {
			t.Fatal("201 preview lost typed candidate/scope")
		}
		saveFacts(t, c, v.WorkspaceId, "0", 0)
		closePublic(t, c, v)
		p.stop(t)
		isolationDiagnostics(t, p, c.token)
		isolationEvidence(t, f, "binding-preview", struct {
			Status                        int
			Candidate                     protocol.UUID
			NoFormalEffect, AllHostsEnded bool
		}{201, candidate.CandidateId, true, true}, c.token)
	})
	t.Run("default-and-explicit-pagination", func(t *testing.T) {
		f := newFixture(t)
		f.initialize(t)
		p := f.start(t)
		c := isolationConnect(t, p)
		_, v := newInspectionWorkspace(t, c, "binding-pagination")
		v = c.open(t, v.WorkspaceId)
		expected := make([]protocol.TextRecordView, 0, 101)
		receipts := make([]protocol.Receipt, 0, 101)
		for i := 0; i < 101; i++ {
			saved := isolationSave(t, c, v, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: protocol.Revision(strconv.Itoa(i)), Title: protocol.Title("binding " + strconv.Itoa(i)), Body: protocol.Body("synthetic exact row " + strconv.Itoa(i))})
			expected = append(expected, saved.Row)
			receipts = append(receipts, saved.Receipt)
		}
		prefix := "/v1/workspaces/" + string(v.WorkspaceId)
		var page, explicit, tail, one protocol.RecordList
		c.request(t, "GET", prefix+"/records", nil, &page, 200)
		if page.WorkspaceId != v.WorkspaceId || len(page.Items) != 100 || page.NextCursor == nil || *page.NextCursor == "" {
			t.Fatal("OpenAPI default limit100 did not return first100 of101 with cursor")
		}
		c.request(t, "GET", prefix+"/records?limit=100", nil, &explicit, 200)
		if !reflect.DeepEqual(page, explicit) {
			t.Fatal("omitted limit differs from explicit contractual default100")
		}
		c.request(t, "GET", prefix+"/records?cursor="+url.QueryEscape(*page.NextCursor), nil, &tail, 200)
		if tail.WorkspaceId != v.WorkspaceId || len(tail.Items) != 1 || tail.NextCursor != nil {
			t.Fatal("default cursor pagination lost terminal row")
		}
		all := append(append([]protocol.TextRecordView{}, page.Items...), tail.Items...)
		isolationRows(t, all, expected...)
		c.request(t, "GET", prefix+"/records?limit=1", nil, &one, 200)
		if len(one.Items) != 1 || one.NextCursor == nil || one.Items[0] != page.Items[0] {
			t.Fatal("explicit smaller limit no longer honored")
		}
		var bad protocol.ErrorView
		c.request(t, "GET", prefix+"/records?limit=101", nil, &bad, 400)
		if bad.Outcome != "rejected" || bad.OperationId != nil || bad.CorrelationId == "" {
			t.Fatal("overmaximum limit invented business admission")
		}
		var view protocol.WorkspaceView
		c.request(t, "GET", prefix, nil, &view, 200)
		if view.Revision == nil || *view.Revision != "101" {
			t.Fatal("pagination changed confirmed head")
		}
		allowed := map[protocol.Capability]bool{protocol.CapabilityWorkspaceOpen: true, protocol.CapabilityWorkspaceClose: true, protocol.CapabilityWorkspaceRead: true, protocol.CapabilityWorkspaceWrite: true, protocol.CapabilityWorkspaceOperator: true, protocol.CapabilityTextRead: true, protocol.CapabilityTextPreview: true, protocol.CapabilityTextCommit: true}
		seen := map[protocol.Capability]bool{}
		for _, capability := range view.AvailableCapabilities {
			if !allowed[capability] || seen[capability] {
				t.Fatal("deferred or duplicate capability advertised")
			}
			seen[capability] = true
		}
		if !seen[protocol.CapabilityTextPreview] || !seen[protocol.CapabilityTextCommit] {
			t.Fatal("current writable text capabilities absent")
		}
		var unavailable protocol.ErrorView
		c.request(t, "GET", "/v1/runs", nil, &unavailable, 501)
		closePublic(t, c, v)
		p.stop(t)
		isolationDiagnostics(t, p, c.token)
		isolationEvidence(t, f, "binding-pagination", struct {
			Workspace                                 protocol.UUID
			Receipts                                  []protocol.Receipt
			Default, Explicit, Tail, Small            int
			EqualDefault, Exact101Rows, AllHostsEnded bool
			Capabilities                              []protocol.Capability
		}{v.WorkspaceId, receipts, 100, 100, 1, 1, true, true, true, view.AvailableCapabilities}, c.token)
	})
}
