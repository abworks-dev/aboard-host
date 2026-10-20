package workspace

import (
	"context"
	"errors"
	"testing"
)

// Metadata-only lookup must not enqueue a new runnable control ahead of drain.
// This policy test complements the separate real native COMMIT/Host timeline.
func TestRequestClaimCloseMetadataDuringDrainIsExactAndBounded(t *testing.T) {
	ctx := context.Background()
	o, _, s := openTestOwner(t)
	check := func(context.Context) error { return nil }
	ref, err := o.queue.capacity.reserve(o.queue, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	key := s.WorkspaceID
	o.closes[key] = &closeOperation{view: CloseView{Scope: s, OperationID: key, PrincipalID: s.InstallationID, Epoch: s.TenantID, State: LifecycleDraining}, reservation: ref}
	o.queue.drainWaiting = true
	for _, tc := range []struct {
		principal, key string
		operator       bool
	}{{s.TenantID, key, false}, {s.InstallationID, s.TenantID, true}} {
		if r, e := o.ClaimOperationInvestigation(ctx, s, tc.principal, tc.key, tc.operator, check); e == nil {
			r.Release()
			t.Fatal("wrong actor/key borrowed closing allocation")
		}
	}
	denied := errors.New("synthetic current authorization revoked")
	if _, e := o.ClaimOperationInvestigation(ctx, s, s.InstallationID, key, false, func(context.Context) error { return denied }); !errors.Is(e, denied) {
		t.Fatal("close lookup bypassed current authorization", e)
	}
	claim, err := o.ClaimOperationInvestigation(ctx, s, s.InstallationID, key, false, check)
	if err != nil {
		t.Fatal("exact metadata lookup starved behind physical drain", err)
	}
	defer claim.Release()
	if o.queue.capacity.Usage() != 1 {
		t.Fatal("metadata allocated a second operation")
	}
	if _, err = o.ClaimOperationInvestigation(ctx, s, s.InstallationID, key, true, check); !errors.Is(err, ErrCapacity) {
		t.Fatal("unbounded concurrent close lookup", err)
	}
	v, err := o.GetCloseOperation(claim.Context(ctx), s, s.InstallationID, key, false, check)
	if err != nil || v.Closed || v.State != LifecycleDraining {
		t.Fatal("metadata query fabricated safe physical completion", err)
	}
	if _, err = o.GetCloseOperation(claim.Context(ctx), s, s.InstallationID, s.TenantID, true, check); !errors.Is(err, ErrScope) {
		t.Fatal("metadata claim changed key", err)
	}
	if _, err = o.Status(claim.Context(ctx), s, check); !errors.Is(err, ErrScope) {
		t.Fatal("metadata claim entered general workspace IO", err)
	}
	if _, err = o.queue.enter(ctx, true, false); !errors.Is(err, ErrLifecycle) {
		t.Fatal("metadata lookup unblocked queued controls ahead of drain", err)
	}
}
