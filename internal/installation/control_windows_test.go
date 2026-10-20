//go:build windows && amd64

package installation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
)

func controlQuery(t *testing.T, f authFixture, request security.PreconditionRequest) (security.PreconditionReply, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	name := fmt.Sprintf("aboard-g5-t022-control-%d", time.Now().UnixNano())
	pipe, err := security.NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- f.auth.ServeOperator(ctx, pipe, security.Grant{PrincipalID: f.store.Identity().OwnerPrincipalID, Action: "installation.operator"}, time.Minute)
	}()
	reply, queryErr := security.ReceivePrecondition(ctx, name, security.ProcessID(), request)
	if err = <-done; err != nil {
		t.Error("protected query server failed", err)
	}
	if err = pipe.Close(); err != nil {
		t.Error(err)
	}
	return reply, queryErr
}
func TestControlPreconditionsRealNativeAuthorization(t *testing.T) {
	f, creator, operator, token, request := creationFixture(t)
	ctx := context.Background()
	created, err := creator.CreateWorkspace(ctx, operator, request)
	if err != nil {
		t.Fatal(err)
	}
	// Metadata reads do not open or inspect the registered directory; obtaining its
	// current registration CAS value must not contend with its actual writer.
	lease, err := platform.Acquire(request.RootLocator, platform.Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if lease != nil {
			lease.Close()
		}
	}()
	query := security.PreconditionRequest{Version: 1, Kind: "registration", Bearer: token, WorkspaceID: created.WorkspaceID, IntendedUse: "open"}
	reply, err := controlQuery(t, f, query)
	if err != nil || reply.Code != "ok" || string(reply.WorkspaceID) != created.WorkspaceID || reply.RegistrationVersion != "1" || reply.ExpectedPhase != "" {
		t.Fatal("current registration precondition missing", err)
	}
	payload, err := json.Marshal(reply)
	if err != nil || strings.Contains(string(payload), token) || strings.Contains(string(payload), request.RootLocator) || strings.Contains(string(payload), f.allowed) {
		t.Fatal("protected precondition reply leaked credential or locator")
	}
	query.IntendedUse = "relink"
	if _, err = controlQuery(t, f, query); err != nil {
		t.Fatal(err)
	}
	phase := security.PreconditionRequest{Version: 1, Kind: "installation-operation", Bearer: token, OperationID: request.OperationID}
	reply, err = controlQuery(t, f, phase)
	if err != nil || string(reply.ExpectedPhase) != string(created.Phase) || string(reply.WorkspaceID) != created.WorkspaceID || string(reply.OperationID) != request.OperationID || reply.RegistrationVersion != "" {
		t.Fatal("original phase precondition missing", err)
	}
	if err = f.auth.SetPrincipalAuthorization(ctx, operator, authRestricted, false, []sqlite.WorkspaceGrant{{WorkspaceID: created.WorkspaceID, Action: "workspace.open"}}); err != nil {
		t.Fatal(err)
	}
	restricted := authIssue(t, f, authRestricted, created.WorkspaceID, "workspace.open", time.Minute)
	query.Bearer = restricted
	query.IntendedUse = "open"
	if _, err = controlQuery(t, f, query); err != nil {
		t.Fatal("authorized nonoperator opener cannot obtain its own version", err)
	}
	query.IntendedUse = "relink"
	if _, err = controlQuery(t, f, query); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("opener gained operator precondition", err)
	}
	query.IntendedUse = "open"
	query.WorkspaceID = authB
	if _, err = controlQuery(t, f, query); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("same SID exposed another workspace", err)
	}
	phase.Bearer = restricted
	if _, err = controlQuery(t, f, phase); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("workspace grant gained installation operation history", err)
	}
	if err = f.auth.RevokeSession(ctx, operator, restricted); err != nil {
		t.Fatal(err)
	}
	query.WorkspaceID = created.WorkspaceID
	if _, err = controlQuery(t, f, query); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked session received a precondition", err)
	}
	if err = operator.Check(ctx); err != nil {
		t.Fatal("read-only metadata changed authorization", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	lease = nil
	if err = f.auth.SetPrincipalAuthorization(ctx, operator, authRestricted, false, []sqlite.WorkspaceGrant{{WorkspaceID: created.WorkspaceID, Action: "workspace.operator"}}); err != nil {
		t.Fatal(err)
	}
	foreignToken := authIssue(t, f, authRestricted, created.WorkspaceID, "workspace.operator", time.Minute)
	foreign, err := f.auth.Authorize(ctx, foreignToken, created.WorkspaceID, "workspace.operator")
	if err != nil {
		t.Fatal(err)
	}
	moved := request.RootLocator + "-moved"
	if err = os.Rename(request.RootLocator, moved); err != nil {
		t.Fatal(err)
	}
	directories, err := NewDirectoryCoordinator(f.auth)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = directories.Relink(ctx, foreign, created.WorkspaceID, RelinkDirectoryRequest{OperationID: authB, RootLocator: moved, ExpectedRegistrationVersion: 1}); err != nil {
		t.Fatal(err)
	}
	phase.Bearer = token
	phase.OperationID = authB
	_, forbidden := controlQuery(t, f, phase)
	phase.OperationID = "31000000-0000-4000-8000-000000000099"
	_, missing := controlQuery(t, f, phase)
	if !errors.Is(forbidden, ErrUnauthorized) || !errors.Is(missing, ErrUnauthorized) {
		t.Fatal("protected history distinguished missing from another actor", forbidden, missing)
	}
}
func TestControlOperatorPreservesOriginalSessionDelivery(t *testing.T) {
	f := newAuthFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	name := fmt.Sprintf("aboard-g5-t022-session-%d", time.Now().UnixNano())
	pipe, err := security.NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- f.auth.ServeOperator(ctx, pipe, security.Grant{PrincipalID: f.store.Identity().OwnerPrincipalID, Action: "installation.operator"}, time.Minute)
	}()
	token, clientErr := security.ReceiveSession(ctx, name, security.ProcessID())
	serverErr := <-done
	if err = pipe.Close(); err != nil {
		t.Error(err)
	}
	if clientErr != nil || serverErr != nil {
		t.Fatal("existing handshake changed", clientErr, serverErr)
	}
	if _, err = f.auth.Authorize(ctx, token, "", "installation.operator"); err != nil {
		t.Fatal(err)
	}
	f.store.mu.Lock()
	pending := f.store.pendingDeliveries
	f.store.mu.Unlock()
	if pending != 0 {
		t.Fatal("session delivery did not physically finish", pending)
	}
}
