package sqlite

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	testWorkspace    = "11111111-1111-4111-8111-111111111111"
	testTenant       = "22222222-2222-4222-8222-222222222222"
	testPrincipal    = "33333333-3333-4333-8333-333333333333"
	testInstallation = "44444444-4444-4444-8444-444444444444"
	testCandidate    = "55555555-5555-4555-8555-555555555555"
	testResource     = "66666666-6666-4666-8666-666666666666"
	testOperation    = "77777777-7777-4777-8777-777777777777"
)

func testGenesis(kind DatabaseKind) Genesis {
	return Genesis{Kind: kind, WorkspaceID: testWorkspace, TenantID: testTenant, InstallationID: testInstallation, OwnerPrincipalID: testPrincipal, WindowsSID: "S-1-5-21-synthetic", SecretReference: "synthetic-reference-only", DisplayName: "合成 workspace"}
}
func newStorageDB(t *testing.T, kind DatabaseKind) *Database {
	t.Helper()
	d, err := OpenDatabase(context.Background(), filepath.Join(t.TempDir(), "synthetic.sqlite"), CreateNew, testGenesis(kind))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func scalar(t *testing.T, d *Database, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := d.h.Conn.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func closeStorage(t *testing.T, d *Database) {
	t.Helper()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationGenesisAndOwnership(t *testing.T) {
	for _, kind := range []DatabaseKind{InstallationDatabase, WorkspaceDatabase} {
		t.Run(string(kind), func(t *testing.T) {
			d := newStorageDB(t, kind)
			path := d.path
			if n := scalar(t, d, "PRAGMA user_version"); n != 1 {
				t.Fatal(n)
			}
			if n := scalar(t, d, "SELECT count(*) FROM migration_ledger WHERE sequence=1 AND version=1 AND state='applied'"); n != 1 {
				t.Fatal(n)
			}
			if kind == WorkspaceDatabase {
				if scalar(t, d, "SELECT revision FROM workspace_head") != 0 || scalar(t, d, "SELECT count(*) FROM text_record") != 0 || scalar(t, d, "SELECT count(*) FROM commit_receipt") != 0 {
					t.Fatal("nonempty genesis")
				}
				if scalar(t, d, "SELECT count(*) FROM sqlite_schema WHERE name IN ('principal','grant_entry','client_session','directory_registration','installation_operation')") != 0 {
					t.Fatal("installation state in workspace")
				}
			} else {
				if scalar(t, d, "SELECT count(*) FROM installation_identity") != 1 || scalar(t, d, "SELECT count(*) FROM principal WHERE grant_version=1 AND revoked=0") != 1 {
					t.Fatal("incomplete installation genesis")
				}
				for _, table := range []string{"directory_registration", "grant_entry", "client_session", "installation_operation"} {
					if scalar(t, d, "SELECT count(*) FROM "+table) != 0 {
						t.Fatal(table)
					}
				}
				if scalar(t, d, "SELECT count(*) FROM sqlite_schema WHERE name IN ('candidate','text_record','text_version','workspace_head','commit_receipt','audit_entry')") != 0 {
					t.Fatal("workspace business in installation")
				}
				if _, err := d.h.Conn.ExecContext(context.Background(), "INSERT INTO principal VALUES(?,?,1,0)", testWorkspace, testWorkspace); err == nil {
					t.Fatal("cross-tenant principal")
				}
				registration := "INSERT INTO directory_registration VALUES(?,?,?,'synthetic-approved-parent','synthetic-volume',zeroblob(16),1,'registered')"
				if _, err := d.h.Conn.ExecContext(context.Background(), registration, testWorkspace, testWorkspace, "synthetic-root"); err == nil {
					t.Fatal("cross-tenant directory")
				}
				if _, err := d.h.Conn.ExecContext(context.Background(), registration, testWorkspace, testTenant, "synthetic-root"); err != nil {
					t.Fatal(err)
				}
				if _, err := d.h.Conn.ExecContext(context.Background(), "INSERT INTO grant_entry VALUES(?,?,?,'save')", testPrincipal, testWorkspace, testWorkspace); err == nil {
					t.Fatal("cross-tenant grant")
				}
				if _, err := d.h.Conn.ExecContext(context.Background(), "INSERT INTO grant_entry VALUES(?,?,?,'save')", testPrincipal, testTenant, testWorkspace); err != nil {
					t.Fatal(err)
				}
				if _, err := d.h.Conn.ExecContext(context.Background(), "INSERT INTO client_session VALUES(?,?,?,'2099-01-01T00:00:00Z',0,1)", testOperation, strings.Repeat("b", 64), testResource); err == nil {
					t.Fatal("orphan session")
				}
				if _, err := d.h.Conn.ExecContext(context.Background(), "INSERT INTO installation_operation VALUES(?,?,?,?,'register','synthetic-root',?,'intentRecorded','accepted')", testWorkspace, testOperation, testPrincipal, testWorkspace, strings.Repeat("a", 64)); err == nil {
					t.Fatal("cross-installation operation")
				}

			}
			closeStorage(t, d)
			// Supplying different seed values on existing-open must not overwrite IDs.
			g := testGenesis(kind)
			g.WorkspaceID = testOperation
			g.TenantID = testOperation
			reopened, err := OpenDatabase(context.Background(), path, ReadWriteExisting, g)
			if err != nil {
				t.Fatal(err)
			}
			closeStorage(t, reopened)
			frozen, err := OpenDatabase(context.Background(), path, ReadOnlyScratch, Genesis{Kind: kind})
			if err != nil {
				t.Fatal(err)
			}
			closeStorage(t, frozen)
		})
	}
}

func TestMigrationRejectsWithoutRepair(t *testing.T) {
	mutations := []struct{ name, sql string }{
		{"missing-ledger-row", "DELETE FROM migration_ledger"},
		{"out-of-order", "UPDATE migration_ledger SET sequence=2"},
		{"old-ledger", "PRAGMA ignore_check_constraints=ON; UPDATE migration_ledger SET version=0"},
		{"future-ledger", "UPDATE migration_ledger SET version=2"},
		{"extra-ledger", "INSERT INTO migration_ledger VALUES(2,2,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','applied')"},
		{"checksum", "UPDATE migration_ledger SET checksum='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'"},
		{"state", "PRAGMA ignore_check_constraints=ON; UPDATE migration_ledger SET state='started'"},
		{"user-version-old", "PRAGMA user_version=0"},
		{"user-version-future", "PRAGMA user_version=2"},
		{"metadata-missing", "DELETE FROM schema_metadata"},
		{"metadata-kind", "PRAGMA ignore_check_constraints=ON; UPDATE schema_metadata SET kind='alien'"},
		{"metadata-version", "PRAGMA ignore_check_constraints=ON; UPDATE schema_metadata SET version=2"},
		{"schema-missing", "DROP TABLE audit_entry"},
		{"schema-extra", "CREATE TABLE alien(id INTEGER)"},
		{"head-missing", "DELETE FROM workspace_head"},
		{"head-not-genesis", "UPDATE workspace_head SET revision=1"},
		{"identity-digest", "UPDATE workspace_identity SET identity_digest='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'"},
	}
	for _, kind := range []DatabaseKind{WorkspaceDatabase, InstallationDatabase} {
		for _, m := range mutations {
			if kind == InstallationDatabase && (strings.HasPrefix(m.name, "head-") || m.name == "identity-digest") {
				continue
			}
			t.Run(string(kind)+"/"+m.name, func(t *testing.T) {
				d := newStorageDB(t, kind)
				path := d.path
				statement := m.sql
				if kind == InstallationDatabase && m.name == "schema-missing" {
					statement = "DROP TABLE client_session"
				}
				if _, err := d.h.Conn.ExecContext(context.Background(), statement); err != nil {
					t.Fatal(err)
				}
				closeStorage(t, d)
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				reopened, err := OpenDatabase(context.Background(), path, ReadWriteExisting, testGenesis(kind))
				if reopened != nil {
					reopened.Close()
				}
				if !errors.Is(err, ErrSchema) {
					t.Fatalf("accepted %s: %v", m.name, err)
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("verify repaired/mutated %s: %v", m.name, err)
				}
				if _, err := OpenDatabase(context.Background(), path, CreateNew, testGenesis(kind)); !errors.Is(err, os.ErrExist) {
					t.Fatalf("new-only repaired %s: %v", m.name, err)
				}
			})
		}
	}
	for _, kind := range []DatabaseKind{WorkspaceDatabase, InstallationDatabase} {
		t.Run("empty-existing-"+string(kind), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic-empty.sqlite")
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenDatabase(context.Background(), path, CreateNew, testGenesis(kind)); !errors.Is(err, os.ErrExist) {
				t.Fatal(err)
			}
			if _, err := OpenDatabase(context.Background(), path, ReadWriteExisting, testGenesis(kind)); !errors.Is(err, ErrSchema) {
				t.Fatal(err)
			}
			b, err := os.ReadFile(path)
			if err != nil || len(b) != 0 {
				t.Fatalf("empty remnant seeded %d %v", len(b), err)
			}
		})
	}
}

// Abrupt physical termination at each initialization phase leaves an existing
// artifact that neither verify nor CreateNew is permitted to complete/reseed.
func TestMigrationInterruptedInitialization(t *testing.T) {
	for _, kind := range []DatabaseKind{WorkspaceDatabase, InstallationDatabase} {
		for stage := 0; stage <= 6; stage++ {
			t.Run(fmt.Sprintf("%s-stage-%d", kind, stage), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "synthetic-interrupted.sqlite")
				exe, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(exe, "-test.run=^TestMigrationInitChild$", "-test.v")
				cmd.Env = append(os.Environ(), "ABOARD_INIT_KIND="+string(kind), fmt.Sprintf("ABOARD_INIT_STAGE=%d", stage), "ABOARD_INIT_PATH="+path)
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				stdin, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				defer stdin.Close()
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				ended := false
				defer func() {
					if !ended {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
					}
				}()
				ready := make(chan bool, 1)
				go func() {
					s := bufio.NewScanner(stdout)
					for s.Scan() {
						if strings.Contains(s.Text(), "INIT_READY") {
							ready <- true
							return
						}
					}
					ready <- false
				}()
				select {
				case ok := <-ready:
					if !ok {
						t.Fatalf("child missing readiness %s", &stderr)
					}
				case <-time.After(30 * time.Second):
					t.Fatal("init child timeout")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				err = cmd.Wait()
				ended = true
				if err == nil {
					t.Fatal("expected abnormal end")
				}
				t.Logf("physical init termination kind=%s stage=%d pid=%d: %v", kind, stage, cmd.Process.Pid, err)
				if _, err := OpenDatabase(context.Background(), path, ReadWriteExisting, testGenesis(kind)); !errors.Is(err, ErrSchema) {
					t.Fatalf("remnant repaired/accepted: %v", err)
				}
				if _, err := OpenDatabase(context.Background(), path, CreateNew, testGenesis(kind)); !errors.Is(err, os.ErrExist) {
					t.Fatalf("remnant reseeded: %v", err)
				}
			})
		}
	}
}
func TestMigrationInitChild(t *testing.T) {
	path := os.Getenv("ABOARD_INIT_PATH")
	if path == "" {
		return
	}
	if !strings.Contains(strings.ToLower(filepath.ToSlash(path)), "/.data/logs/001-validation/tmp/") {
		t.Fatal("not synthetic runner root")
	}
	var stage int
	if _, err := fmt.Sscanf(os.Getenv("ABOARD_INIT_STAGE"), "%d", &stage); err != nil {
		t.Fatal(err)
	}
	kind := DatabaseKind(os.Getenv("ABOARD_INIT_KIND"))
	g := testGenesis(kind)
	h := openGate(t, path, CreateNew)
	hold := func() {
		fmt.Printf("INIT_READY kind=%s stage=%d\n", kind, stage)
		_, _ = bufio.NewReader(os.Stdin).ReadByte()
		t.Fatal("child must be terminated")
	}
	if stage == 0 {
		hold()
		return
	}
	execGate(t, h, "PRAGMA cache_size=2")
	tx, err := h.Conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	steps, err := migrations(kind)
	if err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		s string
		a []any
	}{
		{steps[0].sql, nil},
		{"INSERT INTO migration_ledger VALUES(1,1,?,'applied')", []any{steps[0].checksum}},
		{"INSERT INTO schema_metadata VALUES(1,?,1)", []any{string(kind)}},
		{"PRAGMA user_version=1", nil},
	}
	if kind == WorkspaceDatabase {
		statements = append(statements, struct {
			s string
			a []any
		}{"INSERT INTO workspace_identity VALUES(1,?,?,2,1,?,?,'ready')", []any{g.WorkspaceID, g.TenantID, strings.Repeat("a", 64), g.DisplayName}}, struct {
			s string
			a []any
		}{"INSERT INTO workspace_head VALUES(?,0)", []any{g.WorkspaceID}})
	} else {
		statements = append(statements, struct {
			s string
			a []any
		}{"INSERT INTO installation_identity VALUES(1,?,?,?,?,1,?)", []any{g.InstallationID, g.TenantID, g.OwnerPrincipalID, g.WindowsSID, g.SecretReference}}, struct {
			s string
			a []any
		}{"INSERT INTO principal VALUES(?,?,1,0)", []any{g.OwnerPrincipalID, g.TenantID}})
	}
	for i, s := range statements {
		if _, err := tx.ExecContext(context.Background(), s.s, s.a...); err != nil {
			t.Fatal(err)
		}
		if stage == i+1 {
			hold()
			return
		}
	}
	t.Fatal("invalid child stage")
}
