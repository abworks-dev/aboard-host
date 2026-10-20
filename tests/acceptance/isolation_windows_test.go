//go:build windows && amd64

package acceptance

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// Inspect actual public wire bytes without ever persisting a bearer or raw wire.
func isolationSecrets(t *testing.T, raw []byte, f *fixture, tokens ...string) {
	t.Helper()
	for _, s := range append(tokens, f.root, f.config.InstallationRoot, f.configPath) {
		if s == "" {
			continue
		}
		encoded, err := json.Marshal(s)
		if err != nil {
			t.Fatal("secret scan fixture encoding")
		}
		if bytes.Contains(raw, []byte(s)) || bytes.Contains(raw, encoded[1:len(encoded)-1]) {
			t.Fatal("public response/diagnostic disclosed credential or private locator")
		}
	}
	for _, field := range []string{`"bearer":`, `"token":`, `"secret":`, `"rootLocator":`, `"bootstrap":`} {
		if bytes.Contains(raw, []byte(field)) {
			t.Fatal("public response disclosed private field")
		}
	}
}

// Keep raw public reply bytes in memory only, including successes used by the
// generic save/close helpers. No bearer, response dump or private path is logged.
type isolationHTTPAudit struct {
	t       *testing.T
	c       *client
	base    http.RoundTripper
	secrets []string
}

func (a isolationHTTPAudit) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := a.base.RoundTrip(req)
	if err != nil {
		return response, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	closeErr := response.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(raw) > 4<<20 {
		a.t.Fatal("isolation wire audit exceeded bound")
	}
	token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	isolationSecrets(a.t, raw, a.c.process.fixture, append(a.secrets, a.c.token, token)...)
	headers, err := json.Marshal(response.Header)
	if err != nil {
		return nil, err
	}
	isolationSecrets(a.t, headers, a.c.process.fixture, append(a.secrets, a.c.token, token)...)
	response.Body = io.NopCloser(bytes.NewReader(raw))
	return response, nil
}
func isolationWatch(t *testing.T, c *client, secrets ...string) *client {
	copyClient := *c.http
	base := copyClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copyClient.Transport = isolationHTTPAudit{t, c, base, secrets}
	c.http = &copyClient
	return c
}
func isolationConnect(t *testing.T, p *process) *client { return isolationWatch(t, p.connect(t)) }

func isolationEvidence(t *testing.T, f *fixture, name string, value any, tokens ...string) {
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal("isolation evidence encoding")
	}
	isolationSecrets(t, raw, f, tokens...)
	matrixEvidence(t, f, name, value)
}
func isolationRows(t *testing.T, got []protocol.TextRecordView, want ...protocol.TextRecordView) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatal("isolated row set length")
	}
	seen := map[protocol.UUID]bool{}
	for _, row := range got {
		if seen[row.ResourceId] {
			t.Fatal("duplicate isolated resource")
		}
		seen[row.ResourceId] = true
		found := false
		for _, expected := range want {
			if row == expected {
				found = true
			}
		}
		if !found {
			t.Fatal("unexpected isolated row")
		}
	}
	for _, row := range want {
		if !seen[row.ResourceId] {
			t.Fatal("missing isolated resource")
		}
	}
}
func isolationMissing(t *testing.T, c *client, method, path string, input any, targets ...string) {
	t.Helper()
	var body io.Reader
	if input != nil {
		raw, e := json.Marshal(input)
		if e != nil {
			t.Fatal("missing auth input")
		}
		body = bytes.NewReader(raw)
	}
	req, e := http.NewRequest(method, "http://"+c.authority+path, body)
	if e != nil {
		t.Fatal("missing auth request")
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	reply, e := c.http.Do(req)
	if e != nil {
		t.Fatal("missing auth exchange")
	}
	raw, re := io.ReadAll(io.LimitReader(reply.Body, 4097))
	ce := reply.Body.Close()
	if re != nil || ce != nil || len(raw) > 4096 || reply.StatusCode != 403 {
		t.Fatal("missing authentication not rejected")
	}
	isolationDeniedRaw(t, c, raw, protocol.ErrorCodeFORBIDDEN, targets...)
}
func isolationDeniedRaw(t *testing.T, c *client, raw []byte, code protocol.ErrorCode, targets ...string) protocol.ErrorView {
	t.Helper()
	isolationSecrets(t, raw, c.process.fixture, c.token)
	var out protocol.ErrorView
	if json.Unmarshal(raw, &out) != nil || out.Code != code || out.Outcome != "rejected" || out.RetryPolicy != "none" || out.CorrelationId == "" {
		t.Fatal("scope denial lost typed correlated rejection")
	}
	isolationSecrets(t, raw, c.process.fixture, append(targets, `"receipt"`, `"resourceId"`, `"candidateId"`, `"contentDigest"`, `"newRevision"`)...)
	return out
}
func isolationDenied(t *testing.T, c *client, method, path string, input any, status int, code protocol.ErrorCode, targets ...string) protocol.ErrorView {
	t.Helper()
	var raw json.RawMessage
	c.request(t, method, path, input, &raw, status)
	isolationDeniedRaw(t, c, raw, code, targets...)
	var out protocol.ErrorView
	if json.Unmarshal(raw, &out) != nil || out.Code != code || out.Outcome != "rejected" || out.RetryPolicy != "none" || out.CorrelationId == "" {
		t.Fatal("scope denial lost typed, correlated rejection", out.Code)
	}
	for _, s := range append(targets, `"receipt"`, `"resourceId"`, `"candidateId"`, `"contentDigest"`, `"newRevision"`) {
		if s != "" && bytes.Contains(raw, []byte(s)) {
			t.Fatal("denial disclosed target content or formal result")
		}
	}
	return out
}

func isolationCreate(t *testing.T, c *client, folder string) (string, protocol.WorkspaceView) {
	t.Helper()
	root := filepath.Join(c.process.fixture.config.AllowedRoots[0], folder)
	var raw json.RawMessage
	c.request(t, "POST", "/v1/workspace-creations", protocol.CreateWorkspace{OperationId: newID(t), Path: protocol.Path(root), DisplayName: "same workspace display name"}, &raw, 201)
	isolationSecrets(t, raw, c.process.fixture, c.token)
	var out protocol.WorkspaceView
	if json.Unmarshal(raw, &out) != nil || out.WorkspaceId == "" {
		t.Fatal("same-name workspace fixture failed")
	}
	return root, out
}

func isolationSave(t *testing.T, c *client, view protocol.WorkspaceView, key protocol.UUID, preview protocol.PreviewRecord) restartSaved {
	t.Helper()
	p := savePreview(t, c, view, preview)
	in := saveInput(key, view, p)
	var raw json.RawMessage
	prefix := "/v1/workspaces/" + string(view.WorkspaceId)
	c.request(t, "POST", prefix+"/record-commits", in, &raw, 200)
	isolationSecrets(t, raw, c.process.fixture, c.token)
	var receipt protocol.Receipt
	if json.Unmarshal(raw, &receipt) != nil || receipt.OperationId != key {
		t.Fatal("isolated save receipt key changed")
	}
	c.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil, &raw, 200)
	isolationSecrets(t, raw, c.process.fixture, c.token)
	var row protocol.TextRecordView
	if json.Unmarshal(raw, &row) != nil {
		t.Fatal("isolated formal row invalid")
	}
	saveMatches(t, row, receipt, p)
	return restartSaved{p, in, receipt, row}
}

func isolationUpdate(t *testing.T, c *client, view protocol.WorkspaceView, base protocol.Revision, old restartSaved, body protocol.Body) restartSaved {
	t.Helper()
	saved := isolationSave(t, c, view, newID(t), protocol.PreviewRecord{Kind: "update-text", ResourceId: &old.Receipt.ResourceId, BaseRevision: base, BaseDigest: &old.Receipt.ContentDigest, Title: old.Row.Title, Body: body})
	if saved.Receipt.ResourceId != old.Receipt.ResourceId || saved.Receipt.NewVersion != "2" || saved.Candidate.Diff.BeforeBody == nil || *saved.Candidate.Diff.BeforeBody != old.Row.Body {
		t.Fatal("B update lost actual baseline/version")
	}
	return saved
}

func isolationDiagnostics(t *testing.T, p *process, tokens ...string) {
	t.Helper()
	for _, capture := range []*boundedCapture{&p.out, &p.stderr} {
		raw, overflow := capture.snapshot()
		if overflow {
			t.Fatal("isolation diagnostics overflow")
		}
		isolationSecrets(t, raw, p.fixture, tokens...)
	}
}

func TestV013IndependentScopesAndRestrictedDenials(t *testing.T) {
	if os.Getenv(principalHelperEnvironment) == "" {
		t.Skip("strict installation test-helper executable not configured")
	}
	f := newFixture(t)
	f.initialize(t)
	first := f.start(t)
	c := isolationConnect(t, first)
	var actor protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &actor, 200)
	_, a := isolationCreate(t, c, "principal-target")
	_, b := isolationCreate(t, c, "isolation-B")
	a = c.open(t, a.WorkspaceId)
	b = c.open(t, b.WorkspaceId)
	if a.WorkspaceId == b.WorkspaceId {
		t.Fatal("same-name fixtures not independent")
	}
	key := newID(t)
	sa := isolationSave(t, c, a, key, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "same text display name", Body: "A private 正文 sentinel"})
	sb := isolationSave(t, c, b, key, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "same text display name", Body: "B private 正文 sentinel"})
	pa := "/v1/workspaces/" + string(a.WorkspaceId)
	pb := "/v1/workspaces/" + string(b.WorkspaceId)
	if sa.Receipt.OperationId != sb.Receipt.OperationId || sa.Receipt.ResourceId == sb.Receipt.ResourceId {
		t.Fatal("same temporary operation key conflated scopes")
	}
	isolationDenied(t, c, "GET", pa+"/records/"+string(sb.Receipt.ResourceId), nil, 404, protocol.ErrorCodeNOTFOUND, string(sb.Row.Body))
	isolationDenied(t, c, "GET", pb+"/records/"+string(sa.Receipt.ResourceId), nil, 404, protocol.ErrorCodeNOTFOUND, string(sa.Row.Body))
	foreign := saveInput(newID(t), a, sb.Candidate)
	isolationDenied(t, c, "POST", pa+"/record-commits", foreign, 409, protocol.ErrorCodeRECOVERYREQUIRED, string(sb.Row.Body))
	foreignUpdate := protocol.PreviewRecord{OpenEpoch: *a.OpenEpoch, Kind: "update-text", ResourceId: &sb.Receipt.ResourceId, BaseRevision: "1", BaseDigest: &sb.Receipt.ContentDigest, Title: "foreign", Body: "must not replace B"}
	isolationDenied(t, c, "POST", pa+"/record-previews", foreignUpdate, 404, protocol.ErrorCodeNOTFOUND, string(sb.Row.Body))
	// No request body may create identity authority. The protocol rejects these
	// extra fields, rather than treating the self-reported owner/tenant as a grant.
	for _, field := range []string{"workspaceId", "tenantId", "principalId"} {
		raw, _ := json.Marshal(sa.Input)
		var forged map[string]any
		if json.Unmarshal(raw, &forged) != nil {
			t.Fatal("forgery fixture")
		}
		forged[field] = actor.PrincipalId
		isolationDenied(t, c, "POST", pa+"/record-commits", forged, 400, protocol.ErrorCodeINVALIDINPUT, string(sa.Row.Body), string(sb.Row.Body))
	}
	// A unique operation in B must not be found through A's operation namespace.
	uniqueB := isolationSave(t, c, b, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "1", Title: "B-only operation", Body: "B operation result sentinel"})
	isolationDenied(t, c, "GET", pa+"/operations/"+string(uniqueB.Input.OperationId), nil, 409, protocol.ErrorCodeOPERATIONHISTORYUNAVAILABLE, string(uniqueB.Row.Body))
	principalReceipt(t, c, pa, sa.Input, sa.Receipt)
	principalReceipt(t, c, pb, sb.Input, sb.Receipt)
	closePublic(t, c, a)
	closePublic(t, c, b)
	first.stop(t)
	token, helper := principalFixtureSession(t, f, first, actor, a.WorkspaceId, "read-write")
	second := f.start(t)
	d := isolationConnect(t, second)
	aa := d.open(t, a.WorkspaceId)
	bb := d.open(t, b.WorkspaceId)
	isolationWatch(t, d, token, c.token)
	r := isolationWatch(t, restrictedClient(t, d, token), c.token, d.token)
	var identity protocol.PrincipalView
	r.request(t, "GET", "/v1/context", nil, &identity, 200)
	if identity.PrincipalId != restrictedFixturePrincipal || identity.PrincipalId == actor.PrincipalId {
		t.Fatal("restricted identity not distinct")
	}
	var positive protocol.TextRecordView
	r.request(t, "GET", pa+"/records/"+string(sa.Receipt.ResourceId), nil, &positive, 200)
	if positive != sa.Row {
		t.Fatal("restricted positive A read failed")
	}
	forbidden := []string{string(sb.Row.Body), string(sb.Row.Title), string(uniqueB.Row.Body), string(sb.Receipt.ContentDigest)}
	for _, probe := range []struct {
		method, path string
		input        any
	}{
		{"GET", pb, nil}, {"GET", pb + "/records", nil}, {"GET", pb + "/records/" + string(sb.Receipt.ResourceId), nil},
		{"GET", pb + "/operations/" + string(key), nil}, {"POST", pb + "/operations/" + string(uniqueB.Input.OperationId) + "/reconcile", nil},
		{"POST", pb + "/record-commits", sb.Input},
		{"POST", pb + "/record-previews", protocol.PreviewRecord{OpenEpoch: *bb.OpenEpoch, Kind: "create-text", BaseRevision: "2", Title: "forbidden", Body: "not admitted"}},
		{"POST", pb + "/close", protocol.CloseWorkspace{OperationId: newID(t), OpenEpoch: *bb.OpenEpoch}},
	} {
		isolationDenied(t, r, probe.method, probe.path, probe.input, 403, protocol.ErrorCodeFORBIDDEN, forbidden...)
	}
	// Missing Authorization is a real header omission, not Bearer-empty.
	for _, probe := range []struct {
		method, path string
		input        any
	}{
		{"GET", pb + "/records/" + string(sb.Receipt.ResourceId), nil},
		{"POST", pb + "/record-commits", sb.Input},
		{"POST", pb + "/operations/" + string(uniqueB.Input.OperationId) + "/reconcile", nil},
	} {
		isolationMissing(t, d, probe.method, probe.path, probe.input, forbidden...)
	}
	invalid := &client{process: second, http: d.http, authority: d.authority, token: "synthetic-invalid-session"}
	isolationDenied(t, invalid, "GET", pb+"/records/"+string(sb.Receipt.ResourceId), nil, 403, protocol.ErrorCodeFORBIDDEN, forbidden...)
	isolationDenied(t, invalid, "POST", pb+"/record-commits", sb.Input, 403, protocol.ErrorCodeFORBIDDEN, forbidden...)
	isolationDenied(t, invalid, "POST", pb+"/operations/"+string(uniqueB.Input.OperationId)+"/reconcile", nil, 403, protocol.ErrorCodeFORBIDDEN, forbidden...)
	// An authenticated A-only actor cannot smuggle B's resource/candidate back
	// through its permitted A route, nor query another actor's original receipt.
	isolationDenied(t, r, "GET", pa+"/records/"+string(sb.Receipt.ResourceId), nil, 404, protocol.ErrorCodeNOTFOUND, forbidden...)
	foreignUpdate.OpenEpoch = *aa.OpenEpoch
	isolationDenied(t, r, "POST", pa+"/record-previews", foreignUpdate, 404, protocol.ErrorCodeNOTFOUND, forbidden...)
	foreign.OpenEpoch = *aa.OpenEpoch
	foreign.OperationId = newID(t)
	isolationDenied(t, r, "POST", pa+"/record-commits", foreign, 409, protocol.ErrorCodeRECOVERYREQUIRED, forbidden...)
	isolationDenied(t, r, "GET", pa+"/operations/"+string(key), nil, 403, protocol.ErrorCodeFORBIDDEN, string(sa.Row.Body))
	if saveFacts(t, d, a.WorkspaceId, "1", 1)[0] != sa.Row {
		t.Fatal("denials changed A")
	}
	rows := saveFacts(t, d, b.WorkspaceId, "2", 2)
	if len(rows) != 2 {
		t.Fatal("denials changed B")
	}
	isolationRows(t, rows, sb.Row, uniqueB.Row)
	principalReceipt(t, d, pa, sa.Input, sa.Receipt)
	principalReceipt(t, d, pb, sb.Input, sb.Receipt)
	principalReceipt(t, d, pb, uniqueB.Input, uniqueB.Receipt)
	closePublic(t, d, aa)
	closePublic(t, d, bb)
	second.stop(t)
	isolationDiagnostics(t, first, c.token, token, string(sa.Row.Body), string(sb.Row.Body), string(uniqueB.Row.Body))
	isolationDiagnostics(t, second, d.token, token, string(sa.Row.Body), string(sb.Row.Body), string(uniqueB.Row.Body))
	isolationEvidence(t, f, "isolation-scopes", struct {
		A, B                                                                      protocol.UUID
		SharedKey                                                                 protocol.UUID
		AReceipt, BReceipt, UniqueB                                               protocol.Receipt
		Helper                                                                    principalHelperEvidence
		Restricted                                                                protocol.UUID
		SameDisplayName, NoForeignEffect, MissingAndInvalidAuthDenied, HostsEnded bool
	}{a.WorkspaceId, b.WorkspaceId, key, sa.Receipt, sb.Receipt, uniqueB.Receipt, helper, identity.PrincipalId, true, true, true, true}, c.token, d.token, token, string(sa.Row.Body), string(sb.Row.Body), string(uniqueB.Row.Body))
}

func TestV013IndependentExpiredSession(t *testing.T) {
	if os.Getenv(principalHelperEnvironment) == "" {
		t.Skip("strict installation test-helper executable not configured")
	}
	f := newFixture(t)
	f.initialize(t)
	first := f.start(t)
	c := isolationConnect(t, first)
	var actor protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &actor, 200)
	_, a := isolationCreate(t, c, "principal-target")
	_, b := isolationCreate(t, c, "expiry-B")
	a = c.open(t, a.WorkspaceId)
	b = c.open(t, b.WorkspaceId)
	sa := isolationSave(t, c, a, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "A expiry", Body: "A expiry target sentinel"})
	sb := isolationSave(t, c, b, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B expiry", Body: "B expiry target sentinel"})
	closePublic(t, c, a)
	closePublic(t, c, b)
	first.stop(t)
	token, helper := principalFixtureSession(t, f, first, actor, a.WorkspaceId, "read-write-short")
	second := f.start(t)
	d := isolationConnect(t, second)
	aa := d.open(t, a.WorkspaceId)
	bb := d.open(t, b.WorkspaceId)
	isolationWatch(t, d, token, c.token)
	r := isolationWatch(t, restrictedClient(t, d, token), c.token, d.token)
	var positive protocol.TextRecordView
	pa := "/v1/workspaces/" + string(a.WorkspaceId)
	pb := "/v1/workspaces/" + string(b.WorkspaceId)
	r.request(t, "GET", pa+"/records/"+string(sa.Receipt.ResourceId), nil, &positive, 200)
	positiveAt := time.Now().UTC()
	if positive != sa.Row {
		t.Fatal("short session was not positively authorized before expiry")
	}
	// Issuance completed before helper.Ended. Waiting to that time + full TTL
	// cannot reject merely because the helper/Host was stopped or SID was wrong.
	expiryBound := helper.Ended.Add(3*time.Second + 50*time.Millisecond)
	if !positiveAt.Before(expiryBound) {
		t.Fatal("short-session timing fixture invalid")
	}
	timer := time.NewTimer(time.Until(expiryBound))
	defer timer.Stop()
	<-timer.C
	checked := time.Now().UTC()
	for _, path := range []string{"/v1/context", pa + "/records/" + string(sa.Receipt.ResourceId), pb + "/records/" + string(sb.Receipt.ResourceId), pb + "/operations/" + string(sb.Input.OperationId)} {
		isolationDenied(t, r, "GET", path, nil, 403, protocol.ErrorCodeFORBIDDEN, string(sa.Row.Body), string(sb.Row.Body))
	}
	isolationDenied(t, r, "POST", pa+"/record-commits", sa.Input, 403, protocol.ErrorCodeFORBIDDEN, string(sa.Row.Body))
	isolationDenied(t, r, "POST", pb+"/record-commits", sb.Input, 403, protocol.ErrorCodeFORBIDDEN, string(sb.Row.Body))
	isolationDenied(t, r, "POST", pb+"/operations/"+string(sb.Input.OperationId)+"/reconcile", nil, 403, protocol.ErrorCodeFORBIDDEN, string(sb.Row.Body))
	if saveFacts(t, d, a.WorkspaceId, "1", 1)[0] != sa.Row || saveFacts(t, d, b.WorkspaceId, "1", 1)[0] != sb.Row {
		t.Fatal("expired session changed formal facts")
	}
	closePublic(t, d, aa)
	closePublic(t, d, bb)
	second.stop(t)
	isolationDiagnostics(t, first, c.token, token, string(sa.Row.Body), string(sb.Row.Body))
	isolationDiagnostics(t, second, d.token, token, string(sa.Row.Body), string(sb.Row.Body))
	isolationEvidence(t, f, "isolation-expiry", struct {
		Helper                                  principalHelperEvidence
		PositiveAt, ExpiryUpperBound, CheckedAt time.Time
		RealTTLSeconds                          int
		A, B                                    protocol.Receipt
		HostsEnded                              bool
	}{helper, positiveAt, expiryBound, checked, 3, sa.Receipt, sb.Receipt, true}, c.token, d.token, token, string(sa.Row.Body), string(sb.Row.Body))
}

func TestV013IndependentClosedAndCorruptAWithBProgress(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	first := f.start(t)
	c := isolationConnect(t, first)
	root, a := isolationCreate(t, c, "isolation-A")
	_, b := isolationCreate(t, c, "isolation-B")
	a = c.open(t, a.WorkspaceId)
	b = c.open(t, b.WorkspaceId)
	sa := isolationSave(t, c, a, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "A preserved", Body: "A baseline must survive normal close"})
	closePublic(t, c, a)
	// B must really CREATE, UPDATE that row and GET its exact bytes while A is closed.
	created := isolationSave(t, c, b, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B while A closed", Body: "closed-A B create"})
	updated := isolationUpdate(t, c, b, "1", created, "closed-A B updated 正文")
	if updated.Receipt.NewRevision != "2" || saveFacts(t, c, b.WorkspaceId, "2", 1)[0] != updated.Row {
		t.Fatal("closed A blocked or contaminated B")
	}
	reopened := c.open(t, a.WorkspaceId)
	if reopened.OpenEpoch == nil || *reopened.OpenEpoch == *a.OpenEpoch || saveFacts(t, c, a.WorkspaceId, "1", 1)[0] != sa.Row {
		t.Fatal("close deleted A or reused epoch")
	}
	closePublic(t, c, reopened)
	closePublic(t, c, b)
	first.stop(t)
	// Destructive fixture mutation is bounded to this known stopped synthetic DB.
	db := filepath.Join(root, "data", "workspace.sqlite")
	if requireSyntheticRoot(f.root) != nil || !within(f.root, db) || !unreparsed(db) {
		t.Fatal("corrupt fixture outside owned root")
	}
	if err := os.WriteFile(db, []byte("synthetic V013 invalid SQLite header; not a real user database"), 0600); err != nil {
		t.Fatal("stopped synthetic corruption failed")
	}
	before := tree(t, root)
	second := f.start(t)
	d := isolationConnect(t, second)
	bb := d.open(t, b.WorkspaceId)
	pa := "/v1/workspaces/" + string(a.WorkspaceId)
	pre := registrationPrecondition(t, d, a.WorkspaceId, "open")
	isolationDenied(t, d, "POST", pa+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, 409, protocol.ErrorCodeRECOVERYREQUIRED, string(sa.Row.Body))
	if !reflect.DeepEqual(before, tree(t, root)) {
		t.Fatal("failed open repaired corrupt A")
	}
	var rejected protocol.WorkspaceView
	d.request(t, "GET", pa, nil, &rejected, 200)
	if rejected.OpenEpoch != nil {
		t.Fatal("corrupt A published writable epoch")
	}
	isolationDenied(t, d, "POST", pa+"/record-commits", sa.Input, 409, protocol.ErrorCodeWORKSPACECLOSED, string(sa.Row.Body))
	if saveFacts(t, d, b.WorkspaceId, "2", 1)[0] != updated.Row {
		t.Fatal("bad A corrupted previous B history")
	}
	added := isolationSave(t, d, bb, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "2", Title: "B while A corrupt", Body: "bad-A B create"})
	changed := isolationUpdate(t, d, bb, "3", added, "bad-A B updated independently")
	if changed.Receipt.NewRevision != "4" {
		t.Fatal("B head not independent")
	}
	rows := saveFacts(t, d, b.WorkspaceId, "4", 2)
	isolationRows(t, rows, updated.Row, changed.Row)
	principalReceipt(t, d, "/v1/workspaces/"+string(b.WorkspaceId), updated.Input, updated.Receipt)
	closePublic(t, d, bb)
	second.stop(t)
	after := tree(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("shutdown or B progress modified rejected A")
	}
	isolationDiagnostics(t, first, c.token, string(sa.Row.Body), string(updated.Row.Body))
	isolationDiagnostics(t, second, d.token, string(sa.Row.Body), string(updated.Row.Body), string(changed.Row.Body))
	isolationEvidence(t, f, "isolation-local-fault", struct {
		A, B                                             protocol.UUID
		ClosedCreate, ClosedUpdate, BadCreate, BadUpdate protocol.Receipt
		Before, After                                    map[string]string
		BadA                                             protocol.WorkspaceView
		HostsEnded                                       bool
	}{a.WorkspaceId, b.WorkspaceId, created.Receipt, updated.Receipt, added.Receipt, changed.Receipt, before, after, rejected, true}, c.token, d.token, string(sa.Row.Body), string(updated.Row.Body), string(changed.Row.Body))
}
