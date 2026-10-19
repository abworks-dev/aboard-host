//go:build windows && amd64

package installation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	win "golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestWorkspaceReconcileCurrentActorAndPositivePendingReceipt(t *testing.T) {
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
	oa, ca, wa, ra, investigator := auth("workspace.open"), auth("workspace.close"), auth("workspace.write"), auth("workspace.read"), auth("workspace.operator")
	v, err := opener.OpenWorkspace(ctx, oa, 1)
	if err != nil {
		t.Fatal(err)
	}
	if missing, err := opener.GetWorkspaceOperation(ctx, ra, authB); !errors.Is(err, workspace.ErrOperationHistoryUnavailable) || missing.Result.Status != workspace.SaveUnknown {
		t.Fatal(missing, err)
	}
	// Unknown random key did not fence healthy writing.
	p, err := opener.PreviewRecord(ctx, wa, workspace.PreviewInput{Epoch: v.Epoch, Kind: "create-text", Title: "original actor", Body: "receipt survives retained uncertainty"})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := opener.CommitRecord(ctx, wa, commitPreviewInput(authA, p))
	if err != nil || saved.Receipt == nil {
		t.Fatal(saved, err)
	}
	own, err := opener.GetWorkspaceOperation(ctx, ra, authA)
	if err != nil || own.Result.Receipt == nil || *own.Result.Receipt != *saved.Receipt {
		t.Fatal(own, err)
	}
	if _, err := opener.ReconcileWorkspaceOperation(ctx, wa, authA); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("workspace.write gained operator investigation", err)
	}
	if err = f.auth.SetPrincipalAuthorization(ctx, operator, authRestricted, false, []sqlite.WorkspaceGrant{{WorkspaceID: created.WorkspaceID, Action: "workspace.read"}, {WorkspaceID: created.WorkspaceID, Action: "workspace.operator"}}); err != nil {
		t.Fatal(err)
	}
	restrictedToken := authIssue(t, f, authRestricted, created.WorkspaceID, "workspace.read", time.Minute)
	restricted, err := f.auth.Authorize(ctx, restrictedToken, created.WorkspaceID, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	if other, err := opener.GetWorkspaceOperation(ctx, restricted, authA); !errors.Is(err, workspace.ErrOperationAccess) || other.Result.Receipt != nil {
		t.Fatal("foreign read actor learned receipt", other, err)
	}
	explicitOperator, err := f.auth.Authorize(ctx, restrictedToken, created.WorkspaceID, "workspace.operator")
	if err != nil {
		t.Fatal(err)
	}
	if other, err := opener.GetWorkspaceOperation(ctx, explicitOperator, authA); err != nil || other.Result.Receipt == nil {
		t.Fatal("current explicit operator denied", other, err)
	}
	if err = f.auth.RevokeSession(ctx, operator, restrictedToken); err != nil {
		t.Fatal(err)
	}
	if other, err := opener.GetWorkspaceOperation(ctx, explicitOperator, authA); !errors.Is(err, ErrUnauthorized) || other.Result.Receipt != nil {
		t.Fatal("stale operator permission leaked receipt", other, err)
	}
	// Explicit state injection reproduces a retained delivery/physical uncertainty
	// AFTER a genuine confirmed transaction. It is not a native failure claim.
	if err = opener.DrainWorkspace(ctx, ca); err != nil {
		t.Fatal(err)
	}
	scope, err := ra.Scope()
	if err != nil {
		t.Fatal(err)
	}
	lease, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	descriptorPin, err := lease.PinFile("aboard.workspace.json", true)
	if err != nil {
		t.Fatal(err)
	}
	databasePin, err := lease.PinFile(`data\workspace.sqlite`, false)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sqlite.OpenDatabase(ctx, filepath.Join(request.RootLocator, "data", "workspace.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
	if err != nil {
		t.Fatal(err)
	}
	gate, err := sqlite.NewCommitGate(database, authA)
	if err != nil {
		t.Fatal(err)
	}
	retained := &openResources{store: f.store, scope: scope, pending: true, lease: lease, descriptorPin: descriptorPin, databasePin: databasePin, database: database, gate: gate}
	if err = retained.Close(); !errors.Is(err, workspace.ErrOpenUncertain) {
		t.Fatal("uncertain resources closed", err)
	}
	check := func(ctx context.Context) error { return investigator.CheckAction(ctx, scope, "workspace.operator") }
	positive, positiveErr := retained.ReconcileSaveOperation(ctx, scope, authA, ra.principalID, check)
	if positiveErr != nil || positive.Result.Status != workspace.SaveConfirmed || retained.pending || !positive.RequiresReopen || !positive.PhysicalEnded {
		t.Fatal("positive receipt skipped pending physical resolution", positive, positiveErr)
	}
	// A failed preflight necessarily occurs after the prior source connection has
	// ended. Reproduce that exact state, not an impossible freeze of a live writer.
	if err = retained.database.Close(); err != nil {
		t.Fatal(err)
	}
	retained.database = nil
	retained.investigationView = false
	retained.pending = true
	// A previous interrupted preflight retains exact temporary objects. A real
	// delete-denying handle makes its first cleanup fail, then retry must close
	// these objects (not replace and leak them) before acquiring a fresh view.
	oldInputs, err := lease.FreezeWorkspaceOpenInputs()
	if err != nil {
		t.Fatal(err)
	}
	oldScratch, err := platform.NewScratch(f.store.root, "inspect-"+strings.Repeat("a", 48))
	if err != nil {
		t.Fatal(err)
	}
	out, err := oldScratch.CreateFile("workspace.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
	name, err := win.UTF16PtrFromString(filepath.Join(oldScratch.Path(), "workspace.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := win.CreateFile(name, win.GENERIC_READ, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE, nil, win.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	retained.recoveryInputs, retained.recoveryScratch = oldInputs, oldScratch
	first, firstErr := retained.ReconcileSaveOperation(ctx, scope, authA, ra.principalID, check)
	if firstErr == nil || first.Result.Status != workspace.SaveUnknown || retained.recoveryInputs != oldInputs || retained.recoveryScratch != oldScratch {
		win.CloseHandle(blocker)
		t.Fatal("failed cleanup discarded original temporary ownership", first, firstErr)
	}
	if err = win.CloseHandle(blocker); err != nil {
		t.Fatal(err)
	}
	resolved, err := retained.ReconcileSaveOperation(ctx, scope, authA, ra.principalID, check)
	if _, err := oldInputs.Database.FrozenInputSize(); err == nil {
		t.Fatal("retry leaked original frozen source handle")
	}
	if _, err := os.Stat(oldScratch.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("retry leaked original scratch", err)
	}
	if err != nil || resolved.Result.Status != workspace.SaveConfirmed || resolved.Result.Receipt == nil || !resolved.RequiresReopen || !resolved.PhysicalEnded {
		t.Fatal("positive receipt failed to resolve pending physical owner", resolved, err)
	}
	if _, err = opener.ReadSnapshot(ctx, ra); !errors.Is(err, workspace.ErrLifecycle) {
		t.Fatal("fresh investigation silently reopened ordinary view", err)
	}
	if retained.pending {
		t.Fatal("positive receipt left pending forever")
	}
	if err = retained.Close(); err != nil {
		t.Fatal("resolved physical owner cannot drain", err)
	}
	if closed, err := opener.GetWorkspaceOperation(ctx, ra, authA); !errors.Is(err, workspace.ErrLifecycle) || closed.Result.Receipt != nil {
		t.Fatal("query implicitly opened closed directory", closed, err)
	}
	next, err := opener.OpenWorkspace(ctx, oa, 1)
	if err != nil || next.Epoch == v.Epoch {
		t.Fatal(next, err)
	}
	duplicate, err := opener.CommitRecord(ctx, wa, commitPreviewInput(authA, p))
	if err != nil || duplicate.Receipt == nil || *duplicate.Receipt != *saved.Receipt {
		t.Fatal("reconciliation replayed original key", duplicate, err)
	}
	if err = opener.DrainWorkspace(ctx, ca); err != nil {
		t.Fatal(err)
	}
}

// Native fault children reuse T019's real FlushFileBuffers stimulus and actual
// A/B business owner, adding only explicit investigation AFTER retention checks.
func TestWorkspaceReconcileNativeFlushHistory(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for ordinal := 1; ordinal <= 6; ordinal++ {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWorkspaceCommitNativeFlushUnknownRetainsOwnership$", "-test.count=1")
		cmd.Env = append(os.Environ(), fmt.Sprintf("ABOARD_T019_COMMIT_FLUSH_CHILD=%d", ordinal), "ABOARD_T020_RECONCILE=1")
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("ordinal %d: %v %s", ordinal, err, out)
		}
		var e commitFaultEvidence
		if err = json.Unmarshal(out, &e); err != nil {
			t.Fatalf("evidence %v %s", err, out)
		}
		base, err := filepath.Abs(filepath.Join("..", "..", ".data", "logs", "001-validation"))
		if err != nil || !underLocation(base, e.RootA) || !underLocation(base, e.RootB) {
			t.Fatal("unscoped fixture", err)
		}
		for _, root := range []string{e.RootA, e.RootB} {
			lease, err := platform.Acquire(root, platform.Workspace, false, false)
			if err != nil {
				t.Fatal(err)
			}
			d, err := sqlite.OpenDatabase(context.Background(), filepath.Join(root, "data", "workspace.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
			if err != nil {
				t.Fatal(err)
			}
			if err = d.Close(); err != nil {
				t.Fatal(err)
			}
			h, err := sqlite.Open(context.Background(), filepath.Join(root, "data", "workspace.sqlite"), sqlite.ReadWriteExisting)
			if err != nil {
				t.Fatal(err)
			}
			var head int
			if err = h.Conn.QueryRowContext(context.Background(), "SELECT revision FROM workspace_head").Scan(&head); err != nil || head < 0 || head > 2 {
				t.Fatal(head, err)
			}
			for _, table := range []string{"text_record", "text_version", "commit_receipt", "audit_entry"} {
				var n int
				if err = h.Conn.QueryRowContext(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil || n != head {
					t.Fatal("partial/replayed formal facts", table, n, head, err)
				}
			}
			if err = h.Close(); err != nil {
				t.Fatal(err)
			}
			if err = lease.Close(); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("native ordinal %d: unknown retained, explicit same-history investigation after physical end; missing admission stayed unknown, positive admission resolved without replay", ordinal)
	}
}

func reconcileCommitFaultChild(t *testing.T, opener *WorkspaceOpener, f *authFixture, workspaceID string, oa, ca, wa, ra, operator *Authorization, in workspace.SaveInput) {
	t.Helper()
	ctx := context.Background()
	resolved, err := opener.ReconcileWorkspaceOperation(ctx, operator, in.OperationID)
	if resolved.Result.Status == workspace.SaveUnknown {
		if !errors.Is(err, workspace.ErrOperationHistoryUnavailable) {
			t.Fatal("unexpected unavailable investigation", resolved, err)
		}
		if err = opener.DrainWorkspace(ctx, ca); !errors.Is(err, workspace.ErrOpenUncertain) {
			t.Fatal("missing admission invented rollback/drained", err)
		}
		if err = f.store.Close(); !errors.Is(err, ErrWorkspaceOpenPending) {
			t.Fatal("unknown shared install released", err)
		}
		return
	}
	if err != nil || (resolved.Result.Status != workspace.SaveNotCommitted && resolved.Result.Status != workspace.SaveConfirmed) || !resolved.RequiresReopen || !resolved.PhysicalEnded {
		t.Fatal("fresh resolution", resolved, err)
	}
	current, err := opener.GetWorkspaceOperation(ctx, ra, in.OperationID)
	if err != nil || current.Result.Status != resolved.Result.Status {
		t.Fatal("lost investigated result", current, err)
	}
	if err = opener.DrainWorkspace(ctx, ca); err != nil {
		t.Fatal("resolved owner could not physically drain", err)
	}
	v, err := opener.OpenWorkspace(ctx, oa, 1)
	if err != nil || v.Epoch == in.Epoch {
		t.Fatal("resolved operation poisoned explicit reopen", v, err)
	}
	old, err := opener.CommitRecord(ctx, wa, in)
	if err != nil || old.Status != resolved.Result.Status {
		t.Fatal("old-key replay/reclassification", old, err)
	}
	p, err := opener.PreviewRecord(ctx, wa, workspace.PreviewInput{Epoch: v.Epoch, Kind: "create-text", BaseRevision: v.Snapshot.Revision, Title: "new explicit preview", Body: "new operation after proven resolution"})
	if err != nil {
		t.Fatal(p, err)
	}
	saved, err := opener.CommitRecord(ctx, wa, commitPreviewInput(authB, p))
	if err != nil || saved.Status != workspace.SaveConfirmed {
		t.Fatal("known non-commit blocked all future work", saved, err)
	}
	if err = opener.DrainWorkspace(ctx, ca); err != nil {
		t.Fatal(err)
	}
	_ = workspaceID
}

// A real durable admission without a formal transaction is encountered by a new
// physical open, not just by a previously running/fenced CommitGate. This is a
// local cold-resource test; the full Host/new-process acceptance remains T027.
func TestWorkspaceReconcileColdAcceptedHistory(t *testing.T) {
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
	oa, ca, wa, ra, investigator := auth("workspace.open"), auth("workspace.close"), auth("workspace.write"), auth("workspace.read"), auth("workspace.operator")
	v, err := opener.OpenWorkspace(ctx, oa, 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := opener.PreviewRecord(ctx, wa, workspace.PreviewInput{Epoch: v.Epoch, Kind: "create-text", Title: "cold accepted", Body: "must never replay"})
	if err != nil {
		t.Fatal(err)
	}
	if err = opener.DrainWorkspace(ctx, ca); err != nil {
		t.Fatal(err)
	}
	// Fixture writes ONLY the actual durable admission under the real exclusive
	// lease, using the production gate; the formal transaction is never invoked.
	lease, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.OpenDatabase(ctx, filepath.Join(request.RootLocator, "data", "workspace.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
	if err != nil {
		t.Fatal(err)
	}
	gate, err := sqlite.NewCommitGate(db, v.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := wa.Scope()
	if err != nil {
		t.Fatal(err)
	}
	in := commitPreviewInput(authA, p)
	result, admission, err := gate.Admit(ctx, sqlite.SaveRequest{InstallationID: scope.InstallationID, TenantID: scope.TenantID, WorkspaceID: scope.WorkspaceID, PrincipalID: wa.Principal().PrincipalID, OperationID: in.OperationID, CandidateID: in.CandidateID, CandidateDigest: in.CandidateDigest, Epoch: in.Epoch, PreviewVersion: in.PreviewVersion}, time.Now().UTC())
	if err != nil || result.Status != sqlite.Accepted || admission == nil {
		t.Fatal(result, admission, err)
	}
	if err = gate.EndPhysical(); err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	cold, err := opener.OpenWorkspace(ctx, oa, 1)
	if !errors.Is(err, workspace.ErrOpenUncertain) || cold.Epoch != "" {
		t.Fatal("cold pending history published an ordinary view", cold, err)
	}
	if err = f.store.Close(); !errors.Is(err, ErrWorkspaceOpenPending) {
		t.Fatal("cold pending released installation ownership", err)
	}
	queried, err := opener.GetWorkspaceOperation(ctx, ra, in.OperationID)
	if err != nil || queried.Result.Status != workspace.SaveUnknown {
		t.Fatal(queried, err)
	}
	resolved, err := opener.ReconcileWorkspaceOperation(ctx, investigator, in.OperationID)
	if err != nil || resolved.Result.Status != workspace.SaveNotCommitted || !resolved.PhysicalEnded || !resolved.RequiresReopen {
		t.Fatal("cold accepted history could not be proven without replay", resolved, err)
	}
	if err = opener.DrainWorkspace(ctx, ca); err != nil {
		t.Fatal(err)
	}
	reopened, err := opener.OpenWorkspace(ctx, oa, 1)
	if err != nil || reopened.Epoch == v.Epoch || reopened.Snapshot.Revision != 0 {
		t.Fatal("investigation changed formal history", reopened, err)
	}
	old, err := opener.CommitRecord(ctx, wa, in)
	if err != nil || old.Status != workspace.SaveNotCommitted || old.Receipt != nil {
		t.Fatal("old accepted key replayed", old, err)
	}
	if err = opener.DrainWorkspace(ctx, ca); err != nil {
		t.Fatal(err)
	}
}
