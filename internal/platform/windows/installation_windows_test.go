//go:build windows && amd64

package windows

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	win "golang.org/x/sys/windows"
)

func TestInstallationPinsScopeLifetime(t *testing.T) {
	base := syntheticDir(t)
	root := filepath.Join(base, "install")
	if err := CreatePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	lease, err := Acquire(root, Installation, true, false)
	if err != nil {
		t.Fatal(err)
	}
	material, err := lease.CreateInstallationMaterial()
	if err != nil {
		t.Fatal(err)
	}
	if err = win.FlushFileBuffers(material.Handle()); err != nil {
		t.Fatal(err)
	}
	if err = lease.ValidateInstallationFile(root, "bootstrap.dpapi", material); err != nil {
		t.Fatal(err)
	}
	if err = lease.ValidateInstallationFile(base, "bootstrap.dpapi", material); err == nil {
		t.Fatal("wrong root accepted")
	}
	if _, err = lease.PinInstallationFile("arbitrary"); err == nil {
		t.Fatal("arbitrary file accepted")
	}
	dbPath := filepath.Join(root, "installation.sqlite")
	if err = os.WriteFile(dbPath, []byte("synthetic database pin"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := lease.PinInstallationFile("installation.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range []*File{material, db} {
		var flags uint32
		proc := win.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation")
		ok, _, e := proc.Call(uintptr(pin.Handle()), uintptr(unsafe.Pointer(&flags)))
		if ok == 0 {
			t.Fatal(e)
		}
		if flags&win.HANDLE_FLAG_INHERIT != 0 {
			t.Fatal("inheritable installation pin")
		}
		if err = os.Remove(pin.Path()); err == nil {
			t.Fatal("pinned installation file deleted")
		}
		if err = os.Rename(pin.Path(), pin.Path()+".replace"); err == nil {
			t.Fatal("pinned installation file replaced")
		}
	}
	if err = lease.Close(); err == nil {
		t.Fatal("lease released with live installation pins")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = lease.Validate(); err == nil {
		t.Fatal("closed pin reused as stable history")
	}
	if err = material.Close(); err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err = lease.ValidateInstallation(root); err == nil {
		t.Fatal("closed lease accepted")
	}
	workspace := gateRoot(t)
	writer, err := Acquire(workspace, Workspace, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err = writer.ValidateInstallation(workspace); err == nil {
		t.Fatal("workspace scope accepted as installation")
	}
	if _, err = writer.PinInstallationFile("bootstrap.dpapi"); err == nil {
		t.Fatal("workspace lease pinned installation material")
	}
}

func TestInstallationPinsRejectExistingHardlinks(t *testing.T) {
	for _, name := range []string{"bootstrap.dpapi", "installation.sqlite", "host.lock"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(syntheticDir(t), "install")
			if err := CreatePrivateDirectory(root); err != nil {
				t.Fatal(err)
			}
			lease, err := Acquire(root, Installation, true, false)
			if err != nil {
				t.Fatal(err)
			}
			material, err := lease.CreateInstallationMaterial()
			if err != nil {
				t.Fatal(err)
			}
			if err = material.Close(); err != nil {
				t.Fatal(err)
			}
			if err = lease.Close(); err != nil {
				t.Fatal(err)
			}
			if name == "installation.sqlite" {
				if err = os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err = os.Link(filepath.Join(root, name), filepath.Join(root, "link")); err != nil {
				t.Fatal(err)
			}
			lease, err = Acquire(root, Installation, false, false)
			if name == "host.lock" {
				if err == nil {
					lease.Close()
					t.Fatal("hardlinked host.lock accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = lease.PinInstallationFile(name); err == nil {
				t.Fatal("hardlinked installation key accepted")
			}
			if err = lease.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
