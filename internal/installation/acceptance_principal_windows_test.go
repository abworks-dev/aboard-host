//go:build windows && amd64

package installation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
)

const acceptancePrincipalEnvironment = "ABOARD_ACCEPTANCE_PRINCIPAL"
const acceptancePrincipalID = "61111111-1111-4111-8111-111111111111"
const acceptancePrincipalReady = "ABOARD_PRINCIPAL_READY"

var errAcceptancePrincipal = errors.New("synthetic principal fixture rejected")

type acceptancePrincipalRequest struct {
	Version                                               int
	Fixture, InstallationID, TenantID, WorkspaceID, Phase string
}

func acceptancePrincipalProject() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
func acceptancePrincipalLocal(path string) bool {
	return len(path) > 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1:3] == `:\` && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path[2:], "/:\x00")
}
func acceptancePrincipalUnreparsed(path string) bool {
	if !acceptancePrincipalLocal(path) {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.EqualFold(filepath.Clean(resolved), path) {
		return false
	}
	for current := path; ; current = filepath.Dir(current) {
		p, err := win.UTF16PtrFromString(current)
		if err != nil {
			return false
		}
		attributes, err := win.GetFileAttributes(p)
		if err != nil || attributes&win.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return false
		}
		if filepath.Dir(current) == current {
			return true
		}
	}
}

var acceptancePrincipalUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func acceptancePrincipalUUID(s string) bool { return acceptancePrincipalUUIDPattern.MatchString(s) }
func readAcceptancePrincipal(encoded string) (acceptancePrincipalRequest, error) {
	var out acceptancePrincipalRequest
	if len(encoded) == 0 || len(encoded) > 8192 {
		return out, errAcceptancePrincipal
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != encoded {
		return out, errAcceptancePrincipal
	}
	value, err := protocol.ParseJSON(raw)
	if err != nil {
		return out, errAcceptancePrincipal
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 6 {
		return out, errAcceptancePrincipal
	}
	if fields["Version"] != json.Number("1") {
		return out, errAcceptancePrincipal
	}
	for _, key := range []string{"Fixture", "InstallationID", "TenantID", "WorkspaceID", "Phase"} {
		if _, ok := fields[key].(string); !ok {
			return out, errAcceptancePrincipal
		}
	}
	out = acceptancePrincipalRequest{1, fields["Fixture"].(string), fields["InstallationID"].(string), fields["TenantID"].(string), fields["WorkspaceID"].(string), fields["Phase"].(string)}
	logRoot := filepath.Join(acceptancePrincipalProject(), ".data", "logs", "001-validation")
	if !acceptancePrincipalLocal(out.Fixture) || filepath.Dir(out.Fixture) != logRoot || !strings.HasPrefix(filepath.Base(out.Fixture), "t023-") || len(filepath.Base(out.Fixture)) > 40 || !acceptancePrincipalUUID(out.InstallationID) || !acceptancePrincipalUUID(out.TenantID) || !acceptancePrincipalUUID(out.WorkspaceID) || (out.Phase != "read-write" && out.Phase != "open-only" && out.Phase != "read-write-short") {
		return acceptancePrincipalRequest{}, errAcceptancePrincipal
	}
	return out, nil
}

// Test-executable only. A failed first pipe/Store Close is sticky and retains
// its ancestors; process termination is not reported as successful cleanup.
type acceptancePrincipalOwner struct {
	store     *Store
	pipe      *security.ControlPipe
	uncertain error
}

var acceptancePrincipalRetained []*acceptancePrincipalOwner

func (o *acceptancePrincipalOwner) closePipe() error {
	if o.uncertain != nil {
		return o.uncertain
	}
	if o.pipe != nil {
		if err := o.pipe.Close(); err != nil {
			o.uncertain = errAcceptancePrincipal
			return o.uncertain
		}
		o.pipe = nil
	}
	return nil
}
func (o *acceptancePrincipalOwner) close() error {
	if err := o.closePipe(); err != nil {
		return err
	}
	if o.store != nil {
		if err := o.store.Close(); err != nil {
			o.uncertain = errAcceptancePrincipal
			return o.uncertain
		}
		o.store = nil
	}
	return nil
}
func runAcceptancePrincipal(ctx context.Context, in acceptancePrincipalRequest) (err error) {
	root := filepath.Join(in.Fixture, "installation")
	allowed := filepath.Join(in.Fixture, "allowed")
	target := filepath.Join(allowed, "principal-target")
	exe, e := os.Executable()
	logs := filepath.Join(acceptancePrincipalProject(), ".data", "logs", "001-validation")
	relative, re := filepath.Rel(logs, exe)
	if e != nil || re != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.Ext(exe) != ".exe" || !acceptancePrincipalUnreparsed(exe) || !acceptancePrincipalUnreparsed(root) || !acceptancePrincipalUnreparsed(target) {
		return errAcceptancePrincipal
	}
	o := &acceptancePrincipalOwner{}
	defer func() {
		err = errors.Join(err, o.close())
		if o.uncertain != nil {
			acceptancePrincipalRetained = append(acceptancePrincipalRetained, o)
		}
	}()
	o.store, e = Open(ctx, root)
	if e != nil {
		// Open's retained nonnil error result already attempted its first Close.
		// Never retry it or release its ancestors as apparently successful cleanup.
		if o.store != nil {
			o.uncertain = errAcceptancePrincipal
		}
		return errAcceptancePrincipal
	}
	id := o.store.Identity()
	if id.InstallationID != in.InstallationID || id.TenantID != in.TenantID || id.OwnerPrincipalID == acceptancePrincipalID {
		return errAcceptancePrincipal
	}
	a, e := NewAuthenticator(o.store, []string{allowed})
	if e != nil {
		return errAcceptancePrincipal
	}
	ownerPipe := "aboard-g5-" + filepath.Base(in.Fixture) + "-principal-owner"
	o.pipe, e = security.NewControlPipe(ownerPipe)
	if e != nil {
		return errAcceptancePrincipal
	}
	serveCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- a.ServeSession(serveCtx, o.pipe, security.Grant{PrincipalID: id.OwnerPrincipalID, Action: "installation.operator"}, time.Minute)
	}()
	token, receiveErr := security.ReceiveSession(serveCtx, ownerPipe, uint32(os.Getpid()))
	if receiveErr != nil {
		cancel()
	}
	serveErr := <-done
	cancel()
	if receiveErr != nil || serveErr != nil {
		return errAcceptancePrincipal
	}
	if e = o.closePipe(); e != nil {
		return errAcceptancePrincipal
	}
	operator, e := a.Authorize(ctx, token, "", "installation.operator")
	if e != nil {
		return errAcceptancePrincipal
	}
	bound, e := a.Authorize(ctx, token, in.WorkspaceID, "workspace.read")
	if e != nil {
		return errAcceptancePrincipal
	}
	scope, e := bound.Scope()
	if e != nil || scope.InstallationID != id.InstallationID || scope.TenantID != id.TenantID || scope.WorkspaceID != in.WorkspaceID {
		return errAcceptancePrincipal
	}
	location := bound.Location()
	if !strings.EqualFold(location.RootLocator, target) || !strings.EqualFold(location.AllowedLocation, allowed) || location.RegistrationVersion < 1 || location.VolumeIdentity == "" || location.RootFileID == [16]byte{} {
		return errAcceptancePrincipal
	}
	grants := []sqlite.WorkspaceGrant{{WorkspaceID: in.WorkspaceID, Action: "workspace.read"}, {WorkspaceID: in.WorkspaceID, Action: "workspace.write"}}
	action := "workspace.read"
	if in.Phase == "open-only" {
		grants = []sqlite.WorkspaceGrant{{WorkspaceID: in.WorkspaceID, Action: "workspace.open"}}
		action = "workspace.open"
	}
	if e = a.SetPrincipalAuthorization(ctx, operator, acceptancePrincipalID, false, grants); e != nil {
		return errAcceptancePrincipal
	}
	if e = a.RevokeSession(ctx, operator, token); e != nil {
		return errAcceptancePrincipal
	}
	token = ""
	name := "aboard-g5-" + filepath.Base(in.Fixture) + "-principal"
	o.pipe, e = security.NewControlPipe(name)
	if e != nil {
		return errAcceptancePrincipal
	}
	fmt.Println(acceptancePrincipalReady)
	// Parent pins THIS helper PID, not a stopped Host PID. Full server completion
	// and successful Close/exit, not ReceiveSession alone, authorize Host restart.
	ttl := 5 * time.Minute
	if in.Phase == "read-write-short" {
		ttl = 3 * time.Second // Real durable expiry; no clock/SQL mutation.
	}
	if e = a.ServeSession(ctx, o.pipe, security.Grant{PrincipalID: acceptancePrincipalID, WorkspaceID: in.WorkspaceID, Action: action}, ttl); e != nil {
		return errAcceptancePrincipal
	}
	return nil
}
func TestAcceptancePrincipalFixtureHelper(t *testing.T) {
	encoded := os.Getenv(acceptancePrincipalEnvironment)
	if encoded == "" {
		t.Skip("strict subprocess fixture only")
	}
	in, err := readAcceptancePrincipal(encoded)
	if err != nil {
		t.Fatal("principal fixture configuration rejected")
	}
	// No Store access before the parent has pinned this launched process.
	permit, err := io.ReadAll(io.LimitReader(os.Stdin, 2))
	if err != nil || string(permit) != "P" {
		t.Fatal("principal fixture launch permit missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if runAcceptancePrincipal(ctx, in) != nil {
		t.Fatal("principal fixture failed (sensitive diagnostics withheld)")
	}
}

// Policy-only: an already-uncertain owner must not touch any ancestor again.
// This is not a simulated claim of an actual native Close failure.
func TestAcceptancePrincipalFixtureUncertainOwnerIsSticky(t *testing.T) {
	s := &Store{}
	o := &acceptancePrincipalOwner{store: s, uncertain: errAcceptancePrincipal}
	for range 2 {
		if !errors.Is(o.close(), errAcceptancePrincipal) || o.store != s || o.uncertain == nil {
			t.Fatal("uncertain principal owner retried or released an ancestor")
		}
	}
}
func TestAcceptancePrincipalFixtureStrictGuard(t *testing.T) {
	fixture := filepath.Join(acceptancePrincipalProject(), ".data", "logs", "001-validation", "t023-principal-guard")
	good := acceptancePrincipalRequest{1, fixture, "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333", "read-write"}
	raw, _ := json.Marshal(good)
	encode := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	if _, err := readAcceptancePrincipal(encode(raw)); err != nil {
		t.Fatal("valid synthetic fixture refused")
	}
	for _, phase := range []string{"open-only", "read-write-short"} {
		if _, err := readAcceptancePrincipal(encode([]byte(strings.Replace(string(raw), `"read-write"`, `"`+phase+`"`, 1)))); err != nil {
			t.Fatal("valid scoped principal phase refused")
		}
	}
	for _, bad := range []string{"", encode(append(raw, []byte(`{}`)...)), encode([]byte(`{"Version":1,"Version":1}`)), encode([]byte(strings.Replace(string(raw), `"Version":1`, `"Version":1.0`, 1))), encode([]byte(strings.Replace(string(raw), `"read-write"`, `"operator"`, 1))), encode([]byte(strings.Replace(string(raw), `"WorkspaceID":`, `"Unknown":`, 1))), encode([]byte(strings.Replace(string(raw), `t023-principal-guard`, `t011-auth-real`, 1)))} {
		if _, err := readAcceptancePrincipal(bad); err == nil {
			t.Fatal("unsafe principal fixture accepted")
		}
	}
	// A lexically valid but nonexistent fixture must fail before opening any Store.
	good.Fixture = filepath.Join(filepath.Dir(fixture), "t023-principal-absent")
	if _, err := os.Lstat(good.Fixture); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("negative fixture unexpectedly exists")
	}
	if runAcceptancePrincipal(context.Background(), good) == nil {
		t.Fatal("missing fixture opened")
	}
}
