//go:build windows && amd64 && !aboard_acceptance

package installation

import (
	"context"
	"testing"
)

func TestProductionInspectionObservationIgnoresEnvironment(t *testing.T) {
	t.Setenv("ABOARD_ACCEPTANCE_INSPECTION_PROCESS", "1")
	for _, value := range []string{"", "definitely-not-a-probe", "eyJ2ZXJzaW9uIjoxfQ"} {
		t.Setenv("ABOARD_ACCEPTANCE_INSPECTION_IO", value)
		if beginInspectionObservation(context.Background(), nil, nil, "not-a-source") != nil {
			t.Fatal("production observer must be identity/no environment or IO")
		}
	}
}
