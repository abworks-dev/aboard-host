package workspace

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOperationKeysCutoffDoesNotWaitForAdmissionOrCommitIO(t *testing.T) {
	for _, phase := range []string{"admission", "commit"} {
		t.Run(phase, func(t *testing.T) {
			o, _, scope, r, input := commitOwnerFixture(t)
			allow := func(context.Context) error { return nil }
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			block := func() { close(started); <-release }
			r.admit = func(context.Context, Scope, SaveRequest, bool) (SaveResult, AcceptedSave, error) {
				if phase == "admission" {
					block()
				}
				return SaveResult{Status: SaveAccepted}, commitCapability(func(context.Context) (SaveResult, error) {
					if phase == "commit" {
						block()
					}
					return SaveResult{Status: SaveConfirmed, Receipt: &Receipt{OperationID: input.OperationID}}, nil
				}), nil
			}
			saved := make(chan error, 1)
			go func() {
				_, err := o.CommitRecord(context.Background(), scope, scope.InstallationID, allow, input)
				saved <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("save did not reach blocked port")
			}
			collision := make(chan error, 1)
			go func() {
				_, err := o.CloseWorkspace(context.Background(), scope, scope.InstallationID, input.OperationID, input.Epoch, allow)
				collision <- err
			}()
			select {
			case err := <-collision:
				if !errors.Is(err, ErrSaveKeyReuse) {
					t.Fatal("same-key close not rejected", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("same-key close waited for physical IO")
			}
			o.queue.mu.Lock()
			cutoff := o.queue.closing
			o.queue.mu.Unlock()
			if cutoff {
				t.Fatal("rejected kind conflict cut off ordinary admission")
			}
			closing := make(chan error, 1)
			go func() {
				_, err := o.CloseWorkspace(context.Background(), scope, scope.InstallationID, scope.TenantID, input.Epoch, allow)
				closing <- err
			}()
			select {
			case err := <-closing:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("unrelated close waited for physical IO before cutoff")
			}
			if r.closed != 0 {
				t.Fatal("close released active resources")
			}
			unblock()
			if err := <-saved; err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if view, err := o.WaitClose(ctx, scope, scope.InstallationID, scope.TenantID, false, allow); err != nil || !view.Closed {
				t.Fatal(view, err)
			}
		})
	}
}

func TestOperationKeysForgetOnlyProvenNewNonAdmission(t *testing.T) {
	for _, mode := range []string{"unadmitted", "durable-rejected", "unknown", "existing-mismatch", "new-key-mismatch", "accepted-late-revoked"} {
		t.Run(mode, func(t *testing.T) {
			o, _, scope, r, input := commitOwnerFixture(t)
			if mode == "existing-mismatch" {
				o.saveKeys[input.OperationID] = struct{}{}
			}
			denied := errors.New("synthetic current authority revoked")
			revoked := false
			authorize := func(context.Context) error {
				if revoked {
					return denied
				}
				return nil
			}
			r.admit = func(context.Context, Scope, SaveRequest, bool) (SaveResult, AcceptedSave, error) {
				switch mode {
				case "unadmitted":
					return SaveResult{Status: SaveRejected}, nil, ErrBaseline
				case "durable-rejected":
					return SaveResult{Status: SaveRejected, FailureCode: "WRITE_FAILED"}, nil, nil
				case "unknown":
					return SaveResult{Status: SaveUnknown}, nil, ErrOpenUncertain
				case "existing-mismatch", "new-key-mismatch":
					return SaveResult{Status: SaveRejected}, nil, ErrSaveKeyReuse
				default:
					revoked = true
					return SaveResult{Status: SaveAccepted}, commitCapability(func(context.Context) (SaveResult, error) {
						t.Fatal("revoked admission executed")
						return SaveResult{}, nil
					}), nil
				}
			}
			_, _ = o.CommitRecord(context.Background(), scope, scope.InstallationID, authorize, input)
			o.closeMu.Lock()
			_, known := o.saveKeys[input.OperationID]
			o.closeMu.Unlock()
			if known != (mode != "unadmitted") {
				t.Fatal("admission classification lost/overretained scoped key", mode, known)
			}
			allow := func(context.Context) error { return nil }
			view, err := o.CloseWorkspace(context.Background(), scope, scope.InstallationID, input.OperationID, input.Epoch, allow)
			if mode != "unadmitted" {
				if !errors.Is(err, ErrSaveKeyReuse) || view.OperationID != "" {
					t.Fatal(view, err)
				}
				return
			}
			if err != nil {
				t.Fatal("unadmitted key could not become close", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if v, err := o.WaitClose(ctx, scope, scope.InstallationID, input.OperationID, false, allow); err != nil || !v.Closed {
				t.Fatal(v, err)
			}
		})
	}
}

func TestOpenOperationKeysRejectMalformedOrConflictingHistory(t *testing.T) {
	for _, mode := range []string{"invalid", "duplicate", "close-conflict", "valid"} {
		t.Run(mode, func(t *testing.T) {
			o, l, scope := openTestOwner(t)
			keys := []string{scope.WorkspaceID}
			if mode == "invalid" {
				keys[0] = "not-an-operation-uuid"
			}
			if mode == "duplicate" {
				keys = append(keys, keys[0])
			}
			if mode == "close-conflict" {
				o.closes[scope.WorkspaceID] = &closeOperation{}
			}
			r := &openTestResources{}
			allow := func(context.Context) error { return nil }
			view, err := o.Open(context.Background(), scope, allow, func(context.Context, string) (OpenEvidence, error) {
				return OpenEvidence{Resources: r, Snapshot: Snapshot{Identity: Identity{WorkspaceID: scope.WorkspaceID, TenantID: scope.TenantID, BusinessState: "ready"}}, SaveOperationIDs: keys}, nil
			})
			if mode == "valid" {
				if err != nil || view.Epoch == "" {
					t.Fatal(view, err)
				}
				keys[0] = scope.TenantID // Caller cannot mutate the published denial index.
				if _, err := o.CloseWorkspace(context.Background(), scope, scope.InstallationID, scope.WorkspaceID, view.Epoch, allow); !errors.Is(err, ErrSaveKeyReuse) {
					t.Fatal(err)
				}
				if err := o.DrainOpen(context.Background(), scope, allow); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || view.Epoch != "" || r.closed != 1 {
				t.Fatal("invalid index published/retained ordinary view", view, err, r.closed)
			}
			if state, _ := l.State(scope); state != LifecycleClosed {
				t.Fatal(state)
			}
		})
	}
}
