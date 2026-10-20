//go:build windows && amd64

// Package acceptance drives the production executable from an external process.
// No Host Run, SQL, direct save, or authentication bypass lives in this client.
package acceptance

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/host"
	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	win "golang.org/x/sys/windows"
)

const executableEnvironment = "ABOARD_ACCEPTANCE_HOST_EXE"

func projectRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
func logRoot() string { return filepath.Join(projectRoot(), ".data", "logs", "001-validation") }
func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && relative != ".." && !filepath.IsAbs(relative)
}

// Reject every reparse ancestor, not only a final-file symlink. This is a
// trusted synthetic-fixture guard, not a sandbox against malicious same-SID races.
func unreparsed(path string) bool {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.EqualFold(filepath.Clean(resolved), path) {
		return false
	}
	for current := path; ; current = filepath.Dir(current) {
		p, err := win.UTF16PtrFromString(current)
		if err != nil {
			return false
		}
		attributes, err := win.GetFileAttributes(p)
		if err != nil || attributes&win.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return false
		}
		if filepath.Dir(current) == current {
			return true
		}
	}
}
func requireSyntheticRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || !within(logRoot(), root) || !strings.HasPrefix(filepath.Base(root), "t023-") || !unreparsed(root) {
		return errors.New("acceptance synthetic root required")
	}
	return nil
}
func checkedExecutable(path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !within(logRoot(), path) || filepath.Ext(path) != ".exe" || !unreparsed(path) {
		return "", errors.New("acceptance executable must be a local diagnostic build")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("acceptance executable unavailable")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("acceptance executable unreadable")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

type fixture struct {
	root, configPath, executable, sha string
	config                            host.Config
	environment                       []string // test-only nonsecret scoped fault fixture
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	executable := os.Getenv(executableEnvironment)
	if executable == "" {
		t.Skip("independent executable not configured; build CLI then set " + executableEnvironment)
	}
	sha, err := checkedExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(logRoot(), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(logRoot(), "t023-")
	if err != nil {
		t.Fatal(err)
	}
	if err = requireSyntheticRoot(root); err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(root, "allowed")
	if err = os.Mkdir(allowed, 0700); err != nil {
		t.Fatal(err)
	}
	c := host.Config{InstallationRoot: filepath.Join(root, "installation"), AllowedRoots: []string{allowed}, Listen: "127.0.0.1:0", PipeName: "aboard-g5-" + filepath.Base(root)}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	if err = os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return &fixture{root: root, configPath: configPath, executable: executable, sha: sha, config: c}
}

// Bounded in-memory capture, not an unbounded child-process log sink. Output is
// never copied into diagnostics on failure because it is not trusted input.
type boundedCapture struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	overflow bool
}

func (b *boundedCapture) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := (1 << 20) - b.buf.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.overflow = true
	}
	_, _ = b.buf.Write(p)
	return n, nil
}
func (b *boundedCapture) snapshot() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes()), b.overflow
}

type processEvidence struct {
	PID          int          `json:"pid"`
	ParentPID    int          `json:"parentPid"`
	Birth        uint64       `json:"birthFiletime"`
	BinarySHA256 string       `json:"binarySHA256"`
	Go           string       `json:"go"`
	Started      time.Time    `json:"started"`
	Ready        time.Time    `json:"ready,omitempty"`
	Interrupt    time.Time    `json:"interrupt,omitempty"`
	Ended        time.Time    `json:"ended,omitempty"`
	Graceful     bool         `json:"graceful"`
	Forced       bool         `json:"forced"`
	FaultPhase   string       `json:"faultPhase,omitempty"`
	Events       []host.Event `json:"events"`
}
type process struct {
	fixture          *fixture
	cmd              *exec.Cmd
	kernelHandle     win.Handle
	birth            uint64
	out, stderr      boundedCapture
	ready            chan host.Event
	done             chan struct{}
	scanned          chan struct{}
	mu               sync.Mutex
	waitErr, scanErr error
	evidence         processEvidence
}

func (f *fixture) initialize(t *testing.T) {
	t.Helper()
	if err := requireSyntheticRoot(f.root); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.executable, "init", "--config", f.configPath)
	cmd.Dir = projectRoot()
	var output, diagnostics boundedCapture
	cmd.Stdout = &output
	cmd.Stderr = &diagnostics
	if err := cmd.Run(); err != nil {
		t.Fatal("independent CLI init failed (raw output withheld)")
	}
	raw, overflow := output.snapshot()
	var e host.Event
	if overflow || json.Unmarshal(bytes.TrimSpace(raw), &e) != nil || e.Event != "initialized" {
		t.Fatal("CLI init did not confirm initialized")
	}
	if raw, _ = diagnostics.snapshot(); len(raw) != 0 {
		t.Fatal("CLI init produced diagnostics")
	}
}
func (f *fixture) start(t *testing.T) *process {
	t.Helper()
	if err := requireSyntheticRoot(f.root); err != nil {
		t.Fatal(err)
	}
	p := &process{fixture: f, ready: make(chan host.Event, 1), done: make(chan struct{}), scanned: make(chan struct{})}
	p.cmd = exec.Command(f.executable, "serve", "--config", f.configPath)
	p.cmd.Dir = projectRoot()
	p.cmd.Env = append(os.Environ(), f.environment...)
	p.cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: win.CREATE_NEW_CONSOLE | win.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	reader, writer := io.Pipe()
	p.cmd.Stdout = writer
	p.cmd.Stderr = &p.stderr
	p.evidence = processEvidence{ParentPID: os.Getpid(), BinarySHA256: f.sha, Go: runtime.Version(), Started: time.Now().UTC()}
	if err := p.cmd.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatal("independent CLI launch failed")
	}
	// Pin the launch BEFORE Wait can close os.Process's own handle. Never
	// establish target identity later from a possibly reused numerical PID.
	var actual string
	var identityErr error
	p.kernelHandle, p.birth, actual, identityErr = processIdentity(p.cmd.Process.Pid)
	if identityErr != nil || !strings.EqualFold(filepath.Clean(actual), f.executable) {
		_ = reader.Close()
		_ = writer.Close()
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		if p.kernelHandle != 0 {
			_ = win.CloseHandle(p.kernelHandle)
		}
		t.Fatal("could not pin exact launched process")
	}
	p.evidence.PID = p.cmd.Process.Pid
	p.evidence.Birth = p.birth
	go func() {
		defer close(p.scanned)
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), 16384)
		for scanner.Scan() {
			line := bytes.Clone(scanner.Bytes())
			_, _ = p.out.Write(append(line, '\n'))
			var event host.Event
			if err := json.Unmarshal(line, &event); err != nil {
				p.mu.Lock()
				p.scanErr = errors.New("invalid Host event encoding")
				p.mu.Unlock()
				continue
			}
			p.mu.Lock()
			if len(p.evidence.Events) < 4096 {
				p.evidence.Events = append(p.evidence.Events, event)
			} else {
				p.scanErr = errors.New("Host event limit reached")
			}
			p.mu.Unlock()
			if event.Event == "ready" {
				select {
				case p.ready <- event:
				default:
				}
			}
		}
		if err := scanner.Err(); err != nil {
			p.mu.Lock()
			p.scanErr = errors.New("Host event stream failed")
			p.mu.Unlock()
		}
	}()
	go func() {
		err := p.cmd.Wait()
		_ = writer.Close()
		<-p.scanned
		p.mu.Lock()
		p.waitErr = err
		p.evidence.Ended = time.Now().UTC()
		p.mu.Unlock()
		close(p.done)
	}()
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			// Test failure cleanup is deliberate forced termination, NEVER graceful
			// drain/safeToMove evidence. Join actual process end before next fixture.
			p.mu.Lock()
			p.evidence.Forced = true
			p.mu.Unlock()
			_ = p.cmd.Process.Kill()
			select {
			case <-p.done:
			case <-time.After(10 * time.Second):
				t.Error("test child did not actually terminate")
			}
		}
		if err := win.CloseHandle(p.kernelHandle); err != nil {
			t.Error("pinned process handle close failed")
		}
		if err := p.writeEvidence(); err != nil {
			t.Error(err)
		}
	})
	select {
	case endpoint := <-p.ready:
		if endpoint.PID != p.cmd.Process.Pid || endpoint.Authority == "" {
			t.Fatal("ready event does not match retained child")
		}
		p.mu.Lock()
		p.evidence.Ready = time.Now().UTC()
		p.mu.Unlock()
	case <-p.done:
		t.Fatal("Host ended before readiness (raw output withheld)")
	case <-time.After(15 * time.Second):
		t.Fatal("Host readiness timed out")
	}
	return p
}
func (p *process) endpoint() (host.Event, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, event := range p.evidence.Events {
		if event.Event == "ready" {
			return event, nil
		}
	}
	return host.Event{}, errors.New("Host has no observed endpoint")
}
func (p *process) writeEvidence() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	raw, err := json.MarshalIndent(p.evidence, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(p.fixture.root, fmt.Sprintf("process-%d.json", p.evidence.PID)), raw, 0600)
}
func (p *process) stop(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	p.evidence.Interrupt = time.Now().UTC()
	p.mu.Unlock()
	if err := interruptHost(p.kernelHandle, p.cmd.Process.Pid, p.birth, p.fixture.executable, p.fixture.root); err != nil {
		t.Fatal("native graceful interrupt failed", err)
	}
	select {
	case <-p.done:
	case <-time.After(20 * time.Second):
		t.Fatal("normal Host shutdown did not physically end")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.waitErr != nil || p.scanErr != nil {
		t.Fatal("Host shutdown/event processing failed (raw output withheld)")
	}
	stopped := false
	for _, event := range p.evidence.Events {
		if event.Event == "stopped" {
			stopped = true
		}
	}
	if !stopped {
		t.Fatal("process ended without positive stopped evidence")
	}
	p.evidence.Graceful = true
}

// Best-effort post-failure evidence collection, never a retry or a passing test.
// Its interrupt can cancel an otherwise pending ACK: record that causal boundary
// separately. An unknown owner may prevent flush; existing exact forced cleanup
// still handles that case and cannot be described as a graceful result.
func (p *process) collectFailedPipeServer() bool {
	p.mu.Lock()
	p.evidence.Interrupt = time.Now().UTC()
	p.mu.Unlock()
	if interruptHost(p.kernelHandle, p.cmd.Process.Pid, p.birth, p.fixture.executable, p.fixture.root) != nil {
		return false
	}
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.waitErr != nil || p.scanErr != nil {
		return false
	}
	for _, e := range p.evidence.Events {
		if e.Event == "stopped" {
			p.evidence.Graceful = true
			return true
		}
	}
	return false
}

type client struct {
	process          *process
	http             *http.Client
	authority, token string
}

// The sole instance may still be finishing a native disconnect. Retry only
// ERROR_PIPE_BUSY (CreateFile refused before a connection/credential exchange),
// never a handshake, identity, authorization or delivery error.
func pipeAvailable[T any](ctx context.Context, call func() (T, error)) (T, error) {
	for {
		if err := ctx.Err(); err != nil {
			var zero T
			return zero, err
		}
		value, err := call()
		// Only the bare CreateFile Win32 result is eligible. A wrapped/joined
		// error may contain later authority/delivery uncertainty and is terminal.
		code, native := err.(syscall.Errno)
		if !native || code != win.ERROR_PIPE_BUSY {
			return value, err
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			var zero T
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
}
func safePipeFailure(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	if errors.Is(err, security.ErrUnauthorized) {
		return "unauthorized"
	}
	var status win.NTStatus
	if errors.As(err, &status) {
		return fmt.Sprintf("ntstatus-%08x", uint32(status))
	}
	var code syscall.Errno
	if errors.As(err, &code) {
		return fmt.Sprintf("win32-%d", uint32(code))
	}
	return "unclassified"
}
func (p *process) connect(t *testing.T) *client {
	t.Helper()
	endpoint, err := p.endpoint()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	started := time.Now()
	token, err := pipeAvailable(ctx, func() (string, error) {
		return security.ReceiveSession(ctx, p.fixture.config.PipeName, uint32(p.cmd.Process.Pid))
	})
	if err != nil {
		diagnostic := security.DiagnosePipe(ctx, err, time.Since(started))
		matrixEvidence(t, p.fixture, fmt.Sprintf("pipe-client-failure-%d", p.cmd.Process.Pid), struct {
			PID            int
			Birth          uint64
			Started, Ended time.Time
			Diagnostic     security.PipeDiagnostic
		}{p.cmd.Process.Pid, p.birth, started, time.Now(), diagnostic})
		collectedAt := time.Now().UTC()
		graceful := p.collectFailedPipeServer()
		matrixEvidence(t, p.fixture, fmt.Sprintf("pipe-server-collection-%d", p.cmd.Process.Pid), struct {
			RequestedAt                  time.Time
			GracefulEnded                bool
			ShutdownCanCauseCancellation bool
		}{collectedAt, graceful, true})
		t.Fatal("independent native authentication failed (credential withheld)", safePipeFailure(err), diagnostic)
	}
	c := &client{process: p, http: &http.Client{Timeout: 15 * time.Second}, authority: endpoint.Authority, token: token}
	t.Cleanup(func() {
		c.http.CloseIdleConnections()
		for _, capture := range []*boundedCapture{&p.out, &p.stderr} {
			raw, overflow := capture.snapshot()
			if overflow || bytes.Contains(raw, []byte(c.token)) || bytes.Contains(raw, []byte(p.fixture.config.InstallationRoot)) {
				t.Error("Host log overflow or secret/locator leak")
			}
		}
		c.token = ""
	})
	return c
}
func (c *client) request(t *testing.T, method, path string, input, output any, want int) int {
	t.Helper()
	var raw []byte
	var err error
	if input != nil {
		raw, err = json.Marshal(input)
		if err != nil {
			t.Fatal("request schema invalid", err)
		}
	}
	req, err := http.NewRequest(method, "http://"+c.authority+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal("test request invalid")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	reply, err := c.http.Do(req)
	if err != nil {
		t.Fatal("external HTTP request failed (request withheld)")
	}
	defer reply.Body.Close()
	body, err := io.ReadAll(io.LimitReader(reply.Body, 64*protocol.RequestMaxBytes+1))
	if err != nil || len(body) > 64*protocol.RequestMaxBytes {
		t.Fatal("external response unreadable/oversized")
	}
	if want != 0 && reply.StatusCode != want {
		t.Fatalf("external %s status %d, want %d (body withheld)", method, reply.StatusCode, want)
	}
	if output != nil && json.Unmarshal(body, output) != nil {
		t.Fatal("generated response validation failed")
	}
	return reply.StatusCode
}
func (c *client) open(t *testing.T, workspaceID protocol.UUID) protocol.WorkspaceView {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	precondition, err := pipeAvailable(ctx, func() (security.PreconditionReply, error) {
		return security.ReceivePrecondition(ctx, c.process.fixture.config.PipeName, uint32(c.process.cmd.Process.Pid), security.PreconditionRequest{Version: 1, Kind: "registration", Bearer: c.token, WorkspaceID: string(workspaceID), IntendedUse: "open"})
	})
	if err != nil {
		t.Fatal("protected registration precondition failed")
	}
	var view protocol.WorkspaceView
	c.request(t, "POST", "/v1/workspaces/"+string(workspaceID)+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(precondition.RegistrationVersion)}, &view, 200)
	return view
}
func newID(t *testing.T) protocol.UUID {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	s := hex.EncodeToString(b[:])
	return protocol.UUID(s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:])
}
