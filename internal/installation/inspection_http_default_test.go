//go:build windows && amd64 && !aboard_acceptance

package installation

import (
	"net/http"
	"testing"
)

func TestProductionHTTPObservationDoesNotChangeTransport(t *testing.T) {
	t.Setenv("ABOARD_ACCEPTANCE_INSPECTION_HTTP", "untrusted-placeholder")
	original := http.DefaultTransport
	InstallAcceptanceHTTPObservation()
	InstallAcceptanceHTTPObservation()
	if http.DefaultTransport != original {
		t.Fatal("production must not install acceptance transport or read config")
	}
}
