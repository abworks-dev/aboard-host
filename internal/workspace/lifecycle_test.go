package workspace

import (
	"context"
	"errors"
	"testing"
	"time"
)

func lifecycleFixture(t *testing.T) (*Lifecycle, Scope) {
	t.Helper()
	const id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	scope := Scope{InstallationID: id, TenantID: id, WorkspaceID: id}
	l, err := NewLifecycle(scope)
	if err != nil {
		t.Fatal(err)
	}
	return l, scope
}

func TestLifecycleScopeMismatch(t *testing.T) {
	l, scope := lifecycleFixture(t)
	if _, err := NewLifecycle(Scope{}); !errors.Is(err, ErrScope) {
		t.Fatalf("invalid construction: %v", err)
	}
	for _, field := range []string{"installation", "tenant", "workspace"} {
		bad := scope
		const other = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		switch field {
		case "installation":
			bad.InstallationID = other
		case "tenant":
			bad.TenantID = other
		case "workspace":
			bad.WorkspaceID = other
		}
		called := false
		checks := []func() error{
			func() error {
				return l.WithClosed(context.Background(), bad, func() error { called = true; return nil })
			},
			func() error { return l.BeginOpen(bad) },
			func() error { return l.MarkOpen(bad) },
			func() error { return l.BeginDrain(bad) },
			func() error { return l.FinishDrain(bad, DrainSucceeded) },
			func() error { _, err := l.State(bad); return err },
		}
		for _, check := range checks {
			if err := check(); !errors.Is(err, ErrScope) {
				t.Fatalf("%s mismatch: %v", field, err)
			}
		}
		if called {
			t.Fatal("mismatched binding action ran")
		}
	}
	if state, err := l.State(scope); err != nil || state != LifecycleClosed {
		t.Fatalf("mismatch changed owner: %s %v", state, err)
	}
}

func TestLifecycleTransitionsAndDrainEvidence(t *testing.T) {
	for _, outcome := range []DrainOutcome{DrainFailed, DrainUnknown, DrainOutcome(255)} {
		l, scope := lifecycleFixture(t)
		if err := l.MarkOpen(scope); !errors.Is(err, ErrLifecycle) {
			t.Fatalf("mark open without admission: %v", err)
		}
		if err := l.FinishDrain(scope, DrainSucceeded); !errors.Is(err, ErrLifecycle) {
			t.Fatalf("finish without drain: %v", err)
		}
		if err := l.BeginOpen(scope); err != nil {
			t.Fatal(err)
		}
		assertBindingRejected(t, l, scope)
		if err := l.MarkOpen(scope); err != nil {
			t.Fatal(err)
		}
		assertBindingRejected(t, l, scope)
		if err := l.BeginOpen(scope); !errors.Is(err, ErrLifecycle) {
			t.Fatalf("double open: %v", err)
		}
		if err := l.BeginDrain(scope); err != nil {
			t.Fatal(err)
		}
		assertBindingRejected(t, l, scope)
		if err := l.FinishDrain(scope, outcome); !errors.Is(err, ErrLifecycle) {
			t.Fatalf("non-success drain: %v", err)
		}
		if state, _ := l.State(scope); state != LifecycleUnknown {
			t.Fatalf("failed/unknown physical drain became %s", state)
		}
		assertBindingRejected(t, l, scope)
		if err := l.BeginOpen(scope); !errors.Is(err, ErrLifecycle) {
			t.Fatalf("unknown admitted open: %v", err)
		}
		if err := l.FinishDrain(scope, DrainSucceeded); !errors.Is(err, ErrLifecycle) {
			t.Fatalf("unknown bypassed explicit drain: %v", err)
		}
		if err := l.BeginDrain(scope); err != nil {
			t.Fatal(err)
		}
		if err := l.FinishDrain(scope, DrainSucceeded); err != nil {
			t.Fatal(err)
		}
		if state, _ := l.State(scope); state != LifecycleClosed {
			t.Fatalf("trusted physical completion did not close: %s", state)
		}
	}
}

func assertBindingRejected(t *testing.T, l *Lifecycle, scope Scope) {
	t.Helper()
	called := false
	if err := l.WithClosed(context.Background(), scope, func() error { called = true; return nil }); !errors.Is(err, ErrLifecycle) || called {
		t.Fatalf("nonclosed binding admission: called=%v error=%v", called, err)
	}
}

func TestLifecycleClosedActionExcludesOpen(t *testing.T) {
	l, scope := lifecycleFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	actionDone, openDone := make(chan error, 1), make(chan error, 1)
	go func() {
		actionDone <- l.WithClosed(context.Background(), scope, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	// Deterministic check that the action retains the admission mutex.
	if l.mu.TryLock() {
		l.mu.Unlock()
		close(release)
		t.Fatal("binding action did not retain lifecycle mutex")
	}
	go func() { openDone <- l.BeginOpen(scope) }()
	select {
	case err := <-openDone:
		close(release)
		t.Fatalf("open escaped closed binding action: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-actionDone; err != nil {
		t.Fatal(err)
	}
	if err := <-openDone; err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleUncertainActionFencesOpen(t *testing.T) {
	for _, want := range []error{ErrLifecycleUncertain, errors.Join(errors.New("physical close failed"), ErrLifecycleUncertain)} {
		l, scope := lifecycleFixture(t)
		if err := l.WithClosed(context.Background(), scope, func() error { return want }); !errors.Is(err, ErrLifecycleUncertain) || err != want {
			t.Fatalf("uncertain action error lost: %v", err)
		}
		if state, err := l.State(scope); err != nil || state != LifecycleUnknown {
			t.Fatalf("uncertain action did not fence: %s %v", state, err)
		}
		if err := l.BeginOpen(scope); !errors.Is(err, ErrLifecycle) {
			t.Fatalf("uncertain action admitted open: %v", err)
		}
		assertBindingRejected(t, l, scope)
	}
}

func TestLifecycleCanceledContextAndActionError(t *testing.T) {
	l, scope := lifecycleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if err := l.WithClosed(ctx, scope, func() error { called = true; return nil }); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("canceled context: called=%v error=%v", called, err)
	}
	want := errors.New("binding failed")
	if err := l.WithClosed(context.Background(), scope, func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("action error lost: %v", err)
	}
	if err := l.BeginOpen(scope); err != nil {
		t.Fatalf("action error retained mutex: %v", err)
	}
}
