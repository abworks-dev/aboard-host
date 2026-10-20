//go:build windows && amd64

package acceptance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	win "golang.org/x/sys/windows"
)

type faultInput struct {
	Fixture, Image, Phase string
	OperationID           string
	PID                   int
	Birth                 uint64
}

// No production fault endpoint/environment is added. This independently gated
// test process can terminate only its parent's pinned synthetic Host identity.
// The initial stimulus is process termination, NOT a power-loss/FSYNC simulation.
func TestAcceptanceFaultHelper(t *testing.T) {
	encoded := os.Getenv("ABOARD_ACCEPTANCE_FAULT")
	if encoded == "" {
		t.Skip("fault helper subprocess only")
	}
	if len(encoded) > 16384 {
		t.Fatal("fault fixture too large")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal("fault fixture malformed")
	}
	var in faultInput
	if json.Unmarshal(raw, &in) != nil || requireSyntheticRoot(in.Fixture) != nil || in.PID <= 0 || in.PID == os.Getpid() || in.Birth == 0 || !faultPhaseAllowed(in) {
		t.Fatal("fault fixture rejected")
	}
	if _, err = checkedExecutable(in.Image); err != nil {
		t.Fatal("fault executable rejected")
	}
	pinned, birth, image, err := processIdentity(in.PID)
	if err != nil {
		t.Fatal("fault process unavailable")
	}
	defer win.CloseHandle(pinned)
	if birth != in.Birth || !strings.EqualFold(filepath.Clean(image), in.Image) {
		t.Fatal("fault process identity changed")
	}
	if state, _ := win.WaitForSingleObject(pinned, 0); state != uint32(win.WAIT_TIMEOUT) {
		t.Fatal("fault target already ended")
	}
	// Keep the query handle pinned while opening the terminating handle: PID
	// recycling cannot redirect the second OpenProcess to a replacement instance.
	target, err := win.OpenProcess(win.PROCESS_TERMINATE|win.SYNCHRONIZE, false, uint32(in.PID))
	if err != nil {
		t.Fatal("fault native process access refused")
	}
	defer win.CloseHandle(target)
	if err = win.TerminateProcess(target, 137); err != nil {
		t.Fatal("fault native termination failed")
	}
	state, err := win.WaitForSingleObject(target, 5000)
	if err != nil || state != win.WAIT_OBJECT_0 {
		t.Fatal("fault process end not confirmed")
	}
}
func (p *process) terminateKnownIdle(t *testing.T) { p.terminateAt(t, "known-idle-termination", "") }
func (p *process) terminateAt(t *testing.T, phase, key string) {
	t.Helper()
	if requireSyntheticRoot(p.fixture.root) != nil {
		t.Fatal("invalid fault root")
	}
	in := faultInput{Fixture: p.fixture.root, Image: p.fixture.executable, Phase: phase, OperationID: key, PID: p.cmd.Process.Pid, Birth: p.birth}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	helper := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAcceptanceFaultHelper$", "-test.count=1")
	helper.Env = append(os.Environ(), "ABOARD_ACCEPTANCE_FAULT="+base64.RawURLEncoding.EncodeToString(raw))
	var output boundedCapture
	helper.Stdout = &output
	helper.Stderr = &output
	p.mu.Lock()
	p.evidence.Forced = true
	p.evidence.FaultPhase = in.Phase
	p.mu.Unlock()
	if err = helper.Run(); err != nil {
		t.Fatal("synthetic fault helper failed (output withheld)")
	}
	select {
	case <-p.done:
	case <-time.After(8 * time.Second):
		t.Fatal("terminated child has not actually ended")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.waitErr == nil {
		t.Fatal("forced termination was misreported successful")
	}
	for _, event := range p.evidence.Events {
		if event.Event == "stopped" {
			t.Fatal("forced termination falsely claimed graceful stop")
		}
	}
}
func TestIndependentFaultHelperControllability(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	p := f.start(t)
	c := p.connect(t)
	var identity protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &identity, 200)
	p.terminateKnownIdle(t)
	lease, err := platform.Acquire(f.config.InstallationRoot, platform.Installation, false, false)
	if err != nil {
		t.Fatal("actual killed process still owns exclusion")
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	next := f.start(t)
	d := next.connect(t)
	var after protocol.PrincipalView
	d.request(t, "GET", "/v1/context", nil, &after, 200)
	if after.InstallationId != identity.InstallationId || after.PrincipalId != identity.PrincipalId {
		t.Fatal("synthetic termination replaced durable identity")
	}
	next.stop(t)
	t.Log("separate guarded fault process, known-idle termination, actual child exit and new Host authentication; NOT transaction-phase crash or hardware power-loss evidence")
}
