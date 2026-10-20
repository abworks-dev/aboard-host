//go:build windows && amd64

// Package windows provides the narrow Windows/NTFS handle primitives used by
// workspace adapters. Handles are evidence of exclusion, not authorization or
// an OS sandbox. SQLite and logical workspace identity have different owners.
package windows

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	win "golang.org/x/sys/windows"
)

// Identity describes this physical object on this volume, not a workspace UUID.
// File IDs may be reused after deletion; callers must not treat them as grants.
type Identity struct {
	Volume uint64
	FileID [16]byte
}

type Scope uint8

const (
	Installation Scope = iota + 1
	Workspace
)

// File is held open, denies deletion, and is explicitly non-inheritable.
// Freeze denies write sharing as well. No finalizer silently releases a lease.
type File struct {
	handle win.Handle
	path   string
	id     Identity
	dir    bool
	closed bool
}

func (f *File) Identity() Identity { return f.id }
func (f *File) Handle() win.Handle { return f.handle }
func (f *File) Path() string       { return f.path }
func (f *File) Close() error {
	if f.closed {
		return nil
	}
	if err := win.CloseHandle(f.handle); err != nil {
		return physicalCloseUnknown(err, f)
	}
	f.closed = true
	return nil
}

func (f *File) Validate() error {
	if f.closed {
		return errors.New("closed file pin")
	}
	id, dir, err := inspectHandle(f.handle, f.path)
	if err != nil {
		return err
	}
	if id != f.id || dir != f.dir {
		return errors.New("physical identity changed")
	}
	return nil
}

// Lease owns ancestor pins and one fixed lock object. The caller must physically
// drain/close all users before Close. It must not delete or recreate the lock.
type Lease struct {
	root string
	pins []*File
	keys []*File
	lock *File
	id   Identity
}

func (l *Lease) RootIdentity() Identity { return l.id }
func (l *Lease) RootPath() string       { return l.root }
func (l *Lease) LockIdentity() Identity { return l.lock.id }
func (l *Lease) LockHandle() win.Handle { return l.lock.handle }
func (l *Lease) Validate() error {
	for _, p := range l.pins {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	for _, p := range l.keys {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return l.lock.Validate()
}

// ClosePinnedFiles drains every key owned by this lease, including a key whose
// post-open validation failed before PinFile could return it. No IO user may be
// active. Close still independently refuses live keys before releasing its lock.
func (l *Lease) ClosePinnedFiles() error {
	for i := len(l.keys) - 1; i >= 0; i-- {
		if err := l.keys[i].Close(); err != nil {
			return retainPhysicalClose(err, l)
		}
	}
	return nil
}

func (l *Lease) Close() error {
	// This checks tracked file pins, but cannot observe arbitrary SQLite,
	// worker or provider IO: the caller still owns physical drain.
	for _, p := range l.keys {
		if !p.closed {
			return errors.New("close key-file pins before releasing the writer lock")
		}
	}
	// Exclusion is last: an ancestor pin close failure must not release the
	// writer/installation lock while earlier native resources remain unknown.
	if err := closePins(l.pins); err != nil {
		return retainPhysicalClose(err, l)
	}
	return retainPhysicalClose(l.lock.Close(), l)
}

// Acquire opens an existing directory and fixed lock. create only creates the
// new lock with CREATE_NEW; it never creates parents, repairs, or replaces.
// readOnly is for closed-directory inspection, and cannot be combined with create.
func Acquire(root string, scope Scope, create, readOnly bool) (_ *Lease, err error) {
	root, err = supportedPath(root)
	if err != nil {
		return nil, err
	}
	var rel string
	switch scope {
	case Installation:
		rel = "host.lock"
	case Workspace:
		rel = `.aboard\locks\writer.lock`
	default:
		return nil, errors.New("unknown lock scope")
	}
	if create && readOnly {
		return nil, errors.New("read-only inspection cannot create locks")
	}
	pins, err := pinDirectories(filepath.Dir(filepath.Join(root, rel)))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = unwindPins(err, pins)
		}
	}()
	var rootID Identity
	for _, p := range pins {
		if sameDOSSpelling(p.path, root) {
			rootID = p.id
		}
	}
	access := uint32(win.GENERIC_READ)
	if !readOnly {
		access |= win.GENERIC_WRITE
	}
	creation := uint32(win.OPEN_EXISTING)
	if create {
		creation = win.CREATE_NEW
	}
	lock, err := openRelativePinned(pins[len(pins)-1], filepath.Base(rel), access, 0, creation, false)
	if err != nil {
		return nil, err
	}
	lease := &Lease{root: root, pins: pins, lock: lock, id: rootID}
	if err = lease.Validate(); err != nil {
		return nil, unwindPins(err, []*File{lock}, lease)
	}
	return lease, nil
}

// PinFile holds an existing direct child, never a path with caller-controlled
// traversal. A frozen file is opened GENERIC_READ/share-read only for inspect;
// an ordinary file pin uses read/share-read-write for the SQLite owner. Merely
// opening metadata-only does not establish the required sharing exclusion.
// The caller must close returned files before releasing its lease. Once any
// tracked key is closed, this lease's protected view is invalid: Validate and
// further PinFile calls fail. Release all remaining keys and the lease; callers
// needing a new view must physically drain and reacquire, not reuse a tombstone.
func (l *Lease) PinFile(relative string, freeze bool) (_ *File, err error) {
	if err = l.Validate(); err != nil {
		return nil, err
	}
	if relative != "aboard.workspace.json" && relative != `data\workspace.sqlite` {
		return nil, errors.New("not an approved workspace key file")
	}
	path := filepath.Join(l.root, relative)
	// Pin the data directory as well; a final-file handle alone does not pin
	// replacement of an ancestor. Keep these pins until the lease is closed.
	var rootPin *File
	for _, p := range l.pins {
		if sameDOSSpelling(p.path, l.root) {
			rootPin = p
			break
		}
	}
	if rootPin == nil {
		return nil, errors.New("missing pinned root")
	}
	parent := rootPin
	if relative == `data\workspace.sqlite` {
		d, e := openRelativePinned(rootPin, "data", win.FILE_READ_ATTRIBUTES|win.FILE_LIST_DIRECTORY, win.FILE_SHARE_READ, win.OPEN_EXISTING, true)
		if e != nil {
			return nil, e
		}
		l.pins = append(l.pins, d)
		parent = d
	}
	access, share := uint32(win.GENERIC_READ), uint32(win.FILE_SHARE_READ|win.FILE_SHARE_WRITE)
	if freeze {
		access, share = win.GENERIC_READ, win.FILE_SHARE_READ
	}
	f, err := openRelativePinned(parent, filepath.Base(path), access, share, win.OPEN_EXISTING, false)
	if err != nil {
		return nil, err
	}
	l.keys = append(l.keys, f)
	if err := l.Validate(); err != nil {
		return nil, retainPhysicalClose(errors.Join(err, f.Close()), l, f)
	}
	return f, nil
}

func supportedPath(path string) (string, error) {
	// Reject UNC/device/relative, ADS, drive aliases expressed as device paths,
	// ambiguous components and Win32 trailing-dot/space normalization.
	if len(path) < 3 || path[1:3] != `:\` || !isLetter(path[0]) || strings.ContainsAny(path[2:], "/:\x00") {
		return "", errors.New("only absolute local DOS paths are supported")
	}
	if path != filepath.Clean(path) {
		return "", errors.New("noncanonical path components")
	}
	for _, part := range strings.Split(path[3:], `\`) {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return "", errors.New("ambiguous path component")
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (base[:3] == "COM" || base[:3] == "LPT") && base[3] >= '0' && base[3] <= '9') {
			return "", errors.New("reserved DOS path component")
		}
	}
	vol := path[:3]
	p, _ := win.UTF16PtrFromString(vol)
	if win.GetDriveType(p) != win.DRIVE_FIXED {
		return "", errors.New("only fixed local volumes are supported")
	}
	var fs [64]uint16
	if err := win.GetVolumeInformation(p, nil, 0, nil, nil, nil, &fs[0], uint32(len(fs))); err != nil {
		return "", err
	}
	if win.UTF16ToString(fs[:]) != "NTFS" {
		return "", errors.New("unverified filesystem")
	}
	return path, nil
}
func isLetter(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' }

// sameDOSSpelling accepts only the DOS drive-letter case alias. All names must
// retain exact bytes: Unicode EqualFold is not an NTFS identity/authorization
// proof, nor does it account for per-directory case-sensitive behavior.
func sameDOSSpelling(a, b string) bool {
	return len(a) >= 3 && len(b) >= 3 && isLetter(a[0]) && isLetter(b[0]) &&
		a[1:3] == `:\` && b[1:3] == `:\` &&
		(a[0]|0x20) == (b[0]|0x20) && a[1:] == b[1:]
}

func pinDirectories(path string) (pins []*File, err error) {
	// Resolve each component relative to the held parent, never by re-resolving
	// an absolute pathname. Real read access makes sharing exclusion effective,
	// but metadata-only FSCTL can still reparse an empty directory: relative NT
	// opens with OBJ_DONT_REPARSE reject that race rather than following it.
	// Retained children prevent their parent becoming empty during later IO.
	parts := []string{path[:3]}
	cur := path[:3]
	for _, component := range strings.Split(path[3:], `\`) {
		if component == "" {
			continue
		}
		cur = filepath.Join(cur, component)
		parts = append(parts, cur)
	}
	for i, part := range parts {
		var p *File
		var e error
		if i == 0 {
			p, e = openPinned(part, win.FILE_READ_ATTRIBUTES|win.FILE_LIST_DIRECTORY, win.FILE_SHARE_READ, win.OPEN_EXISTING, true)
		} else {
			p, e = openRelativePinned(pins[len(pins)-1], filepath.Base(part), win.FILE_READ_ATTRIBUTES|win.FILE_LIST_DIRECTORY, win.FILE_SHARE_READ, win.OPEN_EXISTING, true)
		}
		if e != nil {
			return nil, unwindPins(e, pins)
		}
		pins = append(pins, p)
	}
	return pins, nil
}

func openPinned(path string, access, share, creation uint32, directory bool) (*File, error) {
	p, err := win.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := win.CreateFile(p, access, share, nil, creation, win.FILE_FLAG_BACKUP_SEMANTICS|win.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fmt.Errorf("open fixed object: %w", err)
	}
	return adoptPinned(h, path, directory)
}

// openRelativePinned is deliberately limited to a single approved child name.
// It does not grant an arbitrary native path/NT object-manager interface.
func openRelativePinned(parent *File, name string, access, share, creation uint32, directory bool) (*File, error) {
	if parent == nil || !parent.dir || name == "" || name == "." || name == ".." || strings.ContainsAny(name, `\/:`+"\x00") {
		return nil, errors.New("invalid relative object")
	}
	if err := parent.Validate(); err != nil {
		return nil, err
	}
	n, err := win.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	// No OBJ_CASE_INSENSITIVE: a lexical allow-root must not be broadened by
	// native case lookup, including directories with case-sensitive semantics.
	oa := win.OBJECT_ATTRIBUTES{RootDirectory: parent.handle, ObjectName: n, Attributes: win.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	disposition := uint32(win.FILE_OPEN)
	if creation == win.CREATE_NEW {
		disposition = win.FILE_CREATE
	} else if creation != win.OPEN_EXISTING {
		return nil, errors.New("unsupported creation disposition")
	}
	options := uint32(win.FILE_SYNCHRONOUS_IO_NONALERT | win.FILE_OPEN_REPARSE_POINT)
	if directory {
		options |= win.FILE_DIRECTORY_FILE
	} else {
		options |= win.FILE_NON_DIRECTORY_FILE
	}
	var h win.Handle
	var ios win.IO_STATUS_BLOCK
	if err := win.NtCreateFile(&h, access|win.SYNCHRONIZE, &oa, &ios, nil, win.FILE_ATTRIBUTE_NORMAL, share, disposition, options, 0, 0); err != nil {
		return nil, fmt.Errorf("relative fixed object: %w", err)
	}
	if err := parent.Validate(); err != nil {
		f := &File{handle: h, path: filepath.Join(parent.path, name), dir: directory}
		return nil, retainPhysicalClose(errors.Join(err, f.Close()), parent, f)
	}
	f, err := adoptPinned(h, filepath.Join(parent.path, name), directory)
	return f, retainPhysicalClose(err, parent)
}

func adoptPinned(h win.Handle, path string, directory bool) (*File, error) {
	f := &File{handle: h, path: path, dir: directory}
	fail := func(e error) (*File, error) { return nil, errors.Join(e, f.Close()) }
	if err := win.SetHandleInformation(h, win.HANDLE_FLAG_INHERIT, 0); err != nil {
		return fail(err)
	}
	id, dir, err := inspectHandle(h, path)
	if err != nil {
		return fail(err)
	}
	if dir != directory {
		return fail(errors.New("unexpected file/directory kind"))
	}
	f.id, f.dir = id, dir
	return f, nil
}

func inspectHandle(h win.Handle, path string) (Identity, bool, error) {
	var id Identity
	var info win.ByHandleFileInformation
	if err := win.GetFileInformationByHandle(h, &info); err != nil {
		return id, false, err
	}
	dir := info.FileAttributes&win.FILE_ATTRIBUTE_DIRECTORY != 0
	if info.FileAttributes&win.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (!dir && info.NumberOfLinks != 1) {
		return id, dir, errors.New("reparse point or hard-linked key object")
	}
	if typ, err := win.GetFileType(h); err != nil || typ != win.FILE_TYPE_DISK {
		return id, dir, errors.Join(errors.New("not a disk object"), err)
	}
	if err := win.GetFileInformationByHandleEx(h, win.FileIdInfo, (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id))); err != nil {
		return id, dir, err
	}
	var buf [32768]uint16
	n, err := win.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil || n == 0 || n >= uint32(len(buf)) {
		return id, dir, errors.Join(errors.New("cannot determine final DOS path"), err)
	}
	final := strings.TrimPrefix(win.UTF16ToString(buf[:n]), `\\?\`)
	if !sameDOSSpelling(final, path) {
		return id, dir, errors.New("path alias or changed resolution")
	}
	return id, dir, nil
}
