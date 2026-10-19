//go:build windows && amd64

package security

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
	"unsafe"

	win "golang.org/x/sys/windows"
)

var impersonatePipe = win.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateNamedPipeClient")

// ControlPipe owns exactly one first instance. No fallback suffix or second
// server is created on collision. Close is only valid after ServeOne returns.
type ControlPipe struct {
	handle win.Handle
	name   string
	sid    *win.SID
}

func localPipeName(name string) (string, error) {
	if !strings.HasPrefix(name, "aboard-g5-") || len(name) > 100 || strings.ContainsAny(name, `\/:`+"\x00") {
		return "", errors.New("invalid local control pipe name")
	}
	return `\\.\pipe\` + name, nil
}
func NewControlPipe(name string) (*ControlPipe, error) {
	path, err := localPipeName(name)
	if err != nil {
		return nil, err
	}
	sid, err := currentSID()
	if err != nil {
		return nil, err
	}
	sd, err := privateSD(sid, false)
	if err != nil {
		return nil, err
	}
	sa := win.SecurityAttributes{Length: uint32(unsafe.Sizeof(win.SecurityAttributes{})), SecurityDescriptor: sd}
	p, _ := win.UTF16PtrFromString(path)
	h, err := win.CreateNamedPipe(p, win.PIPE_ACCESS_DUPLEX|win.FILE_FLAG_FIRST_PIPE_INSTANCE|win.FILE_FLAG_OVERLAPPED, win.PIPE_TYPE_BYTE|win.PIPE_READMODE_BYTE|win.PIPE_WAIT|win.PIPE_REJECT_REMOTE_CLIENTS, 1, 4096, 4096, 1000, &sa)
	runtime.KeepAlive(sd)
	if err != nil {
		return nil, fmt.Errorf("control pipe unavailable: %w", err)
	}
	if err = checkPrivateObject(h, sid, false); err != nil {
		win.CloseHandle(h)
		return nil, err
	}
	return &ControlPipe{handle: h, name: path, sid: sid}, nil
}
func (p *ControlPipe) Close() error { return win.CloseHandle(p.handle) }

// ServeOne authenticates the actual client token AFTER a bounded handshake,
// restores impersonation BEFORE minting, then delivers one opaque bearer. Failed
// or uncertain delivery revokes it rather than logging or retrying the secret.
type Issuer interface {
	Issue(Grant, time.Duration) (string, error)
	Revoke(string)
}

// DeliveryConfirmer allows a persistent issuer to commit pending-to-active only
// after the authenticated peer's initial ACK. Prototype Sessions stays a G5
// primitive, not installation business authorization.
type DeliveryConfirmer interface {
	ConfirmDelivery(string) error
}

func (p *ControlPipe) ServeOne(ctx context.Context, sessions Issuer, grant Grant, ttl time.Duration) (err error) {
	if err = pipeOperation(ctx, p.handle, nil, func(ov *win.Overlapped) error { return win.ConnectNamedPipe(p.handle, ov) }); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, win.DisconnectNamedPipe(p.handle)) }()
	var hello [1]byte
	if err = readExact(ctx, p.handle, hello[:]); err != nil {
		return err
	}
	if hello[0] != 1 {
		return ErrUnauthorized
	}
	if err = verifyPipeCaller(p.handle, p.sid); err != nil {
		return err
	}
	token, err := sessions.Issue(grant, ttl)
	if err != nil {
		return err
	}
	delivered := false
	defer func() {
		if !delivered {
			sessions.Revoke(token)
		}
	}()
	payload := []byte(token)
	defer clear(payload)
	var header [2]byte
	binary.LittleEndian.PutUint16(header[:], uint16(len(payload)))
	if err = writeExact(ctx, p.handle, header[:]); err != nil {
		return err
	}
	if err = writeExact(ctx, p.handle, payload); err != nil {
		return err
	}
	// Explicit acknowledgement means read delivery, not just an OS pipe buffer.
	if err = readExact(ctx, p.handle, hello[:]); err != nil {
		return err
	}
	if hello[0] != 2 {
		return ErrUnauthorized
	}
	if confirmer, ok := sessions.(DeliveryConfirmer); ok {
		if err = confirmer.ConfirmDelivery(token); err != nil {
			return err
		}
	}
	// Fixed internal protocol: 1 -> token -> 2 -> activation -> 3 -> 4.
	// ReceiveSession never returns a token before final server confirmation.
	// Wait for 4 before DisconnectNamedPipe, which may discard unread bytes.
	// All waits are canceled/drained; no installation mutex spans pipe IO.
	if err = writeExact(ctx, p.handle, []byte{3}); err != nil {
		return err
	}
	if err = readExact(ctx, p.handle, hello[:]); err != nil {
		return err
	}
	if hello[0] != 4 {
		return ErrUnauthorized
	}
	delivered = true
	return nil
}

// ReceiveSession binds the kernel-reported server PID to an independently
// trusted live launch PID AND reads its real process TokenUser. PID alone is not
// identity. The process handle pins the kernel object while the pipe is used.
// No credential bytes are sent before server identity and DACL verification.
func ReceiveSession(ctx context.Context, name string, expectedPID uint32) (string, error) {
	return receiveSession(ctx, name, expectedPID, win.SECURITY_IMPERSONATION)
}
func receiveSession(ctx context.Context, name string, expectedPID uint32, qos uint32) (string, error) {
	path, err := localPipeName(name)
	if err != nil {
		return "", err
	}
	if expectedPID == 0 {
		return "", ErrUnauthorized
	}
	p, _ := win.UTF16PtrFromString(path)
	h, err := win.CreateFile(p, win.GENERIC_READ|win.GENERIC_WRITE|win.READ_CONTROL, 0, nil, win.OPEN_EXISTING, win.FILE_FLAG_OVERLAPPED|win.SECURITY_SQOS_PRESENT|qos, 0)
	if err != nil {
		return "", err
	}
	defer win.CloseHandle(h)
	sid, err := currentSID()
	if err != nil {
		return "", err
	}
	if err = checkPrivateObject(h, sid, false); err != nil {
		return "", err
	}
	process, err := verifyPipeServer(h, expectedPID, sid)
	if err != nil {
		return "", err
	}
	defer win.CloseHandle(process)
	if err = writeExact(ctx, h, []byte{1}); err != nil {
		return "", err
	}
	var header [2]byte
	if err = readExact(ctx, h, header[:]); err != nil {
		return "", err
	}
	size := binary.LittleEndian.Uint16(header[:])
	if size != 43 {
		return "", ErrUnauthorized
	}
	payload := make([]byte, size)
	defer clear(payload)
	if err = readExact(ctx, h, payload); err != nil {
		return "", err
	}
	if err = writeExact(ctx, h, []byte{2}); err != nil {
		return "", err
	}
	var confirmed [1]byte
	if err = readExact(ctx, h, confirmed[:]); err != nil {
		return "", err
	}
	if confirmed[0] != 3 {
		return "", ErrUnauthorized
	}
	if err = writeExact(ctx, h, []byte{4}); err != nil {
		return "", err
	}
	return string(payload), nil
}
func verifyPipeServer(h win.Handle, expectedPID uint32, sid *win.SID) (win.Handle, error) {
	var pid uint32
	if err := win.GetNamedPipeServerProcessId(h, &pid); err != nil {
		return 0, err
	}
	if pid == 0 || pid != expectedPID {
		return 0, ErrUnauthorized
	}
	process, err := win.OpenProcess(win.PROCESS_QUERY_LIMITED_INFORMATION|win.SYNCHRONIZE, false, pid)
	if err != nil {
		return 0, err
	}
	fail := func(e error) (win.Handle, error) { win.CloseHandle(process); return 0, e }
	// Refuse an exited process: a stale PID is not a trusted live server.
	status, err := win.WaitForSingleObject(process, 0)
	if err != nil || status != uint32(win.WAIT_TIMEOUT) {
		return fail(ErrUnauthorized)
	}
	var token win.Token
	if err = win.OpenProcessToken(process, win.TOKEN_QUERY, &token); err != nil {
		return fail(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return fail(err)
	}
	if !user.User.Sid.Equals(sid) {
		return fail(ErrUnauthorized)
	}
	return process, nil
}

// Each impersonation occurs on a dedicated pinned goroutine. If RevertToSelf or
// its postcondition fails, it NEVER unlocks: Go destroys that thread when the
// goroutine exits, rather than returning a contaminated thread to its pool.
func verifyPipeCaller(pipe win.Handle, expected *win.SID) error {
	result := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		safe := false
		defer func() {
			if safe {
				runtime.UnlockOSThread()
			}
		}()
		var old win.Token
		err := win.OpenThreadToken(win.CurrentThread(), win.TOKEN_QUERY, true, &old)
		if err == nil {
			old.Close()
			result <- errors.New("unexpected initial thread impersonation")
			return
		}
		if !errors.Is(err, win.ERROR_NO_TOKEN) {
			result <- err
			return
		}
		ok, _, nativeErr := impersonatePipe.Call(uintptr(pipe))
		if ok == 0 {
			safe = true
			result <- fmt.Errorf("pipe impersonation refused: %w", nativeErr)
			return
		}
		authErr := func() error {
			var token win.Token
			if err := win.OpenThreadToken(win.CurrentThread(), win.TOKEN_QUERY, true, &token); err != nil {
				return ErrUnauthorized
			}
			defer token.Close()
			user, err := token.GetTokenUser()
			if err != nil {
				return ErrUnauthorized
			}
			if !user.User.Sid.Equals(expected) {
				return ErrUnauthorized
			}
			return nil
		}()
		if err = win.RevertToSelf(); err != nil {
			result <- errors.New("impersonation restoration failed; thread quarantined")
			return
		}
		err = win.OpenThreadToken(win.CurrentThread(), win.TOKEN_QUERY, true, &old)
		if err == nil {
			old.Close()
			result <- errors.New("impersonation remained; thread quarantined")
			return
		}
		if !errors.Is(err, win.ERROR_NO_TOKEN) {
			result <- errors.New("impersonation restoration unverified; thread quarantined")
			return
		}
		safe = true
		result <- authErr
	}()
	return <-result
}

// All overlapped operations cancel AND drain on cancellation. A timeout alone
// does not free buffers/events/handles while physical kernel IO is outstanding.
func pipeOperation(ctx context.Context, h win.Handle, transferred *uint32, start func(*win.Overlapped) error) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	event, err := win.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer win.CloseHandle(event)
	ov := win.Overlapped{HEvent: event}
	err = start(&ov)
	if errors.Is(err, win.ERROR_PIPE_CONNECTED) {
		return nil
	}
	if err == nil {
		return nil
	}
	if !errors.Is(err, win.ERROR_IO_PENDING) {
		return err
	}
	for {
		status, e := win.WaitForSingleObject(event, 10)
		if e != nil {
			err = e
			break
		}
		if status == win.WAIT_OBJECT_0 {
			var n uint32
			e := win.GetOverlappedResult(h, &ov, &n, false)
			if transferred != nil {
				*transferred = n
			}
			return e
		}
		if status != uint32(win.WAIT_TIMEOUT) {
			err = errors.New("unexpected pipe IO wait")
			break
		}
		if e = ctx.Err(); e != nil {
			err = e
			break
		}
	}
	cancelErr := win.CancelIoEx(h, &ov)
	if cancelErr != nil && !errors.Is(cancelErr, win.ERROR_NOT_FOUND) {
		err = errors.Join(err, cancelErr)
	}
	var n uint32
	drainErr := win.GetOverlappedResult(h, &ov, &n, true)
	if drainErr != nil && !errors.Is(drainErr, win.ERROR_OPERATION_ABORTED) {
		err = errors.Join(err, drainErr)
	}
	return err
}
func readExact(ctx context.Context, h win.Handle, b []byte) error {
	for len(b) > 0 {
		var n uint32
		err := pipeOperation(ctx, h, &n, func(ov *win.Overlapped) error { return win.ReadFile(h, b, &n, ov) })
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
		b = b[n:]
	}
	return nil
}
func writeExact(ctx context.Context, h win.Handle, b []byte) error {
	for len(b) > 0 {
		var n uint32
		err := pipeOperation(ctx, h, &n, func(ov *win.Overlapped) error { return win.WriteFile(h, b, &n, ov) })
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

// Kept local to avoid treating a caller-supplied PID as Windows identity.
func ProcessID() uint32 { return uint32(os.Getpid()) }
