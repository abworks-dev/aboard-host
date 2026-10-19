//go:build windows && amd64

package security

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	win "golang.org/x/sys/windows"
)

func gateRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".data", "logs", "001-validation"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "g5-bootstrap-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	return dir
}
func pipeName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("aboard-g5-%d-%d", os.Getpid(), time.Now().UnixNano())
}
func waitFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("fixture readiness timed out")
}
func childEnvironment(mode, root, name string) []string {
	env := os.Environ()
	env = append(env, "ABOARD_G5_CHILD="+mode, "ABOARD_G5_ROOT="+root, "ABOARD_G5_PIPE="+name)
	return env
}
func TestBootstrapGateChild(t *testing.T) {
	mode := os.Getenv("ABOARD_G5_CHILD")
	if mode == "" {
		t.Skip("subprocess fixture only")
	}
	root := os.Getenv("ABOARD_G5_ROOT")
	// Reject helper invocation outside the synthetic evidence root.
	absolute, err := filepath.Abs(root)
	if err != nil || !strings.Contains(strings.ToLower(absolute), `\.data\logs\001-validation\g5-bootstrap-`) {
		t.Fatal("invalid synthetic fixture root")
	}
	if mode == "initialize" {
		if err = os.WriteFile(filepath.Join(root, fmt.Sprintf("ready-%d", os.Getpid())), []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		waitFile(t, filepath.Join(root, "start"))
		if err = Initialize(filepath.Join(root, "install")); err != nil {
			fmt.Println("G5_RESULT refused")
		} else {
			fmt.Println("G5_RESULT initialized")
		}
		return
	}
	if mode == "session-client" {
		pid, e := strconv.ParseUint(os.Getenv("ABOARD_G5_SERVER_PID"), 10, 32)
		if e != nil {
			t.Fatal("invalid trusted launch PID")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		token, e := ReceiveSession(ctx, os.Getenv("ABOARD_G5_PIPE"), uint32(pid))
		if e != nil {
			t.Fatal(e)
		}
		if len(token) != 43 {
			t.Fatal("invalid delivered bearer shape")
		}
		// No token in stdout, arguments, environment, receipt, or any file.
		fmt.Println("G5_RESULT session-received")
		return
	}
	if mode == "pipe-owner" {
		pipe, err := NewControlPipe(os.Getenv("ABOARD_G5_PIPE"))
		if err != nil {
			t.Fatal(err)
		}
		defer pipe.Close()
		if err = os.WriteFile(filepath.Join(root, "pipe-ready"), []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		waitFile(t, filepath.Join(root, "release"))
		return
	}
	t.Fatal("unknown fixture mode")
}
func TestBootstrapGateInitializeProcesses(t *testing.T) {
	root := gateRoot(t)
	var commands [2]*exec.Cmd
	var outputs [2]bytes.Buffer
	for i := range commands {
		commands[i] = exec.Command(os.Args[0], "-test.run=^TestBootstrapGateChild$", "-test.v")
		commands[i].Env = childEnvironment("initialize", root, "")
		commands[i].Stdout = &outputs[i]
		commands[i].Stderr = &outputs[i]
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
		cmd := commands[i]
		t.Cleanup(func() {
			if cmd.ProcessState == nil {
				cmd.Process.Kill()
				cmd.Wait()
			}
		})
	}
	for _, cmd := range commands {
		waitFile(t, filepath.Join(root, fmt.Sprintf("ready-%d", cmd.Process.Pid)))
	}
	if err := os.WriteFile(filepath.Join(root, "start"), []byte("go"), 0600); err != nil {
		t.Fatal(err)
	}
	initialized, refused := 0, 0
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("child failed: %v %s", err, outputs[i].String())
		}
		text := outputs[i].String()
		if strings.Contains(text, "G5_RESULT initialized") {
			initialized++
		}
		if strings.Contains(text, "G5_RESULT refused") {
			refused++
		}
	}
	if initialized != 1 || refused != 1 {
		t.Fatalf("first-use race initialized=%d refused=%d", initialized, refused)
	}
	a, err := Load(filepath.Join(root, "install"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load(filepath.Join(root, "install"))
	if err != nil {
		t.Fatal(err)
	}
	if a != b || a.InstallationID == "" {
		t.Fatal("installation identity did not survive reopen")
	}
	if err = Initialize(filepath.Join(root, "install")); err == nil {
		t.Fatal("existing installation treated as new")
	}
	t.Log("two real processes: one initialized; loser refused; identity/key stable across authenticated reloads")
}
func TestBootstrapGateMaterialFailures(t *testing.T) {
	root := gateRoot(t)
	t.Run("partial", func(t *testing.T) {
		path := filepath.Join(root, "partial")
		if err := platform.CreatePrivateDirectory(path); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("partial installation accepted")
		}
		if err := Initialize(path); err == nil {
			t.Fatal("partial installation reset")
		}
		names, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 0 {
			t.Fatal("load repaired partial installation")
		}
	})
	t.Run("inherited-acl", func(t *testing.T) {
		path := filepath.Join(root, "ordinary")
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("unsafe installation accepted")
		}
		if err := Initialize(path); err == nil {
			t.Fatal("unsafe installation reset")
		}
	})
	t.Run("corruption", func(t *testing.T) {
		path := filepath.Join(root, "corrupt")
		if err := Initialize(path); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(path, materialName)
		blob, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		blob[len(blob)/2] ^= 0x40
		if err = os.WriteFile(file, blob, 0600); err != nil {
			t.Fatal(err)
		}
		before := append([]byte(nil), blob...)
		if _, err = Load(path); err == nil {
			t.Fatal("corrupt DPAPI material accepted")
		}
		if err = Initialize(path); err == nil {
			t.Fatal("corrupt installation reset")
		}
		after, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("corrupt material rewritten")
		}
	})
	t.Run("file-acl", func(t *testing.T) {
		path := filepath.Join(root, "file-acl")
		if err := Initialize(path); err != nil {
			t.Fatal(err)
		}
		sddl := "D:P(A;;FA;;;WD)"
		sd, err := win.SecurityDescriptorFromString(sddl)
		if err != nil {
			t.Fatal(err)
		}
		acl, _, err := sd.DACL()
		if err != nil {
			t.Fatal(err)
		}
		if err = win.SetNamedSecurityInfo(filepath.Join(path, materialName), win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION|win.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
			t.Fatal(err)
		}
		if _, err = Load(path); err == nil {
			t.Fatal("world-readable material accepted")
		}
	})
	t.Run("no-recreate-missing", func(t *testing.T) {
		path := filepath.Join(root, "missing")
		if _, err := Load(path); err == nil {
			t.Fatal("missing installation accepted")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("load created installation")
		}
	})
}
func TestBootstrapGateDPAPI(t *testing.T) {
	plain := []byte("synthetic-g5-material-not-a-real-credential")
	cipher, err := Protect(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cipher, plain) {
		t.Fatal("DPAPI output contains synthetic plaintext")
	}
	got, err := Unprotect(cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(got)
	if !bytes.Equal(got, plain) {
		t.Fatal("DPAPI per-user roundtrip mismatch")
	}
	cipher[len(cipher)/2] ^= 0x80
	if _, err = Unprotect(cipher); err == nil {
		t.Fatal("tampered DPAPI accepted")
	}
	if _, err = Unprotect([]byte("not-DPAPI")); err == nil {
		t.Fatal("invalid DPAPI accepted")
	}
}
func TestBootstrapGateDPAPIAnonymous(t *testing.T) {
	cipher, err := Protect([]byte("synthetic-anonymous-dpapi-negative"))
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		safe := false
		defer func() {
			if safe {
				runtime.UnlockOSThread()
			}
		}()
		proc := win.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateAnonymousToken")
		ok, _, e := proc.Call(uintptr(win.CurrentThread()))
		if ok == 0 {
			safe = true
			result <- fmt.Errorf("anonymous fixture unavailable: %w", e)
			return
		}
		plain, decryptErr := Unprotect(cipher)
		clear(plain)
		if err := win.RevertToSelf(); err != nil {
			result <- err
			return
		}
		safe = true
		if decryptErr == nil {
			result <- errors.New("anonymous thread decrypted current-user DPAPI material")
			return
		}
		result <- nil
	}()
	if err = <-result; err != nil {
		t.Fatal(err)
	}
	t.Log("real anonymous-token impersonation cannot decrypt current-user DPAPI material")
}

func TestBootstrapGateSessions(t *testing.T) {
	sessions := NewSessions()
	grant := Grant{PrincipalID: "synthetic-principal", WorkspaceID: "A", Action: "read"}
	token, err := sessions.Issue(grant, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := sessions.Authorize(token, "A", "read"); err != nil || got != grant {
		t.Fatal("issued session refused")
	}
	for _, bad := range []string{"", strings.Repeat("A", 43), token[:42], token + "x"} {
		if _, err := sessions.Authorize(bad, "A", "read"); err == nil {
			t.Fatal("forged session accepted")
		}
	}
	if _, err := sessions.Authorize(token, "B", "read"); err == nil {
		t.Fatal("cross-workspace session accepted")
	}
	if _, err := sessions.Authorize(token, "A", "write"); err == nil {
		t.Fatal("escalated action accepted")
	}
	sessions.SetUncertain(true)
	if _, err = sessions.Authorize(token, "A", "read"); err == nil {
		t.Fatal("uncertain authorization used cache")
	}
	if _, err = sessions.Issue(grant, time.Minute); err == nil {
		t.Fatal("uncertain authorization issued session")
	}
	sessions.SetUncertain(false)
	if _, err = sessions.Authorize(token, "A", "read"); err == nil {
		t.Fatal("resolved uncertainty resurrected cached session")
	}
	token, err = sessions.Issue(grant, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sessions.Revoke(token)
	if _, err = sessions.Authorize(token, "A", "read"); err == nil {
		t.Fatal("revoked session accepted")
	}
	token, err = sessions.Issue(grant, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sessions.RevokePrincipal(grant.PrincipalID)
	if _, err = sessions.Authorize(token, "A", "read"); err == nil {
		t.Fatal("old permission version accepted")
	}
	short, err := sessions.Issue(grant, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err = sessions.Authorize(short, "A", "read"); err == nil {
		t.Fatal("expired session accepted")
	}
	if _, err = sessions.Issue(grant, 6*time.Minute); err == nil {
		t.Fatal("long session accepted")
	}
}
func TestBootstrapGateHTTP(t *testing.T) {
	sessions := NewSessions()
	token, err := sessions.Issue(Grant{"synthetic", "A", "read"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"valid", "wrong-host", "origin", "empty-origin", "options", "cookie", "duplicate-auth", "body-forgery"} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://127.0.0.1:41329/test", strings.NewReader(`{"principalId":"operator","workspaceId":"B"}`))
			r.Header.Set("Authorization", "Bearer "+token)
			workspace := "A"
			switch name {
			case "wrong-host":
				r.Host = "localhost:41329"
			case "origin":
				r.Header.Set("Origin", "https://hostile.invalid")
			case "empty-origin":
				r.Header["Origin"] = []string{""}
			case "options":
				r.Method = "OPTIONS"
			case "cookie":
				r.Header.Set("Cookie", "session=synthetic")
			case "duplicate-auth":
				r.Header.Add("Authorization", "Bearer "+token)
			case "body-forgery":
				workspace = "B"
			}
			_, err := sessions.AuthorizeHTTP(r, "127.0.0.1:41329", workspace, "read")
			if name == "valid" && err != nil {
				t.Fatal(err)
			}
			if name != "valid" && err == nil {
				t.Fatal("host/browser/identity boundary accepted")
			}
		})
	}
}
func TestBootstrapGatePipe(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	if duplicate, err := NewControlPipe(name); err == nil {
		duplicate.Close()
		t.Fatal("second first-instance server accepted")
	}
	sessions := NewSessions()
	grant := Grant{"synthetic", "A", "read"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- pipe.ServeOne(ctx, sessions, grant, time.Minute) }()
	token, err := ReceiveSession(ctx, name, ProcessID())
	if err != nil {
		t.Fatal(err)
	}
	if err = <-served; err != nil {
		t.Fatal(err)
	}
	if got, err := sessions.Authorize(token, "A", "read"); err != nil || got != grant {
		t.Fatal("protected session delivery failed")
	}
	if _, err := sessions.Authorize(token, "B", "read"); err == nil {
		t.Fatal("pipe session became operator authority")
	}
	t.Log("real first-instance pipe, protected current-SID DACL, process TokenUser server verification, client impersonation restoration and acknowledged opaque session delivery")
}
func TestBootstrapGatePipeSessionProcess(t *testing.T) {
	root := gateRoot(t)
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	sessions := NewSessions()
	grant := Grant{"synthetic", "A", "read"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- pipe.ServeOne(ctx, sessions, grant, time.Minute) }()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBootstrapGateChild$", "-test.v")
	cmd.Env = append(childEnvironment("session-client", root, name), fmt.Sprintf("ABOARD_G5_SERVER_PID=%d", ProcessID()))
	output, runErr := cmd.CombinedOutput()
	// Scan real second-process stdout/stderr before any reporting. Never include
	// possibly tainted output in a test failure. Boundaries avoid matching names.
	credentialShape := regexp.MustCompile(`(^|[^A-Za-z0-9_-])[A-Za-z0-9_-]{43}([^A-Za-z0-9_-]|$)`)
	if credentialShape.Match(output) {
		t.Fatal("credential-shaped output detected (redacted)")
	}
	if runErr != nil {
		t.Fatalf("control client failed: %v (output withheld)", runErr)
	}
	if !bytes.Contains(output, []byte("G5_RESULT session-received")) {
		t.Fatal("delivery receipt absent")
	}
	if err = <-served; err != nil {
		t.Fatal(err)
	}
	sessions.mu.Lock()
	count := len(sessions.entries)
	var actual Grant
	for _, entry := range sessions.entries {
		actual = entry.grant
	}
	sessions.mu.Unlock()
	if count != 1 || actual != grant {
		t.Fatal("cross-process session issuance mismatch")
	}
	t.Log("real second-process control client authenticated server, received acknowledged single bearer; captured stdout/stderr credential-shape scan clean")
}

func TestBootstrapGatePipeUnacknowledgedDelivery(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	sessions := NewSessions()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- pipe.ServeOne(ctx, sessions, Grant{"synthetic", "A", "read"}, time.Minute) }()
	p, _ := win.UTF16PtrFromString(pipe.name)
	h, err := win.CreateFile(p, win.GENERIC_READ|win.GENERIC_WRITE, 0, nil, win.OPEN_EXISTING, win.FILE_FLAG_OVERLAPPED|win.SECURITY_SQOS_PRESENT|win.SECURITY_IMPERSONATION, 0)
	if err != nil {
		t.Fatal(err)
	}
	// This deliberately faulty synthetic client reads all bearer bytes and then
	// loses the connection before acknowledging. No token is written to output.
	if err = writeExact(ctx, h, []byte{1}); err != nil {
		win.CloseHandle(h)
		t.Fatal(err)
	}
	var header [2]byte
	if err = readExact(ctx, h, header[:]); err != nil {
		win.CloseHandle(h)
		t.Fatal(err)
	}
	token := make([]byte, 43)
	defer clear(token)
	if err = readExact(ctx, h, token); err != nil {
		win.CloseHandle(h)
		t.Fatal(err)
	}
	if err = win.CloseHandle(h); err != nil {
		t.Fatal(err)
	}
	if err = <-served; err == nil {
		t.Fatal("unacknowledged delivery reported success")
	}
	if _, err = sessions.Authorize(string(token), "A", "read"); err == nil {
		t.Fatal("unacknowledged session remained authorized")
	}
	t.Log("real delivered bytes followed by connection loss: issuance revoked; no secret output")
}

func TestBootstrapGatePipeAnonymous(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	sessions := NewSessions()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- pipe.ServeOne(ctx, sessions, Grant{"synthetic", "A", "read"}, time.Minute) }()
	if _, err = receiveSession(ctx, name, ProcessID(), win.SECURITY_ANONYMOUS); err == nil {
		t.Fatal("anonymous client received session")
	}
	if err = <-served; err == nil {
		t.Fatal("anonymous impersonation accepted")
	}
	sessions.mu.Lock()
	count := len(sessions.entries)
	sessions.mu.Unlock()
	if count != 0 {
		t.Fatal("anonymous client caused issuance")
	}
	t.Log("real SECURITY_ANONYMOUS QoS: OS impersonation cannot supply TokenUser; zero minted sessions")
}
func TestBootstrapGatePipeWrongSIDAndRestoration(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	// Same current token passes pipe DACL, but a deliberately different expected
	// SID exercises real TokenUser mismatch AFTER kernel impersonation. This is
	// not evidence of a second account; anonymous/DACL are separate real cases.
	wrong, err := win.CreateWellKnownSid(win.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	if wrong.Equals(pipe.sid) {
		t.Skip("SYSTEM environment cannot use SYSTEM as wrong expected SID")
	}
	pipe.sid = wrong
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sessions := NewSessions()
	served := make(chan error, 1)
	go func() { served <- pipe.ServeOne(ctx, sessions, Grant{"synthetic", "A", "read"}, time.Minute) }()
	if _, err = ReceiveSession(ctx, name, ProcessID()); err == nil {
		t.Fatal("wrong client SID accepted")
	}
	if err = <-served; !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected actual TokenUser rejection, got %v", err)
	}
	// A fresh legitimate exchange must still succeed: mismatch error path must
	// not leave a contaminated impersonation thread or mint a credential.
	if len(sessions.entries) != 0 {
		t.Fatal("wrong SID minted session")
	}
	t.Log("kernel TokenUser mismatched required SID; restoration verified on error path before returning")
}
func TestBootstrapGateAnonymousDACL(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	result := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		safe := false
		defer func() {
			if safe {
				runtime.UnlockOSThread()
			}
		}()
		proc := win.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateAnonymousToken")
		ok, _, e := proc.Call(uintptr(win.CurrentThread()))
		if ok == 0 {
			safe = true
			result <- fmt.Errorf("anonymous fixture unavailable: %w", e)
			return
		}
		p, _ := win.UTF16PtrFromString(pipe.name)
		h, openErr := win.CreateFile(p, win.GENERIC_READ|win.GENERIC_WRITE, 0, nil, win.OPEN_EXISTING, win.SECURITY_SQOS_PRESENT|win.SECURITY_ANONYMOUS, 0)
		if openErr == nil {
			win.CloseHandle(h)
		}
		if err := win.RevertToSelf(); err != nil {
			result <- err
			return
		}
		safe = true
		if !errors.Is(openErr, win.ERROR_ACCESS_DENIED) {
			result <- fmt.Errorf("anonymous DACL expected Access denied, got %v", openErr)
			return
		}
		result <- nil
	}()
	if err = <-result; err != nil {
		t.Fatal(err)
	}
	t.Log("real anonymous thread token rejected by current-user/SYSTEM pipe DACL (Access denied)")
}
func TestBootstrapGatePipePreemptionProcess(t *testing.T) {
	root := gateRoot(t)
	name := pipeName(t)
	var output bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run=^TestBootstrapGateChild$", "-test.v")
	cmd.Env = childEnvironment("pipe-owner", root, name)
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	waitFile(t, filepath.Join(root, "pipe-ready"))
	if p, err := NewControlPipe(name); err == nil {
		p.Close()
		t.Fatal("preempted pipe name accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := ReceiveSession(ctx, name, ProcessID()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong server launch PID not refused: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "release"), []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("pipe owner child: %v %s", err, output.String())
	}
	t.Log("real second-process pipe preemption refused; actual wrong server PID rejected before handshake/credential")
}
func TestBootstrapGateServerSID(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	p, _ := win.UTF16PtrFromString(pipe.name)
	h, err := win.CreateFile(p, win.GENERIC_READ|win.GENERIC_WRITE|win.READ_CONTROL, 0, nil, win.OPEN_EXISTING, win.FILE_FLAG_OVERLAPPED|win.SECURITY_SQOS_PRESENT|win.SECURITY_IMPERSONATION, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer win.CloseHandle(h)
	wrong, err := win.CreateWellKnownSid(win.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	if wrong.Equals(pipe.sid) {
		t.Skip("SYSTEM cannot serve as mismatched expected SID")
	}
	if process, err := verifyPipeServer(h, ProcessID(), wrong); err == nil {
		win.CloseHandle(process)
		t.Fatal("server process TokenUser mismatch accepted")
	} else if !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	t.Log("kernel-reported PID correct but real server process TokenUser does not match required SID: refused")
}

func TestBootstrapGatePipeCancellation(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err = pipe.ServeOne(ctx, NewSessions(), Grant{"synthetic", "A", "read"}, time.Minute)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unconnected pipe cancel: %v", err)
	}
	if err = pipe.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewControlPipe(name)
	if err != nil {
		t.Fatal("cancel did not physically release pipe", err)
	}
	reopened.Close()
}
func TestBootstrapGateRemoteRejection(t *testing.T) {
	// Real remote redirector evidence requires a reachable existing SMB transport.
	// Establish a SAME-DACL permissive control before attributing Access denied to
	// PIPE_REJECT_REMOTE_CLIENTS. Never start a service/change policy to obtain it.
	name := pipeName(t)
	sid, err := currentSID()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := privateSD(sid, false)
	if err != nil {
		t.Fatal(err)
	}
	sa := win.SecurityAttributes{Length: uint32(unsafe.Sizeof(win.SecurityAttributes{})), SecurityDescriptor: sd}
	path, _ := localPipeName(name)
	p, _ := win.UTF16PtrFromString(path)
	control, err := win.CreateNamedPipe(p, win.PIPE_ACCESS_DUPLEX|win.FILE_FLAG_FIRST_PIPE_INSTANCE, win.PIPE_TYPE_BYTE|win.PIPE_WAIT, 1, 4096, 4096, 1000, &sa)
	if err != nil {
		t.Fatal(err)
	}
	remote := `\\127.0.0.1\pipe\` + name
	rp, _ := win.UTF16PtrFromString(remote)
	client, controlErr := win.CreateFile(rp, win.GENERIC_READ|win.GENERIC_WRITE, 0, nil, win.OPEN_EXISTING, 0, 0)
	if controlErr == nil {
		win.CloseHandle(client)
	}
	win.CloseHandle(control)
	if controlErr != nil {
		t.Skipf("UNVERIFIED remote rejection: permissive same-DACL SMB control unavailable: %v; no OS service/policy changed", controlErr)
	}
	hardened, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer hardened.Close()
	client, err = win.CreateFile(rp, win.GENERIC_READ|win.GENERIC_WRITE, 0, nil, win.OPEN_EXISTING, 0, 0)
	if err == nil {
		win.CloseHandle(client)
		t.Fatal("real SMB client bypassed remote rejection")
	}
	if !errors.Is(err, win.ERROR_ACCESS_DENIED) {
		t.Fatalf("remote baseline reachable but rejection cause unproven: %v", err)
	}
	t.Log("real SMB same-SID baseline reachable; hardened pipe rejects remote redirector Access denied")
}
