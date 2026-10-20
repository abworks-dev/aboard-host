//go:build windows && amd64 && !aboard_acceptance

package installation

// Production cannot read fault environment or install an acceptance callback.
func creationRequestCreator(c *WorkspaceCreator, _ CreateWorkspaceRequest) (*WorkspaceCreator, error) {
	return c, nil
}
