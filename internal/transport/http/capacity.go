//go:build windows && amd64

package httptransport

import (
	"errors"
	"net/http"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func (h *Handler) authorizeRoute(r *http.Request, identity *installation.Authorization, route protocol.Route) (*installation.Authorization, error) {
	action := route.Action
	if action == "authenticated" || action == "" {
		return identity, nil
	}
	// This query's documented alternative is an EXPLICIT current operator grant,
	// not a grant inferred from workspace.read or an installation owner label.
	if route.ID == "getWorkspaceOperation" {
		a, err := h.auth.AuthorizeHTTP(r.Context(), r, h.authority, route.Parameters["workspaceId"], "workspace.operator")
		if err == nil {
			return a, nil
		}
		if !errors.Is(err, installation.ErrUnauthorized) {
			return nil, err
		}
	}
	return h.auth.AuthorizeHTTP(r.Context(), r, h.authority, route.Parameters["workspaceId"], action)
}

// One request claim survives body parsing, sequential business calls, response
// serialization/reauthorization, Write, and the actual Flush return. It never
// creates another semaphore or consumes the runnable-control reserve.
func (h *Handler) reserveRequest(r *http.Request, a *installation.Authorization, route protocol.Route) (*workspace.OperationReservation, error) {
	key := route.Parameters["operationId"]
	var ref *workspace.OperationReservation
	var err error
	switch route.ID {
	case "getInstallationOperation", "reconcileInstallationOperation":
		ref, err = h.store.ClaimInstallationInvestigation(r.Context(), a, key)
	case "getWorkspaceOperation", "reconcileWorkspaceOperation":
		ref, err = h.opener.ClaimWorkspaceInvestigation(r.Context(), a, key)
	default:
		err = workspace.ErrCapacity // no original-resource borrowing for other routes
	}
	if err == nil {
		return ref, nil
	}
	if !errors.Is(err, workspace.ErrCapacity) {
		return nil, err
	}
	if route.Parameters["workspaceId"] == "" {
		ref, err = h.store.ReserveOperation(r.Context(), a)
	} else {
		// control only determines closed/draining eligibility here. ALL prebody
		// reservations are ordinary-backed, including reads and reconciliation.
		ref, err = h.opener.ReserveWorkspaceOperation(r.Context(), a, investigationRoute(route) || route.ID == "relinkWorkspace", route.ID == "openWorkspace")
	}
	if err != nil {
		return nil, err
	}
	claimed, err := ref.ClaimRequest()
	ref.Release()
	return claimed, err
}
