//go:build windows && amd64

package acceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

const stageExecutableEnvironment = "ABOARD_ACCEPTANCE_STAGE_EXE"

type creationProbeConfig struct {
	Version                     int `json:"version"`
	Fixture, OperationID, Phase string
}
type observedCheckpoint struct {
	Version     int    `json:"version"`
	PID         int    `json:"pid"`
	OperationID string `json:"operationId"`
	Phase       string `json:"phase"`
}

func creationPhase(phase string) bool {
	switch phase {
	case "intentRecorded", "directoryPrepared", "genesisConfirmed", "descriptorPublished", "registrationConfirmed":
		return true
	}
	return false
}
func readCheckpoint(root string) (observedCheckpoint, error) {
	var out observedCheckpoint
	ready, err := os.Lstat(filepath.Join(root, "creation-checkpoint.ready"))
	if err != nil {
		return out, err
	}
	if !ready.Mode().IsRegular() || ready.Size() != 0 {
		return out, errors.New("invalid readiness marker")
	}
	file, err := os.Open(filepath.Join(root, "creation-checkpoint.json"))
	if err != nil {
		return out, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, 4097))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return out, err
	}
	if len(raw) > 4096 || json.Unmarshal(raw, &out) != nil || out.Version != 1 {
		return out, errors.New("invalid creation checkpoint")
	}
	return out, nil
}
func faultPhaseAllowed(in faultInput) bool {
	if in.Phase == "known-idle-termination" || in.Phase == "unresolved-installation-cleanup" {
		return in.OperationID == ""
	}
	if in.Phase == "save-full" {
		return fullSaveFaultAllowed(in)
	}
	if in.Phase == "save-busy" {
		return busySaveFaultAllowed(in)
	}
	if strings.HasPrefix(in.Phase, "save-checkpoint-") {
		return saveCheckpointFaultAllowed(in)
	}
	if in.Phase == "save-native-admission" || in.Phase == "save-native-formal" {
		return nativeSaveFaultAllowed(in)
	}
	phase := strings.TrimPrefix(in.Phase, "creation-")
	if phase == in.Phase || !creationPhase(phase) || in.OperationID == "" {
		return false
	}
	marker, err := readCheckpoint(in.Fixture)
	return err == nil && marker.PID == in.PID && marker.OperationID == in.OperationID && marker.Phase == phase
}
func stagedFixture(t *testing.T, original *fixture, operation protocol.UUID, phase string) *fixture {
	t.Helper()
	image := os.Getenv(stageExecutableEnvironment)
	if image == "" {
		t.Skip("scoped acceptance build not configured: " + stageExecutableEnvironment)
	}
	hash, err := checkedExecutable(image)
	if err != nil {
		t.Fatal(err)
	}
	copy := *original
	copy.executable = image
	copy.sha = hash
	config := creationProbeConfig{Version: 1, Fixture: copy.root, OperationID: string(operation), Phase: phase}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	copy.environment = []string{"ABOARD_ACCEPTANCE_CREATION=" + base64.RawURLEncoding.EncodeToString(raw)}
	return &copy
}

type lostResponse struct {
	status int
	err    error
}

func beginCreate(t *testing.T, c *client, in protocol.CreateWorkspace) <-chan lostResponse {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest("POST", "http://"+c.authority+"/v1/workspace-creations", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	done := make(chan lostResponse, 1)
	go func() {
		reply, e := c.http.Do(request)
		result := lostResponse{err: e}
		if reply != nil {
			result.status = reply.StatusCode
			_, _ = io.Copy(io.Discard, io.LimitReader(reply.Body, 1<<20))
			_ = reply.Body.Close()
		}
		done <- result
	}()
	return done
}
func waitCreationCheckpoint(t *testing.T, p *process, key protocol.UUID, phase string, response <-chan lostResponse) {
	t.Helper()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		marker, err := readCheckpoint(p.fixture.root)
		if err == nil {
			if marker.PID != p.cmd.Process.Pid || marker.OperationID != string(key) || marker.Phase != phase {
				t.Fatal("checkpoint is not the actual original operation")
			}
			return
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("checkpoint marker invalid")
		}
		select {
		case ended := <-response:
			t.Fatalf("creation ended before durable stop: status=%d transportError=%t", ended.status, ended.err != nil)
		case <-p.done:
			t.Fatal("stage Host ended before durable checkpoint")
		case <-deadline.C:
			t.Fatal("stage checkpoint not reached")
		case <-ticker.C:
		}
	}
}

// Exclusion is already ended when used; no source file is read through an owned
// SQLite handle. Directories and every byte are included (not just main DB).
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("synthetic source unexpectedly aliased")
		}
		if d.IsDir() {
			out[relative] = "directory"
			return nil
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		sum := sha256.Sum256(raw)
		out[relative] = stringHex(sum[:])
		return nil
	}); err != nil {
		t.Fatal("cannot inventory synthetic source", err)
	}
	return out
}
func stringHex(raw []byte) string {
	const alphabet = "0123456789abcdef"
	b := make([]byte, 2*len(raw))
	for n, v := range raw {
		b[n*2] = alphabet[v>>4]
		b[n*2+1] = alphabet[v&15]
	}
	return string(b)
}

// Each subtest is a genuinely new Host killed after a real coordinator commit.
// Accepted is NOT renamed unknown; this does not inject native SQLite COMMIT
// failure, cover every V008 case, or claim hardware power-loss atomicity.
func TestV008CreationDurablePhaseCrashes(t *testing.T) {
	for _, phase := range []string{"intentRecorded", "directoryPrepared", "genesisConfirmed", "descriptorPublished", "registrationConfirmed"} {
		t.Run(phase, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			key := newID(t)
			root := filepath.Join(f.config.AllowedRoots[0], "stage-target")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			initial := tree(t, root)
			stage := stagedFixture(t, f, key, phase)
			p := stage.start(t)
			c := p.connect(t)
			input := protocol.CreateWorkspace{OperationId: key, Path: protocol.Path(root), DisplayName: "original staged creation"}
			response := beginCreate(t, c, input)
			waitCreationCheckpoint(t, p, key, phase, response)
			p.terminateAt(t, "creation-"+phase, string(key))
			select {
			case lost := <-response:
				if lost.status >= 200 && lost.status < 300 {
					t.Fatal("parked creation published success before interrupted delivery")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("old HTTP waiter outlived actual process end")
			}
			before := tree(t, root)
			if phase == "intentRecorded" && !reflect.DeepEqual(initial, before) {
				t.Fatal("intent-only phase mutated source")
			}
			if phase != "intentRecorded" {
				lease, err := platform.Acquire(root, platform.Workspace, false, false)
				if err != nil {
					t.Fatal("killed stage still owns writer")
				}
				if err = lease.Close(); err != nil {
					t.Fatal(err)
				}
			}
			next := f.start(t)
			d := next.connect(t)
			path := "/v1/installation-operations/" + string(key)
			var original protocol.OperationView
			d.request(t, "GET", path, nil, &original, 200)
			expectedLogical := "accepted"
			if phase == "registrationConfirmed" {
				expectedLogical = "committed"
			}
			if string(original.LogicalState) != expectedLogical || original.WorkspaceId == nil || original.OperationId != key {
				t.Fatal("cold original operation lost phase outcome or frozen identity")
			}
			// Same key is inquiry-only, never a new create even when target is incomplete.
			if phase != "registrationConfirmed" {
				var duplicate protocol.OperationView
				d.request(t, "POST", "/v1/workspace-creations", input, &duplicate, 202)
				if duplicate.OperationId != key || duplicate.WorkspaceId == nil || *duplicate.WorkspaceId != *original.WorkspaceId {
					t.Fatal("same key reseeded identity")
				}
			}
			var sameKey protocol.ErrorView
			changed := input
			changed.DisplayName = "different semantic input"
			d.request(t, "POST", "/v1/workspace-creations", changed, &sameKey, 409)
			if sameKey.Code != protocol.ErrorCodeOPERATIONKEYREUSE {
				t.Fatal("changed input not rejected as original-key conflict")
			}
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("cold query/duplicate mutated original source")
			}
			if phase == "intentRecorded" || phase == "directoryPrepared" || phase == "genesisConfirmed" {
				var failure protocol.ErrorView
				d.request(t, "POST", path+"/reconcile", protocol.ReconcileInstallation{ExpectedPhase: protocol.InstallationPhase(phase)}, &failure, 503)
				if failure.Code != protocol.ErrorCodeREGISTRATIONPENDING {
					t.Fatal("incomplete original was not kept pending")
				}
				// No registration/grant exists for this original UUID. This is a
				// deliberately invalid unregistered-open precondition, not a guessed
				// current registration version or permission inferred from a path.
				var unopened protocol.ErrorView
				d.request(t, "POST", "/v1/workspaces/"+string(*original.WorkspaceId)+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: "9223372036854775807"}, &unopened, 403)
				if unopened.Code != protocol.ErrorCodeFORBIDDEN {
					t.Fatal("incomplete UUID acquired open authority")
				}
				var principal protocol.PrincipalView
				d.request(t, "GET", "/v1/context", nil, &principal, 200)
				descriptor, e := workspace.NewDirectoryDescriptor(string(*original.WorkspaceId), string(principal.TenantId))
				if e != nil {
					t.Fatal(e)
				}
				registerKey := newID(t)
				var registrationFailure protocol.ErrorView
				registerStatus := d.request(t, "POST", "/v1/workspace-registrations", protocol.RegisterWorkspace{OperationId: registerKey, Path: protocol.Path(root), ExpectedWorkspaceId: *original.WorkspaceId, ExpectedIdentityDigest: protocol.Digest(descriptor.IdentityDigest)}, &registrationFailure, 0)
				if registerStatus != 409 && registerStatus != 503 {
					t.Fatal("incomplete residual not cleanly refused by register", registerStatus)
				}
				var absentRegistration protocol.ErrorView
				d.request(t, "GET", "/v1/installation-operations/"+string(registerKey), nil, &absentRegistration, 404)
				// Changing the key never grants a second creation over original intent.
				freshInput := input
				freshInput.OperationId = newID(t)
				var freshRefusal protocol.ErrorView
				d.request(t, "POST", "/v1/workspace-creations", freshInput, &freshRefusal, 409)
				if freshRefusal.Code != protocol.ErrorCodeOPERATIONKEYREUSE {
					t.Fatal("new key bypassed retained original target")
				}
				var absentFresh protocol.ErrorView
				d.request(t, "GET", "/v1/installation-operations/"+string(freshInput.OperationId), nil, &absentFresh, 404)
				var inspection protocol.ErrorView
				status := d.request(t, "POST", "/v1/workspace-inspections", protocol.InspectWorkspace{Path: protocol.Path(root)}, &inspection, 0)
				if status >= 200 && status < 300 {
					t.Fatal("incomplete source falsely inspected as complete")
				}
				if !reflect.DeepEqual(before, tree(t, root)) {
					t.Fatal("reconcile/inspection repaired incomplete original")
				}
				var unchanged protocol.OperationView
				d.request(t, "GET", path, nil, &unchanged, 200)
				if unchanged.LogicalState != original.LogicalState || unchanged.WorkspaceId == nil || *unchanged.WorkspaceId != *original.WorkspaceId {
					t.Fatal("failed investigation changed original intent")
				}
				next.terminateAt(t, "unresolved-installation-cleanup", "") // explicit test cleanup, not graceful proof
			} else {
				var preReconcile protocol.WorkspaceView
				if phase == "descriptorPublished" {
					var inaccessible protocol.ErrorView
					d.request(t, "GET", "/v1/workspaces/"+string(*original.WorkspaceId), nil, &inaccessible, 403)
				}
				var reconciled protocol.OperationView
				d.request(t, "POST", path+"/reconcile", protocol.ReconcileInstallation{ExpectedPhase: protocol.InstallationPhase(phase)}, &reconciled, 200)
				if reconciled.LogicalState != "committed" || reconciled.WorkspaceId == nil || *reconciled.WorkspaceId != *original.WorkspaceId {
					t.Fatal("reconcile did not finalize ONLY original identity")
				}
				if !reflect.DeepEqual(before, tree(t, root)) {
					t.Fatal("registration finalize changed source genesis/descriptor")
				}
				d.request(t, "GET", "/v1/workspaces/"+string(*original.WorkspaceId), nil, &preReconcile, 200)
				opened := d.open(t, *original.WorkspaceId)
				if opened.OpenEpoch == nil || opened.Revision == nil || *opened.Revision != "0" {
					t.Fatal("original registered genesis not usable")
				}
				next.stop(t)
			}
			after := tree(t, root)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("final open/close window changed original source")
			}
			evidence := struct {
				Phase         string
				OperationID   protocol.UUID
				WorkspaceID   *protocol.UUID
				Before, After map[string]string
				Outcome       string
			}{phase, key, original.WorkspaceId, before, after, expectedLogical}
			raw, err := json.MarshalIndent(evidence, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(f.root, "creation-evidence.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			t.Logf("phase=%s original=%s cold=%s original source conserved; actual process termination/new Host investigation", phase, key, expectedLogical)
		})
	}
}
