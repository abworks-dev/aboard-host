//go:build windows && amd64

package security

import (
	"context"
	"testing"
	"time"
)

func TestControlPipeReusesFirstInstanceAcrossConnectionCancellationOrders(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	for n := 0; n < 32; n++ {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			done := make(chan error, 1)
			joined := false
			defer func() {
				cancel()
				if !joined {
					<-done
				}
			}()
			issuer := &pipeReuseIssuer{sessions: NewSessions()}
			go func() { done <- pipe.ServeOne(ctx, issuer, Grant{"abandoned", "A", "read"}, time.Minute) }()
			// Real client/kernel connection, no greeting or authority-bearing payload.
			abandonedPipeClient(t, pipe)
			cancel()
			firstErr := <-done
			joined = true
			if firstErr == nil || issuer.issued != 0 {
				t.Fatal("abandoned/canceled connection unexpectedly completed or minted authority")
			}
			assertSamePipeSession(t, pipe, name, NewSessions())
		}()
	}
	// These are actual ordering trials, not proof a particular CancelIoEx/Connect
	// completion race occurred in every iteration.
}

func TestControlPipeSameInstanceMetadataAfterAbandonedAccept(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	abandonedPipeClient(t, pipe)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = pipe.ServeOne(ctx, NewSessions(), Grant{}, time.Minute); err == nil {
		t.Fatal("abandoned client succeeded")
	}
	called, checked := false, false
	reader := controlReaderFunc(func(_ context.Context, r PreconditionRequest) (PreconditionReply, func(context.Context) error, error) {
		called = true
		return controlTestReply(r), func(context.Context) error { checked = true; return nil }, nil
	})
	done := make(chan error, 1)
	go func() { done <- pipe.ServeControl(ctx, NewSessions(), Grant{}, time.Minute, reader) }()
	request := controlTestRequest()
	reply, clientErr := awaitReusablePipe(ctx, func() (PreconditionReply, error) { return ReceivePrecondition(ctx, name, ProcessID(), request) })
	serverErr := <-done
	if clientErr != nil || serverErr != nil {
		t.Fatalf("same first-instance metadata failed: client=%v server=%v (payload withheld)", clientErr, serverErr)
	}
	if !called || !checked || string(reply.WorkspaceID) != request.WorkspaceID || reply.RegistrationVersion != "1" {
		t.Fatal("new metadata exchange skipped current reader/check or changed scope")
	}
}
