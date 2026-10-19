package workspace

import (
	"context"
	"errors"
	"testing"
)

// These tests exercise owner admission/cleanup faults only. The installation
// package separately exercises this owner with real SQLite and native leases.
type openTestResources struct {
	closed      int
	closeErr    error
	validateErr error
}

func (r *openTestResources) Validate() error { return r.validateErr }
func (r *openTestResources) Close() error    { r.closed++; return r.closeErr }
func (r *openTestResources) Snapshot(context.Context, Scope) (Snapshot, error) {
	return Snapshot{}, nil
}
func (r *openTestResources) Current(context.Context, Scope, string) (TextRecord, error) {
	return TextRecord{}, nil
}
func (r *openTestResources) List(context.Context, Scope, string, int) ([]TextRecord, error) {
	return nil, nil
}
func openTestOwner(t *testing.T) (*OpenOwner, *Lifecycle, Scope) {
	t.Helper()
	scope := Scope{"10000000-0000-4000-8000-000000000001", "10000000-0000-4000-8000-000000000002", "10000000-0000-4000-8000-000000000003"}
	l, err := NewLifecycle(scope)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewOpenOwner(scope, l)
	if err != nil {
		t.Fatal(err)
	}
	return owner, l, scope
}
func TestOpenOwnerLateAuthorizationAndCleanupFence(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "known", true: "unknown"}[unknown], func(t *testing.T) {
			owner, l, scope := openTestOwner(t)
			denied := errors.New("synthetic revoked authorization")
			factoryRan := false
			auth := func(context.Context) error {
				if factoryRan {
					return denied
				}
				return nil
			}
			r := &openTestResources{}
			if unknown {
				r.closeErr = ErrOpenUncertain
			}
			got, err := owner.Open(context.Background(), scope, auth, func(context.Context, string) (OpenEvidence, error) {
				factoryRan = true
				return OpenEvidence{Resources: r, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}}, nil
			})
			if !errors.Is(err, denied) || got.Epoch != "" || r.closed != 1 {
				t.Fatalf("late authorization %+v %v close=%d", got, err, r.closed)
			}
			state, _ := l.State(scope)
			expected := LifecycleClosed
			if unknown {
				expected = LifecycleUnknown
			}
			if state != expected {
				t.Fatalf("state %s want %s", state, expected)
			}
			if unknown {
				called := false
				_, err = owner.Open(context.Background(), scope, func(context.Context) error { return nil }, func(context.Context, string) (OpenEvidence, error) { called = true; return OpenEvidence{}, nil })
				if !errors.Is(err, ErrLifecycle) || called {
					t.Fatal("cleanup uncertainty replaced owner", err)
				}
			}
		})
	}
}
func TestOpenOwnerFailedFactoryRetainsPartialResources(t *testing.T) {
	owner, l, scope := openTestOwner(t)
	r := &openTestResources{closeErr: ErrOpenUncertain}
	cause := errors.New("synthetic factory failure")
	got, err := owner.Open(context.Background(), scope, func(context.Context) error { return nil }, func(context.Context, string) (OpenEvidence, error) { return OpenEvidence{Resources: r}, cause })
	if !errors.Is(err, cause) || !errors.Is(err, ErrOpenUncertain) || got.Epoch != "" || r.closed != 1 {
		t.Fatal("partial factory cleanup lost", got, err)
	}
	state, _ := l.State(scope)
	if state != LifecycleUnknown {
		t.Fatal(state)
	}
	if err = owner.DrainOpen(context.Background(), scope, func(context.Context) error { return nil }); !errors.Is(err, ErrOpenUncertain) {
		t.Fatal("failed cleanup magically cleared", err)
	}
	state, _ = l.State(scope)
	if state != LifecycleUnknown {
		t.Fatal(state)
	}
}
func TestOpenOwnerCannotDrainForeignUncertainty(t *testing.T) {
	for _, afterOpen := range []bool{false, true} {
		owner, l, scope := openTestOwner(t)
		auth := func(context.Context) error { return nil }
		if afterOpen {
			_, err := owner.Open(context.Background(), scope, auth, func(context.Context, string) (OpenEvidence, error) {
				return OpenEvidence{Resources: &openTestResources{}, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if err = owner.DrainOpen(context.Background(), scope, auth); err != nil {
				t.Fatal(err)
			}
		}
		if err := l.WithClosed(context.Background(), scope, func() error { return ErrLifecycleUncertain }); !errors.Is(err, ErrLifecycleUncertain) {
			t.Fatal(err)
		}
		if err := owner.DrainOpen(context.Background(), scope, auth); !errors.Is(err, ErrLifecycle) {
			t.Fatal("foreign fence cleared", err)
		}
		if state, _ := l.State(scope); state != LifecycleUnknown {
			t.Fatal("false safe state", state)
		}
	}
}

func TestOpenOwnerAuthorizationBeforeFactory(t *testing.T) {
	owner, l, scope := openTestOwner(t)
	denied := errors.New("synthetic denied")
	called := false
	_, err := owner.Open(context.Background(), scope, func(context.Context) error { return denied }, func(context.Context, string) (OpenEvidence, error) { called = true; return OpenEvidence{}, nil })
	if !errors.Is(err, denied) || called {
		t.Fatal("unauthorized IO", err)
	}
	state, _ := l.State(scope)
	if state != LifecycleClosed {
		t.Fatal(state)
	}
}
