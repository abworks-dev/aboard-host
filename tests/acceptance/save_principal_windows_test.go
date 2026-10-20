//go:build windows && amd64

package acceptance

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	win "golang.org/x/sys/windows"
)

const principalHelperEnvironment = "ABOARD_ACCEPTANCE_INSTALLATION_HELPER_EXE"
const restrictedFixturePrincipal protocol.UUID = "61111111-1111-4111-8111-111111111111"

type principalHelperEvidence struct {
	PID                 int
	Birth               uint64
	BinarySHA256, Phase string
	Started, Ended      time.Time
	ServerAndStoreEnded bool
}

// This client does no grant writes/SQL/Host Run. A normal-built _test helper
// uses original typed production APIs only while the Host is physically stopped.
// Its protected delivery pins its OWN PID; this does not claim Host-issued R.
func principalFixtureSession(t *testing.T, f *fixture, stopped *process, actor protocol.PrincipalView, wid protocol.UUID, phase string) (string, principalHelperEvidence) {
	t.Helper()
	var evidence principalHelperEvidence
	select {
	case <-stopped.done:
	default:
		t.Fatal("principal helper cannot overlap a live Host")
	}
	stopped.mu.Lock()
	graceful := stopped.evidence.Graceful
	stopped.mu.Unlock()
	if !graceful || stopped.fixture != f || requireSyntheticRoot(f.root) != nil {
		t.Fatal("principal fixture requires actual graceful original Host end")
	}
	executable := os.Getenv(principalHelperEnvironment)
	hash, err := checkedExecutable(executable)
	if err != nil {
		t.Fatal("installation test-helper build not configured")
	}
	input := struct {
		Version                                               int
		Fixture, InstallationID, TenantID, WorkspaceID, Phase string
	}{1, f.root, string(actor.InstallationId), string(actor.TenantId), string(wid), phase}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal("principal fixture encoding")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestAcceptancePrincipalFixtureHelper$")
	cmd.Dir = projectRoot()
	cmd.Env = append(os.Environ(), "ABOARD_ACCEPTANCE_PRINCIPAL="+base64.RawURLEncoding.EncodeToString(raw))
	permit, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("principal launch permit")
	}
	reader, writer := io.Pipe()
	var output, diagnostics boundedCapture
	cmd.Stdout = writer
	cmd.Stderr = &diagnostics
	evidence = principalHelperEvidence{BinarySHA256: hash, Phase: phase, Started: time.Now().UTC()}
	if cmd.Start() != nil {
		permit.Close()
		reader.Close()
		writer.Close()
		t.Fatal("principal helper launch failed")
	}
	// Keep an exact launch pin through native delivery AND actual Wait completion.
	handle, birth, image, pinErr := processIdentity(cmd.Process.Pid)
	if pinErr != nil || !strings.EqualFold(filepath.Clean(image), executable) {
		permit.Close()
		reader.Close()
		writer.Close()
		cmd.Process.Kill()
		cmd.Wait()
		if handle != 0 {
			win.CloseHandle(handle)
		}
		t.Fatal("principal helper launch identity unavailable")
	}
	defer func() {
		if win.CloseHandle(handle) != nil {
			t.Error("principal helper pin close failed")
		}
	}()
	evidence.PID = cmd.Process.Pid
	evidence.Birth = birth
	ready := make(chan struct{}, 1)
	scanned := make(chan error, 1)
	go func() {
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), 16384)
		for scanner.Scan() {
			line := scanner.Bytes()
			output.Write(append(bytes.Clone(line), '\n'))
			if string(line) == "ABOARD_PRINCIPAL_READY" {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		}
		scanned <- scanner.Err()
	}()
	done := make(chan error, 1)
	go func() {
		e := cmd.Wait()
		writer.Close()
		se := <-scanned
		if e == nil {
			e = se
		}
		done <- e
	}()
	ended := false
	defer func() {
		permit.Close()
		if !ended {
			cancel()
			cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("principal helper cleanup did not actually end")
			}
		}
	}()
	if n, e := permit.Write([]byte("P")); e != nil || n != 1 {
		t.Fatal("principal launch permit write failed")
	}
	if permit.Close() != nil {
		t.Fatal("principal launch permit close failed")
	}
	select {
	case <-ready:
	case <-done:
		ended = true
		t.Fatal("principal helper ended before delivery (output withheld)")
	case <-time.After(12 * time.Second):
		t.Fatal("principal helper readiness timeout")
	}
	receiveCtx, receiveCancel := context.WithTimeout(ctx, 8*time.Second)
	token, err := security.ReceiveSession(receiveCtx, "aboard-g5-"+filepath.Base(f.root)+"-principal", uint32(cmd.Process.Pid))
	receiveCancel()
	if err != nil {
		t.Fatal("restricted protected delivery failed (credential withheld)", safePipeFailure(err))
	}
	select {
	case e := <-done:
		ended = true
		if e != nil {
			t.Fatal("principal helper server/Store/exit failed (output withheld)")
		}
	case <-time.After(12 * time.Second):
		t.Fatal("restricted delivery not physically drained")
	}
	for _, b := range []*boundedCapture{&output, &diagnostics} {
		captured, overflow := b.snapshot()
		isolationSecrets(t, captured, f, token)
		if overflow || bytes.Contains(captured, []byte(token)) || bytes.Contains(captured, []byte(f.config.InstallationRoot)) {
			t.Fatal("principal helper diagnostics leaked/overflowed")
		}
	}
	evidence.Ended = time.Now().UTC()
	evidence.ServerAndStoreEnded = true
	return token, evidence
}
func restrictedClient(t *testing.T, owner *client, token string) *client {
	t.Helper()
	c := &client{process: owner.process, http: &http.Client{Timeout: 15 * time.Second}, authority: owner.authority, token: token}
	t.Cleanup(func() {
		c.http.CloseIdleConnections()
		for _, b := range []*boundedCapture{&c.process.out, &c.process.stderr} {
			raw, overflow := b.snapshot()
			if overflow || bytes.Contains(raw, []byte(c.token)) {
				t.Error("restricted bearer leaked in Host diagnostics")
			}
		}
		c.token = ""
	})
	return c
}
func principalDenied(t *testing.T, c *client, method, path string, input any, status int, code protocol.ErrorCode, secrets ...string) {
	t.Helper()
	var raw json.RawMessage
	c.request(t, method, path, input, &raw, status)
	var v protocol.ErrorView
	if json.Unmarshal(raw, &v) != nil || v.Code != code || v.Outcome != "rejected" || v.RetryPolicy != "none" {
		t.Fatal("principal denial lost safe typed rejection")
	}
	for _, key := range []string{`"receipt"`, `"resourceId"`, `"candidateId"`, `"contentDigest"`, `"newRevision"`} {
		if bytes.Contains(raw, []byte(key)) {
			t.Fatal("denial disclosed formal result fields")
		}
	}
	for _, s := range secrets {
		if s != "" && bytes.Contains(raw, []byte(s)) {
			t.Fatal("denial disclosed target content/result")
		}
	}
}
func principalReceipt(t *testing.T, c *client, prefix string, input protocol.CommitRecord, want protocol.Receipt) {
	t.Helper()
	var replay protocol.Receipt
	c.request(t, "POST", prefix+"/record-commits", input, &replay, 200)
	if replay != want {
		t.Fatal("principal check changed/replayed original receipt")
	}
	var op protocol.OperationView
	c.request(t, "GET", prefix+"/operations/"+string(want.OperationId), nil, &op, 200)
	if op.Scope != "workspace" || op.WorkspaceId == nil || *op.WorkspaceId != want.WorkspaceId || op.LogicalState != "committed" || op.PersistenceState != "known" || op.Receipt == nil || *op.Receipt != want {
		t.Fatal("principal check altered original operation")
	}
}
func TestV010IndependentPrincipalKeyAndCurrentAuthorization(t *testing.T) {
	if os.Getenv(principalHelperEnvironment) == "" {
		t.Skip("strict installation test-helper executable not configured")
	}
	f := newFixture(t)
	f.initialize(t)
	first := f.start(t)
	c := first.connect(t)
	var actor protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &actor, 200)
	_, a := newInspectionWorkspace(t, c, "principal-target")
	_, b := newInspectionWorkspace(t, c, "independent-B")
	a = c.open(t, a.WorkspaceId)
	b = c.open(t, b.WorkspaceId)
	prefix := "/v1/workspaces/" + string(a.WorkspaceId)
	candidate := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "owner result sentinel", Body: "OWNER-only-operation-result: formal bytes remain A"})
	ownerInput := saveInput(newID(t), a, candidate)
	var ownerReceipt protocol.Receipt
	c.request(t, "POST", prefix+"/record-commits", ownerInput, &ownerReceipt, 200)
	saveMatches(t, saveFacts(t, c, a.WorkspaceId, "1", 1)[0], ownerReceipt, candidate)
	bCandidate := savePreview(t, c, b, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B baseline", Body: "B preserved owner baseline"})
	bInput := saveInput(newID(t), b, bCandidate)
	var bReceipt protocol.Receipt
	c.request(t, "POST", "/v1/workspaces/"+string(b.WorkspaceId)+"/record-commits", bInput, &bReceipt, 200)
	closePublic(t, c, a)
	closePublic(t, c, b)
	first.stop(t)
	restricted, helper1 := principalFixtureSession(t, f, first, actor, a.WorkspaceId, "read-write")
	second := f.start(t)
	d := second.connect(t)
	aa := d.open(t, a.WorkspaceId)
	bb := d.open(t, b.WorkspaceId)
	if aa.OpenEpoch == nil || *aa.OpenEpoch == *a.OpenEpoch {
		t.Fatal("principal restart reused old epoch")
	}
	r := restrictedClient(t, d, restricted)
	var identity protocol.PrincipalView
	r.request(t, "GET", "/v1/context", nil, &identity, 200)
	if identity.InstallationId != actor.InstallationId || identity.TenantId != actor.TenantId || identity.PrincipalId != restrictedFixturePrincipal || identity.PrincipalId == actor.PrincipalId {
		t.Fatal("fixture bearer was not a distinct bound DB principal")
	}
	// Positive read/write in A makes the following failure genuinely actor/key
	// association, not simply a nonexistent principal or missing workspace grant.
	var row protocol.TextRecordView
	r.request(t, "GET", prefix+"/records/"+string(ownerReceipt.ResourceId), nil, &row, 200)
	saveMatches(t, row, ownerReceipt, candidate)
	secrets := []string{string(candidate.Body), string(candidate.Title), string(ownerReceipt.ResourceId), string(ownerReceipt.ContentDigest), string(candidate.CandidateId)}
	principalDenied(t, r, "GET", prefix+"/operations/"+string(ownerInput.OperationId), nil, 403, protocol.ErrorCodeFORBIDDEN, secrets...)
	// Identical saved body, original old epoch/applied candidate/key: ONLY bearer
	// changes. Existing original actor association must outrank stale preview.
	principalDenied(t, r, "POST", prefix+"/record-commits", ownerInput, 409, protocol.ErrorCodeOPERATIONKEYREUSE, secrets...)
	principalReceipt(t, d, prefix, ownerInput, ownerReceipt)
	ownCandidate := savePreview(t, r, aa, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "1", Title: "restricted own result sentinel", Body: "R own formal bytes, later rights must be rechecked"})
	ownInput := saveInput(newID(t), aa, ownCandidate)
	var ownReceipt protocol.Receipt
	r.request(t, "POST", prefix+"/record-commits", ownInput, &ownReceipt, 200)
	principalReceipt(t, r, prefix, ownInput, ownReceipt)
	rows := saveFacts(t, d, a.WorkspaceId, "2", 2)
	found := false
	for _, v := range rows {
		if v.ResourceId == ownReceipt.ResourceId {
			saveMatches(t, v, ownReceipt, ownCandidate)
			found = true
		}
	}
	if !found {
		t.Fatal("restricted legitimate write absent")
	}
	principalDenied(t, r, "GET", "/v1/workspaces/"+string(b.WorkspaceId)+"/records", nil, 403, protocol.ErrorCodeFORBIDDEN, string(bCandidate.Body))
	nextB := savePreview(t, d, bb, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "1", Title: "B progress", Body: "B saves during principal A rejection"})
	var nextBReceipt protocol.Receipt
	d.request(t, "POST", "/v1/workspaces/"+string(b.WorkspaceId)+"/record-commits", saveInput(newID(t), bb, nextB), &nextBReceipt, 200)
	saveFacts(t, d, b.WorkspaceId, "2", 2)
	closePublic(t, d, aa)
	closePublic(t, d, bb)
	second.stop(t)
	fresh, helper2 := principalFixtureSession(t, f, second, actor, a.WorkspaceId, "open-only")
	third := f.start(t)
	e := third.connect(t)
	aaa := e.open(t, a.WorkspaceId)
	bbb := e.open(t, b.WorkspaceId)
	if aaa.OpenEpoch == nil || *aaa.OpenEpoch == *aa.OpenEpoch {
		t.Fatal("permission restart reused epoch")
	}
	current := restrictedClient(t, e, fresh)
	obsolete := restrictedClient(t, e, restricted)
	var after protocol.PrincipalView
	current.request(t, "GET", "/v1/context", nil, &after, 200)
	if after.InstallationId != actor.InstallationId || after.TenantId != actor.TenantId || after.PrincipalId != restrictedFixturePrincipal {
		t.Fatal("current restricted principal not authenticated")
	}
	// Session issuance follows helper1.Started. This conservative bound excludes
	// ordinary expiry as the reason the prior authorization version is rejected.
	oldSessionWindowEnd := helper1.Started.Add(5 * time.Minute)
	oldSessionCheck := time.Now().UTC()
	if !oldSessionCheck.Before(oldSessionWindowEnd) {
		t.Fatal("old session expiry could obscure authorization replacement")
	}
	principalDenied(t, obsolete, "GET", "/v1/context", nil, 403, protocol.ErrorCodeFORBIDDEN)
	if !time.Now().Before(oldSessionWindowEnd) {
		t.Fatal("old session expired during authorization replacement check")
	}
	ownSecrets := []string{string(ownCandidate.Body), string(ownCandidate.Title), string(ownReceipt.ResourceId), string(ownReceipt.ContentDigest), string(ownCandidate.CandidateId)}
	principalDenied(t, current, "GET", prefix+"/operations/"+string(ownInput.OperationId), nil, 403, protocol.ErrorCodeFORBIDDEN, ownSecrets...)
	principalDenied(t, current, "POST", prefix+"/record-commits", ownInput, 403, protocol.ErrorCodeFORBIDDEN, ownSecrets...)
	principalDenied(t, current, "GET", prefix+"/records", nil, 403, protocol.ErrorCodeFORBIDDEN, ownSecrets...)
	principalDenied(t, current, "GET", prefix+"/operations/"+string(ownerInput.OperationId), nil, 403, protocol.ErrorCodeFORBIDDEN, secrets...)
	principalReceipt(t, e, prefix, ownerInput, ownerReceipt)
	// Operator may investigate R's receipt, but cannot become its original actor
	// by resubmitting R's operation key with the owner's bearer.
	principalDenied(t, e, "POST", prefix+"/record-commits", ownInput, 409, protocol.ErrorCodeOPERATIONKEYREUSE, ownSecrets...)
	var restrictedOperation protocol.OperationView
	e.request(t, "GET", prefix+"/operations/"+string(ownInput.OperationId), nil, &restrictedOperation, 200)
	if restrictedOperation.Scope != "workspace" || restrictedOperation.WorkspaceId == nil || *restrictedOperation.WorkspaceId != a.WorkspaceId || restrictedOperation.LogicalState != "committed" || restrictedOperation.PersistenceState != "known" || restrictedOperation.Receipt == nil || *restrictedOperation.Receipt != ownReceipt {
		t.Fatal("operator inquiry changed original restricted receipt")
	}
	finalRows := saveFacts(t, e, a.WorkspaceId, "2", 2)
	if len(finalRows) != len(rows) {
		t.Fatal("permission change produced a formal effect")
	}
	for i := range rows {
		if rows[i] != finalRows[i] {
			t.Fatal("permission change altered formal record")
		}
	}
	bp := "/v1/workspaces/" + string(b.WorkspaceId)
	principalReceipt(t, e, bp, bInput, bReceipt)
	saveFacts(t, e, b.WorkspaceId, "2", 2)
	closePublic(t, e, aaa)
	closePublic(t, e, bbb)
	third.stop(t)
	original, _ := json.Marshal(ownerInput)
	sum := sha256.Sum256(original)
	evidence := struct {
		Helpers                                                                              []principalHelperEvidence
		Workspace, RestrictedPrincipal                                                       protocol.UUID
		OriginalBodySHA256                                                                   string
		OriginalReceipt, RestrictedReceipt, BReceipt                                         protocol.Receipt
		CrossPrincipalQuery, CrossPrincipalPost, OldSession, CurrentOwnQuery, CurrentOwnPost int
		Boundary                                                                             string
		OldSessionCheckedAt, OldSessionConservativeExpiry                                    time.Time
	}{[]principalHelperEvidence{helper1, helper2}, a.WorkspaceId, restrictedFixturePrincipal, hex.EncodeToString(sum[:]), ownerReceipt, ownReceipt, bReceipt, 403, 409, 403, 403, 403, "helper-issued durable protected sessions; normal Host HTTP; current-rights replacement while stopped, NOT live revocation or Host-PID restricted issuance", oldSessionCheck, oldSessionWindowEnd}
	encoded, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal("principal evidence encoding")
	}
	if err = os.WriteFile(filepath.Join(f.root, "save-principal.json"), encoded, 0600); err != nil {
		t.Fatal("principal evidence write")
	}
	t.Log(fmt.Sprintf("normal Host principal/key and current-rights checks passed; helper PIDs %d/%d physically ended", helper1.PID, helper2.PID))
}
