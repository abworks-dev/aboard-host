package workspace

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

var ErrRecordQuery = errors.New("workspace record query is invalid")

type RecordPage struct {
	Scope      Scope
	Items      []TextRecord
	NextCursor string
}

// Cursors are scoped positions, not credentials or a snapshot promise. Every
// request must independently authorize the retained owner. No path/SQL/secret
// is encoded; a cursor cannot select or open another workspace.
func recordCursor(scope Scope, after string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join([]string{"1", scope.InstallationID, scope.TenantID, scope.WorkspaceID, after}, "|")))
}
func cursorPosition(scope Scope, cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > 256 {
		return "", ErrRecordQuery
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil {
		return "", ErrRecordQuery
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 5 || parts[0] != "1" || parts[1] != scope.InstallationID || parts[2] != scope.TenantID || parts[3] != scope.WorkspaceID || !recordID(parts[4]) {
		return "", ErrRecordQuery
	}
	return parts[4], nil
}
func recordID(id string) bool {
	_, err := protocol.UUID(id).MarshalJSON()
	return err == nil
}

// withRead holds only the workspace-local owner, serializing the trusted view
// with physical drain. The callback must not recurse into this owner. Current
// authorization is checked on both sides of IO, without retaining an installation
// mutex over that IO. Closed/unknown has no ordinary read view and never opens.
func (o *OpenOwner) withRead(ctx context.Context, scope Scope, authorize func(context.Context) error, read func(TextFacts) error) error {
	ticket, admissionErr := o.beginOperation(ctx, scope, authorize, false, false)
	if admissionErr != nil {
		return admissionErr
	}
	defer ticket.finish()
	o.mu.Lock()
	defer o.mu.Unlock()
	if scope != o.scope {
		return ErrScope
	}
	if authorize == nil || read == nil {
		return ErrRecordQuery
	}
	if err := authorize(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state, err := o.lifecycle.State(scope)
	if err != nil {
		return err
	}
	if state != LifecycleOpen || o.resources == nil || o.view.Epoch == "" || o.view.NeedsInvestigation {
		return ErrLifecycle
	}
	if err = o.resources.Validate(); err != nil {
		return err
	}
	if err = read(o.resources); err != nil {
		return err
	}
	if err = o.resources.Validate(); err != nil {
		return err
	}
	if err = authorize(ctx); err != nil {
		return err
	}
	return ctx.Err()
}

func (o *OpenOwner) ReadSnapshot(ctx context.Context, scope Scope, authorize func(context.Context) error) (Snapshot, error) {
	var out Snapshot
	err := o.withRead(ctx, scope, authorize, func(f TextFacts) error { var err error; out, err = f.Snapshot(ctx, scope); return err })
	if err != nil {
		return Snapshot{}, err
	}
	return out, nil
}
func (o *OpenOwner) ReadRecord(ctx context.Context, scope Scope, authorize func(context.Context) error, id string) (TextRecord, error) {
	var out TextRecord
	err := o.withRead(ctx, scope, authorize, func(f TextFacts) error {
		if !recordID(id) {
			return ErrRecordQuery
		}
		var err error
		out, err = f.Current(ctx, scope, id)
		return err
	})
	if err != nil {
		return TextRecord{}, err
	}
	return out, nil
}
func (o *OpenOwner) ListRecords(ctx context.Context, scope Scope, authorize func(context.Context) error, cursor string, limit int) (RecordPage, error) {
	out := RecordPage{Scope: scope, Items: make([]TextRecord, 0)}
	err := o.withRead(ctx, scope, authorize, func(f TextFacts) error {
		if limit < 1 || limit > 100 {
			return ErrRecordQuery
		}
		after, err := cursorPosition(scope, cursor)
		if err != nil {
			return err
		}
		items, err := f.List(ctx, scope, after, limit)
		if err != nil {
			return err
		}
		out.Items = append(out.Items, items...)
		// A full page may lead to an empty terminal page. This is keyset pagination
		// of CURRENT formal facts, never a cached view or cross-request transaction.
		if len(items) == limit {
			out.NextCursor = recordCursor(scope, items[len(items)-1].ResourceID)
		}
		return nil
	})
	if err != nil {
		return RecordPage{}, err
	}
	return out, nil
}
