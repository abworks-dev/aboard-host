//go:build windows && amd64

package installation

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	win "golang.org/x/sys/windows"
)

const authA = "41000000-0000-4000-8000-000000000001"
const authB = "41000000-0000-4000-8000-000000000002"
const authRestricted = "41000000-0000-4000-8000-000000000003"
const authSameResource = "41000000-0000-4000-8000-000000000004"

type authFixture struct {
	store                  *Store
	auth                   *Authenticator
	root, allowed, install string
}

func newAuthFixture(t *testing.T) authFixture {
	t.Helper()
	base, err := filepath.Abs(filepath.Join("..", "..", ".data", "logs", "001-validation"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "t011-auth-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	install := filepath.Join(root, "install")
	s, err := Initialize(context.Background(), install)
	if err != nil {
		t.Fatal(err)
	}
	id := s.Identity()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(root, "allowed")
	if err = os.MkdirAll(allowed, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A", "B"} {
		if err = os.Mkdir(filepath.Join(allowed, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	h, err := sqlite.Open(context.Background(), filepath.Join(install, "installation.sqlite"), sqlite.ReadWriteExisting)
	if err != nil {
		t.Fatal(err)
	}
	for index, w := range []string{authA, authB} {
		name := []string{"A", "B"}[index]
		if _, err = h.Conn.ExecContext(context.Background(), `INSERT INTO directory_registration VALUES(?,?,?,?,'synthetic-volume',zeroblob(16),1,'registered')`, w, id.TenantID, filepath.Join(allowed, name), allowed); err != nil {
			t.Fatal(err)
		}
		for _, action := range []string{"workspace.read", "workspace.write", "workspace.open", "workspace.close", "workspace.operator"} {
			if _, err = h.Conn.ExecContext(context.Background(), `INSERT INTO grant_entry VALUES(?,?,?,?)`, id.OwnerPrincipalID, id.TenantID, w, action); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err = h.Conn.ExecContext(context.Background(), `INSERT INTO principal VALUES(?,?,1,0)`, authRestricted, id.TenantID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.Conn.ExecContext(context.Background(), `INSERT INTO grant_entry VALUES(?,?,?,'workspace.read')`, authRestricted, id.TenantID, authA); err != nil {
		t.Fatal(err)
	}
	if err = h.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(context.Background(), install)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	a, err := NewAuthenticator(s, []string{allowed})
	if err != nil {
		t.Fatal(err)
	}
	return authFixture{s, a, root, allowed, install}
}
func authIssue(t *testing.T, f authFixture, principal, ws, action string, ttl time.Duration) string {
	t.Helper()
	name := fmt.Sprintf("aboard-g5-t011-%d-%d", os.Getpid(), time.Now().UnixNano())
	pipe, err := security.NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- f.auth.ServeSession(ctx, pipe, security.Grant{PrincipalID: principal, WorkspaceID: ws, Action: action}, ttl)
	}()
	token, receiveErr := security.ReceiveSession(ctx, name, security.ProcessID())
	serveErr := <-done
	if err = errors.Join(receiveErr, serveErr); err != nil {
		t.Fatal(err)
	}
	return token
}
func authOperator(t *testing.T, f authFixture) (*Authorization, string) {
	t.Helper()
	token := authIssue(t, f, f.store.Identity().OwnerPrincipalID, "", "installation.operator", time.Minute)
	c, err := f.auth.Authorize(context.Background(), token, "", "installation.operator")
	if err != nil {
		t.Fatal(err)
	}
	return c, token
}
func authFixtureSQL(t *testing.T, f authFixture, sql string, args ...any) {
	t.Helper()
	h, err := sqlite.Open(context.Background(), filepath.Join(f.install, "installation.sqlite"), sqlite.ReadWriteExisting)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Conn.ExecContext(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
	if err = h.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestInstallationAuthTrustedRootsConservativeCase(t *testing.T) {
	for _, row := range []struct {
		root, path string
		allowed    bool
	}{
		{`E:\Root\K`, `e:\ROOT\k`, false},
		{`E:\Root\K`, `e:\root\K\child`, false},
		{`C:\scope\A`, `C:\scope\a`, false},
		{`C:\scope\A`, `C:\scope\A\child`, true},
		{`E:\Root\K`, `E:\Root\K`, true},
		{`E:\Root\K`, `E:\Root\K-sibling`, false},
		{`E:\Root\K`, `E:\Root\K`, false},
		{`E:\Root\K`, `E:\Root\K`, false},
		{`E:\Root\K`, `E:\Root\K\child`, true},
		{`E:\Root\Ä`, `E:\Root\ä`, false},
		{`E:\`, `e:\child`, false},
		{`E:\`, `E:\child`, true},
	} {
		if got := underLocation(row.root, row.path); got != row.allowed {
			t.Fatalf("trusted root containment got=%t want=%t", got, row.allowed)
		}
	}
}
func TestInstallationAuthGrantIsMinimumNotAttenuation(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	token := authIssue(t, f, f.store.Identity().OwnerPrincipalID, authA, "workspace.read", time.Minute)
	if _, err := f.auth.Authorize(ctx, token, authB, "workspace.write"); err != nil {
		t.Fatal("owner current DB grants unexpectedly attenuated", err)
	}
	if _, err := f.auth.Authorize(ctx, token, "", "installation.operator"); err != nil {
		t.Fatal("owner current DB role lost", err)
	}
	// The separate restricted principal, not the minting Grant argument, limits
	// the token. Its B/write/operator denials are covered by the next test.
}
func TestInstallationAuthCurrentDatabaseAndScope(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	token := authIssue(t, f, authRestricted, authA, "workspace.read", time.Minute)
	a, err := f.auth.Authorize(ctx, token, authA, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	if a.Principal().PrincipalID != authRestricted || a.Principal().TenantID != f.store.Identity().TenantID {
		t.Fatal("client subject replaced DB identity")
	}
	for _, target := range []struct{ w, action string }{{authB, "workspace.read"}, {authA, "workspace.write"}, {"", "installation.operator"}, {authA, "installation.operator"}, {"", "workspace.read"}} {
		if _, err = f.auth.Authorize(ctx, token, target.w, target.action); err == nil {
			t.Fatal("restricted principal elevated")
		}
	}
	if _, err = f.auth.Authorize(ctx, strings.Repeat("a", 43), authA, "workspace.read"); err == nil {
		t.Fatal("forged token accepted")
	}
	scope, _ := a.Scope()
	if err = a.CheckAction(ctx, scope, "workspace.read"); err != nil {
		t.Fatal(err)
	}
	if err = a.CheckAction(ctx, scope, "workspace.write"); err == nil {
		t.Fatal("read handle accepted business write requirement")
	}
	if err = a.CheckAction(ctx, scope, ""); err == nil {
		t.Fatal("empty business action accepted")
	}
	// Identical resource IDs are intentionally not authorization keys. The retained
	// resource/database owner still selects its explicit workspace/tenant scope.
	resources := []struct {
		id    string
		owner workspace.Scope
	}{{authSameResource, scope}, {authSameResource, workspace.Scope{InstallationID: scope.InstallationID, TenantID: scope.TenantID, WorkspaceID: authB}}}
	if err = a.CheckScope(ctx, resources[0].owner); err != nil {
		t.Fatal(err)
	}
	if err = a.CheckScope(ctx, resources[1].owner); err == nil {
		t.Fatal("same-ID B resource borrowed A permission")
	}
	wrongTenant := scope
	wrongTenant.TenantID = authRestricted
	if err = a.CheckScope(ctx, wrongTenant); err == nil {
		t.Fatal("fake resource tenant accepted")
	}
	if err = a.CheckLocation(ctx, filepath.Join(f.allowed, "A")); err == nil {
		t.Fatal("workspace read became filesystem operator")
	}
	operator, _ := authOperator(t, f)
	if err = f.auth.SetPrincipalAuthorization(ctx, a, authRestricted, false, []sqlite.WorkspaceGrant{{WorkspaceID: authB, Action: "workspace.read"}}); err == nil {
		t.Fatal("read capability managed grants")
	}
	// Mutation occurs between initial request authorization and transaction start.
	if err = f.auth.SetPrincipalAuthorization(ctx, operator, authRestricted, false, []sqlite.WorkspaceGrant{{WorkspaceID: authA, Action: "workspace.read"}}); err != nil {
		t.Fatal(err)
	}
	if err = a.Check(ctx); err == nil {
		t.Fatal("business-boundary check accepted old version")
	}
	if _, err = f.auth.Authenticate(ctx, token); err == nil {
		t.Fatal("old session survived equal-grant replacement")
	}
	fresh := authIssue(t, f, authRestricted, authA, "workspace.read", time.Minute)
	c, err := f.auth.Authorize(ctx, fresh, authA, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.auth.SetPrincipalAuthorization(ctx, operator, authRestricted, true, nil); err != nil {
		t.Fatal(err)
	}
	if err = c.Check(ctx); err == nil {
		t.Fatal("revoked principal accepted")
	}
	// Old receipts are just data; no authorization entry accepts a receipt/hash/ID.
	if _, err = f.auth.Authorize(ctx, authSameResource, authB, "workspace.read"); err == nil {
		t.Fatal("historical ID used as bearer")
	}
}
func TestInstallationAuthRootsRegistrationAndExpiry(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	token := authIssue(t, f, authRestricted, authA, "workspace.read", time.Minute)
	original, err := f.auth.Authorize(ctx, token, authA, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	empty, _ := NewAuthenticator(f.store, nil)
	if _, err = empty.Authorize(ctx, token, authA, "workspace.read"); err == nil {
		t.Fatal("absent trusted machine scope allowed")
	}
	sibling, _ := NewAuthenticator(f.store, []string{f.allowed + "-sibling"})
	if _, err = sibling.Authorize(ctx, token, authA, "workspace.read"); err == nil {
		t.Fatal("sibling root prefix accepted")
	}
	for _, bad := range []string{`relative`, `C:\allowed\..\escape`, `\\server\share`, `C:\allowed\file:stream`, `C:\allowed\bad.`} {
		if _, err = NewAuthenticator(f.store, []string{bad}); err == nil {
			t.Fatal("invalid trusted path accepted")
		}
	}
	operator, _ := authOperator(t, f)
	if err = operator.CheckLocation(ctx, filepath.Join(f.allowed, "new-empty-target")); err != nil {
		t.Fatal(err)
	}
	if err = operator.CheckLocation(ctx, filepath.Join(f.root, "unregistered-outside")); err == nil {
		t.Fatal("operator unrestricted FS")
	}
	authFixtureSQL(t, f, `UPDATE directory_registration SET allowed_location=? WHERE workspace_id=?`, filepath.Join(f.root, "elsewhere"), authA)
	if err = original.Check(ctx); err == nil {
		t.Fatal("current allowed_location ignored")
	}
	authFixtureSQL(t, f, `UPDATE directory_registration SET allowed_location=?,registration_version=registration_version+1 WHERE workspace_id=?`, f.allowed, authA)
	if err = original.Check(ctx); err == nil {
		t.Fatal("registration version change ignored")
	}
	if _, err = f.auth.Authorize(ctx, token, authA, "workspace.read"); err != nil {
		t.Fatal(err)
	}
	authFixtureSQL(t, f, `UPDATE directory_registration SET state='pending' WHERE workspace_id=?`, authA)
	if _, err = f.auth.Authorize(ctx, token, authA, "workspace.read"); err == nil {
		t.Fatal("unregistered target accepted")
	}
	authFixtureSQL(t, f, `UPDATE directory_registration SET state='registered' WHERE workspace_id=?`, authA)
	hash, _ := tokenHash(token)
	authFixtureSQL(t, f, `UPDATE client_session SET expires_at=? WHERE token_hash=?`, time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano), hash)
	if _, err = f.auth.Authenticate(ctx, token); err == nil {
		t.Fatal("expired session accepted")
	}
}
func TestInstallationAuthSessionRevocationReopenAndHTTP(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	token := authIssue(t, f, authRestricted, authA, "workspace.read", time.Minute)
	check, err := f.auth.Authorize(ctx, token, authA, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"Origin", "Cookie"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:45678/v1/context", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(header, "forbidden")
		if _, err = f.auth.AuthorizeHTTP(ctx, r, "127.0.0.1:45678", authA, "workspace.read"); err == nil {
			t.Fatal("browser boundary bypassed")
		}
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:45678/v1/context", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if _, err = f.auth.AuthorizeHTTP(ctx, r, "127.0.0.1:45678", authA, "workspace.read"); err != nil {
		t.Fatal(err)
	}
	r.Host = "localhost:45678"
	if _, err = f.auth.AuthorizeHTTP(ctx, r, "127.0.0.1:45678", authA, "workspace.read"); err == nil {
		t.Fatal("host alias accepted")
	}
	if err = f.auth.RevokeSession(ctx, check, token); err != nil {
		t.Fatal(err)
	}
	if err = check.Check(ctx); err == nil {
		t.Fatal("revoked session retained approval")
	}
	fresh := authIssue(t, f, authRestricted, authA, "workspace.read", time.Minute)
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = f.auth.Authorize(ctx, fresh, authA, "workspace.read"); err == nil {
		t.Fatal("closed store old wrapper accepted")
	}
	reopened, err := Open(ctx, f.install)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	next, _ := NewAuthenticator(reopened, []string{f.allowed})
	if _, err = next.Authorize(ctx, fresh, authA, "workspace.read"); err != nil {
		t.Fatal("DB session not durable", err)
	}
	if _, err = next.Authorize(ctx, token, authA, "workspace.read"); err == nil {
		t.Fatal("revocation not durable")
	}
	data, err := os.ReadFile(filepath.Join(f.install, "installation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(token)) || bytes.Contains(data, []byte(fresh)) {
		t.Fatal("raw token persisted")
	}
	sum := sha256.Sum256([]byte(fresh))
	if !bytes.Contains(data, []byte(hex.EncodeToString(sum[:]))) {
		t.Fatal("token hash not durable")
	}
}
func TestInstallationAuthSharedUnknownDependencyFence(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	token := authIssue(t, f, authRestricted, authA, "workspace.read", time.Minute)
	check, err := f.auth.Authorize(ctx, token, authA, "workspace.read")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := NewAuthenticator(f.store, []string{f.allowed})
	f.store.mu.Lock()
	// Real COMMIT flush faults are independently exercised in the SQL package;
	// here verify their ErrUnknown mapping reaches ALL real DB-backed dependencies.
	mapped := f.auth.mutationErrorLocked(errors.Join(sqlite.ErrUnknown, errors.New("synthetic post-COMMIT uncertainty")))
	f.store.mu.Unlock()
	if !errors.Is(mapped, ErrAuthorizationUnknown) {
		t.Fatal("unknown lost")
	}
	for _, call := range []func() error{
		func() error { return check.Check(ctx) },
		func() error { _, e := other.Authorize(ctx, token, authA, "workspace.read"); return e },
		func() error { _, e := other.Authenticate(ctx, token); return e },
	} {
		if err = call(); !errors.Is(err, ErrAuthorizationUnknown) {
			t.Fatal("dependency used stale auth", err)
		}
	}
	// Unknown auth persistence is NOT unknown physical close. Close can really
	// finish; old capabilities still cannot be used afterwards.
	if err = f.store.Close(); err != nil {
		t.Fatal("authorization unknown falsified physical close", err)
	}
	if err = check.Check(ctx); err == nil {
		t.Fatal("old wrapper resurrected")
	}
}

func TestInstallationAuthPipeFailedACKRevokesDatabaseSession(t *testing.T) {
	f := newAuthFixture(t)
	name := fmt.Sprintf("aboard-g5-t011-ack-%d-%d", os.Getpid(), time.Now().UnixNano())
	pipe, err := security.NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- f.auth.ServeSession(ctx, pipe, security.Grant{PrincipalID: authRestricted, WorkspaceID: authA, Action: "workspace.read"}, time.Minute)
	}()
	path, _ := win.UTF16PtrFromString(`\\.\pipe\` + name)
	h, err := win.CreateFile(path, win.GENERIC_READ|win.GENERIC_WRITE, 0, nil, win.OPEN_EXISTING, win.SECURITY_SQOS_PRESENT|win.SECURITY_IMPERSONATION, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer win.CloseHandle(h)
	var n uint32
	if err = win.WriteFile(h, []byte{1}, &n, nil); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 2)
	if err = win.ReadFile(h, header, &n, nil); err != nil || n != 2 {
		t.Fatal("header", err)
	}
	payload := make([]byte, binary.LittleEndian.Uint16(header))
	if err = win.ReadFile(h, payload, &n, nil); err != nil || n != 43 {
		t.Fatal("payload", err)
	}
	token := string(payload)
	clear(payload)
	// Server is now awaiting ACK. Its DB transaction/mutex must already be gone.
	progress := make(chan error, 1)
	go func() { _, e := f.auth.Authenticate(context.Background(), token); progress <- e }()
	select {
	case err = <-progress:
		if err == nil {
			t.Fatal("pending bearer accepted before ACK")
		}
	case <-time.After(time.Second):
		t.Fatal("ACK wait held installation authorization mutex")
	}
	if err = win.WriteFile(h, []byte{3}, &n, nil); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("bad ACK accepted")
	}
	if _, err = f.auth.Authorize(context.Background(), token, authA, "workspace.read"); err == nil {
		t.Fatal("unacknowledged bearer live")
	}
}

func authReadPending(t *testing.T, name string) (win.Handle, string) {
	t.Helper()
	path, _ := win.UTF16PtrFromString(`\\.\pipe\` + name)
	h, err := win.CreateFile(path, win.GENERIC_READ|win.GENERIC_WRITE, 0, nil, win.OPEN_EXISTING, win.SECURITY_SQOS_PRESENT|win.SECURITY_IMPERSONATION, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { win.CloseHandle(h) })
	var n uint32
	if err = win.WriteFile(h, []byte{1}, &n, nil); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 2)
	if err = win.ReadFile(h, header, &n, nil); err != nil || n != 2 {
		t.Fatal("header", err)
	}
	if binary.LittleEndian.Uint16(header) != 43 {
		t.Fatal("invalid token size")
	}
	payload := make([]byte, 43)
	defer clear(payload)
	if err = win.ReadFile(h, payload, &n, nil); err != nil || n != 43 {
		t.Fatal("payload", err)
	}
	return h, string(payload)
}
func TestInstallationAuthDeliveryCloseBeforeACK(t *testing.T) {
	f := newAuthFixture(t)
	operator, _ := authOperator(t, f)
	name := fmt.Sprintf("aboard-g5-t011-close-%d-%d", os.Getpid(), time.Now().UnixNano())
	pipe, err := security.NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- f.auth.ServeSession(ctx, pipe, security.Grant{PrincipalID: authRestricted, WorkspaceID: authA, Action: "workspace.read"}, time.Minute)
	}()
	h, token := authReadPending(t, name)
	if _, err = f.auth.Authenticate(context.Background(), token); err == nil {
		t.Fatal("pending usable before ACK")
	}
	if err = operator.Check(context.Background()); err != nil {
		t.Fatal("unacknowledged peer blocked unrelated current authorization", err)
	}
	closeErr := f.store.Close()
	var n uint32
	if err = win.WriteFile(h, []byte{9}, &n, nil); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("bad ACK accepted")
	}
	if !errors.Is(closeErr, ErrSessionDeliveryPending) {
		t.Fatal("Close released DB/lease during unacknowledged delivery", closeErr)
	}
	if _, err = f.auth.Authenticate(context.Background(), token); err == nil {
		t.Fatal("bad ACK token remained active")
	}
	if err = f.store.Close(); err != nil {
		t.Fatal("delivery guard did not drain", err)
	}
}

type authRevokingDeliveryIssuer struct {
	base     *databaseIssuer
	operator *Authorization
}

func (i *authRevokingDeliveryIssuer) Issue(grant security.Grant, ttl time.Duration) (string, error) {
	token, err := i.base.Issue(grant, ttl)
	if err != nil {
		return "", err
	}
	if err = i.base.auth.RevokeSession(context.Background(), i.operator, token); err != nil {
		i.base.Revoke(token)
		return "", err
	}
	return token, nil
}
func (i *authRevokingDeliveryIssuer) Revoke(token string) { i.base.Revoke(token) }
func (i *authRevokingDeliveryIssuer) ConfirmDelivery(token string) error {
	return i.base.ConfirmDelivery(token)
}
func TestInstallationAuthDeliveryReceiverWaitsForActivation(t *testing.T) {
	f := newAuthFixture(t)
	operator, _ := authOperator(t, f)
	name := fmt.Sprintf("aboard-g5-t011-final-%d-%d", os.Getpid(), time.Now().UnixNano())
	pipe, err := security.NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	base := &databaseIssuer{auth: f.auth, ctx: ctx}
	defer base.finish()
	issuer := &authRevokingDeliveryIssuer{base: base, operator: operator}
	done := make(chan error, 1)
	go func() {
		done <- pipe.ServeOne(ctx, issuer, security.Grant{PrincipalID: authRestricted, WorkspaceID: authA, Action: "workspace.read"}, time.Minute)
	}()
	token, receiveErr := security.ReceiveSession(ctx, name, security.ProcessID())
	serveErr := <-done
	if token != "" || receiveErr == nil || serveErr == nil {
		t.Fatal("receiver returned bearer before real terminal-revoke activation denial")
	}
}
func TestInstallationAuthDeliveryFinalACKFailures(t *testing.T) {
	for _, kind := range []string{"bad-final-ack", "cancel-final-ack"} {
		t.Run(kind, func(t *testing.T) {
			f := newAuthFixture(t)
			name := fmt.Sprintf("aboard-g5-t011-finalack-%d-%d", os.Getpid(), time.Now().UnixNano())
			pipe, err := security.NewControlPipe(name)
			if err != nil {
				t.Fatal(err)
			}
			defer pipe.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- f.auth.ServeSession(ctx, pipe, security.Grant{PrincipalID: authRestricted, WorkspaceID: authA, Action: "workspace.read"}, time.Minute)
			}()
			h, token := authReadPending(t, name)
			if _, err = f.auth.Authenticate(context.Background(), token); err == nil {
				t.Fatal("pending usable")
			}
			var n uint32
			if err = win.WriteFile(h, []byte{2}, &n, nil); err != nil {
				t.Fatal(err)
			}
			var final [1]byte
			if err = win.ReadFile(h, final[:], &n, nil); err != nil || n != 1 || final[0] != 3 {
				t.Fatal("server final confirmation missing", err)
			}
			if _, err = f.auth.Authorize(context.Background(), token, authA, "workspace.read"); err != nil {
				t.Fatal("ACK did not activate current row", err)
			}
			closeErr := f.store.Close()
			if kind == "bad-final-ack" {
				if err = win.WriteFile(h, []byte{9}, &n, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			if err = <-done; err == nil {
				t.Fatal("unfinished final confirmation accepted")
			}
			if !errors.Is(closeErr, ErrSessionDeliveryPending) {
				t.Fatal("Close escaped final ACK window", closeErr)
			}
			if _, err = f.auth.Authenticate(context.Background(), token); err == nil {
				t.Fatal("final ACK failure remained live")
			}
			if err = f.store.Close(); err != nil {
				t.Fatal("final delivery did not drain", err)
			}
		})
	}
}

func TestInstallationAuthDeliveryChild(t *testing.T) {
	if os.Getenv("ABOARD_T011_DELIVERY_CHILD") != "serve" {
		t.Skip("subprocess fixture only")
	}
	root := os.Getenv("ABOARD_T011_DELIVERY_ROOT")
	if !strings.Contains(root, `\.data\logs\001-validation\t011-auth-`) {
		t.Fatal("not synthetic")
	}
	s, err := Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := NewAuthenticator(s, []string{filepath.Join(filepath.Dir(root), "allowed")})
	if err != nil {
		t.Fatal(err)
	}
	pipe, err := security.NewControlPipe(os.Getenv("ABOARD_T011_DELIVERY_PIPE"))
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	fmt.Println("T011 delivery child ready")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = a.ServeSession(ctx, pipe, security.Grant{PrincipalID: s.Identity().OwnerPrincipalID, Action: "installation.operator"}, time.Minute); err != nil {
		t.Fatal(err)
	}
}
func TestInstallationAuthDeliveryKillBeforeACKColdReopen(t *testing.T) {
	f := newAuthFixture(t)
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("aboard-g5-t011-kill-%d-%d", os.Getpid(), time.Now().UnixNano())
	cmd := exec.Command(os.Args[0], "-test.run=^TestInstallationAuthDeliveryChild$")
	cmd.Env = append(os.Environ(), "ABOARD_T011_DELIVERY_CHILD=serve", "ABOARD_T011_DELIVERY_ROOT="+f.install, "ABOARD_T011_DELIVERY_PIPE="+name)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	ready := make(chan bool, 1)
	go func() {
		line, e := bufio.NewReader(stdout).ReadString('\n')
		ready <- e == nil && line == "T011 delivery child ready\n"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("delivery child not ready")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delivery child timeout")
	}
	_, token := authReadPending(t, name)
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	waitErr := cmd.Wait()
	stopped = true
	if waitErr == nil {
		t.Fatal("child not actually killed")
	}
	t.Logf("real child terminated after token payload, before ACK; child_exit=%d (expected termination)", cmd.ProcessState.ExitCode())
	fresh, err := Open(context.Background(), f.install)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	a, err := NewAuthenticator(fresh, []string{f.allowed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Authenticate(context.Background(), token); err == nil {
		t.Fatal("unacknowledged token survived cold reopen after process kill")
	}
}
