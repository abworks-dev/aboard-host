//go:build windows && amd64

package windows

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
)

const freshWorkspace = "62000000-0000-4000-8000-000000000001"
const freshTenant = "62000000-0000-4000-8000-000000000002"

func TestWorkspaceCreationFreshPinsBeforeSQLiteAndOneShotGenesis(t *testing.T) {
	root := filepath.Join(syntheticDir(t), "created")
	c, err := InspectWorkspaceCreation(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("inspection created absent root")
	}
	if err = c.Prepare(); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err = os.Rename(filepath.Join(root, "data", "workspace.sqlite"), filepath.Join(root, "data", "moved")); err == nil {
		t.Fatal("fresh database was not pinned before SQLite")
	}
	if other, err := Acquire(root, Workspace, false, false); err == nil {
		other.Close()
		t.Fatal("real exclusive creation lock not held")
	}
	g := sqlite.Genesis{Kind: sqlite.WorkspaceDatabase, WorkspaceID: freshWorkspace, TenantID: freshTenant, DisplayName: "synthetic"}
	d, err := sqlite.InitializePinnedWorkspace(context.Background(), c.FreshDatabase(), g)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.VerifyCreationGenesis(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if err = d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = sqlite.InitializePinnedWorkspace(context.Background(), c.FreshDatabase(), g); err == nil {
		t.Fatal("fresh proof reused")
	}
	if err = c.PublishDescriptor([]byte("synthetic descriptor")); err != nil {
		t.Fatal(err)
	}
	if err = c.PublishDescriptor([]byte("overwrite")); err == nil {
		t.Fatal("descriptor overwrite accepted")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = InspectWorkspaceCreation(root); !errors.Is(err, ErrDirectoryNotEmpty) {
		t.Fatalf("residual tree became first use: %v", err)
	}
}
func TestWorkspaceCreationClosedFreshAndOldZeroFileNeverInitialize(t *testing.T) {
	root := filepath.Join(syntheticDir(t), "created")
	c, err := InspectWorkspaceCreation(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Prepare(); err != nil {
		t.Fatal(err)
	}
	fresh := c.FreshDatabase()
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = sqlite.InitializePinnedWorkspace(context.Background(), fresh, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase, WorkspaceID: freshWorkspace, TenantID: freshTenant, DisplayName: "synthetic"}); err == nil {
		t.Fatal("closed fresh initialized")
	}
	path := filepath.Join(root, "data", "workspace.sqlite")
	if _, err = sqlite.OpenDatabase(context.Background(), path, sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase, WorkspaceID: freshWorkspace, TenantID: freshTenant}); err == nil {
		t.Fatal("old zero file silently initialized")
	}
}
func TestWorkspaceCreationMetadataReparseCannotRedirectPrepare(t *testing.T) {
	for _, existingRoot := range []bool{false, true} {
		t.Run(map[bool]string{false: "retained-parent", true: "empty-root"}[existingRoot], func(t *testing.T) {
			parent := syntheticDir(t)
			root := filepath.Join(parent, "new-root")
			mutated := parent
			if existingRoot {
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
				mutated = root
			}
			target := syntheticDir(t)
			c, err := InspectWorkspaceCreation(root)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if err = markJunctionAccess(mutated, target, win.FILE_WRITE_ATTRIBUTES); err != nil {
				t.Fatalf("required metadata-only mutation stimulus failed: %v", err)
			}
			if err = c.Prepare(); err == nil {
				t.Fatal("metadata-reparsed ancestor redirected creation")
			}
			entries, err := os.ReadDir(target)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatal("redirect target changed")
			}
		})
	}
}
