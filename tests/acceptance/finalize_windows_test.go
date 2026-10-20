//go:build windows && amd64

package acceptance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

type finalizeConfig struct {
	Version              int `json:"version"`
	Fixture, OperationID string
	Ordinal              int
}
type nativeFinalizeEvidence struct {
	Version                                          int    `json:"version"`
	PID                                              int    `json:"pid"`
	OperationID                                      string `json:"operationId"`
	Ordinal, Calls, Injected, NativeBOOL, SQLiteCode int
	Unknown, NativeControlFailed, CallbackDrained    bool
}

func finalizeFixture(t *testing.T, f *fixture, key protocol.UUID, ordinal int) *fixture {
	t.Helper()
	image := os.Getenv(stageExecutableEnvironment)
	if image == "" {
		t.Skip("scoped acceptance build not configured: " + stageExecutableEnvironment)
	}
	hash, e := checkedExecutable(image)
	if e != nil {
		t.Fatal(e)
	}
	copy := *f
	copy.executable = image
	copy.sha = hash
	raw, e := json.Marshal(finalizeConfig{Version: 1, Fixture: f.root, OperationID: string(key), Ordinal: ordinal})
	if e != nil {
		t.Fatal(e)
	}
	copy.environment = []string{"ABOARD_ACCEPTANCE_FINALIZE=" + base64.RawURLEncoding.EncodeToString(raw)}
	return &copy
}
func readFinalizeEvidence(t *testing.T, f *fixture) nativeFinalizeEvidence {
	t.Helper()
	var out nativeFinalizeEvidence
	file, e := os.Open(filepath.Join(f.root, "finalize-evidence.json"))
	if e != nil {
		t.Fatal("native finalize evidence missing")
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, 4097))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(raw) > 4096 || json.Unmarshal(raw, &out) != nil || out.Version != 1 {
		t.Fatal("invalid native finalize evidence")
	}
	return out
}
func operationPrecondition(t *testing.T, c *client, key protocol.UUID) security.PreconditionReply {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, e := pipeAvailable(ctx, func() (security.PreconditionReply, error) {
		return security.ReceivePrecondition(ctx, c.process.fixture.config.PipeName, uint32(c.process.cmd.Process.Pid), security.PreconditionRequest{Version: 1, Kind: "installation-operation", Bearer: c.token, OperationID: string(key)})
	})
	if e != nil {
		t.Fatal("original phase precondition unavailable (credential withheld)")
	}
	return out
}
func joinOriginalSessionDelivery(t *testing.T, c *client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// ServeOperator is serial. Reaching a following mode-5 request proves the
	// prior session issuer finished. Mode 5 itself never mints/pends a delivery.
	absentKey := newID(t)
	_, e := pipeAvailable(ctx, func() (security.PreconditionReply, error) {
		return security.ReceivePrecondition(ctx, c.process.fixture.config.PipeName, uint32(c.process.cmd.Process.Pid), security.PreconditionRequest{Version: 1, Kind: "installation-operation", Bearer: c.token, OperationID: string(absentKey)})
	})
	if e == nil {
		t.Fatal("absent original operation falsely acquired phase")
	}
}

func TestV008NativeFinalizeFlushUnknown(t *testing.T) {
	count := 0
	for ordinal := 0; ordinal == 0 || ordinal <= count; ordinal++ {
		t.Run(fmt.Sprintf("native-flush-%d", ordinal), func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			key := newID(t)
			stage := finalizeFixture(t, f, key, ordinal)
			p := stage.start(t)
			c := p.connect(t)
			joinOriginalSessionDelivery(t, c)
			root := filepath.Join(f.config.AllowedRoots[0], "finalize-target")
			input := protocol.CreateWorkspace{OperationId: key, Path: protocol.Path(root), DisplayName: "native finalization original"}
			if ordinal == 0 {
				var created protocol.WorkspaceView
				c.request(t, "POST", "/v1/workspace-creations", input, &created, 201)
				report := readFinalizeEvidence(t, f)
				if report.PID != p.cmd.Process.Pid || report.OperationID != string(key) || report.Ordinal != 0 || report.Calls < 1 || report.Calls > 8 || report.Injected != 0 || report.Unknown || report.SQLiteCode != 0 || !report.NativeControlFailed || !report.CallbackDrained {
					t.Fatal("real finalization flush baseline invalid", report)
				}
				count = report.Calls
				p.stop(t)
				return
			}
			var refused protocol.ErrorView
			c.request(t, "POST", "/v1/workspace-creations", input, &refused, 503)
			report := readFinalizeEvidence(t, f)
			if report.PID != p.cmd.Process.Pid || report.OperationID != string(key) || report.Ordinal != ordinal || report.Calls < ordinal || report.Injected != 1 || report.NativeBOOL != 0 || report.SQLiteCode != 1034 || !report.Unknown || !report.NativeControlFailed || !report.CallbackDrained {
				t.Fatal("not actual native SQLITE_IOERR_FSYNC/unknown", report)
			}
			if refused.Code != protocol.ErrorCodeINSTALLATIONSTATEUNKNOWN || refused.Outcome != "unknown" || refused.OperationId == nil || *refused.OperationId != key {
				t.Fatal("actual shared authorization uncertainty misreported")
			}
			var fenced protocol.ErrorView
			c.request(t, "GET", "/v1/context", nil, &fenced, 503)
			if fenced.Code != protocol.ErrorCodeINSTALLATIONSTATEUNKNOWN {
				t.Fatal("same installation continued trusting unknown grants")
			}
			c.request(t, "POST", "/v1/workspace-creations", input, &fenced, 503)
			// Workspace IO has really ended even though authorization persistence has
			// not become trustworthy. Installation lock must still be held.
			lease, e := platform.Acquire(root, platform.Workspace, false, false)
			if e != nil {
				t.Fatal("source physical cleanup not completed")
			}
			if e = lease.Close(); e != nil {
				t.Fatal(e)
			}
			if other, e := platform.Acquire(f.config.InstallationRoot, platform.Installation, false, false); e == nil {
				_ = other.Close()
				t.Fatal("unknown installation owner released lock")
			}
			before := tree(t, root)
			p.terminateAt(t, "unresolved-installation-cleanup", "") // actual pinned child exit, NOT graceful/safeToMove
			next := f.start(t)
			d := next.connect(t)
			path := "/v1/installation-operations/" + string(key)
			var original protocol.OperationView
			d.request(t, "GET", path, nil, &original, 200)
			if original.OperationId != key || original.WorkspaceId == nil || (original.LogicalState != "accepted" && original.LogicalState != "committed" && original.LogicalState != "unknown") {
				t.Fatal("cold original outcome lost or invented rejection")
			}
			pre := operationPrecondition(t, d, key)
			var reconciled protocol.OperationView
			d.request(t, "POST", path+"/reconcile", protocol.ReconcileInstallation{ExpectedPhase: protocol.InstallationPhase(pre.ExpectedPhase)}, &reconciled, 200)
			if reconciled.OperationId != key || reconciled.WorkspaceId == nil || *reconciled.WorkspaceId != *original.WorkspaceId || reconciled.LogicalState != "committed" || reconciled.PersistenceState != "known" {
				t.Fatal("fresh original finalization not proven")
			}
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("reconciliation replayed source genesis/descriptor")
			}
			opened := d.open(t, *original.WorkspaceId)
			if opened.OpenEpoch == nil || opened.Revision == nil || *opened.Revision != "0" {
				t.Fatal("cold original registration unusable or replayed business")
			}
			caps := map[protocol.Capability]bool{}
			for _, v := range opened.AvailableCapabilities {
				caps[v] = true
			}
			for _, v := range []protocol.Capability{protocol.CapabilityWorkspaceRead, protocol.CapabilityWorkspaceWrite, protocol.CapabilityWorkspaceClose, protocol.CapabilityWorkspaceOperator} {
				if !caps[v] {
					t.Fatal("partial initial grant surfaced as finalization", v)
				}
			}
			var rows protocol.RecordList
			d.request(t, "GET", "/v1/workspaces/"+string(*original.WorkspaceId)+"/records", nil, &rows, 200)
			if len(rows.Items) != 0 {
				t.Fatal("investigation executed business")
			}
			next.stop(t)
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("final source conservation failed")
			}
			matrixEvidence(t, f, "native-finalize-result", struct {
				Native         nativeFinalizeEvidence
				Cold, Resolved protocol.OperationView
				Before, After  map[string]string
			}{report, original, reconciled, before, tree(t, root)})
			t.Logf("native ordinal %d: actual FSYNC1034 -> installation fence; original cold outcome %s -> explicit known registration; source bytes conserved", ordinal, original.LogicalState)
		})
	}
}
