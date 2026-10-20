//go:build windows && amd64

package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

type eventSink struct {
	mu    sync.Mutex
	items []Event
}

func (e *eventSink) add(v Event) { e.mu.Lock(); defer e.mu.Unlock(); e.items = append(e.items, v) }
func (e *eventSink) snapshot() []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Event(nil), e.items...)
}

type httpFixture struct {
	base    string
	store   *installation.Store
	auth    *installation.Authenticator
	handler *Handler
	server  *httptest.Server
	token   string
	events  *eventSink
}

func newHTTPFixture(t *testing.T) httpFixture {
	t.Helper()
	logs, err := filepath.Abs(filepath.Join("..", "..", "..", ".data", "logs", "001-validation"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(logs, 0700); err != nil {
		t.Fatal(err)
	}
	base, err := os.MkdirTemp(logs, "t022-http-")
	if err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(base, "allowed")
	if err = os.Mkdir(allowed, 0700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "installation")
	store, err := installation.Initialize(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error("installation resource did not drain", err)
		}
	})
	auth, err := installation.NewAuthenticator(store, []string{allowed})
	if err != nil {
		t.Fatal(err)
	}
	pipeName := "aboard-g5-t022-" + filepath.Base(base)
	pipe, err := security.NewControlPipe(pipeName)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- auth.ServeSession(ctx, pipe, security.Grant{PrincipalID: store.Identity().OwnerPrincipalID, Action: "installation.operator"}, 5*time.Minute)
	}()
	token, err := security.ReceiveSession(ctx, pipeName, uint32(os.Getpid()))
	if err != nil {
		t.Fatal("protected session delivery failed", err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	events := &eventSink{}
	handler, err := New(auth, store, root, server.Listener.Addr().String(), events.add)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	t.Cleanup(func() {
		server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := handler.opener.ShutdownWorkspaces(ctx); err != nil {
			t.Error("workspace drain failed", err)
		}
	})
	return httpFixture{base, store, auth, handler, server, token, events}
}
func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func (f httpFixture) request(t *testing.T, method, path string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Correlation-ID") == "" {
		t.Fatal("missing safe response metadata")
	}
	return response.StatusCode, raw
}
func requireStatus(t *testing.T, status, want int) {
	t.Helper()
	if status != want {
		t.Fatalf("HTTP status %d, want %d", status, want)
	}
}
func decodeResponse(t *testing.T, raw []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatal("response violates generated DTO", err)
	}
}
func (f httpFixture) create(t *testing.T, label, operation string) protocol.WorkspaceView {
	t.Helper()
	root := filepath.Join(f.base, "allowed", label)
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	status, raw := f.request(t, "POST", "/v1/workspace-creations", encode(t, protocol.CreateWorkspace{OperationId: protocol.UUID(operation), Path: protocol.Path(root), DisplayName: protocol.Title(label)}))
	requireStatus(t, status, 201)
	var v protocol.WorkspaceView
	decodeResponse(t, raw, &v)
	return v
}
func (f httpFixture) open(t *testing.T, ws protocol.UUID) protocol.WorkspaceView {
	t.Helper()
	a, err := f.auth.Authorize(context.Background(), f.token, string(ws), "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	status, raw := f.request(t, "POST", "/v1/workspaces/"+string(ws)+"/open", encode(t, protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(strconv.FormatInt(a.Location().RegistrationVersion, 10))}))
	requireStatus(t, status, 200)
	var v protocol.WorkspaceView
	decodeResponse(t, raw, &v)
	return v
}
func TestHTTPBusinessClosureUsesActualPipeSQLiteAndNetwork(t *testing.T) {
	f := newHTTPFixture(t)
	status, raw := f.request(t, "GET", "/v1/context", nil)
	requireStatus(t, status, 200)
	var principal protocol.PrincipalView
	decodeResponse(t, raw, &principal)
	if string(principal.PrincipalId) != f.store.Identity().OwnerPrincipalID {
		t.Fatal("wrong authenticated principal")
	}
	a := f.create(t, "A", "62000000-0000-4000-8000-000000000001")
	b := f.create(t, "B", "62000000-0000-4000-8000-000000000002")
	av, bv := f.open(t, a.WorkspaceId), f.open(t, b.WorkspaceId)
	if av.OpenEpoch == nil || bv.OpenEpoch == nil || *av.OpenEpoch == *bv.OpenEpoch {
		t.Fatal("independent epochs missing")
	}
	prefix := "/v1/workspaces/" + string(a.WorkspaceId)
	// A committed original creation key is inquiry-only, not evidence that A
	// remains closed after the later open. No second filesystem creation occurs.
	status, raw = f.request(t, "POST", "/v1/workspace-creations", encode(t, protocol.CreateWorkspace{OperationId: "62000000-0000-4000-8000-000000000001", Path: protocol.Path(filepath.Join(f.base, "allowed", "A")), DisplayName: "A"}))
	requireStatus(t, status, 201)
	var originalCreation protocol.WorkspaceView
	decodeResponse(t, raw, &originalCreation)
	if originalCreation.DirectoryLifecycle != "open" || originalCreation.OpenEpoch == nil || *originalCreation.OpenEpoch != *av.OpenEpoch {
		t.Fatal("duplicate creation fabricated closed state")
	}
	oldBody := strings.Repeat("\x01", 65536)
	status, raw = f.request(t, "POST", prefix+"/record-previews", encode(t, protocol.PreviewRecord{OpenEpoch: *av.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "synthetic", Body: protocol.Body(oldBody)}))
	requireStatus(t, status, 201)
	var preview protocol.CandidateView
	decodeResponse(t, raw, &preview)
	commit := protocol.CommitRecord{OperationId: "63000000-0000-4000-8000-000000000001", OpenEpoch: *av.OpenEpoch, CandidateId: preview.CandidateId, CandidateDigest: preview.CandidateDigest, PreviewVersion: preview.PreviewVersion}
	status, raw = f.request(t, "POST", prefix+"/record-commits", encode(t, commit))
	requireStatus(t, status, 200)
	var receipt protocol.Receipt
	decodeResponse(t, raw, &receipt)
	if receipt.NewRevision != "1" {
		t.Fatal("wrong committed revision")
	}
	status, raw = f.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil)
	requireStatus(t, status, 200)
	var record protocol.TextRecordView
	decodeResponse(t, raw, &record)
	if string(record.Body) != oldBody || record.OperationId != commit.OperationId {
		t.Fatal("receipt-backed current text missing")
	}
	// A legal update response exceeds the REQUEST-only 1 MiB boundary. Neither
	// before/after text nor strict response validation may be truncated or relaxed.
	newBody := strings.Repeat("\x02", 65536)
	digest := record.ContentDigest
	id := record.ResourceId
	status, raw = f.request(t, "POST", prefix+"/record-previews", encode(t, protocol.PreviewRecord{OpenEpoch: *av.OpenEpoch, Kind: "update-text", ResourceId: &id, BaseRevision: "1", BaseDigest: &digest, Title: "updated", Body: protocol.Body(newBody)}))
	requireStatus(t, status, 201)
	if len(raw) <= protocol.RequestMaxBytes {
		t.Fatal("large legal response fixture did not cross request boundary")
	}
	decodeResponse(t, raw, &preview)
	if preview.Diff.BeforeBody == nil || string(*preview.Diff.BeforeBody) != oldBody || string(preview.Diff.AfterBody) != newBody {
		t.Fatal("large response was altered")
	}
	commit.OperationId = "63000000-0000-4000-8000-000000000002"
	commit.CandidateId = preview.CandidateId
	commit.CandidateDigest = preview.CandidateDigest
	status, raw = f.request(t, "POST", prefix+"/record-commits", encode(t, commit))
	requireStatus(t, status, 200)
	decodeResponse(t, raw, &receipt)
	if receipt.NewRevision != "2" {
		t.Fatal("update not committed")
	}
	status, raw = f.request(t, "GET", "/v1/workspaces/"+string(b.WorkspaceId)+"/records", nil)
	requireStatus(t, status, 200)
	var list protocol.RecordList
	decodeResponse(t, raw, &list)
	if len(list.Items) != 0 {
		t.Fatal("A leaked into B")
	}
	status, raw = f.request(t, "POST", prefix+"/close", encode(t, protocol.CloseWorkspace{OperationId: commit.OperationId, OpenEpoch: *av.OpenEpoch}))
	requireStatus(t, status, 409)
	var keyConflict protocol.ErrorView
	decodeResponse(t, raw, &keyConflict)
	if keyConflict.Code != protocol.ErrorCodeOPERATIONKEYREUSE {
		t.Fatal("HTTP save/close collision was not scoped key reuse", keyConflict)
	}
	status, raw = f.request(t, "POST", prefix+"/close", encode(t, protocol.CloseWorkspace{OperationId: "64000000-0000-4000-8000-000000000001", OpenEpoch: *av.OpenEpoch}))
	if status != 200 && status != 202 {
		t.Fatalf("unexpected close status %d", status)
	}
	var closeOp protocol.OperationView
	decodeResponse(t, raw, &closeOp)
	auth, err := f.auth.Authorize(context.Background(), f.token, string(a.WorkspaceId), "workspace.close")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	closed, err := f.handler.opener.WaitClose(ctx, auth, "64000000-0000-4000-8000-000000000001")
	if err != nil || !closed.Closed {
		t.Fatal("physical close incomplete", err)
	}
	status, raw = f.request(t, "GET", prefix, nil)
	requireStatus(t, status, 200)
	var state protocol.WorkspaceView
	decodeResponse(t, raw, &state)
	if state.DirectoryLifecycle != "registeredClosed" || state.AccessMode != "none" {
		t.Fatal("status implicitly reopened closed workspace")
	}
	status, _ = f.request(t, "GET", prefix+"/records", nil)
	requireStatus(t, status, 409)
	reopened := f.open(t, a.WorkspaceId)
	if reopened.OpenEpoch == nil || *reopened.OpenEpoch == *av.OpenEpoch || reopened.Revision == nil || *reopened.Revision != "2" {
		t.Fatal("physical reopen lost facts or reused epoch")
	}
	status, raw = f.request(t, "POST", prefix+"/record-commits", encode(t, commit))
	requireStatus(t, status, 200)
	var duplicate protocol.Receipt
	decodeResponse(t, raw, &duplicate)
	if duplicate != receipt {
		t.Fatal("old key replayed rather than returning original receipt")
	}
	// A valid new-epoch candidate cannot reuse the process-retained close key.
	currentDigest := receipt.ContentDigest
	status, raw = f.request(t, "POST", prefix+"/record-previews", encode(t, protocol.PreviewRecord{OpenEpoch: *reopened.OpenEpoch, Kind: "update-text", ResourceId: &id, BaseRevision: "2", BaseDigest: &currentDigest, Title: "still candidate", Body: "never formally applied"}))
	requireStatus(t, status, 201)
	decodeResponse(t, raw, &preview)
	status, raw = f.request(t, "POST", prefix+"/record-commits", encode(t, protocol.CommitRecord{OperationId: "64000000-0000-4000-8000-000000000001", OpenEpoch: *reopened.OpenEpoch, CandidateId: preview.CandidateId, CandidateDigest: preview.CandidateDigest, PreviewVersion: preview.PreviewVersion}))
	requireStatus(t, status, 409)
	decodeResponse(t, raw, &keyConflict)
	if keyConflict.Code != protocol.ErrorCodeOPERATIONKEYREUSE {
		t.Fatal("HTTP close key acquired a save", keyConflict)
	}
	status, raw = f.request(t, "GET", prefix+"/operations/"+string(commit.OperationId), nil)
	requireStatus(t, status, 200)
	var op protocol.OperationView
	decodeResponse(t, raw, &op)
	if op.PersistenceState != "known" || op.LogicalState != "committed" || op.Receipt == nil || op.Receipt.OperationId != commit.OperationId {
		t.Fatal("original operation inquiry lost receipt")
	}
	status, raw = f.request(t, "GET", "/v1/installation-operations/62000000-0000-4000-8000-000000000001", nil)
	requireStatus(t, status, 200)
	decodeResponse(t, raw, &op)
	if op.PersistenceState != "known" || op.LogicalState != "committed" {
		t.Fatal("creation outcome incorrectly projected")
	}
}

func TestHTTPStrictBoundaryRedactsAndRejectsBeforeIO(t *testing.T) {
	f := newHTTPFixture(t)
	tests := []struct {
		name, method, path, body, contentType string
		mutate                                func(*http.Request)
		status                                int
	}{
		{"absolute-form", "GET", "/v1/context", "", "", func(r *http.Request) { r.URL.Scheme = "http"; r.URL.Host = "attacker.invalid" }, 403},
		{"host", "GET", "/v1/context", "", "", func(r *http.Request) { r.Host = "attacker.invalid" }, 403},
		{"origin", "GET", "/v1/context", "", "", func(r *http.Request) { r.Header.Set("Origin", "null") }, 403},
		{"cookie", "GET", "/v1/context", "", "", func(r *http.Request) { r.Header.Set("Cookie", "session=not-authority") }, 403},
		{"options", "OPTIONS", "/v1/context", "", "", nil, 403},
		{"duplicate-bearer", "GET", "/v1/context", "", "", func(r *http.Request) { r.Header.Add("Authorization", "Bearer "+f.token) }, 403},
		{"whitespace-get", "GET", "/v1/context", " ", "application/json", nil, 400},
		{"query", "GET", "/v1/context?unexpected=1", "", "", nil, 400},
		{"encoded-path", "GET", "/v1/%63ontext", "", "", nil, 400},
		{"content-type", "POST", "/v1/workspace-inspections", `{"path":"C:\\synthetic"}`, "text/plain", nil, 400},
		{"duplicate-json", "POST", "/v1/workspace-inspections", `{"path":"C:\\synthetic","path":"D:\\other"}`, "application/json", nil, 400},
		{"oversize", "POST", "/v1/workspace-inspections", strings.Repeat(" ", protocol.RequestMaxBytes+1), "application/json", nil, 413},
		{"future-route", "POST", "/v1/runs", `{}`, "application/json", nil, 501},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, f.server.URL+tc.path, strings.NewReader(tc.body))
			r.URL.Scheme = ""
			r.URL.Host = ""
			r.Header.Set("Authorization", "Bearer "+f.token)
			if tc.contentType != "" {
				r.Header.Set("Content-Type", tc.contentType)
			}
			if tc.mutate != nil {
				tc.mutate(r)
			}
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, r)
			requireStatus(t, w.Code, tc.status)
			var v protocol.ErrorView
			decodeResponse(t, w.Body.Bytes(), &v)
			if bytes.Contains(w.Body.Bytes(), []byte(f.token)) || bytes.Contains(w.Body.Bytes(), []byte(f.base)) {
				t.Fatal("HTTP error exposed secret or private path")
			}
		})
	}
	logs := encode(t, f.events.snapshot())
	if bytes.Contains(logs, []byte(f.token)) || bytes.Contains(logs, []byte(f.base)) {
		t.Fatal("structured events exposed bearer or private path")
	}
}
