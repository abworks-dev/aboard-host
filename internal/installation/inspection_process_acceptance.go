//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"context"
	"errors"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	win "golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const inspectionProcessEnvironment = "ABOARD_ACCEPTANCE_INSPECTION_PROCESS"

// Native JOBOBJECT_BASIC_ACCOUNTING_INFORMATION; cumulative successful process
// membership is not an attempted-CreateProcess, network or Provider counter.
type inspectionJobAccounting struct {
	TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
	PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
}
type inspectionProcessEndUnknown struct {
	command *exec.Cmd
	cause   error
}

func (e *inspectionProcessEndUnknown) Error() string { return "inspection process end unknown" }
func (e *inspectionProcessEndUnknown) Unwrap() error { return platform.ErrPhysicalCloseUnknown }

type inspectionProcessObservation struct {
	ControlBefore, ControlAfter, Before, After inspectionJobAccounting
	ChildPID, ChildExit                        int
	Available, Conserved                       bool
}

func inspectionJobSnapshot() (inspectionJobAccounting, bool) {
	var v inspectionJobAccounting
	var length uint32
	err := win.QueryInformationJobObject(0, win.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&v)), uint32(unsafe.Sizeof(v)), &length)
	return v, err == nil && length == uint32(unsafe.Sizeof(v))
}
func inspectionProcessControl(c inspectionObservationConfig, id string, owner *inspectionOwner) *inspectionProcessObservation {
	v := &inspectionProcessObservation{ChildExit: -1}
	image, err := os.Executable()
	if err != nil || filepath.Dir(image) != filepath.Dir(c.Fixture) || filepath.Ext(image) != ".exe" {
		return v
	}
	var ok bool
	v.ControlBefore, ok = inspectionJobSnapshot()
	if !ok || v.ControlBefore.TotalProcesses != 1 || v.ControlBefore.ActiveProcesses != 1 {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// A fixed invalid invocation of this SAME binary: existing CLI returns2 before
	// config/Host/credentials. No new command, data mutation or arbitrary executable.
	file, e := os.OpenFile(filepath.Join(c.Fixture, "inspection-process-"+id+".txt"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return v
	}
	defer func() {
		if !closeInspectionObservation(owner, file) {
			v.Available = false
		}
	}()
	cmd := exec.CommandContext(ctx, image, "--acceptance-invalid-process-control")
	cmd.Stdin = file
	cmd.Stdout = file
	cmd.Stderr = file
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS: no console, never BREAKAWAY
	if err = cmd.Start(); err != nil {
		return v
	}
	v.ChildPID = cmd.Process.Pid
	waitErr := cmd.Wait() // actual Wait even if timeout killed this exact os.Process
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		owner.failure = errors.Join(owner.failure, &inspectionProcessEndUnknown{command: cmd, cause: waitErr})
		return v
	}
	var exit *exec.ExitError
	if !errors.As(waitErr, &exit) {
		return v
	}
	v.ChildExit = cmd.ProcessState.ExitCode()
	v.ControlAfter, ok = inspectionJobSnapshot()
	for ok && v.ControlAfter.TotalProcesses == v.ControlBefore.TotalProcesses+1 && v.ControlAfter.ActiveProcesses == 2 {
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return v
		case <-timer.C:
		}
		v.ControlAfter, ok = inspectionJobSnapshot()
	}
	v.Available = ok && ctx.Err() == nil && v.ChildPID > 0 && v.ChildPID != os.Getpid() && v.ChildExit == 2 && v.ControlAfter.TotalProcesses == v.ControlBefore.TotalProcesses+1 && v.ControlAfter.ActiveProcesses == 1
	return v
}
