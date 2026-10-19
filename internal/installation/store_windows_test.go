//go:build windows && amd64

package installation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
)

func installationTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".data", "logs", "001-validation"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "t010-installation-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	return dir
}
func initializeTestStore(t *testing.T, root string) *Store {
	t.Helper()
	s, err := Initialize(context.Background(), root)
	if s != nil {
		t.Cleanup(func() {
			if err := s.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func refuseInstallation(t *testing.T, root string) {
	t.Helper()
	s, err := Open(context.Background(), root)
	if s != nil {
		if closeErr := s.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if err == nil {
		t.Fatal("bad existing installation accepted")
	}
	if s, err = Initialize(context.Background(), root); err == nil {
		s.Close()
		t.Fatal("bad existing installation reset")
	}
}
func TestInstallationStoreChild(t *testing.T) {
	mode := os.Getenv("ABOARD_T010_CHILD")
	if mode == "" {
		t.Skip("subprocess fixture only")
	}
	root := os.Getenv("ABOARD_T010_ROOT")
	if !strings.Contains(strings.ToLower(root), `\.data\logs\001-validation\t010-installation-`) {
		t.Fatal("not synthetic")
	}
	s, err := Open(context.Background(), root)
	if mode == "refuse" {
		if err == nil {
			s.Close()
			t.Fatal("second process acquired live host.lock")
		}
		if s != nil {
			s.Close()
		}
		fmt.Println("T010 refused")
		return
	}
	if mode != "reopen" {
		t.Fatal("unknown child mode")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	data, err := json.Marshal(s.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != os.Getenv("ABOARD_T010_IDENTITY") {
		t.Fatal("fresh process identity changed")
	}
	fmt.Println("T010 reopened identical stable IDs")
}
func runInstallationChild(t *testing.T, root, mode string, id Identity) {
	t.Helper()
	data, _ := json.Marshal(id)
	cmd := exec.Command(os.Args[0], "-test.run=^TestInstallationStoreChild$", "-test.v")
	cmd.Env = append(os.Environ(), "ABOARD_T010_CHILD="+mode, "ABOARD_T010_ROOT="+root, "ABOARD_T010_IDENTITY="+string(data))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("second process failed: %v %s", err, output)
	}
	if !bytes.Contains(output, []byte("T010 ")) {
		t.Fatal("child evidence absent")
	}
}
func TestInstallationStoreStableReopenAndProcessLease(t *testing.T) {
	root := filepath.Join(installationTestRoot(t), "install")
	s := initializeTestStore(t, root)
	id := s.Identity()
	if len(id.InstallationID) != 36 || len(id.TenantID) != 36 || len(id.OwnerPrincipalID) != 36 {
		t.Fatal("IDs are not canonical UUIDs")
	}
	if _, err := security.Load(root); err == nil {
		t.Fatal("prototype reacquired live host.lock")
	}
	runInstallationChild(t, root, "refuse", id)
	for _, name := range []string{"host.lock", "bootstrap.dpapi", "installation.sqlite"} {
		path := filepath.Join(root, name)
		if err := os.Remove(path); err == nil {
			t.Fatal("live installation file removed")
		}
		if err := os.Rename(path, path+".replaced"); err == nil {
			t.Fatal("live installation file replaced")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	runInstallationChild(t, root, "reopen", id)
	reopened, err := Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Identity() != id {
		t.Fatal("same-process reopen changed identity")
	}
	if err = reopened.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestInstallationStoreExistingFailuresNoReseed(t *testing.T) {
	for _, kind := range []string{"missing-db", "missing-material", "missing-lock", "bad-material", "zero-db", "bad-ledger", "wrong-id", "wrong-sid", "wrong-secret", "missing-owner", "owner-tenant", "wrong-owner", "db-acl", "material-acl", "db-hardlink", "material-hardlink", "lock-hardlink"} {
		t.Run(kind, func(t *testing.T) {
			root := filepath.Join(installationTestRoot(t), "install")
			s := initializeTestStore(t, root)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			dbPath := filepath.Join(root, "installation.sqlite")
			materialPath := filepath.Join(root, "bootstrap.dpapi")
			switch kind {
			case "missing-db":
				if err := os.Remove(dbPath); err != nil {
					t.Fatal(err)
				}
			case "missing-material":
				if err := os.Remove(materialPath); err != nil {
					t.Fatal(err)
				}
			case "missing-lock":
				if err := os.Remove(filepath.Join(root, "host.lock")); err != nil {
					t.Fatal(err)
				}
			case "bad-material":
				if err := os.WriteFile(materialPath, []byte("synthetic-bad-DPAPI"), 0600); err != nil {
					t.Fatal(err)
				}
			case "zero-db":
				if err := os.WriteFile(dbPath, nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "bad-ledger", "wrong-id", "wrong-sid", "wrong-secret":
				h, err := sqlite.Open(context.Background(), dbPath, sqlite.ReadWriteExisting)
				if err != nil {
					t.Fatal(err)
				}
				query := map[string]string{"bad-ledger": "UPDATE migration_ledger SET checksum=printf('%064d',0)", "wrong-id": "UPDATE installation_identity SET installation_id='aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'", "wrong-sid": "UPDATE installation_identity SET windows_sid='S-1-5-18'", "wrong-secret": "UPDATE installation_identity SET secret_reference='synthetic-wrong-reference'"}[kind]
				_, err = h.Conn.ExecContext(context.Background(), query)
				closeErr := h.Close()
				if err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
			case "missing-owner", "owner-tenant", "wrong-owner":
				h, err := sqlite.Open(context.Background(), dbPath, sqlite.ReadWriteExisting)
				if err != nil {
					t.Fatal(err)
				}
				// Corrupt only a closed synthetic database. Existing open must verify
				// relationships rather than trusting the external writer's FK setting.
				if _, err = h.Conn.ExecContext(context.Background(), "PRAGMA foreign_keys=OFF"); err != nil {
					t.Fatal(err)
				}
				query := map[string]string{"missing-owner": "DELETE FROM principal", "owner-tenant": "UPDATE principal SET tenant_id='aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'", "wrong-owner": "UPDATE installation_identity SET owner_principal_id='aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'"}[kind]
				_, err = h.Conn.ExecContext(context.Background(), query)
				closeErr := h.Close()
				if err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
			case "db-acl", "material-acl":
				path := dbPath
				if kind == "material-acl" {
					path = materialPath
				}
				sd, err := win.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
				if err != nil {
					t.Fatal(err)
				}
				acl, _, err := sd.DACL()
				if err != nil {
					t.Fatal(err)
				}
				if err = win.SetNamedSecurityInfo(path, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION|win.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
					t.Fatal(err)
				}
			default:
				path := dbPath
				if kind == "material-hardlink" {
					path = materialPath
				}
				if kind == "lock-hardlink" {
					path = filepath.Join(root, "host.lock")
				}
				if err := os.Link(path, filepath.Join(root, "synthetic-link")); err != nil {
					t.Fatal(err)
				}
			}
			before := installationSnapshot(t, root)
			refuseInstallation(t, root)
			after := installationSnapshot(t, root)
			if !bytes.Equal(before, after) {
				t.Fatal("refusal changed existing file names or bytes")
			}
		})
	}
}
func installationSnapshot(t *testing.T, root string) []byte {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out.WriteString(entry.Name())
		out.WriteByte(0)
		out.Write(data)
		out.WriteByte(0)
	}
	return out.Bytes()
}
func TestInstallationStoreExplicitPartialAndMissing(t *testing.T) {
	base := installationTestRoot(t)
	root := filepath.Join(base, "missing")
	if s, err := Open(context.Background(), root); err == nil || s != nil {
		t.Fatal("missing installation accepted")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("open created missing root")
	}
	partial := filepath.Join(base, "partial")
	if err := security.Initialize(partial); err != nil {
		t.Fatal(err)
	}
	before := installationSnapshot(t, partial)
	refuseInstallation(t, partial)
	if !bytes.Equal(before, installationSnapshot(t, partial)) {
		t.Fatal("material-only prototype became DB authority")
	}
	empty := filepath.Join(base, "empty")
	if err := platform.CreatePrivateDirectory(empty); err != nil {
		t.Fatal(err)
	}
	refuseInstallation(t, empty)
	if entries, err := os.ReadDir(empty); err != nil || len(entries) != 0 {
		t.Fatal("empty existing root was seeded")
	}
	inherited := filepath.Join(base, "inherited")
	if err := os.Mkdir(inherited, 0700); err != nil {
		t.Fatal(err)
	}
	refuseInstallation(t, inherited)
}
func TestInstallationStoreMaterialDatabaseMismatch(t *testing.T) {
	base := installationTestRoot(t)
	a := filepath.Join(base, "a")
	b := filepath.Join(base, "b")
	sa := initializeTestStore(t, a)
	sb := initializeTestStore(t, b)
	if err := sa.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sb.Close(); err != nil {
		t.Fatal(err)
	}
	// Keep the original private file object/ACL but replace its encrypted content
	// with a different synthetic installation's valid DPAPI material.
	blob, err := os.ReadFile(filepath.Join(b, "bootstrap.dpapi"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(a, "bootstrap.dpapi"), blob, 0600); err != nil {
		t.Fatal(err)
	}
	before := installationSnapshot(t, a)
	refuseInstallation(t, a)
	if !bytes.Equal(before, installationSnapshot(t, a)) {
		t.Fatal("mismatch repaired")
	}
}
func TestInstallationStoreUnknownPhysicalCloseRetainsLease(t *testing.T) {
	root := filepath.Join(installationTestRoot(t), "install")
	s := initializeTestStore(t, root)
	s.openFailure = true // models the adapter's explicit unknown-close marker only
	if err := s.Close(); err == nil {
		t.Fatal("unknown physical end released lease")
	}
	runInstallationChild(t, root, "refuse", s.Identity())
	s.openFailure = false // test-only: live healthy DB can now actually be closed
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
