package sqlite

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	driver "modernc.org/sqlite"
)

func openGate(t *testing.T, path string, mode Mode) *Handle {
	t.Helper()
	h, err := Open(context.Background(), path, mode)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func closeGate(t *testing.T, h *Handle) {
	t.Helper()
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
}

func execGate(t *testing.T, h *Handle, statement string, args ...any) {
	t.Helper()
	if _, err := h.Conn.ExecContext(context.Background(), statement, args...); err != nil {
		t.Fatal(err)
	}
}

func countGate(t *testing.T, h *Handle, id int) int {
	t.Helper()
	var count int
	if err := h.Conn.QueryRowContext(context.Background(), "SELECT count(*) FROM notes WHERE id=?", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestProfileGate(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Fatal("V003 requires the actual Windows/amd64 profile")
	}
	root := t.TempDir()
	path := filepath.Join(root, "synthetic 库 #1.sqlite")
	t.Logf("Go=%s OS=%s/%s PID=%d root=%s engine=%s VFS=win32", runtime.Version(), runtime.GOOS, runtime.GOARCH, os.Getpid(), root, EngineVersion)
	h := openGate(t, path, CreateNew)
	execGate(t, h, "CREATE TABLE notes(id INTEGER PRIMARY KEY, body TEXT NOT NULL)")
	execGate(t, h, "CREATE TABLE refs(id INTEGER PRIMARY KEY, note_id INTEGER NOT NULL REFERENCES notes(id))")
	body := "confirmed 合成 😀\r\n"
	tx, err := h.Conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO notes VALUES(1,?)", body); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	closeGate(t, h)

	t.Run("normal-close-reopen-and-errors", func(t *testing.T) {
		h := openGate(t, path, ReadWriteExisting)
		defer closeGate(t, h)
		var got string
		if err := h.Conn.QueryRowContext(context.Background(), "SELECT body FROM notes WHERE id=1").Scan(&got); err != nil || got != body {
			t.Fatalf("confirmed body changed: %q %v", got, err)
		}
		_, err := h.Conn.ExecContext(context.Background(), "INSERT INTO refs VALUES(1,999)")
		var sqliteErr *driver.Error
		if !errors.As(err, &sqliteErr) || sqliteErr.Code() != 787 {
			t.Fatalf("foreign key not enforced: %v", err)
		}
		_, err = h.Conn.ExecContext(context.Background(), "INSERT INTO notes VALUES(1,'duplicate')")
		if !errors.As(err, &sqliteErr) || sqliteErr.Code()&255 != 19 {
			t.Fatalf("constraint result missing: %v", err)
		}
		if _, err := Open(context.Background(), path, CreateNew); !errors.Is(err, os.ErrExist) {
			t.Fatalf("existing file not refused: %v", err)
		}
	})

	t.Run("fixed-uri-no-DSN-injection", func(t *testing.T) {
		u, err := fixedURI(filepath.Join(root, "x#_pragma=journal_mode(WAL);x.sqlite"), ReadWriteExisting)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(u)
		if err != nil || parsed.Query().Encode() != "mode=rw&vfs=win32" || parsed.Fragment != "" {
			t.Fatalf("path became DSN: %s %v", u, err)
		}
		for _, bad := range []string{"relative.sqlite", `\\server\share\x.sqlite`, "file:x?mode=rwc", path + "\x00"} {
			if _, err := fixedURI(bad, ReadWriteExisting); err == nil {
				t.Fatalf("accepted non-profile path: %q", bad)
			}
		}
		if _, err := fixedURI(path, Mode(99)); err == nil {
			t.Fatal("accepted arbitrary mode")
		}
	})

	t.Run("no-missing-library-seed-or-readonly-write", func(t *testing.T) {
		missing := filepath.Join(root, "missing.sqlite")
		for _, mode := range []Mode{ReadWriteExisting, ReadOnlyScratch} {
			if h, err := Open(context.Background(), missing, mode); err == nil {
				h.Close()
				t.Fatal("missing file opened")
			}
			if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing file seeded: %v", err)
			}
		}
		h := openGate(t, path, ReadOnlyScratch) // stable closed synthetic sample only
		defer closeGate(t, h)
		_, err := h.Conn.ExecContext(context.Background(), "INSERT INTO notes VALUES(90,'forbidden')")
		var sqliteErr *driver.Error
		if !errors.As(err, &sqliteErr) || sqliteErr.Code()&255 != 8 {
			t.Fatalf("readonly write result: %v", err)
		}
	})

	t.Run("existing-wal-is-rejected-not-converted", func(t *testing.T) {
		walPath := filepath.Join(root, "synthetic-wrong-profile.sqlite")
		h := openGate(t, walPath, CreateNew)
		execGate(t, h, "CREATE TABLE sample(id INTEGER)")
		var journal string
		if err := h.Conn.QueryRowContext(context.Background(), "PRAGMA journal_mode=WAL").Scan(&journal); err != nil || journal != "wal" {
			t.Fatalf("WAL fixture unavailable: %s %v", journal, err)
		}
		closeGate(t, h)
		before, err := os.ReadFile(walPath)
		if err != nil {
			t.Fatal(err)
		}
		if h, err := Open(context.Background(), walPath, ReadWriteExisting); !errors.Is(err, ErrProfile) {
			if h != nil {
				h.Close()
			}
			t.Fatalf("wrong mode accepted: %v", err)
		}
		after, err := os.ReadFile(walPath)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("existing mode/file changed: %v", err)
		}
	})

	t.Run("real-busy-result-and-clean-rollback", func(t *testing.T) {
		a := openGate(t, path, ReadWriteExisting)
		b := openGate(t, path, ReadWriteExisting)
		defer closeGate(t, a)
		defer closeGate(t, b)
		tx, err := a.Conn.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec("INSERT INTO notes VALUES(5,'not committed')"); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		_, err = b.Conn.ExecContext(context.Background(), "INSERT INTO notes VALUES(6,'blocked')")
		var sqliteErr *driver.Error
		if !errors.As(err, &sqliteErr) || sqliteErr.Code()&255 != 5 {
			t.Fatalf("busy not observable: %v", err)
		}
		t.Logf("busy result elapsed=%s code=%d", time.Since(start), sqliteErr.Code())
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if countGate(t, b, 5) != 0 || countGate(t, b, 6) != 0 {
			t.Fatal("failed/rolled-back write became visible")
		}
	})

	t.Run("fresh-process-commit", func(t *testing.T) {
		cmd := profileHelper(t, "commit", path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fresh process failed: %v %s", err, out)
		}
		t.Logf("fresh process output: %s", out)
		h := openGate(t, path, ReadWriteExisting)
		defer closeGate(t, h)
		if countGate(t, h, 2) != 1 {
			t.Fatal("new process confirmed data not visible")
		}
	})

	for _, mode := range []string{"kill-after-confirmed", "kill-before-commit"} {
		t.Run(mode, func(t *testing.T) {
			cmd := profileHelper(t, mode, path)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			done := false
			defer func() {
				if !done {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			ready := make(chan string, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					if strings.HasPrefix(scanner.Text(), "PROFILE_READY") {
						ready <- scanner.Text()
						return
					}
				}
				ready <- ""
			}()
			select {
			case line := <-ready:
				if line == "" {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					done = true
					t.Fatalf("child readiness missing: %s", &stderr)
				}
				t.Logf("%s at %s", line, time.Now().UTC().Format(time.RFC3339Nano))
			case <-time.After(30 * time.Second):
				t.Fatal("child readiness timed out")
			}
			if mode == "kill-before-commit" {
				info, err := os.Stat(path + "-journal")
				if err != nil || info.Size() == 0 {
					t.Fatalf("expected real rollback journal: %v", err)
				}
				t.Logf("pre-kill journal bytes=%d", info.Size())
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			done = true
			if err == nil {
				t.Fatal("expected abnormal process termination")
			}
			t.Logf("child PID=%d confirmed terminated: %v", cmd.Process.Pid, err)
			h := openGate(t, path, ReadWriteExisting) // explicit recovery, not inspect
			defer closeGate(t, h)
			if countGate(t, h, 3) != 1 || countGate(t, h, 4) != 0 {
				t.Fatal("kill/reopen did not preserve confirmed/reject uncommitted data")
			}
		})
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(path + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected sidecar after physical close: %s %v", suffix, err)
		}
	}
	t.Log("G1/G2 process-level checks complete; no OS-lock, Host, auth or hardware-power-loss claim")
}

func profileHelper(t *testing.T, mode, path string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProfileGateHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "ABOARD_PROFILE_HELPER="+mode, "ABOARD_SYNTHETIC_DB="+path)
	return cmd
}

// This helper is compiled only into tests and cannot be a production fault API.
func TestProfileGateHelper(t *testing.T) {
	mode := os.Getenv("ABOARD_PROFILE_HELPER")
	if mode == "" {
		return
	}
	path := os.Getenv("ABOARD_SYNTHETIC_DB")
	if !strings.Contains(strings.ToLower(filepath.ToSlash(path)), "/.data/logs/001-validation/tmp/") {
		t.Fatal("helper target is outside authorized synthetic root")
	}
	h := openGate(t, path, ReadWriteExisting)
	if mode == "commit" {
		execGate(t, h, "INSERT INTO notes VALUES(2,'new process confirmed')")
		closeGate(t, h)
		t.Logf("child PID=%d committed and closed", os.Getpid())
		return
	}
	switch mode {
	case "kill-after-confirmed":
		execGate(t, h, "INSERT INTO notes VALUES(3,'confirmed before kill')")
	case "kill-before-commit":
		execGate(t, h, "PRAGMA cache_size=2")
		tx, err := h.Conn.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec("INSERT INTO notes VALUES(4,?)", strings.Repeat("uncommitted", 32768)); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("unsupported helper mode")
	}
	fmt.Printf("PROFILE_READY mode=%s PID=%d\n", mode, os.Getpid())
	_, _ = bufio.NewReader(os.Stdin).ReadByte() // parent kills after readiness
	t.Fatal("holding helper must be explicitly terminated")
}
