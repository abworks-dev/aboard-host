//go:build windows && amd64

package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	win "golang.org/x/sys/windows"
)

func matrixEvidence(t *testing.T, f *fixture, name string, value any) {
	t.Helper()
	if requireSyntheticRoot(f.root) != nil || filepath.Base(name) != name {
		t.Fatal("unowned evidence target")
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(f.root, name+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func registrationPrecondition(t *testing.T, c *client, id protocol.UUID, use string) security.PreconditionReply {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	started := time.Now()
	out, err := pipeAvailable(ctx, func() (security.PreconditionReply, error) {
		return security.ReceivePrecondition(ctx, c.process.fixture.config.PipeName, uint32(c.process.cmd.Process.Pid), security.PreconditionRequest{Version: 1, Kind: "registration", Bearer: c.token, WorkspaceID: string(id), IntendedUse: use})
	})
	if err != nil {
		// Preserve the FIRST failed exchange, then best-effort collect the
		// existing server ring by the original pinned graceful interrupt. No
		// bearer/request/locator, no exchange retry, and collection cannot turn
		// this original failure into a pass. Shutdown may cancel a late ACK.
		diagnostic := security.DiagnosePipe(ctx, err, time.Since(started))
		matrixEvidence(t, c.process.fixture, fmt.Sprintf("pipe-precondition-failure-%d", c.process.cmd.Process.Pid), struct {
			PID            int
			Birth          uint64
			Started, Ended time.Time
			Diagnostic     security.PipeDiagnostic
		}{c.process.cmd.Process.Pid, c.process.birth, started, time.Now(), diagnostic})
		collectedAt := time.Now().UTC()
		graceful := c.process.collectFailedPipeServer()
		matrixEvidence(t, c.process.fixture, fmt.Sprintf("pipe-server-collection-%d", c.process.cmd.Process.Pid), struct {
			RequestedAt                  time.Time
			GracefulEnded                bool
			ShutdownCanCauseCancellation bool
		}{collectedAt, graceful, true})
		t.Fatal("native registration precondition refused", safePipeFailure(err), diagnostic)
	}
	return out
}

func closePublic(t *testing.T, c *client, v protocol.WorkspaceView) protocol.OperationView {
	t.Helper()
	if v.OpenEpoch == nil {
		t.Fatal("public close needs actual epoch")
	}
	key := newID(t)
	prefix := "/v1/workspaces/" + string(v.WorkspaceId)
	var out protocol.OperationView
	status := c.request(t, "POST", prefix+"/close", protocol.CloseWorkspace{OperationId: key, OpenEpoch: *v.OpenEpoch}, &out, 0)
	if status != 200 && status != 202 {
		t.Fatal("public close rejected", status)
	}
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for out.PhysicalState != "drained" {
		select {
		case <-deadline.C:
			t.Fatal("physical close not confirmed")
		case <-time.After(10 * time.Millisecond):
		}
		c.request(t, "GET", prefix+"/operations/"+string(key), nil, &out, 200)
	}
	if out.OperationId != key || out.Kind != "close" || out.LogicalState != "committed" || out.PersistenceState != "unavailable" {
		t.Fatal("close fabricated durable or logical result")
	}
	return out
}

func assertOriginalInstallation(t *testing.T, c *client, key, id protocol.UUID, kind string) {
	t.Helper()
	var out protocol.OperationView
	c.request(t, "GET", "/v1/installation-operations/"+string(key), nil, &out, 200)
	if out.Scope != "installation" || string(out.Kind) != kind || out.OperationId != key || out.WorkspaceId == nil || *out.WorkspaceId != id || out.LogicalState != "committed" || out.PersistenceState != "known" {
		t.Fatal("installation operation lost original identity or confirmed outcome")
	}
}

func TestV008NonemptyCreateRejectsWithoutMutation(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	p := f.start(t)
	c := p.connect(t)
	root := filepath.Join(f.config.AllowedRoots[0], "nonempty")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "synthetic-sentinel"), []byte("never overwrite\n\x00synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	before := tree(t, root)
	key := newID(t)
	var refused protocol.ErrorView
	c.request(t, "POST", "/v1/workspace-creations", protocol.CreateWorkspace{OperationId: key, Path: protocol.Path(root), DisplayName: "must be refused"}, &refused, 409)
	if refused.Code != protocol.ErrorCodeDIRECTORYNOTEMPTY || refused.Outcome != "rejected" {
		t.Fatal("nonempty source misclassified", refused.Code)
	}
	if !reflect.DeepEqual(before, tree(t, root)) {
		t.Fatal("rejected create changed original source")
	}
	// The authorized coordinator records intent BEFORE native empty-directory
	// inspection. Preserve that rejected original key; do not demand a bypass.
	var original protocol.OperationView
	c.request(t, "GET", "/v1/installation-operations/"+string(key), nil, &original, 200)
	if original.OperationId != key || original.Kind != "create" || original.LogicalState != "rejected" || original.PersistenceState != "known" || original.WorkspaceId == nil {
		t.Fatal("admitted rejection lost its original investigation key")
	}
	var duplicate protocol.ErrorView
	c.request(t, "POST", "/v1/workspace-creations", protocol.CreateWorkspace{OperationId: key, Path: protocol.Path(root), DisplayName: "must be refused"}, &duplicate, 409)
	if duplicate.Outcome != "rejected" || !reflect.DeepEqual(before, tree(t, root)) {
		t.Fatal("original rejected key replayed creation")
	}
	p.stop(t)
	matrixEvidence(t, f, "nonempty-evidence", struct {
		Operation     protocol.UUID
		Code          protocol.ErrorCode
		Before, After map[string]string
	}{key, refused.Code, before, tree(t, root)})
}

// Copying is ONLY a stopped, owned synthetic-fixture stimulus, not a Host backup
// API. All paths/regular objects are checked; no SQL or credentials are imported.
func copyStoppedWorkspace(t *testing.T, f *fixture, source, target string) {
	t.Helper()
	if requireSyntheticRoot(f.root) != nil || !within(f.config.AllowedRoots[0], source) || !within(f.config.AllowedRoots[0], target) || !unreparsed(source) {
		t.Fatal("unsafe synthetic copy")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("copy target already exists")
	}
	if err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if !unreparsed(path) {
			t.Fatal("copy source contains reparse")
		}
		dest := filepath.Join(target, relative)
		if d.IsDir() {
			return os.Mkdir(dest, 0700)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			t.Fatal("copy source not regular")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(raw)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}); err != nil {
		t.Fatal("synthetic copy failed", err)
	}
}

func workspaceRootIdentity(t *testing.T, root string) platform.Identity {
	t.Helper()
	lease, err := platform.Acquire(root, platform.Workspace, false, false)
	if err != nil {
		t.Fatal("stopped source not exclusively available", err)
	}
	id := lease.RootIdentity()
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestV008RegistrationCopyAndClosedRelinkPreserveFormalFacts(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	p := f.start(t)
	c := p.connect(t)
	root := filepath.Join(f.config.AllowedRoots[0], "original")
	var created protocol.WorkspaceView
	createKey := newID(t)
	c.request(t, "POST", "/v1/workspace-creations", protocol.CreateWorkspace{OperationId: createKey, Path: protocol.Path(root), DisplayName: "same stable workspace"}, &created, 201)
	opened := c.open(t, created.WorkspaceId)
	prefix := "/v1/workspaces/" + string(created.WorkspaceId)
	var preview protocol.CandidateView
	const body = "same-volume identity and formal bytes: 合成\nkept across rename"
	c.request(t, "POST", prefix+"/record-previews", protocol.PreviewRecord{OpenEpoch: *opened.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "same title", Body: body}, &preview, 201)
	saveKey := newID(t)
	commit := protocol.CommitRecord{OperationId: saveKey, OpenEpoch: *opened.OpenEpoch, CandidateId: preview.CandidateId, CandidateDigest: preview.CandidateDigest, PreviewVersion: preview.PreviewVersion}
	var receipt protocol.Receipt
	c.request(t, "POST", prefix+"/record-commits", commit, &receipt, 200)
	// Even an existing physical path cannot relink an actively open lifecycle.
	current := registrationPrecondition(t, c, created.WorkspaceId, "relink")
	var active protocol.ErrorView
	c.request(t, "POST", prefix+"/relink", protocol.RelinkWorkspace{OperationId: newID(t), Path: protocol.Path(root), ExpectedRegistrationVersion: current.RegistrationVersion}, &active, 409)
	if active.Code != protocol.ErrorCodeWORKSPACECLOSED {
		t.Fatal("active relink escaped lifecycle", active.Code)
	}
	closePublic(t, c, opened)
	p.stop(t)
	before := tree(t, root)
	originalPhysical := workspaceRootIdentity(t, root)
	clone := filepath.Join(f.config.AllowedRoots[0], "copied")
	copyStoppedWorkspace(t, f, root, clone)
	copyBefore := tree(t, clone)
	if !reflect.DeepEqual(before, copyBefore) || reflect.DeepEqual(originalPhysical, workspaceRootIdentity(t, clone)) {
		t.Fatal("copy control did not create same bytes with distinct native identity")
	}
	p2 := f.start(t)
	d := p2.connect(t)
	descriptor, err := workspace.NewDirectoryDescriptor(string(created.WorkspaceId), string(created.TenantId))
	if err != nil {
		t.Fatal(err)
	}
	baselineVersion := registrationPrecondition(t, d, created.WorkspaceId, "relink").RegistrationVersion
	register := protocol.RegisterWorkspace{OperationId: newID(t), Path: protocol.Path(root), ExpectedWorkspaceId: created.WorkspaceId, ExpectedIdentityDigest: protocol.Digest(descriptor.IdentityDigest)}
	for n := 0; n < 3; n++ {
		if n == 2 {
			register.OperationId = newID(t)
		}
		var registered protocol.WorkspaceView
		d.request(t, "POST", "/v1/workspace-registrations", register, &registered, 200)
		if registered.WorkspaceId != created.WorkspaceId || registered.TenantId != created.TenantId || registered.OpenEpoch != nil || registered.DirectoryLifecycle != "registeredClosed" {
			t.Fatal("duplicate registration copied facts or opened source")
		}
		assertOriginalInstallation(t, d, register.OperationId, created.WorkspaceId, "register")
		if registrationPrecondition(t, d, created.WorkspaceId, "relink").RegistrationVersion != baselineVersion {
			t.Fatal("duplicate registration mutated registration version")
		}
	}
	copiedRequest := register
	copiedRequest.OperationId = newID(t)
	copiedRequest.Path = protocol.Path(clone)
	var conflict protocol.ErrorView
	d.request(t, "POST", "/v1/workspace-registrations", copiedRequest, &conflict, 409)
	if conflict.Code != protocol.ErrorCodeIDENTITYCONFLICT {
		t.Fatal("same UUID different root must be identity conflict", conflict.Code)
	}
	if !reflect.DeepEqual(copyBefore, tree(t, clone)) || !reflect.DeepEqual(before, tree(t, root)) {
		t.Fatal("registration modified either workspace")
	}
	version := registrationPrecondition(t, d, created.WorkspaceId, "relink")
	oldVersion, err := strconv.ParseInt(string(version.RegistrationVersion), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(f.config.AllowedRoots[0], "moved")
	// Exact verified synthetic paths, both on the same existing NTFS volume.
	if !within(f.config.AllowedRoots[0], root) || !within(f.config.AllowedRoots[0], moved) {
		t.Fatal("unowned move")
	}
	if err = os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(originalPhysical, workspaceRootIdentity(t, moved)) || !reflect.DeepEqual(before, tree(t, moved)) {
		t.Fatal("move changed native root identity or formal bytes")
	}
	moveKey := newID(t)
	move := protocol.RelinkWorkspace{OperationId: moveKey, Path: protocol.Path(moved), ExpectedRegistrationVersion: version.RegistrationVersion}
	var bound protocol.WorkspaceView
	d.request(t, "POST", prefix+"/relink", move, &bound, 200)
	if bound.WorkspaceId != created.WorkspaceId || bound.OpenEpoch != nil || bound.DirectoryLifecycle != "registeredClosed" {
		t.Fatal("relink changed identity or implicitly opened")
	}
	assertOriginalInstallation(t, d, moveKey, created.WorkspaceId, "relink")
	afterVersion := registrationPrecondition(t, d, created.WorkspaceId, "relink")
	if string(afterVersion.RegistrationVersion) != strconv.FormatInt(oldVersion+1, 10) {
		t.Fatal("relink did not compare-and-advance exact registration version")
	}
	// Existing key remains inquiry-only despite its now-stale expected version.
	d.request(t, "POST", prefix+"/relink", move, &bound, 200)
	// Isolate CAS from all other predicates: a second valid stopped rename
	// has a DISTINCT target and identical native identity; only version is stale.
	movedAgain := filepath.Join(f.config.AllowedRoots[0], "moved-again")
	if !within(f.config.AllowedRoots[0], moved) || !within(f.config.AllowedRoots[0], movedAgain) {
		t.Fatal("unowned second move")
	}
	if err = os.Rename(moved, movedAgain); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(originalPhysical, workspaceRootIdentity(t, movedAgain)) {
		t.Fatal("second rename changed physical identity")
	}
	var stale protocol.ErrorView
	move.OperationId = newID(t)
	move.Path = protocol.Path(movedAgain)
	d.request(t, "POST", prefix+"/relink", move, &stale, 409)
	if stale.Code != protocol.ErrorCodeSTALEHANDLE {
		t.Fatal("stale locator CAS unexpectedly accepted or misclassified", stale.Code)
	}
	var notAdmitted protocol.ErrorView
	d.request(t, "GET", "/v1/installation-operations/"+string(move.OperationId), nil, &notAdmitted, 404)
	if notAdmitted.Code != protocol.ErrorCodeNOTFOUND || !reflect.DeepEqual(before, tree(t, movedAgain)) {
		t.Fatal("stale CAS admitted or changed source")
	}
	move.OperationId = newID(t)
	move.ExpectedRegistrationVersion = afterVersion.RegistrationVersion
	d.request(t, "POST", prefix+"/relink", move, &bound, 200)
	assertOriginalInstallation(t, d, move.OperationId, created.WorkspaceId, "relink")
	finalVersion := registrationPrecondition(t, d, created.WorkspaceId, "relink").RegistrationVersion
	if string(finalVersion) != strconv.FormatInt(oldVersion+2, 10) || bound.OpenEpoch != nil || bound.WorkspaceId != created.WorkspaceId {
		t.Fatal("current-version control did not complete ONLY locator change")
	}
	if _, err = os.Lstat(moved); !os.IsNotExist(err) {
		t.Fatal("intermediate old locator was recreated")
	}
	moved = movedAgain
	if _, err = os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("old location was recreated")
	}
	reopened := d.open(t, created.WorkspaceId)
	if reopened.OpenEpoch == nil || *reopened.OpenEpoch == *opened.OpenEpoch {
		t.Fatal("moved reopen did not publish fresh epoch")
	}
	var record protocol.TextRecordView
	d.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil, &record, 200)
	if record.Body != body || record.OperationId != saveKey || record.ContentDigest != receipt.ContentDigest || record.WorkspaceRevision != receipt.NewRevision || record.Version != receipt.NewVersion {
		t.Fatal("rename lost actual formal record")
	}
	var originalReceipt protocol.Receipt
	d.request(t, "POST", prefix+"/record-commits", commit, &originalReceipt, 200)
	if originalReceipt != receipt {
		t.Fatal("relink replayed old save instead of same original receipt")
	}
	p2.stop(t)
	if !reflect.DeepEqual(before, tree(t, moved)) || !reflect.DeepEqual(copyBefore, tree(t, clone)) {
		t.Fatal("reopen/rejection changed stopped bytes")
	}
	matrixEvidence(t, f, "registration-relink-evidence", struct {
		Workspace                               protocol.UUID
		Create, Register, Move, MoveAgain, Save protocol.UUID
		Physical                                platform.Identity
		VersionBefore, VersionAfter             protocol.PositiveRevision
		Receipt                                 protocol.Receipt
		Before, After, Copy                     map[string]string
	}{created.WorkspaceId, createKey, register.OperationId, moveKey, move.OperationId, saveKey, originalPhysical, version.RegistrationVersion, finalVersion, receipt, before, tree(t, moved), tree(t, clone)})
}

type cliObservation struct {
	PID                                  int
	Exit                                 int
	Born, Ended                          uint64 // native FILETIME from a pinned original process handle
	IdentityProven                       bool
	Initialized, Unavailable, SafeOutput bool
	OutputBytes, DiagnosticBytes         int
}

func runObservedCLI(ctx context.Context, f *fixture, mode string) cliObservation {
	var out, diagnostics boundedCapture
	cmd := exec.CommandContext(ctx, f.executable, mode, "--config", f.configPath)
	cmd.Dir = projectRoot()
	cmd.Stdout = &out
	cmd.Stderr = &diagnostics
	result := cliObservation{Exit: -1}
	if cmd.Start() != nil {
		return result
	}
	result.PID = cmd.Process.Pid
	pinned, birth, image, identityErr := processIdentity(result.PID)
	waitErr := cmd.Wait()
	if cmd.ProcessState != nil {
		result.Exit = cmd.ProcessState.ExitCode()
	}
	if pinned != 0 {
		var born, ended, kernel, user win.Filetime
		timeErr := win.GetProcessTimes(pinned, &born, &ended, &kernel, &user)
		result.Born = uint64(born.HighDateTime)<<32 | uint64(born.LowDateTime)
		result.Ended = uint64(ended.HighDateTime)<<32 | uint64(ended.LowDateTime)
		closeErr := win.CloseHandle(pinned)
		result.IdentityProven = identityErr == nil && timeErr == nil && closeErr == nil && birth != 0 && result.Born == birth && result.Ended >= birth && strings.EqualFold(filepath.Clean(image), f.executable)
	}
	raw, overflow := out.snapshot()
	diagnostic, over := diagnostics.snapshot()
	result.OutputBytes, result.DiagnosticBytes = len(raw), len(diagnostic)
	// A refused startup MUST emit no readiness or other stdout event. A successful
	// init has exactly the one fixed initialized event, not arbitrary nonzero exit.
	value, parseErr := protocol.ParseJSON([]byte(strings.TrimSpace(string(raw))))
	fields, object := value.(map[string]any)
	initialEvent := parseErr == nil && object && len(fields) == 1 && fields["event"] == "initialized"
	result.Initialized = waitErr == nil && mode == "init" && initialEvent && result.Exit == 0 && len(diagnostic) == 0
	result.SafeOutput = !overflow && !over && !bytesContainAny(raw, f.root, f.configPath) && !bytesContainAny(diagnostic, f.root, f.configPath) && (len(raw) == 0 || result.Initialized)
	result.Unavailable = result.Exit == 1 && strings.TrimSpace(string(diagnostic)) == "HOST_UNAVAILABLE" && len(raw) == 0
	return result
}
func bytesContainAny(raw []byte, values ...string) bool {
	for _, value := range values {
		if strings.Contains(string(raw), value) {
			return true
		}
	}
	return false
}

func TestV008ConcurrentFirstInitializationHasOneOwner(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan cliObservation, 2)
	for n := 0; n < 2; n++ {
		go func() { <-start; results <- runObservedCLI(ctx, f, "init") }()
	}
	close(start)
	first, second := <-results, <-results
	if ctx.Err() != nil {
		t.Fatal("init race timed out; deadline termination is failure cleanup, not drain proof")
	}
	successful := 0
	for _, result := range []cliObservation{first, second} {
		if result.PID == 0 || !result.IdentityProven || !result.SafeOutput {
			t.Fatal("CLI did not launch with proven identity/safe stdout and stderr")
		}
		if result.Initialized && result.Exit == 0 {
			successful++
		} else if !result.Unavailable {
			t.Fatal("losing contender did not give a clean fixed CLI refusal")
		}
	}
	if first.PID == second.PID || successful != 1 {
		t.Fatal("concurrent first use did not have exactly one actual initializer", first, second)
	}
	// Birth/end are actual kernel times from launch-pinned handles. No proof is
	// inferred merely from goroutine scheduling or two serialized outcomes.
	if first.Born >= second.Ended || second.Born >= first.Ended {
		t.Fatal("initializer process lifetimes did not actually overlap")
	}
	before := tree(t, f.config.InstallationRoot)
	again := runObservedCLI(ctx, f, "init")
	if !again.Unavailable || !again.IdentityProven || !again.SafeOutput || !reflect.DeepEqual(before, tree(t, f.config.InstallationRoot)) {
		t.Fatal("existing installation silently reinitialized")
	}
	p := f.start(t)
	c := p.connect(t)
	var identity protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &identity, 200)
	p.stop(t)
	matrixEvidence(t, f, "initialization-race-evidence", struct {
		Contenders [2]cliObservation
		Repeated   cliObservation
		Identity   protocol.PrincipalView
	}{[2]cliObservation{first, second}, again, identity})
}

func TestV008InvalidInstallationsNeverBecomeFirstUse(t *testing.T) {
	for _, kind := range []string{"missing", "empty", "missing-bootstrap", "missing-database", "missing-lock", "corrupt-bootstrap", "corrupt-database"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			if kind == "empty" {
				if err := platform.CreatePrivateDirectory(f.config.InstallationRoot); err != nil {
					t.Fatal(err)
				}
			} else if kind != "missing" {
				f.initialize(t)
				name := "installation.sqlite"
				if strings.Contains(kind, "bootstrap") {
					name = "bootstrap.dpapi"
				}
				if kind == "missing-lock" {
					name = "host.lock"
				}
				path := filepath.Join(f.config.InstallationRoot, name)
				if !within(f.config.InstallationRoot, path) || requireSyntheticRoot(f.root) != nil {
					t.Fatal("unowned installation mutation")
				}
				if strings.HasPrefix(kind, "missing-") {
					if err := os.Rename(path, path+".held"); err != nil {
						t.Fatal(err)
					}
				} else {
					raw, err := os.ReadFile(path)
					if err != nil || len(raw) == 0 {
						t.Fatal("cannot prepare corruption control")
					}
					raw[0] ^= 0xff
					if err = os.WriteFile(path, raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := tree(t, f.root)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			out := runObservedCLI(ctx, f, "serve")
			if ctx.Err() != nil {
				t.Fatal("invalid startup retained/timed out; forced deadline is not successful refusal")
			}
			if out.PID == 0 || !out.Unavailable || !out.IdentityProven || !out.SafeOutput || out.OutputBytes != 0 {
				t.Fatal("invalid installation became available or leaked diagnostics", out)
			}
			after := tree(t, f.root)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("failed open seeded/repaired installation")
			}
			matrixEvidence(t, f, "invalid-installation-evidence", struct {
				Kind          string
				Process       cliObservation
				Before, After map[string]string
			}{kind, out, before, after})
		})
	}
}
