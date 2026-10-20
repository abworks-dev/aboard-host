//go:build windows && amd64

package httptransport

import (
	"context"
	"sync"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// Request quiescence is not a second operation queue or capacity owner. It lets
// Host stop new HTTP work and join already entered handlers before teardown.
// Durable/unknown work and native resources remain owned by business adapters.
type requestDrain struct {
	mu               sync.Mutex
	draining, paused bool
	active, mutating int
	changed          chan struct{}
}

func newRequestDrain() *requestDrain { return &requestDrain{changed: make(chan struct{})} }
func (g *requestDrain) signal()      { close(g.changed); g.changed = make(chan struct{}) }
func investigationRoute(route protocol.Route) bool {
	return route.ID == "getContext" || route.ID == "getWorkspace" || route.ID == "getRecord" || route.ID == "listRecords" || route.ID == "getWorkspaceOperation" || route.ID == "getInstallationOperation" || route.ID == "reconcileWorkspaceOperation" || route.ID == "reconcileInstallationOperation" || route.ID == "closeWorkspace"
}
func (g *requestDrain) enter(route protocol.Route) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	mutation := !investigationRoute(route)
	if g.paused || (g.draining && mutation) {
		return nil, false
	}
	g.active++
	if mutation {
		g.mutating++
	}
	g.signal()
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.active--
			if mutation {
				g.mutating--
			}
			g.signal()
		})
	}, true
}
func (g *requestDrain) wait(ctx context.Context, predicate func() bool) error {
	for {
		g.mu.Lock()
		ready := predicate()
		changed := g.changed
		g.mu.Unlock()
		if ready {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
func (h *Handler) FreezeNewWork() {
	g := h.drain
	g.mu.Lock()
	g.draining = true
	g.signal()
	g.mu.Unlock()
}
func (h *Handler) BeginDrain(ctx context.Context) error {
	h.FreezeNewWork()
	g := h.drain
	return g.wait(ctx, func() bool { return g.mutating == 0 })
}
func (h *Handler) Pause(ctx context.Context) error {
	g := h.drain
	g.mu.Lock()
	g.paused = true
	g.signal()
	g.mu.Unlock()
	return g.wait(ctx, func() bool { return g.active == 0 })
}
func (h *Handler) ResumeInvestigation() {
	g := h.drain
	g.mu.Lock()
	g.paused = false
	g.signal()
	g.mu.Unlock()
}
func (h *Handler) Changes() <-chan struct{} {
	g := h.drain
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.changed
}
func (h *Handler) isDraining() bool {
	g := h.drain
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.draining
}
