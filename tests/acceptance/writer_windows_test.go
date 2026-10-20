//go:build windows && amd64

package acceptance

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/host"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	win "golang.org/x/sys/windows"
)

const writerPrefix = "ABOARD_SYNTHETIC_WRITER "

func nativeContention(err error) bool {
	if errors.Is(err, win.ERROR_SHARING_VIOLATION) || errors.Is(err, win.ERROR_LOCK_VIOLATION) {
		return true
	}
	var status win.NTStatus
	return errors.As(err, &status) && status == win.STATUS_SHARING_VIOLATION
}

type writerInput struct {
	Fixture, Root                                     string
	Scope                                             workspace.Scope
	PrincipalID, OperationID, CandidateID, ResourceID string
	Hold                                              bool
	StartBarrier                                      bool
}
type writerReply struct {
	Phase               string
	Busy, SQLiteAttempt bool
	Receipt             *workspace.Receipt
}

// Test-build-only competitor. It never reads bearer material or accepts SQL.
// A fixed create-text save is attempted ONLY after the identical native writer
// adapter grants exclusion. A losing process never opens a SQLite connection.
func TestAcceptanceSQLiteWriterHelper(t *testing.T) {
	encoded := os.Getenv("ABOARD_ACCEPTANCE_WRITER")
	if encoded == "" {
		t.Skip("writer helper subprocess only")
	}
	if len(encoded) > 16384 {
		t.Fatal("writer fixture too large")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal("invalid writer fixture")
	}
	var in writerInput
	if json.Unmarshal(raw, &in) != nil || requireSyntheticRoot(in.Fixture) != nil || in.Scope.Validate() != nil {
		t.Fatal("writer synthetic scope rejected")
	}
	config, err := host.LoadConfig(filepath.Join(in.Fixture, "config.json"))
	if err != nil {
		t.Fatal("writer config rejected")
	}
	if len(config.AllowedRoots) != 1 || !within(in.Fixture, config.AllowedRoots[0]) || !within(in.Fixture, in.Root) || !within(config.AllowedRoots[0], in.Root) || !unreparsed(in.Root) {
		t.Fatal("writer target outside synthetic root")
	}
	for _, id := range []string{in.PrincipalID, in.OperationID, in.CandidateID, in.ResourceID} {
		var validated protocol.UUID
		b, _ := json.Marshal(id)
		if json.Unmarshal(b, &validated) != nil {
			t.Fatal("writer identity invalid")
		}
	}
	reply := func(value writerReply) {
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(writerPrefix + string(b))
	}
	// Test-only common starting boundary: no native lease or SQLite handle has
	// been acquired yet. The saved/Hold boundary below is a different byte.
	if in.StartBarrier {
		reply(writerReply{Phase: "ready"})
		var start [1]byte
		if _, err = io.ReadFull(os.Stdin, start[:]); err != nil || start[0] != 2 {
			t.Fatal("writer common start not confirmed")
		}
	}
	lease, err := platform.Acquire(in.Root, platform.Workspace, false, false)
	if nativeContention(err) {
		reply(writerReply{Phase: "busy", Busy: true})
		return
	}
	if err != nil {
		t.Fatal("writer native acquire failed (path withheld)")
	}
	defer func() {
		// A failed helper keeps exclusion until its actual unsuccessful process
		// end. In particular never unwind pins after an unknown SQL/native close.
		if t.Failed() {
			return
		}
		if err := lease.Close(); err != nil {
			t.Error("writer native owner did not close")
		}
	}()
	descriptorPin, err := lease.PinFile("aboard.workspace.json", true)
	if err != nil {
		t.Fatal("writer descriptor pin failed")
	}
	defer func() {
		if !t.Failed() {
			if err := descriptorPin.Close(); err != nil {
				t.Error("writer descriptor close unknown")
			}
		}
	}()
	bytes, err := platform.ReadPinnedWorkspaceFile(descriptorPin, 4096)
	if err != nil {
		t.Fatal("writer descriptor unreadable")
	}
	descriptor, err := workspace.ParseDirectoryDescriptor(bytes)
	if err != nil || descriptor.WorkspaceID != in.Scope.WorkspaceID || descriptor.TenantID != in.Scope.TenantID {
		t.Fatal("writer target identity mismatch")
	}
	databasePin, err := lease.PinFile(`data\workspace.sqlite`, false)
	if err != nil {
		t.Fatal("writer DB pin failed")
	}
	defer func() {
		if !t.Failed() {
			if err := databasePin.Close(); err != nil {
				t.Error("writer DB pin close unknown")
			}
		}
	}()
	ctx := context.Background()
	db, err := sqlite.OpenDatabase(ctx, filepath.Join(in.Root, "data", "workspace.sqlite"), sqlite.ReadWriteExisting, sqlite.Genesis{Kind: sqlite.WorkspaceDatabase})
	if err != nil {
		t.Fatal("writer existing database refused")
	}
	gate, err := sqlite.NewCommitGate(db, "synthetic-external-writer")
	if err != nil {
		t.Fatal("writer gate refused; owner retained until process end")
	}
	defer func() {
		if t.Failed() {
			return
		}
		if err := gate.EndPhysical(); err != nil {
			t.Error("writer physical connection did not end")
		}
	}()
	facts, err := sqlite.NewWorkspaceFacts(gate, in.Scope)
	if err != nil {
		t.Fatal("writer scope refused")
	}
	snapshot, err := facts.Snapshot(ctx, in.Scope)
	if err != nil {
		t.Fatal("writer snapshot refused")
	}
	candidate := workspace.Candidate{WorkspaceID: in.Scope.WorkspaceID, CandidateID: in.CandidateID, PrincipalID: in.PrincipalID, ResourceID: in.ResourceID, Kind: "create-text", BaseRevision: snapshot.Revision, Title: "synthetic external competitor", Body: "fixed real SQLite competitor body", PreviewVersion: 1, PreviewEpoch: "synthetic-external-writer", ExpiresAt: time.Now().UTC().Add(time.Minute)}
	content, err := digest.Content(digest.ContentInput{WorkspaceID: protocol.UUID(candidate.WorkspaceID), ResourceID: protocol.UUID(candidate.ResourceID), Title: protocol.Title(candidate.Title), Body: protocol.Body(candidate.Body)})
	if err != nil {
		t.Fatal("writer content invalid")
	}
	hash, err := digest.Candidate(digest.CandidateInput{WorkspaceID: protocol.UUID(candidate.WorkspaceID), PrincipalID: protocol.UUID(in.PrincipalID), CandidateID: protocol.UUID(candidate.CandidateID), ResourceID: protocol.UUID(candidate.ResourceID), Kind: "create-text", BaseRevision: protocol.Revision(fmt.Sprint(candidate.BaseRevision)), ContentDigest: content, PreviewVersion: "1"})
	if err != nil {
		t.Fatal("writer candidate invalid")
	}
	candidate.ContentDigest = string(content)
	candidate.CandidateDigest = string(hash)
	if err = gate.PutCandidate(ctx, candidate); err != nil {
		t.Fatal("writer preview failed")
	}
	_, admission, err := gate.Admit(ctx, workspace.SaveRequest{InstallationID: in.Scope.InstallationID, TenantID: in.Scope.TenantID, WorkspaceID: in.Scope.WorkspaceID, PrincipalID: in.PrincipalID, OperationID: in.OperationID, CandidateID: in.CandidateID, CandidateDigest: candidate.CandidateDigest, Epoch: candidate.PreviewEpoch, PreviewVersion: 1}, time.Now().UTC())
	if err != nil || admission == nil {
		t.Fatal("writer admission not confirmed")
	}
	result, err := gate.CommitAccepted(ctx, admission)
	if err != nil || result.Status != sqlite.Confirmed || result.Receipt == nil {
		t.Fatal("writer formal save not confirmed")
	}
	reply(writerReply{Phase: "saved", SQLiteAttempt: true, Receipt: result.Receipt})
	if in.Hold {
		var release [1]byte
		if _, err = io.ReadFull(os.Stdin, release[:]); err != nil || release[0] != 1 {
			t.Fatal("writer release not confirmed")
		}
	}
	// Explicitly close before announcing end; deferred calls are idempotent here.
	if err = gate.EndPhysical(); err != nil {
		t.Fatal("writer connection still unknown")
	}
	if err = databasePin.Close(); err != nil {
		t.Fatal("writer database pin still unknown")
	}
	if err = descriptorPin.Close(); err != nil {
		t.Fatal("writer descriptor pin still unknown")
	}
	if err = lease.Close(); err != nil {
		t.Fatal("writer exclusion still unknown")
	}
	reply(writerReply{Phase: "ended", SQLiteAttempt: true, Receipt: result.Receipt})
}

type writerProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	replies chan writerReply
	done    chan struct{}
	waitErr error
	pinned  win.Handle
	birth   uint64
	image   string
}

// Called only at a ready-before-acquire or saved-and-held boundary, where the
// original child is guaranteed live. Pin before Wait/release, never rediscover a
// terminated process by its numeric PID.
func (p *writerProcess) pin(t *testing.T) {
	t.Helper()
	if p.pinned != 0 {
		return
	}
	h, birth, image, err := processIdentity(p.cmd.Process.Pid)
	if err != nil {
		t.Fatal("writer live process identity unavailable")
	}
	self, err := os.Executable()
	if err != nil || !strings.EqualFold(filepath.Clean(self), filepath.Clean(image)) {
		_ = win.CloseHandle(h)
		t.Fatal("writer image differs from the launched test-only helper")
	}
	p.pinned, p.birth, p.image = h, birth, image
}

func startWriter(t *testing.T, input writerInput) *writerProcess {
	t.Helper()
	if requireSyntheticRoot(input.Fixture) != nil {
		t.Fatal("invalid writer fixture")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	p := &writerProcess{cmd: exec.Command(os.Args[0], "-test.run=^TestAcceptanceSQLiteWriterHelper$", "-test.count=1"), replies: make(chan writerReply, 3), done: make(chan struct{})}
	p.cmd.Env = append(os.Environ(), "ABOARD_ACCEPTANCE_WRITER="+base64.RawURLEncoding.EncodeToString(raw))
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	read, write := io.Pipe()
	p.cmd.Stdout = write
	var diagnostics boundedCapture
	p.cmd.Stderr = &diagnostics
	if err = p.cmd.Start(); err != nil {
		_ = read.Close()
		_ = write.Close()
		t.Fatal("writer helper launch failed")
	}
	scanned := make(chan struct{})
	var output boundedCapture
	go func() {
		defer close(scanned)
		defer read.Close()
		scanner := bufio.NewScanner(read)
		for scanner.Scan() {
			line := scanner.Text()
			_, _ = output.Write(append([]byte(line), '\n'))
			if strings.HasPrefix(line, writerPrefix) {
				var reply writerReply
				if json.Unmarshal([]byte(strings.TrimPrefix(line, writerPrefix)), &reply) == nil {
					select {
					case p.replies <- reply:
					default:
					}
				}
			}
		}
	}()
	go func() {
		p.waitErr = p.cmd.Wait()
		_ = write.Close()
		<-scanned
		// Helper contains no credential path. Preserve bounded synthetic failure
		// evidence locally, never echo arbitrary child output into a test error.
		raw, _ := output.snapshot()
		_ = os.WriteFile(filepath.Join(input.Fixture, fmt.Sprintf("writer-%d-output.txt", p.cmd.Process.Pid)), raw, 0600)
		close(p.done)
	}()
	t.Cleanup(func() {
		_ = p.stdin.Close()
		select {
		case <-p.done:
		default:
			_ = p.cmd.Process.Kill()
			select {
			case <-p.done:
			case <-time.After(10 * time.Second):
				t.Error("writer helper did not actually exit")
			}
		}
		select {
		case <-p.done:
			if p.pinned != 0 {
				if err := win.CloseHandle(p.pinned); err != nil {
					t.Error("writer process pin close unknown")
				}
			}
		default: // unknown child end retains its original identity pin.
		}
	})
	return p
}
func (p *writerProcess) next(t *testing.T) writerReply {
	t.Helper()
	select {
	case value := <-p.replies:
		return value
	case <-p.done:
		if p.waitErr != nil {
			t.Fatal("writer helper exited before evidence (bounded output saved in synthetic fixture)")
		}
		select {
		case value := <-p.replies:
			return value
		default:
			t.Fatal("writer helper exited without evidence")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("writer helper evidence timed out (output withheld)")
	}
	return writerReply{}
}
func (p *writerProcess) wait(t *testing.T) {
	t.Helper()
	select {
	case <-p.done:
		if p.waitErr != nil {
			t.Fatal("writer helper ended unsuccessfully (output withheld)")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("writer helper did not actually end")
	}
}

func TestIndependentSQLiteWriterHelperControllability(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	server := f.start(t)
	c := server.connect(t)
	var principal protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &principal, 200)
	root := filepath.Join(f.config.AllowedRoots[0], "writer")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	var created protocol.WorkspaceView
	c.request(t, "POST", "/v1/workspace-creations", protocol.CreateWorkspace{OperationId: newID(t), Path: protocol.Path(root), DisplayName: "writer fixture"}, &created, 201)
	c.open(t, created.WorkspaceId)
	input := writerInput{Fixture: f.root, Root: root, Scope: workspace.Scope{InstallationID: string(principal.InstallationId), TenantID: string(principal.TenantId), WorkspaceID: string(created.WorkspaceId)}, PrincipalID: string(principal.PrincipalId), OperationID: string(newID(t)), CandidateID: string(newID(t)), ResourceID: string(newID(t)), Hold: true}
	loser := startWriter(t, input)
	result := loser.next(t)
	loser.wait(t)
	if result.Phase != "busy" || !result.Busy || result.SQLiteAttempt || result.Receipt != nil {
		t.Fatal("second real process was not excluded before SQLite")
	}
	server.stop(t)
	winner := startWriter(t, input)
	saved := winner.next(t)
	if saved.Phase != "saved" || !saved.SQLiteAttempt || saved.Receipt == nil {
		t.Fatal("free second process did not actually save")
	}
	if lease, err := platform.Acquire(root, platform.Workspace, false, false); err == nil {
		_ = lease.Close()
		t.Fatal("writer helper did not continuously retain native exclusion")
	} else if !nativeContention(err) {
		t.Fatal("native acquire failed without proving contention")
	}
	if _, err := winner.stdin.Write([]byte{1}); err != nil {
		t.Fatal("writer release failed")
	}
	if ended := winner.next(t); ended.Phase != "ended" || ended.Receipt == nil || *ended.Receipt != *saved.Receipt {
		t.Fatal("writer did not confirm same-history physical end")
	}
	winner.wait(t)
	restarted := f.start(t)
	d := restarted.connect(t)
	d.open(t, created.WorkspaceId)
	var record protocol.TextRecordView
	d.request(t, "GET", "/v1/workspaces/"+string(created.WorkspaceId)+"/records/"+input.ResourceID, nil, &record, 200)
	if record.OperationId != protocol.UUID(input.OperationID) || record.Body != "fixed real SQLite competitor body" {
		t.Fatal("Host did not read actual competitor commit")
	}
	restarted.stop(t)
	t.Log("external competitor truly saved via fixed SQLite under same native adapter, held writer.lock, then normal release; loser never opened SQLite; not the full ten-round V012 matrix")
}
