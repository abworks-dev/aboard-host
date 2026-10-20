//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"time"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func (o *WorkspaceOpener) CommitRecord(ctx context.Context, a *Authorization, in workspace.SaveInput) (workspace.SaveResult, error) {
	owner, scope, check, err := o.readOwner(ctx, a, "workspace.write")
	if err != nil {
		return workspace.SaveResult{Status: workspace.SaveRejected}, err
	}
	return owner.CommitRecord(ctx, scope, a.Principal().PrincipalID, check, in)
}

// This capability never leaves the retained workspace owner. It cannot be minted
// by existing-key investigation, copied into an HTTP DTO, or reused after restart.
type acceptedWorkspaceSave struct {
	resources *openResources
	admission *sqlite.Admission
	request   workspace.SaveRequest // original trusted metadata, never a second admission capability
}

func (a *acceptedWorkspaceSave) Commit(ctx context.Context) (workspace.SaveResult, error) {
	r := a.resources
	if r.pending || r.quarantined || r.gate == nil {
		return workspace.SaveResult{Status: workspace.SaveUnknown}, workspace.ErrOpenUncertain
	}
	result, err := workspaceSaveCommit(ctx, r, a.request, a.admission, func() (workspace.SaveResult, error) {
		return r.gate.CommitAccepted(ctx, a.admission)
	})
	if result.Status == workspace.SaveUnknown || errors.Is(err, sqlite.ErrUnknown) || errors.Is(err, sqlite.ErrFenced) {
		r.pending = true
	}
	return result, err
}
func (r *openResources) AdmitSave(ctx context.Context, scope workspace.Scope, request workspace.SaveRequest, investigationOnly bool) (workspace.SaveResult, workspace.AcceptedSave, error) {
	if scope != r.scope || request.InstallationID != scope.InstallationID || request.TenantID != scope.TenantID || request.WorkspaceID != scope.WorkspaceID {
		return workspace.SaveResult{Status: workspace.SaveRejected}, nil, workspace.ErrScope
	}
	if r.quarantined || r.gate == nil {
		return workspace.SaveResult{Status: workspace.SaveUnknown}, nil, workspace.ErrOpenUncertain
	}
	if r.pending || investigationOnly {
		result, err := r.gate.Investigate(ctx, request)
		return result, nil, err
	}
	result, admission, err := workspaceSaveAdmit(r, request, func() (workspace.SaveResult, *sqlite.Admission, error) {
		return r.gate.Admit(ctx, request, time.Now())
	})
	if result.Status == workspace.SaveUnknown || errors.Is(err, sqlite.ErrUnknown) || errors.Is(err, sqlite.ErrFenced) {
		r.pending = true
	}
	if admission == nil {
		return result, nil, err
	}
	return result, &acceptedWorkspaceSave{resources: r, admission: admission, request: request}, err
}
func (r *openResources) RetainSaveUncertainty() { r.pending = true }

var _ workspace.SaveFacts = (*openResources)(nil)
