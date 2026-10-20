//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

const inspectionHTTPEnvironment = "ABOARD_ACCEPTANCE_INSPECTION_HTTP"
const inspectionHTTPControlPath = "/aboard-acceptance-inspection-control/"

type inspectionHTTPDelegate struct {
	original http.RoundTripper
	attempts atomic.Uint64
}

func (d *inspectionHTTPDelegate) RoundTrip(r *http.Request) (*http.Response, error) {
	d.attempts.Add(1)
	return d.original.RoundTrip(r)
}
func (d *inspectionHTTPDelegate) CloseIdleConnections() {
	if closer, ok := d.original.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

var inspectionHTTPInstall sync.Once
var inspectionHTTPInstalled *inspectionHTTPDelegate

// Called ONCE before execute/Host goroutines, including for invalid CLI argv.
// Installation performs no network IO and reads no configuration or credentials.
// Counts are monotonic; a window never replaces/resets/restores the transport.
func InstallAcceptanceHTTPObservation() {
	inspectionHTTPInstall.Do(func() {
		if http.DefaultTransport == nil {
			return
		}
		d := &inspectionHTTPDelegate{original: http.DefaultTransport}
		inspectionHTTPInstalled = d
		http.DefaultTransport = d
	})
}
func inspectionHTTPSnapshot() (uint64, bool) {
	d := inspectionHTTPInstalled
	if d == nil || http.DefaultTransport != d {
		return 0, false
	}
	return d.attempts.Load(), true
}

type inspectionHTTPConfig struct {
	Version                        int `json:"version"`
	Fixture, Root, Port, Challenge string
}
type inspectionHTTPObservation struct {
	ControlBefore, ControlAfter, Before, After uint64
	Available, Conserved, BodyEnded            bool
	ConfigDigest                               string
}

// This is a CLOSED loopback diagnostic configuration, not a supplied URL or
// outbound client capability. Original current-auth + fixture gates run first.
func readInspectionHTTP(c inspectionObservationConfig, encoded string) (inspectionHTTPConfig, bool) {
	var h inspectionHTTPConfig
	if encoded == "" || len(encoded) > 8192 {
		return h, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return h, false
	}
	value, err := protocol.ParseJSON(raw)
	if err != nil {
		return h, false
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 5 {
		return h, false
	}
	version, ok := fields["version"].(json.Number)
	if !ok || string(version) != "1" {
		return h, false
	}
	for _, key := range []string{"Fixture", "Root", "Port", "Challenge"} {
		if _, ok := fields[key].(string); !ok {
			return h, false
		}
	}
	if json.Unmarshal(raw, &h) != nil || h.Fixture != c.Fixture || h.Root != c.Root {
		return h, false
	}
	port, err := strconv.Atoi(h.Port)
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != h.Port {
		return h, false
	}
	challenge, err := hex.DecodeString(h.Challenge)
	if err != nil || len(challenge) != 16 || hex.EncodeToString(challenge) != h.Challenge {
		return h, false
	}
	return h, true
}

type inspectionHTTPEndUnknown struct {
	client   *http.Client
	request  *http.Request
	response *http.Response
	cause    error
}

func (e *inspectionHTTPEndUnknown) Error() string { return "inspection HTTP control end unknown" }
func (e *inspectionHTTPEndUnknown) Unwrap() error { return platform.ErrPhysicalCloseUnknown }

// Actual DEFAULT delegate positive, outside Before/After. The response body must
// reach EOF and successfully close. The process-owned original transport is NOT
// a per-Inspect resource; idle cleanup is forwarded, not claimed as active join.
func inspectionHTTPControl(c inspectionHTTPConfig, id string, owner *inspectionOwner) *inspectionHTTPObservation {
	v := &inspectionHTTPObservation{}
	raw, _ := json.Marshal(c)
	hash := sha256.Sum256(raw)
	v.ConfigDigest = hex.EncodeToString(hash[:])
	before, ok := inspectionHTTPSnapshot()
	v.ControlBefore = before
	if !ok {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+c.Port+inspectionHTTPControlPath+c.Challenge+"/"+id, nil)
	if err != nil {
		return v
	}
	request.Close = true
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, callErr := client.Do(request) // nil Transport/Jar exercises installed DEFAULT
	var body []byte
	var readErr, closeErr error
	if response != nil {
		body, readErr = io.ReadAll(io.LimitReader(response.Body, 129))
		closeErr = response.Body.Close() // FIRST result only; never retry an unknown Close
	}
	client.CloseIdleConnections()
	after, two := inspectionHTTPSnapshot()
	v.ControlAfter = after
	expected := "aboard-inspection-control:" + c.Challenge + ":" + id
	v.BodyEnded = callErr == nil && response != nil && readErr == nil && closeErr == nil && len(body) <= 128
	v.Available = two && after == before+1 && v.BodyEnded && response.StatusCode == 200 && response.ContentLength == int64(len(expected)) && response.Close && string(body) == expected
	if !v.BodyEnded {
		owner.failure = errors.Join(owner.failure, &inspectionHTTPEndUnknown{client, request, response, errors.Join(errors.New("HTTP body end not proven"), callErr, readErr, closeErr)})
	}
	// No URLs, response/header/body/credentials are logged or serialized.
	clear(body)
	return v
}
