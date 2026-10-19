package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type previewTestResources struct {
	recordTestResources
	put    []Candidate
	putErr error
	head   int64
}

func (r *previewTestResources) Snapshot(context.Context, Scope) (Snapshot, error) {
	return Snapshot{Revision: r.head}, nil
}
func (r *previewTestResources) PutPreview(_ context.Context, _ Scope, c Candidate) error {
	r.put = append(r.put, c)
	return r.putErr
}

func TestPreviewValidationAllocationAndFences(t *testing.T) {
	ctx := context.Background()
	owner, l, scope := openTestOwner(t)
	check := func(context.Context) error { return nil }
	r := &previewTestResources{}
	view, err := owner.Open(ctx, scope, check, func(context.Context, string) (OpenEvidence, error) {
		return OpenEvidence{Resources: r, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	in := PreviewInput{Epoch: view.Epoch, Kind: "create-text", Title: "同名", Body: "text\r\né😀"}
	first, err := owner.PreviewRecord(ctx, scope, scope.InstallationID, check, in)
	if err != nil || first.Before != nil || !recordID(first.Candidate.ResourceID) || !recordID(first.Candidate.CandidateID) || first.Candidate.PreviewVersion != 1 || first.Candidate.PreviewEpoch != view.Epoch || first.Candidate.ExpiresAt.Before(time.Now()) || first.Candidate.ExpiresAt.After(time.Now().Add(CandidateLifetime)) {
		t.Fatal(first, err)
	}
	second, err := owner.PreviewRecord(ctx, scope, scope.InstallationID, check, in)
	if err != nil || second.Candidate.CandidateID == first.Candidate.CandidateID || second.Candidate.ResourceID == first.Candidate.ResourceID || second.Candidate.CandidateDigest == first.Candidate.CandidateDigest {
		t.Fatal(second, err)
	}
	before := len(r.put)
	for name, change := range map[string]func(*PreviewInput){
		"utf8":                  func(v *PreviewInput) { v.Body = string([]byte{255}) },
		"nul":                   func(v *PreviewInput) { v.Body = "x\x00y" },
		"body byte limit":       func(v *PreviewInput) { v.Body = strings.Repeat("é", 32769) },
		"title empty":           func(v *PreviewInput) { v.Title = "" },
		"title scalars":         func(v *PreviewInput) { v.Title = strings.Repeat("😀", 257) },
		"supplied resource":     func(v *PreviewInput) { v.ResourceID = first.Candidate.ResourceID },
		"supplied base digest":  func(v *PreviewInput) { s := first.Candidate.ContentDigest; v.BaseDigest = &s },
		"update missing target": func(v *PreviewInput) { v.Kind = "update-text" },
		"kind":                  func(v *PreviewInput) { v.Kind = "apply-file" },
		"negative revision":     func(v *PreviewInput) { v.BaseRevision = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := in
			change(&bad)
			if _, err := owner.PreviewRecord(ctx, scope, scope.InstallationID, check, bad); !errors.Is(err, ErrPreview) {
				t.Fatal(err)
			}
		})
	}
	stale := in
	stale.Epoch = scope.InstallationID
	if _, err = owner.PreviewRecord(ctx, scope, scope.InstallationID, check, stale); !errors.Is(err, ErrStaleHandle) {
		t.Fatal(err)
	}
	stale = in
	stale.BaseRevision = 1
	if _, err = owner.PreviewRecord(ctx, scope, scope.InstallationID, check, stale); !errors.Is(err, ErrBaseline) {
		t.Fatal(err)
	}
	denied := errors.New("synthetic revoked")
	calls := 0
	late := func(context.Context) error {
		calls++
		if calls >= 2 {
			return denied
		}
		return nil
	}
	if got, err := owner.PreviewRecord(ctx, scope, scope.InstallationID, late, in); !errors.Is(err, denied) || got.Candidate.CandidateID != "" {
		t.Fatal(got, err)
	}
	if len(r.put) != before {
		t.Fatal("invalid/late-revoked preview persisted", len(r.put), before)
	}
	r.putErr = ErrOpenUncertain
	r.closeErr = ErrOpenUncertain
	if got, err := owner.PreviewRecord(ctx, scope, scope.InstallationID, check, in); !errors.Is(err, ErrOpenUncertain) || got.Candidate.CandidateID != "" {
		t.Fatal(got, err)
	}
	if state, _ := l.State(scope); state != LifecycleUnknown {
		t.Fatal(state)
	}
	before = len(r.put)
	if _, err = owner.PreviewRecord(ctx, scope, scope.InstallationID, check, in); !errors.Is(err, ErrLifecycle) || len(r.put) != before {
		t.Fatal("unknown admitted", err)
	}
	if err = owner.DrainOpen(ctx, scope, check); !errors.Is(err, ErrOpenUncertain) {
		t.Fatal("unknown drained", err)
	}
	t.Log("injected owner-only validation and late-auth/unknown boundaries; not native persistence or physical-drain acceptance")
}
