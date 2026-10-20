//go:build windows && amd64

package acceptance

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	win "golang.org/x/sys/windows"
)

func TestHarnessPipeRetryNeverRepeatsUncertainDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	value, err := pipeAvailable(ctx, func() (int, error) {
		calls++
		if calls < 3 {
			return 0, win.ERROR_PIPE_BUSY
		}
		return 7, nil
	})
	if err != nil || value != 7 || calls != 3 {
		t.Fatal("pre-connection busy did not retry boundedly")
	}
	deliveryUnknown := errors.New("synthetic uncertain delivery")
	calls = 0
	_, err = pipeAvailable(ctx, func() (int, error) { calls++; return 0, deliveryUnknown })
	if !errors.Is(err, deliveryUnknown) || calls != 1 {
		t.Fatal("uncertain delivery retried")
	}
	for _, failure := range []error{errors.Join(win.ERROR_PIPE_BUSY, security.ErrUnauthorized), errors.Join(win.ERROR_PIPE_BUSY, context.DeadlineExceeded), errors.Join(win.ERROR_PIPE_BUSY, deliveryUnknown)} {
		calls = 0
		_, got := pipeAvailable(ctx, func() (int, error) {
			calls++
			if calls == 1 {
				return 0, failure
			}
			return 7, nil
		})
		if got != failure || calls != 1 {
			t.Fatal("mixed authority/delivery error retried")
		}
	}
	exhausted, finish := context.WithTimeout(context.Background(), 30*time.Millisecond)
	_, err = pipeAvailable(exhausted, func() (int, error) { return 0, win.ERROR_PIPE_BUSY })
	finish()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("busy retry escaped original deadline")
	}
	cancel()
	calls = 0
	_, err = pipeAvailable(ctx, func() (int, error) { calls++; return 1, nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled connection attempted")
	}
}

func TestHarnessRejectsActualReparseAncestors(t *testing.T) {
	if err := os.MkdirAll(logRoot(), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(logRoot(), "t023-guards-")
	if err != nil {
		t.Fatal(err)
	}
	if requireSyntheticRoot(root) != nil {
		t.Fatal("guard fixture not owned")
	}
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "t023-link")
	if err = os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(target, "synthetic.exe"), []byte("not an executable; guard only"), 0600); err != nil {
		t.Fatal(err)
	}
	// A junction requires no symlink privilege; both target and link are inside
	// this test's own synthetic root. Never create or traverse a real-data alias.
	cmd := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target)
	var output boundedCapture
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Run(); err != nil {
		t.Fatal("synthetic junction control failed (output withheld)")
	}
	t.Cleanup(func() {
		if !within(root, link) || requireSyntheticRoot(root) != nil {
			t.Error("unchecked cleanup target")
			return
		}
		if err := os.Remove(link); err != nil {
			t.Error("test junction did not remove")
			return
		}
		if err := os.RemoveAll(root); err != nil {
			t.Error("test guard fixture did not remove")
		}
	})
	if requireSyntheticRoot(link) == nil {
		t.Fatal("reparse synthetic-root alias accepted")
	}
	if _, err = checkedExecutable(filepath.Join(link, "synthetic.exe")); err == nil {
		t.Fatal("reparse executable ancestry accepted")
	}
}

func TestIndependentConsoleHelperRefusesWrongLaunchIdentity(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	p := f.start(t)
	c := p.connect(t)
	if err := interruptHost(p.kernelHandle, p.cmd.Process.Pid, p.birth+1, f.executable, f.root); err == nil {
		t.Fatal("wrong creation FILETIME was permitted")
	}
	fake := filepath.Join(f.root, "wrong-image.exe")
	if err := os.WriteFile(fake, []byte("not the owned Host image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := interruptHost(p.kernelHandle, p.cmd.Process.Pid, p.birth, fake, f.root); err == nil {
		t.Fatal("wrong kernel image was permitted")
	}
	select {
	case <-p.done:
		t.Fatal("refused helper killed the live Host")
	default:
	}
	var identity protocol.PrincipalView
	c.request(t, "GET", "/v1/context", nil, &identity, 200)
	p.stop(t)
	t.Log("wrong image/birth refused before console signal; original actual Host remained responsive then normally drained")
}
