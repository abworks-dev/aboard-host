//go:build windows && amd64

package httptransport

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
)

func requireCapabilities(t *testing.T, v protocol.WorkspaceView, want ...protocol.Capability) {
	t.Helper()
	got := append([]protocol.Capability(nil), v.AvailableCapabilities...)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("capabilities %v, want %v", got, want)
	}
}
func TestHTTPWorkspaceCapabilitiesFollowCurrentGrantAndLifecycle(t *testing.T) {
	f := newHTTPFixture(t)
	ctx := context.Background()
	a := f.create(t, "capabilities", "81000000-0000-4000-8000-000000000001")
	requireCapabilities(t, a, protocol.CapabilityWorkspaceOpen, protocol.CapabilityWorkspaceRead, protocol.CapabilityWorkspaceOperator)
	opened := f.open(t, a.WorkspaceId)
	requireCapabilities(t, opened, protocol.CapabilityWorkspaceClose, protocol.CapabilityWorkspaceRead, protocol.CapabilityWorkspaceOperator, protocol.CapabilityWorkspaceWrite, protocol.CapabilityTextRead, protocol.CapabilityTextPreview, protocol.CapabilityTextCommit)
	actor, err := f.auth.Authorize(ctx, f.token, "", "installation.operator")
	if err != nil {
		t.Fatal(err)
	}
	const principal = "81000000-0000-4000-8000-000000000002"
	grants := []sqlite.WorkspaceGrant{{WorkspaceID: string(a.WorkspaceId), Action: "workspace.read"}}
	if err = f.auth.SetPrincipalAuthorization(ctx, actor, principal, false, grants); err != nil {
		t.Fatal(err)
	}
	pipeName := "aboard-g5-cap-" + string(a.WorkspaceId)
	pipe, err := security.NewControlPipe(pipeName)
	if err != nil {
		t.Fatal(err)
	}
	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- f.auth.ServeOperator(pctx, pipe, security.Grant{PrincipalID: principal, WorkspaceID: string(a.WorkspaceId), Action: "workspace.read"}, time.Minute)
	}()
	token, clientErr := security.ReceiveSession(pctx, pipeName, uint32(os.Getpid()))
	serverErr := <-done
	closeErr := pipe.Close()
	if clientErr != nil || serverErr != nil || closeErr != nil {
		t.Fatal("restricted native session", clientErr, serverErr, closeErr)
	}
	restricted := f
	restricted.token = token
	status, raw := restricted.request(t, "GET", "/v1/workspaces/"+string(a.WorkspaceId), nil)
	requireStatus(t, status, 200)
	var view protocol.WorkspaceView
	decodeResponse(t, raw, &view)
	requireCapabilities(t, view, protocol.CapabilityWorkspaceRead, protocol.CapabilityTextRead)
	read, err := f.auth.Authorize(ctx, token, string(a.WorkspaceId), "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	state, err := f.handler.opener.WorkspaceStatus(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := read.Scope()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", f.server.URL+"/v1/workspaces/"+string(a.WorkspaceId), nil)
	request.Header.Set("Authorization", "Bearer "+token)
	_, check, err := f.handler.currentWorkspaceView(ctx, request, scope, state)
	if err != nil {
		t.Fatal(err)
	}
	closeAuth, err := f.auth.Authorize(ctx, f.token, string(a.WorkspaceId), "workspace.close")
	if err != nil {
		t.Fatal(err)
	}
	const closeKey = "81000000-0000-4000-8000-000000000003"
	if _, err = f.handler.opener.CloseWorkspace(ctx, closeAuth, closeKey, string(*opened.OpenEpoch)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.handler.opener.WaitClose(ctx, closeAuth, closeKey); err != nil {
		t.Fatal(err)
	}
	// A principal with only read may still inspect authorized closed status, but
	// neither records nor reopen/write become available merely because A exists.
	status, raw = restricted.request(t, "GET", "/v1/workspaces/"+string(a.WorkspaceId), nil)
	requireStatus(t, status, 200)
	decodeResponse(t, raw, &view)
	requireCapabilities(t, view, protocol.CapabilityWorkspaceRead)
	if err = f.auth.SetPrincipalAuthorization(ctx, actor, principal, false, nil); err != nil {
		t.Fatal(err)
	}
	if err = check(); !errors.Is(err, installation.ErrUnauthorized) {
		t.Fatal("stale advertised capabilities survived current grant revocation", err)
	}
}
