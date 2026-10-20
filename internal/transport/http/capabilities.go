//go:build windows && amd64

package httptransport

import (
	"context"
	"errors"
	"net/http"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// This is a projection of existing current grants and retained lifecycle facts,
// not a permission source. Each advertised grant is rechecked before delivery.
func (h *Handler) currentWorkspaceView(ctx context.Context, r *http.Request, scope workspace.Scope, s workspace.Status) (protocol.WorkspaceView, func() error, error) {
	v := workspaceView(scope, s)
	var checks []*installation.Authorization
	add := func(action string, caps ...protocol.Capability) error {
		a, err := h.auth.AuthorizeHTTP(ctx, r, h.authority, scope.WorkspaceID, action)
		if errors.Is(err, installation.ErrUnauthorized) {
			return nil
		}
		if err != nil {
			return err
		}
		checks = append(checks, a)
		v.AvailableCapabilities = append(v.AvailableCapabilities, caps...)
		return nil
	}
	if err := add("workspace.read", protocol.CapabilityWorkspaceRead); err != nil {
		return protocol.WorkspaceView{}, nil, err
	}
	if err := add("workspace.operator", protocol.CapabilityWorkspaceOperator); err != nil {
		return protocol.WorkspaceView{}, nil, err
	}
	if s.State == workspace.LifecycleClosed && !h.isDraining() {
		if err := add("workspace.open", protocol.CapabilityWorkspaceOpen); err != nil {
			return protocol.WorkspaceView{}, nil, err
		}
	}
	if s.Epoch != "" {
		if err := add("workspace.close", protocol.CapabilityWorkspaceClose); err != nil {
			return protocol.WorkspaceView{}, nil, err
		}
	}
	if s.State == workspace.LifecycleOpen && s.Snapshot != nil && !s.NeedsInvestigation {
		if err := add("workspace.read", protocol.CapabilityTextRead); err != nil {
			return protocol.WorkspaceView{}, nil, err
		}
		if !h.isDraining() {
			if err := add("workspace.write", protocol.CapabilityWorkspaceWrite, protocol.CapabilityTextPreview, protocol.CapabilityTextCommit); err != nil {
				return protocol.WorkspaceView{}, nil, err
			}
		}
	}
	return v, func() error {
		for _, a := range checks {
			if err := a.Check(ctx); err != nil {
				return err
			}
		}
		return nil
	}, nil
}
