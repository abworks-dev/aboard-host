//go:build windows && amd64

package installation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	inspection "github.com/abworks-dev/aboard-host/internal/workspace/inspect"
)

func inspectionTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		key, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if e.IsDir() {
			out[key] = "directory"
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		out[key] = hex.EncodeToString(sum[:])
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func inspectionSource(t *testing.T, root, tenant string) workspace.DirectoryDescriptor {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".aboard", "locks"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".aboard", "locks", "writer.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	// Synthetic fixture preparation only; production creation uses T013's
	// retained native fresh-file proof rather than these absolute test writes.
	db, err := sqlite.OpenDatabase(context.Background(), filepath.Join(root, "data", "workspace.sqlite"), sqlite.CreateNew, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase, WorkspaceID: authA, TenantID: tenant, DisplayName: "synthetic 客服"})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	d, err := workspace.NewDirectoryDescriptor(authA, tenant)
	if err != nil {
		t.Fatal(err)
	}
	b, err := workspace.EncodeDirectoryDescriptor(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "aboard.workspace.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDirectoryInspectionUseCase(t *testing.T) {
	f := newAuthFixture(t)
	operator, token := authOperator(t, f)
	scratch := filepath.Join(f.root, "scratch")
	if err := platform.CreatePrivateDirectory(scratch); err != nil {
		t.Fatal(err)
	}
	inspector, err := NewDirectoryInspector(f.store, scratch)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		want   protocol.ErrorCode
		mutate func(string, workspace.DirectoryDescriptor)
	}{
		{"valid", "", nil},
		{"future format", protocol.ErrorCodeUNSUPPORTEDFORMAT, func(root string, d workspace.DirectoryDescriptor) {
			d.FormatVersion = 99
			inspectionWriteDescriptor(t, root, d)
		}},
		{"legacy format", protocol.ErrorCodeUNSUPPORTEDFORMAT, func(root string, d workspace.DirectoryDescriptor) {
			d.FormatVersion = 1
			inspectionWriteDescriptor(t, root, d)
		}},
		{"future digest", protocol.ErrorCodeUNSUPPORTEDFORMAT, func(root string, d workspace.DirectoryDescriptor) {
			d.IdentityDigestVersion = 2
			inspectionWriteDescriptor(t, root, d)
		}},
		{"bad digest", protocol.ErrorCodeDIGESTMISMATCH, func(root string, d workspace.DirectoryDescriptor) {
			d.IdentityDigest = strings.Repeat("0", 64)
			inspectionWriteDescriptor(t, root, d)
		}},
		{"forged identity", protocol.ErrorCodeIDENTITYCONFLICT, func(root string, d workspace.DirectoryDescriptor) {
			other, e := workspace.NewDirectoryDescriptor(authB, d.TenantID)
			if e != nil {
				t.Fatal(e)
			}
			inspectionWriteDescriptor(t, root, other)
		}},
		{"unknown field", protocol.ErrorCodeINCOMPLETEDIRECTORY, func(root string, d workspace.DirectoryDescriptor) {
			p := filepath.Join(root, "aboard.workspace.json")
			b, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			b = append(b[:len(b)-1], []byte(`,"principalId":"not-a-grant"}`)...)
			if e = os.WriteFile(p, b, 0600); e != nil {
				t.Fatal(e)
			}
		}},
		{"future schema", protocol.ErrorCodeUNSUPPORTEDSCHEMA, func(root string, _ workspace.DirectoryDescriptor) { inspectionSQL(t, root, "PRAGMA user_version=2") }},
		{"ledger mismatch", protocol.ErrorCodeCORRUPTDATABASE, func(root string, _ workspace.DirectoryDescriptor) {
			inspectionSQL(t, root, "UPDATE migration_ledger SET checksum='"+strings.Repeat("0", 64)+"'")
		}},
		{"missing DB", protocol.ErrorCodeMISSINGDATABASE, func(root string, _ workspace.DirectoryDescriptor) {
			if e := os.Remove(filepath.Join(root, "data", "workspace.sqlite")); e != nil {
				t.Fatal(e)
			}
		}},
		{"missing descriptor", protocol.ErrorCodeINSPECTIONUNAVAILABLE, func(root string, _ workspace.DirectoryDescriptor) {
			if e := os.Remove(filepath.Join(root, "aboard.workspace.json")); e != nil {
				t.Fatal(e)
			}
		}},
		{"missing lock", protocol.ErrorCodeINSPECTIONUNAVAILABLE, func(root string, _ workspace.DirectoryDescriptor) {
			if e := os.Remove(filepath.Join(root, ".aboard", "locks", "writer.lock")); e != nil {
				t.Fatal(e)
			}
		}},
		{"corrupt DB", protocol.ErrorCodeCORRUPTDATABASE, func(root string, _ workspace.DirectoryDescriptor) {
			if e := os.WriteFile(filepath.Join(root, "data", "workspace.sqlite"), bytes.Repeat([]byte("synthetic invalid header"), 10), 0600); e != nil {
				t.Fatal(e)
			}
		}},
		{"journal", protocol.ErrorCodeRECOVERYREQUIRED, func(root string, _ workspace.DirectoryDescriptor) {
			if e := os.WriteFile(filepath.Join(root, "data", "workspace.sqlite-journal"), []byte("synthetic residue"), 0600); e != nil {
				t.Fatal(e)
			}
		}},
	}
	for index, row := range cases {
		t.Run(row.name, func(t *testing.T) {
			root := filepath.Join(f.allowed, strings.ReplaceAll(row.name, " ", "-"))
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			d := inspectionSource(t, root, f.store.Identity().TenantID)
			if row.mutate != nil {
				row.mutate(root, d)
			}
			before := inspectionTree(t, root)
			got, err := inspector.Inspect(context.Background(), operator, root)
			if row.want == "" {
				if err != nil || got.WorkspaceID != d.WorkspaceID || got.TenantID != d.TenantID || got.FormatVersion != 2 || got.ScratchCleanupFailed {
					t.Fatalf("valid disk evidence: %+v %v", got, err)
				}
			} else {
				var failure *InspectionFailure
				if !errors.As(err, &failure) || failure.Code != row.want {
					t.Fatalf("case %d want %s: %+v %v", index, row.want, got, err)
				}
				if got.WorkspaceID != "" || got.TenantID != "" || strings.Contains(err.Error(), root) {
					t.Fatal("rejected inspection leaked source identity/path")
				}
			}
			if !reflect.DeepEqual(before, inspectionTree(t, root)) {
				t.Fatal("inspection changed source bytes or inventory")
			}
			entries, e := os.ReadDir(scratch)
			if e != nil || len(entries) != 0 {
				t.Fatalf("scratch retained on normal cleanup: %v %v", entries, e)
			}
		})
	}
	root := filepath.Join(f.allowed, "writer-held")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	inspectionSource(t, root, f.store.Identity().TenantID)
	before := inspectionTree(t, root)
	lease, err := platform.Acquire(root, platform.Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = inspector.Inspect(context.Background(), operator, root)
	var failure *InspectionFailure
	if !errors.As(err, &failure) || failure.Code != protocol.ErrorCodeINSPECTIONUNAVAILABLE {
		t.Errorf("unestablished tenant sharing state leaked: %v", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, inspectionTree(t, root)) {
		t.Fatal("busy source changed")
	}
	for _, kind := range []string{"healthy", "future-schema", "bad-ledger", "bad-digest", "missing-db", "forged-own-descriptor"} {
		foreign := filepath.Join(f.allowed, "foreign-"+kind)
		if err = os.Mkdir(foreign, 0700); err != nil {
			t.Fatal(err)
		}
		d := inspectionSource(t, foreign, authB)
		switch kind {
		case "future-schema":
			inspectionSQL(t, foreign, "PRAGMA user_version=2")
		case "bad-ledger":
			inspectionSQL(t, foreign, "UPDATE migration_ledger SET checksum='"+strings.Repeat("0", 64)+"'")
		case "bad-digest":
			d.IdentityDigest = strings.Repeat("0", 64)
			inspectionWriteDescriptor(t, foreign, d)
		case "missing-db":
			if err = os.Remove(filepath.Join(foreign, "data", "workspace.sqlite")); err != nil {
				t.Fatal(err)
			}
		case "forged-own-descriptor":
			d, err = workspace.NewDirectoryDescriptor(authA, f.store.Identity().TenantID)
			if err != nil {
				t.Fatal(err)
			}
			inspectionWriteDescriptor(t, foreign, d)
		}
		foreignBefore := inspectionTree(t, foreign)
		if result, e := inspector.Inspect(context.Background(), operator, foreign); !errors.Is(e, ErrUnauthorized) || result.WorkspaceID != "" || result.ScratchCleanupFailed {
			t.Fatalf("foreign %s health/identity leaked: %+v %v", kind, result, e)
		}
		if !reflect.DeepEqual(foreignBefore, inspectionTree(t, foreign)) {
			t.Fatal("foreign source changed")
		}
	}
	readToken := authIssue(t, f, authRestricted, authA, "workspace.read", time.Minute)
	readAuth, err := f.auth.Authorize(context.Background(), readToken, authA, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = inspector.Inspect(context.Background(), readAuth, root); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("read capability became arbitrary inspect")
	}
	if _, err = inspector.Inspect(context.Background(), operator, filepath.Join(f.root, "outside-allowed")); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("allowed-root policy bypass")
	}
	if err = f.auth.RevokeSession(context.Background(), operator, token); err != nil {
		t.Fatal(err)
	}
	if _, err = inspector.Inspect(context.Background(), operator, root); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old approval survived current revocation")
	}
}

func TestDirectoryInspectionMidFreezeRevocation(t *testing.T) {
	f := newAuthFixture(t)
	operator, token := authOperator(t, f)
	scratch := filepath.Join(f.root, "scratch-revoke")
	if err := platform.CreatePrivateDirectory(scratch); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(f.allowed, "mid-revoke")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	inspectionSource(t, root, f.store.Identity().TenantID)
	before := inspectionTree(t, root)
	checks := 0
	result, err := inspection.InspectDirectory(context.Background(), root, scratch, f.store.Identity().TenantID, func(ctx context.Context) error {
		checks++
		if err := operator.CheckLocation(ctx, root); err != nil {
			return err
		}
		if checks == 2 {
			// Real DB revocation at the frozen-metadata policy port, immediately
			// after that point's real authorization check. The post-IO check must
			// suppress even a successfully validated private snapshot.
			if err := f.auth.RevokeSession(ctx, operator, token); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	})
	if !errors.Is(err, ErrUnauthorized) || checks != 3 || result.Descriptor.WorkspaceID != "" {
		t.Fatalf("mid-freeze revocation leaked result: checks=%d %+v %v", checks, result, err)
	}
	if !reflect.DeepEqual(before, inspectionTree(t, root)) {
		t.Fatal("revocation caused source mutation")
	}
	inspector, e := NewDirectoryInspector(f.store, scratch)
	if e != nil {
		t.Fatal(e)
	}
	if output, e := inspector.Inspect(context.Background(), operator, root); !errors.Is(e, ErrUnauthorized) || output.WorkspaceID != "" {
		t.Fatal("public usecase retained old permission")
	}
}

func inspectionWriteDescriptor(t *testing.T, root string, d workspace.DirectoryDescriptor) {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "aboard.workspace.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}
func inspectionSQL(t *testing.T, root, query string) {
	t.Helper()
	h, err := sqlite.Open(context.Background(), filepath.Join(root, "data", "workspace.sqlite"), sqlite.ReadWriteExisting)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Conn.ExecContext(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	if err = h.Close(); err != nil {
		t.Fatal(err)
	}
}
