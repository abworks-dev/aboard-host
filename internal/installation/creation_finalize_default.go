//go:build windows && amd64 && !aboard_acceptance

package installation

import "github.com/abworks-dev/aboard-host/internal/storage/sqlite"

// Production neither parses fault configuration nor changes the VFS table.
func creationFinalize(_ *WorkspaceCreator, _ *Authorization, _ sqlite.DirectoryOperation, _ bool, apply func() error) error {
	return apply()
}
