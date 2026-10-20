//go:build !windows || !amd64 || !aboard_acceptance

package sqlite

import "database/sql"

// No instrumentation, environment, additional IO, retry or result substitution.
func commitAcceptedTransaction(_ *CommitGate, tx *sql.Tx) error { return tx.Commit() }
