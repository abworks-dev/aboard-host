//go:build windows && amd64

package windows

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	win "golang.org/x/sys/windows"
)

// ValidateInstallation requires the actual retained host.lock, not a workspace
// lease or a caller's claim about its scope. Closed key pins poison this view.
func (l *Lease) ValidateInstallation(root string) error {
	if l == nil || l.lock == nil {
		return errors.New("missing installation lease")
	}
	canonical, err := supportedPath(root)
	if err != nil {
		return err
	}
	if !sameDOSSpelling(canonical, l.root) || !sameDOSSpelling(l.lock.path, filepath.Join(l.root, "host.lock")) {
		return errors.New("installation lease scope/root mismatch")
	}
	return l.Validate()
}

func (l *Lease) installationRoot() (*File, error) {
	if err := l.ValidateInstallation(l.root); err != nil {
		return nil, err
	}
	for _, p := range l.pins {
		if sameDOSSpelling(p.path, l.root) {
			return p, nil
		}
	}
	return nil, errors.New("missing installation root pin")
}

// InstallationDatabaseHasNoSidecars checks the held native directory BEFORE any
// writable SQLite open. An unproved installation journal must not cause implicit
// rollback of an unfinished genesis just to discover that its schema is invalid.
// This is refusal only, not an installation recovery or migration entry point.
func (l *Lease) InstallationDatabaseHasNoSidecars() error {
	root, err := l.installationRoot()
	if err != nil {
		return err
	}
	var parent *File
	for _, pin := range l.pins {
		if sameDOSSpelling(pin.path, filepath.Dir(l.root)) {
			parent = pin
			break
		}
	}
	if parent == nil {
		return errors.New("missing installation parent pin")
	}
	listed, err := openRelativePinned(parent, filepath.Base(l.root), win.FILE_LIST_DIRECTORY|win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ, win.OPEN_EXISTING, true)
	if err != nil {
		return err
	}
	// Publish ownership before all subsequent validation/query failures. Close
	// uncertainty is then retained by the original lease/Store, not a defer.
	l.pins = append(l.pins, listed)
	if listed.id != root.id {
		return errors.New("installation directory identity changed")
	}
	names, err := directoryNames(listed)
	if err != nil {
		return err
	}
	for _, name := range names {
		if strings.HasPrefix(strings.ToLower(name), "installation.sqlite-") {
			return errors.New("installation database sidecar requires investigation")
		}
	}
	return l.ValidateInstallation(l.root)
}

// PinInstallationFile pins only the two installation-owned fixed files. The
// database pin permits SQLite writes, but never delete sharing; bootstrap is
// frozen. Native relative opens reject reparses at the actual open instant.
func (l *Lease) PinInstallationFile(name string) (*File, error) {
	parent, err := l.installationRoot()
	if err != nil {
		return nil, err
	}
	share := uint32(win.FILE_SHARE_READ)
	if name == "installation.sqlite" {
		share |= win.FILE_SHARE_WRITE
	} else if name != "bootstrap.dpapi" {
		return nil, errors.New("not an approved installation file")
	}
	f, err := openRelativePinned(parent, name, win.GENERIC_READ|win.READ_CONTROL, share, win.OPEN_EXISTING, false)
	if err != nil {
		return nil, err
	}
	l.keys = append(l.keys, f)
	if err = l.Validate(); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if err = checkInstallationFileACL(f, name); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}

// ValidateInstallationFile proves membership in this lease, not just a matching
// path/handle supplied by another owner.
func (l *Lease) ValidateInstallationFile(root, name string, f *File) error {
	if err := l.ValidateInstallation(root); err != nil {
		return err
	}
	if f == nil || (name != "bootstrap.dpapi" && name != "installation.sqlite") || !sameDOSSpelling(f.path, filepath.Join(l.root, name)) {
		return errors.New("installation file scope mismatch")
	}
	for _, p := range l.keys {
		if p == f {
			return checkInstallationFileACL(f, name)
		}
	}
	return errors.New("installation file is not owned by lease")
}

// CreateInstallationMaterial creates only bootstrap.dpapi, with an explicit
// protected current-user/SYSTEM ACL. It never replaces or repairs an object.
func (l *Lease) CreateInstallationMaterial() (*File, error) {
	parent, err := l.installationRoot()
	if err != nil {
		return nil, err
	}
	sid, err := privateUserSID()
	if err != nil {
		return nil, err
	}
	sd, err := win.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;;FA;;;" + sid.String() + ")(A;;FA;;;SY)")
	if err != nil {
		return nil, err
	}
	name, err := win.NewNTUnicodeString("bootstrap.dpapi")
	if err != nil {
		return nil, err
	}
	oa := win.OBJECT_ATTRIBUTES{RootDirectory: parent.handle, ObjectName: name, Attributes: win.OBJ_DONT_REPARSE, SecurityDescriptor: sd}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h win.Handle
	var ios win.IO_STATUS_BLOCK
	err = win.NtCreateFile(&h, win.GENERIC_WRITE|win.GENERIC_READ|win.READ_CONTROL|win.SYNCHRONIZE, &oa, &ios, nil, win.FILE_ATTRIBUTE_NORMAL, win.FILE_SHARE_READ, win.FILE_CREATE, win.FILE_NON_DIRECTORY_FILE|win.FILE_OPEN_REPARSE_POINT|win.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	runtime.KeepAlive(sd)
	if err != nil {
		return nil, err
	}
	f, err := adoptPinned(h, filepath.Join(l.root, "bootstrap.dpapi"), false)
	if err != nil {
		return nil, err
	}
	l.keys = append(l.keys, f)
	if err = l.Validate(); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if err = checkInstallationFileACL(f, "bootstrap.dpapi"); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}

func checkInstallationFileACL(f *File, name string) error {
	if err := f.Validate(); err != nil {
		return err
	}
	sid, err := privateUserSID()
	if err != nil {
		return err
	}
	sd, err := win.GetSecurityInfo(f.handle, win.SE_FILE_OBJECT, win.OWNER_SECURITY_INFORMATION|win.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil || !owner.Equals(sid) {
		return errors.New("installation file owner mismatch")
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if name == "bootstrap.dpapi" && control&win.SE_DACL_PROTECTED == 0 {
		return errors.New("material DACL is not protected")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil || acl.AceCount != 2 {
		return errors.New("installation file requires owner and SYSTEM only")
	}
	system, err := win.CreateWellKnownSid(win.WinLocalSystemSid)
	if err != nil {
		return err
	}
	seenUser, seenSystem := false, false
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *win.ACCESS_ALLOWED_ACE
		if err = win.GetAce(acl, i, &ace); err != nil {
			return err
		}
		flags := ace.Header.AceFlags
		if ace.Header.AceType != win.ACCESS_ALLOWED_ACE_TYPE || uint32(ace.Mask) != 0x001f01ff || (flags != 0 && (name != "installation.sqlite" || flags != win.INHERITED_ACE)) {
			return errors.New("unsafe installation file grant")
		}
		s := (*win.SID)(unsafe.Pointer(&ace.SidStart))
		if s.Equals(sid) && !seenUser {
			seenUser = true
		} else if s.Equals(system) && !seenSystem {
			seenSystem = true
		} else {
			return errors.New("unexpected installation file principal")
		}
	}
	runtime.KeepAlive(sd)
	if !seenUser || !seenSystem {
		return errors.New("missing installation file grant")
	}
	return nil
}
