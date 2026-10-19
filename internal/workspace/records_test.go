package workspace

import (
	"context"
	"errors"
	"testing"
)

type recordTestResources struct {
	openTestResources
	records []TextRecord
	reads   int
}

func (r *recordTestResources) List(_ context.Context, _ Scope, after string, limit int) ([]TextRecord, error) {
	r.reads++
	out := make([]TextRecord, 0)
	for _, item := range r.records {
		if item.ResourceID > after && len(out) < limit {
			out = append(out, item)
		}
	}
	return out, nil
}
func (r *recordTestResources) Current(_ context.Context, _ Scope, id string) (TextRecord, error) {
	r.reads++
	for _, item := range r.records {
		if item.ResourceID == id {
			return item, nil
		}
	}
	return TextRecord{}, ErrNotFound
}
func TestRecordReadScopeCursorAndCurrentAuthorization(t *testing.T) {
	owner, _, scope := openTestOwner(t)
	ctx := context.Background()
	auth := func(context.Context) error { return nil }
	r := &recordTestResources{records: []TextRecord{{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, ResourceID: "20000000-0000-4000-8000-000000000001", Body: "one"}, {WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, ResourceID: "20000000-0000-4000-8000-000000000002", Body: "two"}}}
	if _, err := owner.ListRecords(ctx, scope, auth, "", 1); !errors.Is(err, ErrLifecycle) {
		t.Fatal("closed read", err)
	}
	if _, err := owner.Open(ctx, scope, auth, func(context.Context, string) (OpenEvidence, error) {
		return OpenEvidence{Resources: r, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	first, err := owner.ListRecords(ctx, scope, auth, "", 1)
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatal(first, err)
	}
	second, err := owner.ListRecords(ctx, scope, auth, first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || second.Items[0].Body != "two" {
		t.Fatal(second, err)
	}
	end, err := owner.ListRecords(ctx, scope, auth, second.NextCursor, 1)
	if err != nil || end.Items == nil || len(end.Items) != 0 || end.NextCursor != "" {
		t.Fatal(end, err)
	}
	foreign := scope
	foreign.WorkspaceID = "30000000-0000-4000-8000-000000000003"
	foreignToken := recordCursor(foreign, r.records[0].ResourceID)
	before := r.reads
	foreignTenant := scope
	foreignTenant.TenantID = foreign.WorkspaceID
	foreignInstall := scope
	foreignInstall.InstallationID = foreign.WorkspaceID
	for _, token := range []string{foreignToken, recordCursor(foreignTenant, r.records[0].ResourceID), recordCursor(foreignInstall, r.records[0].ResourceID), "SQL; path", first.NextCursor + "=", recordCursor(scope, "invalid"), string(make([]byte, 257))} {
		if _, err = owner.ListRecords(ctx, scope, auth, token, 1); !errors.Is(err, ErrRecordQuery) {
			t.Fatal("invalid cursor", err)
		}
	}
	if _, err = owner.ListRecords(ctx, foreign, auth, "", 1); !errors.Is(err, ErrScope) {
		t.Fatal("foreign scope", err)
	}
	for _, n := range []int{0, -1, 101} {
		if _, err = owner.ListRecords(ctx, scope, auth, "", n); !errors.Is(err, ErrRecordQuery) {
			t.Fatal("limit", err)
		}
	}
	if _, err = owner.ReadRecord(ctx, scope, auth, "not a uuid"); !errors.Is(err, ErrRecordQuery) {
		t.Fatal("id", err)
	}
	if r.reads != before {
		t.Fatal("invalid query reached facts")
	}
	denied := errors.New("synthetic revoked")
	calls := 0
	late := func(context.Context) error {
		calls++
		if calls > 1 {
			return denied
		}
		return nil
	}
	got, err := owner.ReadRecord(ctx, scope, late, r.records[0].ResourceID)
	if !errors.Is(err, denied) || got.Body != "" {
		t.Fatal("late denial leaked record", got, err)
	}
	if err = owner.DrainOpen(ctx, scope, auth); err != nil {
		t.Fatal(err)
	}
	before = r.reads
	if _, err = owner.ReadRecord(ctx, scope, auth, r.records[0].ResourceID); !errors.Is(err, ErrLifecycle) || before != r.reads {
		t.Fatal("read after close", err)
	}
}
