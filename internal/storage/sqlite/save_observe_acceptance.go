//go:build windows && amd64 && aboard_acceptance

package sqlite

import "errors"

// Read-only evidence for the SAME original COMMIT, never a receipt or capability.
// Snapshot does not acquire the gate mutex (which the native caller already owns).
type AcceptanceCommitObservation struct{ state *acceptanceBusyCommitState }
type AcceptanceCommitSnapshot struct {
	Calls, Code        int
	InFlight, Returned bool
}

func (o *AcceptanceCommitObservation) Snapshot() AcceptanceCommitSnapshot {
	if o == nil || o.state == nil {
		return AcceptanceCommitSnapshot{}
	}
	s := o.state
	s.mu.Lock()
	defer s.mu.Unlock()
	return AcceptanceCommitSnapshot{s.calls, s.code, s.inFlight.Load(), s.returned}
}

var errAcceptanceObserve = errors.New("acceptance original commit observation unavailable")

// Validates the original unconsumed admission under its own gate, then releases
// the gate lock BEFORE the one original apply. No SQL, admission or retry here.
func (g *CommitGate) AcceptanceObserveCommit(a *Admission, request SaveRequest, apply func(*AcceptanceCommitObservation) (SaveResult, error)) (result SaveResult, err error) {
	result.Status = Unknown
	if g == nil {
		return result, errAcceptanceObserve
	}
	g.mu.Lock()
	if g.closed || g.fenced || a == nil || a.gate != g || a.used || a.request != request || request.Epoch != g.epoch || apply == nil {
		g.mu.Unlock()
		return result, errors.Join(ErrUnknown, errAcceptanceObserve)
	}
	state := &acceptanceBusyCommitState{}
	if _, exists := acceptanceBusyCommits.LoadOrStore(g, state); exists {
		g.mu.Unlock()
		return result, errors.Join(ErrUnknown, errAcceptanceObserve)
	}
	g.mu.Unlock()
	completed := false
	defer func() {
		if !completed {
			_ = recover() // legacy panic(nil) is a missing original return, not a success
			result = SaveResult{Status: Unknown}
			err = errors.Join(err, ErrUnknown, errAcceptanceObserve)
		}
		removed := acceptanceBusyCommits.CompareAndDelete(g, state)
		if !completed || !removed {
			g.mu.Lock()
			g.fenced = true
			g.mu.Unlock()
			if result.Status != Confirmed {
				result = SaveResult{Status: Unknown}
			}
			err = errors.Join(err, ErrUnknown, errAcceptanceObserve)
		}
	}()
	result, err = apply(&AcceptanceCommitObservation{state})
	completed = true
	return
}
