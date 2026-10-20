//go:build windows && amd64

package acceptance

import (
	"fmt"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	win "golang.org/x/sys/windows"
)

type writerMatrixIdentity struct {
	PID   int
	Birth uint64
	Image string
}

type writerMatrixResult struct {
	Mode                    string
	Round                   int
	Baseline                protocol.Receipt
	WinnerInput, LoserInput writerInput
	Winner, Loser           writerReply
	WinnerIdentity          writerMatrixIdentity
	ContenderIdentities     []writerMatrixIdentity
	RefusedOpen             protocol.ErrorView
	MissingLoser            protocol.ErrorView
	RecoveredRows           []protocol.TextRecordView
	HostPID                 int
	HostBirth               uint64
	HostWasKilled           bool
	FinalHostEnded          bool
}

func writerMatrixInput(t *testing.T, f *fixture, root string, view protocol.WorkspaceView, principal protocol.PrincipalView) writerInput {
	t.Helper()
	return writerInput{Fixture: f.root, Root: root, Scope: workspace.Scope{InstallationID: string(principal.InstallationId), TenantID: string(principal.TenantId), WorkspaceID: string(view.WorkspaceId)}, PrincipalID: string(principal.PrincipalId), OperationID: string(newID(t)), CandidateID: string(newID(t)), ResourceID: string(newID(t)), Hold: true}
}
func writerMatrixPin(t *testing.T, p *writerProcess) writerMatrixIdentity {
	t.Helper()
	p.pin(t)
	if state, err := win.WaitForSingleObject(p.pinned, 0); err != nil || state != uint32(win.WAIT_TIMEOUT) {
		t.Fatal("writer boundary did not retain the actual live child")
	}
	return writerMatrixIdentity{p.cmd.Process.Pid, p.birth, p.image}
}
func writerMatrixBusy(t *testing.T, p *writerProcess, reply writerReply) {
	t.Helper()
	if reply.Phase != "busy" || !reply.Busy || reply.SQLiteAttempt || reply.Receipt != nil {
		t.Fatal("losing real process was not refused before SQLite")
	}
	p.wait(t)
}
func writerMatrixEnd(t *testing.T, p *writerProcess, saved writerReply) {
	t.Helper()
	if saved.Phase != "saved" || !saved.SQLiteAttempt || saved.Busy || saved.Receipt == nil {
		t.Fatal("winner did not confirm a real formal save")
	}
	if _, err := p.stdin.Write([]byte{1}); err != nil {
		t.Fatal("winner release unavailable")
	}
	ended := p.next(t)
	if ended.Phase != "ended" || !ended.SQLiteAttempt || ended.Receipt == nil || *ended.Receipt != *saved.Receipt {
		t.Fatal("winner release lost original confirmed history")
	}
	p.wait(t)
	if state, err := win.WaitForSingleObject(p.pinned, 0); err != nil || state != win.WAIT_OBJECT_0 {
		t.Fatal("winner process end not physically confirmed")
	}
}
func writerMatrixRefusedOpen(t *testing.T, c *client, id protocol.UUID) protocol.ErrorView {
	t.Helper()
	pre := registrationPrecondition(t, c, id, "open")
	var refused protocol.ErrorView
	c.request(t, "POST", "/v1/workspaces/"+string(id)+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, &refused, 409)
	if refused.Code != protocol.ErrorCodeWRITERBUSY {
		t.Fatal("held writer did not produce the precise native writer-busy refusal", refused.Code)
	}
	var current protocol.WorkspaceView
	c.request(t, "GET", "/v1/workspaces/"+string(id), nil, &current, 200)
	if current.WorkspaceId != id || current.OpenEpoch != nil {
		t.Fatal("failed native exclusion published an epoch")
	}
	return refused
}
func writerMatrixReceipt(t *testing.T, saved writerReply, in writerInput) protocol.Receipt {
	t.Helper()
	r := saved.Receipt
	if r == nil || r.WorkspaceID != in.Scope.WorkspaceID || r.PrincipalID != in.PrincipalID || r.OperationID != in.OperationID || r.CandidateID != in.CandidateID || r.ResourceID != in.ResourceID || r.NewVersion != 1 || r.NewRevision != 2 {
		t.Fatal("winner receipt is not the exact original formal request")
	}
	return protocol.Receipt{WorkspaceId: protocol.UUID(r.WorkspaceID), OperationId: protocol.UUID(r.OperationID), CandidateId: protocol.UUID(r.CandidateID), ResourceId: protocol.UUID(r.ResourceID), NewVersion: "1", NewRevision: "2", ContentDigest: protocol.Digest(r.ContentDigest)}
}

// Three independent OS-process scenarios, ten fresh real workspaces each.
// The competitor executes the original native adapter + fixed workspace SQLite
// PutCandidate/Admit/CommitAccepted. There is no second production writer API.
// All forced terminations target the already-pinned known-idle Host, NOT an
// unpinned helper, and do not represent transaction crashes or hardware loss.
func TestV012IndependentWriterContentionAndRecovery(t *testing.T) {
	for _, mode := range []string{"continuous", "simultaneous", "killed"} {
		t.Run(mode, func(t *testing.T) {
			for round := 0; round < 10; round++ {
				t.Run(fmt.Sprintf("%02d", round), func(t *testing.T) {
					f := newFixture(t)
					f.initialize(t)
					server := f.start(t)
					c := server.connect(t)
					var principal protocol.PrincipalView
					c.request(t, "GET", "/v1/context", nil, &principal, 200)
					root, created := newInspectionWorkspace(t, c, "V012 formal writer")
					opened := c.open(t, created.WorkspaceId)
					candidate := savePreview(t, c, opened, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "confirmed baseline", Body: protocol.Body(fmt.Sprintf("%s round %d durable original", mode, round))})
					baselineInput := saveInput(newID(t), opened, candidate)
					var baseline protocol.Receipt
					prefix := "/v1/workspaces/" + string(opened.WorkspaceId)
					c.request(t, "POST", prefix+"/record-commits", baselineInput, &baseline, 200)
					if baseline.OperationId != baselineInput.OperationId {
						t.Fatal("baseline receipt lost original submitted key")
					}
					baselineRows := saveFacts(t, c, opened.WorkspaceId, "1", 1)
					saveMatches(t, baselineRows[0], baseline, candidate)
					out := writerMatrixResult{Mode: mode, Round: round, Baseline: baseline, HostPID: server.cmd.Process.Pid, HostBirth: server.birth}
					a := writerMatrixInput(t, f, root, opened, principal)
					b := writerMatrixInput(t, f, root, opened, principal)
					var winner *writerProcess
					if mode == "simultaneous" {
						closePublic(t, c, opened)
						a.StartBarrier, b.StartBarrier = true, true
						first, second := startWriter(t, a), startWriter(t, b)
						for _, p := range []*writerProcess{first, second} {
							ready := p.next(t)
							if ready.Phase != "ready" || ready.Busy || ready.SQLiteAttempt || ready.Receipt != nil {
								t.Fatal("common barrier is not before native/SQLite admission")
							}
							out.ContenderIdentities = append(out.ContenderIdentities, writerMatrixPin(t, p))
						}
						if out.ContenderIdentities[0].PID == out.ContenderIdentities[1].PID && out.ContenderIdentities[0].Birth == out.ContenderIdentities[1].Birth {
							t.Fatal("two contenders are one kernel process")
						}
						goSignal := make(chan error, 2)
						for _, p := range []*writerProcess{first, second} {
							go func(p *writerProcess) { _, err := p.stdin.Write([]byte{2}); goSignal <- err }(p)
						}
						for i := 0; i < 2; i++ {
							if <-goSignal != nil {
								t.Fatal("common start signal failed")
							}
						}
						one, two := first.next(t), second.next(t)
						switch {
						case one.Phase == "saved" && two.Phase == "busy":
							winner = first
							out.WinnerInput, out.LoserInput = a, b
							out.Winner, out.Loser = one, two
							writerMatrixBusy(t, second, two)
						case two.Phase == "saved" && one.Phase == "busy":
							winner = second
							out.WinnerInput, out.LoserInput = b, a
							out.Winner, out.Loser = two, one
							writerMatrixBusy(t, first, one)
						default:
							t.Fatal("common-start contenders did not have exactly one actual SQLite winner")
						}
					} else {
						loser := startWriter(t, b)
						out.LoserInput = b
						out.Loser = loser.next(t)
						writerMatrixBusy(t, loser, out.Loser)
						if mode == "killed" {
							// No request/admitted transaction remains active: the baseline has been
							// confirmed and read. Existing guarded kill pins Host birth/image + Wait.
							server.terminateKnownIdle(t)
							out.HostWasKilled = true
						} else {
							closePublic(t, c, opened)
						}
						winner = startWriter(t, a)
						out.WinnerInput = a
						out.Winner = winner.next(t)
					}
					out.WinnerIdentity = writerMatrixPin(t, winner)
					_ = writerMatrixReceipt(t, out.Winner, out.WinnerInput)
					if mode == "killed" {
						server = f.start(t)
						c = server.connect(t)
						var after protocol.PrincipalView
						c.request(t, "GET", "/v1/context", nil, &after, 200)
						if after.InstallationId != principal.InstallationId || after.TenantId != principal.TenantId || after.PrincipalId != principal.PrincipalId {
							t.Fatal("kill/restart changed current durable identity")
						}
					}
					// A different full Host is not competing for host.lock: this original/new
					// Host is live while the child exclusively holds the actual workspace.
					out.RefusedOpen = writerMatrixRefusedOpen(t, c, opened.WorkspaceId)
					writerMatrixEnd(t, winner, out.Winner)
					recovered := c.open(t, opened.WorkspaceId)
					if recovered.WorkspaceId != opened.WorkspaceId || recovered.OpenEpoch == nil || *recovered.OpenEpoch == *opened.OpenEpoch {
						t.Fatal("reacquired workspace did not establish a new validated generation")
					}
					rows := saveFacts(t, c, recovered.WorkspaceId, "2", 2)
					helperReceipt := writerMatrixReceipt(t, out.Winner, out.WinnerInput)
					seenBase, seenWinner := false, false
					for _, row := range rows {
						switch row.ResourceId {
						case baseline.ResourceId:
							saveMatches(t, row, baseline, candidate)
							if row != baselineRows[0] {
								t.Fatal("confirmed baseline changed during exclusion/recovery")
							}
							seenBase = true
						case helperReceipt.ResourceId:
							if row.WorkspaceId != helperReceipt.WorkspaceId || row.OperationId != helperReceipt.OperationId || row.Version != "1" || row.WorkspaceRevision != "2" || row.ContentDigest != helperReceipt.ContentDigest || row.Title != "synthetic external competitor" || row.Body != "fixed real SQLite competitor body" {
								t.Fatal("cold Host did not read the real child formal commit")
							}
							seenWinner = true
						default:
							t.Fatal("unexpected formal resource from losing contender")
						}
					}
					if !seenBase || !seenWinner {
						t.Fatal("confirmed data lost after releasing/killing owner")
					}
					for _, receipt := range []protocol.Receipt{baseline, helperReceipt} {
						var op protocol.OperationView
						c.request(t, "GET", prefix+"/operations/"+string(receipt.OperationId), nil, &op, 200)
						if op.OperationId != receipt.OperationId || op.Scope != "workspace" || op.WorkspaceId == nil || *op.WorkspaceId != opened.WorkspaceId || op.Kind != "save" || op.LogicalState != "committed" || op.PersistenceState != "known" || op.PhysicalState != "drained" || op.Receipt == nil || *op.Receipt != receipt {
							t.Fatal("same confirmed receipt did not survive owner transition")
						}
					}
					c.request(t, "GET", prefix+"/operations/"+out.LoserInput.OperationID, nil, &out.MissingLoser, 409)
					if out.MissingLoser.Code != protocol.ErrorCodeOPERATIONHISTORYUNAVAILABLE || out.MissingLoser.OperationId == nil || string(*out.MissingLoser.OperationId) != out.LoserInput.OperationID {
						t.Fatal("loser unexpectedly published a durable operation")
					}
					var missing protocol.ErrorView
					c.request(t, "GET", prefix+"/records/"+out.LoserInput.ResourceID, nil, &missing, 404)
					if missing.Code != protocol.ErrorCodeNOTFOUND {
						t.Fatal("loser produced a formal resource")
					}
					out.RecoveredRows = rows
					closePublic(t, c, recovered)
					server.stop(t)
					out.FinalHostEnded = true
					matrixEvidence(t, f, "writer-contention-matrix", out)
				})
			}
		})
	}
}
