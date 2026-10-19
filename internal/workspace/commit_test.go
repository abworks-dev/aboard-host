package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type commitCapability func(context.Context) (SaveResult, error)

func (f commitCapability) Commit(ctx context.Context) (SaveResult, error) { return f(ctx) }

type commitTestResources struct {
	openTestResources
	admit    func(context.Context, Scope, SaveRequest, bool) (SaveResult, AcceptedSave, error)
	retained int
}

func (r *commitTestResources) AdmitSave(ctx context.Context, s Scope, q SaveRequest, only bool) (SaveResult, AcceptedSave, error) {
	return r.admit(ctx, s, q, only)
}
func (r *commitTestResources) RetainSaveUncertainty() { r.retained++ }
func commitOwnerFixture(t *testing.T) (*OpenOwner, *Lifecycle, Scope, *commitTestResources, SaveInput) {
	t.Helper()
	o, l, s := openTestOwner(t)
	r := &commitTestResources{}
	v, err := o.Open(context.Background(), s, func(context.Context) error { return nil }, func(context.Context, string) (OpenEvidence, error) {
		return OpenEvidence{Resources: r, Snapshot: Snapshot{Identity: Identity{WorkspaceID: s.WorkspaceID, TenantID: s.TenantID, BusinessState: "ready"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	in := SaveInput{OperationID: s.WorkspaceID, Epoch: v.Epoch, CandidateID: s.TenantID, CandidateDigest: strings.Repeat("a", 64), PreviewVersion: 1}
	return o, l, s, r, in
}
func TestCommitOwnerAcceptedCancellationAndLateDelivery(t *testing.T) {
	for _, mode := range []string{"canceled-waiter", "revoked-after-commit", "physical-after-commit"} {
		t.Run(mode, func(t *testing.T) {
			o, l, s, r, in := commitOwnerFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			denied := errors.New("synthetic revoked authorization")
			revoked := false
			commits := 0
			auth := func(context.Context) error {
				if revoked {
					return denied
				}
				return nil
			}
			r.admit = func(ctx context.Context, scope Scope, q SaveRequest, only bool) (SaveResult, AcceptedSave, error) {
				if scope != s || only || q.PrincipalID != s.InstallationID || q.PreviewVersion != 1 {
					t.Fatal("trusted admission context", scope, q, only)
				}
				if mode == "canceled-waiter" {
					cancel()
				}
				return SaveResult{Status: SaveAccepted}, commitCapability(func(ctx context.Context) (SaveResult, error) {
					commits++
					if ctx.Err() != nil {
						t.Fatal("accepted attempt inherited canceled waiter", ctx.Err())
					}
					if mode == "revoked-after-commit" {
						revoked = true
					}
					if mode == "physical-after-commit" {
						r.validateErr = ErrOpenUncertain
					}
					return SaveResult{Status: SaveConfirmed, Receipt: &Receipt{OperationID: in.OperationID}}, nil
				}), nil
			}
			got, err := o.CommitRecord(ctx, s, s.InstallationID, auth, in)
			if commits != 1 || got.Status != SaveConfirmed || !got.ViewDeliveryFailed || !errors.Is(err, ErrSaveViewDelivery) {
				t.Fatal(got, err, commits)
			}
			if mode == "revoked-after-commit" {
				if got.Receipt != nil || !errors.Is(err, denied) {
					t.Fatal("late auth leaked receipt", got, err)
				}
			} else if got.Receipt == nil {
				t.Fatal("known commit was relabeled rollback", got, err)
			}
			if mode == "canceled-waiter" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			state, _ := l.State(s)
			if mode == "physical-after-commit" {
				if state != LifecycleUnknown || r.retained != 1 {
					t.Fatal(state, r.retained)
				}
			} else if state != LifecycleOpen || r.retained != 0 {
				t.Fatal(state, r.retained)
			}
		})
	}
}
func TestCommitOwnerUnknownNeverMintsAnotherAttempt(t *testing.T) {
	o, l, s, r, in := commitOwnerFixture(t)
	calls := 0
	auth := func(context.Context) error { return nil }
	r.admit = func(_ context.Context, _ Scope, _ SaveRequest, only bool) (SaveResult, AcceptedSave, error) {
		calls++
		if calls == 1 {
			if only {
				t.Fatal("fresh admission unexpectedly fenced")
			}
			return SaveResult{Status: SaveUnknown}, nil, ErrOpenUncertain
		}
		if !only {
			t.Fatal("unknown permitted new admission")
		}
		return SaveResult{Status: SaveConfirmed, Receipt: &Receipt{OperationID: in.OperationID}}, nil, nil
	}
	got, err := o.CommitRecord(context.Background(), s, s.InstallationID, auth, in)
	if got.Status != SaveUnknown || !errors.Is(err, ErrOpenUncertain) || r.retained != 1 {
		t.Fatal(got, err, r.retained)
	}
	state, _ := l.State(s)
	if state != LifecycleUnknown {
		t.Fatal(state)
	}
	got, err = o.CommitRecord(context.Background(), s, s.InstallationID, auth, in)
	if err != nil || got.Status != SaveConfirmed || calls != 2 {
		t.Fatal(got, err, calls)
	}
	state, _ = l.State(s)
	if state != LifecycleUnknown {
		t.Fatal("receipt for one key cleared physical/other-key uncertainty", state)
	}
}
func TestCommitOwnerRejectsBeforeAdmission(t *testing.T) {
	o, _, s, r, in := commitOwnerFixture(t)
	r.admit = func(context.Context, Scope, SaveRequest, bool) (SaveResult, AcceptedSave, error) {
		t.Fatal("invalid input performed IO")
		return SaveResult{}, nil, nil
	}
	in.PreviewVersion = 0
	if got, err := o.CommitRecord(context.Background(), s, s.InstallationID, func(context.Context) error { return nil }, in); !errors.Is(err, ErrSaveInput) || got.Status != SaveRejected {
		t.Fatal(got, err)
	}
	in.PreviewVersion = 1
	other := s
	other.WorkspaceID = s.TenantID
	if _, err := o.CommitRecord(context.Background(), other, s.InstallationID, func(context.Context) error { return nil }, in); !errors.Is(err, ErrScope) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := o.CommitRecord(ctx, s, s.InstallationID, func(context.Context) error { return nil }, in); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
