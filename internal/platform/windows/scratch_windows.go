//go:build windows && amd64

package windows

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	win "golang.org/x/sys/windows"
)

// Scratch pins a newly created, private temporary directory and its ancestors.
// It is sensitive cache, not a workspace, registration or authorization grant.
type Scratch struct {
	path            string
	pins            []*File
	cleanupPrepared bool
}

func (s *Scratch) Path() string { return s.path }

// PrepareCleanup releases only the scratch root's delete-denying pin. Its
// protected parent and all ancestors stay pinned until Close, so another SID
// cannot rename the cache anchor and turn path deletion into a false success.
// No further CreateFile is allowed after this transition.
func (s *Scratch) PrepareCleanup() error {
	if len(s.pins) == 0 {
		return errors.New("scratch artifact has no verified cleanup pin")
	}
	if s.cleanupPrepared {
		// The scratch pin is deliberately closed, but every protected anchor
		// must still validate before retrying deletion after a transient failure.
		for _, p := range s.pins[:len(s.pins)-1] {
			if err := p.Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	if err := s.validate(); err != nil {
		return err
	}
	if err := s.pins[len(s.pins)-1].Close(); err != nil {
		return retainPhysicalClose(err, s)
	}
	s.cleanupPrepared = true
	return nil
}
func (s *Scratch) Close() error {
	return retainPhysicalClose(closePins(s.pins), s)
}
func (s *Scratch) validate() error {
	if len(s.pins) == 0 {
		return errors.New("scratch artifact has no verified directory pin")
	}
	if s.cleanupPrepared {
		return errors.New("scratch cleanup already prepared")
	}
	for _, p := range s.pins {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return retainPhysicalClose(CheckPrivateDirectory(s.path), s)
}

// NewScratch requires an already private parent and a single random child name.
// Failure artifacts are not silently repaired. No source path is accepted here.
// A nonnil Scratch may accompany an error and must be retained by the caller.
// A created artifact without verified pins cannot be safely deleted through it.
func NewScratch(parent, name string) (s *Scratch, err error) {
	if !strings.HasPrefix(name, "inspect-") || len(name) != len("inspect-")+48 {
		return nil, errors.New("invalid scratch name")
	}
	for _, c := range name[len("inspect-"):] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return nil, errors.New("invalid scratch nonce")
		}
	}
	if _, err := supportedPath(parent); err != nil {
		return nil, err
	}
	pins, err := pinDirectories(parent)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = unwindPins(err, pins, s)
	}()
	if err = CheckPrivateDirectory(parent); err != nil {
		return nil, err
	}
	path := filepath.Join(parent, name)
	created, err := createPrivateDirectory(path)
	if created {
		// Keep the exact fresh artifact even if later validation/pinning fails.
		// Path evidence alone does not authorize arbitrary path-based deletion.
		s = &Scratch{path: path}
	}
	if err != nil {
		return s, retainPhysicalClose(err, s)
	}
	// A metadata-only reparse between creation and this open is rejected, not
	// followed. Future creates are relative to this actual pinned directory.
	all, err := pinDirectories(path)
	if err != nil {
		return s, retainPhysicalClose(err, s)
	}
	s.pins = all
	if err = s.validate(); err != nil {
		return s, unwindPins(err, s.pins, s)
	}
	return s, nil
}

// CreateFile allows only fixed inspection/recovery copy names. OBJ_DONT_REPARSE
// prevents redirecting creation through an empty, metadata-mutated directory.
func (s *Scratch) CreateFile(name string) (*os.File, error) {
	if name != "descriptor.json" && name != "workspace.sqlite" && name != "workspace.sqlite-journal" {
		return nil, errors.New("invalid scratch file")
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	f, err := openRelativePinned(s.pins[len(s.pins)-1], name, win.GENERIC_READ|win.GENERIC_WRITE, win.FILE_SHARE_READ, win.CREATE_NEW, false)
	if err != nil {
		return nil, retainPhysicalClose(err, s)
	}
	// Transfer the handle to os.File; File has no finalizer and is not reused.
	return os.NewFile(uintptr(f.handle), "private-scratch-file"), nil
}
