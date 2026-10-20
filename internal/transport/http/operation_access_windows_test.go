//go:build windows && amd64

package httptransport

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
)

func scopedHTTPPrincipal(t *testing.T, f httpFixture, principal, ws, action string) httpFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	owner, err := f.auth.Authorize(ctx, f.token, "", "installation.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.auth.SetPrincipalAuthorization(ctx, owner, principal, false, []sqlite.WorkspaceGrant{{WorkspaceID: ws, Action: action}}); err != nil {
		t.Fatal(err)
	}
	name := "aboard-g5-http-role-" + principal
	pipe, err := security.NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- f.auth.ServeOperator(ctx, pipe, security.Grant{PrincipalID: principal, WorkspaceID: ws, Action: action}, time.Minute)
	}()
	token, clientErr := security.ReceiveSession(ctx, name, uint32(os.Getpid()))
	serverErr := <-done
	closeErr := pipe.Close()
	if clientErr != nil || serverErr != nil || closeErr != nil {
		t.Fatal("native role session", clientErr, serverErr, closeErr)
	}
	f.token = token
	return f
}
func TestHTTPOriginalOperationAccessAcceptsExplicitOperatorNotReader(t *testing.T) {
	f := newHTTPFixture(t)
	v := f.create(t, "operation-access", "76000000-0000-4000-8000-000000000020")
	v = f.open(t, v.WorkspaceId)
	prefix := "/v1/workspaces/" + string(v.WorkspaceId)
	status, raw := f.request(t, "POST", prefix+"/record-previews", encode(t, protocol.PreviewRecord{OpenEpoch: *v.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "formal", Body: "content"}))
	requireStatus(t, status, 201)
	var p protocol.CandidateView
	decodeResponse(t, raw, &p)
	const key = "76000000-0000-4000-8000-000000000021"
	status, _ = f.request(t, "POST", prefix+"/record-commits", encode(t, protocol.CommitRecord{OperationId: key, OpenEpoch: *v.OpenEpoch, CandidateId: p.CandidateId, CandidateDigest: p.CandidateDigest, PreviewVersion: p.PreviewVersion}))
	requireStatus(t, status, 200)
	operator := scopedHTTPPrincipal(t, f, "76000000-0000-4000-8000-000000000022", string(v.WorkspaceId), "workspace.operator")
	if _, err := f.auth.Authorize(context.Background(), operator.token, string(v.WorkspaceId), "workspace.read"); !errors.Is(err, installation.ErrUnauthorized) {
		t.Fatal("fixture operator unexpectedly has read", err)
	}
	status, raw = operator.request(t, "GET", prefix+"/operations/"+key, nil)
	requireStatus(t, status, 200)
	var evidence protocol.OperationView
	decodeResponse(t, raw, &evidence)
	if evidence.LogicalState != "committed" || evidence.PersistenceState != "known" {
		t.Fatal("operator failed to query original actor")
	}
	reader := scopedHTTPPrincipal(t, f, "76000000-0000-4000-8000-000000000023", string(v.WorkspaceId), "workspace.read")
	status, _ = reader.request(t, "GET", prefix+"/operations/"+key, nil)
	requireStatus(t, status, 403)
}
