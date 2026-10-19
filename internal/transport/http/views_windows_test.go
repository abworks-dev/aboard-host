//go:build windows && amd64

package httptransport

import (
	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"net/http/httptest"
	"testing"
)

func TestLateResponseAuthorizationKeepsOriginalOperationNamespace(t *testing.T) {
	key := "65000000-0000-4000-8000-000000000006"
	value := protocol.PrincipalView{InstallationId: "65000000-0000-4000-8000-000000000001", TenantId: "65000000-0000-4000-8000-000000000002", PrincipalId: "65000000-0000-4000-8000-000000000005", Capabilities: []protocol.Capability{}}
	for _, op := range []string{"createWorkspace", "registerWorkspace", "relinkWorkspace"} {
		t.Run(op, func(t *testing.T) {
			w := httptest.NewRecorder()
			h := &Handler{}
			h.write(w, 200, value, "fixture", op, key, true, func() error { return installation.ErrAuthorizationUnknown })
			requireStatus(t, w.Code, 503)
			var got protocol.ErrorView
			decodeResponse(t, w.Body.Bytes(), &got)
			if got.Code != protocol.ErrorCodeINSTALLATIONSTATEUNKNOWN || got.Scope == nil || *got.Scope != "installation" || got.OperationId == nil || string(*got.OperationId) != key {
				t.Fatal("late authorization failure lost original investigation namespace")
			}
		})
	}
}

func TestInstallationLogicalOutcomeNeverProvesPhysicalDrain(t *testing.T) {
	for _, outcome := range []sqlite.InstallationOutcome{sqlite.InstallationAccepted, sqlite.InstallationUnknown, sqlite.InstallationRejected, sqlite.InstallationCommitted} {
		t.Run(string(outcome), func(t *testing.T) {
			v := installationOperation(installation.InstallationOperationView{InstallationID: "65000000-0000-4000-8000-000000000001", OperationID: "65000000-0000-4000-8000-000000000002", WorkspaceID: "65000000-0000-4000-8000-000000000003", Kind: "create", Phase: sqlite.RegistrationConfirmed, Outcome: outcome}, protocol.CorrelationId("fixture"))
			if v.PhysicalState == "drained" {
				t.Fatal("persisted outcome was laundered into native cleanup evidence")
			}
			if outcome == sqlite.InstallationCommitted && (v.LogicalState != "committed" || v.PersistenceState != "known") {
				t.Fatal("known commit was relabeled unknown")
			}
			if outcome == sqlite.InstallationRejected && (v.LogicalState != "rejected" || v.PersistenceState != "known") {
				t.Fatal("known rejection was relabeled unknown")
			}
			_ = encode(t, v) // retain strict generated response validation
		})
	}
}
