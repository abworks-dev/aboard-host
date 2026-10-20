//go:build windows && amd64

package acceptance

import (
	win "golang.org/x/sys/windows"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

type observedJobAccounting struct {
	TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
	PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
}
type observedInspectionProcesses struct {
	ControlBefore, ControlAfter, Before, After observedJobAccounting
	ChildPID, ChildExit                        int
	Available, Conserved                       bool
}

var inspectionJobMembership = win.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

type inspectionProcessWatch struct {
	job      win.Handle
	before   observedJobAccounting
	assigned bool
	drained  bool
	once     sync.Once
}

func sameObservationJob(process, job win.Handle) bool {
	var member uint32
	ok, _, _ := inspectionJobMembership.Call(uintptr(process), uintptr(job), uintptr(unsafe.Pointer(&member)))
	return ok != 0 && member != 0
}
func jobSnapshot(t *testing.T, h win.Handle) observedJobAccounting {
	t.Helper()
	var value observedJobAccounting
	if unsafe.Sizeof(value) != 48 {
		t.Fatal("job accounting ABI size")
	}
	var length uint32
	if err := win.QueryInformationJobObject(h, win.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&value)), uint32(unsafe.Sizeof(value)), &length); err != nil || length != uint32(unsafe.Sizeof(value)) {
		t.Fatal("job accounting unavailable")
	}
	return value
}
func startInspectionProcessWatch(t *testing.T, p *process) *inspectionProcessWatch {
	t.Helper()
	// The launch-time handle remains pinned. This second handle needs assignment
	// rights, and must identify that SAME birth/image before any assignment.
	h, err := win.OpenProcess(win.PROCESS_SET_QUOTA|win.PROCESS_TERMINATE|win.PROCESS_QUERY_LIMITED_INFORMATION|win.SYNCHRONIZE, false, uint32(p.cmd.Process.Pid))
	if err != nil {
		t.Fatal("assignment handle unavailable")
	}
	defer func() {
		if win.CloseHandle(h) != nil {
			t.Error("assignment handle close unknown")
		}
	}()
	var born, end, kernel, user win.Filetime
	var image [32768]uint16
	length := uint32(len(image))
	state, waitErr := win.WaitForSingleObject(p.kernelHandle, 0)
	if waitErr != nil || state != uint32(win.WAIT_TIMEOUT) || win.GetProcessTimes(h, &born, &end, &kernel, &user) != nil || uint64(born.HighDateTime)<<32|uint64(born.LowDateTime) != p.birth || win.QueryFullProcessImageName(h, 0, &image[0], &length) != nil || !strings.EqualFold(win.UTF16ToString(image[:length]), p.fixture.executable) {
		t.Fatal("assignment target not original live child")
	}
	job, err := win.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal("unnamed observation job unavailable")
	}
	watch := &inspectionProcessWatch{job: job}
	t.Cleanup(func() { watch.cleanup(t, p) })
	// Verify default flags; never enable breakaway, active-process limits or kill-on-close.
	var limits win.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err = win.QueryInformationJobObject(job, win.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil || limits.BasicLimitInformation.LimitFlags != 0 {
		t.Fatal("observation job has unexpected restrictions/breakaway")
	}
	if err = win.AssignProcessToJobObject(job, h); err != nil {
		t.Fatal("native job assignment unavailable; no fallback")
	}
	watch.assigned = true
	if !sameObservationJob(h, job) {
		t.Fatal("exact Host membership not established")
	}
	watch.before = jobSnapshot(t, job)
	if watch.before.TotalProcesses != 1 || watch.before.ActiveProcesses != 1 {
		t.Fatal("job baseline is not exactly original Host")
	}
	return watch
}
func (w *inspectionProcessWatch) cleanup(t *testing.T, p *process) {
	w.once.Do(func() {
		var v observedJobAccounting
		query := func() bool {
			return win.QueryInformationJobObject(w.job, win.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&v)), uint32(unsafe.Sizeof(v)), nil) == nil
		}
		if !query() {
			t.Error("job cleanup accounting unavailable; retain handle")
			return
		}
		if v.ActiveProcesses != 0 {
			// Failure cleanup only, NEVER normal stop/acceptance. Retain job through
			// actual original Host Wait and complete descendant accounting.
			t.Error("forced observation-job failure cleanup")
			select {
			case <-p.done:
			default:
				p.mu.Lock()
				p.evidence.Forced = true
				p.mu.Unlock()
			}
			if win.TerminateJobObject(w.job, 3) != nil {
				t.Error("job termination unknown; retain handle")
				return
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				if !query() {
					t.Error("job end unavailable; retain handle")
					return
				}
				if v.ActiveProcesses == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Error("job not physically empty; retain handle")
					return
				}
				timer := time.NewTimer(10 * time.Millisecond)
				<-timer.C
			}
		}
		if w.assigned {
			select {
			case <-p.done:
			case <-time.After(10 * time.Second):
				t.Error("original Host Wait not joined; retain job")
				return
			}
		}
		w.drained = true // proved no live members and original actual Wait before fixture mutation
		if win.CloseHandle(w.job) != nil {
			t.Error("observation job close unknown")
		}
	})
}
func (w *inspectionProcessWatch) stopped(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		v := jobSnapshot(t, w.job)
		if v.ActiveProcesses == 0 {
			if v.TotalProcesses != 2 {
				t.Fatal("unexpected lifetime job total")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("normal Host exit left job active")
		}
		timer := time.NewTimer(10 * time.Millisecond)
		<-timer.C
	}
}
func (w *inspectionProcessWatch) verify(t *testing.T, p *process, value *observedInspectionProcesses) {
	t.Helper()
	after := jobSnapshot(t, w.job)
	if value == nil || !value.Available || !value.Conserved || value.ChildPID <= 0 || value.ChildPID == p.cmd.Process.Pid || value.ChildExit != 2 {
		t.Fatal("missing genuine child control")
	}
	if after.TotalProcesses != w.before.TotalProcesses+1 || after.ActiveProcesses != 1 {
		t.Fatal("inspection created unexpected successful descendants")
	}
	if value.ControlBefore.TotalProcesses != w.before.TotalProcesses || value.ControlBefore.ActiveProcesses != 1 || value.ControlAfter.TotalProcesses != after.TotalProcesses || value.ControlAfter.ActiveProcesses != 1 || value.Before.TotalProcesses != after.TotalProcesses || value.After.TotalProcesses != after.TotalProcesses || value.Before.ActiveProcesses != 1 || value.After.ActiveProcesses != 1 {
		t.Fatal("Host and independent job observations disagree")
	}
}
