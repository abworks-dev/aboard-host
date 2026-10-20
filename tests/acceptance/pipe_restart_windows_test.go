//go:build windows && amd64

package acceptance

import (
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"strconv"
	"testing"
)

// Same installed fixture and pipe name, genuinely new normal Host each time.
// This is bounded observed authentication/restart coverage, not a root-cause
// proof, workspace-grant proof or permission to retry a failed exchange.
func TestV010IndependentProtectedSessionRestarts(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	type row struct {
		PID             int
		Birth           uint64
		Identity        protocol.PrincipalView
		Graceful        bool
		ServerConfirmed bool
	}
	rows := make([]row, 0, 64)
	var identity protocol.PrincipalView
	for n := 0; n < 64; n++ {
		if !t.Run(strconv.Itoa(n), func(t *testing.T) {
			p := f.start(t)
			c := p.connect(t)
			var current protocol.PrincipalView
			c.request(t, "GET", "/v1/context", nil, &current, 200)
			allowed := false
			for _, capability := range current.Capabilities {
				if capability == protocol.CapabilityInstallationOperator {
					allowed = true
				}
			}
			if !allowed || current.InstallationId == "" || current.TenantId == "" || current.PrincipalId == "" {
				t.Fatal("actual current installation operator authority absent")
			}
			if n == 0 {
				identity = current
			} else if current.InstallationId != identity.InstallationId || current.TenantId != identity.TenantId || current.PrincipalId != identity.PrincipalId {
				t.Fatal("restart changed current installation identity")
			}
			p.stop(t)
			p.mu.Lock()
			confirmed := false
			for _, event := range p.evidence.Events {
				if event.Event == "operator" && event.Pipe != nil && event.Pipe.Stage == "complete" && event.Pipe.Outcome == "ok" {
					confirmed = true
				}
			}
			rows = append(rows, row{p.evidence.PID, p.birth, current, p.evidence.Graceful, confirmed})
			p.mu.Unlock()
			if !confirmed {
				t.Fatal("original server exchange did not confirm before actual stop")
			}
		}) {
			t.Fatal("failed protected exchange stops fixture reuse")
		}
		// t.Run has finished every cleanup: client and pinned launch handles are
		// closed before the next real process, not deferred until all64 complete.
	}
	matrixEvidence(t, f, "protected-session-restarts", rows)
}
