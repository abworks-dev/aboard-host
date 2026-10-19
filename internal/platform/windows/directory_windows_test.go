//go:build windows && amd64

package windows

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
)

type childResult struct {
	PID      int
	Acquired bool
	Error    string
}

func syntheticDir(t *testing.T) string {
	t.Helper()
	root, e := filepath.Abs(filepath.Join("..", "..", ".."))
	if e != nil {
		t.Fatal(e)
	}
	allowed := filepath.Join(root, ".data", "logs", "001-validation")
	temp := filepath.Clean(os.TempDir())
	if !strings.EqualFold(temp, allowed) && !strings.HasPrefix(strings.ToLower(temp), strings.ToLower(allowed)+`\`) {
		t.Fatal("required project-local test TEMP/TMP missing before fixture creation")
	}
	return t.TempDir()
}

func gateRoot(t *testing.T) string {
	t.Helper()
	root := syntheticDir(t)
	if !strings.Contains(strings.ToLower(root), `\.data\logs\001-validation\`) {
		t.Fatal("synthetic roots require the project-local runner TEMP/TMP")
	}
	if err := os.MkdirAll(filepath.Join(root, ".aboard", "locks"), 0700); err != nil {
		t.Fatal(err)
	}
	l, err := Acquire(root, Workspace, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}
func mustClose(t *testing.T, l *Lease) {
	t.Helper()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
}
func readyFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func waitFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for {
		b, err := os.ReadFile(path)
		if err == nil && len(b) != 0 {
			return b
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s: %v", filepath.Base(path), err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func readResult(t *testing.T, path string) childResult {
	t.Helper()
	var result childResult
	deadline := time.Now().Add(8 * time.Second)
	for {
		if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &result) == nil && result.PID != 0 {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("child did not provide complete result: %s", filepath.Base(path))
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func childCmd(t *testing.T, root, tag, mode string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestDirectoryGateHelper$", "-test.timeout=20s", "--", root, tag, mode)
	// No shell, service, provider, or inherited business handles are involved.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd
}
func setupDB(t *testing.T, root string) {
	t.Helper()
	l, err := Acquire(root, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer mustClose(t, l)
	h, err := sqlite.Open(context.Background(), filepath.Join(root, "gate.sqlite"), sqlite.CreateNew)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Conn.ExecContext(context.Background(), "CREATE TABLE owner_writes(pid INTEGER NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
}
func writeDB(root string) error {
	h, err := sqlite.Open(context.Background(), filepath.Join(root, "gate.sqlite"), sqlite.ReadWriteExisting)
	if err != nil {
		return err
	}
	_, err = h.Conn.ExecContext(context.Background(), "INSERT INTO owner_writes(pid) VALUES (?)", os.Getpid())
	return errors.Join(err, h.Close())
}
func countDB(t *testing.T, root string) int {
	t.Helper()
	l, err := Acquire(root, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer mustClose(t, l)
	h, err := sqlite.Open(context.Background(), filepath.Join(root, "gate.sqlite"), sqlite.ReadWriteExisting)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := h.Conn.QueryRowContext(context.Background(), "SELECT count(*) FROM owner_writes").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDirectoryGatePrimitives(t *testing.T) {
	root := gateRoot(t)
	l, err := Acquire(root, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer mustClose(t, l)
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(root, Workspace, true, false); err == nil {
		t.Fatal("CREATE_NEW replaced the existing fixed object")
	}
	for _, p := range []string{filepath.Join(root, ".aboard", "locks", "writer.lock"), filepath.Join(root, ".aboard", "locks"), root} {
		if err := os.Remove(p); err == nil {
			t.Fatalf("held object deleted: %s", p)
		}
		if err := os.Rename(p, p+"-replaced"); err == nil {
			t.Fatalf("held object renamed: %s", p)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".aboard", "locks", "writer.lock"), []byte("replace"), 0600); err == nil {
		t.Fatal("fixed lock overwritten while held")
	}
	install, err := Acquire(root, Installation, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if install.LockIdentity() == l.LockIdentity() {
		t.Fatal("installation lock and workspace lock are the same object")
	}
	mustClose(t, install)
	if err := win.GetFileInformationByHandleEx(l.LockHandle(), win.FileIdInfo, (*byte)(unsafe.Pointer(&Identity{})), uint32(unsafe.Sizeof(Identity{}))); err != nil {
		t.Fatal(err)
	}
	proc := win.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation")
	for _, f := range append(append([]*File(nil), l.pins...), l.lock) {
		var flags uint32
		ok, _, e := proc.Call(uintptr(f.handle), uintptr(unsafe.Pointer(&flags)))
		if ok == 0 || flags&win.HANDLE_FLAG_INHERIT != 0 {
			t.Fatalf("inheritable/unknown handle: flags=%d error=%v", flags, e)
		}
	}
}

func TestDirectoryGateRejectedPathsAndKeys(t *testing.T) {
	root := gateRoot(t)
	for _, path := range []string{`relative`, `\\server\share\workspace`, `\\?\` + root, `E:relative`, root + `\..\x`, root + `\.\x`, root + `\NUL.txt`, root + `\x:stream`, root + `\x.`, root + "\\x ", root + "\x00"} {
		if _, err := Acquire(path, Workspace, false, false); err == nil {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}
	missing := filepath.Join(root, "missing")
	if _, err := Acquire(missing, Workspace, false, false); err == nil {
		t.Fatal("missing directory accepted")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("acquire created a missing directory")
	}
	lock := filepath.Join(root, ".aboard", "locks", "writer.lock")
	before, err := os.ReadFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(root, Workspace, true, false); err == nil {
		t.Fatal("CREATE_NEW accepted old lock")
	}
	after, err := os.ReadFile(lock)
	if err != nil || string(before) != string(after) {
		t.Fatal("existing lock changed by create refusal")
	}
	alias := filepath.Join(root, "hardlink.lock")
	if err := os.Link(lock, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(root, Workspace, false, false); err == nil {
		t.Fatal("hard-linked lock accepted")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(root, "data", "workspace.sqlite")
	readyFile(t, key, []byte("synthetic key"))
	if err := os.Link(key, filepath.Join(root, "key.alias")); err != nil {
		t.Fatal(err)
	}
	l, err := Acquire(root, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := l.PinFile(`data\workspace.sqlite`, false); err == nil {
		_ = p.Close()
		t.Fatal("hard-linked database key accepted")
	}
	mustClose(t, l)
}

func TestDirectoryGateFrozenFiles(t *testing.T) {
	root := gateRoot(t)
	readyFile(t, filepath.Join(root, "aboard.workspace.json"), []byte("synthetic manifest"))
	l, err := Acquire(root, Workspace, false, true)
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.PinFile("aboard.workspace.json", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.path, []byte("changed"), 0600); err == nil {
		t.Fatal("frozen source accepted write")
	}
	if err := os.Rename(f.path, f.path+"-renamed"); err == nil {
		t.Fatal("frozen source accepted replacement")
	}
	if _, err := Acquire(root, Workspace, false, false); err == nil {
		t.Fatal("read-only lock holder failed to exclude writer")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	mustClose(t, l)
	if _, err := Acquire(root, Workspace, true, true); err == nil {
		t.Fatal("inspect created lock")
	}
	if err := os.Remove(filepath.Join(root, ".aboard", "locks", "writer.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(root, Workspace, false, true); err == nil {
		t.Fatal("inspect accepted missing lock")
	}
	if _, err := os.Stat(filepath.Join(root, ".aboard", "locks", "writer.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inspect recreated lock")
	}
}

func TestDirectoryGateStoppedRename(t *testing.T) {
	parent := syntheticDir(t)
	original := filepath.Join(parent, "original")
	if err := os.MkdirAll(filepath.Join(original, ".aboard", "locks"), 0700); err != nil {
		t.Fatal(err)
	}
	readyFile(t, filepath.Join(original, "aboard.workspace.json"), []byte(`{"workspaceId":"10000000-0000-4000-8000-000000000001"}`))
	l, err := Acquire(original, Workspace, true, false)
	if err != nil {
		t.Fatal(err)
	}
	id := l.RootIdentity()
	moved := filepath.Join(parent, "moved")
	if err := os.Rename(original, moved); err == nil {
		t.Fatal("active root moved")
	}
	mustClose(t, l)
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	next, err := Acquire(moved, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.RootIdentity() != id {
		t.Fatal("same-volume rename changed physical identity")
	}
	mustClose(t, next)
}

// A real NTFS mount-point reparse is possible without adding an OS account,
// modifying security policy, or enabling Developer Mode. Failure is a gate
// failure, not a skipped proof. All paths are disposable synthetic directories.
func junction(t *testing.T, link, target string) {
	t.Helper()
	if err := os.Mkdir(link, 0700); err != nil {
		t.Fatal(err)
	}
	if err := markJunction(link, target); err != nil {
		t.Fatal(err)
	}
}
func markJunction(link, target string) error {
	return markJunctionAccess(link, target, win.GENERIC_WRITE)
}
func markJunctionAccess(link, target string, access uint32) error {
	p, err := win.UTF16PtrFromString(link)
	if err != nil {
		return err
	}
	h, err := win.CreateFile(p, access, 0, nil, win.OPEN_EXISTING, win.FILE_FLAG_OPEN_REPARSE_POINT|win.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer win.CloseHandle(h)
	sub, _ := win.UTF16FromString(`\??\` + target)
	printName, _ := win.UTF16FromString(target)
	buf := make([]byte, 16+2*(len(sub)+len(printName)))
	binary.LittleEndian.PutUint32(buf[0:4], win.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(buf)-8))
	binary.LittleEndian.PutUint16(buf[8:10], 0)
	binary.LittleEndian.PutUint16(buf[10:12], uint16(2*(len(sub)-1)))
	binary.LittleEndian.PutUint16(buf[12:14], uint16(2*len(sub)))
	binary.LittleEndian.PutUint16(buf[14:16], uint16(2*(len(printName)-1)))
	for i, ch := range append(sub, printName...) {
		binary.LittleEndian.PutUint16(buf[16+2*i:], ch)
	}
	var n uint32
	return win.DeviceIoControl(h, win.FSCTL_SET_REPARSE_POINT, &buf[0], uint32(len(buf)), nil, 0, &n, nil)
}
func TestDirectoryGateReparseAndCopy(t *testing.T) {
	root := gateRoot(t)
	parent := filepath.Dir(root)
	link := filepath.Join(parent, filepath.Base(root)+"-junction")
	junction(t, link, root)
	t.Cleanup(func() { _ = os.Remove(link) })
	if _, err := Acquire(link, Workspace, false, false); err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatal("reparse root not specifically rejected:", err)
	}
	child := filepath.Join(root, "child")
	if err := os.MkdirAll(filepath.Join(child, ".aboard", "locks"), 0700); err != nil {
		t.Fatal(err)
	}
	childLease, err := Acquire(child, Workspace, true, false)
	if err != nil {
		t.Fatal(err)
	}
	mustClose(t, childLease)
	if _, err := Acquire(filepath.Join(link, "child"), Workspace, false, false); err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatal("existing target beneath reparse ancestor not specifically rejected:", err)
	}
	keyRoot := gateRoot(t)
	keyLink := filepath.Join(keyRoot, ".aboard", "locks")
	realLocks := filepath.Join(keyRoot, "original-locks")
	if err := os.Rename(keyLink, realLocks); err != nil {
		t.Fatal(err)
	}
	junction(t, keyLink, realLocks)
	t.Cleanup(func() { _ = os.Remove(keyLink) })
	if _, err := Acquire(keyRoot, Workspace, false, false); err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatal("reparse lock ancestor not specifically rejected:", err)
	}
	copyRoot := gateRoot(t)
	first, err := Acquire(root, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(copyRoot, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.RootIdentity() == second.RootIdentity() {
		t.Fatal("different physical roots collapsed into one identity")
	}
	mustClose(t, first)
	mustClose(t, second)
	// Registration's UUID conflict and content-copy semantics remain V008/V012;
	// this OS adapter deliberately does not invent a logical identity authority.
}

func TestDirectoryGateCommonStability(t *testing.T) {
	root := gateRoot(t)
	if err := os.Mkdir(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	readyFile(t, filepath.Join(root, "aboard.workspace.json"), []byte("synthetic manifest"))
	readyFile(t, filepath.Join(root, "data", "workspace.sqlite"), []byte("synthetic database bytes"))
	l, err := Acquire(root, Workspace, false, true)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := l.PinFile("aboard.workspace.json", true)
	if err != nil {
		t.Fatal(err)
	}
	database, err := l.PinFile(`data\workspace.sqlite`, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, filepath.Join(root, ".aboard"), filepath.Join(root, ".aboard", "locks"), filepath.Join(root, "data"), manifest.path, database.path} {
		if err := os.Rename(path, path+"-replaced"); err == nil {
			t.Fatal("common snapshot member renamed:", path)
		}
		if path == manifest.path || path == database.path {
			if err := os.Remove(path); err == nil {
				t.Fatal("common key deleted")
			}
			if err := os.WriteFile(path, []byte("replace"), 0600); err == nil {
				t.Fatal("common key overwritten")
			}
		}
		if err := l.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Close(); err == nil {
		t.Fatal("lock released while tracked key pins remain open")
	}
	if competitor, err := Acquire(root, Workspace, false, false); err == nil {
		mustClose(t, competitor)
		t.Fatal("early close released exclusion")
	}
	target := gateRoot(t)
	for _, path := range []string{root, filepath.Join(root, "data"), filepath.Join(root, ".aboard", "locks")} {
		if err := markJunction(path, target); err == nil {
			t.Fatal("held directory mutated to reparse in place:", path)
		} else {
			t.Logf("in-place reparse rejected: component=%s error=%v", filepath.Base(path), err)
		}
		if err := l.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := manifest.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Validate(); err == nil {
		t.Fatal("released key incorrectly left a valid protected view")
	}
	if p, err := l.PinFile("aboard.workspace.json", true); err == nil {
		_ = p.Close()
		t.Fatal("closed protected view was repinned")
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	mustClose(t, l)
}

func TestDirectoryGateEmptyAncestorMutation(t *testing.T) {
	root := syntheticDir(t)
	empty := filepath.Join(root, "new-empty-lock-parent")
	if err := os.Mkdir(empty, 0700); err != nil {
		t.Fatal(err)
	}
	target := gateRoot(t)
	pins, err := pinDirectories(empty)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for i := len(pins) - 1; i >= 0; i-- {
			_ = pins[i].Close()
		}
	}()
	if err := os.Rename(empty, empty+"-moved"); err == nil {
		t.Fatal("pinned empty ancestor renamed before lock creation")
	}
	mutated := false
	for _, access := range []uint32{win.GENERIC_WRITE, win.FILE_WRITE_DATA, win.FILE_WRITE_ATTRIBUTES, 0} {
		if err := markJunctionAccess(empty, target, access); err == nil {
			mutated = true
			// Metadata-only mutation is possible despite directory sharing. The
			// required invariant is refusal/no redirected create, NOT a sandbox
			// claim that all metadata mutation is physically impossible.
			f, e := openRelativePinned(pins[len(pins)-1], "attempt.lock", win.GENERIC_READ|win.GENERIC_WRITE, 0, win.CREATE_NEW, false)
			if e == nil {
				_ = f.Close()
				t.Fatalf("G4 BLOCKED: reparse mutation allowed protected create access=%#x", access)
			}
			if _, e := os.Stat(filepath.Join(target, "attempt.lock")); !os.IsNotExist(e) {
				t.Fatalf("redirected target changed: %v", e)
			}
			t.Logf("metadata reparse access=%#x succeeded; protected create rejected: %v", access, e)
		} else {
			t.Logf("empty pinned ancestor reparse rejected: access=%#x error=%v", access, err)
		}
	}
	if !mutated {
		t.Fatal("required real metadata mutation stimulus unavailable")
	}
	if e := pins[len(pins)-1].Validate(); e == nil {
		t.Fatal("metadata-reparsed pin remained valid")
	}

}

func TestDirectoryGateValidAlias(t *testing.T) {
	root := gateRoot(t)
	p, err := win.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	var buf [32768]uint16
	n, err := win.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || n >= uint32(len(buf)) {
		t.Fatal("short-path query failed:", err)
	}
	alias := win.UTF16ToString(buf[:n])
	if strings.EqualFold(alias, root) {
		t.Log("8.3 alias not supplied by this volume; real junction alias and hard-link cases separately required")
		return
	}
	if l, err := Acquire(alias, Workspace, false, false); err == nil {
		mustClose(t, l)
		t.Fatal("valid short-name alias accepted")
	}
	t.Logf("real 8.3 alias rejected: %s", alias)
}

func TestDirectoryGateProcesses(t *testing.T) {
	for _, mode := range []string{"continuous", "simultaneous", "killed"} {
		t.Run(mode, func(t *testing.T) {
			for round := 0; round < 10; round++ {
				root := gateRoot(t)
				setupDB(t, root)
				start := time.Now()
				var childPIDs []int
				switch mode {
				case "continuous":
					l, err := Acquire(root, Workspace, false, false)
					if err != nil {
						t.Fatal(err)
					}
					cmd := childCmd(t, root, "one", "attempt")
					result := readResult(t, filepath.Join(root, "one.result"))
					childPIDs = append(childPIDs, result.PID)
					if result.Acquired || result.PID != cmd.Process.Pid {
						t.Fatal("second writer acquired held lock / wrong process")
					}
					if err := cmd.Wait(); err != nil {
						t.Fatal(err)
					}
					if err := writeDB(root); err != nil {
						t.Fatal(err)
					}
					mustClose(t, l)
					if countDB(t, root) != 1 {
						t.Fatal("failed competitor wrote data")
					}
				case "simultaneous":
					a := childCmd(t, root, "one", "barrier")
					b := childCmd(t, root, "two", "barrier")
					waitFile(t, filepath.Join(root, "one.ready"))
					waitFile(t, filepath.Join(root, "two.ready"))
					readyFile(t, filepath.Join(root, "go"), []byte("start"))
					ra, rb := readResult(t, filepath.Join(root, "one.result")), readResult(t, filepath.Join(root, "two.result"))
					childPIDs = append(childPIDs, ra.PID, rb.PID)
					if ra.PID != a.Process.Pid || rb.PID != b.Process.Pid || ra.Acquired == rb.Acquired {
						t.Fatal("not exactly one real competing writer")
					}
					readyFile(t, filepath.Join(root, "release"), []byte("done"))
					if err := a.Wait(); err != nil {
						t.Fatal(err)
					}
					if err := b.Wait(); err != nil {
						t.Fatal(err)
					}
					if countDB(t, root) != 1 {
						t.Fatal("competing writes did not equal one")
					}
				case "killed":
					cmd := childCmd(t, root, "one", "hold")
					result := readResult(t, filepath.Join(root, "one.result"))
					childPIDs = append(childPIDs, result.PID)
					if !result.Acquired || result.PID != cmd.Process.Pid {
						t.Fatal("child did not acquire")
					}
					if next, err := Acquire(root, Workspace, false, false); err == nil {
						mustClose(t, next)
						t.Fatal("live child did not hold lock")
					}
					if err := cmd.Process.Kill(); err != nil {
						t.Fatal(err)
					}
					if err := cmd.Wait(); err == nil {
						t.Fatal("kill unexpectedly exited normally")
					}
					deadline := time.Now().Add(3 * time.Second)
					var l *Lease
					var err error
					for {
						l, err = Acquire(root, Workspace, false, false)
						if err == nil || time.Now().After(deadline) {
							break
						}
						time.Sleep(5 * time.Millisecond)
					}
					if err != nil {
						t.Fatal("OS cleanup did not release fixed lock:", err)
					}
					if err := writeDB(root); err != nil {
						t.Fatal(err)
					}
					mustClose(t, l)
					if countDB(t, root) != 2 {
						t.Fatal("confirmed pre-kill row or new writer row missing")
					}
				}
				t.Logf("mode=%s round=%d parentPID=%d childPIDs=%v started=%s finished=%s elapsed=%s", mode, round, os.Getpid(), childPIDs, start.Format(time.RFC3339Nano), time.Now().Format(time.RFC3339Nano), time.Since(start))
			}
		})
	}
}

func TestDirectoryGateHelper(t *testing.T) {
	var args []string
	for i, a := range os.Args {
		if a == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		return
	}
	if len(args) != 3 {
		t.Fatal("invalid fixture arguments")
	}
	root, tag, mode := args[0], args[1], args[2]
	if !strings.Contains(strings.ToLower(root), `\.data\logs\001-validation\`) {
		t.Fatal("fixture root outside synthetic area")
	}
	if mode == "barrier" {
		readyFile(t, filepath.Join(root, tag+".ready"), []byte("ready"))
		waitFile(t, filepath.Join(root, "go"))
	}
	l, err := Acquire(root, Workspace, false, false)
	r := childResult{PID: os.Getpid(), Acquired: err == nil}
	if err != nil {
		r.Error = err.Error()
	} else if err = writeDB(root); err != nil {
		_ = l.Close()
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	readyFile(t, filepath.Join(root, tag+".result"), data)
	if l != nil {
		defer mustClose(t, l)
		if mode != "attempt" {
			waitFile(t, filepath.Join(root, "release"))
		}
	}
}

func TestDirectoryGateNonInheritance(t *testing.T) {
	root := gateRoot(t)
	if err := os.Mkdir(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	readyFile(t, filepath.Join(root, "aboard.workspace.json"), []byte("synthetic manifest"))
	readyFile(t, filepath.Join(root, "data", "workspace.sqlite"), []byte("synthetic DB"))
	l, err := Acquire(root, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := l.PinFile("aboard.workspace.json", true)
	if err != nil {
		t.Fatal(err)
	}
	database, err := l.PinFile(`data\workspace.sqlite`, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manifest.Close(); _ = database.Close(); mustClose(t, l) }()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var handles, ids []string
	files := append(append(append([]*File(nil), l.pins...), l.keys...), l.lock)
	for _, f := range files {
		// Synthetic descendants have no legitimate child startup/CWD handle;
		// avoid mistaking a separately opened shared volume/CWD for inheritance.
		if f.path == root || strings.HasPrefix(strings.ToLower(f.path), strings.ToLower(root)+`\`) {
			handles = append(handles, strconv.FormatUint(uint64(f.handle), 10))
			ids = append(ids, fmt.Sprintf("%x", f.id.FileID))
		}
	}
	args := []string{exe, "-test.run=^TestDirectoryGateInheritanceHelper$", "-test.timeout=15s", "--", root, strings.Join(handles, ","), strings.Join(ids, ",")}
	for i := range args {
		args[i] = win.EscapeArg(args[i])
	}
	line, err := win.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		t.Fatal(err)
	}
	var si win.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi win.ProcessInformation
	// inheritHandles=true exercises Windows inheritance, not Go's default child
	// handle whitelist. The lease and all ancestor pins must still not cross.
	if err := win.CreateProcess(nil, line, nil, nil, true, win.CREATE_NO_WINDOW, nil, nil, &si, &pi); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = win.TerminateProcess(pi.Process, 1)
		_, _ = win.WaitForSingleObject(pi.Process, 5000)
		_ = win.CloseHandle(pi.Thread)
		_ = win.CloseHandle(pi.Process)
	}()
	waitFile(t, filepath.Join(root, "inherit.ready"))
	if err := manifest.Close(); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	mustClose(t, l)
	readyFile(t, filepath.Join(root, "release"), []byte("release parent"))
	status, err := win.WaitForSingleObject(pi.Process, 10000)
	if err != nil || status != win.WAIT_OBJECT_0 {
		_ = win.TerminateProcess(pi.Process, 1)
		t.Fatal("inheritance child wait:", status, err)
	}
	var code uint32
	if err := win.GetExitCodeProcess(pi.Process, &code); err != nil || code != 0 {
		t.Fatal("inheritance child failed:", code, err)
	}
}
func TestDirectoryGateInheritanceHelper(t *testing.T) {
	var args []string
	for i, a := range os.Args {
		if a == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		return
	}
	if len(args) != 3 {
		t.Fatal("bad inherited child arguments")
	}
	root := args[0]
	if !strings.Contains(strings.ToLower(root), `\.data\logs\001-validation\`) {
		t.Fatal("unsafe root")
	}
	handles, ids := strings.Split(args[1], ","), strings.Split(args[2], ",")
	if len(handles) != len(ids) || len(handles) < 6 {
		t.Fatal("incomplete inheritance evidence")
	}
	for i, handle := range handles {
		h, err := strconv.ParseUint(handle, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		var id Identity
		err = win.GetFileInformationByHandleEx(win.Handle(h), win.FileIdInfo, (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id)))
		if err == nil && fmt.Sprintf("%x", id.FileID) == ids[i] {
			t.Fatal("synthetic directory/lock/key pin inherited by real child")
		}
	}
	readyFile(t, filepath.Join(root, "inherit.ready"), []byte("not inherited"))
	waitFile(t, filepath.Join(root, "release"))
	l, err := Acquire(root, Workspace, false, false)
	if err != nil {
		t.Fatal("child retained an inherited exclusion handle:", err)
	}
	mustClose(t, l)
}
