//go:build windows && amd64 && aboard_acceptance

package sqlite

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAcceptanceObserveOriginalCommit(t *testing.T) {
	d, g := newTestGate(t)
	defer g.EndPhysical()
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	a := prepareAdmission(t, g, c, r)
	calls := 0
	var retained *AcceptanceCommitObservation
	result, err := g.AcceptanceObserveCommit(a, r, func(o *AcceptanceCommitObservation) (SaveResult, error) {
		calls++
		retained = o
		before := o.Snapshot()
		if before.Calls != 0 || before.InFlight || before.Returned {
			t.Error("observer fabricated original entry")
		}
		return g.CommitAccepted(context.Background(), a)
	})
	after := retained.Snapshot()
	if err != nil || result.Status != Confirmed || calls != 1 || after.Calls != 1 || !after.Returned || after.InFlight || after.Code != 0 {
		t.Fatalf("original observation: %+v %v %+v", result, err, after)
	}
	if _, exists := acceptanceBusyCommits.Load(g); exists {
		t.Fatal("observer remains armed")
	}
	assertFormalState(t, d, 1)
}
func TestAcceptanceObserveRejectsEveryChangedRequestField(t *testing.T) {
	typ := reflect.TypeOf(SaveRequest{})
	for i := 0; i < typ.NumField(); i++ {
		field := i
		t.Run(typ.Field(i).Name, func(t *testing.T) {
			_, g := newTestGate(t)
			defer g.EndPhysical()
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			a := prepareAdmission(t, g, c, r)
			different := r
			value := reflect.ValueOf(&different).Elem().Field(field)
			switch value.Kind() {
			case reflect.String:
				value.SetString(value.String() + "-changed")
			case reflect.Int64:
				value.SetInt(value.Int() + 1)
			default:
				t.Fatal("unhandled field: exact-request test must evolve")
			}
			calls := 0
			result, err := g.AcceptanceObserveCommit(a, different, func(*AcceptanceCommitObservation) (SaveResult, error) {
				calls++
				return SaveResult{Status: Confirmed}, nil
			})
			if calls != 0 || err == nil || result.Status != Unknown || a.used {
				t.Fatal("changed exact metadata consumed original capability")
			}
			if _, exists := acceptanceBusyCommits.Load(g); exists {
				t.Fatal("rejection armed observer")
			}
		})
	}
}
func TestAcceptanceObserveCapabilityAndPanic(t *testing.T) {
	for _, mode := range []string{"nil", "foreign", "used", "closed", "fenced", "nil-apply", "panic-string", "panic-nil"} {
		t.Run(mode, func(t *testing.T) {
			_, g := newTestGate(t)
			defer func() { g.closed = false; _ = g.EndPhysical() }()
			c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			a := prepareAdmission(t, g, c, r)
			calls := 0
			apply := func(*AcceptanceCommitObservation) (SaveResult, error) {
				calls++
				if mode == "panic-string" {
					panic("policy-only panic")
				}
				panic(nil)
			}
			switch mode {
			case "nil":
				a = nil
			case "foreign":
				a = &Admission{}
			case "used":
				a.used = true
			case "closed":
				g.closed = true
			case "fenced":
				g.fenced = true
			case "nil-apply":
				apply = nil
			}
			result, err := g.AcceptanceObserveCommit(a, r, apply)
			isPanic := mode == "panic-string" || mode == "panic-nil"
			if result.Status != Unknown || !errors.Is(err, ErrUnknown) || (isPanic && (calls != 1 || !g.fenced)) || (!isPanic && calls != 0) {
				t.Fatal("capability/panic did not conservatively end observer")
			}
			if _, exists := acceptanceBusyCommits.Load(g); exists {
				t.Fatal("observer not disarmed")
			}
		})
	}
}
