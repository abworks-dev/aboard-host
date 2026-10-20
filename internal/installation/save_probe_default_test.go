//go:build windows && amd64 && !aboard_acceptance

package installation

import (
	"context"
	"errors"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	"testing"
)

func TestProductionSaveProbeDelegatesOnceAndIgnoresEnvironment(t *testing.T) {
	t.Setenv("ABOARD_ACCEPTANCE_SAVE", "malformed must never be parsed")
	sentinel := errors.New("unchanged result")
	want := workspace.SaveResult{Status: workspace.SaveAccepted}
	cap := new(sqlite.Admission)
	calls := 0
	got, a, e := workspaceSaveAdmit(nil, workspace.SaveRequest{}, func() (workspace.SaveResult, *sqlite.Admission, error) { calls++; return want, cap, sentinel })
	if calls != 1 || a != cap || got.Status != want.Status || e != sentinel {
		t.Fatal("production admission wrapper changed original call")
	}
	want.Status = workspace.SaveConfirmed
	calls = 0
	got, e = workspaceSaveCommit(context.Background(), nil, workspace.SaveRequest{}, cap, func() (workspace.SaveResult, error) { calls++; return want, sentinel })
	if calls != 1 || got.Status != want.Status || e != sentinel {
		t.Fatal("production commit wrapper changed original call")
	}
}
