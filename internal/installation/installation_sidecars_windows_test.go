//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
)

type sidecarFixtureEnd struct {
	close     func() error
	attempted bool
	err       error
}

func (e *sidecarFixtureEnd) finish() error {
	if !e.attempted {
		e.attempted = true
		e.err = e.close()
	}
	return e.err
}

func TestInstallationOpenRefusesUnprovedSidecarsBeforeSQLite(t *testing.T) {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		t.Run(suffix, func(t *testing.T) {
			base, err := filepath.Abs(filepath.Join("..", "..", ".data", "logs", "001-validation"))
			if err != nil {
				t.Fatal(err)
			}
			if err = os.MkdirAll(base, 0700); err != nil {
				t.Fatal(err)
			}
			outer, err := os.MkdirTemp(base, "t025-install-sidecar-")
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(outer, "installation") // Initialize requires a nonexistent final child
			var ends []*sidecarFixtureEnd
			track := func(close func() error, incoming error) *sidecarFixtureEnd {
				end := &sidecarFixtureEnd{close: close}
				if incoming != nil {
					end.attempted = true
					end.err = incoming
				}
				ends = append(ends, end)
				return end
			}
			retainUnknown := func(err error) {
				if errors.Is(err, platform.ErrPhysicalCloseUnknown) || errors.Is(err, sqlite.ErrPhysicalCloseUnknown) {
					ends = append(ends, &sidecarFixtureEnd{attempted: true, err: err})
				}
			}
			// Register EVERY nonnil result before checking its error. Unknown physical
			// cleanup retains this fixture; never RemoveAll through a live writer/pin.
			t.Cleanup(func() {
				ended := true
				for i := len(ends) - 1; i >= 0; i-- {
					if ends[i].finish() != nil {
						ended = false
						t.Error("fixture native end unproved; source retained")
					}
				}
				if ended {
					if err := os.RemoveAll(outer); err != nil {
						t.Error(err)
					}
				}
			})
			initial, err := Initialize(context.Background(), root)
			retainUnknown(err)
			var initialEnd *sidecarFixtureEnd
			if initial != nil {
				initialEnd = track(initial.Close, err)
			}
			if err != nil {
				t.Fatal("fixture initialization failed")
			}
			if initialEnd.finish() != nil {
				t.Fatal("fixture store physical close failed")
			}
			path := filepath.Join(root, "installation.sqlite"+suffix)
			if err = os.WriteFile(path, []byte("fixed unproved installation sidecar"), 0600); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			store, openErr := Open(context.Background(), root)
			retainUnknown(openErr)
			if store != nil {
				track(store.Close, openErr)
			}
			if openErr == nil || store != nil {
				t.Fatal("installation unproved recovery was admitted")
			}
			remaining, err := os.ReadFile(path)
			if err != nil || string(remaining) != string(original) {
				t.Fatal("existing startup touched unproved sidecar")
			}
			lease, err := platform.Acquire(root, platform.Installation, false, false)
			retainUnknown(err)
			var leaseEnd *sidecarFixtureEnd
			if lease != nil {
				leaseEnd = track(lease.Close, err)
			}
			if err != nil {
				t.Fatal("known refusal retained installation writer")
			}
			if leaseEnd.finish() != nil {
				t.Fatal("fixture lease close failed")
			}
			if err = os.Remove(path); err != nil {
				t.Fatal(err)
			}
			known, err := Open(context.Background(), root)
			retainUnknown(err)
			var knownEnd *sidecarFixtureEnd
			if known != nil {
				knownEnd = track(known.Close, err)
			}
			if err != nil {
				t.Fatal("test-only sidecar removal did not restore valid installation")
			}
			if knownEnd.finish() != nil {
				t.Fatal("known physical close failed")
			}
		})
	}
}
