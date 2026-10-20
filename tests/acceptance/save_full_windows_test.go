//go:build windows && amd64

package acceptance

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/digest"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

type fullPagerEvidence struct {
	PageSize, PageCount, FreePages, OldCap, EffectiveCap, RestoredCap int64
	InputDigest                                                       string
	Armed, OriginalReturned, Restored                                 bool
	SQLiteCode                                                        int
}
type busyReadEvidence struct {
	InputDigest                                                                             string
	ReaderStarted, SnapshotHeld, OriginalReturned, ReaderEnded, ReadOnlyURI, CommitReturned bool
	ReaderHead                                                                              int64
	QueryOnly, SQLiteCode, CommitCalls, CommitCode                                          int
}
type fullSaveEvidence struct {
	saveNativeReport
	Full fullPagerEvidence
	Busy busyReadEvidence
}

func loadFullSaveEvidence(root string) (fullSaveEvidence, error) {
	return loadSavePressureEvidence(root, "full")
}
func loadSavePressureEvidence(root, phase string) (fullSaveEvidence, error) {
	var out fullSaveEvidence
	if requireSyntheticRoot(root) != nil || (phase != "full" && phase != "busy") {
		return out, fmt.Errorf("full fixture unavailable")
	}
	path := filepath.Join(root, "save-"+phase+"-evidence.json")
	stat, e := os.Lstat(path)
	if e != nil || !stat.Mode().IsRegular() || stat.Size() > 4096 {
		return out, fmt.Errorf("full evidence unavailable")
	}
	f, e := os.Open(path)
	if e != nil {
		return out, e
	}
	raw, re := io.ReadAll(io.LimitReader(f, 4097))
	ce := f.Close()
	if re != nil || ce != nil || len(raw) > 4096 || json.Unmarshal(raw, &out) != nil || out.Version != 1 {
		return out, fmt.Errorf("full evidence invalid")
	}
	return out, nil
}
func fullSaveFaultAllowed(in faultInput) bool {
	if in.Phase != "save-full" || in.OperationID == "" {
		return false
	}
	r, e := loadFullSaveEvidence(in.Fixture)
	return e == nil && r.PID == in.PID && r.OperationID == in.OperationID && r.Phase == "full" && r.Status == "unknown" && r.Unknown && r.SQLiteCode == 13 && r.Full.SQLiteCode == 13 && r.Full.Armed && r.Full.OriginalReturned && r.Full.Restored && r.Full.FreePages == 0 && r.Full.PageCount > 0 && r.Full.EffectiveCap == r.Full.PageCount && r.Full.RestoredCap == r.Full.OldCap
}

func busySaveFaultAllowed(in faultInput) bool {
	if in.Phase != "save-busy" || in.OperationID == "" {
		return false
	}
	r, e := loadSavePressureEvidence(in.Fixture, "busy")
	b := r.Busy
	return e == nil && r.PID == in.PID && r.OperationID == in.OperationID && r.Phase == "busy" && r.Status == "unknown" && r.Unknown && r.SQLiteCode == 5 && b.SQLiteCode == 5 && b.CommitCalls == 1 && b.CommitReturned && b.CommitCode == 5 && b.ReadOnlyURI && b.QueryOnly == 1 && b.ReaderStarted && b.SnapshotHeld && b.OriginalReturned && b.ReaderEnded
}

// This limits only the original pager after durable admission; it neither fills
// the machine disk nor replaces the SQLite error with a synthetic Go error.
func TestV010IndependentSavePagerFULL(t *testing.T)  { testIndependentSavePressure(t, "full") }
func TestV010IndependentSaveSQLiteBusy(t *testing.T) { testIndependentSavePressure(t, "busy") }
func testIndependentSavePressure(t *testing.T, phase string) {
	f := newFixture(t)
	f.initialize(t)
	setup := f.start(t)
	initial := setup.connect(t)
	var actor protocol.PrincipalView
	initial.request(t, "GET", "/v1/context", nil, &actor, 200)
	root, a := newInspectionWorkspace(t, initial, "save-target")
	_, b := newInspectionWorkspace(t, initial, "independent-B")
	setup.stop(t)
	key := newID(t)
	p := saveNativeFixture(t, f, key, a.WorkspaceId, actor.PrincipalId, phase, 0).start(t)
	c := p.connect(t)
	joinOriginalSessionDelivery(t, c)
	a = c.open(t, a.WorkspaceId)
	b = c.open(t, b.WorkspaceId)
	prefix := "/v1/workspaces/" + string(a.WorkspaceId)
	body := strings.Repeat("X", 65536)
	if phase == "busy" {
		body = "small original business write held by genuine SHARED reader"
	}
	candidate := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: protocol.Title("real storage " + phase), Body: protocol.Body(body)})
	saveFacts(t, c, a.WorkspaceId, "0", 0)
	input := saveInput(key, a, candidate)
	var refusal protocol.ErrorView
	status := c.request(t, "POST", prefix+"/record-commits", input, &refusal, 0)
	report, e := loadSavePressureEvidence(f.root, phase)
	if e != nil {
		t.Fatal(e)
	}
	expected, e := digest.Save(digest.SaveInput{InstallationID: actor.InstallationId, TenantID: actor.TenantId, PrincipalID: actor.PrincipalId, WorkspaceID: a.WorkspaceId, OperationID: key, CandidateID: candidate.CandidateId, CandidateDigest: candidate.CandidateDigest})
	if e != nil {
		t.Fatal(e)
	}
	if report.PID != p.cmd.Process.Pid || report.InstallationID != string(actor.InstallationId) || report.TenantID != string(actor.TenantId) || report.WorkspaceID != string(a.WorkspaceId) || report.PrincipalID != string(actor.PrincipalId) || report.OperationID != string(key) || report.Epoch != string(*a.OpenEpoch) || report.Phase != phase || report.Ordinal != 0 {
		t.Fatal("actual original accepted pager FULL13/restore/scope proof missing")
	}
	if phase == "full" {
		if report.SQLiteCode != 13 || report.Full.SQLiteCode != 13 || !report.Full.Armed || !report.Full.OriginalReturned || !report.Full.Restored || report.Full.PageSize < 512 || report.Full.FreePages != 0 || report.Full.EffectiveCap != report.Full.PageCount || report.Full.OldCap <= report.Full.PageCount || report.Full.RestoredCap != report.Full.OldCap || report.Full.InputDigest != string(expected) {
			t.Fatal("actual original FULL13/cap restore proof missing")
		}
	} else {
		busy := report.Busy
		if report.Status != "unknown" || !report.Unknown || report.SQLiteCode != 5 || busy.SQLiteCode != 5 || busy.InputDigest != string(expected) || !busy.ReadOnlyURI || busy.QueryOnly != 1 || !busy.ReaderStarted || !busy.SnapshotHeld || !busy.OriginalReturned || !busy.ReaderEnded || busy.ReaderHead != 0 || busy.CommitCalls != 1 || !busy.CommitReturned || busy.CommitCode != 5 {
			t.Fatal("actual original COMMIT5/read-only SHARED proof missing")
		}
	}
	if report.Status != "unknown" && report.Status != "rejected" {
		t.Fatal("failed FULL attempt falsely confirmed")
	}
	if refusal.OperationId == nil || *refusal.OperationId != key || refusal.Scope == nil || *refusal.Scope != "workspace" {
		t.Fatal("FULL lost original operation scope")
	}
	var original protocol.OperationView
	c.request(t, "GET", prefix+"/operations/"+string(key), nil, &original, 200)
	if original.Scope != "workspace" || original.WorkspaceId == nil || *original.WorkspaceId != a.WorkspaceId || original.OperationId != key || original.Receipt != nil || original.LogicalState == "committed" {
		t.Fatal("formal FULL lost admission or fabricated receipt")
	}
	if report.Status == "unknown" {
		if original.PersistenceState != "unknown" || (original.LogicalState != "unknown" && original.LogicalState != "accepted") {
			t.Fatal("retained FULL uncertainty fabricated a rejection")
		}
		if !report.Unknown || status != 503 || refusal.Code != protocol.ErrorCodeCOMMITUNKNOWN || refusal.Outcome != "unknown" || refusal.RetryPolicy != "investigate" {
			t.Fatal("FULL cleanup uncertainty misreported as rejection/retry")
		}
		var denied protocol.ErrorView
		c.request(t, "GET", prefix+"/records", nil, &denied, 409)
	} else {
		if report.Unknown || status != 409 || refusal.Outcome != "rejected" {
			t.Fatal("known FULL rejection mislabeled")
		}
		saveFacts(t, c, a.WorkspaceId, "0", 0)
	}
	// Same key must be investigation only, never cap/apply again.
	var again json.RawMessage
	againStatus := c.request(t, "POST", prefix+"/record-commits", input, &again, 0)
	if report.Status == "unknown" {
		switch againStatus {
		case 202:
			var inquiry protocol.OperationView
			if json.Unmarshal(again, &inquiry) != nil || inquiry.Scope != "workspace" || inquiry.OperationId != key || inquiry.WorkspaceId == nil || *inquiry.WorkspaceId != a.WorkspaceId || inquiry.PersistenceState != "unknown" || inquiry.Receipt != nil || (inquiry.LogicalState != "unknown" && inquiry.LogicalState != "accepted") {
				t.Fatal("same-key unknown inquiry published success or crossed scope")
			}
		case 503:
			var inquiry protocol.ErrorView
			if json.Unmarshal(again, &inquiry) != nil || inquiry.Code != protocol.ErrorCodeCOMMITUNKNOWN || inquiry.OperationId == nil || *inquiry.OperationId != key || inquiry.Scope == nil || *inquiry.Scope != "workspace" || inquiry.Outcome != "unknown" || inquiry.RetryPolicy != "investigate" {
				t.Fatal("same-key retained uncertainty changed association or retry policy")
			}
		default:
			t.Fatal("unknown key became success or ordinary rejection")
		}
	} else if againStatus != 409 {
		t.Fatal("known rejected key became success")
	}
	if after, e := loadSavePressureEvidence(f.root, phase); e != nil || after != report {
		t.Fatal("same-key request rearmed FULL")
	}
	lease, e := platform.Acquire(root, platform.Workspace, false, false)
	if e == nil {
		_ = lease.Close()
		t.Fatal("original active/unknown owner released writer")
	}
	independent := savePreview(t, c, b, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B independent of pager cap", Body: "same Host B actual commit"})
	var br protocol.Receipt
	c.request(t, "POST", "/v1/workspaces/"+string(b.WorkspaceId)+"/record-commits", saveInput(newID(t), b, independent), &br, 200)
	saveMatches(t, saveFacts(t, c, b.WorkspaceId, "1", 1)[0], br, independent)
	if report.Status == "unknown" {
		p.terminateAt(t, "save-"+phase, string(key))
	} else {
		p.stop(t)
	}
	lease, e = platform.Acquire(root, platform.Workspace, false, false)
	if e != nil {
		t.Fatal("original process not physically ended")
	}
	if lease.Close() != nil {
		t.Fatal("post-exit lease observation failed")
	}
	next := f.start(t)
	d := next.connect(t)
	var freshActor protocol.PrincipalView
	d.request(t, "GET", "/v1/context", nil, &freshActor, 200)
	if freshActor.InstallationId != actor.InstallationId || freshActor.TenantId != actor.TenantId || freshActor.PrincipalId != actor.PrincipalId {
		t.Fatal("cold process changed original authority scope")
	}
	pre := registrationPrecondition(t, d, a.WorkspaceId, "open")
	var raw json.RawMessage
	openStatus := d.request(t, "POST", prefix+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, &raw, 0)
	if openStatus != 200 && openStatus != 503 {
		t.Fatal("fresh normal open refused after pager-only fault")
	}
	if openStatus == 503 {
		var v protocol.ErrorView
		if json.Unmarshal(raw, &v) != nil || v.Code != protocol.ErrorCodeCOMMITUNKNOWN {
			t.Fatal("cold pending history misprojected")
		}
	}
	var cold, resolved protocol.OperationView
	d.request(t, "GET", prefix+"/operations/"+string(key), nil, &cold, 200)
	if cold.OperationId != key || cold.WorkspaceId == nil || *cold.WorkspaceId != a.WorkspaceId || cold.Receipt != nil || cold.LogicalState == "committed" {
		t.Fatal("cold FULL admission absent or business replayed")
	}
	d.request(t, "POST", prefix+"/operations/"+string(key)+"/reconcile", nil, &resolved, 200)
	if resolved.OperationId != key || resolved.WorkspaceId == nil || *resolved.WorkspaceId != a.WorkspaceId || resolved.LogicalState != "rejected" || resolved.PersistenceState != "known" || resolved.Receipt != nil {
		t.Fatal("fresh exclusive FULL history not resolved without replay")
	}
	var fresh protocol.WorkspaceView
	if openStatus == 503 {
		fresh = d.open(t, a.WorkspaceId)
	} else if json.Unmarshal(raw, &fresh) != nil {
		t.Fatal("cold open missing current view")
	}
	if fresh.WorkspaceId != a.WorkspaceId || fresh.OpenEpoch == nil || *fresh.OpenEpoch == *a.OpenEpoch {
		t.Fatal("new owner reused old write epoch")
	}
	saveFacts(t, d, a.WorkspaceId, "0", 0)
	d.request(t, "POST", prefix+"/record-commits", input, &refusal, 409)
	saveFacts(t, d, a.WorkspaceId, "0", 0)
	d.open(t, b.WorkspaceId)
	saveMatches(t, saveFacts(t, d, b.WorkspaceId, "1", 1)[0], br, independent)
	next.stop(t)
	matrixEvidence(t, f, "save-"+phase, struct {
		Report                   fullSaveEvidence
		InitialStatus, ColdOpen  int
		Original, Cold, Resolved protocol.OperationView
		BReceipt                 protocol.Receipt
	}{report, status, openStatus, original, cold, resolved, br})
}
