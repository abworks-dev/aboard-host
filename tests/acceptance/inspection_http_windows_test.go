//go:build windows && amd64

package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type observedInspectionHTTP struct {
	ControlBefore, ControlAfter, Before, After uint64
	Available, Conserved, BodyEnded            bool
	ConfigDigest                               string
}
type inspectionHTTPWatch struct {
	mu                           sync.Mutex
	authority, challenge, digest string
	requests                     int
	invalid                      bool
	ids                          []string
	handlers                     int
	connections                  map[net.Conn]bool
	changed                      chan struct{}
	server                       *http.Server
	done                         chan error
	attempted, joined            bool // only first completed native/handler join proves safe reuse
}

func (w *inspectionHTTPWatch) signalLocked() { close(w.changed); w.changed = make(chan struct{}) }
func startInspectionHTTPWatch(t *testing.T, f *fixture, root string, measured *fixture) *inspectionHTTPWatch {
	t.Helper()
	if _, present := os.LookupEnv("ABOARD_ACCEPTANCE_INSPECTION_HTTP"); present {
		t.Fatal("conflicting inherited HTTP observation environment")
	}
	challenge := strings.ReplaceAll(string(newID(t)), "-", "")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal("owned HTTP control listener unavailable")
	}
	w := &inspectionHTTPWatch{authority: listener.Addr().String(), challenge: challenge, connections: make(map[net.Conn]bool), changed: make(chan struct{}), done: make(chan error, 1)}
	w.server = &http.Server{ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: time.Second}
	w.server.ConnState = func(conn net.Conn, state http.ConnState) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if state == http.StateClosed {
			delete(w.connections, conn)
		} else {
			w.connections[conn] = true
			if state == http.StateHijacked {
				w.invalid = true
			}
		}
		w.signalLocked()
	}
	w.server.Handler = http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		w.handlers++
		w.requests++
		w.signalLocked()
		w.mu.Unlock()
		defer func() { w.mu.Lock(); w.handlers--; w.signalLocked(); w.mu.Unlock() }()
		prefix := "/aboard-acceptance-inspection-control/" + w.challenge + "/"
		id := strings.TrimPrefix(r.URL.Path, prefix)
		decoded, e := hex.DecodeString(id)
		valid := r.Method == http.MethodGet && r.Host == w.authority && strings.HasPrefix(r.URL.Path, prefix) && len(decoded) == 16 && e == nil && hex.EncodeToString(decoded) == id && r.URL.RawQuery == "" && r.URL.RawPath == "" && !r.URL.IsAbs() && r.URL.Host == "" && r.URL.User == nil && r.ContentLength == 0 && len(r.TransferEncoding) == 0 && len(r.Header.Values("Authorization")) == 0 && len(r.Header.Values("Proxy-Authorization")) == 0 && len(r.Header.Values("Cookie")) == 0 && r.Close
		w.mu.Lock()
		w.invalid = w.invalid || !valid
		w.ids = append(w.ids, id)
		w.mu.Unlock()
		out.Header().Set("Connection", "close")
		if !valid {
			out.WriteHeader(400)
			return
		}
		body := "aboard-inspection-control:" + w.challenge + ":" + id
		out.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = fmt.Fprint(out, body)
	})
	go func() { w.done <- w.server.Serve(listener) }()
	t.Cleanup(func() { w.close(t) })
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	raw, err := json.Marshal(struct {
		Version                        int `json:"version"`
		Fixture, Root, Port, Challenge string
	}{1, f.root, root, port, challenge})
	if err != nil {
		_ = listener.Close()
		t.Fatal("HTTP control config unavailable")
	}
	digest := sha256.Sum256(raw)
	w.digest = hex.EncodeToString(digest[:])
	measured.environment = append(measured.environment, "ABOARD_ACCEPTANCE_INSPECTION_HTTP="+base64.RawURLEncoding.EncodeToString(raw))
	// Caller registers close immediately before starting Host; restore gate requires
	// joined, not merely server.Close/Serve-return or context cancellation.
	return w
}
func (w *inspectionHTTPWatch) verify(t *testing.T, record observedInspectionIO) {
	t.Helper()
	v := record.HTTP
	if v == nil || !v.Available || !v.Conserved || !v.BodyEnded || v.ConfigDigest != w.digest || v.ControlAfter != v.ControlBefore+1 || v.Before != v.ControlAfter || v.After != v.Before {
		t.Fatal("missing/unavailable/nonconserving actual default HTTP delegate evidence")
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		w.mu.Lock()
		valid := !w.invalid && w.requests == 1 && len(w.ids) == 1 && w.ids[0] == record.ID
		idle := w.handlers == 0 && len(w.connections) == 0
		changed := w.changed
		w.mu.Unlock()
		if !valid {
			t.Fatal("test-owned HTTP positive identity/request count failed")
		}
		if idle {
			return
		}
		select {
		case <-changed:
		case <-deadline.C:
			t.Fatal("HTTP positive handler/connection end not proven")
		}
	}
}
func (w *inspectionHTTPWatch) close(t *testing.T) {
	t.Helper()
	if w.attempted {
		return
	}
	w.attempted = true
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	shutdownErr := w.server.Shutdown(ctx)
	if shutdownErr != nil {
		if err := w.server.Close(); err != nil {
			t.Error("owned HTTP server close failed")
			return
		}
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case err := <-w.done:
		if err != nil && err != http.ErrServerClosed {
			t.Error("owned HTTP Serve did not end normally")
			return
		}
	case <-timer.C:
		t.Error("owned HTTP Serve not joined")
		return
	}
	for {
		w.mu.Lock()
		idle := w.handlers == 0 && len(w.connections) == 0
		valid := !w.invalid && w.requests == 1
		changed := w.changed
		w.mu.Unlock()
		if idle {
			if !valid {
				t.Error("unexpected HTTP control request count")
				return
			}
			w.joined = true
			return
		}
		select {
		case <-changed:
		case <-timer.C:
			t.Error("owned HTTP handlers/connections not joined")
			return
		}
	}
}
