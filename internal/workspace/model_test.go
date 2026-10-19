package workspace_test

import (
	"strings"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestStableScopeRequiresAllCanonicalIDs(t *testing.T) {
	const id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	good := workspace.Scope{InstallationID: id, TenantID: id, WorkspaceID: id}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", strings.ToUpper(id), "aaaaaaaa-aaaa-1aaa-8aaa-aaaaaaaaaaaa", "aaaaaaaa-aaaa-4aaa-0aaa-aaaaaaaaaaaa", "../other", id + "\x00"} {
		for field := 0; field < 3; field++ {
			scope := good
			switch field {
			case 0:
				scope.InstallationID = bad
			case 1:
				scope.TenantID = bad
			case 2:
				scope.WorkspaceID = bad
			}
			if err := scope.Validate(); err == nil {
				t.Fatalf("scope accepted invalid identity field=%d", field)
			}
		}
	}
}
