//go:build windows && amd64

package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrUnauthorized = errors.New("unauthorized")

// Grant is a minimal prototype grant, not workspace/installation persistence.
// A returned copy never replaces current authorization on the next request.
type Grant struct {
	PrincipalID string
	WorkspaceID string
	Action      string
}
type session struct {
	grant   Grant
	expires time.Time
	version uint64
}
type Sessions struct {
	mu        sync.Mutex
	entries   map[[32]byte]session
	versions  map[string]uint64
	uncertain bool
}

func NewSessions() *Sessions {
	return &Sessions{entries: make(map[[32]byte]session), versions: make(map[string]uint64)}
}

// Issue is called only by the authenticated operator control plane. Only a
// SHA-256 digest is retained. Its opaque bearer is delivered exactly once.
func (s *Sessions) Issue(grant Grant, ttl time.Duration) (string, error) {
	if grant.PrincipalID == "" || grant.WorkspaceID == "" || grant.Action == "" || ttl <= 0 || ttl > 5*time.Minute {
		return "", ErrUnauthorized
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	defer clear(secret[:])
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	digest := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.uncertain {
		return "", ErrUnauthorized
	}
	// Expired entries are discarded on each issuance, keeping short sessions from
	// accumulating indefinitely. Live-capacity policy belongs to later integration.
	now := time.Now()
	for key, entry := range s.entries {
		if !now.Before(entry.expires) {
			delete(s.entries, key)
		}
	}
	s.entries[digest] = session{grant: grant, expires: now.Add(ttl), version: s.versions[grant.PrincipalID]}
	return token, nil
}
func (s *Sessions) Revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, sha256.Sum256([]byte(token)))
}
func (s *Sessions) RevokePrincipal(principal string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versions[principal]++
}
func (s *Sessions) SetUncertain(uncertain bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uncertain = uncertain
	if uncertain {
		// Resolving uncertainty never resurrects an earlier cached bearer.
		clear(s.entries)
	}
}
func (s *Sessions) Authorize(token, workspace, action string) (Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var zero Grant
	if len(token) != 43 || workspace == "" || action == "" || s.uncertain {
		return zero, ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(token))
	entry, ok := s.entries[digest]
	if !ok || !time.Now().Before(entry.expires) || entry.version != s.versions[entry.grant.PrincipalID] || entry.grant.WorkspaceID != workspace || entry.grant.Action != action {
		return zero, ErrUnauthorized
	}
	return entry.grant, nil
}

// AuthorizeHTTP is a pure prototype boundary check, not an HTTP listener. The
// caller supplies the exact trusted loopback listener authority; request body
// identity is deliberately not an input. Browsers/cookies/CORS are not supported.
func (s *Sessions) AuthorizeHTTP(r *http.Request, authority, workspace, action string) (Grant, error) {
	host, port, splitErr := net.SplitHostPort(authority)
	ip := net.ParseIP(host)
	number, portErr := strconv.ParseUint(port, 10, 16)
	if splitErr != nil || ip == nil || !ip.IsLoopback() || portErr != nil || number == 0 || r.Host != authority || r.Method == http.MethodOptions || len(r.Header.Values("Origin")) != 0 || len(r.Header.Values("Cookie")) != 0 {
		return Grant{}, ErrUnauthorized
	}
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return Grant{}, ErrUnauthorized
	}
	return s.Authorize(strings.TrimPrefix(values[0], "Bearer "), workspace, action)
}
