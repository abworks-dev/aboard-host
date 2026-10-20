//go:build windows && amd64

package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

type observedSaveCheckpoint struct {
	Version                                                                                                             int `json:"version"`
	PID                                                                                                                 int `json:"pid"`
	InstallationID, TenantID, WorkspaceID, PrincipalID, OperationID, CandidateID, CandidateDigest, Epoch, Phase, Status string
	Receipt                                                                                                             *protocol.Receipt   `json:",omitempty"`
	Native                                                                                                              *observedSaveNative `json:",omitempty"`
}

type observedSaveNative struct {
	JournalCalls, JournalSuccesses, DatabaseCalls, NativeBOOL, CommitCalls int
	InFlight, CommitReturned, CallbackDrained, PhysicalEnded               bool
}

func checkpointSavePhase(phase string) bool {
	return phase == "before-admit" || phase == "after-admit" || phase == "after-confirm" || phase == "formal-inflight"
}
func validSaveNativeMarker(raw []byte, out observedSaveCheckpoint) bool {
	value, e := protocol.ParseJSON(raw)
	if e != nil {
		return false
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 13 {
		return false
	}
	for _, key := range []string{"version", "pid", "InstallationID", "TenantID", "WorkspaceID", "PrincipalID", "OperationID", "CandidateID", "CandidateDigest", "Epoch", "Phase", "Status", "Native"} {
		if _, ok := fields[key]; !ok {
			return false
		}
	}
	for _, key := range []string{"InstallationID", "TenantID", "WorkspaceID", "PrincipalID", "OperationID", "CandidateID", "CandidateDigest", "Epoch", "Phase", "Status"} {
		s, ok := fields[key].(string)
		if !ok || s == "" {
			return false
		}
	}
	for _, key := range []string{"version", "pid"} {
		if _, ok := fields[key].(json.Number); !ok {
			return false
		}
	}
	encoded, e := json.Marshal(fields["CandidateDigest"])
	var digest protocol.Digest
	if e != nil || digest.UnmarshalJSON(encoded) != nil {
		return false
	}
	native, ok := fields["Native"].(map[string]any)
	if !ok || len(native) != 9 {
		return false
	}
	for _, key := range []string{"JournalCalls", "JournalSuccesses", "DatabaseCalls", "NativeBOOL", "CommitCalls"} {
		if _, ok := native[key].(json.Number); !ok {
			return false
		}
	}
	for _, key := range []string{"InFlight", "CommitReturned", "CallbackDrained", "PhysicalEnded"} {
		if _, ok := native[key].(bool); !ok {
			return false
		}
	}
	n := out.Native
	return out.Status == "accepted" && out.Receipt == nil && n != nil && n.JournalCalls == 2 && n.JournalSuccesses == 2 && n.DatabaseCalls == 0 && n.NativeBOOL != 0 && n.CommitCalls == 1 && n.InFlight && !n.CommitReturned && !n.CallbackDrained && !n.PhysicalEnded
}
func readSaveCheckpoint(root string) (observedSaveCheckpoint, error) {
	var out observedSaveCheckpoint
	ready, e := os.Lstat(filepath.Join(root, "save-checkpoint.ready"))
	if e != nil {
		return out, e
	}
	if !ready.Mode().IsRegular() || ready.Size() != 0 {
		return out, errors.New("invalid save readiness")
	}
	path := filepath.Join(root, "save-checkpoint.json")
	stat, e := os.Lstat(path)
	if e != nil || !stat.Mode().IsRegular() || stat.Size() > 4096 {
		return out, errors.New("invalid save marker file")
	}
	f, e := os.Open(path)
	if e != nil {
		return out, e
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, 4097))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || len(raw) > 4096 || json.Unmarshal(raw, &out) != nil || out.Version != 1 || !checkpointSavePhase(out.Phase) {
		return out, errors.New("invalid save marker")
	}
	if out.Phase == "formal-inflight" && !validSaveNativeMarker(raw, out) {
		return out, errors.New("invalid native midpoint evidence")
	}
	return out, nil
}
func saveCheckpointFaultAllowed(in faultInput) bool {
	phase := strings.TrimPrefix(in.Phase, "save-checkpoint-")
	if phase == in.Phase || !checkpointSavePhase(phase) || in.OperationID == "" {
		return false
	}
	m, e := readSaveCheckpoint(in.Fixture)
	if e != nil || m.PID != in.PID || m.Phase != phase || m.OperationID != in.OperationID {
		return false
	}
	for _, id := range []string{m.InstallationID, m.TenantID, m.WorkspaceID, m.PrincipalID, m.OperationID, m.CandidateID, m.Epoch} {
		raw, _ := json.Marshal(id)
		if json.Unmarshal(raw, new(protocol.UUID)) != nil {
			return false
		}
	}
	if phase == "before-admit" {
		return m.Status == "not-admitted" && m.Receipt == nil
	}
	if phase == "after-admit" || phase == "formal-inflight" {
		return m.Status == "accepted" && m.Receipt == nil
	}
	return m.Status == "confirmed" && m.Receipt != nil && string(m.Receipt.OperationId) == in.OperationID && string(m.Receipt.WorkspaceId) == m.WorkspaceID && string(m.Receipt.CandidateId) == m.CandidateID
}

// A single raw complete command, no retrying http.Transport and no response read.
// The bearer exists only in memory and on the authenticated local wire.
func beginSaveWire(t *testing.T, c *client, in protocol.CommitRecord, workspace protocol.UUID) *net.TCPConn {
	t.Helper()
	raw, e := json.Marshal(in)
	if e != nil {
		t.Fatal(e)
	}
	conn, e := net.DialTimeout("tcp", c.authority, 3*time.Second)
	if e != nil {
		t.Fatal("save wire connection failed")
	}
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		t.Fatal("save wire not TCP")
	}
	t.Cleanup(func() { _ = tcp.Close() })
	if tcp.SetDeadline(time.Now().Add(15*time.Second)) != nil {
		t.Fatal("save wire deadline failed")
	}
	_, e = fmt.Fprintf(tcp, "POST /v1/workspaces/%s/record-commits HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", workspace, c.authority, c.token, len(raw))
	if e != nil {
		t.Fatal("save wire headers failed (withheld)")
	}
	if n, e := tcp.Write(raw); e != nil || n != len(raw) {
		t.Fatal("save wire body failed (withheld)")
	}
	return tcp
}
func waitSaveCheckpoint(t *testing.T, p *process, in protocol.CommitRecord, a protocol.WorkspaceView, actor protocol.PrincipalView, phase string) observedSaveCheckpoint {
	t.Helper()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		m, e := readSaveCheckpoint(p.fixture.root)
		if e == nil {
			if m.PID != p.cmd.Process.Pid || m.Phase != phase || m.InstallationID != string(actor.InstallationId) || m.TenantID != string(actor.TenantId) || m.PrincipalID != string(actor.PrincipalId) || m.WorkspaceID != string(a.WorkspaceId) || m.OperationID != string(in.OperationId) || m.CandidateID != string(in.CandidateId) || m.CandidateDigest != string(in.CandidateDigest) || m.Epoch != string(in.OpenEpoch) || !saveCheckpointFaultAllowed(faultInput{Fixture: p.fixture.root, PID: p.cmd.Process.Pid, OperationID: string(in.OperationId), Phase: "save-checkpoint-" + phase}) {
				t.Fatal("checkpoint lost original identity or boundary")
			}
			return m
		}
		if !errors.Is(e, os.ErrNotExist) {
			t.Fatal("invalid save checkpoint")
		}
		select {
		case <-p.done:
			t.Fatal("Host ended before save checkpoint")
		case <-deadline.C:
			t.Fatal("save checkpoint not reached")
		case <-tick.C:
		}
	}
}
func releaseSaveCheckpoint(t *testing.T, f *fixture) {
	t.Helper()
	if requireSyntheticRoot(f.root) != nil {
		t.Fatal("release fixture invalid")
	}
	file, e := os.OpenFile(filepath.Join(f.root, "save-checkpoint.release"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal("release marker creation failed")
	}
	if file.Close() != nil {
		t.Fatal("release marker close failed")
	}
}
func saveCommitCorrelations(t *testing.T, p *process) map[string]bool {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		p.mu.Lock()
		out := make(map[string]bool)
		bad := false
		for _, e := range p.evidence.Events {
			if e.Event == "http" && e.HTTP != nil && e.HTTP.Operation == "commitRecord" {
				out[e.HTTP.CorrelationID] = e.HTTP.Status == 200 && e.HTTP.Code == ""
				if e.HTTP.Code != "" {
					bad = true
				}
			}
		}
		p.mu.Unlock()
		if bad || len(out) > 1 {
			t.Fatal("pre-RST commit evidence not unique successful B")
		}
		for correlation, success := range out {
			if correlation != "" && success {
				return out
			}
		}
		select {
		case <-p.done:
			t.Fatal("Host ended before B event join")
		case <-deadline.C:
			t.Fatal("B stdout event not joined before RST baseline")
		case <-tick.C:
		}
	}
}
func waitSaveDeliveryFailure(t *testing.T, p *process, prior map[string]bool) string {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		p.mu.Lock()
		type observed struct{ success, writeFailed, flushFailed bool }
		fresh := make(map[string]observed)
		for _, e := range p.evidence.Events {
			if e.Event != "http" || e.HTTP == nil || e.HTTP.Operation != "commitRecord" || prior[e.HTTP.CorrelationID] {
				continue
			}
			x := fresh[e.HTTP.CorrelationID]
			if e.HTTP.Status == 200 && e.HTTP.Code == "" {
				x.success = true
			}
			if e.HTTP.Code == protocol.ErrorCodeVIEWDELIVERYFAILED {
				if e.HTTP.Status == 200 {
					x.writeFailed = true
				}
				if e.HTTP.Status == 0 {
					x.flushFailed = true
				}
			}
			fresh[e.HTTP.CorrelationID] = x
		}
		p.mu.Unlock()
		if len(fresh) > 1 {
			t.Fatal("delivery evidence cannot identify original request")
		}
		for correlation, x := range fresh {
			if correlation != "" && (x.writeFailed || (x.success && x.flushFailed)) {
				return correlation
			}
		}
		select {
		case <-p.done:
			t.Fatal("Host ended before actual delivery failure")
		case <-deadline.C:
			t.Fatal("RST alone does not prove actual response Write/Flush failure")
		case <-tick.C:
		}
	}
}

func TestV010IndependentSaveCheckpointAndLostDelivery(t *testing.T) {
	cases := []struct {
		phase      string
		disconnect bool
	}{{"before-admit", false}, {"after-admit", false}, {"after-confirm", false}, {"after-admit", true}, {"after-confirm", true}, {"formal-inflight", false}, {"formal-inflight", true}}
	for _, tc := range cases {
		name := tc.phase + "-process-end"
		if tc.disconnect {
			name = tc.phase + "-disconnect-release"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			setup := f.start(t)
			initial := setup.connect(t)
			var actor protocol.PrincipalView
			initial.request(t, "GET", "/v1/context", nil, &actor, 200)
			root, a := newInspectionWorkspace(t, initial, "save-target")
			_, b := newInspectionWorkspace(t, initial, "independent-B")
			setup.stop(t)
			key := newID(t)
			staged := saveNativeFixture(t, f, key, a.WorkspaceId, actor.PrincipalId, tc.phase, 0)
			p := staged.start(t)
			c := p.connect(t)
			joinOriginalSessionDelivery(t, c)
			a = c.open(t, a.WorkspaceId)
			b = c.open(t, b.WorkspaceId)
			prefix := "/v1/workspaces/" + string(a.WorkspaceId)
			candidate := savePreview(t, c, a, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "checkpoint original", Body: "single original save; lost delivery is not rollback"})
			in := saveInput(key, a, candidate)
			saveFacts(t, c, a.WorkspaceId, "0", 0)
			wire := beginSaveWire(t, c, in, a.WorkspaceId)
			marker := waitSaveCheckpoint(t, p, in, a, actor, tc.phase)
			lease, e := platform.Acquire(root, platform.Workspace, false, false)
			if e == nil {
				_ = lease.Close()
				t.Fatal("held original released native writer exclusion")
			}
			bCandidate := savePreview(t, c, b, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "B during A held", Body: "independent B actually confirms while A is held"})
			var bReceipt protocol.Receipt
			c.request(t, "POST", "/v1/workspaces/"+string(b.WorkspaceId)+"/record-commits", saveInput(newID(t), b, bCandidate), &bReceipt, 200)
			saveMatches(t, saveFacts(t, c, b.WorkspaceId, "1", 1)[0], bReceipt, bCandidate)
			var live, original, resolved protocol.OperationView
			var deliveryCorrelation string
			if tc.disconnect {
				prior := saveCommitCorrelations(t, p) // B completed; the only forthcoming POST is the held original
				if wire.SetLinger(0) != nil || wire.Close() != nil {
					t.Fatal("explicit RST failed")
				}
				releaseSaveCheckpoint(t, f)
				c.request(t, "GET", prefix+"/operations/"+string(key), nil, &live, 200)
				if live.LogicalState != "committed" || live.PersistenceState != "known" || live.Receipt == nil || live.OperationId != key || live.WorkspaceId == nil || *live.WorkspaceId != a.WorkspaceId {
					t.Fatal("disconnect canceled accepted business or lost original receipt")
				}
				if marker.Receipt != nil && *marker.Receipt != *live.Receipt {
					t.Fatal("lost ACK rewrote original confirmed receipt")
				}
				deliveryCorrelation = waitSaveDeliveryFailure(t, p, prior)
				saveMatches(t, saveFacts(t, c, a.WorkspaceId, "1", 1)[0], *live.Receipt, candidate)
				var same protocol.Receipt
				c.request(t, "POST", prefix+"/record-commits", in, &same, 200)
				if same != *live.Receipt {
					t.Fatal("same key reexecuted after disconnected waiter")
				}
				closePublic(t, c, a)
				closePublic(t, c, b)
				p.stop(t)
			} else {
				p.terminateAt(t, "save-checkpoint-"+tc.phase, string(key)) // actual pinned process end, NOT safe drain/power loss
				buf := make([]byte, 1)
				n, readErr := wire.Read(buf)
				if n != 0 || readErr == nil {
					t.Fatal("held checkpoint delivered response bytes")
				}
				var timeout net.Error
				if errors.As(readErr, &timeout) && timeout.Timeout() {
					t.Fatal("wire waiter did not actually end")
				}
				if wire.Close() != nil {
					t.Fatal("ended wire close failed")
				}
			}
			lease, e = platform.Acquire(root, platform.Workspace, false, false)
			if e != nil {
				t.Fatal("ended owner still excludes actual writer")
			}
			if lease.Close() != nil {
				t.Fatal("post-end native exclusion probe close failed")
			}
			next := f.start(t)
			d := next.connect(t)
			var current protocol.PrincipalView
			d.request(t, "GET", "/v1/context", nil, &current, 200)
			if current.InstallationId != actor.InstallationId || current.TenantId != actor.TenantId || current.PrincipalId != actor.PrincipalId {
				t.Fatal("fresh authentication changed original scope")
			}
			pre := registrationPrecondition(t, d, a.WorkspaceId, "open")
			var raw json.RawMessage
			openStatus := d.request(t, "POST", prefix+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, &raw, 0)
			if openStatus != 200 && openStatus != 503 {
				t.Fatal("fresh open missing trustworthy history")
			}
			var opened protocol.WorkspaceView
			if openStatus == 200 {
				if json.Unmarshal(raw, &opened) != nil || opened.OpenEpoch == nil || *opened.OpenEpoch == *a.OpenEpoch {
					t.Fatal("fresh process did not publish fresh epoch")
				}
			} else {
				var failure protocol.ErrorView
				if json.Unmarshal(raw, &failure) != nil || failure.Code != protocol.ErrorCodeCOMMITUNKNOWN {
					t.Fatal("accepted cold history mislabeled or unfenced")
				}
			}
			queryStatus := d.request(t, "GET", prefix+"/operations/"+string(key), nil, &raw, 0)
			if tc.phase == "before-admit" {
				var missing protocol.ErrorView
				if queryStatus != 409 || openStatus != 200 || json.Unmarshal(raw, &missing) != nil || missing.Code != protocol.ErrorCodeOPERATIONHISTORYUNAVAILABLE {
					t.Fatal("pre-admit interruption fabricated durable history")
				}
				saveFacts(t, d, a.WorkspaceId, "0", 0) // missing history is NOT a durable rejection or permission to replay
			} else {
				if queryStatus != 200 || json.Unmarshal(raw, &original) != nil || original.Scope != "workspace" || original.OperationId != key || original.WorkspaceId == nil || *original.WorkspaceId != a.WorkspaceId {
					t.Fatal("original durable admission/association lost")
				}
				d.request(t, "POST", prefix+"/operations/"+string(key)+"/reconcile", nil, &resolved, 200)
				if resolved.OperationId != key || resolved.Scope != "workspace" || resolved.WorkspaceId == nil || *resolved.WorkspaceId != a.WorkspaceId || resolved.PersistenceState != "known" {
					t.Fatal("fresh investigation crossed original scope")
				}
				if !tc.disconnect && (tc.phase == "after-admit" || tc.phase == "formal-inflight") {
					if original.Receipt != nil || original.LogicalState == "committed" || resolved.LogicalState != "rejected" || resolved.Receipt != nil {
						t.Fatal("pre-formal interruption reexecuted business")
					}
					if openStatus != 200 {
						opened = d.open(t, a.WorkspaceId)
					}
					if opened.OpenEpoch == nil || *opened.OpenEpoch == *a.OpenEpoch {
						t.Fatal("fresh investigated history reused prior process epoch")
					}
					saveFacts(t, d, a.WorkspaceId, "0", 0)
				} else {
					if original.LogicalState != "committed" || original.Receipt == nil || resolved.LogicalState != "committed" || resolved.Receipt == nil || *original.Receipt != *resolved.Receipt || openStatus != 200 {
						t.Fatal("confirmed/lost receipt truth did not survive real process end")
					}
					if marker.Receipt != nil && *original.Receipt != *marker.Receipt {
						t.Fatal("cold original receipt changed")
					}
					if tc.disconnect && *original.Receipt != *live.Receipt {
						t.Fatal("disconnected original receipt changed on restart")
					}
					saveMatches(t, saveFacts(t, d, a.WorkspaceId, "1", 1)[0], *original.Receipt, candidate)
					var same protocol.Receipt
					d.request(t, "POST", prefix+"/record-commits", in, &same, 200)
					if same != *original.Receipt {
						t.Fatal("old epoch original-key inquiry replayed business")
					}
					saveFacts(t, d, a.WorkspaceId, "1", 1)
				}
			}
			d.open(t, b.WorkspaceId)
			saveMatches(t, saveFacts(t, d, b.WorkspaceId, "1", 1)[0], bReceipt, bCandidate)
			next.stop(t)
			var liveEvidence, originalEvidence, resolvedEvidence *protocol.OperationView
			if tc.disconnect {
				liveEvidence = &live
			}
			if tc.phase != "before-admit" {
				originalEvidence, resolvedEvidence = &original, &resolved
			}
			matrixEvidence(t, f, "save-checkpoint-result", struct {
				Marker                     observedSaveCheckpoint
				Disconnect                 bool
				DeliveryFailureCorrelation string
				ColdOpen, ColdQuery        int
				Live, Original, Resolved   *protocol.OperationView `json:",omitempty"`
				BReceipt                   protocol.Receipt
				Boundary                   string
			}{marker, tc.disconnect, deliveryCorrelation, openStatus, queryStatus, liveEvidence, originalEvidence, resolvedEvidence, bReceipt, "wrapper edges or explicit native second successful journal sync inside original COMMIT/no prior noncommit sync; zero DB sync is NOT a general no-write proof; RST separately requires actual Write/Flush failure; forced death NOT graceful/power loss"})
		})
	}
}
