//go:build windows && amd64

package inspect

import (
	"errors"
	"io"
	"os"
	"sync"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	store "github.com/abworks-dev/aboard-host/internal/storage/sqlite"
)

// ErrCleanupPending never proves release or permission to move the source.
var ErrCleanupPending = errors.New("inspection resources have not physically drained")
var ErrScratchCleanupPending = errors.New("inspection scratch deletion refused after physical IO ended")

type ownedStream struct {
	file      *os.File
	attempted bool
	result    error
}

// Resources owns all inspection IO, including transient duplicate handles. It is
// internal Host evidence, not a DTO or a grant. Failed os.File/SQL close is sticky:
// a later nil Close cannot prove the first driver's physical release.
type Resources struct {
	mu             sync.Mutex
	lease          *platform.Lease
	files          []*platform.File
	streams        []*ownedStream
	scratch        *platform.Scratch
	database       *store.Handle
	cleanup        func(string) error
	scratchDeleted bool
	poison         error
	ended          bool
	// Per-owner internal test seams; never configurable by HTTP or trusted config.
	closeStream   func(*os.File) error
	closeDatabase func(*store.Handle) error
}

func (r *Resources) own(file *os.File) *ownedStream {
	s := &ownedStream{file: file}
	r.streams = append(r.streams, s)
	return s
}
func (r *Resources) closeOwned(s *ownedStream) error {
	if !s.attempted {
		s.attempted = true
		if r.closeStream != nil {
			s.result = r.closeStream(s.file)
		} else {
			s.result = s.file.Close()
		}
	}
	return s.result
}
func (r *Resources) readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	s := r.own(f)
	data, readErr := io.ReadAll(f)
	return data, errors.Join(readErr, r.closeOwned(s))
}

// Close is serialized and ordered. Only a completed deletion refusal is
// retryable; its protected anchors and source exclusion remain owned. No native
// failure is laundered by trying an already-closed os.File/SQL handle again.
func (r *Resources) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended {
		return nil
	}
	if r.poison != nil {
		return errors.Join(ErrCleanupPending, r.poison)
	}
	fail := func(err error) error { r.poison = err; return errors.Join(ErrCleanupPending, err) }
	// Transient streams must end before the scratch DB can be retired/deleted.
	for i := len(r.streams) - 1; i >= 0; i-- {
		if err := r.closeOwned(r.streams[i]); err != nil {
			return fail(err)
		}
	}
	if r.database != nil {
		var err error
		if r.closeDatabase != nil {
			err = r.closeDatabase(r.database)
		} else {
			err = r.database.Close()
		}
		if err != nil {
			return fail(err)
		}
		r.database = nil
	}
	if r.scratch != nil {
		if !r.scratchDeleted {
			if err := r.scratch.PrepareCleanup(); err != nil {
				return fail(err)
			}
			if err := r.cleanup(r.scratch.Path()); err != nil {
				return errors.Join(ErrCleanupPending, ErrScratchCleanupPending, err)
			}
			r.scratchDeleted = true
		}
		if err := r.scratch.Close(); err != nil {
			return fail(err)
		}
		r.scratch = nil
	}
	for i := len(r.files) - 1; i >= 0; i-- {
		if err := r.files[i].Close(); err != nil {
			return fail(err)
		}
	}
	if r.lease != nil {
		if err := r.lease.ClosePinnedFiles(); err != nil {
			return fail(err)
		}
		if err := r.lease.Close(); err != nil {
			return fail(err)
		}
		r.lease = nil
	}
	r.ended = true
	return nil
}
