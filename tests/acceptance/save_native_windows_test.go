//go:build windows && amd64

package acceptance

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

type saveNativeConfig struct {
	Version                                               int `json:"version"`
	Fixture, WorkspaceID, PrincipalID, OperationID, Phase string
	Ordinal                                               int
}
type saveNativeReport struct {
	Version                                                                       int `json:"version"`
	PID                                                                           int `json:"pid"`
	InstallationID, TenantID, WorkspaceID, PrincipalID, OperationID, Epoch, Phase string
	Ordinal, Calls, Injected, NativeBOOL, SQLiteCode                              int
	Unknown, NativeControlFailed, CallbackDrained                                 bool
	Status                                                                        string
}

func saveNativeFixture(t *testing.T, f *fixture, key, workspace, principal protocol.UUID, phase string, ordinal int) *fixture {
	t.Helper()
	image := os.Getenv(stageExecutableEnvironment)
	if image == "" {
		t.Skip("scoped acceptance executable not configured")
	}
	hash, e := checkedExecutable(image)
	if e != nil {
		t.Fatal(e)
	}
	clone := *f
	clone.executable = image
	clone.sha = hash
	raw, e := json.Marshal(saveNativeConfig{1, f.root, string(workspace), string(principal), string(key), phase, ordinal})
	if e != nil {
		t.Fatal(e)
	}
	clone.environment = []string{"ABOARD_ACCEPTANCE_SAVE=" + base64.RawURLEncoding.EncodeToString(raw)}
	return &clone
}
func loadSaveNativeReport(root, phase string) (saveNativeReport, error) {
	var out saveNativeReport
	if phase != "admission" && phase != "formal" {
		return out, fmt.Errorf("unsupported native phase")
	}
	path := filepath.Join(root, "save-"+phase+"-evidence.json")
	stat, e := os.Lstat(path)
	if e != nil || !stat.Mode().IsRegular() || stat.Size() > 4096 {
		return out, fmt.Errorf("native report unavailable")
	}
	f, e := os.Open(path)
	if e != nil {
		return out, e
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, 4097))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || len(raw) > 4096 || json.Unmarshal(raw, &out) != nil || out.Version != 1 {
		return out, fmt.Errorf("native report invalid")
	}
	return out, nil
}
func nativeSaveFaultAllowed(in faultInput) bool {
	var phase string
	switch in.Phase {
	case "save-native-admission":
		phase = "admission"
	case "save-native-formal":
		phase = "formal"
	default:
		return false
	}
	r, e := loadSaveNativeReport(in.Fixture, phase)
	return e == nil && r.PID == in.PID && r.Phase == phase && r.OperationID == in.OperationID && in.OperationID != "" && r.Unknown && r.SQLiteCode == 1034 && r.Injected == 1 && r.NativeBOOL == 0 && r.CallbackDrained && r.NativeControlFailed && r.Ordinal > 0 && r.Calls >= r.Ordinal
}
func TestV010IndependentNativeSaveFlushUnknown(t *testing.T) {
	for _, phase := range []string{"admission", "formal"} {
		t.Run(phase, func(t *testing.T) {
			count := 0
			for ordinal := 0; ordinal == 0 || ordinal <= count; ordinal++ {
				t.Run(fmt.Sprintf("flush-%d", ordinal), func(t *testing.T) {
					f := newFixture(t)
					f.initialize(t)
					setup := f.start(t)
					initial := setup.connect(t)
					var actor protocol.PrincipalView
					initial.request(t, "GET", "/v1/context", nil, &actor, 200)
					root, a := newInspectionWorkspace(t, initial, "save-target")
					_, b := newInspectionWorkspace(t, initial, "independent-B")
					setup.stop(t)
					key := newID(t)
					staged := saveNativeFixture(t, f, key, a.WorkspaceId, actor.PrincipalId, phase, ordinal)
					p := staged.start(t)
					c := p.connect(t)
					joinOriginalSessionDelivery(t, c)
					a = c.open(t, a.WorkspaceId)
					b = c.open(t, b.WorkspaceId)
					prefix := "/v1/workspaces/" + string(a.WorkspaceId)
					candidate := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "native original", Body: "single original save; no retry under a new key"})
					input := saveInput(key, a, candidate)
					if ordinal == 0 {
						var receipt protocol.Receipt
						c.request(t, "POST", prefix+"/record-commits", input, &receipt, 200)
						report, e := loadSaveNativeReport(f.root, phase)
						if e != nil {
							t.Fatal(e)
						}
						if report.PID != p.cmd.Process.Pid || report.WorkspaceID != string(a.WorkspaceId) || report.PrincipalID != string(actor.PrincipalId) || report.OperationID != string(key) || report.Epoch != string(*a.OpenEpoch) || report.Phase != phase || report.Ordinal != 0 || report.Calls < 1 || report.Calls > 8 || report.Injected != 0 || report.Unknown || report.SQLiteCode != 0 || !report.NativeControlFailed || !report.CallbackDrained {
							t.Fatal("invalid actual per-phase native baseline")
						}
						count = report.Calls
						if receipt.OperationId != key {
							t.Fatal("baseline original key changed")
						}
						saveMatches(t, saveFacts(t, c, a.WorkspaceId, "1", 1)[0], receipt, candidate)
						p.stop(t)
						return
					}
					var refused protocol.ErrorView
					c.request(t, "POST", prefix+"/record-commits", input, &refused, 503)
					report, e := loadSaveNativeReport(f.root, phase)
					if e != nil {
						t.Fatal(e)
					}
					if report.PID != p.cmd.Process.Pid || report.InstallationID != string(actor.InstallationId) || report.TenantID != string(actor.TenantId) || report.PrincipalID != string(actor.PrincipalId) || report.WorkspaceID != string(a.WorkspaceId) || report.OperationID != string(key) || report.Epoch != string(*a.OpenEpoch) || report.Phase != phase || report.Ordinal != ordinal || report.Calls < ordinal || report.Injected != 1 || report.NativeBOOL != 0 || report.SQLiteCode != 1034 || report.Status != "unknown" || !report.Unknown || !report.NativeControlFailed || !report.CallbackDrained {
						t.Fatal("save did not surface actual scoped native FSYNC1034 unknown")
					}
					if refused.Code != protocol.ErrorCodeCOMMITUNKNOWN || refused.OperationId == nil || *refused.OperationId != key || (refused.Scope == nil || *refused.Scope != "workspace") || refused.Outcome != "unknown" {
						t.Fatal("original workspace uncertainty mislabeled")
					}
					c.request(t, "POST", prefix+"/record-commits", input, &refused, 503)
					if again, e := loadSaveNativeReport(f.root, phase); e != nil || again != report {
						t.Fatal("same-key inquiry rearmed native business IO")
					}
					var denied protocol.ErrorView
					c.request(t, "GET", prefix+"/records", nil, &denied, 409)
					lease, e := platform.Acquire(root, platform.Workspace, false, false)
					if e == nil {
						_ = lease.Close()
						t.Fatal("unknown original released actual writer exclusion")
					}
					var live protocol.PrincipalView
					c.request(t, "GET", "/v1/context", nil, &live, 200)
					if live.InstallationId != actor.InstallationId || live.PrincipalId != actor.PrincipalId {
						t.Fatal("local workspace fault contaminated shared authorization")
					}
					independent := savePreview(t, c, b, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B after A unknown", Body: "B actually confirms independently"})
					var bReceipt protocol.Receipt
					c.request(t, "POST", "/v1/workspaces/"+string(b.WorkspaceId)+"/record-commits", saveInput(newID(t), b, independent), &bReceipt, 200)
					saveMatches(t, saveFacts(t, c, b.WorkspaceId, "1", 1)[0], bReceipt, independent)
					p.terminateAt(t, "save-native-"+phase, string(key)) // pinned actual process end, NOT graceful drain
					lease, e = platform.Acquire(root, platform.Workspace, false, false)
					if e != nil {
						t.Fatal("actual killed owner still excludes writer")
					}
					if lease.Close() != nil {
						t.Fatal("post-exit ownership observation failed")
					}
					next := f.start(t)
					d := next.connect(t)
					pre := registrationPrecondition(t, d, a.WorkspaceId, "open")
					var raw json.RawMessage
					openedStatus := d.request(t, "POST", prefix+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, &raw, 0)
					if openedStatus != 200 && openedStatus != 503 {
						var failure protocol.ErrorView
						if json.Unmarshal(raw, &failure) != nil {
							t.Fatalf("cold open invalid response: %d", openedStatus)
						}
						t.Fatalf("cold trustworthy open refused unexpectedly: status=%d code=%s", openedStatus, failure.Code)
					}
					var opened protocol.WorkspaceView
					if openedStatus == 200 {
						if json.Unmarshal(raw, &opened) != nil || opened.OpenEpoch == nil {
							t.Fatal("cold normal open lacked valid evidence")
						}
					} else {
						var failure protocol.ErrorView
						if json.Unmarshal(raw, &failure) != nil || failure.Code != protocol.ErrorCodeCOMMITUNKNOWN {
							t.Fatal("cold accepted history published ordinary epoch")
						}
					}
					queryStatus := d.request(t, "GET", prefix+"/operations/"+string(key), nil, &raw, 0)
					var original, resolved protocol.OperationView
					if queryStatus == 409 {
						if phase != "admission" {
							t.Fatal("formal failure lost previously confirmed durable admission")
						}
						var unavailable protocol.ErrorView
						if json.Unmarshal(raw, &unavailable) != nil || unavailable.Code != protocol.ErrorCodeOPERATIONHISTORYUNAVAILABLE || openedStatus != 200 {
							t.Fatal("missing admission falsely classified")
						}
						// Missing history does NOT establish rollback/non-commit or permit replay.
						saveFacts(t, d, a.WorkspaceId, "0", 0)
					} else {
						if queryStatus != 200 || json.Unmarshal(raw, &original) != nil || original.Scope != "workspace" || original.OperationId != key || original.WorkspaceId == nil || *original.WorkspaceId != a.WorkspaceId {
							t.Fatal("cold original key/scope lost")
						}
						if phase == "admission" && (original.LogicalState == "committed" || original.Receipt != nil) {
							t.Fatal("uncertain admission illegally executed formal business")
						}
						d.request(t, "POST", prefix+"/operations/"+string(key)+"/reconcile", nil, &resolved, 200)
						if resolved.Scope != "workspace" || resolved.OperationId != key || resolved.WorkspaceId == nil || *resolved.WorkspaceId != a.WorkspaceId {
							t.Fatal("reconcile crossed original operation scope")
						}
						if original.LogicalState == "committed" {
							if original.Receipt == nil || resolved.Receipt == nil || *original.Receipt != *resolved.Receipt || resolved.LogicalState != "committed" {
								t.Fatal("reconcile rewrote confirmed cold receipt")
							}
						} else if original.Receipt != nil || resolved.LogicalState != "rejected" || resolved.Receipt != nil {
							t.Fatal("reconcile replayed business absent from trustworthy cold history")
						}
						if resolved.PersistenceState != "known" || (resolved.LogicalState != "committed" && resolved.LogicalState != "rejected") {
							t.Fatal("positive original history unresolved after fresh exclusive proof")
						}
						if openedStatus != 200 {
							opened = d.open(t, a.WorkspaceId)
						}
						if resolved.LogicalState == "committed" {
							if resolved.Receipt == nil || resolved.Receipt.OperationId != key {
								t.Fatal("confirmed original receipt unavailable")
							}
							saveMatches(t, saveFacts(t, d, a.WorkspaceId, "1", 1)[0], *resolved.Receipt, candidate)
						} else {
							if resolved.Receipt != nil {
								t.Fatal("non-committed original invented receipt")
							}
							saveFacts(t, d, a.WorkspaceId, "0", 0)
							d.request(t, "POST", prefix+"/record-commits", input, &denied, 409)
							saveFacts(t, d, a.WorkspaceId, "0", 0)
						}
					}
					d.open(t, b.WorkspaceId)
					saveMatches(t, saveFacts(t, d, b.WorkspaceId, "1", 1)[0], bReceipt, independent)
					next.stop(t)
					var originalEvidence, resolvedEvidence *protocol.OperationView
					if queryStatus == 200 {
						originalEvidence = &original
						resolvedEvidence = &resolved
					}
					matrixEvidence(t, f, "save-native-"+phase, struct {
						Native              saveNativeReport
						ColdOpen, ColdQuery int
						Original, Resolved  *protocol.OperationView
						BReceipt            protocol.Receipt
					}{report, openedStatus, queryStatus, originalEvidence, resolvedEvidence, bReceipt})
				})
			}
		})
	}
}
