//go:build windows && amd64

package sqlite

import (
	"context"
	"strings"
	"testing"
)

func TestInstallationShutdownPredicateDoesNotHideMissingIntent(t *testing.T) {
	for _, outcome := range []string{"accepted", "unknown", "rejected", "committed"} {
		t.Run(outcome, func(t *testing.T) {
			d, _ := authStorageFixture(t)
			ctx := context.Background()
			if pending, err := d.HasUnresolvedInstallationOperations(ctx, testInstallation); err != nil || pending {
				t.Fatal("empty history", pending, err)
			}
			// Deliberately no kind-specific intent row: the shutdown predicate must not
			// join away an unresolved original key. This is a real SQL read-unit fixture,
			// not evidence that incomplete committed business history is valid.
			phase := "intentRecorded"
			if outcome == "committed" {
				phase = "registrationConfirmed"
			}
			_, err := d.h.Conn.ExecContext(ctx, `INSERT INTO installation_operation VALUES(?,?,?,?,?,?,?,?,?)`, testInstallation, testOperation, testPrincipal, testCandidate, "register", `E:\synthetic\pending`, strings.Repeat("a", 64), phase, outcome)
			if err != nil {
				t.Fatal(err)
			}
			pending, err := d.HasUnresolvedInstallationOperations(ctx, testInstallation)
			if err != nil || pending != (outcome == "accepted" || outcome == "unknown") {
				t.Fatal("original unresolved key hidden", pending, err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if _, err = d.HasUnresolvedInstallationOperations(canceled, testInstallation); err == nil {
				t.Fatal("canceled read claimed readiness")
			}
		})
	}
}
