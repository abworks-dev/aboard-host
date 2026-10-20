//go:build windows && amd64

package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	win "golang.org/x/sys/windows"
)

// This mutates only an initialized, stopped synthetic installation. It does not
// change a machine policy, owner, SACL or privilege. Restoring the original DACL
// is test teardown, never a Host repair path.
func fixtureDACL(t *testing.T, f *fixture, path, variant string) func() {
	t.Helper()
	if requireSyntheticRoot(f.root) != nil || !within(f.config.InstallationRoot, path) && path != f.config.InstallationRoot || !unreparsed(path) {
		t.Fatal("unowned ACL target")
	}
	originals := map[string]*win.SECURITY_DESCRIPTOR{}
	ordered := []string{f.config.InstallationRoot, filepath.Join(f.config.InstallationRoot, "bootstrap.dpapi"), filepath.Join(f.config.InstallationRoot, "installation.sqlite"), filepath.Join(f.config.InstallationRoot, "host.lock")}
	for _, p := range ordered {
		sd, e := win.GetNamedSecurityInfo(p, win.SE_FILE_OBJECT, win.OWNER_SECURITY_INFORMATION|win.DACL_SECURITY_INFORMATION)
		if e != nil {
			t.Fatal("fixture security backup failed")
		}
		originals[p] = sd
	}
	name, err := win.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	open := func() win.Handle {
		h, e := win.CreateFile(name, win.READ_CONTROL|win.WRITE_DAC, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE|win.FILE_SHARE_DELETE, nil, win.OPEN_EXISTING, win.FILE_FLAG_BACKUP_SEMANTICS|win.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if e != nil {
			t.Fatal("fixture ACL handle refused")
		}
		return h
	}
	token, err := win.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	user, err := token.GetTokenUser()
	closeErr := token.Close()
	if err != nil || closeErr != nil {
		t.Fatal("fixture user unavailable")
	}
	flags := ""
	if path == f.config.InstallationRoot {
		flags = "OICI"
	}
	sddl := "D:P(A;" + flags + ";FA;;;" + user.User.Sid.String() + ")"
	if variant != "missing-system" {
		sddl += "(A;" + flags + ";FA;;;SY)"
	}
	if variant == "extra-principal" {
		// The extra root grant is deliberately NON-inheritable so that root
		// rejection cannot be masked by an independently unsafe database ACL.
		sddl += "(A;;GR;;;WD)"
	}
	sd, err := win.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatal("fixture descriptor refused")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	restore := func() {
		// Restore parent before children, since native inheritance may update
		// unprotected descendants. No original owner or SACL is changed.
		for _, p := range ordered {
			if path != f.config.InstallationRoot && p != path {
				continue
			}
			old := originals[p]
			if restoredSecurityEquivalent(map[string]string{"s": old.String()}, map[string]string{"s": fixtureACLString(t, p)}) {
				continue
			}
			oldACL, _, e := old.DACL()
			control, _, ce := old.Control()
			if e != nil || ce != nil {
				t.Error("fixture ACL backup invalid")
				return
			}
			if control&win.SE_DACL_PROTECTED != 0 {
				e = win.SetNamedSecurityInfo(p, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION|win.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, oldACL, nil)
			} else {
				e = win.SetNamedSecurityInfo(p, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION|win.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, oldACL, nil)
			}
			runtime.KeepAlive(old)
			if e != nil {
				t.Error("fixture ACL restoration failed")
				return
			}
		}
	}
	t.Cleanup(restore)
	h := open()
	err = win.SetSecurityInfo(h, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION|win.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	closeErr = win.CloseHandle(h)
	runtime.KeepAlive(sd)
	if err != nil || closeErr != nil {
		t.Fatal("fixture ACL stimulus failed")
	}
	return restore
}
func fixtureACLString(t *testing.T, path string) string {
	t.Helper()
	sd, err := win.GetNamedSecurityInfo(path, win.SE_FILE_OBJECT, win.OWNER_SECURITY_INFORMATION|win.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal("fixture ACL observation failed")
	}
	return sd.String()
}
func fixtureSecurityState(t *testing.T, f *fixture) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, name := range []string{"", "bootstrap.dpapi", "installation.sqlite", "host.lock"} {
		result[name] = fixtureACLString(t, filepath.Join(f.config.InstallationRoot, name))
	}
	return result
}
func restoredSecurityEquivalent(a, b map[string]string) bool {
	// SetSecurityInfo sets the AUTO_INHERITED bookkeeping bit (observed D:P
	// -> D:PAI) even with byte-identical ACEs. Restore must preserve owner,
	// protection, ACE order/rights/principals, not that kernel bookkeeping bit.
	normalize := func(m map[string]string) map[string]string {
		n := map[string]string{}
		for k, v := range m {
			v = strings.ReplaceAll(v, "D:PAI(", "D:P(")
			v = strings.ReplaceAll(v, "D:AI(", "D:(")
			n[k] = v
		}
		return n
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}
func refusedMaterialStartup(t *testing.T, f *fixture, kind string) {
	t.Helper()
	before := tree(t, f.config.InstallationRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result := runObservedCLI(ctx, f, "serve")
	if ctx.Err() != nil || !result.IdentityProven || !result.Unavailable || !result.SafeOutput || result.OutputBytes != 0 {
		t.Fatal("private material did not cleanly refuse", kind, result)
	}
	if !reflect.DeepEqual(before, tree(t, f.config.InstallationRoot)) {
		t.Fatal("private material refusal rewrote installation", kind)
	}
	matrixEvidence(t, f, "private-material-"+kind, result)
}
func TestV009IndependentPrivateACLRefusal(t *testing.T) {
	for _, row := range []struct{ name, target, variant string }{{"root-extra-principal", "", "extra-principal"}, {"material-extra-principal", "bootstrap.dpapi", "extra-principal"}, {"material-missing-system", "bootstrap.dpapi", "missing-system"}} {
		t.Run(row.name, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			path := f.config.InstallationRoot
			if row.target != "" {
				path = filepath.Join(path, row.target)
			}
			if row.target == "" {
				// Freeze descendants' original owner/SYSTEM permissions before
				// changing the parent. Otherwise Windows inheritance itself can
				// duplicate ACEs and make a child independently unsupported.
				// This supported, more-restrictive fixture shape has its OWN
				// actual production startup control before the bad root exists.
				for _, name := range []string{"bootstrap.dpapi", "installation.sqlite", "host.lock"} {
					child := filepath.Join(f.config.InstallationRoot, name)
					sd, e := win.GetNamedSecurityInfo(child, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION)
					if e != nil {
						t.Fatal("child fixture security unavailable")
					}
					acl, _, e := sd.DACL()
					if e != nil {
						t.Fatal(e)
					}
					e = win.SetNamedSecurityInfo(child, win.SE_FILE_OBJECT, win.DACL_SECURITY_INFORMATION|win.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
					runtime.KeepAlive(sd)
					if e != nil {
						t.Fatal("child fixture protection failed")
					}
				}
				control := f.start(t)
				control.connect(t)
				control.stop(t)
			}
			original := fixtureSecurityState(t, f)
			restore := fixtureDACL(t, f, path, row.variant)
			changed := fixtureSecurityState(t, f)
			if reflect.DeepEqual(original, changed) {
				t.Fatal("ACL stimulus did not change descriptor")
			}
			if row.target == "" {
				for _, name := range []string{"bootstrap.dpapi", "installation.sqlite", "host.lock"} {
					if !restoredSecurityEquivalent(map[string]string{"s": original[name]}, map[string]string{"s": changed[name]}) {
						t.Fatal("root fixture changed a child permission oracle")
					}
				}
			}
			refusedMaterialStartup(t, f, row.name)
			if !reflect.DeepEqual(fixtureSecurityState(t, f), changed) {
				t.Fatal("Host repaired ACL on refusal")
			}
			restore()
			if !restoredSecurityEquivalent(original, fixtureSecurityState(t, f)) {
				t.Fatal("fixture did not restore original owner, protection and ACEs")
			}
			p := f.start(t)
			c := p.connect(t)
			var view protocol.PrincipalView
			c.request(t, "GET", "/v1/context", nil, &view, 200)
			p.stop(t)
		})
	}
}
func TestV009IndependentDPAPIRefusal(t *testing.T) {
	for _, kind := range []string{"tampered-ciphertext", "decryptable-invalid-json", "decryptable-foreign-binding"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			path := filepath.Join(f.config.InstallationRoot, "bootstrap.dpapi")
			original, err := os.ReadFile(path)
			if err != nil || len(original) < 16 {
				t.Fatal("synthetic ciphertext missing")
			}
			if requireSyntheticRoot(f.root) != nil || !within(f.config.InstallationRoot, path) || !unreparsed(path) {
				t.Fatal("unowned DPAPI stimulus")
			}
			var bad []byte
			switch kind {
			case "tampered-ciphertext":
				bad = append([]byte(nil), original...)
				bad[len(bad)/2] ^= 0x80
				plain, e := security.Unprotect(bad)
				clear(plain)
				if e == nil {
					t.Fatal("DPAPI corruption did not produce native refusal")
				}
			default:
				plain := []byte("synthetic invalid JSON material")
				if kind == "decryptable-foreign-binding" {
					plain, err = json.Marshal(struct {
						InstallationID, TenantID, PrincipalID string
						Key                                   []byte
						Version                               int
					}{string(newID(t)), string(newID(t)), string(newID(t)), bytes.Repeat([]byte{0x69}, 32), 1})
					if err != nil {
						t.Fatal(err)
					}
				}
				bad, err = security.Protect(plain)
				if err != nil {
					clear(plain)
					t.Fatal("native fixture DPAPI encryption failed")
				}
				decoded, e := security.Unprotect(bad)
				same := bytes.Equal(decoded, plain)
				clear(decoded)
				clear(plain)
				if e != nil || !same {
					t.Fatal("valid current-user DPAPI control failed")
				}
			}
			if err = os.WriteFile(path, bad, 0600); err != nil {
				t.Fatal("ciphertext stimulus failed")
			}
			clear(bad)
			refusedMaterialStartup(t, f, kind)
			if err = os.WriteFile(path, original, 0600); err != nil {
				t.Fatal("fixture ciphertext restoration failed")
			}
			clear(original)
			p := f.start(t)
			c := p.connect(t)
			var view protocol.PrincipalView
			c.request(t, "GET", "/v1/context", nil, &view, 200)
			p.stop(t)
		})
	}
}

func TestV009IndependentMissingReparseAndUntrustedPaths(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	p := f.start(t)
	c := p.connect(t)
	root, a := newInspectionWorkspace(t, c, "boundary-source")
	_, b := newInspectionWorkspace(t, c, "boundary-B")
	bRevision := 0
	confirmB := func(label string) {
		openedB := c.open(t, b.WorkspaceId)
		prefix := "/v1/workspaces/" + string(b.WorkspaceId)
		var status protocol.WorkspaceView
		c.request(t, "GET", prefix, nil, &status, 200)
		if status.OpenEpoch == nil || status.Revision == nil || string(*status.Revision) != strconv.Itoa(bRevision) {
			t.Fatal("local path rejection fenced B")
		}
		body := "independent B during " + label
		var candidate protocol.CandidateView
		c.request(t, "POST", prefix+"/record-previews", protocol.PreviewRecord{OpenEpoch: *status.OpenEpoch, Kind: "create-text", BaseRevision: *status.Revision, Title: "path isolation", Body: protocol.Body(body)}, &candidate, 201)
		var receipt protocol.Receipt
		c.request(t, "POST", prefix+"/record-commits", protocol.CommitRecord{OperationId: newID(t), OpenEpoch: *status.OpenEpoch, CandidateId: candidate.CandidateId, CandidateDigest: candidate.CandidateDigest, PreviewVersion: candidate.PreviewVersion}, &receipt, 200)
		var record protocol.TextRecordView
		c.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil, &record, 200)
		bRevision++
		if string(record.Body) != body || record.OperationId != receipt.OperationId || string(receipt.NewRevision) != strconv.Itoa(bRevision) {
			t.Fatal("B actual save/read failed")
		}
		closePublic(t, c, openedB)
	}
	parked := filepath.Join(f.config.AllowedRoots[0], "boundary-parked")
	if !within(f.config.AllowedRoots[0], root) || !within(f.config.AllowedRoots[0], parked) {
		t.Fatal("unowned move")
	}
	original := tree(t, root)
	if err := os.Rename(root, parked); err != nil {
		t.Fatal("synthetic stopped move failed")
	}
	// Registered before later link cleanup; never restore over an unknown object.
	t.Cleanup(func() {
		if _, e := os.Lstat(root); os.IsNotExist(e) {
			if e = os.Rename(parked, root); e != nil {
				t.Error("fixture move restore failed")
			}
		}
	})
	testSource := func(label string) {
		before := tree(t, parked)
		watch := startInspectionWatch(t, f.config.AllowedRoots[0])
		defer watch.finish(t, false)
		var denied protocol.ErrorView
		c.request(t, "POST", "/v1/workspace-inspections", protocol.InspectWorkspace{Path: protocol.Path(root)}, &denied, 409)
		if denied.Code != protocol.ErrorCodeINSPECTIONUNAVAILABLE {
			t.Fatal("unsupported source leaked unproved scope", label, denied.Code)
		}
		pre := registrationPrecondition(t, c, a.WorkspaceId, "open")
		c.request(t, "POST", "/v1/workspaces/"+string(a.WorkspaceId)+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, &denied, 409)
		if denied.Code != protocol.ErrorCodeRECOVERYREQUIRED || !reflect.DeepEqual(before, tree(t, parked)) {
			t.Fatal("source alias/missing path repaired or opened", label, denied.Code)
		}
		watch.finish(t, false)
	}
	testSource("missing")
	confirmB("missing")
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("missing source was created")
	}
	cmd := exec.Command("cmd.exe", "/c", "mklink", "/J", root, parked)
	var output boundedCapture
	cmd.Stdout = &output
	cmd.Stderr = &output
	if cmd.Run() != nil {
		t.Fatal("owned junction stimulus failed")
	}
	linkRemoved := false
	removeLink := func() {
		if linkRemoved {
			return
		}
		n, e := win.UTF16PtrFromString(root)
		if e != nil {
			t.Error("invalid fixture link")
			return
		}
		attrs, e := win.GetFileAttributes(n)
		if os.IsNotExist(e) || e == win.ERROR_FILE_NOT_FOUND || e == win.ERROR_PATH_NOT_FOUND {
			return
		}
		if e != nil || attrs&win.FILE_ATTRIBUTE_REPARSE_POINT == 0 || !within(f.config.AllowedRoots[0], root) {
			t.Error("link cleanup refuses unexpected object")
			return
		}
		if e = os.Remove(root); e != nil {
			t.Error("fixture junction removal failed")
		} else {
			linkRemoved = true
		}
	}
	t.Cleanup(removeLink)
	// On this Go/Windows profile EvalSymlinks returned the junction spelling
	// unchanged despite native REPARSE_POINT=1. Prove the actual referent by
	// following ONLY this exact test-created alias and comparing native IDs.
	nativeName, _ := win.UTF16PtrFromString(root)
	attributes, ae := win.GetFileAttributes(nativeName)
	followedID := func(path string) [2]uint64 {
		if path != root && path != parked {
			t.Fatal("unowned junction identity query")
		}
		name, e := win.UTF16PtrFromString(path)
		if e != nil {
			t.Fatal(e)
		}
		h, e := win.CreateFile(name, win.FILE_READ_ATTRIBUTES, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE|win.FILE_SHARE_DELETE, nil, win.OPEN_EXISTING, win.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if e != nil {
			t.Fatal("native referent query refused")
		}
		var info win.ByHandleFileInformation
		e = win.GetFileInformationByHandle(h, &info)
		ce := win.CloseHandle(h)
		if e != nil || ce != nil || info.FileAttributes&win.FILE_ATTRIBUTE_DIRECTORY == 0 {
			t.Fatal("native referent query failed")
		}
		return [2]uint64{uint64(info.VolumeSerialNumber), uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}
	}
	if ae != nil || attributes&win.FILE_ATTRIBUTE_REPARSE_POINT == 0 || followedID(root) != followedID(parked) {
		t.Fatal("junction target/reparse positive control failed")
	}
	testSource("junction")
	confirmB("junction")
	removeLink()
	if err := os.Rename(parked, root); err != nil {
		t.Fatal("fixture restoration failed")
	}
	if !reflect.DeepEqual(original, tree(t, root)) {
		t.Fatal("source bytes changed through alias")
	}
	// These are locators only, not authority. UNC/device/relative spelling is
	// supplied to HTTP but is NEVER traversed by this harness or snapshotted.
	for _, path := range []string{"relative-synthetic", `\\aboard-invalid-host\not-a-share`, `\\?\` + root, filepath.Join(f.root, "outside-allowed")} {
		watch := startInspectionWatch(t, f.config.AllowedRoots[0])
		var denied protocol.ErrorView
		c.request(t, "POST", "/v1/workspace-inspections", protocol.InspectWorkspace{Path: protocol.Path(path)}, &denied, 403)
		if denied.Code != protocol.ErrorCodeFORBIDDEN {
			t.Fatal("locator spelling granted authority")
		}
		watch.finish(t, false)
		confirmB("untrusted locator")
		if strings.Contains(denied.SafeMessage, string(path)) {
			t.Fatal("private locator reflected")
		}
	}
	opened := c.open(t, a.WorkspaceId)
	closePublic(t, c, opened)
	p.stop(t)
	if !reflect.DeepEqual(original, tree(t, root)) {
		t.Fatal("negative path attempts changed formal source")
	}
}
