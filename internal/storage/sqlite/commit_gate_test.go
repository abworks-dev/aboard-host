//go:build windows && amd64

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"modernc.org/libc"
	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func newTestGate(t *testing.T) (*Database, *CommitGate) {
	t.Helper()
	d := newStorageDB(t, WorkspaceDatabase)
	g, err := NewCommitGate(d, "synthetic-epoch-1")
	if err != nil {
		t.Fatal(err)
	}
	return d, g
}
func candidateFixture(t *testing.T, g *CommitGate, id, resource, kind string, head int64, base *string) (Candidate, SaveRequest) {
	t.Helper()
	c := Candidate{WorkspaceID: testWorkspace, CandidateID: id, PrincipalID: testPrincipal, ResourceID: resource, Kind: kind, BaseRevision: head, BaseDigest: base, Title: "合成 title", Body: "inline synthetic 😀\r\n", PreviewVersion: 1, PreviewEpoch: g.epoch, ExpiresAt: time.Now().Add(time.Hour)}
	var err error
	c.ContentDigest, c.CandidateDigest, err = candidateHashes(c)
	if err != nil {
		t.Fatal(err)
	}
	r := SaveRequest{InstallationID: testInstallation, TenantID: testTenant, WorkspaceID: testWorkspace, PrincipalID: testPrincipal, OperationID: testOperation, CandidateID: id, CandidateDigest: c.CandidateDigest, Epoch: g.epoch, PreviewVersion: c.PreviewVersion}
	return c, r
}
func prepareAdmission(t *testing.T, g *CommitGate, c Candidate, r SaveRequest) *Admission {
	t.Helper()
	if err := g.PutCandidate(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	result, a, err := g.Admit(context.Background(), r, time.Now())
	if err != nil || result.Status != Accepted || a == nil {
		t.Fatalf("admit: %+v %v", result, err)
	}
	return a
}
func sameSyntheticHistory(t *testing.T, path string) func() error {
	t.Helper()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Tests own this synthetic directory exclusively and start no other writers.
	// The real directory owner must supply its retained OS lease/history proof.
	return func() error {
		after, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !os.SameFile(before, after) {
			return errors.New("physical history replaced")
		}
		return nil
	}
}
func assertFormalState(t *testing.T, d *Database, want int64) {
	t.Helper()
	for _, query := range []string{"SELECT revision FROM workspace_head", "SELECT count(*) FROM text_version", "SELECT count(*) FROM commit_receipt", "SELECT count(*) FROM audit_entry", "SELECT count(*) FROM save_operation WHERE state='committed'", "SELECT count(*) FROM candidate WHERE state='applied'"} {
		if n := scalar(t, d, query); n != want {
			t.Fatalf("torn formal state %q=%d want=%d", query, n, want)
		}
	}
	if err := VerifySchema(context.Background(), d.h.Conn, WorkspaceDatabase); err != nil {
		t.Fatal(err)
	}
}
func TestCommitGateAtomicSaveAndLostResponse(t *testing.T) {
	d, g := newTestGate(t)
	path := d.path
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	a := prepareAdmission(t, g, c, r)
	assertFormalState(t, d, 0)
	if scalar(t, d, "SELECT count(*) FROM save_operation WHERE state='accepted'") != 1 {
		t.Fatal("admission not independently durable")
	}
	// Deliberately discard the first delivered response after the REAL commit.
	first, err := g.CommitAccepted(context.Background(), a)
	if err != nil || first.Status != Confirmed {
		t.Fatalf("save %v %+v", err, first)
	}
	assertFormalState(t, d, 1)
	var body string
	if err := d.h.Conn.QueryRowContext(context.Background(), "SELECT body FROM text_version").Scan(&body); err != nil || body != c.Body {
		t.Fatalf("body changed: %q %v", body, err)
	}
	if err := g.EndPhysical(); err != nil {
		t.Fatal(err)
	}
	d, err = OpenDatabase(context.Background(), path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
	if err != nil {
		t.Fatal(err)
	}
	g, err = NewCommitGate(d, "synthetic-new-epoch")
	if err != nil {
		t.Fatal(err)
	}
	defer g.EndPhysical()
	stale := r
	stale.Epoch = "old epoch"
	result, ticket, err := g.Admit(context.Background(), stale, time.Now().Add(48*time.Hour))
	if err != nil || ticket != nil || result.Status != Confirmed || result.Receipt == nil || *result.Receipt != *first.Receipt {
		t.Fatalf("same-key stale candidate not investigated first: %+v %v", result, err)
	}
	changed := stale
	changed.PrincipalID = testWorkspace
	if _, _, err := g.Admit(context.Background(), changed, time.Now()); !errors.Is(err, ErrKeyReuse) {
		t.Fatalf("actor key reuse %v", err)
	}
	changed = stale
	changed.CandidateDigest = strings.Repeat("a", 64)
	if _, _, err := g.Admit(context.Background(), changed, time.Now()); !errors.Is(err, ErrKeyReuse) {
		t.Fatalf("input key reuse %v", err)
	}
	// A genuinely NEW operation cannot bypass stale preview/base checks.
	changed = stale
	changed.OperationID = testWorkspace
	if result, _, err := g.Admit(context.Background(), changed, time.Now()); err == nil || result.Status != Rejected {
		t.Fatalf("new key stale candidate accepted %+v %v", result, err)
	}
	base := c.ContentDigest
	update, req := candidateFixture(t, g, testWorkspace, testResource, "update-text", 1, &base)
	req.OperationID = testTenant
	update.Body = "updated exact bytes\r\n"
	update.ContentDigest, update.CandidateDigest, err = candidateHashes(update)
	if err != nil {
		t.Fatal(err)
	}
	req.CandidateDigest = update.CandidateDigest
	a = prepareAdmission(t, g, update, req)
	result, err = g.CommitAccepted(context.Background(), a)
	if err != nil || result.Status != Confirmed || result.Receipt.NewVersion != 2 || result.Receipt.NewRevision != 2 {
		t.Fatalf("update %+v %v", result, err)
	}
	assertFormalState(t, d, 2)
	for _, statement := range []string{"UPDATE text_version SET body='tampered'", "DELETE FROM text_version", "UPDATE commit_receipt SET new_revision=99", "DELETE FROM commit_receipt"} {
		if _, err := d.h.Conn.ExecContext(context.Background(), statement); err == nil {
			t.Fatalf("immutable history changed: %s", statement)
		}
	}
}

func TestCommitGateAbsenceAndFreshViewPrerequisites(t *testing.T) {
	d, g := newTestGate(t)
	path := d.path
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	a := prepareAdmission(t, g, c, r)
	_ = a
	proof := sameSyntheticHistory(t, path)
	result, err := g.Investigate(context.Background(), r)
	if err != nil || result.Status != Unknown {
		t.Fatalf("ordinary missing receipt inferred rollback %+v %v", result, err)
	}
	stale := r
	stale.Epoch = "old-epoch"
	result, ticket, err := g.Admit(context.Background(), stale, time.Now().Add(48*time.Hour))
	if err != nil || result.Status != Unknown || ticket != nil {
		t.Fatalf("existing accepted key revalidated/replayed stale candidate %+v %v", result, err)
	}
	if result, err := g.InvestigateFresh(context.Background(), r, proof); !errors.Is(err, ErrFenced) || result.Status != Unknown {
		t.Fatalf("active connection accepted fresh proof %+v %v", result, err)
	}
	// Snapshot established before any formal result; absent receipt cannot be
	// promoted to rollback even when another admitted operation may execute.
	old := openGate(t, path, ReadWriteExisting)
	snapshot, err := old.Conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result, found, err := lookup(context.Background(), snapshot, r, a.input); err != nil || !found || result.Status != Unknown {
		t.Fatalf("old snapshot classified %+v %v", result, err)
	}
	if err := snapshot.Rollback(); err != nil {
		t.Fatal(err)
	}
	closeGate(t, old)
	if err := g.EndPhysical(); err != nil {
		t.Fatal(err)
	}
	if result, err := g.InvestigateFresh(context.Background(), r, nil); !errors.Is(err, ErrFenced) || result.Status != Unknown {
		t.Fatal("missing exclusion/history proof accepted")
	}
	failed := func() error { return errors.New("synthetic lease or history evidence unavailable") }
	if result, err := g.InvestigateFresh(context.Background(), r, failed); err == nil || result.Status != Unknown {
		t.Fatal("failed exclusion proof accepted")
	}
	result, err = g.InvestigateFresh(context.Background(), r, proof)
	if err != nil || result.Status != NotCommitted || result.Receipt != nil {
		t.Fatalf("fresh known admission absence %+v %v", result, err)
	}
	unknownKey := r
	unknownKey.OperationID = testWorkspace
	result, err = g.InvestigateFresh(context.Background(), unknownKey, proof)
	if err != nil || result.Status != Unknown {
		t.Fatalf("absent admission invented outcome %+v %v", result, err)
	}
	if _, _, err := g.Admit(context.Background(), r, time.Now()); !errors.Is(err, ErrFenced) {
		t.Fatalf("fresh classification reopened submission %v", err)
	}
	if result, err := g.CommitAccepted(context.Background(), a); !errors.Is(err, ErrFenced) || result.Status != Unknown {
		t.Fatalf("closed gate invented rejection/replayed: %+v %v", result, err)
	}
}

func TestCommitGateRestartAndBrokenHistoryFence(t *testing.T) {
	t.Run("restart-accepted", func(t *testing.T) {
		d, g := newTestGate(t)
		path := d.path
		c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
		_ = prepareAdmission(t, g, c, r)
		if err := g.EndPhysical(); err != nil {
			t.Fatal(err)
		}
		d, err := OpenDatabase(context.Background(), path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
		if err != nil {
			t.Fatal(err)
		}
		restarted, err := NewCommitGate(d, "new-epoch")
		if err != nil {
			t.Fatal(err)
		}
		defer restarted.EndPhysical()
		if err := restarted.PutCandidate(context.Background(), c); !errors.Is(err, ErrFenced) {
			t.Fatalf("restart minted writer despite retained admission: %v", err)
		}
		newKey := r
		newKey.OperationID = testWorkspace
		if result, ticket, err := restarted.Admit(context.Background(), newKey, time.Now()); !errors.Is(err, ErrFenced) || result.Status != Unknown || ticket != nil {
			t.Fatalf("restart admitted another key %+v %v", result, err)
		}
		if result, ticket, err := restarted.Admit(context.Background(), r, time.Now().Add(48*time.Hour)); err != nil || result.Status != Unknown || ticket != nil {
			t.Fatalf("restart resurrected old ticket %+v %v", result, err)
		}
	})
	t.Run("receipt-query-failure", func(t *testing.T) {
		d, g := newTestGate(t)
		c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
		a := prepareAdmission(t, g, c, r)
		proof := sameSyntheticHistory(t, d.path)
		if _, err := d.h.Conn.ExecContext(context.Background(), "DROP TABLE commit_receipt"); err != nil {
			t.Fatal(err)
		}
		if result, err := g.CommitAccepted(context.Background(), a); !errors.Is(err, ErrUnknown) || result.Status != Unknown {
			t.Fatalf("read rollback invented business rejection %+v %v", result, err)
		}
		if err := g.EndPhysical(); err != nil {
			t.Fatal(err)
		}
		if result, err := g.InvestigateFresh(context.Background(), r, proof); !errors.Is(err, ErrSchema) || result.Status != Unknown {
			t.Fatalf("broken history absence promoted %+v %v", result, err)
		}
	})
}

func TestCommitGateRealCommitFlushFailures(t *testing.T) {
	root := t.TempDir()
	stimulusPath := filepath.Join(root, "synthetic-readonly.bin")
	if err := os.WriteFile(stimulusPath, []byte("real synthetic native flush error"), 0600); err != nil {
		t.Fatal(err)
	}
	stimulus, err := os.Open(stimulusPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stimulus.Close()
	if err := windows.FlushFileBuffers(windows.Handle(stimulus.Fd())); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatal(err)
	}
	for _, update := range []bool{false, true} {
		var flushCount int32
		for ordinal := int32(0); ordinal == 0 || ordinal <= flushCount; ordinal++ {
			t.Run(fmt.Sprintf("update-%t/business-flush-%d", update, ordinal), func(t *testing.T) {
				d, g := newTestGate(t)
				path := d.path
				c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
				a := prepareAdmission(t, g, c, r)
				baseCount := int64(0)
				if update {
					if result, err := g.CommitAccepted(context.Background(), a); err != nil || result.Status != Confirmed {
						t.Fatalf("update baseline %+v %v", result, err)
					}
					base := c.ContentDigest
					c, r = candidateFixture(t, g, testWorkspace, testResource, "update-text", 1, &base)
					r.OperationID = testTenant
					c.Body = "new synthetic update body"
					var err error
					c.ContentDigest, c.CandidateDigest, err = candidateHashes(c)
					if err != nil {
						t.Fatal(err)
					}
					r.CandidateDigest = c.CandidateDigest
					a = prepareAdmission(t, g, c, r)
					baseCount = 1
				}
				proof := sameSyntheticHistory(t, path)
				flushFault.handle = stimulus.Fd()
				flushFault.at = ordinal
				flushFault.calls.Store(0)
				flushFault.failed.Store(0)
				restore := replaceFlushForGate(t)
				result, err := g.CommitAccepted(context.Background(), a)
				calls, failures := flushFault.calls.Load(), flushFault.failed.Load()
				restore()
				if ordinal == 0 {
					if err != nil || result.Status != Confirmed || calls < 1 || calls > 8 {
						t.Fatalf("baseline %+v %v calls=%d", result, err, calls)
					}
					flushCount = calls
				} else {
					var sqlErr *driver.Error
					if result.Status != Unknown || result.Receipt != nil || !errors.Is(err, ErrUnknown) || !errors.As(err, &sqlErr) || sqlErr.Code() != sqlite3.SQLITE_IOERR_FSYNC || failures != 1 {
						t.Fatalf("fault false acknowledgment: %+v err=%v calls=%d failures=%d", result, err, calls, failures)
					}
					newKey := r
					newKey.OperationID = testPrincipal
					if result, _, err := g.Admit(context.Background(), newKey, time.Now()); !errors.Is(err, ErrFenced) || result.Status != Unknown {
						t.Fatalf("fenced gate allowed new key: %+v %v", result, err)
					}
				}
				t.Logf("real business COMMIT ordinal=%d calls=%d nativeFailures=%d status=%s err=%v; no retry", ordinal, calls, failures, result.Status, err)
				if err := g.EndPhysical(); err != nil {
					t.Fatal(err)
				}
				investigated, err := g.InvestigateFresh(context.Background(), r, proof)
				if err != nil {
					t.Fatal(err)
				}
				if investigated.Status != Confirmed && investigated.Status != NotCommitted {
					t.Fatalf("fresh result %+v", investigated)
				}
				reopened, err := OpenDatabase(context.Background(), path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
				if err != nil {
					t.Fatal(err)
				}
				defer closeStorage(t, reopened)
				want := baseCount
				if investigated.Status == Confirmed {
					want++
				}
				assertFormalState(t, reopened, want)
				wantRecords := int64(0)
				if want > 0 {
					wantRecords = 1
				}
				if scalar(t, reopened, "SELECT count(*) FROM text_record") != wantRecords || (want > 0 && scalar(t, reopened, "SELECT current_version FROM text_record") != want) {
					t.Fatal("record pointer torn")
				}
				if ordinal == 0 && want != baseCount+1 {
					t.Fatal("confirmed commit lost")
				}
				t.Logf("physical end + fresh verified history: receipt=%s formalVersions=%d", investigated.Status, want)
			})
		}
	}
}

func TestCommitGateRealAdmissionFlushFailures(t *testing.T) {
	stimulusPath := filepath.Join(t.TempDir(), "synthetic-admission-readonly.bin")
	if err := os.WriteFile(stimulusPath, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	stimulus, err := os.Open(stimulusPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stimulus.Close()
	var flushCount int32
	for ordinal := int32(0); ordinal == 0 || ordinal <= flushCount; ordinal++ {
		t.Run(fmt.Sprintf("admission-flush-%d", ordinal), func(t *testing.T) {
			d, g := newTestGate(t)
			path := d.path
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			if err := g.PutCandidate(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			proof := sameSyntheticHistory(t, path)
			flushFault.handle = stimulus.Fd()
			flushFault.at = ordinal
			flushFault.calls.Store(0)
			flushFault.failed.Store(0)
			restore := replaceFlushForGate(t)
			result, ticket, err := g.Admit(context.Background(), r, time.Now())
			calls, failures := flushFault.calls.Load(), flushFault.failed.Load()
			restore()
			if ordinal == 0 {
				if err != nil || result.Status != Accepted || ticket == nil || calls < 1 || calls > 8 {
					t.Fatalf("admission baseline %+v %v calls=%d", result, err, calls)
				}
				flushCount = calls
			} else {
				if !errors.Is(err, ErrUnknown) || result.Status != Unknown || ticket != nil || failures != 1 {
					t.Fatalf("failed admission acknowledged %+v %v failures=%d", result, err, failures)
				}
			}
			if err := g.EndPhysical(); err != nil {
				t.Fatal(err)
			}
			fresh, investigateErr := g.InvestigateFresh(context.Background(), r, proof)
			if investigateErr != nil {
				t.Fatal(investigateErr)
			}
			if fresh.Status != Unknown && fresh.Status != NotCommitted {
				t.Fatalf("admission invented business result %+v", fresh)
			}
			reopened, openErr := OpenDatabase(context.Background(), path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
			if openErr != nil {
				t.Fatal(openErr)
			}
			assertFormalState(t, reopened, 0)
			closeStorage(t, reopened)
			t.Logf("real admission COMMIT ordinal=%d calls=%d failures=%d result=%s err=%v fresh=%s; no business ticket/replay on failure", ordinal, calls, failures, result.Status, err, fresh.Status)
		})
	}
}

func TestMigrationRealInitCommitFlushFailures(t *testing.T) {
	stimulusPath := filepath.Join(t.TempDir(), "synthetic-init-readonly.bin")
	if err := os.WriteFile(stimulusPath, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	stimulus, err := os.Open(stimulusPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stimulus.Close()
	for _, kind := range []DatabaseKind{WorkspaceDatabase, InstallationDatabase} {
		var count int32
		for ordinal := int32(0); ordinal == 0 || ordinal <= count; ordinal++ {
			t.Run(fmt.Sprintf("%s-flush-%d", kind, ordinal), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "synthetic-init.sqlite")
				h := openGate(t, path, CreateNew)
				flushFault.handle = stimulus.Fd()
				flushFault.at = ordinal
				flushFault.calls.Store(0)
				flushFault.failed.Store(0)
				restore := replaceFlushForGate(t)
				err := initialize(context.Background(), h.Conn, testGenesis(kind))
				calls, failures := flushFault.calls.Load(), flushFault.failed.Load()
				restore()
				if ordinal == 0 {
					if err != nil || calls < 1 || calls > 8 {
						t.Fatalf("init baseline calls=%d err=%v", calls, err)
					}
					count = calls
				} else {
					if !errors.Is(err, ErrUnknown) || failures != 1 {
						t.Fatalf("init fault acknowledged: %v failures=%d", err, failures)
					}
				}
				closeGate(t, h)
				d, verifyErr := OpenDatabase(context.Background(), path, ReadWriteExisting, Genesis{Kind: kind})
				if verifyErr == nil {
					closeStorage(t, d)
				} else if !errors.Is(verifyErr, ErrSchema) {
					t.Fatal(verifyErr)
				}
				if ordinal == 0 && verifyErr != nil {
					t.Fatal("confirmed genesis lost")
				}
				if _, err := OpenDatabase(context.Background(), path, CreateNew, testGenesis(kind)); !errors.Is(err, os.ErrExist) {
					t.Fatal("init error remnant reseeded")
				}
				t.Logf("real init COMMIT kind=%s ordinal=%d calls=%d failure=%d err=%v freshVerify=%v", kind, ordinal, calls, failures, err, verifyErr)
			})
		}
	}
}

func TestCommitGateBusyCancellationPhysicalEnd(t *testing.T) {
	for _, cancelWork := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel-%t", cancelWork), func(t *testing.T) {
			d, g := newTestGate(t)
			path := d.path
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			a := prepareAdmission(t, g, c, r)
			proof := sameSyntheticHistory(t, path)
			reader := openGate(t, path, ReadWriteExisting)
			snapshot, err := reader.Conn.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			var head int
			if err := snapshot.QueryRow("SELECT revision FROM workspace_head").Scan(&head); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			cancel := func() {}
			if cancelWork {
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
			}
			defer cancel()
			type response struct {
				result SaveResult
				err    error
			}
			done := make(chan response, 1)
			started := time.Now()
			go func() { result, err := g.CommitAccepted(ctx, a); done <- response{result, err} }()
			// The reader holds a real rollback-journal SHARED lock, so COMMIT cannot
			// complete successfully. Retain it through the result, do not release early.
			var responseValue response
			select {
			case responseValue = <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("busy/cancel driver did not settle")
			}
			if cancelWork && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Fatalf("cancellation stimulus never fired: %v", ctx.Err())
			}
			if responseValue.err == nil || responseValue.result.Status == Confirmed {
				t.Fatalf("busy/cancel falsely confirmed %+v", responseValue)
			}
			if !cancelWork {
				var e *driver.Error
				if !errors.As(responseValue.err, &e) || e.Code()&255 != sqlite3.SQLITE_BUSY {
					t.Fatalf("not real busy: %v", responseValue.err)
				}
			}
			if result, err := g.InvestigateFresh(context.Background(), r, proof); !errors.Is(err, ErrFenced) || result.Status != Unknown {
				t.Fatal("timeout treated as physical end")
			}
			if err := snapshot.Rollback(); err != nil {
				t.Fatal(err)
			}
			closeGate(t, reader)
			// database/sql may have an asynchronous canceled-Tx rollback. EndPhysical
			// must wait for physical connection/pool closure, not just context.Done.
			if err := g.EndPhysical(); err != nil {
				t.Fatal(err)
			}
			result, err := g.InvestigateFresh(context.Background(), r, proof)
			if err != nil || result.Status != NotCommitted {
				t.Fatalf("fresh busy/cancel %+v %v", result, err)
			}
			reopened, err := OpenDatabase(context.Background(), path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
			if err != nil {
				t.Fatal(err)
			}
			assertFormalState(t, reopened, 0)
			closeStorage(t, reopened)
			t.Logf("real busy/cancel=%t elapsed=%s originalStatus=%s originalErr=%v; physical-ended fresh status=%s", cancelWork, time.Since(started), responseValue.result.Status, responseValue.err, result.Status)
		})
	}
}

// Both accepted admissions used Head 0. A commits; B must record its proven
// rollback rather than leave a durable accepted row while returning rejected.
func conflictingAdmissions(t *testing.T) (*Database, *CommitGate, *Admission, SaveRequest) {
	t.Helper()
	d, g := newTestGate(t)
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	a := prepareAdmission(t, g, c, r)
	c, r = candidateFixture(t, g, testWorkspace, testTenant, "create-text", 0, nil)
	r.OperationID = testInstallation
	b := prepareAdmission(t, g, c, r)
	if result, err := g.CommitAccepted(context.Background(), a); err != nil || result.Status != Confirmed {
		t.Fatalf("A %+v %v", result, err)
	}
	return d, g, b, r
}

func TestCommitGateDurableRejectedAdmissionRestart(t *testing.T) {
	d, g, b, r := conflictingAdmissions(t)
	path := d.path
	result, err := g.CommitAccepted(context.Background(), b)
	if !errors.Is(err, ErrConflict) || result.Status != Rejected || result.FailureCode != "STALE_BASELINE" || result.Receipt != nil {
		t.Fatalf("B %+v %v", result, err)
	}
	if scalar(t, d, "SELECT count(*) FROM save_operation WHERE state='accepted'") != 0 || scalar(t, d, "SELECT count(*) FROM save_operation WHERE state='rejected' AND failure_code='STALE_BASELINE'") != 1 {
		t.Fatal("returned rejection without durable matching result")
	}
	assertFormalState(t, d, 1)
	if result, err := g.CommitAccepted(context.Background(), b); err != nil || result.Status != Rejected || result.FailureCode != "STALE_BASELINE" {
		t.Fatalf("used rejection ticket not investigation-only %+v %v", result, err)
	}
	if err := g.EndPhysical(); err != nil {
		t.Fatal(err)
	}
	d, err = OpenDatabase(context.Background(), path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
	if err != nil {
		t.Fatal(err)
	}
	g, err = NewCommitGate(d, "synthetic-restart")
	if err != nil {
		t.Fatal(err)
	}
	defer g.EndPhysical()
	stale := r
	stale.Epoch = "old epoch"
	result, ticket, err := g.Admit(context.Background(), stale, time.Now().Add(48*time.Hour))
	if err != nil || ticket != nil || result.Status != Rejected || result.FailureCode != "STALE_BASELINE" {
		t.Fatalf("restarted rejection revalidated stale candidate %+v %v", result, err)
	}
	if result, err := g.Investigate(context.Background(), r); err != nil || result.Status != Rejected || result.FailureCode != "STALE_BASELINE" {
		t.Fatalf("durable rejection investigation %+v %v", result, err)
	}
	changed := r
	changed.CandidateDigest = strings.Repeat("a", 64)
	if _, _, err := g.Admit(context.Background(), changed, time.Now()); !errors.Is(err, ErrKeyReuse) {
		t.Fatalf("rejected key rebound: %v", err)
	}
	// A proved, durably recorded rejection must not turn into a restart fence.
	c, fresh := candidateFixture(t, g, testPrincipal, testPrincipal, "create-text", 1, nil)
	fresh.OperationID = testResource
	a := prepareAdmission(t, g, c, fresh)
	if result, err := g.CommitAccepted(context.Background(), a); err != nil || result.Status != Confirmed {
		t.Fatalf("rejection fenced unrelated fresh operation %+v %v", result, err)
	}
	assertFormalState(t, d, 2)
}

func TestCommitGateWriteFailureDurableRejection(t *testing.T) {
	d, g := newTestGate(t)
	defer g.EndPhysical()
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	a := prepareAdmission(t, g, c, r)
	// Fail AFTER the real text_version INSERT, proving its rollback is atomic.
	if _, err := d.h.Conn.ExecContext(context.Background(), "CREATE TRIGGER synthetic_write_failure BEFORE INSERT ON text_record BEGIN SELECT RAISE(ABORT,'synthetic write failure'); END"); err != nil {
		t.Fatal(err)
	}
	result, err := g.CommitAccepted(context.Background(), a)
	if err == nil || result.Status != Rejected || result.FailureCode != "WRITE_FAILED" || result.Receipt != nil {
		t.Fatalf("write failure %+v %v", result, err)
	}
	if _, err := d.h.Conn.ExecContext(context.Background(), "DROP TRIGGER synthetic_write_failure"); err != nil {
		t.Fatal(err)
	}
	assertFormalState(t, d, 0)
	if scalar(t, d, "SELECT count(*) FROM save_operation WHERE state='rejected' AND failure_code='WRITE_FAILED'") != 1 {
		t.Fatal("write failure was not durably rejected")
	}
}

func TestCommitGateRealRejectionFlushFailures(t *testing.T) {
	stimulusPath := filepath.Join(t.TempDir(), "synthetic-reject-readonly.bin")
	if err := os.WriteFile(stimulusPath, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	stimulus, err := os.Open(stimulusPath)
	if err != nil {
		t.Fatal(err)
	}
	defer stimulus.Close()
	var count int32
	for ordinal := int32(0); ordinal == 0 || ordinal <= count; ordinal++ {
		t.Run(fmt.Sprintf("rejection-flush-%d", ordinal), func(t *testing.T) {
			d, g, b, r := conflictingAdmissions(t)
			path := d.path
			proof := sameSyntheticHistory(t, path)
			flushFault.handle = stimulus.Fd()
			flushFault.at = ordinal
			flushFault.calls.Store(0)
			flushFault.failed.Store(0)
			restore := replaceFlushForGate(t)
			result, err := g.CommitAccepted(context.Background(), b)
			calls, failures := flushFault.calls.Load(), flushFault.failed.Load()
			restore()
			if ordinal == 0 {
				if !errors.Is(err, ErrConflict) || result.Status != Rejected || result.FailureCode != "STALE_BASELINE" || calls < 1 || calls > 8 {
					t.Fatalf("rejection baseline %+v %v calls=%d", result, err, calls)
				}
				count = calls
			} else {
				var sqlErr *driver.Error
				if result.Status != Unknown || result.FailureCode != "" || result.Receipt != nil || !errors.Is(err, ErrUnknown) || !errors.As(err, &sqlErr) || sqlErr.Code() != sqlite3.SQLITE_IOERR_FSYNC || failures != 1 {
					t.Fatalf("reject-record fault falsely rejected %+v %v calls=%d failures=%d", result, err, calls, failures)
				}
				other := r
				other.OperationID = testPrincipal
				if result, _, err := g.Admit(context.Background(), other, time.Now()); !errors.Is(err, ErrFenced) || result.Status != Unknown {
					t.Fatalf("reject-record unknown allowed another key %+v %v", result, err)
				}
			}
			if err := g.EndPhysical(); err != nil {
				t.Fatal(err)
			}
			fresh, freshErr := g.InvestigateFresh(context.Background(), r, proof)
			if freshErr != nil || (fresh.Status != Rejected && fresh.Status != NotCommitted) || (fresh.Status == Rejected && fresh.FailureCode != "STALE_BASELINE") {
				t.Fatalf("fresh rejection %+v %v", fresh, freshErr)
			}
			if ordinal == 0 && fresh.Status != Rejected {
				t.Fatal("confirmed rejection lost")
			}
			reopened, openErr := OpenDatabase(context.Background(), path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
			if openErr != nil {
				t.Fatal(openErr)
			}
			assertFormalState(t, reopened, 1)
			closeStorage(t, reopened)
			t.Logf("real rejection COMMIT ordinal=%d calls=%d failures=%d result=%s originalErr=%v fresh=%s; no replay", ordinal, calls, failures, result.Status, err, fresh.Status)
		})
	}
}

// Test-only VFS pin: native SQLite is inside its real COMMIT syscall callback,
// not a mock SQL driver. It eventually performs the original real Windows flush.
var commitPause struct {
	entered, release chan struct{}
	once             sync.Once
	returned         atomic.Bool
	timedOut         atomic.Bool
}

func pausedCommitFlush(tls *libc.TLS, actualHandle uintptr) int32 {
	commitPause.once.Do(func() {
		close(commitPause.entered)
		select {
		case <-commitPause.release:
		case <-time.After(10 * time.Second):
			commitPause.timedOut.Store(true)
		}
	})
	result := libc.XFlushFileBuffers(tls, actualHandle)
	commitPause.returned.Store(true)
	return result
}

func installPausedCommitFlush(t *testing.T) func() {
	t.Helper()
	tls := libc.NewTLS()
	name, err := libc.CString("win32")
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, name)
	vfs := sqlite3.Xsqlite3_vfs_find(tls, name)
	if vfs == 0 {
		t.Fatal("required win32 VFS absent")
	}
	var view sqlite3.Tsqlite3_vfs
	libc.Xmemcpy(tls, uintptr(unsafe.Pointer(&view)), vfs, libc.Tsize_t(unsafe.Sizeof(view)))
	if view.FiVersion < 3 || view.FxSetSystemCall == 0 || view.FxGetSystemCall == 0 {
		t.Fatal("required VFS syscall inspection unavailable")
	}
	get := *(*func(*libc.TLS, uintptr, uintptr) uintptr)(unsafe.Pointer(&view.FxGetSystemCall))
	set := *(*func(*libc.TLS, uintptr, uintptr, uintptr) int32)(unsafe.Pointer(&view.FxSetSystemCall))
	syscallName, err := libc.CString("FlushFileBuffers")
	if err != nil {
		t.Fatal(err)
	}
	original := get(tls, vfs, syscallName)
	if original == 0 {
		t.Fatal("VFS flush syscall unavailable")
	}
	callback := pausedCommitFlush
	pointer := *(*uintptr)(unsafe.Pointer(&callback))
	if rc := set(tls, vfs, syscallName, pointer); rc != sqlite3.SQLITE_OK {
		t.Fatalf("cannot install paused real flush: %d", rc)
	}
	var once sync.Once
	restore := func() {
		once.Do(func() {
			if rc := set(tls, vfs, syscallName, original); rc != sqlite3.SQLITE_OK {
				t.Errorf("cannot restore original flush: %d", rc)
			}
			libc.Xfree(tls, syscallName)
			tls.Close()
		})
	}
	t.Cleanup(restore)
	return restore
}

func TestCommitGateEndPhysicalWaitsForCanceledNativeCommit(t *testing.T) {
	d, g := newTestGate(t)
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	a := prepareAdmission(t, g, c, r)
	proof := sameSyntheticHistory(t, d.path)
	commitPause.entered = make(chan struct{})
	commitPause.release = make(chan struct{})
	commitPause.once = sync.Once{}
	commitPause.returned.Store(false)
	commitPause.timedOut.Store(false)
	restore := installPausedCommitFlush(t)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(commitPause.release) }) }
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	type response struct {
		result SaveResult
		err    error
	}
	committed := make(chan response, 1)
	go func() { result, err := g.CommitAccepted(ctx, a); committed <- response{result, err} }()
	select {
	case <-commitPause.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("real COMMIT never entered VFS flush")
	}
	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || commitPause.returned.Load() {
		t.Fatal("deadline did not precede physical COMMIT return")
	}
	select {
	case response := <-committed:
		t.Fatalf("driver returned while native callback pinned %+v", response)
	default:
	}
	closing := make(chan struct{})
	ended := make(chan error, 1)
	go func() { close(closing); ended <- g.EndPhysical() }()
	<-closing
	select {
	case err := <-ended:
		t.Fatalf("EndPhysical acknowledged before native return: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if commitPause.returned.Load() || d.h.db.Stats().OpenConnections != 1 {
		t.Fatal("physical connection closed while native COMMIT pinned")
	}
	release()
	var original response
	select {
	case original = <-committed:
	case <-time.After(5 * time.Second):
		t.Fatal("COMMIT did not physically settle after release")
	}
	select {
	case err := <-ended:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("EndPhysical did not finish after original work ended")
	}
	restore()
	if commitPause.timedOut.Load() || !commitPause.returned.Load() || d.h.db.Stats().OpenConnections != 0 {
		t.Fatal("pin timed out or physical connection remained open")
	}
	// A canceled operation may still commit; neither timeout nor close classifies
	// persistence. Only the physically-ended, excluded fresh view does that.
	fresh, err := g.InvestigateFresh(context.Background(), r, proof)
	if err != nil || (fresh.Status != Confirmed && fresh.Status != NotCommitted) {
		t.Fatalf("fresh after concurrent drain %+v %v", fresh, err)
	}
	if original.result.Status == Confirmed && fresh.Status != Confirmed {
		t.Fatal("confirmed native commit not durable")
	}
	t.Logf("ctx deadline observed while real native COMMIT pinned; EndPhysical waited with connection open, then both ended; original=%s err=%v fresh=%s; adapter mutex/connection only, not OS lease or V014", original.result.Status, original.err, fresh.Status)
}
