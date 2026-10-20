//go:build windows && amd64 && !aboard_acceptance

package installation

import (
	"context"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// No environment access, new persistence, capability or retry in production.
func workspaceSaveAdmit(_ *openResources, _ workspace.SaveRequest, apply func() (workspace.SaveResult, *sqlite.Admission, error)) (workspace.SaveResult, *sqlite.Admission, error) {
	return apply()
}
func workspaceSaveCommit(_ context.Context, _ *openResources, _ workspace.SaveRequest, _ *sqlite.Admission, apply func() (workspace.SaveResult, error)) (workspace.SaveResult, error) {
	return apply()
}
