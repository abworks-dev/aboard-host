//go:build windows && amd64 && aboard_acceptance

package sqlite

import (
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
)

// Scoped observation only; never a receipt/capability cache or a commit decision.
var acceptanceBusyCommits sync.Map

type acceptanceBusyCommitState struct {
	mu          sync.Mutex
	calls, code int
	returned    bool
	inFlight    atomic.Bool
}

func commitAcceptedTransaction(g *CommitGate, tx *sql.Tx) error {
	value, armed := acceptanceBusyCommits.Load(g)
	var state *acceptanceBusyCommitState
	if armed {
		state = value.(*acceptanceBusyCommitState)
		state.mu.Lock()
		state.calls++
		state.mu.Unlock()
		state.inFlight.Store(true)
		defer state.inFlight.Store(false) // includes panic(nil); no native callback may consult g.mu
	}
	err := tx.Commit() // EXACT original call, once, with the identical error returned.
	if state != nil {
		state.mu.Lock()
		state.returned = true
		var coded interface{ Code() int }
		if errors.As(err, &coded) {
			state.code = coded.Code()
		}
		state.mu.Unlock()
	}
	return err
}
