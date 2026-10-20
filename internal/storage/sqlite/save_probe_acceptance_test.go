//go:build windows && amd64 && aboard_acceptance

package sqlite

import "testing"

// Private epoch predicate policy only; not a physical-open or transaction proof.
func TestAcceptanceSaveEpochNeverArmsStaleOrEndedGate(t *testing.T) {
	g := &CommitGate{epoch: "original-private-epoch"}
	if !g.AcceptanceSaveEpoch(g.epoch) || g.AcceptanceSaveEpoch("") || g.AcceptanceSaveEpoch("different-epoch") {
		t.Fatal("stale/empty epoch armed checkpoint")
	}
	g.fenced = true
	if g.AcceptanceSaveEpoch(g.epoch) {
		t.Fatal("unknown gate armed checkpoint")
	}
	g.fenced = false
	g.closed = true
	if g.AcceptanceSaveEpoch(g.epoch) {
		t.Fatal("ended gate armed checkpoint")
	}
}
