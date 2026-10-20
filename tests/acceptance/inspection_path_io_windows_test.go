//go:build windows && amd64

package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/protocol"
	win "golang.org/x/sys/windows"
)

// Unlike tree, this reads ONLY the parent's entries/attributes, never traversing
// the deliberately missing/reparse source. The referent has its own full tree.
func inspectionPathNamespace(t *testing.T, parent string) map[string]uint32 {
	t.Helper()
	if !within(logRoot(), parent) || !unreparsed(parent) {
		t.Fatal("unowned path namespace")
	}
	entries, e := os.ReadDir(parent)
	if e != nil {
		t.Fatal(e)
	}
	out := map[string]uint32{}
	for _, entry := range entries {
		name, e := win.UTF16PtrFromString(filepath.Join(parent, entry.Name()))
		if e != nil {
			t.Fatal(e)
		}
		attrs, e := win.GetFileAttributes(name)
		if e != nil {
			t.Fatal("namespace attribute query failed")
		}
		out[entry.Name()] = attrs
	}
	return out
}

func inspectionOwnedReferentID(t *testing.T, path, root, parked string) [2]uint64 {
	t.Helper()
	if (path != root && path != parked) || !within(logRoot(), path) {
		t.Fatal("unowned referent query")
	}
	name, e := win.UTF16PtrFromString(path)
	if e != nil {
		t.Fatal(e)
	}
	// Following this alias is ONLY a positive control on our exact created link.
	h, e := win.CreateFile(name, win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE|win.FILE_SHARE_DELETE, nil, win.OPEN_EXISTING, win.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if e != nil {
		t.Fatal("native referent query refused")
	}
	var info win.ByHandleFileInformation
	queryErr := win.GetFileInformationByHandle(h, &info)
	closeErr := win.CloseHandle(h)
	if queryErr != nil || closeErr != nil || info.FileAttributes&win.FILE_ATTRIBUTE_DIRECTORY == 0 {
		t.Fatal("referent identity/end unavailable")
	}
	return [2]uint64{uint64(info.VolumeSerialNumber), uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}
}

func TestV009IndependentInspectorNativePathIO(t *testing.T) {
	for _, kind := range []string{"missing", "junction"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			first := f.start(t)
			c := first.connect(t)
			root, a := newInspectionWorkspace(t, c, "observed-source")
			_, b := newInspectionWorkspace(t, c, "native-path-independent-B")
			// Real normal creation establishes identity; close the entire original owner
			// before moving the stopped source. No test runs a second storage connection.
			first.stop(t)
			baseline := tree(t, root)
			parent := f.config.AllowedRoots[0]
			parked := filepath.Join(parent, "observed-parked")
			if root != filepath.Join(parent, "observed-source") || !within(parent, parked) || !unreparsed(root) {
				t.Fatal("unowned stopped source")
			}
			if e := os.Rename(root, parked); e != nil {
				t.Fatal("stopped owned source move failed")
			}
			if kind == "junction" {
				cmd := exec.Command("cmd.exe", "/c", "mklink", "/J", root, parked)
				var output boundedCapture
				cmd.Stdout = &output
				cmd.Stderr = &output
				if cmd.Run() != nil {
					t.Fatal("owned junction creation failed")
				}
				attrs := inspectionPathNamespace(t, parent)
				if attrs[filepath.Base(root)]&win.FILE_ATTRIBUTE_REPARSE_POINT == 0 || inspectionOwnedReferentID(t, root, root, parked) != inspectionOwnedReferentID(t, parked, root, parked) {
					t.Fatal("actual junction/referent control failed")
				}
			} else if _, e := os.Lstat(root); !os.IsNotExist(e) {
				t.Fatal("source is not actually missing")
			}
			before := tree(t, parked)
			namespace := inspectionPathNamespace(t, parent)
			measured := instrumentedInspectionFixture(t, f, root)
			outgoing := startInspectionHTTPWatch(t, f, root, measured)
			defer outgoing.close(t)
			p := measured.start(t)
			d := p.connect(t)
			processes := startInspectionProcessWatch(t, p)
			defer processes.cleanup(t, p)
			prior := inspectionObservationIDs(t, f)
			watch := startInspectionWatch(t, parent)
			watchAttempted, watchJoined := false, false
			finishWatch := func() { finishInspectionOnce(&watchAttempted, &watchJoined, func() { watch.finish(t, false) }) }
			defer finishWatch()
			defer processes.cleanup(t, p) // actual process end before observer/fixture unwind

			var denied protocol.ErrorView
			d.request(t, "POST", "/v1/workspace-inspections", protocol.InspectWorkspace{Path: protocol.Path(root)}, &denied, 409)
			if denied.Code != protocol.ErrorCodeINSPECTIONUNAVAILABLE {
				t.Fatal("native source failure exposed scope or invented access", denied.Code)
			}
			observation := newInspectionObservation(t, f, prior, p, root)
			processes.verify(t, p, observation.Processes)
			outgoing.verify(t, observation)
			if observation.Code != string(protocol.ErrorCodeINSPECTIONUNAVAILABLE) || observation.ScratchCalls != 0 || observation.ScratchBytes != 0 || observation.After.Writes != observation.Before.Writes || observation.After.WriteBytes != observation.Before.WriteBytes {
				t.Fatal("native acquire refusal performed untracked writes/private copy")
			}
			if !reflect.DeepEqual(before, tree(t, parked)) || !reflect.DeepEqual(namespace, inspectionPathNamespace(t, parent)) {
				t.Fatal("inspect changed source/referent/namespace")
			}
			pre := registrationPrecondition(t, d, a.WorkspaceId, "open")
			d.request(t, "POST", "/v1/workspaces/"+string(a.WorkspaceId)+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, &denied, 409)
			if denied.Code != protocol.ErrorCodeRECOVERYREQUIRED || !reflect.DeepEqual(before, tree(t, parked)) || !reflect.DeepEqual(namespace, inspectionPathNamespace(t, parent)) {
				t.Fatal("authorized open created/repaired missing or aliased source")
			}
			finishWatch() // combined source/precondition/open notification window, not PID attribution of open

			other := d.open(t, b.WorkspaceId)
			prefix := "/v1/workspaces/" + string(b.WorkspaceId)
			var candidate protocol.CandidateView
			d.request(t, "POST", prefix+"/record-previews", protocol.PreviewRecord{OpenEpoch: *other.OpenEpoch, Kind: "create-text", BaseRevision: *other.Revision, Title: "after native A boundary", Body: protocol.Body(kind)}, &candidate, 201)
			var receipt protocol.Receipt
			d.request(t, "POST", prefix+"/record-commits", protocol.CommitRecord{OperationId: newID(t), OpenEpoch: *other.OpenEpoch, CandidateId: candidate.CandidateId, CandidateDigest: candidate.CandidateDigest, PreviewVersion: candidate.PreviewVersion}, &receipt, 200)
			var record protocol.TextRecordView
			d.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil, &record, 200)
			if record.Body != protocol.Body(kind) || record.OperationId != receipt.OperationId || receipt.NewRevision != "1" {
				t.Fatal("native A boundary fenced B actual save/read")
			}
			p.stop(t)
			processes.stopped(t)
			processes.cleanup(t, p)
			outgoing.verify(t, observation)
			outgoing.close(t)
			if !processes.drained || !watchJoined || !outgoing.joined {
				t.Fatal("original observers/process physical end unproved; retain fixture")
			}
			if !reflect.DeepEqual(before, tree(t, parked)) || !reflect.DeepEqual(namespace, inspectionPathNamespace(t, parent)) {
				t.Fatal("normal Host end changed native specimen")
			}
			matrixEvidence(t, f, "native-path-io-"+kind, observation)
			// Only this successful, physically joined path restores. Failed fixtures stay
			// intact; no deferred RemoveAll/rename through uncertain resources or aliases.
			if kind == "junction" {
				attrs := inspectionPathNamespace(t, parent)
				if attrs[filepath.Base(root)]&win.FILE_ATTRIBUTE_REPARSE_POINT == 0 || inspectionOwnedReferentID(t, root, root, parked) != inspectionOwnedReferentID(t, parked, root, parked) {
					t.Fatal("junction changed; refuse removal")
				}
				if e := os.Remove(root); e != nil {
					t.Fatal("owned junction removal failed")
				}
			} else if _, e := os.Lstat(root); !os.IsNotExist(e) {
				t.Fatal("missing source unexpectedly exists; refuse restoration")
			}
			if e := os.Rename(parked, root); e != nil {
				t.Fatal("stopped source restoration failed")
			}
			if !reflect.DeepEqual(baseline, tree(t, root)) {
				t.Fatal("restoration changed original source")
			}
			last := f.start(t)
			normal := last.connect(t)
			reopened := normal.open(t, a.WorkspaceId)
			if reopened.WorkspaceId != a.WorkspaceId || reopened.Revision == nil || *reopened.Revision != "0" {
				t.Fatal("normal restored source lost original identity/genesis")
			}
			last.stop(t)
			if !reflect.DeepEqual(baseline, tree(t, root)) {
				t.Fatal("restored normal open changed original bytes")
			}
		})
	}
}
