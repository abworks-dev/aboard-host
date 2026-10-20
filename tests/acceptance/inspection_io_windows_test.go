//go:build windows && amd64

package acceptance

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

type observedProcessIO struct{ Reads, Writes, Other, ReadBytes, WriteBytes, OtherBytes uint64 }
type observedIOControl struct {
	Bytes, Calls, Transferred uint64
	Valid                     bool
}
type observedInspectionIO struct {
	Version, PID                                                 int
	ID, RootDigest                                               string
	Started, Ended                                               time.Time
	Before, After                                                observedProcessIO
	ScratchCalls, ScratchBytes                                   uint64
	Controls                                                     []observedIOControl
	UntrackedControlDetected                                     bool
	QuiescentStart, QuiescentEnd, Complete, Available, Conserved bool
	Code                                                         string
	Processes                                                    *observedInspectionProcesses
	HTTP                                                         *observedInspectionHTTP
}

func instrumentedInspectionFixture(t *testing.T, f *fixture, root string) *fixture {
	t.Helper()
	image := os.Getenv(stageExecutableEnvironment)
	if image == "" {
		t.Skip("instrumented acceptance binary not configured")
	}
	hash, err := checkedExecutable(image)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(struct {
		Version       int `json:"version"`
		Fixture, Root string
	}{1, f.root, root})
	copy := *f
	copy.executable = image
	copy.sha = hash
	copy.environment = []string{"ABOARD_ACCEPTANCE_INSPECTION_IO=" + base64.RawURLEncoding.EncodeToString(raw), "ABOARD_ACCEPTANCE_INSPECTION_PROCESS=1"}
	return &copy
}
func inspectionObservationIDs(t *testing.T, f *fixture) map[string]bool {
	t.Helper()
	paths, e := filepath.Glob(filepath.Join(f.root, "inspection-observation-*.ready"))
	if e != nil {
		t.Fatal(e)
	}
	ids := map[string]bool{}
	for _, p := range paths {
		ids[filepath.Base(p)] = true
	}
	return ids
}
func newInspectionObservation(t *testing.T, f *fixture, old map[string]bool, p *process, root string) observedInspectionIO {
	t.Helper()
	now := inspectionObservationIDs(t, f)
	newPaths := []string{}
	for path := range now {
		if !old[path] {
			newPaths = append(newPaths, path)
		}
	}
	if len(newPaths) != 1 {
		t.Fatal("missing/ambiguous per-request inspection observation", len(newPaths))
	}
	ready := filepath.Join(f.root, newPaths[0])
	info, e := os.Lstat(ready)
	if e != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		t.Fatal("invalid observation ready")
	}
	path := strings.TrimSuffix(ready, ".ready") + ".json"
	info, e = os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() {
		t.Fatal("invalid observation file")
	}
	file, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, 16385))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(raw) > 16384 {
		t.Fatal("observation not bounded")
	}
	if _, e = protocol.ParseJSON(raw); e != nil {
		t.Fatal("observation not strict JSON")
	}
	var out observedInspectionIO
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("invalid observation fields")
	}
	hash := sha256.Sum256([]byte(root))
	if out.Version != 1 || out.PID != p.cmd.Process.Pid || out.RootDigest != hex.EncodeToString(hash[:]) || len(out.ID) != 32 || filepath.Base(ready) != "inspection-observation-"+out.ID+".ready" || out.Started.IsZero() || out.Ended.Before(out.Started) {
		t.Fatal("request/process observation identity mismatch")
	}
	if !out.Available || !out.Complete || !out.QuiescentStart || !out.QuiescentEnd || !out.Conserved || !out.UntrackedControlDetected {
		t.Fatalf("inspection observation unavailable/nonconserving: %+v", out)
	}
	for i, size := range []uint64{1, 65535, 65536} {
		if len(out.Controls) != 3 || !out.Controls[i].Valid || out.Controls[i].Bytes != size || out.Controls[i].Calls != 1 || out.Controls[i].Transferred != size {
			t.Fatal("genuine same-byte control not detected")
		}
	}
	if out.After.Writes < out.Before.Writes || out.After.WriteBytes < out.Before.WriteBytes || out.After.Writes-out.Before.Writes != out.ScratchCalls || out.After.WriteBytes-out.Before.WriteBytes != out.ScratchBytes {
		t.Fatal("unexpected untracked kernel write; never subtract noise")
	}
	return out
}

func TestV009IndependentInspectorKernelIO(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	first := f.start(t)
	c := first.connect(t)
	root, a := newInspectionWorkspace(t, c, "observed-source")
	_, b := newInspectionWorkspace(t, c, "observed-independent-B")
	opened := c.open(t, a.WorkspaceId)
	prefix := "/v1/workspaces/" + string(a.WorkspaceId)
	var originalCandidate protocol.CandidateView
	c.request(t, "POST", prefix+"/record-previews", protocol.PreviewRecord{OpenEpoch: *opened.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "formal attribution specimen", Body: "immutable body before each bad specimen"}, &originalCandidate, 201)
	var originalReceipt protocol.Receipt
	c.request(t, "POST", prefix+"/record-commits", protocol.CommitRecord{OperationId: newID(t), OpenEpoch: *opened.OpenEpoch, CandidateId: originalCandidate.CandidateId, CandidateDigest: originalCandidate.CandidateDigest, PreviewVersion: originalCandidate.PreviewVersion}, &originalReceipt, 200)
	first.stop(t)
	original := saveInspectionBytes(t, root)
	baseline := tree(t, root)
	variants := append([]badDirectoryCase{{Name: "healthy"}}, badDirectoryCases()...)
	for stage := 0; stage <= 6; stage++ {
		variants = append(variants, badDirectoryCase{Name: "initialization-" + strconv.Itoa(stage), Want: "RECOVERY_REQUIRED"})
	}
	reuseSafe := true
	bRevision := 0
	for _, row := range variants {
		variant := row.Name
		t.Run(variant, func(t *testing.T) {
			reuseSafe = false
			var processes *inspectionProcessWatch
			var outgoing *inspectionHTTPWatch
			watchJoined := false
			defer func() {
				if processes != nil && processes.drained && watchJoined && outgoing != nil && outgoing.joined {
					restoreInspectionBytes(t, root, original)
					reuseSafe = true
				}
			}()

			if strings.HasPrefix(variant, "initialization-") {
				stage, e := strconv.Atoi(strings.TrimPrefix(variant, "initialization-"))
				if e != nil {
					t.Fatal(e)
				}
				interruptExternalInitialization(t, f, root, "workspace", stage)
				if _, e = os.Stat(filepath.Join(root, "data", "workspace.sqlite-journal")); os.IsNotExist(e) {
					row.Want = "CORRUPT_DATABASE"
				} else if e != nil {
					t.Fatal(e)
				}
			} else {
				applyBadDirectoryCase(t, f, root, row)
			}
			before := tree(t, root)
			measured := instrumentedInspectionFixture(t, f, root)
			outgoing = startInspectionHTTPWatch(t, f, root, measured)
			defer outgoing.close(t)
			p := measured.start(t)
			d := p.connect(t)
			processes = startInspectionProcessWatch(t, p)
			defer processes.cleanup(t, p) // immediately, even IDs/watcher setup can fail
			prior := inspectionObservationIDs(t, f)
			watch := startInspectionWatch(t, root)
			watchAttempted := false
			finishWatch := func() { finishInspectionOnce(&watchAttempted, &watchJoined, func() { watch.finish(t, false) }) }
			defer finishWatch()           // only first normal return proves join; failed done-guard cannot promote
			defer processes.cleanup(t, p) // all descendants end before watch/fixture teardown
			wantStatus := 409
			wantCode := string(row.Want)
			if row.Want == "" {
				wantStatus = 200
			}
			if row.Want == protocol.ErrorCodeFORBIDDEN {
				wantStatus = 403
				wantCode = "request_refused"
			}
			var reply protocol.InspectionView
			var failure protocol.ErrorView
			if wantStatus == 200 {
				d.request(t, "POST", "/v1/workspace-inspections", protocol.InspectWorkspace{Path: protocol.Path(root)}, &reply, wantStatus)
			} else {
				d.request(t, "POST", "/v1/workspace-inspections", protocol.InspectWorkspace{Path: protocol.Path(root)}, &failure, wantStatus)
			}
			if wantStatus != 200 && failure.Code != row.Want {
				t.Fatalf("wrong public inspect classification: %s got=%s want=%s", variant, failure.Code, row.Want)
			}
			if wantStatus == 200 && (reply.WorkspaceId != a.WorkspaceId || reply.Integrity != "valid" || reply.AccessMode != "none") {
				t.Fatal("inspect invented writable access/business readiness")
			}
			observation := newInspectionObservation(t, f, prior, p, root)
			processes.verify(t, p, observation.Processes)
			outgoing.verify(t, observation)
			if observation.Code != wantCode {
				t.Fatal("observer did not include final error/scope projection", observation.Code, wantCode)
			}
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("measured inspect rewrote source")
			}
			if variant != "healthy" {
				pre := registrationPrecondition(t, d, a.WorkspaceId, "open")
				var refused protocol.ErrorView
				status := d.request(t, "POST", "/v1/workspaces/"+string(a.WorkspaceId)+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, &refused, 0)
				if status != 409 || refused.Code != badDirectoryOpenCode(row) {
					t.Fatalf("attributed specimen open not refused: %s status=%d code=%s", variant, status, refused.Code)
				}
				if !reflect.DeepEqual(before, tree(t, root)) {
					t.Fatal("open repaired measured specimen")
				}
			}
			finishWatch()
			if variant == "healthy" {
				if observation.ScratchCalls == 0 || observation.ScratchBytes == 0 || reply.WorkspaceId != a.WorkspaceId {
					t.Fatal("healthy source did not execute actual private copy")
				}
				opened := d.open(t, a.WorkspaceId)
				closePublic(t, d, opened)
			}
			other := d.open(t, b.WorkspaceId)
			prefix := "/v1/workspaces/" + string(b.WorkspaceId)
			var preview protocol.CandidateView
			d.request(t, "POST", prefix+"/record-previews", protocol.PreviewRecord{OpenEpoch: *other.OpenEpoch, Kind: "create-text", BaseRevision: *other.Revision, Title: "after measured A", Body: protocol.Body(variant)}, &preview, 201)
			var receipt protocol.Receipt
			d.request(t, "POST", prefix+"/record-commits", protocol.CommitRecord{OperationId: newID(t), OpenEpoch: *other.OpenEpoch, CandidateId: preview.CandidateId, CandidateDigest: preview.CandidateDigest, PreviewVersion: preview.PreviewVersion}, &receipt, 200)
			var body protocol.TextRecordView
			d.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil, &body, 200)
			if body.Body != protocol.Body(variant) || receipt.NewRevision != protocol.PositiveRevision(strconv.Itoa(bRevision+1)) || body.OperationId != receipt.OperationId {
				t.Fatal("B did not genuinely save after A observation")
			}
			bRevision++
			// Ready visibility alone cannot prove diagnostic Close. Actual graceful
			// shutdown joins the probe's SAME Store owner/quota and rejects sticky close.
			p.stop(t)
			processes.stopped(t)
			outgoing.verify(t, observation)
			outgoing.close(t)
			if !outgoing.joined {
				t.Fatal("HTTP observer native end not proven; fixture retained")
			}
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("Host shutdown changed measured source")
			}
			matrixEvidence(t, f, "kernel-io-"+variant, observation)
		})
		if !reuseSafe {
			t.Fatal("native process end not proved; fixture preserved, stop variant reuse")
		}
	}
	if !reflect.DeepEqual(baseline, tree(t, root)) {
		t.Fatal("observation fixture restoration not exact")
	}
	// Restore is test-owned only. A NEW normal production process must still read
	// exactly the receipt-backed immutable body; no measured-process cache reuse.
	last := f.start(t)
	positive := last.connect(t)
	restored := positive.open(t, a.WorkspaceId)
	var record protocol.TextRecordView
	positive.request(t, "GET", prefix+"/records/"+string(originalReceipt.ResourceId), nil, &record, 200)
	if restored.Revision == nil || *restored.Revision != "1" || record.OperationId != originalReceipt.OperationId || record.Body != "immutable body before each bad specimen" {
		t.Fatal("restoration lost original formal facts")
	}
	last.stop(t)
}
