//go:build windows && amd64 && aboard_acceptance

package sqlite

// AcceptanceSaveEpoch only checks the retained gate's private epoch; it cannot
// reveal it, mint a capability, read receipt state or cause persistence IO.
func (g *CommitGate) AcceptanceSaveEpoch(epoch string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.closed && !g.fenced && epoch != "" && epoch == g.epoch
}
