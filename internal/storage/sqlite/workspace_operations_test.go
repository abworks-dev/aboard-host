package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestSaveOperationQueryAndProvenFreshResolution(t *testing.T) {
	for _, mode := range []string{"committed", "accepted", "no-admission"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			d, g := newTestGate(t)
			scope := workspace.Scope{InstallationID: testInstallation, TenantID: testTenant, WorkspaceID: testWorkspace}
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			if mode == "no-admission" {
				if err := g.PutCandidate(ctx, c); err != nil {
					t.Fatal(err)
				}
			} else {
				admission := prepareAdmission(t, g, c, r)
				if mode == "committed" {
					if result, err := g.CommitAccepted(ctx, admission); err != nil || result.Status != Confirmed {
						t.Fatal(result, err)
					}
				}
			}
			result, err := g.QuerySaveOperation(ctx, scope, r.OperationID, testPrincipal, false)
			if mode == "committed" && (err != nil || result.Status != Confirmed || result.Receipt == nil) {
				t.Fatal(result, err)
			}
			if mode == "accepted" && (err != nil || result.Status != Unknown || result.Receipt != nil) {
				t.Fatal("current absent receipt became rollback", result, err)
			}
			if mode == "no-admission" && (!errors.Is(err, workspace.ErrOperationHistoryUnavailable) || result.Status != Unknown) {
				t.Fatal(result, err)
			}
			if mode != "no-admission" {
				if other, err := g.QuerySaveOperation(ctx, scope, r.OperationID, testTenant, false); !errors.Is(err, workspace.ErrOperationAccess) || other.Receipt != nil {
					t.Fatal("foreign actor learned receipt", other, err)
				}
				if _, err := g.QuerySaveOperation(ctx, scope, r.OperationID, testTenant, true); err != nil {
					t.Fatal("explicit operator denied", err)
				}
				foreign := scope
				foreign.InstallationID = testTenant
				if got, err := g.QuerySaveOperation(ctx, foreign, r.OperationID, testPrincipal, false); err == nil || got.Receipt != nil {
					t.Fatal("SAVE/1 installation not validated", got, err)
				}
			}
			proof := sameSyntheticHistory(t, d.path)
			if err := g.EndPhysical(); err != nil {
				t.Fatal(err)
			}
			fresh, err := OpenDatabase(ctx, d.path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := fresh.Close(); err != nil {
					t.Error(err)
				}
			}()
			if got, _, err := fresh.ReconcileSaveOperation(ctx, scope, r.OperationID, testPrincipal, nil); !errors.Is(err, ErrFenced) || got.Status != Unknown {
				t.Fatal("missing physical evidence accepted", got, err)
			}
			denied := errors.New("synthetic physical history proof unavailable")
			if got, _, err := fresh.ReconcileSaveOperation(ctx, scope, r.OperationID, testPrincipal, func() error { return denied }); !errors.Is(err, denied) || got.Status != Unknown {
				t.Fatal(got, err)
			}
			got, pending, err := fresh.ReconcileSaveOperation(ctx, scope, r.OperationID, testPrincipal, proof)
			switch mode {
			case "no-admission":
				if !errors.Is(err, workspace.ErrOperationHistoryUnavailable) || got.Status != Unknown || !pending {
					t.Fatal("missing admission terminalized", got, pending, err)
				}
			case "committed":
				if err != nil || got.Status != Confirmed || got.Receipt == nil || pending {
					t.Fatal(got, pending, err)
				}
			case "accepted":
				if err != nil || got.Status != NotCommitted || got.FailureCode != "NOT_COMMITTED" || pending {
					t.Fatal(got, pending, err)
				}
				query, err := fresh.QuerySaveOperation(ctx, scope, r.OperationID, testPrincipal, false)
				if err != nil || query.Status != NotCommitted {
					t.Fatal("positive non-commit evidence not persisted", query, err)
				}
				next, err := NewCommitGate(fresh, "new-explicit-epoch")
				if err != nil {
					t.Fatal(err)
				}
				same, ticket, err := next.Admit(ctx, r, time.Now().Add(24*time.Hour))
				if err != nil || same.Status != NotCommitted || ticket != nil {
					t.Fatal("original key replayed after resolution", same, ticket, err)
				}
			}
			want := int64(0)
			if mode == "committed" {
				want = 1
			}
			for _, table := range []string{"text_record", "text_version", "commit_receipt", "audit_entry"} {
				if got := scalar(t, fresh, "SELECT count(*) FROM "+table); got != want {
					t.Fatal("reconcile changed formal facts", table, got, want)
				}
			}
		})
	}
}
