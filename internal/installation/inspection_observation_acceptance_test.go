//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	win "golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAcceptanceInspectionObservationNativeCloseRetainsOwner(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	operator, _ := authOperator(t, f)
	i := &DirectoryInspector{store: f.store}
	owner, e := i.beginInspection(ctx, operator)
	if e != nil {
		t.Fatal(e)
	}
	file, e := os.OpenFile(filepath.Join(f.root, "diagnostic-close-control.bin"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		if cleanupErr := owner.closeLocked(); cleanupErr != nil {
			t.Error(cleanupErr)
		}
		owner.mu.Unlock()
		t.Fatal(e)
	}
	handle := win.Handle(file.Fd())
	var original win.ByHandleFileInformation
	identityErr := win.GetFileInformationByHandle(handle, &original)
	ended, attempted := false, false
	defer func() {
		if !ended {
			if !attempted {
				_ = win.SetHandleInformation(handle, 2, 0)
				// Before the product Close attempt, the os.File still owns its fd.
				if err := file.Close(); err == nil {
					ended = true
				} else {
					owner.failure = &inspectionObservationClose{file: file, cause: err}
					t.Error("test-owned file cleanup failed")
				}
			} else {
				var current win.ByHandleFileInformation
				// Never raw-close a possibly reused numeric handle.
				if identityErr == nil && win.GetFileInformationByHandle(handle, &current) == nil && current.VolumeSerialNumber == original.VolumeSerialNumber && current.FileIndexHigh == original.FileIndexHigh && current.FileIndexLow == original.FileIndexLow {
					if win.SetHandleInformation(handle, 2, 0) == nil {
						if err := win.CloseHandle(handle); err == nil {
							ended = true
						}
					}
				}
				if !ended {
					t.Error("test-owned handle cleanup not proven; owner fence retained")
				}
			}
		}
		// Only after independent TEST native end, never a product retry/reset.
		if ended {
			owner.failure = nil
			if err := owner.closeLocked(); err != nil {
				t.Error(err)
			}
		}
		owner.mu.Unlock()
	}()
	if identityErr != nil {
		t.Fatal("native fixture identity unavailable")
	}
	if e = win.SetHandleInformation(handle, 2, 2); e != nil {
		t.Fatal(e)
	}
	attempted = true
	ended = closeInspectionObservation(owner, file)
	if ended || !errors.Is(owner.failure, platform.ErrPhysicalCloseUnknown) {
		t.Fatal("actual protected HANDLE close did not retain owning error")
	}
	var info win.ByHandleFileInformation
	if e = win.GetFileInformationByHandle(handle, &info); e != nil {
		t.Fatal("failed close did not leave actual handle alive")
	}
	if !errors.Is(owner.closeLocked(), ErrInspectionPending) || f.store.workspaceCapacity.Usage() != 1 || !errors.Is(f.store.Close(), ErrInspectionPending) {
		t.Fatal("diagnostic uncertainty lost Store/quota fence")
	}
	if e = win.SetHandleInformation(handle, 2, 0); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(owner.closeLocked(), ErrInspectionPending) {
		t.Fatal("unprotect did not authorize product retry")
	}
	if e = win.GetFileInformationByHandle(handle, &info); e != nil {
		t.Fatal("product retried raw file handle")
	}
	if e = win.CloseHandle(handle); e != nil {
		t.Fatal("independent test native end failed")
	}
	ended = true
}

// Parser/scope policy only, not successful kernel/native or side-effect proof.
func TestAcceptanceInspectionObservationHardGate(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	parent := filepath.Join(filepath.Dir(source), "..", "..", ".data", "logs", "001-validation")
	fixture, e := os.MkdirTemp(parent, "t023-inspection-gates-")
	if e != nil {
		t.Fatal(e)
	}
	fixture = filepath.Clean(fixture)
	store := &Store{root: filepath.Join(fixture, "installation")}
	auth := &Authenticator{store: store}
	operator := &Authorization{auth: auth, action: "installation.operator"}
	i := &DirectoryInspector{store: store, scratchParent: store.root}
	root := filepath.Join(fixture, "allowed", "observed-source")
	config := inspectionObservationConfig{Version: 1, Fixture: fixture, Root: root}
	raw, _ := json.Marshal(config)
	read := func(input []byte) bool {
		_, ok := readInspectionObservation(i, operator, root, base64.RawURLEncoding.EncodeToString(input))
		return ok
	}
	if !read(raw) {
		t.Fatal("valid fixed observer scope refused")
	}
	invalid := [][]byte{[]byte("null"), []byte(`{"version":1}`), []byte(strings.Replace(string(raw), `"version":1`, `"version":1e0`, 1)), []byte(strings.Replace(string(raw), `"Fixture"`, `"fixture"`, 1)), []byte(strings.Replace(string(raw), `"Root"`, `"root"`, 1)), []byte(`{"version":1,"Fixture":null,"Root":""}`), append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"Root":null}`)...)}
	for n, value := range invalid {
		if read(value) {
			t.Fatal("invalid config accepted", n)
		}
	}
	if _, ok := readInspectionObservation(i, operator, root, strings.Repeat("x", 8193)); ok {
		t.Fatal("unbounded observer config accepted")
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if _, ok := readInspectionObservation(i, operator, filepath.Join(fixture, "allowed", "other"), encoded); ok {
		t.Fatal("observer followed unmatched source")
	}
	wrong := *operator
	wrong.action = "workspace.operator"
	if _, ok := readInspectionObservation(i, &wrong, root, encoded); ok {
		t.Fatal("observer inferred installation authority")
	}
	foreign := *i
	foreign.store = &Store{root: store.root}
	if _, ok := readInspectionObservation(&foreign, operator, root, encoded); ok {
		t.Fatal("observer accepted another owner")
	}
	wrongRoot := config
	wrongRoot.Fixture = parent
	value, _ := json.Marshal(wrongRoot)
	if read(value) {
		t.Fatal("observer escaped synthetic prefix")
	}
	paths, e := filepath.Glob(filepath.Join(fixture, "inspection-*"))
	if e != nil || len(paths) != 0 {
		t.Fatal("scope parser performed diagnostic/native IO before actual observation")
	}
}
