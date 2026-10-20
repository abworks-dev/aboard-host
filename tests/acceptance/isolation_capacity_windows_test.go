//go:build windows && amd64

package acceptance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func isolationCapacityRefusal(t *testing.T, c *client, path, scope string, deadline time.Time, expectedKeys ...protocol.UUID) protocol.ErrorView {
	conn, reply := capacityHeaders(t, c, "GET", path, 1, deadline)
	raw, e := io.ReadAll(io.LimitReader(reply.Body, 4097))
	ce := reply.Body.Close()
	ne := conn.Close()
	var out protocol.ErrorView
	if e != nil || ce != nil || ne != nil || len(raw) > 4096 || reply.StatusCode != 429 || !reply.Close || json.Unmarshal(raw, &out) != nil || out.Code != protocol.ErrorCodeCAPACITYLIMIT || out.Scope == nil || string(*out.Scope) != scope || out.Outcome != "rejected" || out.RetryPolicy != "none" || out.CorrelationId == "" {
		t.Fatal("ordinary capacity rejection lost prebody scope/correlation")
	}
	if len(expectedKeys) > 1 || (len(expectedKeys) == 0 && out.OperationId != nil) || (len(expectedKeys) == 1 && (out.OperationId == nil || *out.OperationId != expectedKeys[0])) {
		t.Fatal("capacity refusal invented or replaced supplied path key")
	}
	isolationDeniedRaw(t, c, raw, protocol.ErrorCodeCAPACITYLIMIT)
	return out
}

// Real native journal sync checkpoint + real incomplete HTTP bodies in the
// independent tag Host. These are not 63 fabricated in-memory reservations.
func TestV013IndependentNativeIOAndInstallationCapacity(t *testing.T) {
	if workspace.WorkspaceOperationLimit != 32 || workspace.InstallationOperationLimit != 64 {
		t.Fatal("capacity profile changed")
	}
	f := newFixture(t)
	f.initialize(t)
	setup := f.start(t)
	initial := isolationConnect(t, setup)
	var actor protocol.PrincipalView
	initial.request(t, "GET", "/v1/context", nil, &actor, 200)
	root, a := newInspectionWorkspace(t, initial, "save-target")
	_, b := newInspectionWorkspace(t, initial, "independent-B")
	setup.stop(t)
	key := newID(t)
	staged := saveNativeFixture(t, f, key, a.WorkspaceId, actor.PrincipalId, "formal-inflight", 0)
	p := staged.start(t)
	c := isolationConnect(t, p)
	joinOriginalSessionDelivery(t, c)
	a = c.open(t, a.WorkspaceId)
	b = c.open(t, b.WorkspaceId)
	pa := "/v1/workspaces/" + string(a.WorkspaceId)
	pb := "/v1/workspaces/" + string(b.WorkspaceId)
	candidate := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "held native A", Body: "A native formal commit must retain its original capacity claim"})
	in := saveInput(key, a, candidate)
	wire := beginSaveWire(t, c, in, a.WorkspaceId)
	marker := waitSaveCheckpoint(t, p, in, a, actor, "formal-inflight")
	// Readiness binds actual original COMMIT/admission. A callback is blocked only
	// after two real successful journal syncs and before any database sync.
	started := time.Now()
	deadline := started.Add(12 * time.Second)
	var holders []net.Conn
	t.Cleanup(func() {
		for _, conn := range holders {
			_ = conn.Close()
		}
	})
	hold := func(path string) {
		conn, reply := capacityHeaders(t, c, "GET", path, 1, deadline)
		if reply.StatusCode != 100 || reply.Body.Close() != nil {
			t.Fatal("quota holder not admitted before incomplete body")
		}
		holders = append(holders, conn)
	}
	// One native save +30 real ordinary body handlers fill A's 31 ordinary slots;
	// the 32nd control slot is intentionally reserved for investigation/drain.
	for i := 0; i < 30; i++ {
		hold(pa + "/records")
	}
	before := isolationCapacityRefusal(t, c, pa+"/records", "workspace", deadline)
	created := isolationSave(t, c, b, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B while A native full", Body: "B create during A journal sync"})
	updated := isolationUpdate(t, c, b, "1", created, "B update while A native callback and30 handlers remain live")
	if saveFacts(t, c, b.WorkspaceId, "2", 1)[0] != updated.Row {
		t.Fatal("A long IO blocked independent B update/read")
	}
	after := isolationCapacityRefusal(t, c, pa+"/records", "workspace", deadline)
	for i := 0; i < 31; i++ {
		hold(pb + "/records")
	}
	hold("/v1/context") // 1 native +30 A +31 B +1 installation =63 ordinary.
	conn, reply := capacityHeaders(t, c, "GET", "/v1/context", 1, deadline)
	raw, err := io.ReadAll(io.LimitReader(reply.Body, 4097))
	closeErr := reply.Body.Close()
	connErr := conn.Close()
	var global protocol.ErrorView
	if err != nil || closeErr != nil || connErr != nil || len(raw) > 4096 || reply.StatusCode != 429 || !reply.Close || json.Unmarshal(raw, &global) != nil || global.Code != protocol.ErrorCodeCAPACITYLIMIT || global.OperationId != nil || global.Scope == nil || *global.Scope != "installation" || global.Outcome != "rejected" || global.RetryPolicy != "none" || global.CorrelationId == "" {
		t.Fatal("64th installation ordinary claim did not reject before body")
	}
	isolationSecrets(t, raw, f, c.token)
	// None of the holder bodies expired and A's native marker is STILL in flight.
	current := waitSaveCheckpoint(t, p, in, a, actor, "formal-inflight")
	if current.PID != marker.PID || current.Native == nil || !current.Native.InFlight || !time.Now().Before(deadline) {
		t.Fatal("capacity pressure expired or native IO finished early")
	}
	if lease, e := platform.Acquire(root, platform.Workspace, false, false); e == nil {
		_ = lease.Close()
		t.Fatal("full/long IO released original writer lock")
	}
	elapsed := time.Since(started)
	for _, h := range holders {
		if h.Close() != nil {
			t.Fatal("capacity holder socket close")
		}
	}
	// Close sockets is not a join. Release original checkpoint and read its actual
	// response, then current operations/close + graceful process exit join owners.
	releaseSaveCheckpoint(t, f)
	response, e := http.ReadResponse(bufio.NewReader(wire), &http.Request{Method: "POST"})
	if e != nil {
		t.Fatal("original native response unavailable")
	}
	receiptRaw, e := io.ReadAll(io.LimitReader(response.Body, 4097))
	ce := response.Body.Close()
	var receipt protocol.Receipt
	if e != nil || ce != nil || len(receiptRaw) > 4096 || response.StatusCode != 200 || json.Unmarshal(receiptRaw, &receipt) != nil || receipt.OperationId != key {
		t.Fatal("native save lost original receipt")
	}
	if wire.Close() != nil {
		t.Fatal("native response wire close")
	}
	isolationSecrets(t, receiptRaw, f, c.token)
	var original protocol.OperationView
	c.request(t, "GET", pa+"/operations/"+string(key), nil, &original, 200)
	if original.LogicalState != "committed" || original.PhysicalState != "drained" || original.PersistenceState != "known" || original.Receipt == nil || *original.Receipt != receipt {
		t.Fatal("native owner not physically drained/known")
	}
	saveMatches(t, saveFacts(t, c, a.WorkspaceId, "1", 1)[0], receipt, candidate)
	if saveFacts(t, c, b.WorkspaceId, "2", 1)[0] != updated.Row {
		t.Fatal("global pressure changed B")
	}
	closePublic(t, c, a)
	closePublic(t, c, b)
	p.stop(t)
	isolationDiagnostics(t, p, c.token, string(candidate.Body), string(created.Row.Body), string(updated.Row.Body))
	isolationDiagnostics(t, setup, initial.token)
	isolationEvidence(t, f, "isolation-native-capacity", struct {
		Marker                                                observedSaveCheckpoint
		AHolders, BHolders, InstallationHolders, NativeClaims int
		LocalBefore, LocalAfter, Global                       protocol.ErrorView
		BCreate, BUpdate, AReceipt                            protocol.Receipt
		Original                                              protocol.OperationView
		PressureNanoseconds                                   int64
		HostsEnded                                            bool
	}{marker, 30, 31, 1, 1, before, after, global, created.Receipt, updated.Receipt, receipt, original, elapsed.Nanoseconds(), true}, initial.token, c.token, string(candidate.Body), string(updated.Row.Body))
}

// A real failed native formal COMMIT has returned to the client, while the
// original unknown operation still owns ordinary capacity and the writer lease.
// Saturate ordinary capacity; ONLY its exact investigation may borrow its claim.
func TestV013IndependentUnknownInvestigationAtFullCapacity(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	setup := f.start(t)
	initial := isolationConnect(t, setup)
	var actor protocol.PrincipalView
	initial.request(t, "GET", "/v1/context", nil, &actor, 200)
	root, a := newInspectionWorkspace(t, initial, "save-target")
	_, b := newInspectionWorkspace(t, initial, "control-B")
	setup.stop(t)
	key := newID(t)
	staged := saveNativeFixture(t, f, key, a.WorkspaceId, actor.PrincipalId, "formal", 1)
	p := staged.start(t)
	c := isolationConnect(t, p)
	joinOriginalSessionDelivery(t, c)
	a = c.open(t, a.WorkspaceId)
	b = c.open(t, b.WorkspaceId)
	pa := "/v1/workspaces/" + string(a.WorkspaceId)
	pb := "/v1/workspaces/" + string(b.WorkspaceId)
	candidate := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "unknown capacity owner", Body: "original native unknown remains reserved until explicit physical investigation"})
	input := saveInput(key, a, candidate)
	var original protocol.ErrorView
	c.request(t, "POST", pa+"/record-commits", input, &original, 503)
	report, e := loadSaveNativeReport(f.root, "formal")
	if e != nil || report.PID != p.cmd.Process.Pid || report.WorkspaceID != string(a.WorkspaceId) || report.OperationID != string(key) || report.PrincipalID != string(actor.PrincipalId) || report.Epoch != string(*a.OpenEpoch) || report.Ordinal != 1 || report.Injected != 1 || report.NativeBOOL != 0 || report.SQLiteCode != 1034 || !report.Unknown || !report.NativeControlFailed || !report.CallbackDrained || original.Code != protocol.ErrorCodeCOMMITUNKNOWN || original.Scope == nil || *original.Scope != "workspace" || original.OperationId == nil || *original.OperationId != key || original.Outcome != "unknown" || original.RetryPolicy != "investigate" || original.CorrelationId == "" {
		t.Fatal("native original unknown not physically scoped")
	}
	// The original response body has been read and closed by request(), so the
	// requestClaim is free, NOT the unresolved native owner's retained allocation.
	deadline := time.Now().Add(12 * time.Second)
	var holders []net.Conn
	t.Cleanup(func() {
		for _, conn := range holders {
			_ = conn.Close()
		}
	})
	hold := func(path string) {
		conn, reply := capacityHeaders(t, c, "GET", path, 1, deadline)
		if reply.StatusCode != 100 || reply.Body.Close() != nil {
			t.Fatal("unknown full-capacity holder refused")
		}
		holders = append(holders, conn)
	}
	for i := 0; i < 30; i++ {
		hold(pa + "/records")
	}
	local := isolationCapacityRefusal(t, c, pa+"/records", "workspace", deadline)
	for i := 0; i < 31; i++ {
		hold(pb + "/records")
	}
	hold("/v1/context")
	global := isolationCapacityRefusal(t, c, "/v1/context", "installation", deadline)
	// An arbitrary key or the other workspace cannot borrow this allocation.
	wrongKey := newID(t)
	isolationCapacityRefusal(t, c, pa+"/operations/"+string(wrongKey), "workspace", deadline, wrongKey)
	isolationCapacityRefusal(t, c, pb+"/operations/"+string(key), "workspace", deadline, key)
	var queried protocol.OperationView
	for i := 0; i < 2; i++ {
		c.request(t, "GET", pa+"/operations/"+string(key), nil, &queried, 200)
		// A known original may be queried successfully while its durable
		// business outcome remains unknown. Success of this investigation is
		// not a successful save and must not free the retained allocation.
		if queried.OperationId != key || queried.WorkspaceId == nil || *queried.WorkspaceId != a.WorkspaceId || queried.Scope != "workspace" || queried.Kind != "save" || queried.LogicalState != "unknown" || queried.PersistenceState != "unknown" || queried.PhysicalState != "reconciling" || queried.Receipt != nil || queried.CorrelationId == "" {
			t.Fatal("full-capacity original query lost owner")
		}
		isolationCapacityRefusal(t, c, "/v1/context", "installation", deadline)
	}
	if lease, err := platform.Acquire(root, platform.Workspace, false, false); err == nil {
		_ = lease.Close()
		t.Fatal("unknown query released original exclusion")
	}
	var resolved protocol.OperationView
	c.request(t, "POST", pa+"/operations/"+string(key)+"/reconcile", nil, &resolved, 200)
	if resolved.OperationId != key || resolved.Scope != "workspace" || resolved.WorkspaceId == nil || *resolved.WorkspaceId != a.WorkspaceId || resolved.PhysicalState != "drained" || resolved.PersistenceState != "known" || resolved.LogicalState != "rejected" || resolved.Receipt != nil {
		t.Fatal("reserved investigation did not physically drain original failed COMMIT")
	}
	if again, e := loadSaveNativeReport(f.root, "formal"); e != nil || again != report {
		t.Fatal("investigation repeated native business admission")
	}
	if !time.Now().Before(deadline) {
		t.Fatal("control proof exceeded pressure deadline")
	}
	// Original resolved only after physical end; one ordinary slot now reappears
	// even though all62 incomplete request handlers still own their reservations.
	var live protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &live, 200)
	if !time.Now().Before(deadline) {
		t.Fatal("ordinary slot only returned after pressure expired")
	}
	for _, conn := range holders {
		if conn.Close() != nil {
			t.Fatal("unknown-pressure holder close")
		}
	}
	holders = nil
	closePublic(t, c, a)
	a = c.open(t, a.WorkspaceId)
	saveFacts(t, c, a.WorkspaceId, "0", 0)
	created := isolationSave(t, c, b, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B after unknown pressure", Body: "B progresses without replay of A unknown"})
	closePublic(t, c, a)
	closePublic(t, c, b)
	p.stop(t)
	isolationDiagnostics(t, setup, initial.token)
	isolationDiagnostics(t, p, c.token, string(candidate.Body), string(created.Row.Body))
	isolationEvidence(t, f, "isolation-unknown-control", struct {
		Native                  saveNativeReport
		Original, Local, Global protocol.ErrorView
		Query, Resolved         protocol.OperationView
		B                       protocol.Receipt
		OrdinaryHeld            int
		HostsEnded              bool
	}{report, original, local, global, queried, resolved, created.Receipt, 62, true}, initial.token, c.token, string(candidate.Body), string(created.Row.Body))
}

// Sharing authorization uncertainty is NOT a local A failure. Existing B was
// writable, then real native installation finalize FSYNC failure fences each
// actual authorization-dependent read/write; unauthenticated calls remain rejected (the documented 401/403 family).
func TestV013IndependentSharedAuthorizationUnknown(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	setup := f.start(t)
	initial := isolationConnect(t, setup)
	_, a := isolationCreate(t, initial, "shared-A")
	_, b := isolationCreate(t, initial, "shared-B")
	a = initial.open(t, a.WorkspaceId)
	b = initial.open(t, b.WorkspaceId)
	oldB := isolationSave(t, initial, b, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B before shared fault", Body: "B durable result not authorization authority"})
	bCandidate := savePreview(t, initial, b, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "1", Title: "B pending shared grant", Body: "must not commit using uncertain shared authorization"})
	closePublic(t, initial, a)
	closePublic(t, initial, b)
	setup.stop(t)
	key := newID(t)
	staged := finalizeFixture(t, f, key, 1)
	p := staged.start(t)
	c := isolationConnect(t, p)
	joinOriginalSessionDelivery(t, c)
	// This original native finalize fixture requires ZERO opened owners. Preserve
	// that guard: A/B are durable, previously writable but closed in this process;
	// the pending original B candidate/epoch is not renewed across the restart.
	// Shared authorization must fail before any closed/old-epoch business check.
	var healthy protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &healthy, 200)
	// The native fixture only arms the named new installation operation.
	var unknown protocol.ErrorView
	c.request(t, "POST", "/v1/workspace-creations", protocol.CreateWorkspace{OperationId: key, Path: protocol.Path(filepath.Join(f.config.AllowedRoots[0], "finalize-target")), DisplayName: "native shared grant unknown"}, &unknown, 503)
	report := readFinalizeEvidence(t, f)
	if unknown.Code != protocol.ErrorCodeINSTALLATIONSTATEUNKNOWN || unknown.Outcome != "unknown" || unknown.Scope == nil || *unknown.Scope != "installation" || unknown.OperationId == nil || *unknown.OperationId != key || unknown.SafeMessage == "" || unknown.RetryPolicy != "investigate" || unknown.CorrelationId == "" || report.PID != p.cmd.Process.Pid || report.Injected != 1 || report.NativeBOOL != 0 || report.SQLiteCode != 1034 || !report.Unknown || !report.CallbackDrained {
		t.Fatal("shared authorization fault was not actual scoped native uncertainty")
	}
	pb := "/v1/workspaces/" + string(b.WorkspaceId)
	var denials []protocol.ErrorView
	for _, probe := range []struct {
		method, path string
		input        any
	}{
		{"GET", "/v1/context", nil}, {"GET", "/v1/workspaces/" + string(a.WorkspaceId) + "/records", nil}, {"GET", pb + "/records", nil}, {"GET", pb + "/records/" + string(oldB.Receipt.ResourceId), nil},
		{"POST", pb + "/record-commits", saveInput(newID(t), b, bCandidate)},
		{"POST", pb + "/record-previews", protocol.PreviewRecord{OpenEpoch: *b.OpenEpoch, Kind: "create-text", BaseRevision: "1", Title: "shared blocked", Body: "no fallback authorization"}},
		{"GET", pb + "/operations/" + string(oldB.Input.OperationId), nil},
	} {
		var raw json.RawMessage
		c.request(t, probe.method, probe.path, probe.input, &raw, 503)
		isolationSecrets(t, raw, f, c.token)
		var refusal protocol.ErrorView
		if json.Unmarshal(raw, &refusal) != nil || refusal.Code != protocol.ErrorCodeINSTALLATIONSTATEUNKNOWN || refusal.Scope == nil || *refusal.Scope != "installation" || refusal.Outcome != "unknown" || refusal.RetryPolicy != "investigate" || refusal.CorrelationId == "" || refusal.SafeMessage == "" || bytes.Contains(raw, []byte(oldB.Row.Body)) || bytes.Contains(raw, []byte(bCandidate.Body)) {
			t.Fatal("B bypassed shared dependency or disclosed target")
		}
		isolationSecrets(t, raw, f, string(oldB.Row.Body), string(bCandidate.Body), `"receipt"`, `"resourceId"`, `"candidateId"`, `"contentDigest"`, `"newRevision"`)
		denials = append(denials, refusal)
	}
	invalid := &client{process: p, http: c.http, authority: c.authority, token: "synthetic-invalid-session"}
	isolationDenied(t, invalid, "GET", pb+"/records", nil, 403, protocol.ErrorCodeFORBIDDEN, string(oldB.Row.Body))
	p.terminateAt(t, "unresolved-installation-cleanup", "") // known unknown, never claim graceful drain.
	next := f.start(t)
	d := isolationConnect(t, next)
	// Explicitly finish the ORIGINAL uncertain installation operation. Fresh
	// authorization is not a claim that its accepted finalization rolled back.
	// Retaining this original unresolved owner is also why shutdown may not call
	// itself physically drained before this operator investigation completes.
	installationPath := "/v1/installation-operations/" + string(key)
	var cold, resolved protocol.OperationView
	d.request(t, "GET", installationPath, nil, &cold, 200)
	pre := operationPrecondition(t, d, key)
	d.request(t, "POST", installationPath+"/reconcile", protocol.ReconcileInstallation{ExpectedPhase: protocol.InstallationPhase(pre.ExpectedPhase)}, &resolved, 200)
	if cold.OperationId != key || cold.WorkspaceId == nil || resolved.OperationId != key || resolved.WorkspaceId == nil || *resolved.WorkspaceId != *cold.WorkspaceId || resolved.LogicalState != "committed" || resolved.PersistenceState != "known" {
		t.Fatal("shared unknown original installation history not reconciled")
	}
	bb := d.open(t, b.WorkspaceId)
	if saveFacts(t, d, b.WorkspaceId, "1", 1)[0] != oldB.Row {
		t.Fatal("shared fence changed B formal facts or admitted unauthorized effects")
	}
	// New trusted auth/exclusion, not an unknown business replay or cache fallback.
	fresh := isolationSave(t, d, bb, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "1", Title: "fresh B", Body: "B new operation only after fresh shared authorization"})
	closePublic(t, d, bb)
	next.stop(t)
	isolationDiagnostics(t, setup, initial.token, string(oldB.Row.Body), string(bCandidate.Body))
	isolationDiagnostics(t, p, c.token, string(oldB.Row.Body), string(bCandidate.Body))
	isolationDiagnostics(t, next, d.token, string(oldB.Row.Body), string(fresh.Row.Body))
	isolationEvidence(t, f, "isolation-shared-auth", struct {
		Native                                      nativeFinalizeEvidence
		Original                                    protocol.ErrorView
		DependentDenials                            []protocol.ErrorView
		Cold, Resolved                              protocol.OperationView
		OldB, FreshB                                protocol.Receipt
		UnknownHostTerminated, NormalFreshHostEnded bool
	}{report, unknown, denials, cold, resolved, oldB.Receipt, fresh.Receipt, true, true}, initial.token, c.token, d.token, string(oldB.Row.Body), string(fresh.Row.Body), string(bCandidate.Body))
}
