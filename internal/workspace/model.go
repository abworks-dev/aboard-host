// Package workspace owns workspace-scoped use-case models and ports. It does
// not choose a SQLite driver, retain a global current workspace, or authorize a
// client merely because it can name a resource. Transport DTOs are separate.
package workspace

import (
	"context"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

var (
	ErrScope    = errors.New("workspace scope does not match the retained owner")
	ErrNotFound = errors.New("workspace resource not found")
)

// Scope comes from the installation owner, never from self-reported permissions.
// A physical directory locator and a currently authorized principal are distinct.
type Scope struct {
	InstallationID string
	TenantID       string
	WorkspaceID    string
}

func (s Scope) Validate() error {
	for _, id := range []string{s.InstallationID, s.TenantID, s.WorkspaceID} {
		if _, err := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID(id), TenantID: protocol.UUID(s.TenantID), DatabaseRelativePath: "data/workspace.sqlite"}); err != nil {
			return ErrScope
		}
	}
	return nil
}

// Identity and Head are database facts, not directory-open/access-mode/Run state.
// Callers cannot infer a current grant from BusinessState or IdentityDigest.
type Identity struct {
	WorkspaceID    string
	TenantID       string
	DisplayName    string
	IdentityDigest string
	FormatVersion  int
	SchemaVersion  int
	BusinessState  string
}
type Snapshot struct {
	Identity Identity
	Revision int64
}
type TextRecord struct {
	WorkspaceID   string
	TenantID      string
	ResourceID    string
	Version       int64
	Revision      int64
	Title         string
	Body          string
	ContentDigest string
	CreatedBy     string
	OperationID   string
}

// TextFacts is the minimal formal-text read port. Every call includes the stable
// owner scope; it is not an HTTP authorization interface. The use-case owner
// must separately check CURRENT principal/action and serialize its lifecycle.
// Implementations share the formal commit owner's connection/serialization and
// return values (no live SQL handles, arbitrary statements, paths or secrets).
// afterResourceID is a scoped internal key, not a transferable public cursor.
type TextFacts interface {
	Snapshot(context.Context, Scope) (Snapshot, error)
	Current(context.Context, Scope, string) (TextRecord, error)
	List(context.Context, Scope, string, int) ([]TextRecord, error)
}
