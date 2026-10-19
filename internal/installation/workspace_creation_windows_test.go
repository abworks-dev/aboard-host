//go:build windows && amd64

package installation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"

	"golang.org/x/sys/windows"
	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

const creationOperation = "61000000-0000-4000-8000-000000000001"

func creationFixture(t *testing.T) (authFixture, *WorkspaceCreator, *Authorization, string, CreateWorkspaceRequest) {
	t.Helper()
	f := newAuthFixture(t)
	c, err := NewWorkspaceCreator(f.auth)
	if err != nil {
		t.Fatal(err)
	}
	operator, token := authOperator(t, f)
	r := CreateWorkspaceRequest{creationOperation, filepath.Join(f.allowed, "created"), "synthetic workspace"}
	return f, c, operator, token, r
}

// Test-binary-only VFS syscall replacement, the same frozen ABI technique as
// storage's existing flush gate. No production fault API/driver/dep is added.
var creationFlushFault struct {
	handle        uintptr
	at            int32
	calls, failed atomic.Int32
	// Optional child-only crash point after a real successful native flush.
	afterFlush func(int32)
}

func creationFlushStimulus(tls *libc.TLS, actual uintptr) int32 {
	n := creationFlushFault.calls.Add(1)
	if n == creationFlushFault.at {
		creationFlushFault.failed.Add(1)
		return libc.XFlushFileBuffers(tls, creationFlushFault.handle)
	}
	result := libc.XFlushFileBuffers(tls, actual)
	if result != 0 && creationFlushFault.afterFlush != nil {
		creationFlushFault.afterFlush(n)
	}
	return result
}
func replaceCreationFlush(t *testing.T) func() {
	t.Helper()
	tls := libc.NewTLS()
	name, err := libc.CString("win32")
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, name)
	vfs := sqlite3.Xsqlite3_vfs_find(tls, name)
	if vfs == 0 {
		t.Fatal("win32 VFS unavailable")
	}
	var view sqlite3.Tsqlite3_vfs
	libc.Xmemcpy(tls, uintptr(unsafe.Pointer(&view)), vfs, libc.Tsize_t(unsafe.Sizeof(view)))
	if view.FiVersion < 3 || view.FxSetSystemCall == 0 || view.FxGetSystemCall == 0 {
		t.Fatal("VFS syscall table unavailable")
	}
	get := *(*func(*libc.TLS, uintptr, uintptr) uintptr)(unsafe.Pointer(&view.FxGetSystemCall))
	set := *(*func(*libc.TLS, uintptr, uintptr, uintptr) int32)(unsafe.Pointer(&view.FxSetSystemCall))
	syscallName, err := libc.CString("FlushFileBuffers")
	if err != nil {
		t.Fatal(err)
	}
	original := get(tls, vfs, syscallName)
	if original == 0 {
		t.Fatal("VFS flush syscall missing")
	}
	callback := creationFlushStimulus
	pointer := *(*uintptr)(unsafe.Pointer(&callback))
	if rc := set(tls, vfs, syscallName, pointer); rc != sqlite3.SQLITE_OK {
		t.Fatalf("set test syscall failed: %d", rc)
	}
	var once sync.Once
	restore := func() {
		once.Do(func() {
			if rc := set(tls, vfs, syscallName, original); rc != sqlite3.SQLITE_OK {
				t.Errorf("restore syscall failed: %d", rc)
			}
			libc.Xfree(tls, syscallName)
			tls.Close()
		})
	}
	t.Cleanup(restore)
	return restore
}
func TestWorkspaceCreationNativeAdmissionUnknownNeverTouchesTarget(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing-empty-%t", existing), func(t *testing.T) {
			count := int32(0)
			for ordinal := int32(0); ordinal == 0 || ordinal <= count; ordinal++ {
				t.Run(fmt.Sprintf("flush-%d", ordinal), func(t *testing.T) {
					f, c, operator, token, r := creationFixture(t)
					if existing {
						if err := os.Mkdir(r.RootLocator, 0700); err != nil {
							t.Fatal(err)
						}
					}
					path := filepath.Join(f.root, "readonly-stimulus")
					if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
						t.Fatal(err)
					}
					stimulus, err := os.Open(path)
					if err != nil {
						t.Fatal(err)
					}
					defer stimulus.Close()
					if err = windows.FlushFileBuffers(windows.Handle(stimulus.Fd())); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
						t.Fatal("native flush failure stimulus missing", err)
					}
					creationFlushFault.handle = stimulus.Fd()
					creationFlushFault.at = ordinal
					creationFlushFault.calls.Store(0)
					creationFlushFault.failed.Store(0)
					restore := replaceCreationFlush(t)
					c.afterPhase = func(p sqlite.InstallationPhase) error {
						if p == sqlite.IntentRecorded {
							restore()
						}
						return nil
					}
					_, err = c.CreateWorkspace(context.Background(), operator, r)
					calls, failed := creationFlushFault.calls.Load(), creationFlushFault.failed.Load()
					restore()
					if ordinal == 0 {
						if err != nil || calls < 1 || calls > 8 {
							t.Fatalf("admission baseline missing: %v calls=%d", err, calls)
						}
						count = calls
						if err = f.store.Close(); err != nil {
							t.Fatal(err)
						}
						return
					}
					if !errors.Is(err, sqlite.ErrUnknown) || failed != 1 {
						t.Fatalf("real admission error not unknown: %v failed=%d", err, failed)
					}
					if existing {
						entries, e := os.ReadDir(r.RootLocator)
						if e != nil || len(entries) != 0 {
							t.Fatalf("unconfirmed admission touched empty target: %v", e)
						}
					} else if _, e := os.Stat(r.RootLocator); !os.IsNotExist(e) {
						t.Fatalf("unconfirmed admission created target: %v", e)
					}
					second, _ := NewWorkspaceCreator(f.auth)
					for _, wrapper := range []*WorkspaceCreator{c, second} {
						if _, e := wrapper.CreateWorkspace(context.Background(), operator, r); !errors.Is(e, ErrAuthorizationUnknown) {
							t.Fatalf("same-conn unknown admission reentered: %v", e)
						}
					}
					if err = f.store.Close(); err != nil {
						t.Fatal(err)
					}
					s, e := Open(context.Background(), f.install)
					if e != nil {
						t.Fatal(e)
					}
					defer s.Close()
					a, _ := NewAuthenticator(s, []string{f.allowed})
					current, e := a.Authorize(context.Background(), token, "", "installation.operator")
					if e != nil {
						t.Fatal(e)
					}
					cold, _ := NewWorkspaceCreator(a)
					got, e := cold.GetInstallationOperation(context.Background(), current, r.OperationID)
					if errors.Is(e, ErrWorkspaceCreationNotFound) {
						if _, e = cold.ReconcileInstallationOperation(context.Background(), current, r.OperationID, sqlite.IntentRecorded); !errors.Is(e, ErrWorkspaceCreationNotFound) {
							t.Fatalf("cold no-row promoted into recovery: %v", e)
						}
					} else if e != nil || got.Outcome == sqlite.InstallationRejected {
						t.Fatalf("cold observation invented rejection/safe retry: %+v %v", got, e)
					}
					// NO cold Create: missing durable row cannot reconstruct the old RPC result.
					if existing {
						entries, e := os.ReadDir(r.RootLocator)
						if e != nil || len(entries) != 0 {
							t.Fatal("cold inquiry wrote source")
						}
					} else if _, e := os.Stat(r.RootLocator); !os.IsNotExist(e) {
						t.Fatal("cold inquiry created source")
					}
					t.Logf("admission native ordinal=%d failed=%d flush_calls=%d target_unchanged=true", ordinal, failed, calls)
				})
			}
		})
	}
}
func TestWorkspaceCreationRealRoundTrip(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing-empty"}[existing], func(t *testing.T) {
			f, c, operator, token, r := creationFixture(t)
			if existing {
				if err := os.Mkdir(r.RootLocator, 0700); err != nil {
					t.Fatal(err)
				}
			}
			got, err := c.CreateWorkspace(context.Background(), operator, r)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.WorkspaceID) != 36 || got.WorkspaceID[14] != '4' || !strings.ContainsRune("89ab", rune(got.WorkspaceID[19])) {
				t.Fatal("business identity is not canonical UUIDv4")
			}
			if got.Phase != sqlite.RegistrationConfirmed || got.Outcome != sqlite.InstallationCommitted {
				t.Fatalf("not committed: %+v", got)
			}
			if err = f.store.Close(); err != nil {
				t.Fatal(err)
			}
			s, err := Open(context.Background(), f.install)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := s.Close(); err != nil {
					t.Error(err)
				}
			}()
			a, err := NewAuthenticator(s, []string{f.allowed})
			if err != nil {
				t.Fatal(err)
			}
			c, err = NewWorkspaceCreator(a)
			if err != nil {
				t.Fatal(err)
			}
			operator, err = a.Authorize(context.Background(), token, "", "installation.operator")
			if err != nil {
				t.Fatal(err)
			}
			query, err := c.GetInstallationOperation(context.Background(), operator, r.OperationID)
			if err != nil || query != got {
				t.Fatalf("reopen query=%+v err=%v", query, err)
			}
			b, err := json.Marshal(query)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), f.root) || strings.Contains(string(b), "RootLocator") || strings.Contains(string(b), token) {
				t.Fatal("operation view leaks path or bearer")
			}
			for _, action := range []string{"workspace.read", "workspace.write", "workspace.open", "workspace.close", "workspace.operator"} {
				if _, err = a.Authorize(context.Background(), token, got.WorkspaceID, action); err != nil {
					t.Fatalf("missing initial %s grant: %v", action, err)
				}
			}
			manifest, err := os.ReadFile(filepath.Join(r.RootLocator, "aboard.workspace.json"))
			if err != nil {
				t.Fatal(err)
			}
			descriptor, err := workspace.ParseDirectoryDescriptor(manifest)
			if err != nil || descriptor.WorkspaceID != got.WorkspaceID {
				t.Fatalf("descriptor=%+v err=%v", descriptor, err)
			}
			again, err := c.CreateWorkspace(context.Background(), operator, r)
			if err != nil || again != got {
				t.Fatalf("idempotent investigation=%+v %v", again, err)
			}
			if _, err = c.ReconcileInstallationOperation(context.Background(), operator, r.OperationID, sqlite.RegistrationConfirmed); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCommittedCreationReconcileAfterRealSave(t *testing.T) {
	f, creator, operator, _, request := creationFixture(t)
	defer f.store.Close()
	ctx := context.Background()
	created, err := creator.CreateWorkspace(ctx, operator, request)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.OpenDatabase(ctx, filepath.Join(request.RootLocator, "data", "workspace.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
	if err != nil {
		lease.Close()
		t.Fatal(err)
	}
	defer lease.Close()
	defer db.Close()
	gate, err := sqlite.NewCommitGate(db, "synthetic-used-creation")
	if err != nil {
		db.Close()
		lease.Close()
		t.Fatal(err)
	}
	candidate := sqlite.Candidate{WorkspaceID: created.WorkspaceID, CandidateID: authA, PrincipalID: operator.principalID, ResourceID: authB, Kind: "create-text", Title: "synthetic saved title", Body: "synthetic saved body", PreviewVersion: 1, PreviewEpoch: "synthetic-used-creation", ExpiresAt: time.Now().Add(time.Hour)}
	content, err := digest.Content(digest.ContentInput{WorkspaceID: protocol.UUID(candidate.WorkspaceID), ResourceID: protocol.UUID(candidate.ResourceID), Title: protocol.Title(candidate.Title), Body: protocol.Body(candidate.Body)})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := digest.Candidate(digest.CandidateInput{WorkspaceID: protocol.UUID(candidate.WorkspaceID), PrincipalID: protocol.UUID(candidate.PrincipalID), CandidateID: protocol.UUID(candidate.CandidateID), ResourceID: protocol.UUID(candidate.ResourceID), Kind: protocol.PreviewRecordKind(candidate.Kind), BaseRevision: "0", ContentDigest: content, PreviewVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	candidate.ContentDigest, candidate.CandidateDigest = string(content), string(preview)
	if err = gate.PutCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	save := sqlite.SaveRequest{InstallationID: operator.installationID, TenantID: operator.tenantID, WorkspaceID: created.WorkspaceID, PrincipalID: operator.principalID, OperationID: authB, CandidateID: candidate.CandidateID, CandidateDigest: candidate.CandidateDigest, Epoch: candidate.PreviewEpoch, PreviewVersion: candidate.PreviewVersion}
	_, admission, err := gate.Admit(ctx, save, time.Now())
	if err != nil || admission == nil {
		t.Fatalf("admission: %v", err)
	}
	result, err := gate.CommitAccepted(ctx, admission)
	if err != nil || result.Status != sqlite.Confirmed {
		t.Fatalf("save: %+v %v", result, err)
	}
	if err = gate.EndPhysical(); err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	before := inspectionTree(t, request.RootLocator)
	got, err := creator.ReconcileInstallationOperation(ctx, operator, request.OperationID, sqlite.RegistrationConfirmed)
	if err != nil || got != created {
		t.Fatalf("legitimate save invalidated committed creation: %+v %v", got, err)
	}
	after := inspectionTree(t, request.RootLocator)
	if len(before) != len(after) {
		t.Fatal("reconcile changed source inventory")
	}
	for name, value := range before {
		if after[name] != value {
			t.Fatalf("reconcile changed %s", name)
		}
	}
}

func TestWorkspaceCreationOriginalPayloadAndScope(t *testing.T) {
	f, c, operator, _, r := creationFixture(t)
	defer f.store.Close()
	c.afterPhase = func(p sqlite.InstallationPhase) error {
		if p == sqlite.IntentRecorded {
			return sqlite.ErrUnknown
		}
		return nil
	}
	if _, err := c.CreateWorkspace(context.Background(), operator, r); err == nil {
		t.Fatal("injected uncertainty lost")
	}
	for _, change := range []func(*CreateWorkspaceRequest){func(r *CreateWorkspaceRequest) { r.DisplayName = "changed" }, func(r *CreateWorkspaceRequest) { r.RootLocator += "-other" }} {
		other := r
		change(&other)
		if _, err := c.CreateWorkspace(context.Background(), operator, other); !errors.Is(err, sqlite.ErrInstallationOperationConflict) {
			t.Fatalf("changed original payload not conflict: %v", err)
		}
	}
	forged := *operator
	forged.principalID = authRestricted
	if _, err := c.GetInstallationOperation(context.Background(), &forged, r.OperationID); err == nil {
		t.Fatal("changed actor accepted")
	}
	restricted, _ := NewAuthenticator(f.store, []string{filepath.Join(f.allowed, "A")})
	restrictedCreator, _ := NewWorkspaceCreator(restricted)
	if _, err := restrictedCreator.GetInstallationOperation(context.Background(), operator, r.OperationID); err == nil {
		t.Fatal("cross-auth handle accepted")
	}
	if _, err := c.ReconcileInstallationOperation(context.Background(), operator, r.OperationID, sqlite.DescriptorPublished); err == nil {
		t.Fatal("intent replayed by reconcile")
	}
	if _, err := os.Stat(r.RootLocator); !os.IsNotExist(err) {
		t.Fatalf("intent-only retry created target: %v", err)
	}
}
func TestWorkspaceCreationFailureAndUnknownPhases(t *testing.T) {
	for _, phase := range []sqlite.InstallationPhase{sqlite.IntentRecorded, sqlite.DirectoryPrepared, sqlite.GenesisConfirmed, sqlite.DescriptorPublished} {
		t.Run(string(phase), func(t *testing.T) {
			f, c, operator, _, r := creationFixture(t)
			defer f.store.Close()
			c.afterPhase = func(p sqlite.InstallationPhase) error {
				if p == phase {
					return sqlite.ErrUnknown
				}
				return nil
			}
			got, err := c.CreateWorkspace(context.Background(), operator, r)
			if !errors.Is(err, sqlite.ErrUnknown) || got.Outcome != sqlite.InstallationUnknown {
				t.Fatalf("unknown=%+v err=%v", got, err)
			}
			query, err := c.GetInstallationOperation(context.Background(), operator, r.OperationID)
			if err != nil || query.Phase != phase || query.Outcome != sqlite.InstallationUnknown {
				t.Fatalf("durable phase=%+v err=%v", query, err)
			}
			c.afterPhase = nil
			again, err := c.CreateWorkspace(context.Background(), operator, r)
			if err != nil || again != query {
				t.Fatalf("same key replay/investigation=%+v %v", again, err)
			}
			if phase == sqlite.DescriptorPublished {
				if _, err := c.ReconcileInstallationOperation(context.Background(), operator, r.OperationID, sqlite.GenesisConfirmed); !errors.Is(err, sqlite.ErrInstallationOperationConflict) {
					t.Fatalf("stale phase confirmation accepted: %v", err)
				}
				unchanged, err := c.GetInstallationOperation(context.Background(), operator, r.OperationID)
				if err != nil || unchanged != query {
					t.Fatalf("stale confirmation mutated original facts: %+v %v", unchanged, err)
				}
			}
			reconciled, err := c.ReconcileInstallationOperation(context.Background(), operator, r.OperationID, phase)
			if phase == sqlite.DescriptorPublished {
				if err != nil || reconciled.Outcome != sqlite.InstallationCommitted {
					t.Fatalf("complete evidence not reconciled %+v %v", reconciled, err)
				}
			} else if err == nil {
				t.Fatal("partial creation repaired by reconcile")
			}
			if phase == sqlite.DirectoryPrepared {
				st, err := os.Stat(filepath.Join(r.RootLocator, "data", "workspace.sqlite"))
				if err != nil || st.Size() != 0 {
					t.Fatalf("failed fresh DB was seeded: %v", err)
				}
			}
		})
	}
}

// Cancellation is a request outcome, never permission to replay a durable intent.
// Check real residuals and OS lease release, not only a returned context error.
func TestWorkspaceCreationCancellationPreservesOriginalIntent(t *testing.T) {
	for _, phase := range []sqlite.InstallationPhase{sqlite.IntentRecorded, sqlite.DirectoryPrepared, sqlite.GenesisConfirmed, sqlite.DescriptorPublished} {
		t.Run(string(phase), func(t *testing.T) {
			f, c, operator, token, r := creationFixture(t)
			defer func() {
				if err := f.store.Close(); err != nil {
					t.Error(err)
				}
			}()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.afterPhase = func(p sqlite.InstallationPhase) error {
				if p == phase {
					cancel()
				}
				return nil
			}
			got, err := c.CreateWorkspace(ctx, operator, r)
			// The current-authorization boundary deliberately redacts storage/context
			// errors as unauthorized (auth.go currentLocked), rather than exposing IO.
			if !errors.Is(err, ErrUnauthorized) || got.Outcome != sqlite.InstallationRejected {
				t.Fatalf("cancellation outcome=%+v err=%v", got, err)
			}
			query, err := c.GetInstallationOperation(context.Background(), operator, r.OperationID)
			if err != nil || query.Phase != phase || query.Outcome != sqlite.InstallationRejected || query.WorkspaceID != got.WorkspaceID {
				t.Fatalf("canceled context suppressed original bookkeeping: %+v %v", query, err)
			}
			if _, err = f.auth.Authorize(context.Background(), token, query.WorkspaceID, "workspace.read"); err == nil {
				t.Fatal("canceled creation unexpectedly authorized")
			}
			c.afterPhase = nil
			again, err := c.CreateWorkspace(context.Background(), operator, r)
			if err != nil || again != query {
				t.Fatalf("same key replayed canceled operation: %+v %v", again, err)
			}
			if phase == sqlite.IntentRecorded {
				if _, err = os.Stat(r.RootLocator); !os.IsNotExist(err) {
					t.Fatalf("canceled intent created target: %v", err)
				}
				return
			}
			lease, err := platform.Acquire(r.RootLocator, platform.Workspace, false, false)
			if err != nil {
				t.Fatalf("canceled request retained drained writer lease: %v", err)
			}
			if err = lease.Close(); err != nil {
				t.Fatal(err)
			}
			if phase == sqlite.DirectoryPrepared {
				st, err := os.Stat(filepath.Join(r.RootLocator, "data", "workspace.sqlite"))
				if err != nil || st.Size() != 0 {
					t.Fatalf("canceled pre-genesis operation seeded data: %v", err)
				}
			}
		})
	}
}

func TestWorkspaceCreationNonemptyAndResidualAreNeverOverwritten(t *testing.T) {
	f, c, operator, _, r := creationFixture(t)
	defer f.store.Close()
	if err := os.Mkdir(r.RootLocator, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(r.RootLocator, "residual")
	if err := os.WriteFile(sentinel, []byte("keep synthetic artifact"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateWorkspace(context.Background(), operator, r); !errors.Is(err, platform.ErrDirectoryNotEmpty) {
		t.Fatalf("nonempty target accepted: %v", err)
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "keep synthetic artifact" {
		t.Fatal("artifact overwritten")
	}
	if _, err = c.ReconcileInstallationOperation(context.Background(), operator, r.OperationID, sqlite.DescriptorPublished); err == nil {
		t.Fatal("nonempty rejected target repaired")
	}
	if _, err = os.Stat(filepath.Join(r.RootLocator, ".aboard")); !os.IsNotExist(err) {
		t.Fatal("nonempty target modified")
	}
}
func TestWorkspaceCreationFinalConfirmationUnknownFencesSharedAuthorization(t *testing.T) {
	f, c, operator, token, r := creationFixture(t)
	c.afterPhase = func(p sqlite.InstallationPhase) error {
		if p == sqlite.RegistrationConfirmed {
			return sqlite.ErrUnknown
		}
		return nil
	}
	got, err := c.CreateWorkspace(context.Background(), operator, r)
	if !errors.Is(err, sqlite.ErrUnknown) || got.Outcome != sqlite.InstallationUnknown {
		t.Fatalf("lost final confirmation=%+v %v", got, err)
	}
	if err := operator.Check(context.Background()); !errors.Is(err, ErrAuthorizationUnknown) {
		t.Fatalf("shared authorization not fenced: %v", err)
	}
	if _, err := f.auth.Authorize(context.Background(), token, got.WorkspaceID, "workspace.read"); !errors.Is(err, ErrAuthorizationUnknown) {
		t.Fatalf("new workspace grant bypassed fence: %v", err)
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), f.install)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, _ := NewAuthenticator(s, []string{f.allowed})
	operator, err = a.Authorize(context.Background(), token, "", "installation.operator")
	if err != nil {
		t.Fatal(err)
	}
	c, _ = NewWorkspaceCreator(a)
	got, err = c.GetInstallationOperation(context.Background(), operator, r.OperationID)
	if err != nil || got.Outcome != sqlite.InstallationCommitted {
		t.Fatalf("fresh original outcome=%+v %v", got, err)
	}
}
func TestWorkspaceCreationCurrentAuthorizationBeforeGenesisAndFinalize(t *testing.T) {
	for _, phase := range []sqlite.InstallationPhase{sqlite.DirectoryPrepared, sqlite.DescriptorPublished} {
		t.Run(string(phase), func(t *testing.T) {
			f, c, operator, token, r := creationFixture(t)
			defer f.store.Close()
			hash, err := tokenHash(token)
			if err != nil {
				t.Fatal(err)
			}
			c.afterPhase = func(p sqlite.InstallationPhase) error {
				if p == phase {
					authFixtureSQL(t, f, `UPDATE client_session SET revoked=1 WHERE token_hash=?`, hash)
				}
				return nil
			}
			got, err := c.CreateWorkspace(context.Background(), operator, r)
			if err == nil {
				t.Fatal("revoked operator completed creation")
			}
			if _, err = f.auth.Authorize(context.Background(), token, got.WorkspaceID, "workspace.read"); err == nil {
				t.Fatal("revoked session gained workspace authorization")
			}
			if phase == sqlite.DirectoryPrepared {
				st, err := os.Stat(filepath.Join(r.RootLocator, "data", "workspace.sqlite"))
				if err != nil || st.Size() != 0 {
					t.Fatal("genesis ran after revocation")
				}
			}
		})
	}
}
func TestWorkspaceCreationStoreCloseAndSharedWrapperSerialization(t *testing.T) {
	f, c, operator, _, r := creationFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	c.afterPhase = func(p sqlite.InstallationPhase) error {
		if p == sqlite.DirectoryPrepared {
			close(entered)
			<-release
		}
		return nil
	}
	second, _ := NewWorkspaceCreator(f.auth)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := c.CreateWorkspace(context.Background(), operator, r)
		results <- err
	}()
	<-entered
	if err := f.store.Close(); !errors.Is(err, ErrWorkspaceCreationPending) {
		t.Fatalf("Store released live creation: %v", err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := second.CreateWorkspace(context.Background(), operator, r)
		results <- err
	}()
	close(release)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Error(err)
		}
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
}

// This is an explicit failure-branch control, not a claim that Windows failed
// Close on a valid live SQLite handle. Real FS/lease retention is still checked.
func TestWorkspaceCreationNilDatabasePhysicalUnknownRetainsRealLease(t *testing.T) {
	f, c, operator, _, r := creationFixture(t)
	w, err := c.owner(context.Background(), operator, r.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	w.creation, err = platform.InspectWorkspaceCreation(r.RootLocator)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.creation.Prepare(); err != nil {
		t.Fatal(err)
	}
	w.openUnknown = true
	w.mu.Unlock()
	second, _ := NewWorkspaceCreator(f.auth)
	if err = f.store.Close(); !errors.Is(err, ErrWorkspaceCreationPending) {
		t.Fatalf("nilDB released installation: %v", err)
	}
	if err = second.DrainWorkspaceCreation(context.Background(), operator, r.OperationID); !errors.Is(err, sqlite.ErrPhysicalCloseUnknown) {
		t.Fatalf("nilDB drained without evidence: %v", err)
	}
	if other, err := platform.Acquire(r.RootLocator, platform.Workspace, false, false); err == nil {
		other.Close()
		t.Fatal("nilDB unknown released real writer lock")
	}
	// Fixture-only cleanup: this branch stimulus never opened SQLite, so the test
	// knows no physical database IO exists. Production has no such clearing API.
	w.mu.Lock()
	w.openUnknown = false
	if err = c.drain(w); err != nil {
		t.Error(err)
	}
	w.mu.Unlock()
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestWorkspaceCreationCrashHelper(t *testing.T) {
	if os.Getenv("ABOARD_T013_CRASH_CHILD") != "1" {
		return
	}
	ctx := context.Background()
	install, allowed := os.Getenv("ABOARD_T013_INSTALL"), os.Getenv("ABOARD_T013_ALLOWED")
	s, err := Open(ctx, install)
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAuthenticator(s, []string{allowed})
	if err != nil {
		t.Fatal(err)
	}
	f := authFixture{store: s, auth: a, install: install, allowed: allowed}
	operator, _ := authOperator(t, f)
	c, _ := NewWorkspaceCreator(a)
	c.afterPhase = func(p sqlite.InstallationPhase) error {
		if string(p) == os.Getenv("ABOARD_T013_PHASE") {
			os.Exit(0)
		}
		return nil
	}
	if _, err = c.CreateWorkspace(ctx, operator, CreateWorkspaceRequest{creationOperation, filepath.Join(allowed, "created"), "synthetic workspace"}); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash checkpoint not reached")
}
func TestWorkspaceCreationColdProcessInvestigationAndReconcile(t *testing.T) {
	for _, phase := range []sqlite.InstallationPhase{sqlite.GenesisConfirmed, sqlite.DescriptorPublished} {
		t.Run(string(phase), func(t *testing.T) {
			f, _, _, _, r := creationFixture(t)
			if err := f.store.Close(); err != nil {
				t.Fatal(err)
			}
			child := exec.Command(os.Args[0], "-test.run=^TestWorkspaceCreationCrashHelper$")
			child.Env = append(os.Environ(), "ABOARD_T013_CRASH_CHILD=1", "ABOARD_T013_INSTALL="+f.install, "ABOARD_T013_ALLOWED="+f.allowed, "ABOARD_T013_PHASE="+string(phase))
			if out, err := child.CombinedOutput(); err != nil {
				t.Fatalf("child failed: %v %s", err, out)
			}
			s, err := Open(context.Background(), f.install)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			a, _ := NewAuthenticator(s, []string{f.allowed})
			f.store, f.auth = s, a
			operator, _ := authOperator(t, f)
			c, _ := NewWorkspaceCreator(a)
			got, err := c.GetInstallationOperation(context.Background(), operator, r.OperationID)
			if err != nil || got.Phase != phase || got.Outcome != sqlite.InstallationAccepted {
				t.Fatalf("cold investigation=%+v err=%v", got, err)
			}
			if _, err = c.CreateWorkspace(context.Background(), operator, r); err != nil {
				t.Fatal(err)
			}
			got, err = c.ReconcileInstallationOperation(context.Background(), operator, r.OperationID, phase)
			if phase == sqlite.DescriptorPublished {
				if err != nil || got.Outcome != sqlite.InstallationCommitted {
					t.Fatalf("cold reconcile=%+v %v", got, err)
				}
			} else {
				if err == nil {
					t.Fatal("missing descriptor repaired")
				}
				if _, err = os.Stat(filepath.Join(r.RootLocator, "aboard.workspace.json")); !os.IsNotExist(err) {
					t.Fatal("cold investigation created missing descriptor")
				}
			}
		})
	}
}
func TestWorkspaceCreationReconcileRejectsPhysicalSubstitutionAndMissingDB(t *testing.T) {
	for _, substitute := range []string{"root", "database", "descriptor", "missing-database"} {
		t.Run(substitute, func(t *testing.T) {
			f, c, operator, _, r := creationFixture(t)
			defer f.store.Close()
			c.afterPhase = func(p sqlite.InstallationPhase) error {
				if p == sqlite.DescriptorPublished {
					return sqlite.ErrUnknown
				}
				return nil
			}
			if _, err := c.CreateWorkspace(context.Background(), operator, r); err == nil {
				t.Fatal("fault missing")
			}
			switch substitute {
			case "root":
				if err := os.Rename(r.RootLocator, r.RootLocator+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(r.RootLocator, 0700); err != nil {
					t.Fatal(err)
				}
			case "database", "descriptor":
				path := filepath.Join(r.RootLocator, "aboard.workspace.json")
				if substitute == "database" {
					path = filepath.Join(r.RootLocator, "data", "workspace.sqlite")
				}
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-database":
				path := filepath.Join(r.RootLocator, "data", "workspace.sqlite")
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := c.ReconcileInstallationOperation(context.Background(), operator, r.OperationID, sqlite.DescriptorPublished); err == nil {
				t.Fatal("substitution accepted")
			}
			if substitute == "missing-database" {
				if _, err := os.Stat(filepath.Join(r.RootLocator, "data", "workspace.sqlite")); !os.IsNotExist(err) {
					t.Fatal("missing DB reseeded")
				}
			}
		})
	}
}
