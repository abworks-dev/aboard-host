//go:build windows && amd64

package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func openHistoryDescriptor(t *testing.T) workspace.DirectoryDescriptor {
	t.Helper()
	descriptor, err := workspace.NewDirectoryDescriptor(testWorkspace, testTenant)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func openHistoryExec(t *testing.T, d *Database, statement string, args ...any) {
	t.Helper()
	if _, err := d.h.Conn.ExecContext(context.Background(), statement, args...); err != nil {
		t.Fatal(err)
	}
}

func openHistoryCommitted(t *testing.T) (*Database, *CommitGate, Candidate) {
	t.Helper()
	d, g := newTestGate(t)
	t.Cleanup(func() {
		if err := g.EndPhysical(); err != nil {
			t.Error(err)
		}
	})
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	a := prepareAdmission(t, g, c, r)
	if result, err := g.CommitAccepted(context.Background(), a); err != nil || result.Status != Confirmed {
		t.Fatalf("commit %+v %v", result, err)
	}
	return d, g, c
}

func TestOpenHistoryGenesisAndDescriptor(t *testing.T) {
	d := newStorageDB(t, WorkspaceDatabase)
	defer closeStorage(t, d)
	descriptor := openHistoryDescriptor(t)
	if pending, err := d.ValidateOpenHistory(context.Background(), descriptor); err != nil || pending {
		t.Fatalf("genesis pending=%v err=%v", pending, err)
	}
	other, err := workspace.NewDirectoryDescriptor(testResource, testTenant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ValidateOpenHistory(context.Background(), other); !errors.Is(err, ErrSchema) {
		t.Fatalf("descriptor mismatch: %v", err)
	}
	openHistoryExec(t, d, `UPDATE workspace_identity SET business_state='unavailable'`)
	if _, err := d.ValidateOpenHistory(context.Background(), descriptor); !errors.Is(err, ErrSchema) {
		t.Fatalf("unavailable: %v", err)
	}
}

func TestOpenHistoryPendingIsReadOnlyAndNotReplay(t *testing.T) {
	for _, state := range []string{"accepted", "unknown"} {
		t.Run(state, func(t *testing.T) {
			d, g := newTestGate(t)
			defer g.EndPhysical()
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			prepareAdmission(t, g, c, r)
			if state == "unknown" {
				openHistoryExec(t, d, `UPDATE save_operation SET state='unknown'`)
			}
			before := scalar(t, d, `SELECT total_changes()`)
			for i := 0; i < 2; i++ {
				pending, err := d.ValidateOpenHistory(context.Background(), openHistoryDescriptor(t))
				if err != nil || !pending {
					t.Fatalf("pending=%v err=%v", pending, err)
				}
			}
			if scalar(t, d, `SELECT total_changes()`) != before || scalar(t, d, `SELECT revision FROM workspace_head`) != 0 || scalar(t, d, `SELECT count(*) FROM text_version`) != 0 {
				t.Fatal("validation mutated or replayed admission")
			}
			var got string
			if err := d.h.Conn.QueryRowContext(context.Background(), `SELECT state FROM save_operation`).Scan(&got); err != nil || got != state {
				t.Fatalf("state=%s err=%v", got, err)
			}
		})
	}
}

func TestOpenHistoryValidCreateUpdateAndOldVersionDigest(t *testing.T) {
	d, g, c := openHistoryCommitted(t)
	base := c.ContentDigest
	update, r := candidateFixture(t, g, testWorkspace, testResource, "update-text", 1, &base)
	r.OperationID = testTenant
	update.Body = "synthetic second version"
	var err error
	update.ContentDigest, update.CandidateDigest, err = candidateHashes(update)
	if err != nil {
		t.Fatal(err)
	}
	r.CandidateDigest = update.CandidateDigest
	a := prepareAdmission(t, g, update, r)
	if result, err := g.CommitAccepted(context.Background(), a); err != nil || result.Status != Confirmed {
		t.Fatalf("update %+v %v", result, err)
	}
	if pending, err := d.ValidateOpenHistory(context.Background(), openHistoryDescriptor(t)); err != nil || pending {
		t.Fatalf("valid history %v %v", pending, err)
	}
	// Synthetic corruption bypasses the immutable guard, then restores the exact
	// compiled DDL: rejection must be about old bytes, not a missing trigger.
	var ddl string
	if err := d.h.Conn.QueryRowContext(context.Background(), `SELECT sql FROM sqlite_schema WHERE name='immutable_text_version_update'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	openHistoryExec(t, d, `DROP TRIGGER immutable_text_version_update`)
	openHistoryExec(t, d, `UPDATE text_version SET body='tampered old version' WHERE version=1`)
	openHistoryExec(t, d, ddl)
	if _, err := d.ValidateOpenHistory(context.Background(), openHistoryDescriptor(t)); !errors.Is(err, ErrSchema) {
		t.Fatalf("old content tampering: %v", err)
	}
}

func TestOpenHistoryRejectsContradictoryFacts(t *testing.T) {
	cases := map[string]string{
		"head":                `UPDATE workspace_head SET revision=2`,
		"pointer":             `UPDATE text_record SET current_version=2`,
		"candidate bytes":     `UPDATE candidate SET body='different'`,
		"candidate baseline":  `UPDATE candidate SET base_revision=1`,
		"candidate hash":      `UPDATE candidate SET candidate_digest=printf('%064d',0)`,
		"candidate applied":   `UPDATE candidate SET state='previewed'`,
		"operation state":     `UPDATE save_operation SET state='unknown'`,
		"operation failure":   `UPDATE save_operation SET failure_code='contradictory'`,
		"operation epoch":     `UPDATE save_operation SET admission_epoch='different'`,
		"audit actor":         `UPDATE audit_entry SET principal_id='different'`,
		"audit action":        `UPDATE audit_entry SET action='other'`,
		"audit contradiction": `INSERT INTO audit_entry SELECT workspace_id,2,principal_id,operation_id,'save','rejected',revision FROM audit_entry`,
		"ledger":              `UPDATE migration_ledger SET checksum=printf('%064d',0)`,
		"missing audit":       `DELETE FROM audit_entry`,
	}
	for name, statement := range cases {
		t.Run(name, func(t *testing.T) {
			d, _, _ := openHistoryCommitted(t)
			// Disable FK enforcement only while creating this synthetic damaged sample.
			openHistoryExec(t, d, `PRAGMA foreign_keys=OFF`)
			openHistoryExec(t, d, statement)
			openHistoryExec(t, d, `PRAGMA foreign_keys=ON`)
			before := scalar(t, d, `SELECT total_changes()`)
			if pending, err := d.ValidateOpenHistory(context.Background(), openHistoryDescriptor(t)); !errors.Is(err, ErrSchema) || pending {
				t.Fatalf("pending=%v err=%v", pending, err)
			}
			if scalar(t, d, `SELECT total_changes()`) != before {
				t.Fatal("invalid history was repaired")
			}
		})
	}
}

func TestOpenHistoryIgnoresUnconfirmedBadPreview(t *testing.T) {
	d, g, _ := openHistoryCommitted(t)
	c, _ := candidateFixture(t, g, testWorkspace, testTenant, "create-text", 1, nil)
	if err := g.PutCandidate(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	openHistoryExec(t, d, `UPDATE candidate SET body='bad unconfirmed bytes' WHERE state='previewed'`)
	if pending, err := d.ValidateOpenHistory(context.Background(), openHistoryDescriptor(t)); err != nil || pending {
		t.Fatalf("unconfirmed candidate poisoned history %v %v", pending, err)
	}
}

func TestOpenHistoryPendingMayShareAppliedCandidate(t *testing.T) {
	d, g := newTestGate(t)
	defer g.EndPhysical()
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	a := prepareAdmission(t, g, c, r)
	second := r
	second.OperationID = testTenant
	if result, ticket, err := g.Admit(context.Background(), second, c.ExpiresAt.Add(-1)); err != nil || ticket == nil || result.Status != Accepted {
		t.Fatalf("second admission %+v %v", result, err)
	}
	if result, err := g.CommitAccepted(context.Background(), a); err != nil || result.Status != Confirmed {
		t.Fatalf("commit %+v %v", result, err)
	}
	if pending, err := d.ValidateOpenHistory(context.Background(), openHistoryDescriptor(t)); err != nil || !pending {
		t.Fatalf("valid interrupted competing admission %v %v", pending, err)
	}
}
