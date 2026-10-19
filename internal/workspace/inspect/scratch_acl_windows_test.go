//go:build windows && amd64

package inspect

import (
	"context"
	"errors"
	"fmt"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	win "golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"
)

func validateCopiedACL(path string) error {
	sd, err := win.GetNamedSecurityInfo(path, win.SE_FILE_OBJECT, win.OWNER_SECURITY_INFORMATION|win.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	var token win.Token
	if err = win.OpenProcessToken(win.CurrentProcess(), win.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil || !owner.Equals(user.User.Sid) {
		return errors.New("scratch file owner mismatch")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil || acl.AceCount != 2 {
		return errors.New("scratch file grants are not minimal")
	}
	system, err := win.CreateWellKnownSid(win.WinLocalSystemSid)
	if err != nil {
		return err
	}
	seenUser, seenSystem := false, false
	for i := uint32(0); i < 2; i++ {
		var ace *win.ACCESS_ALLOWED_ACE
		if err = win.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != win.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&win.INHERIT_ONLY_ACE != 0 || uint32(ace.Mask) != 0x001f01ff {
			return errors.New("unexpected copied file ACE")
		}
		sid := (*win.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.Equals(user.User.Sid) && !seenUser {
			seenUser = true
		} else if sid.Equals(system) && !seenSystem {
			seenSystem = true
		} else {
			return errors.New("unexpected copied file principal")
		}
	}
	runtime.KeepAlive(sd)
	if !seenUser || !seenSystem {
		return errors.New("missing scratch grant")
	}
	return nil
}
func anonymousCannotRead(paths []string) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		safe := false
		defer func() {
			if safe {
				runtime.UnlockOSThread()
			}
		}()
		var initial win.Token
		e := win.OpenThreadToken(win.CurrentThread(), win.TOKEN_QUERY, true, &initial)
		if e == nil {
			initial.Close()
			done <- errors.New("unexpected thread token")
			return
		}
		if !errors.Is(e, win.ERROR_NO_TOKEN) {
			done <- e
			return
		}
		proc := win.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateAnonymousToken")
		ok, _, callErr := proc.Call(uintptr(win.CurrentThread()))
		if ok == 0 {
			safe = true
			done <- callErr
			return
		}
		var result error
		for _, path := range paths {
			f, e := os.Open(path)
			if e == nil {
				_ = f.Close()
				result = errors.New("anonymous token opened sensitive scratch")
				break
			}
			if !errors.Is(e, win.ERROR_ACCESS_DENIED) {
				result = fmt.Errorf("unexpected anonymous stimulus: %w", e)
				break
			}
		}
		if e = win.RevertToSelf(); e != nil {
			done <- e
			return
		}
		e = win.OpenThreadToken(win.CurrentThread(), win.TOKEN_QUERY, true, &initial)
		if e == nil {
			initial.Close()
			done <- errors.New("thread restoration uncertain")
			return
		}
		if !errors.Is(e, win.ERROR_NO_TOKEN) {
			done <- e
			return
		}
		safe = true
		done <- result
	}()
	return <-done
}
func TestInspectionGateCopiedACL(t *testing.T) {
	root, parent := inspectionFixture(t)
	// The required order is fixture -> byte inventory -> observation -> Inspect.
	// Inventory-only diagnostic independently reproduces NTFS directory
	// metadata notifications if that preparation is inside the watch window.
	before := inventory(t, root)
	watch := startWatch(t, root)
	ioBefore := ioSnapshot(t)
	report, err := inspect(context.Background(), root, parent, func(path string) error {
		if e := platform.CheckPrivateDirectory(path); e != nil {
			return e
		}
		paths := []string{filepath.Join(path, "descriptor.json"), filepath.Join(path, "workspace.sqlite")}
		for _, p := range paths {
			if e := validateCopiedACL(p); e != nil {
				return e
			}
		}
		if e := anonymousCannotRead(paths); e != nil {
			return e
		}
		return os.RemoveAll(path) // exact newly created synthetic callback path
	})
	ioAfter := ioSnapshot(t)
	checkScratchConservation(t, ioBefore, ioAfter, report)
	watch.finish(t, false)
	if err != nil {
		t.Fatal(err)
	}
	if !equalInventory(before, inventory(t, root)) {
		t.Fatal("ACL check modified source")
	}
	t.Log("both actual copied file ACLs owner+SYSTEM only; real anonymous token denied; restoration verified")
}
