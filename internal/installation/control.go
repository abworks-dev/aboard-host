//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// ServeOperator keeps session issuance on the original authenticated pipe and
// adds only two bounded read-only precondition queries, never generic commands.
func (a *Authenticator) ServeOperator(ctx context.Context, pipe *security.ControlPipe, grant security.Grant, ttl time.Duration) error {
	if pipe == nil {
		return ErrUnauthorized
	}
	issuer := &databaseIssuer{auth: a, ctx: ctx}
	defer issuer.finish()
	err := pipe.ServeControl(ctx, issuer, grant, ttl, a)
	return errors.Join(err, issuer.revokeError)
}

// ReadControlPrecondition accepts a real bearer even though the OS peer is also
// checked. SID/co-location does not grant another principal's workspace scope.
func (a *Authenticator) ReadControlPrecondition(ctx context.Context, r security.PreconditionRequest) (security.PreconditionReply, func(context.Context) error, error) {
	empty := security.PreconditionReply{}
	if r.Version != 1 {
		return empty, nil, ErrUnauthorized
	}
	var authorization *Authorization
	reply := security.PreconditionReply{Version: 1, Code: "ok", Kind: r.Kind}
	switch r.Kind {
	case "registration":
		if r.OperationID != "" || (r.IntendedUse != "open" && r.IntendedUse != "relink") {
			return empty, nil, ErrUnauthorized
		}
		action := "workspace.open"
		if r.IntendedUse == "relink" {
			action = "workspace.operator"
		}
		var err error
		authorization, err = a.Authorize(ctx, r.Bearer, r.WorkspaceID, action)
		if err != nil {
			return empty, nil, err
		}
		reply.WorkspaceID = protocol.UUID(r.WorkspaceID)
		reply.RegistrationVersion = protocol.PositiveRevision(strconv.FormatInt(authorization.Location().RegistrationVersion, 10))
	case "installation-operation":
		if r.WorkspaceID != "" || r.IntendedUse != "" {
			return empty, nil, ErrUnauthorized
		}
		var err error
		authorization, err = a.Authorize(ctx, r.Bearer, "", "installation.operator")
		if err != nil {
			return empty, nil, err
		}
		coordinator, err := NewDirectoryCoordinator(a)
		if err != nil {
			return empty, nil, err
		}
		operation, err := coordinator.GetInstallationOperation(ctx, authorization, r.OperationID)
		if errors.Is(err, ErrWorkspaceCreationNotFound) {
			return empty, nil, ErrUnauthorized
		}
		if err != nil {
			return empty, nil, err
		}
		reply.WorkspaceID = protocol.UUID(operation.WorkspaceID)
		reply.OperationID = protocol.UUID(operation.OperationID)
		reply.ExpectedPhase = protocol.InstallationPhase(operation.Phase)
	default:
		return empty, nil, ErrUnauthorized
	}
	principal := authorization.Principal()
	reply.InstallationID = protocol.UUID(principal.InstallationID)
	reply.TenantID = protocol.UUID(principal.TenantID)
	if err := authorization.Check(ctx); err != nil {
		return empty, nil, err
	}
	return reply, authorization.Check, nil
}
