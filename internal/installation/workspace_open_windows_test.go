//go:build windows && amd64

package installation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// The child intentionally retains uncertain resources until process exit. It
// must not use a test-only force-close to manufacture a successful drain.
func TestWorkspaceOpenColdPendingFence(t *testing.T) {
	if os.Getenv("ABOARD_T016_PENDING_CHILD") != "1" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWorkspaceOpenColdPendingFence$", "-test.count=1")
		cmd.Env = append(os.Environ(), "ABOARD_T016_PENDING_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("pending child %v: %s", err, out)
		}
		var evidence struct {
			Root   string
			Before map[string]string
		}
		if err = json.Unmarshal(out, &evidence); err != nil {
			t.Fatal("child evidence", err)
		}
		base, err := filepath.Abs(filepath.Join("..", "..", ".data", "logs", "001-validation"))
		if err != nil || !underLocation(base, evidence.Root) {
			t.Fatal("unscoped synthetic evidence root")
		}
		// The live exclusive lock cannot be byte-read. Inspect the ENTIRE source
		// only after the child really exited, rather than skipping that file.
		if !reflect.DeepEqual(evidence.Before, inspectionTree(t, evidence.Root)) {
			t.Fatal("open replayed/changed pending save")
		}
		lease, err := platform.Acquire(evidence.Root, platform.Workspace, false, false)
		if err != nil {
			t.Fatal("process exit did not release lock", err)
		}
		if err = lease.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, c, operator, token, r := creationFixture(t)
	ctx := context.Background()
	created, err := c.CreateWorkspace(ctx, operator, r)
	if err != nil {
		t.Fatal(err)
	}
	inspectionSQL(t, r.RootLocator, `INSERT INTO candidate SELECT workspace_id,'synthetic-candidate','synthetic-principal','synthetic-resource','create-text',0,NULL,'synthetic','body',printf('%064d',0),printf('%064d',0),1,'synthetic','2099-01-01T00:00:00Z','previewed' FROM workspace_identity;
 INSERT INTO save_operation SELECT workspace_id,'synthetic-operation','synthetic-principal','synthetic-candidate',printf('%064d',0),'synthetic','accepted',NULL FROM workspace_identity;`)
	a, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	closeAuth, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.close")
	if err != nil {
		t.Fatal(err)
	}
	opener, _ := NewWorkspaceOpener(f.auth)
	before := inspectionTree(t, r.RootLocator)
	view, err := opener.OpenWorkspace(ctx, a, 1)
	if !errors.Is(err, workspace.ErrOpenUncertain) || view.Epoch != "" {
		t.Fatalf("pending open %+v %v", view, err)
	}
	if err = opener.DrainWorkspace(ctx, closeAuth); !errors.Is(err, workspace.ErrOpenUncertain) {
		t.Fatal("unknown treated as drained", err)
	}
	if err = f.store.Close(); !errors.Is(err, ErrWorkspaceOpenPending) {
		t.Fatal("installation released uncertainty", err)
	}
	if lease, err := platform.Acquire(r.RootLocator, platform.Workspace, false, false); err == nil {
		lease.Close()
		t.Fatal("uncertain writer released")
	}
	if _, err = opener.OpenWorkspace(ctx, a, 1); !errors.Is(err, workspace.ErrLifecycle) {
		t.Fatal("uncertainty reset via reopen", err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(struct {
		Root   string
		Before map[string]string
	}{r.RootLocator, before}); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestWorkspaceOpenExistingScopeEpochAndDrain(t *testing.T) {
	f, c, operator, token, r := creationFixture(t)
	defer f.store.Close()
	ctx := context.Background()
	created, err := c.CreateWorkspace(ctx, operator, r)
	if err != nil {
		t.Fatal(err)
	}
	openAuth, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.open")
	if err != nil {
		t.Fatal(err)
	}
	closeAuth, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.close")
	if err != nil {
		t.Fatal(err)
	}
	opener, _ := NewWorkspaceOpener(f.auth)
	if _, err = opener.OpenWorkspace(ctx, openAuth, 2); err == nil {
		t.Fatal("stale registration accepted")
	}
	before := inspectionTree(t, r.RootLocator)
	first, err := opener.OpenWorkspace(ctx, openAuth, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Epoch == "" || first.NeedsInvestigation || first.Snapshot.Revision != 0 || first.Scope.WorkspaceID != created.WorkspaceID || first.Snapshot.Identity.DisplayName != r.DisplayName {
		t.Fatalf("view %+v", first)
	}
	if err = f.store.Close(); !errors.Is(err, ErrWorkspaceOpenPending) {
		t.Fatal("installation released live workspace", err)
	}
	if _, err = platform.Acquire(r.RootLocator, platform.Workspace, false, false); err == nil {
		t.Fatal("writer exclusion lost")
	}
	if _, err = opener.OpenWorkspace(ctx, openAuth, 1); !errors.Is(err, workspace.ErrLifecycle) {
		t.Fatal("duplicate open", err)
	}
	if err = opener.DrainWorkspace(ctx, openAuth); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("open permission became close", err)
	}
	if err = opener.DrainWorkspace(ctx, closeAuth); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, inspectionTree(t, r.RootLocator)) {
		t.Fatal("clean open/drain mutated source")
	}
	second, err := opener.OpenWorkspace(ctx, openAuth, 1)
	if err != nil {
		t.Fatal(err)
	}
	if second.Epoch == first.Epoch {
		t.Fatal("epoch reused")
	}
	if err = opener.DrainWorkspace(ctx, closeAuth); err != nil {
		t.Fatal(err)
	}
}
func TestWorkspaceOpenRejectsWithoutRepair(t *testing.T) {
	for _, kind := range []string{"missing-database", "missing-lock", "bad-descriptor", "sidecar", "future-schema", "digest", "not-ready", "copied-root"} {
		t.Run(kind, func(t *testing.T) {
			f, c, operator, token, r := creationFixture(t)
			defer f.store.Close()
			ctx := context.Background()
			created, err := c.CreateWorkspace(ctx, operator, r)
			if err != nil {
				t.Fatal(err)
			}
			a, err := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.open")
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing-database":
				err = os.Remove(filepath.Join(r.RootLocator, "data", "workspace.sqlite"))
			case "missing-lock":
				err = os.Remove(filepath.Join(r.RootLocator, ".aboard", "locks", "writer.lock"))
			case "bad-descriptor":
				err = os.WriteFile(filepath.Join(r.RootLocator, "aboard.workspace.json"), []byte(`{}`), 0600)
			case "sidecar":
				err = os.WriteFile(filepath.Join(r.RootLocator, "data", "workspace.sqlite-journal"), []byte("not a journal"), 0600)
			case "future-schema":
				inspectionSQL(t, r.RootLocator, "PRAGMA user_version=999;")
			case "digest":
				inspectionSQL(t, r.RootLocator, "UPDATE workspace_identity SET identity_digest=printf('%064d',0);")
			case "not-ready":
				inspectionSQL(t, r.RootLocator, "UPDATE workspace_identity SET business_state='unavailable';")
			case "copied-root":
				moved := r.RootLocator + "-original"
				err = os.Rename(r.RootLocator, moved)
				if err == nil {
					err = os.CopyFS(r.RootLocator, os.DirFS(moved))
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			before := inspectionTree(t, r.RootLocator)
			opener, _ := NewWorkspaceOpener(f.auth)
			if got, err := opener.OpenWorkspace(ctx, a, 1); err == nil || got.Epoch != "" {
				t.Fatalf("invalid directory admitted %+v %v", got, err)
			}
			if !reflect.DeepEqual(before, inspectionTree(t, r.RootLocator)) {
				t.Fatal("failed open repaired/changed source")
			}
			if err = f.store.Close(); err != nil {
				t.Fatal("failed known open leaked dependency", err)
			}
		})
	}
}
func TestWorkspaceOpenUsesSharedBindingLifecycle(t *testing.T) {
	f, c, operator, token, r := creationFixture(t)
	defer f.store.Close()
	ctx := context.Background()
	created, err := c.CreateWorkspace(ctx, operator, r)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.open")
	closer, _ := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.close")
	binding, _ := f.auth.Authorize(ctx, token, created.WorkspaceID, "workspace.operator")
	opener, _ := NewWorkspaceOpener(f.auth)
	if _, err = opener.OpenWorkspace(ctx, a, 1); err != nil {
		t.Fatal(err)
	}
	defer opener.DrainWorkspace(ctx, closer)
	l, err := f.store.WorkspaceLifecycle(ctx, binding)
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := binding.Scope()
	called := false
	if err = l.WithClosed(ctx, scope, func() error { called = true; return nil }); !errors.Is(err, workspace.ErrLifecycle) || called {
		t.Fatal("binding bypassed live open", err)
	}
}
