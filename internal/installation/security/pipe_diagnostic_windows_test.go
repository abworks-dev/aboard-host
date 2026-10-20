//go:build windows && amd64

package security

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	win "golang.org/x/sys/windows"
)

func TestPipeDiagnosticPreservesCauseWithoutRenderingIt(t *testing.T) {
	for _, cause := range []error{ErrUnauthorized, context.Canceled, context.DeadlineExceeded, win.ERROR_PIPE_NOT_CONNECTED, errors.New("synthetic-secret-C:\\private\\locator")} {
		err := pipeFault("client-header-shape", cause)
		if !errors.Is(err, cause) {
			t.Fatal("lost original cause")
		}
		got := DiagnosePipe(context.Background(), err, 13*time.Millisecond)
		raw, e := json.Marshal(got)
		if e != nil || got.Stage != "client-header-shape" || got.ElapsedMillis != 13 || strings.Contains(string(raw), "synthetic-secret") || strings.Contains(string(raw), "locator") || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("unsafe or incorrect diagnostic")
		}
	}
	inner := pipeFault("client-server-pid-match", ErrUnauthorized)
	if DiagnosePipe(nil, pipeFault("client-server", inner), 0).Stage != "client-server-pid-match" {
		t.Fatal("lost inner stage")
	}
	joined := errors.Join(pipeFault("server-final-ack", context.DeadlineExceeded), pipeFault("server-reset", win.ERROR_ACCESS_DENIED))
	d := DiagnosePipe(nil, joined, 0)
	if d.Stage != "server-final-ack" || d.Outcome != "deadline" || d.ResetOutcome != "native" || d.ResetNative != uint32(win.ERROR_ACCESS_DENIED) {
		t.Fatal("lost primary/reset causes")
	}
	for _, row := range []struct {
		primary error
		outcome string
		code    uint32
	}{
		{io.ErrUnexpectedEOF, "other", 0},
		{win.ERROR_PIPE_NOT_CONNECTED, "native", 233},
		{errors.Join(ErrUnauthorized, win.ERROR_ACCESS_DENIED), "unauthorized", 5},
	} {
		got := DiagnosePipe(nil, errors.Join(pipeFault("server-final-ack", row.primary), pipeFault("server-reset", win.ERROR_ACCESS_DENIED)), 0)
		if got.Outcome != row.outcome || got.Native != row.code || got.ResetNative != 5 {
			t.Fatal("secondary reset contaminated primary diagnostic", got)
		}
	}
	// Only CreateFile's bare preconnection BUSY is retryable; wrapped BUSY is not.
	calls := 0
	_, e := awaitReusablePipe(context.Background(), func() (string, error) { calls++; return "", pipeFault("client-first-ack", win.ERROR_PIPE_BUSY) })
	if calls != 1 || !errors.Is(e, win.ERROR_PIPE_BUSY) {
		t.Fatal("retried connected BUSY")
	}
}

// Deterministic order: no Serve/Connect goroutine exists before these queries.
// This tests the preaccept metadata hypothesis on this platform, not a historical
// failure attribution or a separate-process identity proof. No IO is pending.
func TestControlPipeServerIdentityBeforeFirstAccept(t *testing.T) {
	p, e := NewControlPipe(pipeName(t))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := p.Close(); e != nil {
			t.Error(e)
		}
	}()
	name, e := win.UTF16PtrFromString(p.name)
	if e != nil {
		t.Fatal(e)
	}
	h, e := win.CreateFile(name, win.GENERIC_READ|win.GENERIC_WRITE|win.READ_CONTROL, 0, nil, win.OPEN_EXISTING, win.FILE_FLAG_OVERLAPPED|win.SECURITY_SQOS_PRESENT|win.SECURITY_IMPERSONATION, 0)
	if e != nil {
		t.Fatal("preaccept client unavailable")
	}
	defer func() {
		if e := win.CloseHandle(h); e != nil {
			t.Error(e)
		}
	}()
	sid, e := currentSID()
	if e != nil {
		t.Fatal(e)
	}
	if e = checkPrivateObject(h, sid, false); e != nil {
		t.Fatal("preaccept private object", e)
	}
	server, e := verifyPipeServer(h, ProcessID(), sid)
	if e != nil {
		t.Fatal("preaccept identity refused", DiagnosePipe(nil, e, 0))
	}
	if e = win.CloseHandle(server); e != nil {
		t.Fatal("server pin did not close")
	}
}

// The next client starts immediately after its own ACK4, not after a test join
// of the server's previous Serve return. This exercises the production serial
// first-instance lifecycle; each failed exchange is terminal, never retried.
func TestControlPipeImmediateSessionReuseLoop(t *testing.T) {
	name := pipeName(t)
	pipe, e := NewControlPipe(name)
	if e != nil {
		t.Fatal(e)
	}
	defer pipe.Close()
	sessions := NewSessions()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	done := make(chan error, 1)
	const rounds = 400
	go func() {
		for n := 0; n < rounds; n++ {
			e := pipe.ServeOne(ctx, sessions, Grant{"reuse", "A", "read"}, time.Minute)
			if e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	joined := false
	defer func() {
		if !joined {
			cancel()
			e := <-done
			if e != nil {
				t.Error("server stage", DiagnosePipe(ctx, e, 0))
			}
		}
		cancel()
	}()
	for n := 0; n < rounds; n++ {
		token, e := awaitReusablePipe(ctx, func() (string, error) { return ReceiveSession(ctx, name, ProcessID()) })
		if e != nil {
			t.Fatal("client stage", n, DiagnosePipe(ctx, e, 0))
		}
		if _, e = sessions.Authorize(token, "A", "read"); e != nil {
			t.Fatal("unconfirmed authority", n)
		}
	}
	e = <-done
	joined = true
	if e != nil {
		t.Fatal("server stage", DiagnosePipe(ctx, e, 0))
	}
}
