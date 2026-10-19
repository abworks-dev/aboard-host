//go:build windows && amd64

package inspect

import (
	"errors"
	win "golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

type processIO struct{ Reads, Writes, Other, ReadBytes, WriteBytes, OtherBytes uint64 }

var processIOProc = win.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessIoCounters")

func ioSnapshot(t *testing.T) processIO {
	t.Helper()
	var c processIO
	r, _, err := processIOProc.Call(uintptr(win.CurrentProcess()), uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		t.Fatal(err)
	}
	return c
}
func checkScratchConservation(t *testing.T, before, after processIO, r Report) {
	t.Helper()
	if after.Writes-before.Writes != r.scratchWrites || after.WriteBytes-before.WriteBytes != r.scratchBytes {
		t.Fatalf("PID %d IO conservation failed: actual writes/bytes=%d/%d; independently known scratch lower bound=%d/%d (no subtraction for noise)", os.Getpid(), after.Writes-before.Writes, after.WriteBytes-before.WriteBytes, r.scratchWrites, r.scratchBytes)
	}
	t.Logf("PID=%d OS writes/bytes delta=%d/%d matches scratch-only lower bound; not a failed-attempt/path trace", os.Getpid(), r.scratchWrites, r.scratchBytes)
}
func TestInspectionGateIOCounterControls(t *testing.T) {
	root, _ := inspectionFixture(t)
	path := filepath.Join(root, "same-bytes.bin")
	data := make([]byte, 64*1024)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{1, 65535, 65536} {
		before := ioSnapshot(t)
		n, writeErr := f.WriteAt(data[:size], 0)
		after := ioSnapshot(t)
		if writeErr != nil || n != size {
			t.Fatal(n, writeErr)
		}
		if after.Writes-before.Writes != 1 || after.WriteBytes-before.WriteBytes != uint64(size) {
			t.Fatalf("unusable PID counter control: %+v %+v", before, after)
		}
		t.Logf("nontruncate existing same-byte control size=%d, kernel writes=1 bytes=%d", size, size)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	before := ioSnapshot(t)
	var n uint32
	writeErr := win.WriteFile(win.Handle(ro.Fd()), data[:1], &n, nil)
	after := ioSnapshot(t)
	if !errors.Is(writeErr, win.ERROR_ACCESS_DENIED) || n != 0 {
		t.Fatal("failed-write stimulus", n, writeErr)
	}
	t.Logf("readonly failed WriteFile: error=%v counter writes/bytes delta=%d/%d; coverage is measured, not assumed", writeErr, after.Writes-before.Writes, after.WriteBytes-before.WriteBytes)
}
