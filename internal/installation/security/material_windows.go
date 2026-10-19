//go:build windows && amd64

// Package security is the G5 technical prototype. It does not start a Host or
// integrate installation/workspace persistence. No API repairs partial installs.
package security

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"unsafe"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	win "golang.org/x/sys/windows"
)

const materialName = "bootstrap.dpapi"

// Material is installation-owned; it must never be a public DTO or log field.
// The key is deliberately unexported; operator clients never read this file.
type Material struct {
	InstallationID string `json:"installationId"`
	TenantID       string `json:"tenantId"`
	PrincipalID    string `json:"principalId"`
	key            [32]byte
}
type diskMaterial struct {
	InstallationID string
	TenantID       string
	PrincipalID    string
	Key            []byte
	Version        int
}

// Initialize preserves the G5 explicit-new-material API. Production installation
// persistence uses InitializeWithLease and owns the lease until its DB is closed.
func Initialize(root string) (err error) {
	if err = platform.CreatePrivateDirectory(root); err != nil {
		return errors.New("installation is not new or private")
	}
	lease, err := platform.Acquire(root, platform.Installation, true, false)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	m, pin, err := InitializeWithLease(root, lease)
	defer m.Clear()
	if pin != nil {
		defer func() { err = errors.Join(err, pin.Close()) }()
	}
	return err
}

// InitializeWithLease creates material only under the retained actual host.lock.
// The returned pin must remain held until all installation users physically end.
func InitializeWithLease(root string, lease *platform.Lease) (_ Material, _ *platform.File, err error) {
	var result Material
	if err = lease.ValidateInstallation(root); err != nil {
		return result, nil, err
	}
	if err = platform.CheckPrivateDirectory(root); err != nil {
		return result, nil, err
	}
	m := diskMaterial{Version: 1, Key: make([]byte, 32)}
	defer clear(m.Key)
	if _, err = rand.Read(m.Key); err != nil {
		return result, nil, err
	}
	for _, id := range []*string{&m.InstallationID, &m.TenantID, &m.PrincipalID} {
		var b [16]byte
		if _, err = rand.Read(b[:]); err != nil {
			return result, nil, err
		}
		b[6] = (b[6] & 0x0f) | 0x40
		b[8] = (b[8] & 0x3f) | 0x80
		s := hex.EncodeToString(b[:])
		*id = s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
	}
	plain, err := json.Marshal(m)
	if err != nil {
		return result, nil, err
	}
	defer clear(plain)
	blob, err := Protect(plain)
	if err != nil {
		return result, nil, err
	}
	defer clear(blob)
	pin, err := lease.CreateInstallationMaterial()
	if err != nil {
		return result, nil, err
	}
	var written uint32
	if err = win.WriteFile(pin.Handle(), blob, &written, nil); err == nil && int(written) != len(blob) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = win.FlushFileBuffers(pin.Handle())
	}
	if err != nil {
		return result, pin, err
	}
	result.InstallationID = m.InstallationID
	result.TenantID = m.TenantID
	result.PrincipalID = m.PrincipalID
	copy(result.key[:], m.Key)
	return result, pin, nil
}

// Load owns a temporary lease for the G5 API. It never creates or repairs.
func Load(root string) (result Material, err error) {
	if err = platform.CheckPrivateDirectory(root); err != nil {
		return result, err
	}
	lease, err := platform.Acquire(root, platform.Installation, false, false)
	if err != nil {
		return result, err
	}
	defer func() {
		err = errors.Join(err, lease.Close())
		if err != nil {
			result.Clear()
		}
	}()
	pin, err := lease.PinInstallationFile(materialName)
	if err != nil {
		return result, err
	}
	defer func() {
		err = errors.Join(err, pin.Close())
		if err != nil {
			result.Clear()
		}
	}()
	result, err = LoadWithLease(root, lease, pin)
	return result, err
}

// LoadWithLease authenticates a fixed material pin owned by the live installation
// lease. It neither reacquires nor releases host.lock; no unpinned path read occurs.
func LoadWithLease(root string, lease *platform.Lease, pin *platform.File) (Material, error) {
	var result Material
	if err := lease.ValidateInstallationFile(root, materialName, pin); err != nil {
		return result, err
	}
	if err := platform.CheckPrivateDirectory(root); err != nil {
		return result, err
	}
	sid, err := currentSID()
	if err != nil {
		return result, err
	}
	if err = checkPrivateObject(pin.Handle(), sid, false); err != nil {
		return result, err
	}
	// A duplicated non-inheritable handle owns only this read wrapper; the original
	// tracked pin stays open. Resetting its shared offset is safe under the owner.
	var duplicate win.Handle
	if err = win.DuplicateHandle(win.CurrentProcess(), pin.Handle(), win.CurrentProcess(), &duplicate, 0, false, win.DUPLICATE_SAME_ACCESS); err != nil {
		return result, err
	}
	f := os.NewFile(uintptr(duplicate), materialName)
	blob, readErr := func() ([]byte, error) {
		if _, e := f.Seek(0, io.SeekStart); e != nil {
			return nil, e
		}
		return io.ReadAll(io.LimitReader(f, 65537))
	}()
	err = errors.Join(readErr, f.Close())
	if err != nil {
		clear(blob)
		return result, err
	}
	defer clear(blob)
	if len(blob) > 65536 {
		return result, errors.New("invalid installation material")
	}
	plain, err := Unprotect(blob)
	if err != nil {
		return result, errors.New("installation material cannot be decrypted")
	}
	defer clear(plain)
	var m diskMaterial
	defer func() { clear(m.Key) }() // also clear partially decoded Key on decode errors
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&m); err != nil {
		return result, errors.New("invalid installation material")
	}
	if decoder.Decode(new(any)) != io.EOF || m.Version != 1 || len(m.Key) != 32 {
		return result, errors.New("invalid installation material")
	}
	for _, id := range []string{m.InstallationID, m.TenantID, m.PrincipalID} {
		if !canonicalUUIDv4(id) {
			return result, errors.New("invalid installation identity")
		}
	}
	if err = lease.ValidateInstallationFile(root, materialName, pin); err != nil {
		return result, err
	}
	result.InstallationID = m.InstallationID
	result.TenantID = m.TenantID
	result.PrincipalID = m.PrincipalID
	copy(result.key[:], m.Key)
	return result, nil
}
func canonicalUUIDv4(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' || s[14] != '4' || (s[19] != '8' && s[19] != '9' && s[19] != 'a' && s[19] != 'b') {
		return false
	}
	for i, c := range []byte(s) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// SecretReference is an internal binding, not a bearer credential or authority.
// The database remains the identity owner; changing the material key is rejected.
func (m *Material) SecretReference() string {
	sum := sha256.Sum256(m.key[:])
	return materialName + ":sha256:" + hex.EncodeToString(sum[:])
}
func (m *Material) Clear() { clear(m.key[:]) }

// Protect uses real per-user DPAPI; LOCAL_MACHINE and UI are never enabled.
func Protect(plain []byte) ([]byte, error)        { return crypt(plain, false) }
func Unprotect(ciphertext []byte) ([]byte, error) { return crypt(ciphertext, true) }
func crypt(input []byte, decrypt bool) ([]byte, error) {
	if len(input) == 0 || len(input) > 65536 {
		return nil, errors.New("invalid protected material size")
	}
	in := win.DataBlob{Size: uint32(len(input)), Data: &input[0]}
	var out win.DataBlob
	var err error
	if decrypt {
		err = win.CryptUnprotectData(&in, nil, nil, 0, nil, win.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = win.CryptProtectData(&in, nil, nil, 0, nil, win.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	runtime.KeepAlive(input)
	if err != nil {
		return nil, errors.New("DPAPI operation refused")
	}
	defer win.LocalFree(win.Handle(unsafe.Pointer(out.Data)))
	native := unsafe.Slice(out.Data, out.Size)
	defer clear(native)
	return append([]byte(nil), native...), nil
}

func currentSID() (*win.SID, error) {
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
func privateSD(sid *win.SID, directory bool) (*win.SECURITY_DESCRIPTOR, error) {
	flags := ""
	if directory {
		flags = "OICI"
	}
	return win.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;" + flags + ";FA;;;" + sid.String() + ")(A;" + flags + ";FA;;;SY)")
}
func checkPrivateObject(h win.Handle, sid *win.SID, directory bool) error {
	sd, err := win.GetSecurityInfo(h, win.SE_FILE_OBJECT, win.OWNER_SECURITY_INFORMATION|win.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil || !owner.Equals(sid) {
		return errors.New("security owner mismatch")
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if control&win.SE_DACL_PROTECTED == 0 {
		return errors.New("security DACL is not protected")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil || acl.AceCount != 2 {
		return errors.New("security DACL has unexpected grants")
	}
	system, err := win.CreateWellKnownSid(win.WinLocalSystemSid)
	if err != nil {
		return err
	}
	flags := uint8(0)
	if directory {
		flags = win.OBJECT_INHERIT_ACE | win.CONTAINER_INHERIT_ACE
	}
	seenUser, seenSystem := false, false
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *win.ACCESS_ALLOWED_ACE
		if err = win.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != win.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != flags || uint32(ace.Mask) != 0x001f01ff {
			return errors.New("unsafe security grant")
		}
		s := (*win.SID)(unsafe.Pointer(&ace.SidStart))
		if s.Equals(sid) && !seenUser {
			seenUser = true
		} else if s.Equals(system) && !seenSystem {
			seenSystem = true
		} else {
			return errors.New("unexpected security principal")
		}
	}
	runtime.KeepAlive(sd)
	if !seenUser || !seenSystem {
		return errors.New("required security grant missing")
	}
	return nil
}
