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
	"sync"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestWorkspaceCloseRealFullQueueNativeIOAndIndependentB(t *testing.T) {
	f, creator, operator, token, request := creationFixture(t)
	ctx := context.Background()
	a, err := creator.CreateWorkspace(ctx, operator, request)
	if err != nil {
		t.Fatal(err)
	}
	bRequest := request
	bRequest.OperationID = authB
	bRequest.RootLocator = filepath.Join(f.allowed, "close-B")
	b, err := creator.CreateWorkspace(ctx, operator, bRequest)
	if err != nil {
		t.Fatal(err)
	}
	opener, err := NewWorkspaceOpener(f.auth)
	if err != nil {
		t.Fatal(err)
	}
	authorize := func(ws, action string) *Authorization {
		t.Helper()
		v, err := f.auth.Authorize(ctx, token, ws, action)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	ao, ac, aw, ar := authorize(a.WorkspaceID, "workspace.open"), authorize(a.WorkspaceID, "workspace.close"), authorize(a.WorkspaceID, "workspace.write"), authorize(a.WorkspaceID, "workspace.read")
	bo, bw, br := authorize(b.WorkspaceID, "workspace.open"), authorize(b.WorkspaceID, "workspace.write"), authorize(b.WorkspaceID, "workspace.read")
	av, err := opener.OpenWorkspace(ctx, ao, 1)
	if err != nil {
		t.Fatal(err)
	}
	bv, err := opener.OpenWorkspace(ctx, bo, 1)
	if err != nil {
		t.Fatal(err)
	}
	ap, err := opener.PreviewRecord(ctx, aw, workspace.PreviewInput{Epoch: av.Epoch, Kind: "create-text", Title: "A long native IO", Body: "A confirmed after close cutoff\r\n"})
	if err != nil {
		t.Fatal(err)
	}
	bp, err := opener.PreviewRecord(ctx, bw, workspace.PreviewInput{Epoch: bv.Epoch, Kind: "create-text", Title: "B independent", Body: "B saves while A native callback is still live"})
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	creationFlushFault.calls.Store(0)
	creationFlushFault.failed.Store(0)
	creationFlushFault.at = 0
	creationFlushFault.afterFlush = func(n int32) {
		if n == 4 {
			close(started)
			<-release
		}
	}
	restore := replaceCreationFlush(t)
	waiter, cancel := context.WithCancel(ctx)
	defer cancel()
	type saveAnswer struct {
		result workspace.SaveResult
		err    error
	}
	answer := make(chan saveAnswer, 1)
	go func() {
		result, err := opener.CommitRecord(waiter, aw, commitPreviewInput(authA, ap))
		answer <- saveAnswer{result, err}
	}()
	saved := false
	defer func() {
		releaseOnce.Do(func() { close(release) })
		if !saved {
			<-answer
		}
		restore()
		creationFlushFault.afterFlush = nil
		cleanup, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		if err := opener.ShutdownWorkspaces(cleanup); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("formal native flush not reached")
	}
	cancel()
	queued := make(chan error, 30)
	for i := 0; i < 30; i++ {
		go func() { _, err := opener.ReadSnapshot(ctx, ar); queued <- err }()
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for f.store.workspaceCapacity.Usage() != 31 {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("A queue did not fill", f.store.workspaceCapacity.Usage())
		}
	}
	if _, err = opener.ReadSnapshot(ctx, ar); !errors.Is(err, workspace.ErrCapacity) {
		t.Fatal("full A was not rejected before IO", err)
	}
	// A still owns its live native callback. B uses its own actual connection and
	// native flushes, not a fake executor, while A's queue remains at its bound.
	bSaved, err := opener.CommitRecord(ctx, bw, commitPreviewInput(authA, bp))
	if err != nil || bSaved.Status != workspace.SaveConfirmed {
		t.Fatal("B blocked by A", bSaved, err)
	}
	if current, err := opener.ReadRecord(ctx, br, bp.Candidate.ResourceID); err != nil || current.Body != bp.Candidate.Body {
		t.Fatal(current, err)
	}
	closing, err := opener.CloseWorkspace(ctx, ac, authB, av.Epoch)
	if err != nil || closing.Closed || closing.State != workspace.LifecycleDraining {
		t.Fatal(closing, err)
	}
	for i := 0; i < 30; i++ {
		select {
		case err := <-queued:
			if !errors.Is(err, workspace.ErrLifecycle) {
				t.Fatal("queued work crossed close", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("queued waiter not rejected")
		}
	}
	if _, err = opener.CommitRecord(ctx, aw, commitPreviewInput(authA, ap)); !errors.Is(err, workspace.ErrLifecycle) {
		t.Fatal("new commit after close", err)
	}
	waiting, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	status, waitErr := opener.WaitClose(waiting, ac, authB)
	stop()
	if !errors.Is(waitErr, context.DeadlineExceeded) || status.Closed || status.State != workspace.LifecycleDraining {
		t.Fatal(status, waitErr)
	}
	if lease, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false); err == nil {
		lease.Close()
		t.Fatal("wait timeout released live native writer")
	}
	if err = f.store.Close(); !errors.Is(err, ErrWorkspaceOpenPending) {
		t.Fatal("installation lock released before workspace drain", err)
	}
	releaseOnce.Do(func() { close(release) })
	got := <-answer
	saved = true
	if got.result.Status != workspace.SaveConfirmed || !got.result.ViewDeliveryFailed || !errors.Is(got.err, context.Canceled) {
		t.Fatal(got)
	}
	waiting, stop = context.WithTimeout(ctx, 5*time.Second)
	closed, err := opener.WaitClose(waiting, ac, authB)
	stop()
	if err != nil || !closed.Closed {
		t.Fatal(closed, err)
	}
	lease, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false)
	if err != nil {
		t.Fatal("closed did not physically release writer", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := opener.OpenWorkspace(ctx, ao, 1)
	if err != nil || reopened.Epoch == av.Epoch {
		t.Fatal(reopened, err)
	}
	current, err := opener.ReadRecord(ctx, ar, ap.Candidate.ResourceID)
	if err != nil || current.Body != ap.Candidate.Body || current.Revision != 1 {
		t.Fatal(current, err)
	}
	original, err := opener.CloseWorkspace(ctx, ac, authB, av.Epoch)
	if err != nil || !original.Closed {
		t.Fatal("old close lost original history", original, err)
	}
	if _, err = opener.ReadRecord(ctx, ar, ap.Candidate.ResourceID); err != nil {
		t.Fatal("old close key closed new epoch", err)
	}
	if _, err = opener.CloseWorkspace(ctx, ac, authA, av.Epoch); !errors.Is(err, workspace.ErrStaleHandle) {
		t.Fatal("old epoch acquired new close", err)
	}
	t.Log("real native formal COMMIT held after durable admission: full A rejects pre-IO, B commits/reads independently, canceled waiter/close timeout retain writer+installation locks, actual completion closes then explicit fresh epoch preserves confirmed bytes; full Host/harness V013/V014 still separate")
}

func closeCommitFaultChild(t *testing.T, opener *WorkspaceOpener, f *authFixture, ws, epoch string, ca, operator *Authorization, in workspace.SaveInput) {
	t.Helper()
	ctx := context.Background()
	if usage := f.store.workspaceCapacity.Usage(); usage != 1 {
		t.Fatal("unknown polling duplicated physical capacity", usage)
	}
	closing, err := opener.CloseWorkspace(ctx, ca, authSameResource, epoch)
	if err != nil || closing.Closed {
		t.Fatal(closing, err)
	}
	waiting, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	view, err := opener.WaitClose(waiting, ca, authSameResource)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) || view.Closed {
		t.Fatal("unknown close claimed safety", view, err)
	}
	// Investigation uses the reserved control path despite close's ordinary cutoff.
	evidence, err := opener.ReconcileWorkspaceOperation(ctx, operator, in.OperationID)
	if evidence.Result.Status == workspace.SaveUnknown {
		if !errors.Is(err, workspace.ErrOperationHistoryUnavailable) {
			t.Fatal(evidence, err)
		}
		if usage := f.store.workspaceCapacity.Usage(); usage != 1 {
			t.Fatal("unresolved original slot released", usage)
		}
		if err := f.store.Close(); !errors.Is(err, ErrWorkspaceOpenPending) {
			t.Fatal(err)
		}
		return
	}
	if err != nil || !evidence.PhysicalEnded || !evidence.RequiresReopen {
		t.Fatal(evidence, err)
	}
	waiting, cancel = context.WithTimeout(ctx, 5*time.Second)
	view, err = opener.WaitClose(waiting, ca, authSameResource)
	cancel()
	if err != nil || !view.Closed || f.store.workspaceCapacity.Usage() != 0 {
		t.Fatal("explicit known resolution did not wake physical close", view, err, f.store.workspaceCapacity.Usage())
	}
	_ = ws
}

func TestWorkspaceCloseUnknownNativeFaultInvestigationRetainsCapacity(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for ordinal := 1; ordinal <= 6; ordinal++ {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWorkspaceCommitNativeFlushUnknownRetainsOwnership$", "-test.count=1")
		cmd.Env = append(os.Environ(), "ABOARD_T019_COMMIT_FLUSH_CHILD="+fmt.Sprint(ordinal), "ABOARD_T021_CLOSE=1")
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("ordinal %d: %v %s", ordinal, err, out)
		}
		var evidence commitFaultEvidence
		if err = json.Unmarshal(out, &evidence); err != nil {
			t.Fatal(err, string(out))
		}
		base, err := filepath.Abs(filepath.Join("..", "..", ".data", "logs", "001-validation"))
		if err != nil || !underLocation(base, evidence.RootA) || !underLocation(base, evidence.RootB) {
			t.Fatal("unscoped synthetic child")
		}
		for _, root := range []string{evidence.RootA, evidence.RootB} {
			lease, err := platform.Acquire(root, platform.Workspace, false, false)
			if err != nil {
				t.Fatal("completed child still owns writer", err)
			}
			if err = lease.Close(); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("native flush ordinal %d: unknown close retained original capacity/ownership; explicit control investigation resolved only proven admission, missing admission remained unknown until process exit", ordinal)
	}
}
