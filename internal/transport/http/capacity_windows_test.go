//go:build windows && amd64

package httptransport

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// A real 100 Continue proves the handler reached Body.Read, after acquiring its
// claim. The one-byte GET body is intentionally unfinished/invalid, not an intent.
func holdHTTPBody(t *testing.T, f httpFixture, path string) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", f.handler.authority, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: 1\r\nExpect: 100-continue\r\n\r\n", path, f.handler.authority, f.token)
	if err != nil {
		c.Close()
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: "GET"})
	if err != nil {
		c.Close()
		t.Fatal(err)
	}
	if response.StatusCode != 100 {
		c.Close()
		t.Fatalf("body did not acquire quota before read: %d", response.StatusCode)
	}
	return c
}
func TestHTTPActualSlowBodiesShareLocalAndInstallationLimits(t *testing.T) {
	f := newHTTPFixture(t)
	a := f.create(t, "quota-A", "76000000-0000-4000-8000-000000000001")
	b := f.create(t, "quota-B", "76000000-0000-4000-8000-000000000002")
	f.open(t, a.WorkspaceId)
	f.open(t, b.WorkspaceId)
	var held []net.Conn
	defer func() {
		for _, c := range held {
			c.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := f.handler.Pause(ctx); err != nil {
			t.Error("body claims failed to drain", err)
		}
	}()
	pa := "/v1/workspaces/" + string(a.WorkspaceId) + "/records"
	pb := "/v1/workspaces/" + string(b.WorkspaceId) + "/records"
	for i := 0; i < workspace.WorkspaceOperationLimit-1; i++ {
		held = append(held, holdHTTPBody(t, f, pa))
	}
	status, raw := f.request(t, "GET", pa, nil)
	requireStatus(t, status, 429)
	var rejected protocol.ErrorView
	decodeResponse(t, raw, &rejected)
	if rejected.Code != protocol.ErrorCodeCAPACITYLIMIT {
		t.Fatal("not a capacity rejection")
	}
	// A's slow body readers neither occupy B's execution loop nor B's local quota.
	status, _ = f.request(t, "GET", pb, nil)
	requireStatus(t, status, 200)
	for i := 0; i < workspace.WorkspaceOperationLimit-1; i++ {
		held = append(held, holdHTTPBody(t, f, pb))
	}
	held = append(held, holdHTTPBody(t, f, "/v1/context")) // 31 A + 31 B + 1 installation
	status, raw = f.request(t, "GET", "/v1/context", nil)
	requireStatus(t, status, 429)
	decodeResponse(t, raw, &rejected)
	if rejected.Code != protocol.ErrorCodeCAPACITYLIMIT {
		t.Fatal("installation-only traffic bypassed quota")
	}
	// Request completion, not client cancellation alone, permits final readiness.
	for _, c := range held {
		c.Close()
	}
	held = nil
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := f.handler.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	f.handler.ResumeInvestigation()
	status, _ = f.request(t, "GET", "/v1/context", nil)
	requireStatus(t, status, 200)
}

type deliveryBarrier struct {
	*httptest.ResponseRecorder
	entered, release chan struct{}
	once             sync.Once
	failure          error
}

func (w *deliveryBarrier) FlushError() error {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.failure
}
func directRequest(f httpFixture, method, path string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, path, body)
	r.Host = f.handler.authority
	r.Header.Set("Authorization", "Bearer "+f.token)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}
func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("operation did not reach barrier")
	}
}
func ownerAuthorization(t *testing.T, f httpFixture) *installation.Authorization {
	t.Helper()
	a, err := f.auth.Authorize(context.Background(), f.token, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func fillOrdinary(t *testing.T, f httpFixture, a *installation.Authorization, n int) []*workspace.OperationReservation {
	t.Helper()
	var refs []*workspace.OperationReservation
	for i := 0; i < n; i++ {
		r, err := f.store.ReserveOperation(context.Background(), a)
		if err != nil {
			for _, r := range refs {
				r.Release()
			}
			t.Fatal("unexpected double charge", i, err)
		}
		refs = append(refs, r)
	}
	return refs
}
func TestHTTPDeliveryFlushRetainsOneClaimAndQuiescence(t *testing.T) {
	f := newHTTPFixture(t)
	a := ownerAuthorization(t, f)
	w := &deliveryBarrier{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{}), failure: io.ErrClosedPipe}
	done := make(chan struct{})
	go func() { f.handler.ServeHTTP(w, directRequest(f, "GET", "/v1/context", nil)); close(done) }()
	waitSignal(t, w.entered)
	var release sync.Once
	defer func() { release.Do(func() { close(w.release) }); waitSignal(t, done) }()
	refs := fillOrdinary(t, f, a, workspace.InstallationOperationLimit-2)
	defer func() {
		for _, r := range refs {
			r.Release()
		}
	}()
	if r, err := f.store.ReserveOperation(context.Background(), a); !errors.Is(err, workspace.ErrCapacity) {
		r.Release()
		t.Fatal("response Flush released its claim", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := f.handler.Pause(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Flush not part of active handler", err)
	}
	release.Do(func() { close(w.release) })
	waitSignal(t, done)
	r, err := f.store.ReserveOperation(context.Background(), a)
	if err != nil {
		t.Fatal("delivery error leaked claim", err)
	}
	r.Release()
	events := f.events.snapshot()
	found := false
	for _, e := range events {
		found = found || e.Code == protocol.ErrorCodeVIEWDELIVERYFAILED
	}
	if !found {
		t.Fatal("Flush delivery failure not reported")
	}
}

type observedBody struct {
	reads     atomic.Int32
	panicRead bool
}

func (b *observedBody) Read([]byte) (int, error) {
	b.reads.Add(1)
	if b.panicRead {
		panic("synthetic body panic")
	}
	return 0, io.EOF
}
func (*observedBody) Close() error { return nil }
func TestHTTPQuotaRejectsBeforeBodyAndPanicReleasesAfterFailureFlush(t *testing.T) {
	f := newHTTPFixture(t)
	a := ownerAuthorization(t, f)
	refs := fillOrdinary(t, f, a, workspace.InstallationOperationLimit-1)
	body := &observedBody{panicRead: true}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, directRequest(f, "GET", "/v1/context", body))
	if w.Code != 429 || body.reads.Load() != 0 {
		t.Fatal("capacity rejection consumed body", w.Code, body.reads.Load())
	}
	for _, r := range refs {
		r.Release()
	}
	body = &observedBody{panicRead: true}
	barrier := &deliveryBarrier{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() { f.handler.ServeHTTP(barrier, directRequest(f, "GET", "/v1/context", body)); close(done) }()
	waitSignal(t, barrier.entered)
	var once sync.Once
	defer func() { once.Do(func() { close(barrier.release) }); waitSignal(t, done) }()
	refs = fillOrdinary(t, f, a, workspace.InstallationOperationLimit-2)
	defer func() {
		for _, r := range refs {
			r.Release()
		}
	}()
	if r, err := f.store.ReserveOperation(context.Background(), a); !errors.Is(err, workspace.ErrCapacity) {
		r.Release()
		t.Fatal("panic path dropped request before failure Flush", err)
	}
	once.Do(func() { close(barrier.release) })
	waitSignal(t, done)
	if barrier.Code != 503 || strings.Contains(barrier.Body.String(), "synthetic body panic") {
		t.Fatal("unsafe panic response")
	}
}

func TestHTTPConfirmedSaveDeliveryFailureDoesNotRewriteReceipt(t *testing.T) {
	f := newHTTPFixture(t)
	v := f.create(t, "delivery-save", "76000000-0000-4000-8000-000000000030")
	v = f.open(t, v.WorkspaceId)
	prefix := "/v1/workspaces/" + string(v.WorkspaceId)
	status, raw := f.request(t, "POST", prefix+"/record-previews", encode(t, protocol.PreviewRecord{OpenEpoch: *v.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "committed", Body: "exact bytes"}))
	requireStatus(t, status, 201)
	var p protocol.CandidateView
	decodeResponse(t, raw, &p)
	input := protocol.CommitRecord{OperationId: "76000000-0000-4000-8000-000000000031", OpenEpoch: *v.OpenEpoch, CandidateId: p.CandidateId, CandidateDigest: p.CandidateDigest, PreviewVersion: p.PreviewVersion}
	request := directRequest(f, "POST", prefix+"/record-commits", bytes.NewReader(encode(t, input)))
	w := &deliveryBarrier{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{}), failure: io.ErrClosedPipe}
	done := make(chan struct{})
	go func() { f.handler.ServeHTTP(w, request); close(done) }()
	waitSignal(t, w.entered)
	var once sync.Once
	defer func() { once.Do(func() { close(w.release) }); waitSignal(t, done) }()
	owner := ownerAuthorization(t, f)
	refs := fillOrdinary(t, f, owner, workspace.InstallationOperationLimit-2)
	defer func() {
		for _, r := range refs {
			r.Release()
		}
	}()
	if r, err := f.store.ReserveOperation(context.Background(), owner); !errors.Is(err, workspace.ErrCapacity) {
		r.Release()
		t.Fatal("confirmed save released response before Flush", err)
	}
	once.Do(func() { close(w.release) })
	waitSignal(t, done)
	requireStatus(t, w.Code, 200)
	var receipt protocol.Receipt
	decodeResponse(t, w.Body.Bytes(), &receipt)
	status, raw = f.request(t, "GET", prefix+"/operations/"+string(input.OperationId), nil)
	requireStatus(t, status, 200)
	var evidence protocol.OperationView
	decodeResponse(t, raw, &evidence)
	if evidence.LogicalState != "committed" || evidence.PersistenceState != "known" {
		t.Fatal("delivery error rewrote confirmed truth")
	}
	status, raw = f.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil)
	requireStatus(t, status, 200)
	var row protocol.TextRecordView
	decodeResponse(t, raw, &row)
	if row.Body != "exact bytes" || row.OperationId != input.OperationId || row.WorkspaceRevision != "1" {
		t.Fatal("delivery error replayed or lost formal text")
	}
}
