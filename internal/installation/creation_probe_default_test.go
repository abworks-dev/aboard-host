//go:build windows && amd64 && !aboard_acceptance

package installation

import (
	"errors"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"testing"
)

func TestProductionCreationProbeIsIdentityAndIgnoresEnvironment(t *testing.T) {
	t.Setenv("ABOARD_ACCEPTANCE_CREATION", "malformed arbitrary fault input")
	t.Setenv("ABOARD_ACCEPTANCE_FINALIZE", "malformed arbitrary fault input")
	sentinel := errors.New("original adapter result")
	calls := 0
	if err := creationFinalize(nil, nil, sqlite.DirectoryOperation{}, false, func() error { calls++; return sentinel }); err != sentinel || calls != 1 {
		t.Fatal("production changed original finalize invocation/result")
	}
	creator := &WorkspaceCreator{}
	got, err := creationRequestCreator(creator, CreateWorkspaceRequest{})
	if got != creator || err != nil {
		t.Fatal("production acquired a fault configuration branch")
	}
}
