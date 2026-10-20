package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestSaveOperationIDsIncludeEveryDurableState(t *testing.T) {
	for _, state := range []string{"accepted", "unknown", "rejected", "committed"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			d, g := newTestGate(t)
			defer func() {
				if err := g.EndPhysical(); err != nil {
					t.Error(err)
				}
			}()
			scope := workspace.Scope{InstallationID: testInstallation, TenantID: testTenant, WorkspaceID: testWorkspace}
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			a := prepareAdmission(t, g, c, r)
			switch state {
			case "committed":
				if result, err := g.CommitAccepted(ctx, a); err != nil || result.Status != Confirmed {
					t.Fatal(result, err)
				}
			case "unknown", "rejected":
				// Raw synthetic states exercise the index predicate only, not a
				// claim of business rollback, recovery or physical completion.
				code := any(nil)
				if state == "rejected" {
					code = "NOT_COMMITTED"
				}
				if _, err := d.h.Conn.ExecContext(ctx, `UPDATE save_operation SET state=?,failure_code=? WHERE workspace_id=? AND operation_id=?`, state, code, testWorkspace, r.OperationID); err != nil {
					t.Fatal(err)
				}
			}
			ids, err := g.SaveOperationIDs(ctx, scope)
			if err != nil || len(ids) != 1 || ids[0] != r.OperationID {
				t.Fatal(state, ids, err)
			}
			foreign := scope
			foreign.WorkspaceID = testTenant
			if ids, err := g.SaveOperationIDs(ctx, foreign); !errors.Is(err, workspace.ErrScope) || len(ids) != 0 {
				t.Fatal(ids, err)
			}
			foreign = scope
			foreign.TenantID = testPrincipal
			if ids, err := g.SaveOperationIDs(ctx, foreign); !errors.Is(err, workspace.ErrScope) || len(ids) != 0 {
				t.Fatal(ids, err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if ids, err := g.SaveOperationIDs(canceled, scope); err == nil || len(ids) != 0 {
				t.Fatal("canceled index query accepted", ids, err)
			}
			if g.fenced {
				t.Fatal("canceled metadata query fenced the gate")
			}
			if ids, err := g.SaveOperationIDs(ctx, scope); err != nil || len(ids) != 1 {
				t.Fatal("read changed key history", ids, err)
			}
		})
	}
}
