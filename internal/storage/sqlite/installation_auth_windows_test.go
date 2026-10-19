//go:build windows && amd64

package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func installationHash(label string) string {
	sum := sha256.Sum256([]byte(label))
	return hex.EncodeToString(sum[:])
}
func authStorageFixture(t *testing.T) (*Database, string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".data", "logs", "001-validation"))
	if err != nil {
		t.Fatal(err)
	}
	path, err := os.MkdirTemp(root, "t011-sql-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Error(err)
		}
	})
	d, err := OpenDatabase(context.Background(), filepath.Join(path, "installation.sqlite"), CreateNew, testGenesis(InstallationDatabase))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, w := range []string{testWorkspace, testResource} {
		if _, err = d.h.Conn.ExecContext(context.Background(), `INSERT INTO directory_registration VALUES(?,?,?,?,'synthetic-volume',zeroblob(16),1,'registered')`, w, testTenant, `E:\synthetic\`+w, `E:\synthetic`); err != nil {
			t.Fatal(err)
		}
	}
	hash := installationHash("synthetic-owner-session")
	pending, err := d.CreateInstallationSession(context.Background(), testPrincipal, testPrincipal, testCandidate, hash, time.Now().Add(time.Minute), "", "installation.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err = d.ActivateInstallationSession(context.Background(), testPrincipal, pending); err != nil {
		t.Fatal(err)
	}
	return d, hash
}
func TestInstallationAuthTypedAtomicPermissions(t *testing.T) {
	ctx := context.Background()
	d, owner := authStorageFixture(t)
	grants := []WorkspaceGrant{{testWorkspace, "workspace.read"}, {testResource, "workspace.write"}}
	if err := d.ReplacePrincipalAuthorization(ctx, owner, 1, testOperation, false, grants); err != nil {
		t.Fatal(err)
	}
	sessionHash := installationHash("synthetic-restricted-session")
	pending, err := d.CreateInstallationSession(ctx, testPrincipal, testOperation, testResource, sessionHash, time.Now().Add(time.Minute), testWorkspace, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.CurrentSessionAuthorization(ctx, sessionHash, testWorkspace, "workspace.read"); err == nil {
		t.Fatal("pending usable before ACK")
	}
	if err = d.ActivateInstallationSession(ctx, testPrincipal, pending); err != nil {
		t.Fatal(err)
	}
	a, err := d.CurrentSessionAuthorization(ctx, sessionHash, testWorkspace, "workspace.read")
	if err != nil || a.PrincipalID != testOperation || a.GrantVersion != 1 {
		t.Fatal("real session authority", err)
	}
	if _, err = d.CurrentSessionAuthorization(ctx, sessionHash, testResource, "workspace.read"); err == nil {
		t.Fatal("cross-zone action leaked")
	}
	// Replacing grants with an unregistered target must roll back version, revoked
	// and all old grants together, NOT fence or partially change current state.
	for _, bad := range [][]WorkspaceGrant{{{testCandidate, "workspace.read"}}, {{testWorkspace, "installation.operator"}}, {{testWorkspace, "workspace.read"}, {testWorkspace, "workspace.read"}}} {
		err = d.ReplacePrincipalAuthorization(ctx, owner, 1, testOperation, true, bad)
		if err == nil || errors.Is(err, ErrUnknown) {
			t.Fatal("invalid known mutation not rejected", err)
		}
		current, e := d.CurrentSessionAuthorization(ctx, sessionHash, testWorkspace, "workspace.read")
		if e != nil || current.GrantVersion != 1 {
			t.Fatal("known rollback changed authority", e)
		}
	}
	if err = d.ReplacePrincipalAuthorization(ctx, sessionHash, 1, testOperation, false, nil); err == nil {
		t.Fatal("nonowner managed authorization")
	}
	if _, err = d.CreateInstallationSession(ctx, testPrincipal, testOperation, testWorkspace, installationHash("forged operator"), time.Now().Add(time.Minute), "", "installation.operator"); err == nil {
		t.Fatal("action promoted nonowner")
	}
	if err = d.ReplacePrincipalAuthorization(ctx, owner, 1, testOperation, false, grants); err != nil {
		t.Fatal(err)
	}
	if _, err = d.CurrentSessionAuthorization(ctx, sessionHash, testWorkspace, "workspace.read"); err == nil {
		t.Fatal("old version accepted")
	}
	// Session rows contain only SHA-256 text; there is no raw token column/table.
	if scalar(t, d, `SELECT count(*) FROM pragma_table_info('client_session') WHERE name IN ('token','secret','bearer')`) != 0 {
		t.Fatal("raw bearer schema")
	}
	if err = d.RevokeInstallationSession(ctx, owner, 1, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = d.CurrentSessionAuthorization(ctx, owner, "", ""); err == nil {
		t.Fatal("revocation cached")
	}
}

func TestInstallationAuthCommitFlushUnknown(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"issue", "activate", "replace-grants", "revoke", "undelivered"} {
		t.Run(kind, func(t *testing.T) {
			flushCount := int32(0)
			for ordinal := int32(0); ordinal == 0 || ordinal <= flushCount; ordinal++ {
				t.Run(fmt.Sprintf("flush-%d", ordinal), func(t *testing.T) {
					d, owner := authStorageFixture(t)
					var pending SessionDelivery
					if kind == "activate" {
						var e error
						pending, e = d.CreateInstallationSession(ctx, testPrincipal, testPrincipal, testOperation, installationHash("synthetic-new-session"), time.Now().Add(time.Minute), "", "installation.operator")
						if e != nil {
							t.Fatal(e)
						}
					}
					stimulusPath := filepath.Join(filepath.Dir(d.path), "readonly-stimulus")
					if err := os.WriteFile(stimulusPath, []byte("synthetic readonly"), 0600); err != nil {
						t.Fatal(err)
					}
					stimulus, err := os.Open(stimulusPath)
					if err != nil {
						t.Fatal(err)
					}
					defer stimulus.Close()
					if err = windows.FlushFileBuffers(windows.Handle(stimulus.Fd())); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
						t.Fatal("native stimulus unavailable", err)
					}
					flushFault.handle = stimulus.Fd()
					flushFault.at = ordinal
					flushFault.calls.Store(0)
					flushFault.failed.Store(0)
					restore := replaceFlushForGate(t)
					switch kind {
					case "issue":
						_, err = d.CreateInstallationSession(ctx, testPrincipal, testPrincipal, testOperation, installationHash("synthetic-new-session"), time.Now().Add(time.Minute), "", "installation.operator")
					case "activate":
						err = d.ActivateInstallationSession(ctx, testPrincipal, pending)
					case "replace-grants":
						err = d.ReplacePrincipalAuthorization(ctx, owner, 1, testOperation, false, []WorkspaceGrant{{testWorkspace, "workspace.read"}})
					case "revoke":
						err = d.RevokeInstallationSession(ctx, owner, 1, owner)
					case "undelivered":
						err = d.RevokeUndeliveredSession(ctx, owner)
					}
					calls, failed := flushFault.calls.Load(), flushFault.failed.Load()
					restore()
					if ordinal == 0 {
						if err != nil || calls < 1 || calls > 8 {
							t.Fatal("successful real flush baseline absent", err, calls)
						}
						flushCount = calls
					} else {
						var driverErr *driver.Error
						if !errors.Is(err, ErrUnknown) || !errors.As(err, &driverErr) || driverErr.Code() != sqlite3.SQLITE_IOERR_FSYNC || failed != 1 {
							t.Fatal("unknown/error propagation lost", err, failed)
						}
					}
					t.Logf("operation=%s ordinal=%d native_failures=%d flush_calls=%d unknown=%t", kind, ordinal, failed, calls, errors.Is(err, ErrUnknown))
					path := d.path
					if err = d.Close(); err != nil {
						t.Fatal(err)
					}
					fresh, err := OpenDatabase(ctx, path, ReadWriteExisting, Genesis{Kind: InstallationDatabase})
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Close()
					// Fresh physical view only: no retry. Fault outcomes are observed, never
					// assumed safe from the driver COMMIT error or missing record alone.
					sessions := scalar(t, fresh, `SELECT count(*) FROM client_session`)
					principals := scalar(t, fresh, `SELECT count(*) FROM principal`)
					revoked := scalar(t, fresh, `SELECT revoked FROM client_session WHERE token_hash=?`, owner)
					if scalar(t, fresh, `SELECT count(*) FROM installation_identity WHERE installation_id=? AND tenant_id=? AND owner_principal_id=?`, testInstallation, testTenant, testPrincipal) != 1 || scalar(t, fresh, `SELECT count(*) FROM principal WHERE principal_id=? AND tenant_id=? AND grant_version=1 AND revoked=0`, testPrincipal, testTenant) != 1 {
						t.Fatal("COMMIT fault changed retained installation/owner identity")
					}
					if scalar(t, fresh, `SELECT count(*) FROM client_session WHERE token_hash=? AND principal_id=? AND grant_version=1`, owner, testPrincipal) != 1 {
						t.Fatal("original owner session lost or changed")
					}
					switch kind {
					case "issue":
						created := scalar(t, fresh, `SELECT count(*) FROM client_session WHERE token_hash=? AND session_id=? AND principal_id=? AND grant_version=1 AND revoked=1`, installationHash("synthetic-new-session"), testOperation, testPrincipal)
						if principals != 1 || revoked != 0 || (created != 0 && created != 1) || sessions != 1+created || (ordinal == 0 && created != 1) {
							t.Fatal("issue result not atomic old/new set", sessions, principals, revoked, created)
						}
						if _, e := fresh.CurrentSessionAuthorization(ctx, installationHash("synthetic-new-session"), "", ""); e == nil {
							t.Fatal("pending issue became active")
						}
					case "activate":
						row := scalar(t, fresh, `SELECT count(*) FROM client_session WHERE token_hash=? AND session_id=? AND principal_id=? AND grant_version=? AND expires_at=?`, pending.TokenHash, pending.SessionID, pending.PrincipalID, pending.GrantVersion, pending.ExpiresAt)
						active := scalar(t, fresh, `SELECT count(*) FROM client_session WHERE token_hash=? AND revoked=0`, pending.TokenHash)
						if sessions != 2 || principals != 1 || revoked != 0 || row != 1 || (active != 0 && active != 1) || (ordinal == 0 && active != 1) {
							t.Fatal("activation not atomic pending/active set", sessions, principals, revoked, row, active)
						}
						_, e := fresh.CurrentSessionAuthorization(ctx, pending.TokenHash, "", "")
						if (e == nil) != (active == 1) {
							t.Fatal("activation fresh authority disagrees with exact row")
						}
						t.Logf("fresh activation row matched original version/expiry; active=%d pending=%d", active, 1-active)
					case "replace-grants":
						created := scalar(t, fresh, `SELECT count(*) FROM principal WHERE principal_id=? AND tenant_id=? AND grant_version=1 AND revoked=0`, testOperation, testTenant)
						grant := scalar(t, fresh, `SELECT count(*) FROM grant_entry WHERE principal_id=? AND tenant_id=? AND workspace_id=? AND action='workspace.read'`, testOperation, testTenant, testWorkspace)
						allGrants := scalar(t, fresh, `SELECT count(*) FROM grant_entry`)
						if sessions != 1 || revoked != 0 || (created != 0 && created != 1) || grant != created || allGrants != created || principals != 1+created || (ordinal == 0 && created != 1) {
							t.Fatal("grant replacement not atomic old/new set", principals, created, grant, allGrants)
						}
					case "revoke", "undelivered":
						if sessions != 1 || principals != 1 || (revoked != 0 && revoked != 1) || (ordinal == 0 && revoked != 1) {
							t.Fatal("revocation result not atomic old/new set", sessions, principals, revoked)
						}
						terminal := scalar(t, fresh, `SELECT count(*) FROM client_session WHERE token_hash=? AND expires_at='1970-01-01T00:00:00Z'`, owner)
						if terminal != revoked {
							t.Fatal("revocation expiry and flag not atomic")
						}
					}
					t.Logf("fresh physical atomic old/new set verified sessions=%d principals=%d owner_revoked=%d; no replay", sessions, principals, revoked)
				})
			}
		})
	}
}
func TestInstallationAuthPendingActivationRechecks(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"success", "terminal-owner-revoke", "terminal-delivery-revoke", "principal-version", "principal-revoke", "expiry-changed", "expiry-elapsed", "grant-removed"} {
		t.Run(kind, func(t *testing.T) {
			d, owner := authStorageFixture(t)
			grants := []WorkspaceGrant{{testWorkspace, "workspace.read"}}
			if err := d.ReplacePrincipalAuthorization(ctx, owner, 1, testOperation, false, grants); err != nil {
				t.Fatal(err)
			}
			hash := installationHash("synthetic-pending-session")
			pending, err := d.CreateInstallationSession(ctx, testPrincipal, testOperation, testResource, hash, time.Now().Add(time.Minute), testWorkspace, "workspace.read")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = d.CurrentSessionAuthorization(ctx, hash, testWorkspace, "workspace.read"); err == nil {
				t.Fatal("pending accepted before activation")
			}
			err = nil
			switch kind {
			case "terminal-owner-revoke":
				err = d.RevokeInstallationSession(ctx, owner, 1, hash)
			case "terminal-delivery-revoke":
				err = d.RevokeUndeliveredSession(ctx, hash)
			case "principal-version":
				err = d.ReplacePrincipalAuthorization(ctx, owner, 1, testOperation, false, grants)
			case "principal-revoke":
				err = d.ReplacePrincipalAuthorization(ctx, owner, 1, testOperation, true, grants)
			case "expiry-changed":
				_, err = d.h.Conn.ExecContext(ctx, `UPDATE client_session SET expires_at=? WHERE token_hash=?`, time.Now().Add(2*time.Minute).UTC().Format(time.RFC3339Nano), hash)
			case "expiry-elapsed":
				pending.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
				_, err = d.h.Conn.ExecContext(ctx, `UPDATE client_session SET expires_at=? WHERE token_hash=?`, pending.ExpiresAt, hash)
			case "grant-removed":
				_, err = d.h.Conn.ExecContext(ctx, `DELETE FROM grant_entry WHERE principal_id=?`, testOperation)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = d.ActivateInstallationSession(ctx, testPrincipal, pending)
			if kind == "success" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = d.CurrentSessionAuthorization(ctx, hash, testWorkspace, "workspace.read"); err != nil {
					t.Fatal(err)
				}
				if err = d.ActivateInstallationSession(ctx, testPrincipal, pending); err == nil {
					t.Fatal("repeated activation accepted")
				}
			} else {
				if err == nil || errors.Is(err, ErrUnknown) {
					t.Fatal("invalid ACK activation not known-denied", err)
				}
				if _, err = d.CurrentSessionAuthorization(ctx, hash, testWorkspace, "workspace.read"); err == nil {
					t.Fatal("delayed ACK resurrected terminal or obsolete pending row")
				}
			}
		})
	}
}

func TestInstallationAuthRejectMalformedStoredSession(t *testing.T) {
	d, hash := authStorageFixture(t)
	ctx := context.Background()
	for _, value := range []string{"invalid-date", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)} {
		if _, err := d.h.Conn.ExecContext(ctx, `UPDATE client_session SET expires_at=?`, value); err != nil {
			t.Fatal(err)
		}
		if _, err := d.CurrentSessionAuthorization(ctx, hash, "", ""); err == nil {
			t.Fatal("bad expiry accepted")
		}
	}
	for _, hash := range []string{"", strings.Repeat("G", 64), strings.Repeat("A", 64)} {
		if _, err := d.CurrentSessionAuthorization(ctx, hash, "", ""); err == nil {
			t.Fatal("invalid hash accepted")
		}
	}
}
