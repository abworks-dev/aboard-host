//go:build windows && amd64

package acceptance

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	win "golang.org/x/sys/windows"
)

var (
	consoleDLL           = win.NewLazySystemDLL("kernel32.dll")
	attachConsole        = consoleDLL.NewProc("AttachConsole")
	freeConsole          = consoleDLL.NewProc("FreeConsole")
	setConsoleHandler    = consoleDLL.NewProc("SetConsoleCtrlHandler")
	generateConsoleEvent = consoleDLL.NewProc("GenerateConsoleCtrlEvent")
	consoleProcessList   = consoleDLL.NewProc("GetConsoleProcessList")
)

func processIdentity(pid int) (win.Handle, uint64, string, error) {
	h, err := win.OpenProcess(win.PROCESS_QUERY_LIMITED_INFORMATION|win.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return 0, 0, "", errors.New("target process unavailable")
	}
	var created, ended, kernel, user win.Filetime
	if err = win.GetProcessTimes(h, &created, &ended, &kernel, &user); err != nil {
		_ = win.CloseHandle(h)
		return 0, 0, "", errors.New("target process time unavailable")
	}
	var buffer [32768]uint16
	n := uint32(len(buffer))
	if err = win.QueryFullProcessImageName(h, 0, &buffer[0], &n); err != nil {
		_ = win.CloseHandle(h)
		return 0, 0, "", errors.New("target image unavailable")
	}
	return h, uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), win.UTF16ToString(buffer[:n]), nil
}
func interruptHost(h win.Handle, pid int, birth uint64, image, root string) error {
	if h == 0 || pid <= 0 || birth == 0 || requireSyntheticRoot(root) != nil {
		return errors.New("invalid synthetic process target")
	}
	if _, err := checkedExecutable(image); err != nil {
		return err
	}
	if state, _ := win.WaitForSingleObject(h, 0); state != uint32(win.WAIT_TIMEOUT) {
		return errors.New("target already ended")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAcceptanceConsoleSignalHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "ABOARD_ACCEPTANCE_SIGNAL_PID="+strconv.Itoa(pid), "ABOARD_ACCEPTANCE_SIGNAL_BIRTH="+strconv.FormatUint(birth, 10), "ABOARD_ACCEPTANCE_SIGNAL_IMAGE="+image, "ABOARD_ACCEPTANCE_SIGNAL_ROOT="+root)
	var diagnostics boundedCapture
	cmd.Stdout = &diagnostics
	cmd.Stderr = &diagnostics
	if err := cmd.Run(); err != nil {
		return errors.New("isolated native signal helper failed (output withheld)")
	}
	return nil
}

// This helper is test-only. Its environment carries no bearer or key. It pins
// the exact still-live kernel process + creation time/image before attaching,
// and verifies its console has no unrelated process before broadcasting BREAK.
func TestAcceptanceConsoleSignalHelper(t *testing.T) {
	pidText := os.Getenv("ABOARD_ACCEPTANCE_SIGNAL_PID")
	if pidText == "" {
		t.Skip("signal helper subprocess only")
	}
	root, image := os.Getenv("ABOARD_ACCEPTANCE_SIGNAL_ROOT"), os.Getenv("ABOARD_ACCEPTANCE_SIGNAL_IMAGE")
	if requireSyntheticRoot(root) != nil {
		t.Fatal("signal helper synthetic scope rejected")
	}
	if _, err := checkedExecutable(image); err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil || pid <= 0 || pid == os.Getpid() {
		t.Fatal("invalid target PID")
	}
	expected, err := strconv.ParseUint(os.Getenv("ABOARD_ACCEPTANCE_SIGNAL_BIRTH"), 10, 64)
	if err != nil {
		t.Fatal("invalid process birth")
	}
	h, birth, actual, err := processIdentity(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer win.CloseHandle(h)
	if birth != expected || !strings.EqualFold(filepath.Clean(actual), image) {
		t.Fatal("target identity changed")
	}
	if state, _ := win.WaitForSingleObject(h, 0); state != uint32(win.WAIT_TIMEOUT) {
		t.Fatal("target ended")
	}
	// A child may inherit the test runner's console. Detach only this helper,
	// never the parent or Host; AttachConsole resets the helper handler table.
	_, _, _ = freeConsole.Call()
	ok, _, _ := attachConsole.Call(uintptr(uint32(pid)))
	if ok == 0 {
		t.Fatal("target isolated console unavailable")
	}
	defer func() {
		ok, _, _ := freeConsole.Call()
		if ok == 0 {
			t.Error("helper console detach failed")
		}
	}()
	acknowledged := make(chan struct{}, 1)
	handler := syscall.NewCallback(func(kind uint32) uintptr {
		if kind == 1 {
			select {
			case acknowledged <- struct{}{}:
			default:
			}
			return 1
		}
		return 0
	}) // CTRL_BREAK_EVENT
	ok, _, _ = setConsoleHandler.Call(handler, 1)
	if ok == 0 {
		t.Fatal("helper native handler unavailable")
	}
	var members [8]uint32
	count, _, _ := consoleProcessList.Call(uintptr(unsafe.Pointer(&members[0])), uintptr(len(members)))
	if count != 2 {
		t.Fatal("console contains unexpected process count")
	}
	seenTarget, seenSelf := false, false
	for _, member := range members[:count] {
		if member == uint32(pid) {
			seenTarget = true
		} else if member == uint32(os.Getpid()) {
			seenSelf = true
		} else {
			t.Fatal("unrelated process in target console")
		}
	}
	if !seenTarget || !seenSelf {
		t.Fatal("target/helper console membership not proven")
	}
	// NEW_PROCESS_GROUP is ignored alongside NEW_CONSOLE. Group 0 is safe ONLY
	// after the exact two-member isolated-console proof above; PID is not group ID.
	if state, _ := win.WaitForSingleObject(h, 0); state != uint32(win.WAIT_TIMEOUT) {
		t.Fatal("target ended before interrupt")
	}
	ok, _, _ = generateConsoleEvent.Call(1, 0)
	if ok == 0 {
		t.Fatal("native interrupt delivery failed")
	}
	select {
	case <-acknowledged:
	case <-time.After(2 * time.Second):
		t.Fatal("helper did not handle its own asynchronous BREAK")
	}
	// Completion is deliberately checked by the parent against actual process
	// Wait and positive stopped event, not assumed from GenerateConsoleCtrlEvent.
}
