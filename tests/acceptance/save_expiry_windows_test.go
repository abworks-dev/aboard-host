//go:build windows && amd64

package acceptance

import (
	"os"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

const expiryWaitEnvironment = "ABOARD_ACCEPTANCE_EXPIRY_WAIT"

// Explicit long-running gate: actual production CandidateLifetime is 15 minutes.
// No clock seam, candidate SQL mutation, operating-system clock adjustment, or
// shortened product TTL is used. Run this gate separately with timeout >=20m;
// a normal short full-suite run explicitly skips only this wall-clock wait.
func TestV010IndependentRealCandidateExpiry(t *testing.T) {
	enabled := os.Getenv(expiryWaitEnvironment)
	if enabled == "" {
		t.Skip("actual 15-minute expiry gate requires " + expiryWaitEnvironment + "=1")
	}
	if enabled != "1" {
		t.Fatal("invalid explicit wall-clock gate setting")
	}
	f := newFixture(t)
	f.initialize(t)
	p := f.start(t)
	c := p.connect(t)
	var actor protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &actor, 200)
	_, a := newInspectionWorkspace(t, c, "real-expiry")
	a = c.open(t, a.WorkspaceId)
	prefix := "/v1/workspaces/" + string(a.WorkspaceId)
	original := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "confirmed before wait", Body: "original receipt must survive actual expiry"})
	originalInput := saveInput(newID(t), a, original)
	var receipt protocol.Receipt
	c.request(t, "POST", prefix+"/record-commits", originalInput, &receipt, 200)
	if receipt.OperationId != originalInput.OperationId {
		t.Fatal("original receipt is not bound to submitted key")
	}
	// This unconfirmed create has the CURRENT head and SAME live epoch, isolating
	// expiry from stale baseline/applied/epoch errors when the wait finishes.
	waiting := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "1", Title: "unconfirmed expires", Body: "actual wall-clock expired candidate must not apply"})
	expiry, err := time.Parse(time.RFC3339Nano, waiting.ExpiresAt)
	if err != nil {
		t.Fatal("candidate expiration is not a real timestamp")
	}
	originalExpiry, err := time.Parse(time.RFC3339Nano, original.ExpiresAt)
	if err != nil || originalExpiry.After(expiry) {
		t.Fatal("confirmed expiration order unexpected")
	}
	begin := time.Now().UTC()
	delay := time.Until(expiry.Add(150 * time.Millisecond))
	if delay < 14*time.Minute || delay > 16*time.Minute {
		t.Fatal("production expiry is not the expected bounded actual 15-minute wait")
	}
	saveMatches(t, saveFacts(t, c, a.WorkspaceId, "1", 1)[0], receipt, original)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	<-timer.C
	ended := time.Now().UTC()
	if ended.Before(expiry) || ended.Before(originalExpiry) {
		t.Fatal("actual wall clock did not pass both expirations")
	}
	// Existing sessions expire sooner than candidates. Renew through the actual
	// SAME Host's protected native pipe, not a spoofed actor or credential retry.
	d := p.connect(t)
	var renewed protocol.PrincipalView
	d.request(t, "GET", "/v1/context", nil, &renewed, 200)
	if renewed.InstallationId != actor.InstallationId || renewed.TenantId != actor.TenantId || renewed.PrincipalId != actor.PrincipalId {
		t.Fatal("real session renewal changed actor/scope")
	}
	var state protocol.WorkspaceView
	d.request(t, "GET", prefix, nil, &state, 200)
	if state.OpenEpoch == nil || *state.OpenEpoch != *a.OpenEpoch {
		t.Fatal("expiry wait changed the live open generation")
	}
	saveMatches(t, saveFacts(t, d, a.WorkspaceId, "1", 1)[0], receipt, original)
	var prior protocol.Receipt
	d.request(t, "POST", prefix+"/record-commits", originalInput, &prior, 200)
	if prior != receipt {
		t.Fatal("actually expired applied candidate suppressed original receipt")
	}
	var rejected protocol.ErrorView
	expiredInput := saveInput(newID(t), a, waiting)
	d.request(t, "POST", prefix+"/record-commits", expiredInput, &rejected, 409)
	if rejected.Code != protocol.ErrorCodeSTALEBASELINE {
		t.Fatal("expired new admission was not rejected by the existing candidate predicate")
	}
	var absent protocol.ErrorView
	d.request(t, "GET", prefix+"/records/"+string(waiting.ResourceId), nil, &absent, 404)
	if absent.Code != protocol.ErrorCodeNOTFOUND {
		t.Fatal("expired candidate acquired a formal resource")
	}
	saveMatches(t, saveFacts(t, d, a.WorkspaceId, "1", 1)[0], receipt, original)
	// Positive current-epoch/current-base control AFTER the expired refusal.
	fresh := savePreview(t, d, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "1", Title: "fresh after wait", Body: "fresh candidate confirms while expired one cannot"})
	freshInput := saveInput(newID(t), a, fresh)
	var freshReceipt protocol.Receipt
	d.request(t, "POST", prefix+"/record-commits", freshInput, &freshReceipt, 200)
	if freshReceipt.NewRevision != "2" || freshReceipt.OperationId != freshInput.OperationId {
		t.Fatal("fresh control did not advance exactly once under submitted key")
	}
	verifyRows := func(peer *client) {
		seen := map[protocol.UUID]bool{}
		for _, row := range saveFacts(t, peer, a.WorkspaceId, "2", 2) {
			if seen[row.ResourceId] {
				t.Fatal("duplicate post-expiry resource projection")
			}
			seen[row.ResourceId] = true
			switch row.ResourceId {
			case receipt.ResourceId:
				saveMatches(t, row, receipt, original)
			case freshReceipt.ResourceId:
				saveMatches(t, row, freshReceipt, fresh)
			default:
				t.Fatal("unexpected post-expiry formal resource")
			}
		}
		if !seen[receipt.ResourceId] || !seen[freshReceipt.ResourceId] {
			t.Fatal("post-expiry formal resource missing")
		}
	}
	verifyRows(d)
	closePublic(t, d, a)
	p.stop(t)
	next := f.start(t)
	e := next.connect(t)
	reopened := e.open(t, a.WorkspaceId)
	if reopened.OpenEpoch == nil || *reopened.OpenEpoch == *a.OpenEpoch {
		t.Fatal("new process did not publish a fresh epoch")
	}
	e.request(t, "POST", prefix+"/record-commits", originalInput, &prior, 200)
	if prior != receipt {
		t.Fatal("expired/applied/old-epoch original key changed after full restart")
	}
	e.request(t, "GET", prefix+"/records/"+string(waiting.ResourceId), nil, &absent, 404)
	verifyRows(e)
	next.stop(t)
	matrixEvidence(t, f, "save-real-expiry", struct {
		Workspace                                       protocol.UUID
		Begin, Ended, OriginalExpiry, UnconfirmedExpiry time.Time
		ElapsedSeconds                                  float64
		Original                                        protocol.Receipt
		ExpiredOperation                                protocol.UUID
		ExpiredCode                                     protocol.ErrorCode
		Fresh                                           protocol.Receipt
		OldEpoch, NewEpoch                              protocol.UUID
	}{a.WorkspaceId, begin, ended, originalExpiry, expiry, ended.Sub(begin).Seconds(), receipt, expiredInput.OperationId, rejected.Code, freshReceipt, *a.OpenEpoch, *reopened.OpenEpoch})
}
