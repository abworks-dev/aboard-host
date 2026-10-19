//go:build windows && amd64

package windows

import (
	"errors"
	"path/filepath"
	"runtime"
	"unsafe"

	win "golang.org/x/sys/windows"
)

// CreatePrivateDirectory creates exactly one NEW directory, never parents or a
// repair. The caller owns its lifetime and must pin it before subsequent IO.
// Its protected DACL grants inheritable full access only to TokenUser and SYSTEM.
func CreatePrivateDirectory(path string) (err error) {
	path, err = supportedPath(path)
	if err != nil {
		return err
	}
	pins, err := pinDirectories(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() {
		for i := len(pins) - 1; i >= 0; i-- {
			err = errors.Join(err, pins[i].Close())
		}
	}()
	sid, err := privateUserSID()
	if err != nil {
		return err
	}
	sd, err := win.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;OICI;FA;;;" + sid.String() + ")(A;OICI;FA;;;SY)")
	if err != nil {
		return err
	}
	name, err := win.NewNTUnicodeString(filepath.Base(path))
	if err != nil {
		return err
	}
	oa := win.OBJECT_ATTRIBUTES{RootDirectory: pins[len(pins)-1].handle, ObjectName: name, Attributes: win.OBJ_CASE_INSENSITIVE | win.OBJ_DONT_REPARSE, SecurityDescriptor: sd}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h win.Handle
	var ios win.IO_STATUS_BLOCK
	err = win.NtCreateFile(&h, win.READ_CONTROL|win.FILE_READ_ATTRIBUTES|win.FILE_LIST_DIRECTORY|win.SYNCHRONIZE, &oa, &ios, nil, win.FILE_ATTRIBUTE_NORMAL, win.FILE_SHARE_READ, win.FILE_CREATE, win.FILE_DIRECTORY_FILE|win.FILE_OPEN_REPARSE_POINT|win.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	runtime.KeepAlive(sd)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, win.CloseHandle(h)) }()
	if _, _, err = inspectHandle(h, path); err != nil {
		return err
	}
	return checkPrivateHandle(h, sid)
}

// CheckPrivateDirectory inspects an existing directory via pinned, no-reparse
// ancestors. It never changes the owner/DACL and rejects NULL/unprotected ACLs,
// inherited/unknown ACEs, additional principals, and missing required grants.
// This is a point-in-time check, NOT a retained authorization or lifetime pin.
func CheckPrivateDirectory(path string) (err error) {
	path, err = supportedPath(path)
	if err != nil {
		return err
	}
	pins, err := pinDirectories(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() {
		for i := len(pins) - 1; i >= 0; i-- {
			err = errors.Join(err, pins[i].Close())
		}
	}()
	f, err := openRelativePinned(pins[len(pins)-1], filepath.Base(path), win.READ_CONTROL|win.FILE_READ_ATTRIBUTES|win.FILE_LIST_DIRECTORY, win.FILE_SHARE_READ, win.OPEN_EXISTING, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	sid, err := privateUserSID()
	if err != nil {
		return err
	}
	return checkPrivateHandle(f.handle, sid)
}

func privateUserSID() (*win.SID, error) {
	t, err := win.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer t.Close()
	u, err := t.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return u.User.Sid.Copy()
}

func checkPrivateHandle(h win.Handle, sid *win.SID) error {
	sd, err := win.GetSecurityInfo(h, win.SE_FILE_OBJECT, win.OWNER_SECURITY_INFORMATION|win.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil || !owner.Equals(sid) {
		return errors.New("private directory owner mismatch")
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if control&win.SE_DACL_PROTECTED == 0 {
		return errors.New("private directory DACL is not protected")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil || acl.AceCount != 2 {
		return errors.New("private directory requires exactly owner and SYSTEM grants")
	}
	system, err := win.CreateWellKnownSid(win.WinLocalSystemSid)
	if err != nil {
		return err
	}
	seenOwner, seenSystem := false, false
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *win.ACCESS_ALLOWED_ACE
		if err = win.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != win.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != win.OBJECT_INHERIT_ACE|win.CONTAINER_INHERIT_ACE || uint32(ace.Mask) != 0x001f01ff {
			return errors.New("private directory has unsafe grant")
		}
		asid := (*win.SID)(unsafe.Pointer(&ace.SidStart))
		if asid.Equals(sid) && !seenOwner {
			seenOwner = true
		} else if asid.Equals(system) && !seenSystem {
			seenSystem = true
		} else {
			return errors.New("private directory grants another principal")
		}
	}
	runtime.KeepAlive(sd)
	if !seenOwner || !seenSystem {
		return errors.New("private directory required grant missing")
	}
	return nil
}
