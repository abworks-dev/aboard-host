//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
)

// Configuration admission only, not native storage/commit fault evidence.
func TestAcceptanceCreationProbeHardGateAndRequestLocalCopy(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	parent := filepath.Join(filepath.Dir(file), "..", "..", ".data", "logs", "001-validation")
	root, err := os.MkdirTemp(parent, "t023-probe-")
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Clean(root)
	t.Cleanup(func() {
		if creationProbeRoot(root) {
			if err := os.RemoveAll(root); err != nil {
				t.Error(err)
			}
		}
	})
	key := "00112233-4455-4677-8899-aabbccddeeff"
	r := CreateWorkspaceRequest{OperationID: key, RootLocator: filepath.Join(root, "allowed", "stage-target"), DisplayName: "synthetic"}
	manual := 0
	c := &WorkspaceCreator{auth: &Authenticator{store: &Store{root: filepath.Join(root, "installation")}}, afterPhase: func(sqlite.InstallationPhase) error { manual++; return nil }}
	config := creationProbe{Version: 1, Fixture: root, OperationID: key, Phase: string(sqlite.IntentRecorded)}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	set := func(data []byte) {
		t.Helper()
		t.Setenv(creationProbeEnvironment, base64.RawURLEncoding.EncodeToString(data))
	}
	set(raw)
	local, err := creationRequestCreator(c, r)
	if err != nil || local == c || local == nil || local.auth != c.auth {
		t.Fatal("valid probe did not preserve unique authenticated owner")
	}
	if err = local.afterPhase(sqlite.DirectoryPrepared); err != nil || manual != 0 {
		t.Fatal("request copy mutated creator or blocked unmatched phase")
	}
	if err = c.afterPhase(sqlite.DirectoryPrepared); err != nil || manual != 1 {
		t.Fatal("original creator callback changed")
	}
	other := r
	other.OperationID = "10112233-4455-4677-8899-aabbccddeeff"
	if got, e := creationRequestCreator(c, other); e != nil || got != c {
		t.Fatal("unrelated original key inherited another request hook")
	}
	badTarget := r
	badTarget.RootLocator = filepath.Join(root, "allowed", "other")
	if _, e := creationRequestCreator(c, badTarget); e == nil {
		t.Fatal("matched key escaped fixed synthetic target")
	}
	invalid := [][]byte{
		[]byte("null"), []byte(`{"version":1}`), []byte(`{"version":1,"Fixture":null,"OperationID":"` + key + `","Phase":"intentRecorded"}`),
		[]byte(strings.Replace(string(raw), `"version":1`, `"version":1e0`, 1)),
		[]byte(strings.Replace(string(raw), `"Fixture"`, `"fixture"`, 1)),
		[]byte(strings.Replace(string(raw), `"intentRecorded"`, `"beforeAnySQL"`, 1)),
		[]byte(strings.Replace(string(raw), key, "not-an-id", 1)),
	}
	outside := config
	outside.Fixture = filepath.Dir(root)
	badRoot, _ := json.Marshal(outside)
	invalid = append(invalid, badRoot)
	wrong := &WorkspaceCreator{auth: &Authenticator{store: &Store{root: filepath.Join(root, "other-installation")}}}
	set(raw)
	if _, e := creationRequestCreator(wrong, r); e == nil {
		t.Fatal("different installation accepted")
	}
	for n, b := range invalid {
		set(b)
		if _, e := creationRequestCreator(c, r); e == nil {
			t.Fatalf("invalid fixture accepted: %d", n)
		}
	}
	t.Setenv(creationProbeEnvironment, strings.Repeat("x", 8193))
	if _, e := creationRequestCreator(c, r); e == nil {
		t.Fatal("unbounded config accepted")
	}
	if _, err = os.Stat(filepath.Join(root, "creation-checkpoint.json")); !os.IsNotExist(err) {
		t.Fatal("admission emitted a checkpoint without an actual operation")
	}
}
