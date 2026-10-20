//go:build windows && amd64 && aboard_acceptance

package sqlite

import (
	"context"
	"errors"
	"testing"
)

func TestAcceptanceBusySaveOriginalCommit(t *testing.T) {
	for _, cancelCaller := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "cancel-reader-parent"}[cancelCaller], func(t *testing.T) {
			d, g := newTestGate(t)
			defer g.EndPhysical()
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			a := prepareAdmission(t, g, c, r)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			result, ev, err := g.AcceptanceSaveBusy(ctx, a, func() (SaveResult, error) {
				calls++
				if cancelCaller {
					cancel()
				}
				return g.CommitAccepted(context.Background(), a)
			})
			var coded interface{ Code() int }
			if calls != 1 || result.Status != Unknown || result.Receipt != nil || !errors.Is(err, ErrUnknown) || !errors.As(err, &coded) || coded.Code() != 5 || ev.SQLiteCode != 5 || !ev.ReadOnlyURI || ev.QueryOnly != 1 || !ev.ReaderStarted || !ev.SnapshotHeld || !ev.OriginalReturned || !ev.ReaderEnded || ev.ReaderHead != 0 || ev.CommitCalls != 1 || !ev.CommitReturned || ev.CommitCode != 5 || ev.InputDigest != a.input {
				t.Fatalf("real original COMMIT busy missing: result=%+v evidence=%+v err=%v", result, ev, err)
			}
			if _, ok := acceptanceBusyCommits.Load(g); ok {
				t.Fatal("observer not joined/disarmed")
			}
			if g.AcceptanceSaveEpoch(g.epoch) {
				t.Fatal("reader end unfenced original unknown")
			}
			assertFormalState(t, d, 0) // Driver rollback observation only, not physical-end authority.
		})
	}
}
func TestAcceptanceBusyReaderNativeReadOnly(t *testing.T) {
	d, g := newTestGate(t)
	defer g.EndPhysical()
	reader := &acceptanceBusyReader{}
	defer func() {
		if e := reader.close(); e != nil {
			t.Error(e)
		}
	}()
	if _, e := reader.open(context.Background(), d.path, testWorkspace); e != nil {
		t.Fatal(e)
	}
	if _, e := reader.conn.ExecContext(context.Background(), "PRAGMA query_only=OFF"); e != nil {
		t.Fatal(e)
	}
	// Synthetic negative proves URI mode=ro remains enforced independent of the
	// query_only guard. This write must never take effect.
	_, e := reader.tx.Exec("UPDATE workspace_head SET revision=99 WHERE workspace_id=?", testWorkspace)
	var coded interface{ Code() int }
	if !errors.As(e, &coded) || coded.Code() != 8 {
		t.Fatalf("native readonly not enforced: %v", e)
	}
	if n := scalar(t, d, "SELECT revision FROM workspace_head"); n != 0 {
		t.Fatal("readonly negative changed formal state")
	}
}
func TestAcceptanceBusySavePanicRetainsOwner(t *testing.T) {
	for _, mode := range []string{"apply", "setup"} {
		t.Run(mode, func(t *testing.T) {
			d, g := newTestGate(t)
			defer g.EndPhysical()
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			a := prepareAdmission(t, g, c, r)
			if mode == "setup" {
				g.db = nil
			}
			result, ev, err := g.AcceptanceSaveBusy(context.Background(), a, func() (SaveResult, error) { panic(nil) })
			g.db = d
			if result.Status != Unknown || !errors.Is(err, ErrUnknown) || !g.fenced || ev.OriginalReturned || !ev.ReaderEnded {
				t.Fatal("panic lost busy reader/fence")
			}
			if _, ok := acceptanceBusyCommits.Load(g); ok {
				t.Fatal("panic left observation armed")
			}
			if g.AcceptanceSaveEpoch(g.epoch) {
				t.Fatal("panic re-enabled gate")
			}
		})
	}
}

// Policy-only duplicate-end guard, NOT a fabricated native Close failure.
func TestAcceptanceBusyReaderFirstEndFailureIsSticky(t *testing.T) {
	d, g := newTestGate(t)
	defer g.EndPhysical()
	reader := &acceptanceBusyReader{}
	if _, e := reader.open(context.Background(), d.path, testWorkspace); e != nil {
		t.Fatal(e)
	}
	// This test itself proves the transaction ended, then deliberately violates
	// the closer's ownership contract to exercise its conservative sticky branch.
	if e := reader.tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	first := reader.close()
	second := reader.close()
	if !errors.Is(first, ErrPhysicalCloseUnknown) || !errors.Is(second, ErrPhysicalCloseUnknown) || !reader.attempted {
		t.Fatal("uncertain first end was laundered")
	}
	if e := reader.conn.PingContext(context.Background()); e != nil {
		t.Fatal("uncertain child released ancestor")
	}
	// Only test-owned, already-ended transaction: end its retained ancestors.
	if e := reader.conn.Close(); e != nil {
		t.Fatal(e)
	}
	if e := reader.pool.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestAcceptanceBusySaveRejectsForeignCapability(t *testing.T) {
	_, g := newTestGate(t)
	defer g.EndPhysical()
	calls := 0
	result, ev, e := g.AcceptanceSaveBusy(context.Background(), &Admission{}, func() (SaveResult, error) { calls++; return SaveResult{Status: Confirmed}, nil })
	if e == nil || result.Status != Unknown || calls != 0 || ev.ReaderStarted || !ev.ReaderEnded {
		t.Fatal("foreign capability opened reader or executed apply")
	}
}
