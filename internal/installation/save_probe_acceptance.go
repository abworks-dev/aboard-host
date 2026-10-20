//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	win "golang.org/x/sys/windows"
)

const saveProbeEnvironment = "ABOARD_ACCEPTANCE_SAVE"

var errSaveProbe = errors.New("acceptance save fixture rejected")

type saveProbe struct {
	Version                                               int `json:"version"`
	Fixture, WorkspaceID, PrincipalID, OperationID, Phase string
	Ordinal                                               int
}
type saveProbeReport struct {
	Version                                                                       int `json:"version"`
	PID                                                                           int `json:"pid"`
	InstallationID, TenantID, WorkspaceID, PrincipalID, OperationID, Epoch, Phase string
	Ordinal, Calls, Injected, NativeBOOL, SQLiteCode                              int
	Unknown, NativeControlFailed, CallbackDrained                                 bool
	Status                                                                        workspace.PersistenceStatus
	Full                                                                          *sqlite.AcceptanceFullEvidence `json:",omitempty"`
	Busy                                                                          *sqlite.AcceptanceBusyEvidence `json:",omitempty"`
}

var consumedSaveProbes sync.Map // scoped one-shot instrumentation, NEVER a receipt/authority cache

func readSaveProbe(r *openResources, request workspace.SaveRequest) (saveProbe, bool, error) {
	var out saveProbe
	encoded := os.Getenv(saveProbeEnvironment)
	if encoded == "" {
		return out, false, nil
	}
	if len(encoded) > 8192 || os.Getenv(creationProbeEnvironment) != "" || os.Getenv(finalizeProbeEnvironment) != "" {
		return out, false, errSaveProbe
	}
	raw, e := base64.RawURLEncoding.DecodeString(encoded)
	if e != nil {
		return out, false, errSaveProbe
	}
	value, e := protocol.ParseJSON(raw)
	if e != nil {
		return out, false, errSaveProbe
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 7 {
		return out, false, errSaveProbe
	}
	v, vok := fields["version"].(json.Number)
	n, nok := fields["Ordinal"].(json.Number)
	if !vok || string(v) != "1" || !nok {
		return out, false, errSaveProbe
	}
	ordinal, e := strconv.Atoi(string(n))
	if e != nil || ordinal < 0 || ordinal > 8 || strconv.Itoa(ordinal) != string(n) {
		return out, false, errSaveProbe
	}
	for _, key := range []string{"Fixture", "WorkspaceID", "PrincipalID", "OperationID", "Phase"} {
		if _, ok := fields[key].(string); !ok {
			return out, false, errSaveProbe
		}
	}
	if json.Unmarshal(raw, &out) != nil || !creationProbeRoot(out.Fixture) || (out.Phase != "admission" && out.Phase != "formal" && out.Phase != "full" && out.Phase != "busy" && out.Phase != "formal-inflight" && !saveCheckpointPhase(out.Phase)) || ((out.Phase == "full" || out.Phase == "busy" || out.Phase == "formal-inflight" || saveCheckpointPhase(out.Phase)) && out.Ordinal != 0) {
		return out, false, errSaveProbe
	}
	for _, id := range []string{out.WorkspaceID, out.PrincipalID, out.OperationID} {
		b, _ := json.Marshal(id)
		if json.Unmarshal(b, new(protocol.UUID)) != nil {
			return out, false, errSaveProbe
		}
	}
	if r == nil || r.store == nil || r.store.root != filepath.Join(out.Fixture, "installation") {
		return out, false, errSaveProbe
	}
	// Unmatched operations (including B) delegate normally, never arm the probe.
	if request.WorkspaceID != out.WorkspaceID || request.OperationID != out.OperationID {
		return out, false, nil
	}
	if request.PrincipalID != out.PrincipalID || request.InstallationID != r.scope.InstallationID || request.TenantID != r.scope.TenantID || request.WorkspaceID != r.scope.WorkspaceID {
		return out, false, errSaveProbe
	}
	root := filepath.Join(out.Fixture, "allowed", "save-target")
	if r.lease == nil || r.lease.RootPath() != root || r.databasePin == nil || r.databasePin.Path() != filepath.Join(root, "data", "workspace.sqlite") {
		return out, false, errSaveProbe
	}
	return out, true, nil
}
func saveProbeQuiet(r *openResources, request workspace.SaveRequest) error {
	if r.pending || r.quarantined || !r.active || r.gate == nil || r.database == nil || r.nativeFailure != nil || r.Validate() != nil || finalizeDelegateError != nil {
		return errSaveProbe
	}
	s := r.store
	s.mu.Lock()
	defer s.mu.Unlock()
	// Keep the original current-authorized owner; do not wait for workspace IO
	// under Store.mu or require unrelated B to close.
	if s.closed || s.openFailure || s.nativeFailure != nil || s.authUncertain || s.workspaceStopping || s.pendingDeliveries != 0 || s.pendingWorkspaceCreations != 0 || len(s.inspections) != 0 || request.PrincipalID != s.identity.OwnerPrincipalID {
		return errSaveProbe
	}
	return nil
}
func retainSaveProbeFailure(r *openResources, e error) {
	if errors.Is(e, platform.ErrPhysicalCloseUnknown) || errors.Is(e, sqlite.ErrPhysicalCloseUnknown) {
		r.quarantined = true
		r.pending = true
		r.nativeFailure = errors.Join(r.nativeFailure, e)
	}
}
func writeSaveProbeReport(root string, report saveProbeReport) error {
	raw, e := json.Marshal(report)
	if e != nil || len(raw) > 4096 {
		return errSaveProbe
	}
	f, e := os.OpenFile(filepath.Join(root, "save-"+report.Phase+"-evidence.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, writeErr := f.Write(raw)
	if closeErr := f.Close(); closeErr != nil {
		return errors.Join(writeErr, &finalizeCloseError{closeErr, f})
	}
	return writeErr
}

// Reuses the ONE permanently rooted VFS delegate and CAS arm. Native evidence
// comes only from the same actual gate call and exact original workspace files.
func runSaveProbe(r *openResources, request workspace.SaveRequest, phase string, apply func() error, status func() workspace.PersistenceStatus) error {
	cfg, matched, e := readSaveProbe(r, request)
	if e != nil {
		return e
	}
	if !matched || cfg.Phase != phase {
		return apply()
	}
	if e = saveProbeQuiet(r, request); e != nil {
		return e
	}
	if !r.gate.AcceptanceSaveEpoch(request.Epoch) {
		return apply()
	}
	identity := cfg.Fixture + "|" + cfg.WorkspaceID + "|" + cfg.OperationID + "|" + phase
	if _, used := consumedSaveProbes.Load(identity); used {
		return apply()
	}
	path, e := win.UTF16PtrFromString(filepath.Join(cfg.Fixture, "config.json"))
	if e != nil {
		return e
	}
	h, e := win.CreateFile(path, win.GENERIC_READ, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE|win.FILE_SHARE_DELETE, nil, win.OPEN_EXISTING, win.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		return e
	}
	state := &finalizeFlushState{active: true, config: finalizeProbe{Version: 1, Fixture: cfg.Fixture, OperationID: cfg.OperationID, Ordinal: cfg.Ordinal}, handle: h, databasePath: r.databasePin.Path()}
	closeStimulus := func() error {
		if e := win.CloseHandle(h); e != nil {
			return &finalizeCloseError{e, state}
		}
		return nil
	}
	reject := func() error { e := errors.Join(errSaveProbe, closeStimulus()); retainSaveProbeFailure(r, e); return e }
	kind, typeErr := win.GetFileType(h)
	var info win.ByHandleFileInformation
	if typeErr != nil || kind != win.FILE_TYPE_DISK || win.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&(win.FILE_ATTRIBUTE_DIRECTORY|win.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 || finalizeHandlePath(h) != filepath.Join(cfg.Fixture, "config.json") {
		return reject()
	}
	if !errors.Is(win.FlushFileBuffers(h), win.ERROR_ACCESS_DENIED) {
		return reject()
	}
	if !armedFinalize.CompareAndSwap(nil, state) {
		return reject()
	}
	consumedSaveProbes.Store(identity, struct{}{})
	result, closeErr := callFinalizeProbe(state, apply, closeStimulus)
	reportedStatus := status()
	if errors.Is(result, sqlite.ErrUnknown) && reportedStatus != workspace.SaveConfirmed {
		reportedStatus = workspace.SaveUnknown
	}
	state.mu.Lock()
	report := saveProbeReport{Version: 1, PID: os.Getpid(), InstallationID: request.InstallationID, TenantID: request.TenantID, WorkspaceID: request.WorkspaceID, PrincipalID: request.PrincipalID, OperationID: request.OperationID, Epoch: request.Epoch, Phase: phase, Ordinal: cfg.Ordinal, Calls: state.calls, Injected: state.injected, NativeBOOL: state.nativeBOOL, Unknown: errors.Is(result, sqlite.ErrUnknown), NativeControlFailed: true, CallbackDrained: true, Status: reportedStatus}
	state.mu.Unlock()
	var coded interface{ Code() int }
	if errors.As(result, &coded) {
		report.SQLiteCode = coded.Code()
	}
	e = errors.Join(result, closeErr, writeSaveProbeReport(cfg.Fixture, report))
	retainSaveProbeFailure(r, e)
	return e
}
func workspaceSaveAdmit(r *openResources, request workspace.SaveRequest, apply func() (workspace.SaveResult, *sqlite.Admission, error)) (result workspace.SaveResult, admission *sqlite.Admission, err error) {
	result.Status = workspace.SaveRejected
	if err = holdSaveCheckpoint(r, request, "before-admit", nil); err != nil {
		return
	}
	completed := false
	err = runSaveProbe(r, request, "admission", func() error { var e error; result, admission, e = apply(); completed = true; return e }, func() workspace.PersistenceStatus { return result.Status })
	if !completed && errors.Is(err, sqlite.ErrUnknown) {
		result = workspace.SaveResult{Status: workspace.SaveUnknown}
		admission = nil
	}
	if err == nil && admission != nil && result.Status == workspace.SaveAccepted {
		if checkpointErr := holdSaveCheckpoint(r, request, "after-admit", &result); checkpointErr != nil {
			result = workspace.SaveResult{Status: workspace.SaveUnknown}
			admission = nil // durable acceptance is retained; NEVER replay or mint a new capability
			err = errors.Join(checkpointErr, sqlite.ErrUnknown)
		}
	}
	return
}
func workspaceSaveCommit(ctx context.Context, r *openResources, request workspace.SaveRequest, admission *sqlite.Admission, apply func() (workspace.SaveResult, error)) (result workspace.SaveResult, err error) {
	cfg, matched, parseErr := readSaveProbe(r, request)
	if parseErr != nil {
		return workspace.SaveResult{Status: workspace.SaveUnknown}, parseErr
	}
	if matched && cfg.Phase == "formal-inflight" {
		return runSaveMidpointProbe(r, request, admission, cfg, apply)
	}
	if matched && cfg.Phase == "full" {
		return runSaveFullProbe(ctx, r, request, admission, cfg, apply)
	}
	if matched && cfg.Phase == "busy" {
		return runSaveBusyProbe(ctx, r, request, admission, cfg, apply)
	}
	// An accepted capability already exists: any probe-side refusal is UNKNOWN,
	// never a fabricated known rejection and never permission to call apply again.
	result.Status = workspace.SaveUnknown
	err = runSaveProbe(r, request, "formal", func() error { var e error; result, e = apply(); return e }, func() workspace.PersistenceStatus { return result.Status })
	if err == nil && result.Status == workspace.SaveConfirmed && result.Receipt != nil {
		// A diagnostic/delivery failure cannot rewrite the original confirmed truth.
		err = holdSaveCheckpoint(r, request, "after-confirm", &result)
	}
	return
}
