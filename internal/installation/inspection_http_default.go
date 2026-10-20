//go:build windows && amd64 && !aboard_acceptance

package installation

// InstallAcceptanceHTTPObservation is deliberately inert in production builds.
// It grants no capability and never reads the acceptance environment.
func InstallAcceptanceHTTPObservation() {}
