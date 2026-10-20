//go:build windows && amd64

package acceptance

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	win "golang.org/x/sys/windows"
)

const initializerExecutableEnvironment = "ABOARD_ACCEPTANCE_SQLITE_HELPER_EXE"
const initializerEnvironment = "ABOARD_ACCEPTANCE_INITIALIZER"

type initializerCheckpoint struct {
	Version, PID int
	Kind         string
	Stage        int
	Birth        uint64
}

// This is a source fixture constructor, not a production Host command. The
// separate helper owns the SAME native adapter and real unfinished SQLite Tx.
func interruptExternalInitialization(t *testing.T, f *fixture, root, kind string, stage int) initializerCheckpoint {
	t.Helper()
	image := os.Getenv(initializerExecutableEnvironment)
	if image == "" {
		t.Skip("real SQLite initializer executable not configured")
	}
	if _, err := checkedExecutable(image); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(struct {
		Fixture, Root, Kind string
		Stage               int
	}{f.root, root, kind, stage})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, image, "-test.run=^TestAcceptanceInitializerHelper$", "-test.count=1", "-test.v")
	cmd.Dir = projectRoot()
	cmd.Env = append(os.Environ(), initializerEnvironment+"="+base64.RawURLEncoding.EncodeToString(input))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("initializer stdout unavailable")
	}
	inputPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("initializer permit unavailable")
	}
	defer inputPipe.Close()
	var diagnostics boundedCapture
	cmd.Stderr = &diagnostics
	if err = cmd.Start(); err != nil {
		t.Fatal("initializer did not start")
	}
	handle, birth, imagePath, identityErr := processIdentity(cmd.Process.Pid)
	ended := make(chan error, 1)
	go func() { ended <- cmd.Wait() }()
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			select {
			case <-ended:
			case <-time.After(8 * time.Second):
				t.Error("FAILED initializer cleanup end unproved")
			}
		}
		if handle != 0 {
			if e := win.CloseHandle(handle); e != nil {
				t.Error("initializer process pin close failed")
			}
		}
	}()
	if identityErr != nil || birth == 0 || !strings.EqualFold(imagePath, image) {
		t.Fatal("initializer launch identity rejected")
	}
	ready := make(chan initializerCheckpoint, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024), 8192)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "ABOARD_INITIALIZER_READY") {
				var point initializerCheckpoint
				if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "ABOARD_INITIALIZER_READY"))), &point) == nil {
					ready <- point
					return
				}
			}
		}
		ready <- initializerCheckpoint{}
	}()
	if _, err = inputPipe.Write([]byte{1}); err != nil {
		t.Fatal("initializer permit delivery failed")
	}
	var checkpoint initializerCheckpoint
	select {
	case checkpoint = <-ready:
	case <-ctx.Done():
		t.Fatal("initializer checkpoint timeout")
	case <-ended:
		waited = true
		t.Fatal("initializer exited before checkpoint (output withheld)")
	}
	if checkpoint.Version != 1 || checkpoint.PID != cmd.Process.Pid || checkpoint.Kind != kind || checkpoint.Stage != stage {
		t.Fatal("initializer actual stage identity absent")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal("initializer exact process termination failed")
	}
	select {
	case err = <-ended:
		waited = true
	case <-ctx.Done():
		t.Fatal("initializer physical end unproved")
	}
	if err == nil || ctx.Err() != nil {
		t.Fatal("initializer interruption not established")
	}
	checkpoint.Birth = birth
	matrixEvidence(t, f, "actual-initialization-"+kind+"-"+strconv.Itoa(stage), checkpoint)
	return checkpoint
}

func TestV009IndependentInterruptedInstallationInitialization(t *testing.T) {
	for stage := 0; stage <= 6; stage++ {
		t.Run(string(rune('0'+stage)), func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			checkpoint := interruptExternalInitialization(t, f, f.config.InstallationRoot, "installation", stage)
			before := tree(t, f.config.InstallationRoot)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			observed := runObservedCLI(ctx, f, "serve")
			if ctx.Err() != nil || !observed.IdentityProven || !observed.Unavailable || !observed.SafeOutput || observed.OutputBytes != 0 {
				t.Fatal("interrupted installation accepted or unsafe", observed)
			}
			if !reflect.DeepEqual(before, tree(t, f.config.InstallationRoot)) {
				t.Fatal("existing startup repaired unfinished initialization")
			}
			matrixEvidence(t, f, "interrupted-installation-refusal", struct {
				Checkpoint initializerCheckpoint
				Startup    cliObservation
			}{checkpoint, observed})
		})
	}
}
