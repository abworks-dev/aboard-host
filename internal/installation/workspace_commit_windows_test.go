//go:build windows && amd64

package installation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	"golang.org/x/sys/windows"
	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func commitPreviewInput(operation string, p workspace.Preview) workspace.SaveInput {
	c := p.Candidate
	return workspace.SaveInput{OperationID: operation, Epoch: c.PreviewEpoch, CandidateID: c.CandidateID, CandidateDigest: c.CandidateDigest, PreviewVersion: c.PreviewVersion}
}
func TestWorkspaceCommitRealOwnerIsolationAndOriginalKey(t *testing.T) {
	f, creator, operator, token, request := creationFixture(t)
	ctx := context.Background()
	a, err := creator.CreateWorkspace(ctx, operator, request)
	if err != nil {
		t.Fatal(err)
	}
	second := request
	second.OperationID = authSameResource
	second.RootLocator = filepath.Join(f.allowed, "commit-b")
	b, err := creator.CreateWorkspace(ctx, operator, second)
	if err != nil {
		t.Fatal(err)
	}
	opener, err := NewWorkspaceOpener(f.auth)
	if err != nil {
		t.Fatal(err)
	}
	auth := func(ws, action string) *Authorization {
		t.Helper()
		a, err := f.auth.Authorize(ctx, token, ws, action)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	openA, closeA, writeA, readA := auth(a.WorkspaceID, "workspace.open"), auth(a.WorkspaceID, "workspace.close"), auth(a.WorkspaceID, "workspace.write"), auth(a.WorkspaceID, "workspace.read")
	openB, closeB, writeB, readB := auth(b.WorkspaceID, "workspace.open"), auth(b.WorkspaceID, "workspace.close"), auth(b.WorkspaceID, "workspace.write"), auth(b.WorkspaceID, "workspace.read")
	av, err := opener.OpenWorkspace(ctx, openA, 1)
	if err != nil {
		t.Fatal(err)
	}
	bv, err := opener.OpenWorkspace(ctx, openB, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opener.DrainWorkspace(ctx, closeA); err != nil {
			t.Error(err)
		}
		if err := opener.DrainWorkspace(ctx, closeB); err != nil {
			t.Error(err)
		}
	}()
	ap, err := opener.PreviewRecord(ctx, writeA, workspace.PreviewInput{Epoch: av.Epoch, Kind: "create-text", Title: "same title", Body: "A exact 😀\r\né"})
	if err != nil {
		t.Fatal(err)
	}
	bp, err := opener.PreviewRecord(ctx, writeB, workspace.PreviewInput{Epoch: bv.Epoch, Kind: "create-text", Title: "same title", Body: "B distinct\nbytes"})
	if err != nil {
		t.Fatal(err)
	}
	ai, bi := commitPreviewInput(authA, ap), commitPreviewInput(authA, bp)
	bad := ai
	bad.PreviewVersion++
	if got, err := opener.CommitRecord(ctx, writeA, bad); err == nil || got.Status != workspace.SaveRejected || got.Receipt != nil {
		t.Fatal("wrong preview version admitted", got, err)
	}
	bad = ai
	bad.Epoch = authSameResource
	if got, err := opener.CommitRecord(ctx, writeA, bad); !errors.Is(err, workspace.ErrStaleHandle) || got.Status != workspace.SaveRejected {
		t.Fatal("new key old epoch", got, err)
	}
	if got, err := opener.CommitRecord(ctx, readA, ai); !errors.Is(err, ErrUnauthorized) || got.Receipt != nil {
		t.Fatal("read credential saved", got, err)
	}
	ar, err := opener.CommitRecord(ctx, writeA, ai)
	if err != nil || ar.Status != workspace.SaveConfirmed || ar.Receipt == nil || ar.Receipt.NewVersion != 1 || ar.Receipt.NewRevision != 1 {
		t.Fatal(ar, err)
	}
	br, err := opener.CommitRecord(ctx, writeB, bi)
	if err != nil || br.Status != workspace.SaveConfirmed || br.Receipt == nil || br.Receipt.WorkspaceID != b.WorkspaceID {
		t.Fatal("same operation key globally deduped", br, err)
	}
	foreign := bi
	if got, err := opener.CommitRecord(ctx, writeA, foreign); !errors.Is(err, workspace.ErrSaveKeyReuse) || got.Receipt != nil {
		t.Fatal("same-key foreign candidate", got, err)
	}
	current, err := opener.ReadRecord(ctx, readA, ap.Candidate.ResourceID)
	if err != nil || current.Body != ap.Candidate.Body || current.OperationID != ai.OperationID {
		t.Fatal(current, err)
	}
	base := current.ContentDigest
	up, err := opener.PreviewRecord(ctx, writeA, workspace.PreviewInput{Epoch: av.Epoch, Kind: "update-text", ResourceID: current.ResourceID, BaseRevision: 1, BaseDigest: &base, Title: "updated A", Body: "exact update\r\n"})
	if err != nil {
		t.Fatal(err)
	}
	ur, err := opener.CommitRecord(ctx, writeA, commitPreviewInput(authB, up))
	if err != nil || ur.Status != workspace.SaveConfirmed || ur.Receipt.NewVersion != 2 || ur.Receipt.NewRevision != 2 {
		t.Fatal(ur, err)
	}
	// Same immutable base under distinct keys: one formal effect, no lost update.
	inputs := make([]workspace.SaveInput, 2)
	for n := range inputs {
		p, err := opener.PreviewRecord(ctx, writeA, workspace.PreviewInput{Epoch: av.Epoch, Kind: "create-text", BaseRevision: 2, Title: "same base", Body: fmt.Sprint("A concurrent ", n)})
		if err != nil {
			t.Fatal(err)
		}
		inputs[n] = commitPreviewInput(fmt.Sprintf("73000000-0000-4000-8000-%012d", n+1), p)
	}
	type outcome struct {
		r   workspace.SaveResult
		err error
	}
	out := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, in := range inputs {
		wg.Add(1)
		go func(in workspace.SaveInput) {
			defer wg.Done()
			r, err := opener.CommitRecord(ctx, writeA, in)
			out <- outcome{r, err}
		}(in)
	}
	wg.Wait()
	close(out)
	confirmed, rejected := 0, 0
	for got := range out {
		if got.r.Status == workspace.SaveConfirmed && got.err == nil {
			confirmed++
		} else if got.r.Status == workspace.SaveRejected && errors.Is(got.err, sqlite.ErrConflict) {
			rejected++
		} else {
			t.Fatal(got)
		}
	}
	if confirmed != 1 || rejected != 1 {
		t.Fatal(confirmed, rejected)
	}
	if err = f.auth.SetPrincipalAuthorization(ctx, operator, authRestricted, false, []sqlite.WorkspaceGrant{{WorkspaceID: a.WorkspaceID, Action: "workspace.write"}}); err != nil {
		t.Fatal(err)
	}
	restrictedToken := authIssue(t, f, authRestricted, a.WorkspaceID, "workspace.write", time.Minute)
	restricted, err := f.auth.Authorize(ctx, restrictedToken, a.WorkspaceID, "workspace.write")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := opener.CommitRecord(ctx, restricted, ai); !errors.Is(err, workspace.ErrSaveKeyReuse) || got.Receipt != nil {
		t.Fatal("different actor got original receipt", got, err)
	}
	if err = f.auth.RevokeSession(ctx, operator, restrictedToken); err != nil {
		t.Fatal(err)
	}
	if got, err := opener.CommitRecord(ctx, restricted, ai); !errors.Is(err, ErrUnauthorized) || got.Receipt != nil {
		t.Fatal("revoked duplicate", got, err)
	}
	if err = opener.DrainWorkspace(ctx, closeA); err != nil {
		t.Fatal(err)
	}
	if got, err := opener.CommitRecord(ctx, writeA, ai); !errors.Is(err, workspace.ErrLifecycle) || got.Receipt != nil {
		t.Fatal("closed auto-reopened", got, err)
	}
	fresh, err := opener.OpenWorkspace(ctx, openA, 1)
	if err != nil || fresh.Epoch == av.Epoch {
		t.Fatal(fresh, err)
	}
	// Applied candidate, old epoch and stale baseline still retrieve original truth.
	stale := ai
	stale.PreviewVersion = 999
	got, err := opener.CommitRecord(ctx, writeA, stale)
	if err != nil || got.Status != workspace.SaveConfirmed || got.Receipt == nil || *got.Receipt != *ar.Receipt {
		t.Fatal("duplicate replayed/revalidated candidate", got, err)
	}
	stale.OperationID = authSameResource
	if got, err := opener.CommitRecord(ctx, writeA, stale); !errors.Is(err, workspace.ErrStaleHandle) || got.Receipt != nil {
		t.Fatal("old epoch granted fresh rights", got, err)
	}
	snap, err := opener.ReadSnapshot(ctx, readA)
	if err != nil || snap.Revision != 3 {
		t.Fatal(snap, err)
	}
	saved, err := opener.ReadRecord(ctx, readA, ap.Candidate.ResourceID)
	if err != nil || saved.Body != up.Candidate.Body || saved.Version != 2 || saved.ContentDigest != ur.Receipt.ContentDigest {
		t.Fatal(saved, err)
	}
	savedB, err := opener.ReadRecord(ctx, readB, bp.Candidate.ResourceID)
	if err != nil || savedB.Body != bp.Candidate.Body || savedB.Version != 1 {
		t.Fatal(savedB, err)
	}
	t.Log("real retained native leases + SQLite owner path, scoped same key, immutable preview/update, concurrent stale base, current actor/revocation and explicit close/reopen; not full Host process acceptance")
}

type commitFaultEvidence struct {
	RootA, RootB string
	Calls        int32
}

// The child exits with uncertain resources still owned. Never force-close the
// owner's gate/lease just to make cleanup look successful.
func TestWorkspaceCommitNativeFlushUnknownRetainsOwnership(t *testing.T) {
	const marker = "ABOARD_T019_COMMIT_FLUSH_CHILD"
	if os.Getenv(marker) == "" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		var count int32
		for ordinal := int32(0); ordinal == 0 || ordinal <= count; ordinal++ {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWorkspaceCommitNativeFlushUnknownRetainsOwnership$", "-test.count=1")
			cmd.Env = append(os.Environ(), marker+"="+fmt.Sprint(ordinal))
			out, err := cmd.CombinedOutput()
			cancel()
			if err != nil {
				t.Fatalf("ordinal %d: %v %s", ordinal, err, out)
			}
			var e commitFaultEvidence
			if err = json.Unmarshal(out, &e); err != nil {
				t.Fatalf("child evidence: %v %s", err, out)
			}
			if ordinal == 0 {
				count = e.Calls
				if count < 2 || count > 16 {
					t.Fatal("unexpected observed flush count", count)
				}
			}
			base, err := filepath.Abs(filepath.Join("..", "..", ".data", "logs", "001-validation"))
			if err != nil || !underLocation(base, e.RootA) || !underLocation(base, e.RootB) {
				t.Fatal("unscoped synthetic fixture")
			}
			for _, root := range []string{e.RootA, e.RootB} {
				lease, err := platform.Acquire(root, platform.Workspace, false, false)
				if err != nil {
					t.Fatal("child process exit did not release exclusion", err)
				}
				db, err := sqlite.OpenDatabase(context.Background(), filepath.Join(root, "data", "workspace.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
				if err != nil {
					lease.Close()
					t.Fatal(err)
				}
				if err = db.Close(); err != nil {
					t.Fatal(err)
				}
				h, err := sqlite.Open(context.Background(), filepath.Join(root, "data", "workspace.sqlite"), sqlite.ReadWriteExisting)
				if err != nil {
					t.Fatal(err)
				}
				var head int
				if err = h.Conn.QueryRowContext(context.Background(), "SELECT revision FROM workspace_head").Scan(&head); err != nil || head < 0 || head > 1 {
					t.Fatal(head, err)
				}
				if (ordinal == 0 || root == e.RootB) && head != 1 {
					t.Fatal("known save lost", head)
				}
				for _, table := range []string{"text_record", "text_version", "commit_receipt", "audit_entry"} {
					var n int
					if err = h.Conn.QueryRowContext(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil || n != head {
						t.Fatal("partial formal transaction", table, n, head, err)
					}
				}
				var bad int
				if err = h.Conn.QueryRowContext(context.Background(), "SELECT count(*) FROM candidate WHERE state!=CASE WHEN ?=1 THEN 'applied' ELSE 'previewed' END", head).Scan(&bad); err != nil || bad != 0 {
					t.Fatal("candidate result not atomic", bad, err)
				}
				if err = h.Close(); err != nil {
					t.Fatal(err)
				}
				if err = lease.Close(); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("real admission+formal COMMIT flush ordinal=%d/%d child physical lifetime fenced, B saved; process exited before inspection", ordinal, count)
		}
		return
	}
	ordinal64, err := strconv.ParseInt(os.Getenv(marker), 10, 32)
	if err != nil || ordinal64 < 0 {
		t.Fatal("invalid child ordinal")
	}
	ordinal := int32(ordinal64)
	f, c, operator, token, request := creationFixture(t)
	ctx := context.Background()
	a, err := c.CreateWorkspace(ctx, operator, request)
	if err != nil {
		t.Fatal(err)
	}
	second := request
	second.OperationID = authSameResource
	second.RootLocator = filepath.Join(f.allowed, "commit-fault-b")
	b, err := c.CreateWorkspace(ctx, operator, second)
	if err != nil {
		t.Fatal(err)
	}
	opener, err := NewWorkspaceOpener(f.auth)
	if err != nil {
		t.Fatal(err)
	}
	auth := func(ws, action string) *Authorization {
		t.Helper()
		a, err := f.auth.Authorize(ctx, token, ws, action)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	oa, ca, wa, ra := auth(a.WorkspaceID, "workspace.open"), auth(a.WorkspaceID, "workspace.close"), auth(a.WorkspaceID, "workspace.write"), auth(a.WorkspaceID, "workspace.read")
	ob, cb, wb := auth(b.WorkspaceID, "workspace.open"), auth(b.WorkspaceID, "workspace.close"), auth(b.WorkspaceID, "workspace.write")
	av, err := opener.OpenWorkspace(ctx, oa, 1)
	if err != nil {
		t.Fatal(err)
	}
	bv, err := opener.OpenWorkspace(ctx, ob, 1)
	if err != nil {
		t.Fatal(err)
	}
	ap, err := opener.PreviewRecord(ctx, wa, workspace.PreviewInput{Epoch: av.Epoch, Kind: "create-text", Title: "A", Body: "one real attempt"})
	if err != nil {
		t.Fatal(err)
	}
	bp, err := opener.PreviewRecord(ctx, wb, workspace.PreviewInput{Epoch: bv.Epoch, Kind: "create-text", Title: "B", Body: "B survives uncertain A"})
	if err != nil {
		t.Fatal(err)
	}
	stimulusPath := filepath.Join(f.root, "synthetic-readonly-flush")
	if err = os.WriteFile(stimulusPath, []byte("synthetic native fault stimulus"), 0600); err != nil {
		t.Fatal(err)
	}
	stimulus, err := os.Open(stimulusPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.FlushFileBuffers(windows.Handle(stimulus.Fd())); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatal(err)
	}
	creationFlushFault.handle = stimulus.Fd()
	creationFlushFault.at = ordinal
	creationFlushFault.calls.Store(0)
	creationFlushFault.failed.Store(0)
	restore := replaceCreationFlush(t)
	in := commitPreviewInput(authA, ap)
	result, saveErr := opener.CommitRecord(ctx, wa, in)
	calls, failed := creationFlushFault.calls.Load(), creationFlushFault.failed.Load()
	restore()
	if err = stimulus.Close(); err != nil {
		t.Fatal(err)
	}
	if ordinal == 0 {
		if saveErr != nil || result.Status != workspace.SaveConfirmed || result.Receipt == nil {
			t.Fatal(result, saveErr)
		}
	} else {
		var native *driver.Error
		if result.Status != workspace.SaveUnknown || result.Receipt != nil || !errors.Is(saveErr, workspace.ErrOpenUncertain) || !errors.As(saveErr, &native) || native.Code() != sqlite3.SQLITE_IOERR_FSYNC || failed != 1 {
			t.Fatal(result, saveErr, calls, failed)
		}
		if snapshot, err := opener.ReadSnapshot(ctx, ra); !errors.Is(err, workspace.ErrLifecycle) || snapshot.Identity.WorkspaceID != "" || snapshot.Revision != 0 {
			t.Fatal("unknown retained read view", snapshot, err)
		}
		again, err := opener.CommitRecord(ctx, wa, in)
		if again.Status != workspace.SaveUnknown && again.Status != workspace.SaveConfirmed {
			t.Fatal("same key not investigated", again, err)
		}
		newKey := in
		newKey.OperationID = authB
		if got, err := opener.CommitRecord(ctx, wa, newKey); got.Status != workspace.SaveUnknown || !errors.Is(err, workspace.ErrOpenUncertain) {
			t.Fatal("unknown granted new write rights", got, err)
		}
		if err = opener.DrainWorkspace(ctx, ca); !errors.Is(err, workspace.ErrOpenUncertain) {
			t.Fatal("unknown closed", err)
		}
		if err = f.store.Close(); !errors.Is(err, ErrWorkspaceOpenPending) {
			t.Fatal("unknown installation lock released", err)
		}
		if l, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false); err == nil {
			l.Close()
			t.Fatal("unknown physical lock released")
		}
	}
	if result, err := opener.CommitRecord(ctx, wb, commitPreviewInput(authA, bp)); err != nil || result.Status != workspace.SaveConfirmed {
		t.Fatal("B blocked by A", result, err)
	}
	if err = opener.DrainWorkspace(ctx, cb); err != nil {
		t.Fatal(err)
	}
	if ordinal != 0 && os.Getenv("ABOARD_T021_CLOSE") == "1" {
		closeCommitFaultChild(t, opener, &f, a.WorkspaceID, av.Epoch, ca, auth(a.WorkspaceID, "workspace.operator"), in)
	}
	if ordinal != 0 && os.Getenv("ABOARD_T020_RECONCILE") == "1" {
		reconcileCommitFaultChild(t, opener, &f, a.WorkspaceID, oa, ca, wa, ra, auth(a.WorkspaceID, "workspace.operator"), in)
	}
	if ordinal == 0 {
		if err = opener.DrainWorkspace(ctx, ca); err != nil {
			t.Fatal(err)
		}
		if err = f.store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err = json.NewEncoder(os.Stdout).Encode(commitFaultEvidence{request.RootLocator, second.RootLocator, calls}); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestWorkspaceCommitCanceledWaiterAfterAdmissionFinishesRealCommit(t *testing.T) {
	f, c, operator, token, request := creationFixture(t)
	ctx := context.Background()
	created, err := c.CreateWorkspace(ctx, operator, request)
	if err != nil {
		t.Fatal(err)
	}
	opener, err := NewWorkspaceOpener(f.auth)
	if err != nil {
		t.Fatal(err)
	}
	auth := func(action string) *Authorization {
		t.Helper()
		a, err := f.auth.Authorize(ctx, token, created.WorkspaceID, action)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	oa, ca, wa, ra := auth("workspace.open"), auth("workspace.close"), auth("workspace.write"), auth("workspace.read")
	view, err := opener.OpenWorkspace(ctx, oa, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opener.DrainWorkspace(ctx, ca); err != nil {
			t.Error(err)
		}
	}()
	p, err := opener.PreviewRecord(ctx, wa, workspace.PreviewInput{Epoch: view.Epoch, Kind: "create-text", Title: "canceled delivery", Body: "formal body survives caller cancellation\r\n"})
	if err != nil {
		t.Fatal(err)
	}
	waiter, cancel := context.WithCancel(ctx)
	defer cancel()
	// Fixed profile admission COMMIT has 3 successful native flushes. Cancel
	// inside the first actual FORMAL COMMIT flush, after durable admission.
	creationFlushFault.calls.Store(0)
	creationFlushFault.failed.Store(0)
	creationFlushFault.at = 0
	canceledAt := int32(0)
	creationFlushFault.afterFlush = func(n int32) {
		if n == 4 {
			canceledAt = n
			cancel()
		}
	}
	restore := replaceCreationFlush(t)
	defer func() { restore(); creationFlushFault.afterFlush = nil }()
	in := commitPreviewInput(authA, p)
	result, saveErr := opener.CommitRecord(waiter, wa, in)
	calls := creationFlushFault.calls.Load()
	restore()
	creationFlushFault.afterFlush = nil
	if canceledAt != 4 || calls != 6 || result.Status != workspace.SaveConfirmed || result.Receipt == nil || !result.ViewDeliveryFailed || !errors.Is(saveErr, context.Canceled) || !errors.Is(saveErr, workspace.ErrSaveViewDelivery) {
		t.Fatal(canceledAt, calls, result, saveErr)
	}
	current, err := opener.ReadRecord(ctx, ra, p.Candidate.ResourceID)
	if err != nil || current.Version != 1 || current.Revision != 1 || current.Body != p.Candidate.Body {
		t.Fatal(current, err)
	}
	duplicate, err := opener.CommitRecord(ctx, wa, in)
	if err != nil || duplicate.Status != workspace.SaveConfirmed || duplicate.Receipt == nil || *duplicate.Receipt != *result.Receipt || duplicate.ViewDeliveryFailed {
		t.Fatal("delivery failure became persisted rollback/replay", duplicate, err)
	}
	t.Log("caller canceled inside real native formal COMMIT flush after durable admission; attempt completed once, confirmed truth separate from failed delivery, healthy same-key inquiry and physical drain")
}
