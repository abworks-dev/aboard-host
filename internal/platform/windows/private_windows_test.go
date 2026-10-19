//go:build windows && amd64

package windows

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	win "golang.org/x/sys/windows"
)

func TestPrivateDirectoryGate(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".data", "logs", "001-validation"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	base, err := os.MkdirTemp(root, "g5-private-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(base); err != nil {
			t.Error(err)
		}
	})
	private := filepath.Join(base, "private")
	if err = CreatePrivateDirectory(private); err != nil {
		t.Fatal(err)
	}
	if err = CheckPrivateDirectory(private); err != nil {
		t.Fatal(err)
	}
	if err = CreatePrivateDirectory(private); err == nil {
		t.Fatal("existing directory accepted as new")
	}
	if err = CheckPrivateDirectory(filepath.Join(base, "missing")); err == nil {
		t.Fatal("missing directory accepted")
	}
	if _, err = os.Stat(filepath.Join(base, "missing")); !os.IsNotExist(err) {
		t.Fatal("check created missing directory")
	}
	if err = CheckPrivateDirectory(base); err == nil {
		t.Fatal("ordinary inherited directory accepted")
	}
	// Malformed ACLs exist only on synthetic fixtures; product checks never
	// change them. Windows may auto-protect an explicit creation descriptor, so
	// the unprotected case explicitly clears that bit after fixture creation.
	sid, err := privateUserSID()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"unprotected":    "O:" + sid.String() + "D:(A;OICI;FA;;;" + sid.String() + ")(A;OICI;FA;;;SY)",
		"extra-world":    "O:" + sid.String() + "D:P(A;OICI;FA;;;" + sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;GR;;;WD)",
		"missing-system": "O:" + sid.String() + "D:P(A;OICI;FA;;;" + sid.String() + ")",
		"inherit-only":   "O:" + sid.String() + "D:P(A;OICIIO;FA;;;" + sid.String() + ")(A;OICI;FA;;;SY)",
	}
	for name, sddl := range cases {
		t.Run(name, func(t *testing.T) {
			sd, e := win.SecurityDescriptorFromString(sddl)
			if e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(base, name)
			p, _ := win.UTF16PtrFromString(path)
			sa := win.SecurityAttributes{Length: uint32(unsafe.Sizeof(win.SecurityAttributes{})), SecurityDescriptor: sd}
			if e = win.CreateDirectory(p, &sa); e != nil {
				t.Fatal(e)
			}
			// Only AFTER the before/after assertion, restore this test-owned
			// empty fixture's DACL to permit cleanup. This is not product ACL
			// repair: CheckPrivateDirectory remains non-mutating on rejection.
			t.Cleanup(func() {
				if name == "inherit-only" {
					safe, e := win.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid.String() + ")(A;OICI;FA;;;SY)")
					if e != nil {
						t.Error(e)
						return
					}
					acl, _, e := safe.DACL()
					if e != nil {
						t.Error(e)
						return
					}
					if e = win.SetNamedSecurityInfo(path, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION|win.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); e != nil {
						t.Error(e)
						return
					}
				}
				if e := os.Remove(path); e != nil {
					t.Error(e)
				}
			})
			if name == "unprotected" {
				acl, _, e := sd.DACL()
				if e != nil {
					t.Fatal(e)
				}
				if e = win.SetNamedSecurityInfo(path, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION|win.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); e != nil {
					t.Fatal(e)
				}
			}
			before, e := win.GetNamedSecurityInfo(path, win.SE_FILE_OBJECT, win.OWNER_SECURITY_INFORMATION|win.DACL_SECURITY_INFORMATION)
			if e != nil {
				t.Fatal(e)
			}
			if name == "unprotected" {
				control, _, e := before.Control()
				if e != nil {
					t.Fatal(e)
				}
				if control&win.SE_DACL_PROTECTED != 0 {
					t.Fatal("unprotected fixture unexpectedly protected")
				}
			}
			if e = CheckPrivateDirectory(path); e == nil {
				t.Fatal("unsafe existing directory accepted")
			}
			after, e := win.GetNamedSecurityInfo(path, win.SE_FILE_OBJECT, win.OWNER_SECURITY_INFORMATION|win.DACL_SECURITY_INFORMATION)
			if e != nil {
				t.Fatal(e)
			}
			if before.String() != after.String() {
				t.Fatal("check repaired security descriptor")
			}
		})
	}
}
