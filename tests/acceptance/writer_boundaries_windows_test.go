//go:build windows && amd64

package acceptance

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// Mutation targets are always exact, previously observed paths owned by this
// fixture. Do not move a source while a Host or native ancestor pin is live.
func writerBoundaryMove(t *testing.T, f *fixture, source, target string) {
	t.Helper()
	if requireSyntheticRoot(f.root) != nil || !within(f.config.AllowedRoots[0], source) || !within(f.config.AllowedRoots[0], target) || !unreparsed(source) {
		t.Fatal("unowned writer boundary move")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("writer boundary move target exists")
	}
	if err := os.Rename(source, target); err != nil {
		t.Fatal("stopped synthetic move failed", err)
	}
}

func writerBoundaryOriginal(t *testing.T, c *client, opened protocol.WorkspaceView, receipt protocol.Receipt, candidate protocol.CandidateView) {
	t.Helper()
	rows := saveFacts(t, c, opened.WorkspaceId, "1", 1)
	saveMatches(t, rows[0], receipt, candidate)
	var op protocol.OperationView
	c.request(t, "GET", "/v1/workspaces/"+string(opened.WorkspaceId)+"/operations/"+string(receipt.OperationId), nil, &op, 200)
	if op.OperationId != receipt.OperationId || op.Receipt == nil || *op.Receipt != receipt || op.PersistenceState != "known" || op.PhysicalState != "drained" || op.LogicalState != "committed" {
		t.Fatal("original formal receipt changed after path refusal")
	}
}

func TestV012IndependentCopyRelinkAndCanonicalReplacement(t *testing.T) {
	for _, mode := range []string{"copy-relink", "canonical-replacement"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			p := f.start(t)
			c := p.connect(t)
			root, created := newInspectionWorkspace(t, c, "V012 physical binding")
			opened := c.open(t, created.WorkspaceId)
			candidate := savePreview(t, c, opened, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "original physical owner", Body: "confirmed before copy/replacement 合成"})
			input := saveInput(newID(t), opened, candidate)
			var receipt protocol.Receipt
			prefix := "/v1/workspaces/" + string(created.WorkspaceId)
			c.request(t, "POST", prefix+"/record-commits", input, &receipt, 200)
			if receipt.OperationId != input.OperationId {
				t.Fatal("baseline receipt differs from submitted key")
			}
			writerBoundaryOriginal(t, c, opened, receipt, candidate)
			closePublic(t, c, opened)
			version := registrationPrecondition(t, c, created.WorkspaceId, "relink").RegistrationVersion
			p.stop(t)
			before := tree(t, root)
			physical := workspaceRootIdentity(t, root)
			parked := filepath.Join(f.config.AllowedRoots[0], "parked-original")
			writerBoundaryMove(t, f, root, parked)
			copyRoot := filepath.Join(f.config.AllowedRoots[0], "same-uuid-copy")
			if mode == "canonical-replacement" {
				copyRoot = root
			}
			copyStoppedWorkspace(t, f, parked, copyRoot)
			copyPhysical := workspaceRootIdentity(t, copyRoot)
			if physical == copyPhysical || !reflect.DeepEqual(before, tree(t, copyRoot)) || physical != workspaceRootIdentity(t, parked) {
				t.Fatal("copy/replacement control lacks distinct real root with identical bytes")
			}
			q := f.start(t)
			d := q.connect(t)
			key := newID(t)
			var refused protocol.ErrorView
			if mode == "copy-relink" {
				// The old locator is absent and the supplied CAS is current. A refusal
				// cannot be credited to an existing old path or stale registration.
				if _, err := os.Lstat(root); !os.IsNotExist(err) {
					t.Fatal("original locator should be absent before copied relink")
				}
				d.request(t, "POST", prefix+"/relink", protocol.RelinkWorkspace{OperationId: key, Path: protocol.Path(copyRoot), ExpectedRegistrationVersion: version}, &refused, 409)
				if refused.Code != protocol.ErrorCodeIDENTITYCONFLICT {
					t.Fatal("copied relink not refused by physical identity", refused.Code)
				}
				var absent protocol.ErrorView
				d.request(t, "GET", "/v1/installation-operations/"+string(key), nil, &absent, 404)
				if absent.Code != protocol.ErrorCodeNOTFOUND {
					t.Fatal("copied relink admitted a durable installation operation")
				}
			} else {
				pre := registrationPrecondition(t, d, created.WorkspaceId, "open")
				d.request(t, "POST", prefix+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: pre.RegistrationVersion}, &refused, 409)
				// Existing production ErrOpen projection: do not change its contract
				// just to obtain a more specific diagnostic for this negative case.
				if refused.Code != protocol.ErrorCodeRECOVERYREQUIRED {
					t.Fatal("canonical root replacement was not rejected", refused.Code)
				}
			}
			var view protocol.WorkspaceView
			d.request(t, "GET", prefix, nil, &view, 200)
			if view.OpenEpoch != nil || view.WorkspaceId != created.WorkspaceId || registrationPrecondition(t, d, created.WorkspaceId, "relink").RegistrationVersion != version {
				t.Fatal("rejected physical copy changed registration or published a writable epoch")
			}
			var noWrite protocol.ErrorView
			d.request(t, "POST", prefix+"/record-commits", input, &noWrite, 409)
			if noWrite.Code != protocol.ErrorCodeWORKSPACECLOSED {
				t.Fatal("rejected physical copy exposed a save owner", noWrite.Code)
			}
			if !reflect.DeepEqual(before, tree(t, parked)) || !reflect.DeepEqual(before, tree(t, copyRoot)) {
				t.Fatal("physical-copy refusal modified either source")
			}
			q.stop(t)
			if mode == "canonical-replacement" {
				writerBoundaryMove(t, f, copyRoot, filepath.Join(f.config.AllowedRoots[0], "rejected-replacement"))
			}
			writerBoundaryMove(t, f, parked, root)
			if workspaceRootIdentity(t, root) != physical || !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("stopped restoration changed original native identity or bytes")
			}
			r := f.start(t)
			e := r.connect(t)
			reopened := e.open(t, created.WorkspaceId)
			if reopened.OpenEpoch == nil || *reopened.OpenEpoch == *opened.OpenEpoch {
				t.Fatal("original owner did not regain a fresh epoch")
			}
			writerBoundaryOriginal(t, e, reopened, receipt, candidate)
			closePublic(t, e, reopened)
			r.stop(t)
			matrixEvidence(t, f, "writer-physical-boundary", struct {
				Mode                            string
				Original, Copy                  platform.Identity
				Refused, NoWrite                protocol.ErrorView
				Receipt                         protocol.Receipt
				Source                          map[string]string
				OriginalReopened, AllHostsEnded bool
			}{mode, physical, copyPhysical, refused, noWrite, receipt, before, true, true})
		})
	}
}

// Lock bytes are never authority. Preload the EXISTING fixed object while the
// known original Host is stopped; a live-looking PID must not block real acquire,
// and a fake unlocked marker must not let a competitor bypass the native lease.
func TestV012IndependentForgedLockContents(t *testing.T) {
	for _, mode := range []string{"live-pid", "fake-unlocked", "nonexistent-pid-text"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			p := f.start(t)
			c := p.connect(t)
			var principal protocol.PrincipalView
			c.request(t, "GET", "/v1/context", nil, &principal, 200)
			root, created := newInspectionWorkspace(t, c, "V012 forged lock bytes")
			opened := c.open(t, created.WorkspaceId)
			candidate := savePreview(t, c, opened, protocol.PreviewRecord{Kind: "create-text", BaseRevision: "0", Title: "before forged lock", Body: "original confirmed body"})
			input := saveInput(newID(t), opened, candidate)
			var receipt protocol.Receipt
			prefix := "/v1/workspaces/" + string(created.WorkspaceId)
			c.request(t, "POST", prefix+"/record-commits", input, &receipt, 200)
			if receipt.OperationId != input.OperationId {
				t.Fatal("original key changed")
			}
			closePublic(t, c, opened)
			p.stop(t)
			lock := filepath.Join(root, ".aboard", "locks", "writer.lock")
			// Windows os.Stat lazily loads FileID in SameFile. Capture it from
			// the actual pre-write handle instead, then physically close that
			// handle before the Host attempts the real exclusive acquisition.
			original, err := os.Open(lock)
			if err != nil {
				t.Fatal("missing existing synthetic fixed lock")
			}
			info, statErr := original.Stat()
			closeErr := original.Close()
			if statErr != nil || closeErr != nil || !info.Mode().IsRegular() || !unreparsed(lock) {
				t.Fatal("cannot pin then close the original fixed lock identity")
			}
			contents := []byte(fmt.Sprintf("pid=%d\nstate=held\n", os.Getpid()))
			if mode == "fake-unlocked" {
				contents = []byte("pid=0\nstate=unlocked\n")
			} else if mode == "nonexistent-pid-text" {
				// Arbitrary untrusted PID text, NOT an OS liveness assertion.
				contents = []byte("pid=4294967295\nstate=stale\n")
			}
			if err = os.WriteFile(lock, contents, 0600); err != nil {
				t.Fatal("cannot preload stopped synthetic lock")
			}
			after, err := os.Stat(lock)
			if err != nil || !os.SameFile(info, after) {
				t.Fatal("preload replaced rather than edited existing fixed object")
			}
			q := f.start(t)
			d := q.connect(t)
			reopened := d.open(t, created.WorkspaceId)
			writerBoundaryOriginal(t, d, reopened, receipt, candidate)
			loserInput := writerMatrixInput(t, f, root, reopened, principal)
			loser := startWriter(t, loserInput)
			busy := loser.next(t)
			writerMatrixBusy(t, loser, busy)
			// Native held-object delete, rename and ordinary overwrite all fail;
			// textual state cannot create an alternative lock object.
			if os.Remove(lock) == nil || os.Rename(lock, lock+"-replaced") == nil || os.WriteFile(lock, []byte("unlocked"), 0600) == nil {
				t.Fatal("held fixed lock was deletable/replaceable/writable")
			}
			closePublic(t, d, reopened)
			winnerInput := writerMatrixInput(t, f, root, reopened, principal)
			winner := startWriter(t, winnerInput)
			saved := winner.next(t)
			identity := writerMatrixPin(t, winner)
			helperReceipt := writerMatrixReceipt(t, saved, winnerInput)
			writerMatrixRefusedOpen(t, d, created.WorkspaceId)
			writerMatrixEnd(t, winner, saved)
			final := d.open(t, created.WorkspaceId)
			rows := saveFacts(t, d, final.WorkspaceId, "2", 2)
			seenBase, seenWinner := false, false
			for _, row := range rows {
				if row.ResourceId == receipt.ResourceId {
					saveMatches(t, row, receipt, candidate)
					seenBase = true
				} else if row.ResourceId == helperReceipt.ResourceId && row.WorkspaceId == helperReceipt.WorkspaceId && row.OperationId == helperReceipt.OperationId && row.ContentDigest == helperReceipt.ContentDigest && row.Body == "fixed real SQLite competitor body" && row.Title == "synthetic external competitor" && row.Version == "1" && row.WorkspaceRevision == "2" {
					seenWinner = true
				} else {
					t.Fatal("lock-content transition changed formal facts")
				}
			}
			if !seenBase || !seenWinner {
				t.Fatal("lock-content transition lost original or newly confirmed data")
			}
			for _, expected := range []protocol.Receipt{receipt, helperReceipt} {
				var op protocol.OperationView
				d.request(t, "GET", prefix+"/operations/"+string(expected.OperationId), nil, &op, 200)
				if op.OperationId != expected.OperationId || op.Receipt == nil || *op.Receipt != expected || op.LogicalState != "committed" || op.PersistenceState != "known" || op.PhysicalState != "drained" {
					t.Fatal("lock-content transition changed confirmed receipt")
				}
			}
			var missing protocol.ErrorView
			d.request(t, "GET", prefix+"/operations/"+loserInput.OperationID, nil, &missing, 409)
			if missing.Code != protocol.ErrorCodeOPERATIONHISTORYUNAVAILABLE {
				t.Fatal("forged lock loser published operation history")
			}
			closePublic(t, d, final)
			q.stop(t)
			got, err := os.ReadFile(lock)
			last, statErr := os.Stat(lock)
			if err != nil || statErr != nil || string(got) != string(contents) || !os.SameFile(info, last) {
				t.Fatal("lock contents or fixed object replaced during normal acquire/release")
			}
			matrixEvidence(t, f, "writer-lock-contents", struct {
				Mode, Contents                   string
				LiveParentPID                    int
				Busy                             writerReply
				Winner                           writerMatrixIdentity
				Original, Saved                  protocol.Receipt
				Rows                             []protocol.TextRecordView
				FixedObjectUnchanged, HostsEnded bool
			}{mode, string(contents), os.Getpid(), busy, identity, receipt, helperReceipt, rows, true, true})
		})
	}
}
