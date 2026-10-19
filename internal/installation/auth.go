//go:build windows && amd64

package installation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation/security"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

var (
	ErrUnauthorized         = security.ErrUnauthorized
	ErrAuthorizationUnknown = errors.New("installation authorization persistence unknown; dependent authorization fenced")
)

// Authenticator's roots are trusted host configuration, NOT request input or an
// independent permission. They only restrict current DB grants/registered roots.
// This lexical gate never grants filesystem access: open/inspect must still use
// the verified Windows path/handle/ACL adapters before touching target contents.
type Authenticator struct {
	store *Store
	roots []string
}

func NewAuthenticator(store *Store, trustedAllowedRoots []string) (*Authenticator, error) {
	if store == nil {
		return nil, ErrUnauthorized
	}
	a := &Authenticator{store: store}
	for _, root := range trustedAllowedRoots {
		path, err := canonicalLocation(root)
		if err != nil {
			return nil, ErrUnauthorized
		}
		a.roots = append(a.roots, path)
	}
	// Empty configuration deliberately grants no location scope.
	return a, nil
}

func canonicalLocation(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) || strings.ContainsAny(path, "\x00") || strings.Contains(path, "/") || strings.Contains(path, ":") && (len(path) < 3 || path[1] != ':' || strings.Contains(path[2:], ":")) {
		return "", ErrUnauthorized
	}
	clean := filepath.Clean(path)
	if clean != path || len(filepath.VolumeName(clean)) != 2 {
		return "", ErrUnauthorized
	}
	for _, part := range strings.Split(clean[3:], `\`) {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", ErrUnauthorized
		}
	}
	return clean, nil
}
func underLocation(root, path string) bool {
	// Exact trusted spelling only: neither Unicode folding nor ASCII folding is
	// safe for an uninspected case-sensitive directory. Legal aliases may be
	// denied by this policy. Physical file identity still requires owner checks.
	root = strings.TrimSuffix(root, `\`)
	path = strings.TrimSuffix(path, `\`)
	return path == root || strings.HasPrefix(path, root+`\`)
}
func (a *Authenticator) allows(path string) bool {
	if _, err := canonicalLocation(path); err != nil {
		return false
	}
	for _, root := range a.roots {
		if underLocation(root, path) {
			return true
		}
	}
	return false
}
func (a *Authenticator) readyLocked() error {
	s := a.store
	if s.closed || s.openFailure || s.db == nil {
		return ErrUnauthorized
	}
	if s.authUncertain {
		return ErrAuthorizationUnknown
	}
	return nil
}
func tokenHash(token string) (string, error) {
	if len(token) != 43 {
		return "", ErrUnauthorized
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) != 32 || base64.RawURLEncoding.EncodeToString(b) != token {
		return "", ErrUnauthorized
	}
	clear(b)
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:]), nil
}

// Authorization is a scoped, recheckable handle, never a cached approval. No
// exported field can be forged into a different principal/target/action. Keep
// this internal; it is not a bearer, public DTO, or serializable capability.
type Authorization struct {
	auth                                                             *Authenticator
	hash, installationID, tenantID, principalID, workspaceID, action string
	version                                                          int64
	location                                                         RegisteredLocation
}
type Principal struct{ InstallationID, TenantID, PrincipalID string }
type RegisteredLocation struct {
	RootLocator, AllowedLocation, VolumeIdentity string
	RootFileID                                   [16]byte
	RegistrationVersion                          int64
}

func (c *Authorization) Principal() Principal {
	if c == nil {
		return Principal{}
	}
	return Principal{c.installationID, c.tenantID, c.principalID}
}
func (c *Authorization) Scope() (workspace.Scope, error) {
	if c == nil || c.workspaceID == "" {
		return workspace.Scope{}, ErrUnauthorized
	}
	return workspace.Scope{InstallationID: c.installationID, TenantID: c.tenantID, WorkspaceID: c.workspaceID}, nil
}
func (c *Authorization) Location() RegisteredLocation {
	if c == nil {
		return RegisteredLocation{}
	}
	return c.location
}

func (a *Authenticator) currentLocked(ctx context.Context, hash, workspaceID, action string) (sqlite.SessionAuthorization, error) {
	if err := a.readyLocked(); err != nil {
		return sqlite.SessionAuthorization{}, err
	}
	dbAction := action
	if workspaceID == "" {
		if action != "" && action != "installation.operator" {
			return sqlite.SessionAuthorization{}, ErrUnauthorized
		}
		dbAction = ""
	} else if !sqlite.ValidWorkspaceAction(action) {
		return sqlite.SessionAuthorization{}, ErrUnauthorized
	}
	out, err := a.store.db.CurrentSessionAuthorization(ctx, hash, workspaceID, dbAction)
	if err != nil {
		return sqlite.SessionAuthorization{}, ErrUnauthorized
	}
	id := a.store.identity
	if out.InstallationID != id.InstallationID || out.TenantID != id.TenantID || out.OwnerPrincipalID != id.OwnerPrincipalID {
		return sqlite.SessionAuthorization{}, ErrUnauthorized
	}
	if action == "installation.operator" && out.PrincipalID != id.OwnerPrincipalID {
		return sqlite.SessionAuthorization{}, ErrUnauthorized
	}
	if workspaceID != "" {
		if _, err = canonicalLocation(out.AllowedLocation); err != nil || !a.allows(out.RootLocator) || !underLocation(out.AllowedLocation, out.RootLocator) || len(out.RootFileID) != 16 || out.RegistrationVersion <= 0 || out.VolumeIdentity == "" {
			return sqlite.SessionAuthorization{}, ErrUnauthorized
		}
	}
	return out, nil
}

// Authenticate returns identity only. Authorize adds an explicit workspace/action
// or installation.operator scope. Neither accepts a client-reported principal.
func (a *Authenticator) Authenticate(ctx context.Context, token string) (*Authorization, error) {
	return a.Authorize(ctx, token, "", "")
}
func (a *Authenticator) Authorize(ctx context.Context, token, workspaceID, action string) (*Authorization, error) {
	hash, err := tokenHash(token)
	if err != nil {
		return nil, ErrUnauthorized
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	out, err := a.currentLocked(ctx, hash, workspaceID, action)
	if err != nil {
		return nil, err
	}
	c := &Authorization{auth: a, hash: hash, installationID: out.InstallationID, tenantID: out.TenantID, principalID: out.PrincipalID, workspaceID: workspaceID, action: action, version: out.GrantVersion}
	c.location = RegisteredLocation{RootLocator: out.RootLocator, AllowedLocation: out.AllowedLocation, VolumeIdentity: out.VolumeIdentity, RegistrationVersion: out.RegistrationVersion}
	copy(c.location.RootFileID[:], out.RootFileID)
	return c, nil
}
func (c *Authorization) checkLocked(ctx context.Context) error {
	out, err := c.auth.currentLocked(ctx, c.hash, c.workspaceID, c.action)
	if err != nil {
		return err
	}
	if out.InstallationID != c.installationID || out.TenantID != c.tenantID || out.PrincipalID != c.principalID || out.GrantVersion != c.version {
		return ErrUnauthorized
	}
	if c.workspaceID != "" {
		var fileID [16]byte
		copy(fileID[:], out.RootFileID)
		if out.RootLocator != c.location.RootLocator || out.AllowedLocation != c.location.AllowedLocation || out.VolumeIdentity != c.location.VolumeIdentity || fileID != c.location.RootFileID || out.RegistrationVersion != c.location.RegistrationVersion {
			return ErrUnauthorized
		}
	}
	return nil
}

// Check must run per request AND immediately before beginning the workspace
// business transaction. It never holds an installation mutex/transaction over
// workspace IO; transaction/lifecycle serialization belongs to that workspace.
// This is a time-point check, not an atomic installation/workspace DB commit.
func (c *Authorization) Check(ctx context.Context) error {
	if c == nil || c.auth == nil {
		return ErrUnauthorized
	}
	c.auth.store.mu.Lock()
	defer c.auth.store.mu.Unlock()
	return c.checkLocked(ctx)
}

// CheckScope takes a trusted resource/database owner scope, NOT a client claim.
// Resource IDs alone (including identical IDs in A and B) cannot select a scope.
// It rechecks this handle's original action, not a business-required action;
// business owners must use CheckAction with their own trusted action constant.
func (c *Authorization) CheckScope(ctx context.Context, owner workspace.Scope) error {
	if c == nil || c.workspaceID == "" || owner.InstallationID != c.installationID || owner.TenantID != c.tenantID || owner.WorkspaceID != c.workspaceID {
		return ErrUnauthorized
	}
	return c.Check(ctx)
}

// CheckAction binds the business owner's required action and actual owner scope.
// requiredAction is a trusted constant from that operation, never request input.
func (c *Authorization) CheckAction(ctx context.Context, owner workspace.Scope, requiredAction string) error {
	if c == nil || !sqlite.ValidWorkspaceAction(requiredAction) || c.action != requiredAction {
		return ErrUnauthorized
	}
	return c.CheckScope(ctx, owner)
}

// CheckLocation is only for current operator directory operations. For registered
// workspaces Location/CheckScope must also be used; this is not arbitrary FS IO.
func (c *Authorization) CheckLocation(ctx context.Context, path string) error {
	if c == nil || c.auth == nil || c.action != "installation.operator" || !c.auth.allows(path) {
		return ErrUnauthorized
	}
	return c.Check(ctx)
}

func (a *Authenticator) AuthorizeHTTP(ctx context.Context, r *http.Request, authority, workspaceID, action string) (*Authorization, error) {
	host, port, splitErr := net.SplitHostPort(authority)
	ip := net.ParseIP(host)
	number, portErr := strconv.ParseUint(port, 10, 16)
	if r == nil || splitErr != nil || ip == nil || !ip.IsLoopback() || portErr != nil || number == 0 || r.Host != authority || r.Method == http.MethodOptions || len(r.Header.Values("Origin")) != 0 || len(r.Header.Values("Cookie")) != 0 {
		return nil, ErrUnauthorized
	}
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return nil, ErrUnauthorized
	}
	return a.Authorize(ctx, strings.TrimPrefix(values[0], "Bearer "), workspaceID, action)
}

// ServeSession is the only installation bearer minting entry. Grant is trusted
// operator intent selecting a subject/minimum scope, NOT a permission source.
// This is NOT session attenuation: sessions use that principal's current DB
// grants. Use a separate restricted DB principal to restrict a bearer. An owner
// token issued with workspace.read can still request current owner permissions.
// Owner sessions
// derive installation.operator from DB ownership and this real control plane,
// never from grant.Action or co-location. No HTTP/raw-Issue bypass is provided.
func (a *Authenticator) ServeSession(ctx context.Context, pipe *security.ControlPipe, grant security.Grant, ttl time.Duration) error {
	if pipe == nil {
		return ErrUnauthorized
	}
	issuer := &databaseIssuer{auth: a, ctx: ctx}
	defer issuer.finish()
	err := pipe.ServeOne(ctx, issuer, grant, ttl)
	return errors.Join(err, issuer.revokeError)
}

type databaseIssuer struct {
	auth        *Authenticator
	ctx         context.Context
	revokeError error
	pending     *sqlite.SessionDelivery
}

func (i *databaseIssuer) finish() {
	i.auth.store.mu.Lock()
	defer i.auth.store.mu.Unlock()
	if i.pending != nil {
		i.auth.store.pendingDeliveries--
		i.pending = nil
	}
}

// ConfirmDelivery is called by the existing SID/peer-authenticated pipe only
// after its initial token ACK. No mutex/transaction covers pipe IO. A successful
// activation followed by lost final confirmation may still be client-unknown.
func (i *databaseIssuer) ConfirmDelivery(token string) error {
	hash, err := tokenHash(token)
	if err != nil {
		return ErrUnauthorized
	}
	a := i.auth
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if err = a.readyLocked(); err != nil {
		return err
	}
	if i.pending == nil || hash != i.pending.TokenHash {
		return ErrUnauthorized
	}
	if err = a.store.db.ActivateInstallationSession(i.ctx, a.store.identity.OwnerPrincipalID, *i.pending); err != nil {
		return a.mutationErrorLocked(err)
	}
	_, err = a.currentLocked(i.ctx, hash, i.pending.WorkspaceID, i.pending.Action)
	return err
}

func (i *databaseIssuer) Issue(grant security.Grant, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > 5*time.Minute {
		return "", ErrUnauthorized
	}
	a := i.auth
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if err := a.readyLocked(); err != nil {
		return "", err
	}
	if i.pending != nil {
		return "", ErrUnauthorized
	}
	// The actual pipe authenticated OS peer before calling this private issuer.
	// Current DB owner (not requested subject/action) is the controlling authority.
	owner, err := a.store.db.CurrentInstallationAuthorization(i.ctx, a.store.identity.OwnerPrincipalID, "", "")
	if err != nil || owner.Revoked || owner.TenantID != a.store.identity.TenantID {
		return "", ErrUnauthorized
	}
	if grant.WorkspaceID != "" {
		current, err := a.store.db.CurrentInstallationAuthorization(i.ctx, grant.PrincipalID, grant.WorkspaceID, grant.Action)
		if err != nil || !current.WorkspaceAllowed {
			return "", ErrUnauthorized
		}
	}
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return "", ErrUnauthorized
	}
	defer clear(raw[:])
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	hash, _ := tokenHash(token)
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return "", ErrUnauthorized
	}
	id[6] = (id[6] & 15) | 64
	id[8] = (id[8] & 63) | 128
	sessionID := fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	pending, err := a.store.db.CreateInstallationSession(i.ctx, a.store.identity.OwnerPrincipalID, grant.PrincipalID, sessionID, hash, time.Now().Add(ttl), grant.WorkspaceID, grant.Action)
	if err != nil {
		return "", a.mutationErrorLocked(err)
	}
	// Register before unlocking: Close cannot release the DB/lease between the
	// committed pending row and activation/revocation. No long pipe lock is held.
	i.pending = &pending
	a.store.pendingDeliveries++
	return token, nil
}
func (i *databaseIssuer) Revoke(token string) {
	hash, err := tokenHash(token)
	if err != nil {
		return
	}
	a := i.auth
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	// Even a canceled delivery must physically finish revocation or fence it.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = a.readyLocked(); err != nil {
		i.revokeError = err
		return
	}
	if err = a.store.db.RevokeUndeliveredSession(ctx, hash); err != nil {
		// A known rollback of revocation also leaves a potentially delivered bearer
		// live; fence all dependencies rather than pretending failed ACK is safe.
		a.store.authUncertain = true
		i.revokeError = ErrAuthorizationUnknown
	}
}
func (a *Authenticator) mutationErrorLocked(err error) error {
	if errors.Is(err, sqlite.ErrUnknown) {
		a.store.authUncertain = true
		return ErrAuthorizationUnknown
	}
	return ErrUnauthorized
}

// SetPrincipalAuthorization requires a current DB owner session from the actual
// operator pipe. Replacing even equal grants advances the version; no retry or
// implicit clearing of an unknown fence occurs. No registration/T013 writes.
func (a *Authenticator) SetPrincipalAuthorization(ctx context.Context, operator *Authorization, principalID string, revoked bool, grants []sqlite.WorkspaceGrant) error {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if operator == nil || operator.auth == nil || operator.auth.store != a.store || operator.action != "installation.operator" {
		return ErrUnauthorized
	}
	if err := operator.checkLocked(ctx); err != nil {
		return err
	}
	if err := a.store.db.ReplacePrincipalAuthorization(ctx, operator.hash, operator.version, principalID, revoked, grants); err != nil {
		return a.mutationErrorLocked(err)
	}
	return nil
}
func (a *Authenticator) RevokeSession(ctx context.Context, actor *Authorization, token string) error {
	hash, err := tokenHash(token)
	if err != nil {
		return ErrUnauthorized
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if actor == nil || actor.auth == nil || actor.auth.store != a.store {
		return ErrUnauthorized
	}
	if err = actor.checkLocked(ctx); err != nil {
		return err
	}
	// Nonoperator capabilities may revoke only their own bearer.
	if hash != actor.hash && actor.action != "installation.operator" {
		return ErrUnauthorized
	}
	if err = a.store.db.RevokeInstallationSession(ctx, actor.hash, actor.version, hash); err != nil {
		return a.mutationErrorLocked(err)
	}
	return nil
}
