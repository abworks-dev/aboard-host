//go:build windows && amd64

package acceptance

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// Headers only: neither a holder's missing byte nor a victim's valid JSON is
// transmitted. Never put the bearer or raw authenticated wire in diagnostics.
func capacityHeaders(t *testing.T, c *client, method, path string, length int, deadline time.Time) (net.Conn, *http.Response) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", c.authority, time.Second)
	if err != nil {
		t.Fatal("capacity socket unavailable")
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err = conn.SetDeadline(deadline); err != nil {
		t.Fatal("capacity socket deadline unavailable")
	}
	if _, err = fmt.Fprintf(conn, "%s %s HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nExpect: 100-continue\r\n\r\n", method, path, c.authority, c.token, length); err != nil {
		t.Fatal("capacity headers unavailable (wire withheld)")
	}
	reply, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: method})
	if err != nil {
		t.Fatal("capacity header response unavailable (wire withheld)")
	}
	return conn, reply
}

func capacityRefusal(t *testing.T, c *client, path string, length int, deadline time.Time) protocol.ErrorView {
	t.Helper()
	conn, reply := capacityHeaders(t, c, "POST", path, length, deadline)
	defer conn.Close()
	defer reply.Body.Close()
	// An interim100 would prove the victim's body was solicited. Fail without
	// supplying it: a complete-JSON request cannot prove prebody rejection.
	if reply.StatusCode != 429 || !reply.Close || reply.Header.Get("Content-Type") != "application/json" {
		t.Fatal("victim did not receive final capacity refusal before body", reply.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(reply.Body, protocol.RequestMaxBytes+1))
	var out protocol.ErrorView
	if err != nil || len(raw) > protocol.RequestMaxBytes || json.Unmarshal(raw, &out) != nil {
		t.Fatal("capacity refusal invalid")
	}
	// The key is only in the withheld JSON, not in this route. It must not be
	// fabricated or described as a durable rejected admission.
	if out.Code != protocol.ErrorCodeCAPACITYLIMIT || out.Outcome != "rejected" || out.RetryPolicy != "none" || out.OperationId != nil || out.Scope == nil || *out.Scope != "workspace" {
		t.Fatal("capacity refusal invented admission/result or lost scope")
	}
	return out
}

func capacityReadiness(t *testing.T, c *client, path string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for attempts := 1; ; attempts++ {
		req, err := http.NewRequestWithContext(ctx, "GET", "http://"+c.authority+path, nil)
		if err != nil {
			t.Fatal("capacity readiness request invalid")
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		reply, err := c.http.Do(req)
		if err != nil {
			t.Fatal("capacity handler readiness not observed (request withheld)")
		}
		raw, readErr := io.ReadAll(io.LimitReader(reply.Body, protocol.RequestMaxBytes+1))
		closeErr := reply.Body.Close()
		if readErr != nil || closeErr != nil || len(raw) > protocol.RequestMaxBytes {
			t.Fatal("capacity readiness response unavailable")
		}
		if reply.StatusCode == 200 {
			var list protocol.RecordList
			if json.Unmarshal(raw, &list) != nil || len(list.Items) != 0 || list.NextCursor != nil {
				t.Fatal("unadmitted request changed formal records")
			}
			return attempts
		}
		var refusal protocol.ErrorView
		if reply.StatusCode != 429 || json.Unmarshal(raw, &refusal) != nil || refusal.Code != protocol.ErrorCodeCAPACITYLIMIT {
			t.Fatal("capacity readiness had unrelated failure", reply.StatusCode)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatal("capacity handlers did not become available")
		case <-timer.C:
		}
	}
}

// Normal independent production Host: real unfinished HTTP bodies consume A's
// ordinary quota, while global capacity remains available for B's real save.
// This is known transport refusal, not SQLite busy or persistence uncertainty.
func TestV010IndependentSaveCapacityBeforeBodyAndBProgress(t *testing.T) {
	if workspace.WorkspaceOperationLimit != 32 || workspace.InstallationOperationLimit != 64 {
		t.Fatal("fixed acceptance capacity profile changed")
	}
	f := newFixture(t)
	f.initialize(t)
	p := f.start(t)
	c := p.connect(t)
	_, a := newInspectionWorkspace(t, c, "capacity-A")
	_, b := newInspectionWorkspace(t, c, "capacity-B")
	a = c.open(t, a.WorkspaceId)
	b = c.open(t, b.WorkspaceId)
	pa := "/v1/workspaces/" + string(a.WorkspaceId)
	pb := "/v1/workspaces/" + string(b.WorkspaceId)
	ca := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "unadmitted A", Body: "capacity refusal must not consume this candidate"})
	cb := savePreview(t, c, b, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "independent B", Body: "actual confirmed B while all31 A bodies remain withheld"})
	saveFacts(t, c, a.WorkspaceId, "0", 0)
	saveFacts(t, c, b.WorkspaceId, "0", 0)
	key := newID(t)
	input := saveInput(key, a, ca)
	raw, err := json.Marshal(input)
	if err != nil || len(raw) == 0 {
		t.Fatal("valid victim input unavailable")
	}
	started := time.Now()
	// Product ReadTimeout remains15s. The ENTIRE pressure phase must fit12s;
	// post-B429 additionally proves no holder expiry freed an ordinary slot.
	deadline := started.Add(12 * time.Second)
	var holders []net.Conn
	for i := 0; i < 31; i++ {
		conn, reply := capacityHeaders(t, c, "GET", pa+"/records", 1, deadline)
		if reply.StatusCode != 100 {
			t.Fatal("holder did not actually acquire quota before body read", i, reply.StatusCode)
		}
		if err := reply.Body.Close(); err != nil {
			t.Fatal("holder interim response end unavailable")
		}
		holders = append(holders, conn)
	}
	before := capacityRefusal(t, c, pa+"/record-commits", len(raw), deadline)
	var rb protocol.Receipt
	c.request(t, "POST", pb+"/record-commits", saveInput(newID(t), b, cb), &rb, 200)
	saveMatches(t, saveFacts(t, c, b.WorkspaceId, "1", 1)[0], rb, cb)
	after := capacityRefusal(t, c, pa+"/record-commits", len(raw), deadline)
	pressure := time.Since(started)
	if !time.Now().Before(deadline) {
		t.Fatal("pressure phase exceeded bound before Host read timeout")
	}
	for _, conn := range holders {
		if err := conn.Close(); err != nil {
			t.Fatal("holder socket did not close")
		}
	}
	// Socket close is not a handler/Host join. Observe public availability,
	// then formal/history invariants; final normal process end is separate.
	attempts := capacityReadiness(t, c, pa+"/records")
	saveFacts(t, c, a.WorkspaceId, "0", 0)
	var missing protocol.ErrorView
	c.request(t, "GET", pa+"/operations/"+string(key), nil, &missing, 409)
	if missing.Code != protocol.ErrorCodeOPERATIONHISTORYUNAVAILABLE || missing.OperationId == nil || *missing.OperationId != key {
		t.Fatal("unadmitted transport refusal created durable operation history")
	}
	// Explicit first admission of that SAME valid candidate/key after a known
	// transport refusal; not automatic replay of an unknown business operation.
	var ra, again protocol.Receipt
	c.request(t, "POST", pa+"/record-commits", input, &ra, 200)
	c.request(t, "POST", pa+"/record-commits", input, &again, 200)
	if ra != again || ra.OperationId != key {
		t.Fatal("first admitted candidate/key did not have one stable receipt")
	}
	saveMatches(t, saveFacts(t, c, a.WorkspaceId, "1", 1)[0], ra, ca)
	saveMatches(t, saveFacts(t, c, b.WorkspaceId, "1", 1)[0], rb, cb)
	var op protocol.OperationView
	c.request(t, "GET", pa+"/operations/"+string(key), nil, &op, 200)
	if op.Scope != "workspace" || op.WorkspaceId == nil || *op.WorkspaceId != a.WorkspaceId || op.OperationId != key || op.LogicalState != "committed" || op.PersistenceState != "known" || op.Receipt == nil || *op.Receipt != ra {
		t.Fatal("post-capacity original key crossed result/scope")
	}
	closePublic(t, c, a)
	closePublic(t, c, b)
	p.stop(t)
	matrixEvidence(t, f, "save-capacity", struct {
		PID, Holders, WithheldJSONLength, ReadinessAttempts int
		PressureNanoseconds                                 int64
		Before, After, MissingHistory                       protocol.ErrorView
		A, B                                                protocol.Receipt
		Original                                            protocol.OperationView
		NormalHostEnded                                     bool
	}{p.cmd.Process.Pid, len(holders), len(raw), attempts, pressure.Nanoseconds(), before, after, missing, ra, rb, op, true})
}
