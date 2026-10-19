//go:build windows && amd64

package inspect

import (
	"context"
	"errors"
	win "golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectionGateMetadataObservers(t *testing.T) {
	for _, kind := range []string{"create", "delete", "rename", "attributes", "security"} {
		t.Run(kind, func(t *testing.T) {
			root, _ := inspectionFixture(t)
			path := filepath.Join(root, "metadata-control.bin")
			if kind != "create" {
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			watch := startWatch(t, root)
			before := ioSnapshot(t)
			var err error
			switch kind {
			case "create":
				var f *os.File
				f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err == nil {
					err = f.Close()
				}
			case "delete":
				err = os.Remove(path) // exact just-created synthetic file
			case "rename":
				if !filepath.IsAbs(path) || !within(root, path) {
					t.Fatal("unchecked synthetic rename path")
				}
				err = os.Rename(path, path+"-renamed")
			case "attributes":
				var p *uint16
				p, err = win.UTF16PtrFromString(path)
				if err == nil {
					err = win.SetFileAttributes(p, win.FILE_ATTRIBUTE_HIDDEN)
				}
			case "security":
				sd, e := win.GetNamedSecurityInfo(path, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION)
				err = e
				if err == nil {
					acl, _, e := sd.DACL()
					err = e
					if err == nil {
						c, _, e := sd.Control()
						err = e
						if err == nil {
							flag := win.SECURITY_INFORMATION(win.PROTECTED_DACL_SECURITY_INFORMATION)
							if c&win.SE_DACL_PROTECTED != 0 {
								flag = win.UNPROTECTED_DACL_SECURITY_INFORMATION
							}
							err = win.SetNamedSecurityInfo(path, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION|flag, nil, nil, acl, nil)
						}
					}
				}
			}
			after := ioSnapshot(t)
			if err != nil {
				t.Fatal(err)
			}
			watch.finish(t, true)
			t.Logf("metadata positive control %s: PID=%d writes/bytes=%d/%d other=%d; real source change visible separately", kind, os.Getpid(), after.Writes-before.Writes, after.WriteBytes-before.WriteBytes, after.Other-before.Other)
		})
	}
}

func TestInspectionGateFrozenObjectsAtCleanup(t *testing.T) {
	root, parent := inspectionFixture(t)
	before := inventory(t, root)
	watch := startWatch(t, root)
	ioBefore := ioSnapshot(t)
	report, err := inspect(context.Background(), root, parent, func(scratch string) error {
		for _, path := range []string{filepath.Join(root, "aboard.workspace.json"), filepath.Join(root, "data", "workspace.sqlite")} {
			f, e := os.OpenFile(path, os.O_WRONLY, 0)
			if e == nil {
				_ = f.Close()
				return errors.New("frozen key admitted a data writer")
			}
			if !errors.Is(e, win.ERROR_SHARING_VIOLATION) {
				return e
			}
		}
		for _, path := range []string{root, filepath.Join(root, "data"), filepath.Join(root, ".aboard", "locks")} {
			if !filepath.IsAbs(path) || !(path == root || within(root, path)) {
				return errors.New("unchecked rename path")
			}
			if e := os.Rename(path, path+"-moved"); e == nil {
				return errors.New("frozen source directory renamed")
			}
		}
		return os.RemoveAll(scratch) // exact fresh callback path; no caller-supplied target
	})
	ioAfter := ioSnapshot(t)
	checkScratchConservation(t, ioBefore, ioAfter, report)
	watch.finish(t, false)
	if err != nil {
		t.Fatal(err)
	}
	if !equalInventory(before, inventory(t, root)) {
		t.Fatal("freeze checks changed source")
	}
}

func equalInventory(a, b map[string][32]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}
