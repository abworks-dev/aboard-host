//go:build windows && amd64

package httptransport

import (
	"context"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

func TestHTTPDrainAdmissionKeepsInvestigationAndJoinsEnteredWork(t *testing.T) {
	h := &Handler{drain: newRequestDrain()}
	command := protocol.Route{ID: "createWorkspace"}
	query := protocol.Route{ID: "getInstallationOperation"}
	leave, ok := h.drain.enter(command)
	if !ok {
		t.Fatal("initial command rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if h.BeginDrain(ctx) == nil {
		t.Fatal("canceled waiter claimed active command ended")
	}
	if _, ok = h.drain.enter(command); ok {
		t.Fatal("new command entered draining Host")
	}
	investigation, ok := h.drain.enter(query)
	if !ok {
		t.Fatal("drain removed investigation access")
	}
	investigation()
	leave()
	leave() // exact release, including waiter/normal completion races
	if err := h.BeginDrain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := h.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok = h.drain.enter(query); ok {
		t.Fatal("final quiescence still admitted query")
	}
	h.ResumeInvestigation()
	for _, id := range []string{"getWorkspaceOperation", "reconcileWorkspaceOperation", "getInstallationOperation", "reconcileInstallationOperation", "closeWorkspace"} {
		leave, ok = h.drain.enter(protocol.Route{ID: id})
		if !ok {
			t.Fatal("investigation cannot resume", id)
		}
		leave()
	}
	for _, id := range []string{"createWorkspace", "registerWorkspace", "relinkWorkspace", "openWorkspace", "previewRecord", "commitRecord", "inspectWorkspace"} {
		if _, ok = h.drain.enter(protocol.Route{ID: id}); ok {
			t.Fatal("resume lifted irreversible mutation fence", id)
		}
	}
	// Waiting only observes completion. A live HTTP waiter is never decremented
	// by a timeout in the Host's final quiescence waiter.
	leave, ok = h.drain.enter(query)
	if !ok {
		t.Fatal("query rejected")
	}
	timeout, end := context.WithTimeout(context.Background(), time.Millisecond)
	defer end()
	if h.Pause(timeout) == nil {
		t.Fatal("active query was not joined")
	}
	leave()
	if err := h.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
}
