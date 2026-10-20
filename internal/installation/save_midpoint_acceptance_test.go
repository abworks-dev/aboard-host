//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"modernc.org/libc"
)

func TestAcceptanceMidpointUnrelatedNativeFlushBypassesHold(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root, e := os.MkdirTemp(filepath.Join(filepath.Dir(file), "..", "..", ".data", "logs", "001-validation"), "t023-midpoint-guard-")
	if e != nil {
		t.Fatal(e)
	}
	root = filepath.Clean(root)
	defer func() {
		if creationProbeRoot(root) {
			if e := os.RemoveAll(root); e != nil {
				t.Error(e)
			}
		}
	}()
	f, e := os.OpenFile(filepath.Join(root, "unrelated-B.sqlite"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := f.Close(); e != nil {
			t.Error(e)
		}
	}()
	if _, e = f.Write([]byte("synthetic B native positive control")); e != nil {
		t.Fatal(e)
	}
	state := &finalizeFlushState{active: true, databasePath: filepath.Join(root, "held-A.sqlite")}
	if !armedFinalize.CompareAndSwap(nil, state) {
		t.Fatal("another probe armed")
	}
	defer armedFinalize.CompareAndSwap(state, nil)
	state.mu.Lock() // policy hold; B must execute its REAL native flush outside this mutex
	done := make(chan int32, 1)
	go func() {
		tls := libc.NewTLS()
		native := finalizeFlushDelegate(tls, f.Fd())
		tls.Close()
		done <- native
	}()
	var result int32
	timedOut := false
	select {
	case result = <-done:
	case <-time.After(2 * time.Second):
		timedOut = true
	}
	state.mu.Unlock()
	if timedOut {
		result = <-done
		t.Fatal("unrelated B blocked on held A callback")
	}
	if result == 0 || state.calls != 0 || state.injected != 0 {
		t.Fatal("unrelated native flush did not delegate unchanged")
	}
}
func TestAcceptanceMidpointWithoutCommitCannotPublish(t *testing.T) {
	midpoint := &saveNativeMidpoint{} // deliberately no original capability/observation
	midpoint.afterSync("A-journal", "A", 1)
	midpoint.afterSync("A-journal", "A", 1)
	if !midpoint.disqualified || midpoint.published {
		t.Fatal("unobserved original COMMIT published native evidence")
	}
}
