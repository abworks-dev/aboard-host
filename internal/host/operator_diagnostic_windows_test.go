//go:build windows && amd64

package host

import (
	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"testing"
)

func TestOperatorDiagnosticRingFlushRequiresActualStop(t *testing.T) {
	o := &operatorServer{done: make(chan struct{})}
	calls := 0
	o.observe = func(attempt uint64, d security.PipeDiagnostic) {
		calls++
		if attempt != uint64(calls+26) || d.Stage != "complete" {
			t.Error("ring ordering lost")
		}
	}
	for n := uint64(1); n <= 90; n++ {
		o.recordObservation(n, security.PipeDiagnostic{Stage: "complete", Outcome: "ok"})
	}
	o.flushObservations()
	if calls != 0 || len(o.observations) != 64 || o.observationDropped != 26 {
		t.Fatal("active operator emitted IO or ring unbounded")
	}
	close(o.done)
	o.flushObservations()
	o.flushObservations()
	if calls != 64 || len(o.observations) != 0 {
		t.Fatal("joined ring not flushed exactly once")
	}
}
