//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
)

func TestAcceptanceFinalizeDeferredCleanupPreservesOriginalEvidence(t *testing.T) {
	sentinel := errors.New("original adapter failure")
	cleanupErr := errors.New("original cleanup failure")
	for _, mode := range []string{"ordinary", "panic-string", "panic-nil"} {
		state := &finalizeFlushState{active: true}
		if !armedFinalize.CompareAndSwap(nil, state) {
			t.Fatal("another probe active")
		}
		closed := 0
		result, closeErr := callFinalizeProbe(state, func() error {
			if mode == "panic-string" {
				panic("synthetic policy-only panic")
			}
			if mode == "panic-nil" {
				panic(nil)
			}
			return sentinel
		}, func() error {
			closed++
			if state.active || armedFinalize.Load() != nil {
				t.Error("stimulus closed before disarm/drain")
			}
			return cleanupErr
		})
		if closed != 1 || closeErr != cleanupErr {
			t.Fatal("cleanup skipped/retried/rewritten")
		}
		if mode != "ordinary" {
			if !errors.Is(result, sqlite.ErrUnknown) {
				t.Fatal("panic fabricated known noncommit")
			}
		} else if result != sentinel {
			t.Fatal("original adapter error replaced")
		}
		if state.calls != 0 || state.injected != 0 {
			t.Fatal("policy fixture fabricated native evidence")
		}
	}
}

// Configuration/lifecycle guards only; actual native finalization fault evidence is
// supplied by the external Host test, never by these policy-only fixtures.
func TestAcceptanceFinalizeProbeStrictRequestAndOwner(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	parent := filepath.Join(filepath.Dir(file), "..", "..", ".data", "logs", "001-validation")
	if e := os.MkdirAll(parent, 0700); e != nil {
		t.Fatal(e)
	}
	root, e := os.MkdirTemp(parent, "t023-finalize-guard-")
	if e != nil {
		t.Fatal(e)
	}
	root = filepath.Clean(root)
	t.Cleanup(func() {
		if creationProbeRoot(root) {
			if e := os.RemoveAll(root); e != nil {
				t.Error(e)
			}
		}
	})
	t.Setenv(creationProbeEnvironment, "")
	key := "00112233-4455-4677-8899-aabbccddeeff"
	config := finalizeProbe{Version: 1, Fixture: root, OperationID: key, Ordinal: 1}
	raw, e := json.Marshal(config)
	if e != nil {
		t.Fatal(e)
	}
	set := func(b []byte) {
		t.Helper()
		t.Setenv(finalizeProbeEnvironment, base64.RawURLEncoding.EncodeToString(b))
	}
	c := &WorkspaceCreator{auth: &Authenticator{store: &Store{root: filepath.Join(root, "installation")}}}
	r := CreateWorkspaceRequest{OperationID: key, RootLocator: filepath.Join(root, "allowed", "finalize-target")}
	if finalizeDelegateError != nil {
		t.Fatal("permanent fixed-ABI delegate not installed")
	}
	set(raw)
	got, matched, e := readFinalizeProbe(c, r)
	if e != nil || !matched || got != config {
		t.Fatal("valid exact request guard refused")
	}
	if local, e := creationRequestCreator(c, r); e != nil || local != c {
		t.Fatal("native fixture changed sole creator/auth owner")
	}
	other := r
	other.OperationID = "10112233-4455-4677-8899-aabbccddeeff"
	if _, matched, e := readFinalizeProbe(c, other); e != nil || matched {
		t.Fatal("another operation inherited native fault")
	}
	target := r
	target.RootLocator = filepath.Join(root, "allowed", "elsewhere")
	if _, _, e := readFinalizeProbe(c, target); e == nil {
		t.Fatal("matched key escaped fixed native target")
	}
	invalid := [][]byte{
		[]byte(`null`), []byte(`{}`), []byte(strings.Replace(string(raw), `"version":1`, `"version":1e0`, 1)),
		[]byte(strings.Replace(string(raw), `"Ordinal":1`, `"Ordinal":1e0`, 1)),
		[]byte(strings.Replace(string(raw), `"Ordinal":1`, `"Ordinal":-1`, 1)),
		[]byte(strings.Replace(string(raw), `"Ordinal":1`, `"Ordinal":9`, 1)),
		[]byte(strings.Replace(string(raw), `"Ordinal":1`, `"Ordinal":null`, 1)),
		[]byte(strings.Replace(string(raw), `"Fixture"`, `"fixture"`, 1)),
		[]byte(strings.Replace(string(raw), key, "not-a-uuid", 1)),
	}
	outside := config
	outside.Fixture = filepath.Dir(root)
	b, _ := json.Marshal(outside)
	invalid = append(invalid, b)
	for n, b := range invalid {
		set(b)
		if _, _, e := readFinalizeProbe(c, r); e == nil {
			t.Fatalf("malformed native configuration accepted %d", n)
		}
	}
	t.Setenv(finalizeProbeEnvironment, strings.Repeat("x", 8193))
	if _, _, e := readFinalizeProbe(c, r); e == nil {
		t.Fatal("unbounded native config accepted")
	}
	set(raw)
	t.Setenv(creationProbeEnvironment, "also-armed")
	if _, _, e := readFinalizeProbe(c, r); e == nil {
		t.Fatal("two incompatible stimuli accepted")
	}
	t.Setenv(creationProbeEnvironment, "")
	wrong := &WorkspaceCreator{auth: &Authenticator{store: &Store{root: filepath.Join(root, "other-installation")}}}
	if _, _, e := readFinalizeProbe(wrong, r); e == nil {
		t.Fatal("wrong installation accepted")
	}
	calls := 0
	op := sqlite.DirectoryOperation{InstallationOperation: sqlite.InstallationOperation{OperationID: key, Kind: "create", TargetLocator: r.RootLocator, Phase: string(sqlite.DescriptorPublished), Outcome: string(sqlite.InstallationAccepted)}}
	if e := creationFinalize(c, nil, op, false, func() error { calls++; return nil }); e == nil || calls != 0 {
		t.Fatal("synthetic non-owned connection armed or invoked finalize")
	}
	if _, e := os.Stat(filepath.Join(root, "finalize-evidence.json")); !os.IsNotExist(e) {
		t.Fatal("guard tests fabricated native evidence")
	}
}
