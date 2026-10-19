//go:build windows && amd64

package windows

import (
	"encoding/binary"
	"errors"
	"io"
	"path/filepath"
	"sync"
	"unicode/utf16"
	"unsafe"

	win "golang.org/x/sys/windows"
)

var ErrDirectoryNotEmpty = errors.New("workspace creation requires an empty directory")

// WorkspaceCreation retains every ancestor, including an existing empty target.
// No absolute-path create/write or post-hoc pin is used. Failed artifacts remain.
// The business owner must close SQLite before Close, including factory failures.
type WorkspaceCreation struct {
	parents              []*File
	root                 *File
	lease                *Lease
	database, descriptor *File
	fresh                *FreshWorkspaceDatabase
	path                 string
}

// InspectWorkspaceCreation checks only an existing empty target or its existing
// parent. It performs no target writes; Prepare is called after durable intent.
func InspectWorkspaceCreation(path string) (_ *WorkspaceCreation, err error) {
	path, err = supportedPath(path)
	if err != nil {
		return nil, err
	}
	parents, err := pinDirectories(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	c := &WorkspaceCreation{parents: parents, path: path}
	root, openErr := openRelativePinned(parents[len(parents)-1], filepath.Base(path), win.FILE_LIST_DIRECTORY|win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ, win.OPEN_EXISTING, true)
	if openErr == nil {
		c.root = root
		if err = requireEmptyDirectory(root); err != nil {
			if endErr := c.Close(); endErr != nil {
				return c, errors.Join(err, endErr)
			}
			return nil, err
		}
	} else {
		// Missing is the ONLY acceptable existing-open failure. NT relative opens
		// return NTSTATUS, not errno; do not reinterpret access/busy as first use.
		var status win.NTStatus
		if !errors.As(openErr, &status) || (status != win.STATUS_OBJECT_NAME_NOT_FOUND && status != win.STATUS_OBJECT_PATH_NOT_FOUND) {
			if endErr := c.Close(); endErr != nil {
				return c, errors.Join(openErr, endErr)
			}
			return nil, openErr
		}
	}
	return c, nil
}

func requireEmptyDirectory(dir *File) error {
	names, err := directoryNames(dir)
	if err != nil {
		return err
	}
	if len(names) != 0 {
		return ErrDirectoryNotEmpty
	}
	return nil
}

// directoryNames queries the HELD directory, not an absolute name that can be
// reparsed in place by a metadata-only FILE_WRITE_ATTRIBUTES handle.
func directoryNames(dir *File) ([]string, error) {
	if err := dir.Validate(); err != nil {
		return nil, err
	}
	var names []string
	class := uint32(15) // FileFullDirectoryRestartInfo
	for {
		// FILE_FULL_DIR_INFO requires eight-byte aligned output. A byte array on
		// the Go stack has only byte alignment and can produce ERROR_NOACCESS.
		var aligned [8192]uint64
		buf := unsafe.Slice((*byte)(unsafe.Pointer(&aligned[0])), len(aligned)*8)
		err := win.GetFileInformationByHandleEx(dir.handle, class, &buf[0], uint32(len(buf)))
		if errors.Is(err, win.ERROR_NO_MORE_FILES) {
			break
		}
		if err != nil {
			return nil, err
		}
		class = 14 // FileFullDirectoryInfo
		for off := 0; ; {
			if off+68 > len(buf) {
				return nil, errors.New("invalid held directory response")
			}
			next := int(binary.LittleEndian.Uint32(buf[off:]))
			size := int(binary.LittleEndian.Uint32(buf[off+60:]))
			if size%2 != 0 || off+68+size > len(buf) {
				return nil, errors.New("invalid held directory name")
			}
			chars := make([]uint16, size/2)
			for i := range chars {
				chars[i] = binary.LittleEndian.Uint16(buf[off+68+i*2:])
			}
			name := string(utf16.Decode(chars))
			if name != "." && name != ".." {
				names = append(names, name)
			}
			if next == 0 {
				break
			}
			if next < 68 || off+next >= len(buf) {
				return nil, errors.New("invalid held directory offset")
			}
			off += next
		}
	}
	return names, dir.Validate()
}

// Prepare creates only approved fresh direct children through retained parents.
// The DB is pinned BEFORE any SQLite open or first write. No cleanup is repair.
func (c *WorkspaceCreation) Prepare() (err error) {
	if c == nil || c.lease != nil || c.database != nil {
		return errors.New("creation already prepared")
	}
	parent := c.parents[len(c.parents)-1]
	if c.root == nil {
		c.root, err = openRelativePinned(parent, filepath.Base(c.path), win.FILE_LIST_DIRECTORY|win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ, win.CREATE_NEW, true)
		if err != nil {
			return err
		}
	}
	if err = requireEmptyDirectory(c.root); err != nil {
		return err
	}
	c.lease = &Lease{root: c.path, id: c.root.id, pins: append(append([]*File{}, c.parents...), c.root)}
	for _, name := range []string{".aboard", "locks"} {
		p := c.lease.pins[len(c.lease.pins)-1]
		child, e := openRelativePinned(p, name, win.FILE_LIST_DIRECTORY|win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ, win.CREATE_NEW, true)
		if e != nil {
			return e
		}
		c.lease.pins = append(c.lease.pins, child)
	}
	c.lease.lock, err = openRelativePinned(c.lease.pins[len(c.lease.pins)-1], "writer.lock", win.GENERIC_READ|win.GENERIC_WRITE, 0, win.CREATE_NEW, false)
	if err != nil {
		return err
	}
	data, err := openRelativePinned(c.root, "data", win.FILE_LIST_DIRECTORY|win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ, win.CREATE_NEW, true)
	if err != nil {
		return err
	}
	c.lease.pins = append(c.lease.pins, data)
	c.database, err = openRelativePinned(data, "workspace.sqlite", win.GENERIC_READ|win.GENERIC_WRITE, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE, win.CREATE_NEW, false)
	if err != nil {
		return err
	}
	c.lease.keys = append(c.lease.keys, c.database)
	c.fresh = &FreshWorkspaceDatabase{owner: c}
	return c.Validate()
}
func (c *WorkspaceCreation) Validate() error {
	if c == nil || c.lease == nil || c.lease.lock == nil || c.database == nil {
		return errors.New("incomplete creation pins")
	}
	return c.lease.Validate()
}
func (c *WorkspaceCreation) RootIdentity() Identity                 { return c.root.id }
func (c *WorkspaceCreation) DatabaseIdentity() Identity             { return c.database.id }
func (c *WorkspaceCreation) FreshDatabase() *FreshWorkspaceDatabase { return c.fresh }
func (c *WorkspaceCreation) DescriptorIdentity() Identity {
	if c.descriptor == nil {
		return Identity{}
	}
	return c.descriptor.id
}

// PublishDescriptor performs CREATE_NEW and flushes the exact descriptor bytes.
// It does not rename over a residual descriptor or fill in a missing old file.
func (c *WorkspaceCreation) PublishDescriptor(b []byte) (err error) {
	if err = c.Validate(); err != nil {
		return err
	}
	if c.descriptor != nil {
		return errors.New("descriptor already attempted")
	}
	c.descriptor, err = openRelativePinned(c.root, "aboard.workspace.json", win.GENERIC_READ|win.GENERIC_WRITE, win.FILE_SHARE_READ, win.CREATE_NEW, false)
	if err != nil {
		return err
	}
	c.lease.keys = append(c.lease.keys, c.descriptor)
	var n uint32
	if err = win.WriteFile(c.descriptor.handle, b, &n, nil); err != nil {
		return err
	}
	if int(n) != len(b) {
		return io.ErrShortWrite
	}
	if err = win.FlushFileBuffers(c.descriptor.handle); err != nil {
		return err
	}
	return c.Validate()
}
func (c *WorkspaceCreation) Close() error {
	if c == nil {
		return nil
	}
	if c.lease != nil {
		for _, p := range c.lease.keys {
			if err := p.Close(); err != nil {
				return err
			}
		}
		// Partial Prepare can fail before a lock exists. Close retained pins without
		// inventing a valid lease; never delete the newly created artifacts.
		if c.lease.lock != nil {
			return c.lease.Close()
		}
		for i := len(c.lease.pins) - 1; i >= 0; i-- {
			if err := c.lease.pins[i].Close(); err != nil {
				return err
			}
		}
		return nil
	}
	if c.root != nil {
		if err := c.root.Close(); err != nil {
			return err
		}
	}
	for i := len(c.parents) - 1; i >= 0; i-- {
		if err := c.parents[i].Close(); err != nil {
			return err
		}
	}
	return nil
}

// FreshWorkspaceDatabase is opaque, minted only after relative CREATE_NEW under
// this live creation's writer lock. Its one-shot claim cannot initialize an old
// or reopened zero-byte file, and is not serializable across a process restart.
type FreshWorkspaceDatabase struct {
	mu      sync.Mutex
	owner   *WorkspaceCreation
	claimed bool
}

func (f *FreshWorkspaceDatabase) ClaimInitialization() (string, error) {
	if f == nil {
		return "", errors.New("missing fresh database proof")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimed {
		return "", errors.New("fresh database proof already consumed")
	}
	f.claimed = true
	if err := f.ValidateInitialization(); err != nil {
		return "", err
	}
	var info win.ByHandleFileInformation
	if err := win.GetFileInformationByHandle(f.owner.database.handle, &info); err != nil {
		return "", err
	}
	if info.FileSizeHigh != 0 || info.FileSizeLow != 0 {
		return "", errors.New("fresh database is not empty")
	}
	return f.owner.database.path, nil
}
func (f *FreshWorkspaceDatabase) ValidateInitialization() error {
	if f == nil || f.owner == nil {
		return errors.New("missing fresh database owner")
	}
	return f.owner.Validate()
}

// ReadPinnedWorkspaceFile reads only an existing lease-approved descriptor.
func ReadPinnedWorkspaceFile(f *File, limit int) ([]byte, error) {
	if f == nil || f.dir || filepath.Base(f.path) != "aboard.workspace.json" || limit < 1 {
		return nil, errors.New("not a descriptor pin")
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if _, err := win.SetFilePointer(f.handle, 0, nil, 0); err != nil {
		return nil, err
	}
	b := make([]byte, limit+1)
	var n uint32
	if err := win.ReadFile(f.handle, b, &n, nil); err != nil && !errors.Is(err, win.ERROR_HANDLE_EOF) {
		return nil, err
	}
	if int(n) > limit {
		return nil, errors.New("descriptor too large")
	}
	return b[:n], f.Validate()
}

// WorkspaceDataHasOnlyDatabase rejects all sidecars before a non-recovering
// reconcile open. The data directory is queried via its retained relative pin.
func (l *Lease) WorkspaceDataHasOnlyDatabase() error {
	var root *File
	for _, p := range l.pins {
		if sameDOSSpelling(p.path, l.root) {
			root = p
			break
		}
	}
	if root == nil {
		return errors.New("missing root pin")
	}
	data, err := openRelativePinned(root, "data", win.FILE_LIST_DIRECTORY|win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ, win.OPEN_EXISTING, true)
	if err != nil {
		return err
	}
	// Transfer ownership before querying. Never drop a temporary directory pin
	// through an unchecked defer Close; the caller's lease owns every outcome.
	l.pins = append(l.pins, data)
	names, err := directoryNames(data)
	if err != nil {
		return err
	}
	if len(names) != 1 || names[0] != "workspace.sqlite" {
		return errors.New("creation database has residual files")
	}
	return l.Validate()
}
