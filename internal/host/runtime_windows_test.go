//go:build windows && amd64

package host

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

func hostConfig(t *testing.T) Config {
	t.Helper()
	logs, err := filepath.Abs(filepath.Join("..", "..", ".data", "logs", "001-validation"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(logs, 0700); err != nil {
		t.Fatal(err)
	}
	base, err := os.MkdirTemp(logs, "t022-host-")
	if err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(base, "allowed")
	if err = os.Mkdir(allowed, 0700); err != nil {
		t.Fatal(err)
	}
	return Config{InstallationRoot: filepath.Join(base, "installation"), AllowedRoots: []string{allowed}, Listen: "127.0.0.1:0", PipeName: "aboard-g5-" + filepath.Base(base)}
}
func TestHostConfigExactNonsecretAndNoInitializeFallback(t *testing.T) {
	c := hostConfig(t)
	path := filepath.Join(filepath.Dir(c.InstallationRoot), "config.json")
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(path); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"installationRoot":`, `"InstallationRoot":`, 1),
		strings.Replace(string(raw), `"listen":"127.0.0.1:0"`, `"listen":"0.0.0.0:0"`, 1),
		strings.Replace(string(raw), `"allowedRoots":[`, `"allowedRoots":null,"ignored":[`, 1),
		`{"installationRoot":"x","allowedRoots":[],"listen":"127.0.0.1:0","pipeName":"aboard-g5-x","token":"not-permitted"}`,
		string(raw[:len(raw)-1]) + `,"listen":"127.0.0.1:1"}`,
	} {
		if err = os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = LoadConfig(path); err == nil {
			t.Fatal("ambiguous/unsafe configuration accepted")
		}
	}
	if err = Run(context.Background(), c, nil); err == nil {
		t.Fatal("serve initialized missing installation")
	}
	if _, err = os.Stat(c.InstallationRoot); !os.IsNotExist(err) {
		t.Fatal("failed serve created installation artifacts", err)
	}
}

func TestHostRunActualHTTPNativeControlAndPhysicalShutdown(t *testing.T) {
	c := hostConfig(t)
	if err := Initialize(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan Event, 1)
	done := make(chan error, 1)
	var mu sync.Mutex
	var events []Event
	log := func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
		if e.Event == "ready" {
			ready <- e
		}
	}
	go func() { done <- Run(ctx, c, log) }()
	finished := false
	t.Cleanup(func() {
		cancel()
		if !finished {
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(10 * time.Second):
				t.Error("Host did not actually drain")
			}
		}
	})
	var endpoint Event
	select {
	case endpoint = <-ready:
	case err := <-done:
		finished = true
		t.Fatal("Host startup failed", err)
	case <-time.After(10 * time.Second):
		t.Fatal("Host startup timed out")
	}
	pctx, end := context.WithTimeout(context.Background(), 5*time.Second)
	defer end()
	token, err := security.ReceiveSession(pctx, c.PipeName, uint32(os.Getpid()))
	if err != nil {
		t.Fatal("real Host operator delivery", err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	request := func(method, path string, in, out any, want int) {
		t.Helper()
		var raw []byte
		var err error
		if in != nil {
			raw, err = json.Marshal(in)
			if err != nil {
				t.Fatal(err)
			}
		}
		r, err := http.NewRequest(method, "http://"+endpoint.Authority+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		if in != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("%s status %d, want %d", method, response.StatusCode, want)
		}
		if out != nil {
			if err = json.Unmarshal(body, out); err != nil {
				t.Fatal("invalid generated response", err)
			}
		}
	}
	var roots []string
	for i, name := range []string{"A", "B"} {
		root := filepath.Join(c.AllowedRoots[0], name)
		if err = os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		roots = append(roots, root)
		operation := []protocol.UUID{"82000000-0000-4000-8000-000000000001", "82000000-0000-4000-8000-000000000002"}[i]
		var view protocol.WorkspaceView
		request("POST", "/v1/workspace-creations", protocol.CreateWorkspace{OperationId: operation, Path: protocol.Path(root), DisplayName: protocol.Title(name)}, &view, 201)
		precondition, err := security.ReceivePrecondition(pctx, c.PipeName, uint32(os.Getpid()), security.PreconditionRequest{Version: 1, Kind: "registration", Bearer: token, WorkspaceID: string(view.WorkspaceId), IntendedUse: "open"})
		if err != nil {
			t.Fatal("real operator precondition", err)
		}
		base := "/v1/workspaces/" + string(view.WorkspaceId)
		request("POST", base+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(precondition.RegistrationVersion)}, &view, 200)
		var preview protocol.CandidateView
		body := protocol.Body("independent Host bytes " + name)
		request("POST", base+"/record-previews", protocol.PreviewRecord{OpenEpoch: *view.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "same title", Body: body}, &preview, 201)
		var receipt protocol.Receipt
		request("POST", base+"/record-commits", protocol.CommitRecord{OperationId: operation, OpenEpoch: *view.OpenEpoch, CandidateId: preview.CandidateId, CandidateDigest: preview.CandidateDigest, PreviewVersion: preview.PreviewVersion}, &receipt, 200)
		var record protocol.TextRecordView
		request("GET", base+"/records/"+string(receipt.ResourceId), nil, &record, 200)
		if record.Body != body || record.WorkspaceId != view.WorkspaceId || record.ContentDigest != receipt.ContentDigest {
			t.Fatal("Host formal bytes/scope mismatch")
		}
	}
	// The 100 Continue is emitted only when the handler has passed admission and
	// starts reading its body. Hold that entered mutation before a complete JSON
	// command, then require B's real writer lease to drain before this body ends.
	slow, err := net.DialTimeout("tcp", endpoint.Authority, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	if err = slow.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	lateRoot := filepath.Join(c.AllowedRoots[0], "late-body")
	if err = os.Mkdir(lateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	lateInput, err := json.Marshal(protocol.CreateWorkspace{OperationId: "82000000-0000-4000-8000-000000000099", Path: protocol.Path(lateRoot), DisplayName: "must not start after cutoff"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(slow, "POST /v1/workspace-creations HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nExpect: 100-continue\r\n\r\n", endpoint.Authority, token, len(lateInput))
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(slow)
	interim, err := http.ReadResponse(reader, &http.Request{Method: "POST"})
	if err != nil || interim.StatusCode != 100 {
		t.Fatal("handler did not enter actual body read", err)
	}
	if err = interim.Body.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	drained := false
	for !drained {
		candidate, e := platform.Acquire(roots[1], platform.Workspace, false, false)
		if e == nil {
			if e = candidate.Close(); e != nil {
				t.Fatal(e)
			}
			drained = true
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("slow entered HTTP body delayed independent workspace shutdown cutoff")
		}
	}
	select {
	case err := <-done:
		finished = true
		t.Fatal("Host exited while body handler was still live", err)
	default:
	}
	if candidate, e := platform.Acquire(c.InstallationRoot, platform.Installation, false, false); e == nil {
		candidate.Close()
		t.Fatal("installation released before entered handler joined")
	}
	// Finishing a previously unread body is NOT new business admission after
	// cutoff. Reject before constructing a creation intent or touching its root.
	if _, err = slow.Write(lateInput); err != nil {
		t.Fatal(err)
	}
	lateReply, err := http.ReadResponse(reader, &http.Request{Method: "POST"})
	if err != nil {
		t.Fatal(err)
	}
	lateRaw, err := io.ReadAll(lateReply.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err = lateReply.Body.Close(); err != nil {
		t.Fatal(err)
	}
	var lateFailure protocol.ErrorView
	if lateReply.StatusCode != 409 || json.Unmarshal(lateRaw, &lateFailure) != nil || lateFailure.Code != protocol.ErrorCodeWORKSPACEDRAINING {
		t.Fatalf("late body began new command after cutoff: status=%d", lateReply.StatusCode)
	}
	if err = slow.Close(); err != nil {
		t.Fatal(err)
	}
	if entries, e := os.ReadDir(lateRoot); e != nil || len(entries) != 0 {
		t.Fatal("rejected late body mutated synthetic directory", e)
	}
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("normal Host shutdown did not join physical owners")
	}
	for _, root := range roots {
		lease, err := platform.Acquire(root, platform.Workspace, false, false)
		if err != nil {
			t.Fatal("shutdown retained writer without reporting unknown", err)
		}
		if err = lease.Close(); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := platform.Acquire(c.InstallationRoot, platform.Installation, false, false)
	if err != nil {
		t.Fatal("shutdown did not release installation after native completion", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	encoded, err := json.Marshal(events)
	draining, operatorEvidence := false, false
	for _, e := range events {
		if e.Event == "draining" {
			draining = true
		}
		if e.Event == "operator" {
			if !draining || e.Pipe == nil || e.PipeAttempt == 0 {
				t.Error("operator diagnostics escaped drainage boundary")
			}
			operatorEvidence = true
		}
	}
	mu.Unlock()
	if !operatorEvidence {
		t.Fatal("missing joined operator diagnostics")
	}
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(token)) || bytes.Contains(encoded, []byte(c.InstallationRoot)) || !bytes.Contains(encoded, []byte(`"event":"stopped"`)) {
		t.Fatal("unsafe events or missing actual stopped evidence")
	}
	// This composes real owners, standard HTTP and native control within a test
	// process. Complete independent executable restart evidence remains T023/T027.
}
