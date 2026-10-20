//go:build windows && amd64 && !aboard_acceptance

package installation

import (
	"context"
	inspection "github.com/abworks-dev/aboard-host/internal/workspace/inspect"
)

// Production neither reads acceptance environment nor installs observers.
func beginInspectionObservation(context.Context, *DirectoryInspector, *Authorization, string) func(inspection.Report, bool, error) {
	return nil
}
