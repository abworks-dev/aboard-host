//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"context"
	"errors"
	"os"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func runSaveFullProbe(ctx context.Context, r *openResources, request workspace.SaveRequest, admission *sqlite.Admission, cfg saveProbe, apply func() (workspace.SaveResult, error)) (workspace.SaveResult, error) {
	unknown := workspace.SaveResult{Status: workspace.SaveUnknown}
	if e := saveProbeQuiet(r, request); e != nil {
		return unknown, e
	}
	if admission == nil || !r.gate.AcceptanceSaveEpoch(request.Epoch) {
		return unknown, errSaveProbe
	}
	identity := cfg.Fixture + "|" + cfg.WorkspaceID + "|" + cfg.OperationID + "|full"
	if _, used := consumedSaveProbes.LoadOrStore(identity, struct{}{}); used {
		return unknown, errSaveProbe
	}
	result, full, e := r.gate.AcceptanceSaveFull(ctx, admission, apply)
	report := saveProbeReport{Version: 1, PID: os.Getpid(), InstallationID: request.InstallationID, TenantID: request.TenantID, WorkspaceID: request.WorkspaceID, PrincipalID: request.PrincipalID, OperationID: request.OperationID, Epoch: request.Epoch, Phase: "full", Status: result.Status, Unknown: errors.Is(e, sqlite.ErrUnknown), SQLiteCode: full.SQLiteCode, Full: &full}
	e = errors.Join(e, writeSaveProbeReport(cfg.Fixture, report))
	retainSaveProbeFailure(r, e)
	if e != nil && result.Status != workspace.SaveConfirmed && result.Status != workspace.SaveRejected {
		result.Status = workspace.SaveUnknown
	}
	return result, e
}
