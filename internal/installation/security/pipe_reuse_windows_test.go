//go:build windows && amd64

package security

import (
	"context"
	"errors"
	"testing"
	"time"

	win "golang.org/x/sys/windows"
)

type pipeReuseIssuer struct {
	sessions *Sessions
	issued   int
}

func (i *pipeReuseIssuer) Issue(grant Grant, ttl time.Duration) (string, error) {
	i.issued++
	return i.sessions.Issue(grant, ttl)
}
func (i *pipeReuseIssuer) Revoke(token string) { i.sessions.Revoke(token) }

// Wait only for bare CreateFile ERROR_PIPE_BUSY, which is before connection
// and before any identity/credential bytes. Never retry a connected exchange.
func awaitReusablePipe[T any](ctx context.Context, call func() (T, error)) (T, error) {
	for {
		var zero T
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		value, err := call()
		if err != win.ERROR_PIPE_BUSY {
			return value, err
		}
		timer := time.NewTimer(2 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		}
	}
}

func TestControlPipeAvailabilityNeverRetriesConnectedFailure(t *testing.T) {
	for _, terminal := range []error{win.ERROR_PIPE_NOT_CONNECTED, ErrUnauthorized, errors.Join(win.ERROR_PIPE_BUSY, ErrUnauthorized), context.DeadlineExceeded} {
		calls := 0
		_, got := awaitReusablePipe(context.Background(), func() (int, error) { calls++; return 0, terminal })
		if calls != 1 || got != terminal {
			t.Fatal("non-bare-busy failure retried or replaced")
		}
	}
}

// This client only connects and closes. It sends no hello, request or credential.
func abandonedPipeClient(t *testing.T, pipe *ControlPipe) {
	t.Helper()
	name, err := win.UTF16PtrFromString(pipe.name)
	if err != nil {
		t.Fatal("synthetic pipe name unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	h, err := awaitReusablePipe(ctx, func() (win.Handle, error) {
		return win.CreateFile(name, win.GENERIC_READ|win.GENERIC_WRITE, 0, nil, win.OPEN_EXISTING, win.FILE_FLAG_OVERLAPPED|win.SECURITY_SQOS_PRESENT|win.SECURITY_IMPERSONATION, 0)
	})
	if err != nil {
		t.Fatalf("native pre-accept client unavailable: %v", err)
	}
	if err = win.CloseHandle(h); err != nil {
		t.Fatal("native pre-accept client did not close")
	}
}

// A NEW authenticated exchange, not a retry of the abandoned client's bytes.
// The first-instance server handle must remain the same throughout the test.
func assertSamePipeSession(t *testing.T, pipe *ControlPipe, name string, sessions *Sessions) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	served := make(chan error, 1)
	grant := Grant{"synthetic-reuse", "A", "read"}
	go func() { served <- pipe.ServeOne(ctx, sessions, grant, time.Minute) }()
	token, clientErr := awaitReusablePipe(ctx, func() (string, error) { return ReceiveSession(ctx, name, ProcessID()) })
	serverErr := <-served // Join native cancellation/IO before closing the pipe.
	if clientErr != nil || serverErr != nil {
		t.Fatalf("same first-instance exchange failed: client=%v server=%v (credential withheld)", clientErr, serverErr)
	}
	if actual, err := sessions.Authorize(token, "A", "read"); err != nil || actual != grant {
		t.Fatal("new acknowledged session authority differs")
	}
	if _, err := sessions.Authorize(token, "B", "read"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("new session acquired another workspace")
	}
}

func TestControlPipeReusesFirstInstanceAfterAbandonedPreAcceptClient(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	sessions := NewSessions()
	abandonedPipeClient(t, pipe)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	issuer := &pipeReuseIssuer{sessions: sessions}
	err = pipe.ServeOne(ctx, issuer, Grant{"abandoned", "A", "read"}, time.Minute)
	cancel()
	if !errors.Is(err, win.ERROR_NO_DATA) {
		t.Fatalf("prior closed connection did not reach native no-data accept state: %v", err)
	}
	if issuer.issued != 0 {
		t.Fatal("unverified abandoned client minted a session")
	}
	if duplicate, err := NewControlPipe(name); err == nil {
		duplicate.Close()
		t.Fatal("original first instance was replaced/released")
	}
	assertSamePipeSession(t, pipe, name, sessions)
}

func TestControlPipeReusesFirstInstanceAfterCanceledIdleAccept(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := make(chan error, 1)
	joined := make(chan error, 1)
	go func() {
		joined <- pipeOperation(ctx, pipe.handle, nil, func(ov *win.Overlapped) error {
			nativeErr := win.ConnectNamedPipe(pipe.handle, ov)
			started <- nativeErr
			return nativeErr
		})
	}()
	var nativeErr error
	select {
	case nativeErr = <-started:
		cancel()
		err = <-joined
	case err = <-joined:
		t.Fatalf("native accept ended before start evidence: %v", err)
	}
	if nativeErr != win.ERROR_IO_PENDING {
		t.Fatalf("idle accept never actually pended: %v", nativeErr)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("joined idle accept lost cancellation: %v", err)
	}
	assertSamePipeSession(t, pipe, name, NewSessions())
}
