//go:build windows && amd64

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

func TestCurrentInstallationOperationCommonDispatch(t *testing.T) {
	for _, kind := range []string{"create", "register", "relink"} {
		t.Run(kind, func(t *testing.T) {
			d, hash := authStorageFixture(t)
			ctx := context.Background()
			want := creationStorageIntent(t).InstallationOperation
			want.Kind = kind
			input, err := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(want.InstallationID), PrincipalID: protocol.UUID(want.PrincipalID), OperationID: protocol.UUID(want.OperationID), Kind: kind, WorkspaceID: protocol.UUID(want.WorkspaceID), CanonicalLocator: want.TargetLocator})
			if err != nil {
				t.Fatal(err)
			}
			want.InputDigest = string(input)
			// Synthetic dispatch fixture only: deliberately NO creation_intent row.
			// This test does not claim register/relink business execution exists.
			if _, err = d.h.Conn.ExecContext(ctx, `INSERT INTO installation_operation VALUES(?,?,?,?,?,?,?,?,?)`, want.InstallationID, want.OperationID, want.PrincipalID, want.WorkspaceID, want.Kind, want.TargetLocator, want.InputDigest, want.Phase, want.Outcome); err != nil {
				t.Fatal(err)
			}
			got, err := d.ReadCurrentInstallationOperation(ctx, hash, 1, testInstallation, testPrincipal, testOperation)
			if err != nil || got != want {
				t.Fatalf("common dispatch: %+v %v", got, err)
			}
			if _, err = d.ReadCurrentInstallationOperation(ctx, hash, 1, testInstallation, testPrincipal, testResource); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("missing fact: %v", err)
			}
			for _, input := range []struct {
				hash, installation, principal string
				version                       int64
			}{
				{hash, testInstallation, testPrincipal, 2},
				{hash, testInstallation, testResource, 1},
				{hash, testResource, testPrincipal, 1},
				{"not-a-current-session", testInstallation, testPrincipal, 1},
			} {
				got, err := d.ReadCurrentInstallationOperation(ctx, input.hash, input.version, input.installation, input.principal, testOperation)
				if err == nil || got != (InstallationOperation{}) {
					t.Fatalf("unauthorized dispatch leaked fact: %+v %v", got, err)
				}
			}
			if scalar(t, d, `SELECT count(*) FROM directory_registration WHERE workspace_id=?`, want.WorkspaceID) != 0 {
				t.Fatal("query registered target")
			}
			if _, err = d.h.Conn.ExecContext(ctx, `UPDATE installation_operation SET input_digest=? WHERE operation_id=?`, strings.Repeat("0", 64), testOperation); err != nil {
				t.Fatal(err)
			}
			got, err = d.ReadCurrentInstallationOperation(ctx, hash, 1, testInstallation, testPrincipal, testOperation)
			if !errors.Is(err, ErrInstallationOperation) || got != (InstallationOperation{}) {
				t.Fatalf("corrupt digest accepted: %+v %v", got, err)
			}
		})
	}
}
