//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
)

func TestAcceptanceInspectionHTTPConfigHardGate(t *testing.T) {
	c := inspectionObservationConfig{Version: 1, Fixture: "fixed-fixture", Root: "fixed-root"}
	h := inspectionHTTPConfig{Version: 1, Fixture: c.Fixture, Root: c.Root, Port: "12345", Challenge: strings.Repeat("a", 32)}
	raw, _ := json.Marshal(h)
	read := func(raw []byte) bool {
		_, ok := readInspectionHTTP(c, base64.RawURLEncoding.EncodeToString(raw))
		return ok
	}
	if !read(raw) {
		t.Fatal("valid fixed loopback config rejected")
	}
	invalid := []string{"null", `{"version":1}`, strings.Replace(string(raw), `"version":1`, `"version":1e0`, 1), strings.Replace(string(raw), `"Fixture"`, `"fixture"`, 1), strings.Replace(string(raw), `"Port":"12345"`, `"Port":null`, 1), strings.TrimSuffix(string(raw), "}") + `,"Port":"1"}`, strings.TrimSuffix(string(raw), "}") + `,"url":"http://elsewhere"}`}
	for n, s := range invalid {
		if read([]byte(s)) {
			t.Fatal("nonexact config accepted", n)
		}
	}
	for _, port := range []string{"0", "65536", "0123", "+123", "123.0", "123?x", "localhost:123", "-1"} {
		x := h
		x.Port = port
		b, _ := json.Marshal(x)
		if read(b) {
			t.Fatal("noncanonical loopback port accepted", port)
		}
	}
	for _, challenge := range []string{"", strings.Repeat("A", 32), strings.Repeat("a", 31), strings.Repeat("z", 32)} {
		x := h
		x.Challenge = challenge
		b, _ := json.Marshal(x)
		if read(b) {
			t.Fatal("invalid challenge accepted")
		}
	}
	for _, field := range []string{"Fixture", "Root"} {
		x := h
		if field == "Fixture" {
			x.Fixture = "other"
		} else {
			x.Root = "other"
		}
		b, _ := json.Marshal(x)
		if read(b) {
			t.Fatal("cross-fixture config accepted")
		}
	}
	if _, ok := readInspectionHTTP(c, strings.Repeat("x", 8193)); ok {
		t.Fatal("unbounded config accepted")
	}
}

type httpDelegatePolicy struct {
	mu            sync.Mutex
	calls, closed int
	request       *http.Request
	response      *http.Response
	err           error
}

func (p *httpDelegatePolicy) RoundTrip(r *http.Request) (*http.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.request = r
	return p.response, p.err
}
func (p *httpDelegatePolicy) CloseIdleConnections() { p.mu.Lock(); p.closed++; p.mu.Unlock() }
func TestAcceptanceHTTPDelegatePreservesResultsAndCounts(t *testing.T) {
	cause := errors.New("synthetic delegation result")
	response := &http.Response{StatusCode: 503}
	original := &httpDelegatePolicy{response: response, err: cause}
	d := &inspectionHTTPDelegate{original: original}
	request, _ := http.NewRequest("GET", "http://127.0.0.1:1/fixed", nil)
	var wg sync.WaitGroup
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := d.RoundTrip(request)
			if r != response || e != cause {
				t.Error("delegate rewrote result")
			}
		}()
	}
	wg.Wait()
	d.CloseIdleConnections()
	if d.attempts.Load() != 20 || original.calls != 20 || original.closed != 1 || original.request != request {
		t.Fatal("counter/delegate/idle forwarding mismatch")
	}
	// Fake only proves delegation policy; real wire positive is separate below.
}
func TestAcceptanceInspectionHTTPActualDefaultControl(t *testing.T) {
	InstallAcceptanceHTTPObservation()
	installed := http.DefaultTransport
	InstallAcceptanceHTTPObservation()
	if http.DefaultTransport != installed {
		t.Fatal("installer replaced live delegate")
	}
	challenge, id := strings.Repeat("a", 32), strings.Repeat("b", 32)
	for _, oversize := range []bool{false, true} {
		t.Run(strconv.FormatBool(oversize), func(t *testing.T) {
			var calls atomic.Uint32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || len(r.Header.Values("Authorization")) != 0 || len(r.Header.Values("Cookie")) != 0 || len(r.Header.Values("Proxy-Authorization")) != 0 || r.URL.Path != inspectionHTTPControlPath+challenge+"/"+id {
					t.Error("control request changed")
				}
				w.Header().Set("Connection", "close")
				body := "aboard-inspection-control:" + challenge + ":" + id
				if oversize {
					body = strings.Repeat("x", 130)
				}
				w.Header().Set("Content-Length", strconv.Itoa(len(body)))
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			port := strconv.Itoa(server.Listener.Addr().(*net.TCPAddr).Port)
			owner := &inspectionOwner{}
			result := inspectionHTTPControl(inspectionHTTPConfig{Version: 1, Fixture: "synthetic", Root: "synthetic", Port: port, Challenge: challenge}, id, owner)
			if calls.Load() != 1 || result.ControlAfter != result.ControlBefore+1 {
				t.Fatal("real DEFAULT invocation not observed exactly once")
			}
			if oversize {
				if result.Available || result.BodyEnded || !errors.Is(owner.failure, platform.ErrPhysicalCloseUnknown) {
					t.Fatal("capped non-EOF discarded original control ownership")
				}
				var retained *inspectionHTTPEndUnknown
				if !errors.As(owner.failure, &retained) || retained.client == nil || retained.request == nil || retained.response == nil {
					t.Fatal("unknown body lost resource references")
				}
			} else if !result.Available || !result.BodyEnded || owner.failure != nil {
				t.Fatal("real local positive unavailable")
			}
		})
	}
}
