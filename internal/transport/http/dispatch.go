//go:build windows && amd64

package httptransport

import (
	"encoding/json"
	"errors"
	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	"net/http"
	"net/url"
	"strconv"
)

func number(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n } // generated strict validation already proved the 63-bit range
func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request, a *installation.Authorization, route protocol.Route, query url.Values, raw []byte, corr protocol.CorrelationId) {
	ctx := r.Context()
	ws, key := route.Parameters["workspaceId"], route.Parameters["operationId"]
	installationScope := ws == "" || route.ID == "relinkWorkspace"
	reply := func(status int, value any) {
		h.write(w, status, value, corr, route.ID, key, installationScope, func() error {
			action := route.Action
			if action == "authenticated" {
				action = ""
			}
			_, err := h.auth.AuthorizeHTTP(ctx, r, h.authority, ws, action)
			return err
		})
	}
	fail := func(err error) { h.failure(w, err, corr, route.ID, key, installationScope) }
	installationReply := func(v installation.InstallationOperationView, callErr error, successStatus int) {
		if callErr != nil && (v.OperationID == "" || errors.Is(callErr, installation.ErrUnauthorized) || errors.Is(callErr, installation.ErrAuthorizationUnknown)) {
			fail(callErr)
			return
		}
		if v.Outcome == sqlite.InstallationRejected {
			h.fail(w, 409, protocol.ErrorCodeRECOVERYREQUIRED, corr, route.ID, key, true)
			return
		}
		if callErr == nil && v.Outcome == sqlite.InstallationCommitted && v.Phase == sqlite.RegistrationConfirmed {
			// A duplicate is inquiry-only. Project the CURRENT authorized state,
			// never assume a formerly registered directory is still closed.
			readA, authErr := h.auth.AuthorizeHTTP(ctx, r, h.authority, v.WorkspaceID, "workspace.read")
			if authErr == nil {
				current, stateErr := h.opener.WorkspaceStatus(ctx, readA)
				if stateErr == nil {
					currentScope := workspace.Scope{InstallationID: v.InstallationID, TenantID: a.Principal().TenantID, WorkspaceID: v.WorkspaceID}
					h.write(w, successStatus, workspaceView(currentScope, current), corr, route.ID, key, installationScope, func() error {
						if _, e := h.auth.AuthorizeHTTP(ctx, r, h.authority, ws, route.Action); e != nil {
							return e
						}
						_, e := h.auth.AuthorizeHTTP(ctx, r, h.authority, v.WorkspaceID, "workspace.read")
						return e
					})
					return
				}
			}
		}
		w.Header().Set("Location", "/v1/installation-operations/"+key)
		reply(202, installationOperation(v, corr))
	}
	decode := func(v any) bool {
		if err := json.Unmarshal(raw, v); err != nil {
			h.fail(w, 400, protocol.ErrorCodeINVALIDINPUT, corr, route.ID, "", ws == "")
			return false
		}
		return true
	}
	scope := workspace.Scope{InstallationID: a.Principal().InstallationID, TenantID: a.Principal().TenantID, WorkspaceID: ws}
	switch route.ID {
	case "getContext":
		p := a.Principal()
		v := protocol.PrincipalView{InstallationId: protocol.UUID(p.InstallationID), TenantId: protocol.UUID(p.TenantID), PrincipalId: protocol.UUID(p.PrincipalID), Capabilities: []protocol.Capability{}}
		if _, err := h.auth.AuthorizeHTTP(ctx, r, h.authority, "", "installation.operator"); err == nil {
			v.Capabilities = append(v.Capabilities, protocol.CapabilityInstallationOperator)
		}
		reply(200, v)
	case "createWorkspace":
		var in protocol.CreateWorkspace
		if !decode(&in) {
			return
		}
		key = string(in.OperationId)
		result, err := h.creator.CreateWorkspace(ctx, a, installation.CreateWorkspaceRequest{OperationID: key, RootLocator: string(in.Path), DisplayName: string(in.DisplayName)})
		installationReply(result, err, 201)
	case "inspectWorkspace":
		var in protocol.InspectWorkspace
		if !decode(&in) {
			return
		}
		result, err := h.inspector.Inspect(ctx, a, string(in.Path))
		if err != nil {
			fail(err)
			return
		}
		reply(200, protocol.InspectionView{WorkspaceId: protocol.UUID(result.WorkspaceID), TenantId: protocol.UUID(result.TenantID), FormatVersion: int64(result.FormatVersion), Integrity: "valid", AccessMode: "none"})
	case "registerWorkspace":
		var in protocol.RegisterWorkspace
		if !decode(&in) {
			return
		}
		key = string(in.OperationId)
		result, err := h.directories.Register(ctx, a, installation.RegisterDirectoryRequest{OperationID: key, RootLocator: string(in.Path), ExpectedWorkspaceID: string(in.ExpectedWorkspaceId), ExpectedIdentityDigest: string(in.ExpectedIdentityDigest)})
		installationReply(result, err, 200)
	case "relinkWorkspace":
		var in protocol.RelinkWorkspace
		if !decode(&in) {
			return
		}
		key = string(in.OperationId)
		result, err := h.directories.Relink(ctx, a, ws, installation.RelinkDirectoryRequest{OperationID: key, RootLocator: string(in.Path), ExpectedRegistrationVersion: number(string(in.ExpectedRegistrationVersion))})
		installationReply(result, err, 200)
	case "openWorkspace":
		var in protocol.OpenWorkspace
		if !decode(&in) {
			return
		}
		v, err := h.opener.OpenWorkspace(ctx, a, number(string(in.ExpectedRegistrationVersion)))
		if err != nil {
			fail(err)
			return
		}
		reply(200, workspaceView(scope, workspace.Status{Scope: scope, State: workspace.LifecycleOpen, Epoch: v.Epoch, Snapshot: &v.Snapshot, NeedsInvestigation: v.NeedsInvestigation}))
	case "getWorkspace":
		v, err := h.opener.WorkspaceStatus(ctx, a)
		if err != nil {
			fail(err)
			return
		}
		reply(200, workspaceView(scope, v))
	case "closeWorkspace":
		var in protocol.CloseWorkspace
		if !decode(&in) {
			return
		}
		key = string(in.OperationId)
		v, err := h.opener.CloseWorkspace(ctx, a, key, string(in.OpenEpoch))
		if err != nil {
			fail(err)
			return
		}
		status := 202
		if v.Closed {
			status = 200
		}
		w.Header().Set("Location", "/v1/workspaces/"+ws+"/operations/"+key)
		reply(status, closeView(v, corr))
	case "listRecords":
		limit := 20
		if v := query.Get("limit"); v != "" {
			limit = int(number(v))
		}
		page, err := h.opener.ListRecords(ctx, a, query.Get("cursor"), limit)
		if err != nil {
			fail(err)
			return
		}
		v := protocol.RecordList{WorkspaceId: protocol.UUID(ws), Items: make([]protocol.TextRecordView, 0, len(page.Items))}
		for _, row := range page.Items {
			v.Items = append(v.Items, recordView(row))
		}
		if page.NextCursor != "" {
			v.NextCursor = &page.NextCursor
		}
		reply(200, v)
	case "getRecord":
		v, err := h.opener.ReadRecord(ctx, a, route.Parameters["resourceId"])
		if err != nil {
			fail(err)
			return
		}
		reply(200, recordView(v))
	case "previewRecord":
		var in protocol.PreviewRecord
		if !decode(&in) {
			return
		}
		request := workspace.PreviewInput{Epoch: string(in.OpenEpoch), Kind: string(in.Kind), BaseRevision: number(string(in.BaseRevision)), Title: string(in.Title), Body: string(in.Body)}
		if in.ResourceId != nil {
			request.ResourceID = string(*in.ResourceId)
		}
		if in.BaseDigest != nil {
			d := string(*in.BaseDigest)
			request.BaseDigest = &d
		}
		v, err := h.opener.PreviewRecord(ctx, a, request)
		if err != nil {
			fail(err)
			return
		}
		reply(200, candidateView(v))
	case "commitRecord":
		var in protocol.CommitRecord
		if !decode(&in) {
			return
		}
		key = string(in.OperationId)
		v, err := h.opener.CommitRecord(ctx, a, workspace.SaveInput{OperationID: key, Epoch: string(in.OpenEpoch), CandidateID: string(in.CandidateId), CandidateDigest: string(in.CandidateDigest), PreviewVersion: number(string(in.PreviewVersion))})
		if v.Status == workspace.SaveConfirmed && v.Receipt != nil {
			if v.ViewDeliveryFailed {
				reply(202, saveView(ws, key, workspace.OperationEvidence{Result: v}, corr))
			} else {
				reply(200, receiptView(v.Receipt))
			}
			return
		}
		if err != nil {
			fail(err)
			return
		}
		if v.Status == workspace.SaveUnknown || v.Status == workspace.SaveAccepted {
			w.Header().Set("Location", "/v1/workspaces/"+ws+"/operations/"+key)
			reply(202, saveView(ws, key, workspace.OperationEvidence{Result: v}, corr))
			return
		}
		h.fail(w, 409, protocol.ErrorCodeSTALEBASELINE, corr, route.ID, key, false)
	case "getWorkspaceOperation", "reconcileWorkspaceOperation":
		// Process-local close keys and durable save keys share the same workspace
		// namespace, but a lost close history must never be fabricated as a receipt.
		closeA := a
		if route.ID == "reconcileWorkspaceOperation" {
			closeA = a
		} // explicit operator already checked
		cv, closeErr := h.opener.GetCloseOperation(ctx, closeA, key)
		if closeErr == nil {
			reply(200, closeView(cv, corr))
			return
		}
		if !errors.Is(closeErr, workspace.ErrOperationHistoryUnavailable) {
			fail(closeErr)
			return
		}
		var v workspace.OperationEvidence
		var err error
		if route.ID == "reconcileWorkspaceOperation" {
			v, err = h.opener.ReconcileWorkspaceOperation(ctx, a, key)
		} else {
			v, err = h.opener.GetWorkspaceOperation(ctx, a, key)
		}
		if err != nil {
			fail(err)
			return
		}
		reply(200, saveView(ws, key, v, corr))
	case "getInstallationOperation":
		v, err := h.directories.GetInstallationOperation(ctx, a, key)
		if err != nil {
			fail(err)
			return
		}
		reply(200, installationOperation(v, corr))
	case "reconcileInstallationOperation":
		var in protocol.ReconcileInstallation
		if !decode(&in) {
			return
		}
		v, err := h.directories.ReconcileInstallationOperation(ctx, a, key, sqlite.InstallationPhase(in.ExpectedPhase))
		if err != nil {
			fail(err)
			return
		}
		reply(200, installationOperation(v, corr))
	default:
		h.fail(w, 501, protocol.ErrorCodeCAPABILITYUNAVAILABLE, corr, route.ID, "", ws == "")
	}
}
