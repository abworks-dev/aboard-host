//go:build windows && amd64

package security

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
	"unsafe"

	win "golang.org/x/sys/windows"
)

// This context contains NO typed reference/closure to the native read buffer.
// Its second Err call occurs only after a genuine pending native read has waited.
// Grow the same goroutine's stack, then cancel: the peer NEVER writes payload to
// a possibly stale pointer, and pipeOperation must cancel+join before assertions.
//
//go:noinline
func pipeStackAddress(b *byte) uintptr { return uintptr(unsafe.Pointer(b)) }

//go:noinline
func forcePipeStack(depth int) byte {
	var pad [4096]byte
	for i := range pad {
		pad[i] = byte(depth + i)
	}
	at := depth % len(pad)
	if depth > 0 {
		pad[at] ^= forcePipeStack(depth - 1)
	}
	runtime.KeepAlive(&pad)
	return pad[at]
}

type stackCancelPipeContext struct {
	calls int
	moved bool
	done  chan struct{}
}

func (*stackCancelPipeContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *stackCancelPipeContext) Done() <-chan struct{}     { return c.done }
func (*stackCancelPipeContext) Value(any) any               { return nil }
func (c *stackCancelPipeContext) Err() error {
	c.calls++
	if c.calls == 1 {
		return nil
	}
	if c.calls == 2 {
		var marker byte
		before := pipeStackAddress(&marker)
		_ = forcePipeStack(1024)
		c.moved = before != pipeStackAddress(&marker)
		close(c.done)
	}
	return context.Canceled
}

func TestPipePendingReadPinsBufferAcrossRealStackMove(t *testing.T) {
	pipe, err := NewControlPipe(pipeName(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := pipe.Close(); err != nil {
			t.Error("native pipe close unknown")
		}
	}()
	name, err := win.UTF16PtrFromString(pipe.name)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := win.CreateFile(name, win.GENERIC_READ|win.GENERIC_WRITE, 0, nil, win.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := win.CloseHandle(peer); err != nil {
			t.Error("native peer close unknown")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = pipeOperation(ctx, pipe.handle, nil, func(ov *win.Overlapped) error { return win.ConnectNamedPipe(pipe.handle, ov) }); err != nil {
		t.Fatal(err)
	}
	control := &stackCancelPipeContext{done: make(chan struct{})}
	buffer := [1]byte{0xa5}
	before := pipeStackAddress(&buffer[0])
	err = readExact(control, pipe.handle, buffer[:])
	after := pipeStackAddress(&buffer[0])
	if !errors.Is(err, context.Canceled) || control.calls < 2 || !control.moved {
		t.Fatal("native pending/canceled join or genuine same-goroutine stack move was not established")
	}
	if before != after {
		t.Fatal("native pending ReadFile buffer moved with the Go stack before physical cancel/join")
	}
	if buffer[0] != 0xa5 {
		t.Fatal("canceled no-payload control changed buffer")
	}
}
