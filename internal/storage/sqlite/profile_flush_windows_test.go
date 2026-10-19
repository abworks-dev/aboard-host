//go:build windows && amd64

package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
	"modernc.org/libc"
)

// TestProfileGateFlushFailureContract is a required dependency gate. Its
// original failure is retained in the V003 evidence, not skipped after the fix.
// It does not modify dependencies or simulate hardware loss. A real read-only
// handle makes FlushFileBuffers fail without changing ACLs or system policy.
func TestProfileGateFlushFailureContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-readonly-handle.bin")
	if err := os.WriteFile(path, []byte("synthetic flush negative"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path) // read-only access, not the read-only file attribute
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	nativeErr := windows.FlushFileBuffers(windows.Handle(f.Fd()))
	if !errors.Is(nativeErr, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("required native error stimulus unavailable: %v", nativeErr)
	}
	tls := libc.NewTLS()
	defer tls.Close()
	got := libc.XFlushFileBuffers(tls, f.Fd())
	t.Logf("real native FlushFileBuffers error=%v; libc BOOL=%d; SQLite VFS success predicate=(BOOL != 0)", nativeErr, got)
	if got != 0 {
		t.Fatalf("G2 BLOCKED: failed native flush returned nonzero BOOL=%d, which the pinned win32 VFS classifies SQLITE_OK; no dependency integration is permitted", got)
	}
}
