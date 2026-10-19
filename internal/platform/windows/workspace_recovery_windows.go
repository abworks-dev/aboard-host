//go:build windows && amd64

package windows

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	win "golang.org/x/sys/windows"
)

var ErrRecoveryInputCloseUnknown = errors.New("workspace recovery input physical close not confirmed")

// FrozenInputSize reads the size from the actual retained object, never a path.
func (f *File) FrozenInputSize() (int64, error) {
	if err := f.Validate(); err != nil {
		return 0, err
	}
	var info win.ByHandleFileInformation
	if err := win.GetFileInformationByHandle(f.handle, &info); err != nil {
		return 0, err
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size > uint64(1<<63-1) {
		return 0, errors.New("recovery input too large")
	}
	return int64(size), nil
}

// FrozenWorkspaceInputs is an optional recovery preflight view under the actual
// writer lease. These temporary freeze pins must be physically closed before
// SQLite can recover the source. The ordinary database/root pins stay retained.
// It grants neither authorization nor permission to replay any business work.
type FrozenWorkspaceInputs struct{ Database, Journal *File }

func (f *FrozenWorkspaceInputs) Close() error {
	var errs []error
	if f.Journal != nil {
		errs = append(errs, f.Journal.Close())
	}
	if f.Database != nil {
		errs = append(errs, f.Database.Close())
	}
	return errors.Join(errs...)
}

// FreezeWorkspaceOpenInputs rejects every unsupported data-directory shape,
// including WAL/SHM, super-journals, aliases and unknown files. Only the fixed
// rollback journal is eligible for a caller's explicit recovery evaluation.
// A nonnil result accompanies partial acquisition failure and remains owned.
func (l *Lease) FreezeWorkspaceOpenInputs() (*FrozenWorkspaceInputs, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	var root *File
	for _, p := range l.pins {
		if sameDOSSpelling(p.path, l.root) {
			root = p
			break
		}
	}
	if root == nil {
		return nil, errors.New("missing root pin")
	}
	// Repeated explicit investigations keep the same namespace exclusion. Reuse
	// an already validated data pin (PinFile acquired the identical directory
	// access), rather than accumulating an unbounded handle for every retry.
	var data *File
	for _, p := range l.pins {
		if p.dir && sameDOSSpelling(p.path, filepath.Join(l.root, "data")) {
			data = p
			break
		}
	}
	if data == nil {
		var err error
		data, err = openRelativePinned(root, "data", win.FILE_LIST_DIRECTORY|win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ, win.OPEN_EXISTING, true)
		if err != nil {
			return nil, err
		}
		l.pins = append(l.pins, data)
	}
	names, err := directoryNames(data)
	if err != nil {
		return nil, err
	}
	main, journal := false, false
	for _, name := range names {
		switch name {
		case "workspace.sqlite":
			main = true
		case "workspace.sqlite-journal":
			journal = true
		default:
			return nil, errors.New("unsupported workspace storage sidecar")
		}
	}
	if !main || len(names) > 2 {
		return nil, errors.New("missing workspace database")
	}
	f := &FrozenWorkspaceInputs{}
	f.Database, err = openRelativePinned(data, "workspace.sqlite", win.GENERIC_READ, win.FILE_SHARE_READ, win.OPEN_EXISTING, false)
	if err != nil {
		return f, err
	}
	if journal {
		f.Journal, err = openRelativePinned(data, "workspace.sqlite-journal", win.GENERIC_READ, win.FILE_SHARE_READ, win.OPEN_EXISTING, false)
	}
	if err != nil {
		return f, err
	}
	return f, l.Validate()
}

// CopyFrozenWorkspaceInput only copies fixed DB/journal objects through retained
// read handles. The destination is a trusted private scratch writer, not a path.
func (f *File) CopyFrozenWorkspaceInput(out io.Writer) (err error) {
	if f == nil || f.dir || (filepath.Base(f.path) != "workspace.sqlite" && filepath.Base(f.path) != "workspace.sqlite-journal") {
		return errors.New("not a workspace recovery input")
	}
	if err = f.Validate(); err != nil {
		return err
	}
	var duplicate win.Handle
	process := win.CurrentProcess()
	if err = win.DuplicateHandle(process, f.handle, process, &duplicate, 0, false, win.DUPLICATE_SAME_ACCESS); err != nil {
		return err
	}
	in := os.NewFile(uintptr(duplicate), "frozen-workspace-recovery-input")
	defer func() {
		if closeErr := in.Close(); closeErr != nil {
			err = errors.Join(err, ErrRecoveryInputCloseUnknown, closeErr)
		}
	}()
	if _, err = in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return f.Validate()
}
