//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAcceptanceSaveProbeStrictScopeAndQuarantine(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	parent := filepath.Join(filepath.Dir(file), "..", "..", ".data", "logs", "001-validation")
	if os.MkdirAll(parent, 0700) != nil {
		t.Fatal("guard parent unavailable")
	}
	root, e := os.MkdirTemp(parent, "t023-save-guard-")
	if e != nil {
		t.Fatal(e)
	}
	root = filepath.Clean(root)
	defer func() {
		if creationProbeRoot(root) {
			if e := os.RemoveAll(root); e != nil {
				t.Error(e)
			}
		}
	}()
	t.Setenv(creationProbeEnvironment, "")
	t.Setenv(finalizeProbeEnvironment, "")
	cfg := saveProbe{Version: 1, Fixture: root, WorkspaceID: "00112233-4455-4677-8899-aabbccddeeff", PrincipalID: "10112233-4455-4677-8899-aabbccddeeff", OperationID: "20112233-4455-4677-8899-aabbccddeeff", Phase: "admission", Ordinal: 1}
	raw, _ := json.Marshal(cfg)
	set := func(b []byte) { t.Setenv(saveProbeEnvironment, base64.RawURLEncoding.EncodeToString(b)) }
	r := &openResources{store: &Store{root: filepath.Join(root, "installation")}}
	other := workspace.SaveRequest{WorkspaceID: cfg.WorkspaceID, OperationID: "30112233-4455-4677-8899-aabbccddeeff"}
	set(raw)
	got, matched, e := readSaveProbe(r, other)
	if e != nil || matched || got != cfg {
		t.Fatal("unmatched operation inherited probe or parse changed config")
	}
	request := other
	request.OperationID = cfg.OperationID
	request.PrincipalID = cfg.PrincipalID
	if _, _, e := readSaveProbe(r, request); e == nil {
		t.Fatal("non-owned native workspace armed")
	}
	malformed := [][]byte{[]byte(`null`), []byte(`{}`), []byte(strings.Replace(string(raw), `"version":1`, `"version":1e0`, 1)), []byte(strings.Replace(string(raw), `"Ordinal":1`, `"Ordinal":1e0`, 1)), []byte(strings.Replace(string(raw), `"Ordinal":1`, `"Ordinal":9`, 1)), []byte(strings.Replace(string(raw), `"Ordinal":1`, `"Ordinal":null`, 1)), []byte(strings.Replace(string(raw), `"Fixture"`, `"fixture"`, 1)), []byte(strings.Replace(string(raw), cfg.WorkspaceID, "invalid-uuid", 1)), []byte(strings.Replace(string(raw), `"Phase":"admission"`, `"Phase":"query"`, 1))}
	malformed = append(malformed, []byte(strings.Replace(string(raw), `"Phase":"admission"`, `"Phase":"full"`, 1))) // full only accepts ordinal0
	malformed = append(malformed, []byte(strings.Replace(string(raw), `"Phase":"admission"`, `"Phase":"busy"`, 1))) // busy only accepts ordinal0
	for _, phase := range []string{"before-admit", "after-admit", "after-confirm", "formal-inflight"} {
		positive := cfg
		positive.Phase, positive.Ordinal = phase, 0
		encoded, _ := json.Marshal(positive)
		set(encoded)
		parsed, armed, parseErr := readSaveProbe(r, other)
		if parseErr != nil || armed || parsed != positive || holdSaveCheckpoint(r, other, phase, nil) != nil {
			t.Fatal("ordinal0 checkpoint grammar or unmatched-operation isolation changed")
		}
		if _, existsErr := os.Stat(filepath.Join(root, "save-checkpoint.ready")); !os.IsNotExist(existsErr) {
			t.Fatal("unmatched checkpoint published a marker")
		}
		malformed = append(malformed, []byte(strings.Replace(string(raw), `"Phase":"admission"`, `"Phase":"`+phase+`"`, 1))) // checkpoints require ordinal0
	}
	for i, b := range malformed {
		set(b)
		if _, _, e := readSaveProbe(r, other); e == nil {
			t.Fatalf("invalid exact frame accepted %d", i)
		}
	}
	t.Setenv(saveProbeEnvironment, strings.Repeat("x", 8193))
	if _, _, e := readSaveProbe(r, other); e == nil {
		t.Fatal("oversize accepted")
	}
	set(raw)
	t.Setenv(finalizeProbeEnvironment, "also-armed")
	if _, _, e := readSaveProbe(r, other); e == nil {
		t.Fatal("second fault owner accepted")
	}
	t.Setenv(finalizeProbeEnvironment, "")
	wrong := *r
	wrong.store = &Store{root: filepath.Join(root, "elsewhere")}
	if _, _, e := readSaveProbe(&wrong, other); e == nil {
		t.Fatal("foreign installation accepted")
	}
	calls := 0
	t.Setenv(saveProbeEnvironment, "")
	result, e := workspaceSaveCommit(context.Background(), r, request, nil, func() (workspace.SaveResult, error) {
		calls++
		return workspace.SaveResult{Status: workspace.SaveConfirmed}, nil
	})
	if calls != 1 || e != nil || result.Status != workspace.SaveConfirmed {
		t.Fatal("unarmed tagged wrapper did not delegate once")
	}
	marker := &finalizeCloseError{cause: errors.New("policy-only close-unknown"), owner: new(int)}
	retainSaveProbeFailure(r, marker)
	if !r.quarantined || !r.pending || !errors.Is(r.nativeFailure, platform.ErrPhysicalCloseUnknown) || r.Close() == nil || r.Validate() == nil {
		t.Fatal("probe cleanup uncertainty lost physical owner")
	}
	storageUnknown := &openResources{}
	retainSaveProbeFailure(storageUnknown, errors.Join(sqlite.ErrPhysicalCloseUnknown, errors.New("policy-only reader close unknown")))
	if !storageUnknown.quarantined || !storageUnknown.pending || !errors.Is(storageUnknown.nativeFailure, sqlite.ErrPhysicalCloseUnknown) || storageUnknown.Close() == nil {
		t.Fatal("storage reader close unknown lost original owner")
	}
	if _, e := os.Stat(filepath.Join(root, "save-admission-evidence.json")); !os.IsNotExist(e) {
		t.Fatal("guard fixture fabricated native evidence")
	}
}
