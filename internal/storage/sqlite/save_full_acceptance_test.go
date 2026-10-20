//go:build windows && amd64 && aboard_acceptance

package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAcceptanceFullSaveUsesOriginalAdmittedPager(t *testing.T) {
	d, g := newTestGate(t)
	defer g.EndPhysical()
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	c.Body = strings.Repeat("X", 65536)
	var e error
	c.ContentDigest, c.CandidateDigest, e = candidateHashes(c)
	if e != nil {
		t.Fatal(e)
	}
	r.CandidateDigest = c.CandidateDigest
	a := prepareAdmission(t, g, c, r)
	calls := 0
	result, ev, err := g.AcceptanceSaveFull(context.Background(), a, func() (SaveResult, error) { calls++; return g.CommitAccepted(context.Background(), a) })
	var coded interface{ Code() int }
	if calls != 1 || !ev.Armed || !ev.OriginalReturned || !ev.Restored || ev.FreePages != 0 || ev.EffectiveCap != ev.PageCount || ev.RestoredCap != ev.OldCap || ev.SQLiteCode != 13 || !errors.As(err, &coded) || coded.Code() != 13 || result.Receipt != nil || (result.Status != Unknown && result.Status != Rejected) {
		t.Fatalf("original pager FULL evidence missing calls=%d status=%s evidence=%+v err=%v", calls, result.Status, ev, err)
	}
	assertFormalState(t, d, 0)
	// The accepted identity must not vanish even if FULL auto-rollback makes the
	// following explicit rollback conservatively unknown.
	var n int
	if d.h.Conn.QueryRowContext(context.Background(), "SELECT count(*) FROM save_operation WHERE operation_id=?", r.OperationID).Scan(&n) != nil || n != 1 {
		t.Fatal("durable admission lost")
	}
}

// Policy-only pointer/panic guards, not a simulated FULL or a public trigger.
func TestAcceptanceFullSavePanicRetainsFenceAndRestoresCap(t *testing.T) {
	for _, mode := range []string{"apply", "restore", "setup"} {
		t.Run(mode, func(t *testing.T) {
			d, g := newTestGate(t)
			defer g.EndPhysical()
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			a := prepareAdmission(t, g, c, r)
			if mode == "setup" {
				g.db = nil
			}
			result, ev, err := g.AcceptanceSaveFull(context.Background(), a, func() (SaveResult, error) {
				if mode == "restore" {
					g.db = nil
					return SaveResult{Status: Rejected}, nil
				}
				panic(nil)
			})
			g.db = d // Restore only test-owned pointer so original DB cleanup remains owned.
			if !errors.Is(err, ErrUnknown) || result.Status != Unknown || !g.fenced {
				t.Fatal("panic uncertainty lost")
			}
			if mode == "apply" && (!ev.Armed || !ev.Restored || ev.OriginalReturned) {
				t.Fatal("apply panic skipped cap restore")
			}
			if mode == "restore" && (!ev.Armed || ev.Restored || !ev.OriginalReturned) {
				t.Fatal("restore panic mislabeled completed")
			}
			if g.AcceptanceSaveEpoch(g.epoch) {
				t.Fatal("panic stranded mutex or re-enabled gate")
			}
		})
	}
}

func TestAcceptanceFullSaveRejectsNonCapabilities(t *testing.T) {
	_, g := newTestGate(t)
	defer g.EndPhysical()
	calls := 0
	for _, a := range []*Admission{nil, {}, {gate: g, used: true}, {gate: g, request: SaveRequest{Epoch: "foreign"}}} {
		result, ev, err := g.AcceptanceSaveFull(context.Background(), a, func() (SaveResult, error) { calls++; return SaveResult{Status: Confirmed}, nil })
		if err == nil || result.Status != Unknown || ev.Armed || calls != 0 {
			t.Fatal("non-original capability armed pager")
		}
	}
}
