package workspace

import (
	"context"
	"errors"
	"testing"
)

func TestRequestClaimOneDeliveryAndCopiedReleaseDoesNotClearAnotherClaim(t *testing.T) {
	c := NewOperationCapacity()
	base, _ := c.ReserveOperation()
	defer base.Release()
	first, err := base.ClaimRequest()
	if err != nil {
		t.Fatal(err)
	}
	copyOfFirst := *first
	held, _ := first.Retain()
	defer held.Release()
	if _, err := held.ClaimRequest(); !errors.Is(err, ErrCapacity) {
		t.Fatal("resource retain amplified request claims", err)
	}
	first.Release()
	second, err := base.ClaimRequest()
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	copyOfFirst.Release()
	if _, err := base.ClaimRequest(); !errors.Is(err, ErrCapacity) {
		t.Fatal("old reference copy cleared new request claim", err)
	}
	if c.Usage() != 1 {
		t.Fatal(c.Usage())
	}
}

func TestRequestClaimOriginalSaveAtSaturationIsBoundedAndControlOnly(t *testing.T) {
	ctx := context.Background()
	o, _, s, r, in := commitOwnerFixture(t)
	check := func(context.Context) error { return nil }
	pre, err := o.ReserveOperation(ctx, s, check, false, false)
	if err != nil {
		t.Fatal(err)
	}
	request, err := pre.ClaimRequest()
	pre.Release()
	if err != nil {
		t.Fatal(err)
	}
	r.admit = func(context.Context, Scope, SaveRequest, bool) (SaveResult, AcceptedSave, error) {
		return SaveResult{Status: SaveUnknown}, nil, ErrOpenUncertain
	}
	if _, err := o.CommitRecord(request.Context(ctx), s, s.InstallationID, check, in); !errors.Is(err, ErrOpenUncertain) {
		t.Fatal(err)
	}
	if _, err := o.ClaimOperationInvestigation(ctx, s, s.InstallationID, in.OperationID, true, check); !errors.Is(err, ErrCapacity) {
		t.Fatal("original request still owns delivery", err)
	}
	request.Release()
	var extras []*OperationReservation
	defer func() {
		for _, v := range extras {
			v.Release()
		}
	}()
	for i := 0; i < 62; i++ {
		v, err := o.queue.capacity.ReserveOperation()
		if err != nil {
			t.Fatal(i, err)
		}
		extras = append(extras, v)
	}
	if _, err := o.queue.capacity.ReserveOperation(); !errors.Is(err, ErrCapacity) {
		t.Fatal(err)
	}
	if _, err := o.ClaimOperationInvestigation(ctx, s, s.TenantID, in.OperationID, false, check); !errors.Is(err, ErrCapacity) {
		t.Fatal("foreign principal borrowed original", err)
	}
	borrowed, err := o.ClaimOperationInvestigation(ctx, s, s.InstallationID, in.OperationID, false, check)
	if err != nil {
		t.Fatal(err)
	}
	defer borrowed.Release()
	if o.queue.capacity.Usage() != 63 {
		t.Fatal("borrow charged again", o.queue.capacity.Usage())
	}
	if _, err := o.ClaimOperationInvestigation(ctx, s, s.InstallationID, in.OperationID, true, check); !errors.Is(err, ErrCapacity) {
		t.Fatal("duplicate body/delivery admitted", err)
	}
	borrowedCtx := context.WithoutCancel(borrowed.Context(ctx))
	if _, err := o.GetSaveOperation(borrowedCtx, s, s.InstallationID, s.TenantID, true, check); !errors.Is(err, ErrScope) {
		t.Fatal("borrow changed operation key", err)
	}
	if _, err := o.CommitRecord(borrowedCtx, s, s.InstallationID, check, in); !errors.Is(err, ErrScope) {
		t.Fatal("borrow became ordinary save", err)
	}
	if _, err := o.Status(borrowedCtx, s, check); !errors.Is(err, ErrScope) {
		t.Fatal("borrow became unrelated status", err)
	}
	retained, _ := borrowed.Retain()
	defer retained.Release()
	if err := CheckInvestigationReservation(retained.Context(ctx), s.TenantID); !errors.Is(err, ErrScope) {
		t.Fatal("retain stripped key restriction", err)
	}
	if _, err := o.queue.capacity.RetainOperation(borrowedCtx); !errors.Is(err, ErrScope) {
		t.Fatal("borrow became new installation mutation", err)
	}
	o.queue.resolve("save:" + in.OperationID)
	if o.queue.capacity.Usage() != 63 {
		t.Fatal("resolving resource cleared active delivery")
	}
	borrowed.Release()
	retained.Release()
	if o.queue.capacity.Usage() != 62 {
		t.Fatal("last request reference did not retire", o.queue.capacity.Usage())
	}
}

func TestRequestClaimRejectsMetadataAndNamespaceOwnerSwitch(t *testing.T) {
	ctx := context.Background()
	a, _, scope := openTestOwner(t)
	c := a.queue.capacity
	base, err := c.reserve(a.queue, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Release()
	borrowed, err := base.ClaimInvestigation(scope.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	defer borrowed.Release()
	otherScope := scope
	otherScope.WorkspaceID = scope.TenantID
	lifecycle, _ := NewLifecycle(otherScope)
	b, _ := NewOpenOwnerWithCapacity(otherScope, lifecycle, c)
	b.closes[scope.WorkspaceID] = &closeOperation{view: CloseView{Scope: otherScope, OperationID: scope.WorkspaceID, PrincipalID: scope.InstallationID}}
	check := func(context.Context) error { return nil }
	if _, err := b.GetCloseOperation(borrowed.Context(ctx), otherScope, scope.InstallationID, scope.WorkspaceID, true, check); !errors.Is(err, ErrScope) {
		t.Fatal("same UUID metadata query crossed workspace owner", err)
	}
	if err := c.CheckInstallationInvestigation(borrowed.Context(ctx), scope.WorkspaceID); !errors.Is(err, ErrScope) {
		t.Fatal("workspace claim crossed installation namespace", err)
	}
	borrowed.Release()
	for _, bind := range []bool{false, true} {
		var r *OperationReservation
		if bind {
			r, err = c.reserve(a.queue, false, false)
		} else {
			r, err = c.ReserveOperation()
		}
		if err != nil {
			t.Fatal(err)
		}
		install, err := r.ClaimInstallationInvestigation(scope.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.GetSaveOperation(install.Context(ctx), scope, scope.InstallationID, scope.WorkspaceID, true, check); !errors.Is(err, ErrScope) {
			t.Fatal("installation claim became workspace control", bind, err)
		}
		if !bind && r.state.allocation.queue != nil {
			t.Fatal("rejected claim rebound installation allocation")
		}
		install.Release()
		r.Release()
	}
}

func TestRequestClaimColdUnknownRequiresCurrentOperatorAndScope(t *testing.T) {
	ctx := context.Background()
	o, _, s := openTestOwner(t)
	check := func(context.Context) error { return nil }
	ticket, err := o.queue.enter(ctx, false, false)
	if err != nil {
		t.Fatal(err)
	}
	ticket.retain("open")
	ticket.finish()
	if _, err := o.ClaimOperationInvestigation(ctx, s, s.InstallationID, s.WorkspaceID, false, check); !errors.Is(err, ErrCapacity) {
		t.Fatal(err)
	}
	denied := errors.New("synthetic revoked")
	if _, err := o.ClaimOperationInvestigation(ctx, s, s.InstallationID, s.WorkspaceID, true, func(context.Context) error { return denied }); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	borrowed, err := o.ClaimOperationInvestigation(ctx, s, s.InstallationID, s.WorkspaceID, true, check)
	if err != nil {
		t.Fatal(err)
	}
	defer borrowed.Release()
	other := newOperationQueue(o.queue.capacity)
	if turn, err := other.enter(borrowed.Context(ctx), true, false); !errors.Is(err, ErrScope) {
		if turn != nil {
			turn.finish()
		}
		t.Fatal("cold reference crossed scope", err)
	}
	o.queue.resolve("open")
	borrowed.Release()
	if o.queue.capacity.Usage() != 0 {
		t.Fatal(o.queue.capacity.Usage())
	}
}
