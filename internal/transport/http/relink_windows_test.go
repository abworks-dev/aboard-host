//go:build windows && amd64

package httptransport

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

func TestHTTPClosedRelinkPreservesQuotaEligibilityWithoutReopen(t *testing.T) {
	f := newHTTPFixture(t)
	v := f.create(t, "relink-closed", "76000000-0000-4000-8000-000000000010")
	v = f.open(t, v.WorkspaceId)
	prefix := "/v1/workspaces/" + string(v.WorkspaceId)
	closeKey := "76000000-0000-4000-8000-000000000011"
	status, _ := f.request(t, "POST", prefix+"/close", encode(t, protocol.CloseWorkspace{OperationId: protocol.UUID(closeKey), OpenEpoch: *v.OpenEpoch}))
	if status != 200 && status != 202 {
		t.Fatal("close status", status)
	}
	a, err := f.auth.Authorize(context.Background(), f.token, string(v.WorkspaceId), "workspace.close")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if closed, err := f.handler.opener.WaitClose(ctx, a, closeKey); err != nil || !closed.Closed {
		t.Fatal("close not physically complete", err)
	}
	oldRoot := filepath.Join(f.base, "allowed", "relink-closed")
	newRoot := filepath.Join(f.base, "allowed", "relink-moved")
	// Both resolved paths are owned synthetic children of this fixture's allowed root.
	if err = os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	status, raw := f.request(t, "POST", prefix+"/relink", encode(t, protocol.RelinkWorkspace{OperationId: "76000000-0000-4000-8000-000000000012", Path: protocol.Path(newRoot), ExpectedRegistrationVersion: protocol.PositiveRevision(strconv.FormatInt(a.Location().RegistrationVersion, 10))}))
	if status != 200 {
		t.Fatalf("closed-only relink rejected before business lifecycle: %d %s", status, raw)
	}
	var moved protocol.WorkspaceView
	decodeResponse(t, raw, &moved)
	if moved.DirectoryLifecycle != "registeredClosed" || moved.OpenEpoch != nil || moved.WorkspaceId != v.WorkspaceId {
		t.Fatal("relink implicitly reopened or changed identity")
	}
	reopened := f.open(t, v.WorkspaceId)
	if reopened.OpenEpoch == nil || *reopened.OpenEpoch == *v.OpenEpoch {
		t.Fatal("explicit reopen did not publish new generation")
	}
}
