//go:build windows && amd64

package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// Real client Context expiration/cancellation after durable admission and two
// successful native journal syncs. Neither the waiter nor close may end the IO.
func TestV014IndependentCloseDuringNativeAndLostWaiter(t *testing.T) {
	for _, mode := range []string{"deadline", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			setup := f.start(t)
			initial := isolationConnect(t, setup)
			var actor protocol.PrincipalView
			initial.request(t, "GET", "/v1/context", nil, &actor, 200)
			root, a := newInspectionWorkspace(t, initial, "save-target")
			_, b := newInspectionWorkspace(t, initial, "independent-B")
			setup.stop(t)
			key := newID(t)
			staged := saveNativeFixture(t, f, key, a.WorkspaceId, actor.PrincipalId, "formal-inflight", 0)
			p := staged.start(t)
			c := isolationConnect(t, p)
			joinOriginalSessionDelivery(t, c)
			a = c.open(t, a.WorkspaceId)
			b = c.open(t, b.WorkspaceId)
			pa := "/v1/workspaces/" + string(a.WorkspaceId)
			candidate := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "V014 original", Body: "durably admitted native IO outlives its waiting Context and close"})
			in := saveInput(key, a, candidate)
			raw, e := json.Marshal(in)
			if e != nil {
				t.Fatal(e)
			}
			var ctx context.Context
			var cancel context.CancelFunc
			if mode == "deadline" {
				ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()
			req, e := http.NewRequestWithContext(ctx, "POST", "http://"+c.authority+pa+"/record-commits", bytes.NewReader(raw))
			if e != nil {
				t.Fatal(e)
			}
			req.Header.Set("Authorization", "Bearer "+c.token)
			req.Header.Set("Content-Type", "application/json")
			result := make(chan error, 1)
			// No testing callbacks in the worker. This request intentionally loses its
			// delivery; all assertions and secret observations stay on the test goroutine.
			go func() {
				r, err := http.DefaultClient.Do(req)
				if r != nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 4097))
					_ = r.Body.Close()
				}
				result <- err
			}()
			marker := waitSaveCheckpoint(t, p, in, a, actor, "formal-inflight")
			if ctx.Err() != nil {
				t.Fatal("client Context ended before native midpoint observation")
			}
			if mode == "cancel" {
				cancel()
			}
			var waiterErr error
			select {
			case waiterErr = <-result:
			case <-time.After(5 * time.Second):
				t.Fatal("waiting client did not physically return")
			}
			expected := context.Canceled
			if mode == "deadline" {
				expected = context.DeadlineExceeded
			}
			if !errors.Is(waiterErr, expected) || ctx.Err() != expected {
				t.Fatal("real client Context did not end at requested boundary")
			}
			// The callback is still blocked after the waiting client has returned.
			still := waitSaveCheckpoint(t, p, in, a, actor, "formal-inflight")
			if still.Native == nil || !still.Native.InFlight || still.Native.CommitReturned || still.Native.CallbackDrained {
				t.Fatal("waiter ended native IO")
			}
			closeKey := newID(t)
			var closing protocol.OperationView
			c.request(t, "POST", pa+"/close", protocol.CloseWorkspace{OperationId: closeKey, OpenEpoch: *a.OpenEpoch}, &closing, 202)
			checkDraining := func(v protocol.OperationView) {
				if v.OperationId != closeKey || v.WorkspaceId == nil || *v.WorkspaceId != a.WorkspaceId || v.Kind != "close" || v.LogicalState != "accepted" || v.PhysicalState != "executing" || v.PersistenceState != "unavailable" || v.Receipt != nil {
					t.Fatal("close published terminal/durable result while native IO remained")
				}
			}
			checkDraining(closing)
			var polled protocol.OperationView
			c.request(t, "GET", pa+"/operations/"+string(closeKey), nil, &polled, 200)
			checkDraining(polled)
			var state protocol.ErrorView
			c.request(t, "GET", pa, nil, &state, 409)
			// General status needs a queue turn; this refusal is NOT proof that
			// the directory is closed. Only close metadata reports its live phase.
			if state.Code != protocol.ErrorCodeWORKSPACECLOSED || state.Outcome != "rejected" || state.Scope == nil || *state.Scope != "workspace" {
				t.Fatal("drain cutoff admitted an unrelated workspace queue turn")
			}
			denied := in
			denied.OperationId = newID(t)
			var rejection protocol.ErrorView
			c.request(t, "POST", pa+"/record-commits", denied, &rejection, 409)
			if rejection.Code != protocol.ErrorCodeWORKSPACECLOSED || rejection.Outcome != "rejected" {
				t.Fatal("close did not fence new admission")
			}
			if lease, err := platform.Acquire(root, platform.Workspace, false, false); err == nil {
				_ = lease.Close()
				t.Fatal("cancel/close prematurely released native exclusion")
			}
			// A close is not an installation-wide cancellation.
			created := isolationSave(t, c, b, newID(t), protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B during A draining", Body: "independent actual B commit"})
			updated := isolationUpdate(t, c, b, "1", created, "B updated while A remains in original native COMMIT")
			isolationRows(t, saveFacts(t, c, b.WorkspaceId, "2", 1), updated.Row)
			again := waitSaveCheckpoint(t, p, in, a, actor, "formal-inflight")
			if again.Native == nil || !again.Native.InFlight {
				t.Fatal("B or close released native checkpoint")
			}
			releaseSaveCheckpoint(t, f)
			deadline := time.NewTimer(8 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			var drained protocol.OperationView
			for {
				c.request(t, "GET", pa+"/operations/"+string(closeKey), nil, &drained, 200)
				if drained.LogicalState == "committed" && drained.PhysicalState == "drained" {
					break
				}
				checkDraining(drained)
				select {
				case <-tick.C:
				case <-deadline.C:
					t.Fatal("released original failed physical close")
				}
			}
			if drained.PersistenceState != "unavailable" || drained.Receipt != nil {
				t.Fatal("memory close fabricated a durable save receipt")
			}
			// Actual native reacquisition, not simply a closed flag. End it before reopen.
			if lease, err := platform.Acquire(root, platform.Workspace, false, false); err != nil {
				t.Fatal("terminal close retained actual writer exclusion")
			} else if lease.Close() != nil {
				t.Fatal("closed native probe failed")
			}
			checkReady := func(v protocol.WorkspaceView) {
				if v.WorkspaceId != a.WorkspaceId || v.BusinessState != "ready" || v.DirectoryLifecycle != "open" || v.AccessMode != "writable" || v.Integrity != "valid" || v.PersistenceHealth.State != "known" {
					t.Fatal("actual revalidation lost independent workspace state dimensions")
				}
			}
			reopened := c.open(t, a.WorkspaceId)
			checkReady(reopened)
			if reopened.OpenEpoch == nil || *reopened.OpenEpoch == *a.OpenEpoch {
				t.Fatal("close/reopen reused epoch")
			}
			var original protocol.OperationView
			c.request(t, "GET", pa+"/operations/"+string(key), nil, &original, 200)
			if original.OperationId != key || original.Kind != "save" || original.LogicalState != "committed" || original.PhysicalState != "drained" || original.PersistenceState != "known" || original.Receipt == nil {
				t.Fatal("accepted original did not finish durably after waiter/epoch retirement")
			}
			saveMatches(t, saveFacts(t, c, a.WorkspaceId, "1", 1)[0], *original.Receipt, candidate)
			stale := in
			stale.OperationId = newID(t)
			var oldEpoch protocol.ErrorView
			c.request(t, "POST", pa+"/record-commits", stale, &oldEpoch, 409)
			if oldEpoch.Code != protocol.ErrorCodeSTALEHANDLE || oldEpoch.Outcome != "rejected" {
				t.Fatal("retired epoch acquired fresh write authority")
			}
			var same protocol.Receipt
			c.request(t, "POST", pa+"/record-commits", in, &same, 200)
			if same != *original.Receipt {
				t.Fatal("old original key replayed instead of returning receipt")
			}
			closePublic(t, c, reopened)
			closePublic(t, c, b)
			p.stop(t)
			next := f.start(t)
			d := isolationConnect(t, next)
			// Revalidate the actual directory before reading save history; no cold
			// lookup is allowed to implicitly open storage or bypass its lifecycle.
			fresh := d.open(t, a.WorkspaceId)
			checkReady(fresh)
			if fresh.OpenEpoch == nil || *fresh.OpenEpoch == *reopened.OpenEpoch {
				t.Fatal("fresh process reused closed epoch")
			}
			// Same logical identity, but close history was only in the old process.
			var missing protocol.ErrorView
			d.request(t, "GET", pa+"/operations/"+string(closeKey), nil, &missing, 409)
			if missing.Code != protocol.ErrorCodeOPERATIONHISTORYUNAVAILABLE || missing.Outcome != "rejected" {
				t.Fatal("fresh process invented old close success/rollback")
			}
			var cold protocol.OperationView
			d.request(t, "GET", pa+"/operations/"+string(key), nil, &cold, 200)
			if cold.Receipt == nil || *cold.Receipt != same || cold.LogicalState != "committed" || cold.PersistenceState != "known" {
				t.Fatal("save receipt lost across restart while close history disappeared")
			}
			saveMatches(t, saveFacts(t, d, a.WorkspaceId, "1", 1)[0], same, candidate)
			freshB := d.open(t, b.WorkspaceId)
			isolationRows(t, saveFacts(t, d, b.WorkspaceId, "2", 1), updated.Row)
			closePublic(t, d, fresh)
			closePublic(t, d, freshB)
			next.stop(t)
			isolationDiagnostics(t, p, c.token, string(candidate.Body), string(updated.Row.Body))
			isolationDiagnostics(t, next, d.token, string(candidate.Body), string(updated.Row.Body))
			isolationEvidence(t, f, "drain-native-waiter", struct {
				Mode                                                          string
				Marker, AfterWaiter, BeforeRelease                            observedSaveCheckpoint
				CloseAccepted, CloseDrained, Original, Cold                   protocol.OperationView
				MissingClose                                                  protocol.ErrorView
				NewAdmission, OldEpoch                                        protocol.ErrorView
				During                                                        protocol.ErrorView
				Reopened, Fresh                                               protocol.WorkspaceView
				BUpdate                                                       protocol.Receipt
				ClientEndedBeforeRelease, ActualLockReacquired, AllHostsEnded bool
			}{mode, marker, still, again, closing, drained, original, cold, missing, rejection, oldEpoch, state, reopened, fresh, updated.Receipt, true, true, true}, initial.token, c.token, d.token, string(candidate.Body), string(updated.Row.Body))
		})
	}
}
