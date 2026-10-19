//go:build windows && amd64

// Package httptransport only validates, authorizes, projects and delivers. It
// never owns SQL, directories, save admission or physical lifecycle cleanup.
package httptransport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	win "golang.org/x/sys/windows"
)

type Event struct {
	CorrelationID string             `json:"correlationId"`
	Operation     string             `json:"operation"`
	Code          protocol.ErrorCode `json:"code,omitempty"`
	Status        int                `json:"status"`
}
type Handler struct {
	auth        *installation.Authenticator
	creator     *installation.WorkspaceCreator
	directories *installation.DirectoryCoordinator
	inspector   *installation.DirectoryInspector
	opener      *installation.WorkspaceOpener
	authority   string
	log         func(Event)
	sequence    atomic.Uint64
}

func New(auth *installation.Authenticator, store *installation.Store, scratch, authority string, log func(Event)) (*Handler, error) {
	host, port, err := net.SplitHostPort(authority)
	p, e := strconv.ParseUint(port, 10, 16)
	ip := net.ParseIP(host)
	if err != nil || e != nil || p == 0 || ip == nil || !ip.IsLoopback() || auth == nil || store == nil {
		return nil, errors.New("invalid trusted HTTP configuration")
	}
	c, err := installation.NewWorkspaceCreator(auth)
	if err != nil {
		return nil, err
	}
	d, err := installation.NewDirectoryCoordinator(auth)
	if err != nil {
		return nil, err
	}
	i, err := installation.NewDirectoryInspector(store, scratch)
	if err != nil {
		return nil, err
	}
	o, err := installation.NewWorkspaceOpener(auth)
	if err != nil {
		return nil, err
	}
	return &Handler{auth: auth, creator: c, directories: d, inspector: i, opener: o, authority: authority, log: log}, nil
}
func (h *Handler) correlation() protocol.CorrelationId {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return protocol.CorrelationId(hex.EncodeToString(b[:]))
	}
	return protocol.CorrelationId("host-" + strconv.FormatUint(h.sequence.Add(1), 10))
}
func (h *Handler) emit(e Event) {
	if h.log != nil {
		h.log(e)
	}
}
func (h *Handler) write(w http.ResponseWriter, status int, v any, corr protocol.CorrelationId, op, key string, install bool, check func() error) {
	raw, err := json.Marshal(v)
	if err != nil {
		h.fail(w, http.StatusServiceUnavailable, protocol.ErrorCodeVIEWDELIVERYFAILED, corr, op, key, install)
		return
	}
	if check != nil {
		if err = check(); err != nil {
			h.failure(w, err, corr, op, key, install)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err = w.Write(raw); err != nil {
		h.emit(Event{string(corr), op, protocol.ErrorCodeVIEWDELIVERYFAILED, status})
		return
	}
	h.emit(Event{CorrelationID: string(corr), Operation: op, Status: status})
}
func (h *Handler) fail(w http.ResponseWriter, status int, code protocol.ErrorCode, corr protocol.CorrelationId, op, key string, install bool) {
	v := protocol.ErrorView{Code: code, Outcome: "rejected", CorrelationId: corr, RetryPolicy: "none", SafeMessage: "The request cannot be completed in the current authorized state."}
	if code == protocol.ErrorCodeCOMMITUNKNOWN || code == protocol.ErrorCodeINSTALLATIONSTATEUNKNOWN || code == protocol.ErrorCodeREGISTRATIONPENDING || code == protocol.ErrorCodeVIEWDELIVERYFAILED {
		v.Outcome = "unknown"
		v.RetryPolicy = "investigate"
	}
	if code == protocol.ErrorCodeSTALEBASELINE || code == protocol.ErrorCodeINVALIDCANDIDATE {
		v.RetryPolicy = "newPreview"
	}
	if key != "" {
		id := protocol.UUID(key)
		v.OperationId = &id
	}
	scope := protocol.ErrorViewScopeWorkspace
	if install {
		scope = "installation"
	}
	v.Scope = &scope
	// Error text is constant: do not format or log native errors, paths, SQL,
	// request bodies, bearer headers, or untrusted client correlation strings.
	raw, err := json.Marshal(v)
	if err != nil {
		raw = []byte(`{"code":"INSTALLATION_STATE_UNKNOWN","outcome":"unknown","correlationId":"unavailable","retryPolicy":"investigate","safeMessage":"Response unavailable."}`)
		status = 503
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
	h.emit(Event{string(corr), op, code, status})
}
func (h *Handler) failure(w http.ResponseWriter, err error, corr protocol.CorrelationId, op, key string, install bool) {
	code, status := protocol.ErrorCodeRECOVERYREQUIRED, 409
	var inspection *installation.InspectionFailure
	switch {
	case errors.Is(err, installation.ErrAuthorizationUnknown):
		code, status = protocol.ErrorCodeINSTALLATIONSTATEUNKNOWN, 503
	case errors.Is(err, installation.ErrUnauthorized), errors.Is(err, workspace.ErrOperationAccess):
		code, status = protocol.ErrorCodeFORBIDDEN, 403
	case errors.Is(err, workspace.ErrCapacity):
		code, status = protocol.ErrorCodeCAPACITYLIMIT, 429
	case errors.Is(err, workspace.ErrNotFound), errors.Is(err, installation.ErrWorkspaceCreationNotFound):
		code, status = protocol.ErrorCodeNOTFOUND, 404
	case errors.Is(err, workspace.ErrOperationHistoryUnavailable):
		code, status = protocol.ErrorCodeOPERATIONHISTORYUNAVAILABLE, 409
	case errors.Is(err, workspace.ErrSaveKeyReuse), errors.Is(err, sqlite.ErrInstallationOperationConflict):
		code = protocol.ErrorCodeOPERATIONKEYREUSE
	case errors.Is(err, workspace.ErrStaleHandle):
		code = protocol.ErrorCodeSTALEHANDLE
	case errors.Is(err, workspace.ErrBaseline), errors.Is(err, sqlite.ErrConflict):
		code = protocol.ErrorCodeSTALEBASELINE
	case errors.Is(err, workspace.ErrPreview), errors.Is(err, workspace.ErrSaveInput), errors.Is(err, workspace.ErrRecordQuery):
		code, status = protocol.ErrorCodeINVALIDINPUT, 400
	case errors.Is(err, workspace.ErrLifecycle):
		code = protocol.ErrorCodeWORKSPACECLOSED
	case errors.Is(err, workspace.ErrOpenUncertain), errors.Is(err, sqlite.ErrUnknown), errors.Is(err, sqlite.ErrFenced):
		code, status = protocol.ErrorCodeCOMMITUNKNOWN, 503
	case errors.Is(err, installation.ErrWorkspaceCreation), errors.Is(err, installation.ErrDirectoryBinding):
		code, status = protocol.ErrorCodeREGISTRATIONPENDING, 503
	case errors.As(err, &inspection):
		code = inspection.Code
	case errors.Is(err, workspace.ErrUnsupportedDirectoryFormat):
		code = protocol.ErrorCodeUNSUPPORTEDFORMAT
	case errors.Is(err, workspace.ErrDirectoryDigestMismatch):
		code = protocol.ErrorCodeDIGESTMISMATCH
	case errors.Is(err, workspace.ErrDirectoryIdentityMismatch):
		code = protocol.ErrorCodeIDENTITYCONFLICT
	case errors.Is(err, workspace.ErrDirectoryDescriptor):
		code = protocol.ErrorCodeINCOMPLETEDIRECTORY
	case errors.Is(err, sqlite.ErrSchema):
		code = protocol.ErrorCodeUNSUPPORTEDSCHEMA
	case errors.Is(err, win.ERROR_SHARING_VIOLATION):
		code = protocol.ErrorCodeWRITERBUSY
	}
	if install && code == protocol.ErrorCodeCOMMITUNKNOWN {
		code = protocol.ErrorCodeINSTALLATIONSTATEUNKNOWN
	}
	h.fail(w, status, code, corr, op, key, install)
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	corr := h.correlation()
	op := "boundary"
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Correlation-ID", string(corr))
	defer func() {
		if recover() != nil {
			h.fail(w, 503, protocol.ErrorCodeCOMMITUNKNOWN, corr, op, "", false)
		}
	}()
	// Authorize the concrete connection boundary before resolving a resource. An
	// absolute-form URL must never smuggle a second authority past Host matching.
	if r == nil || r.URL == nil || r.URL.IsAbs() || r.URL.Host != "" || r.URL.User != nil || r.URL.Fragment != "" {
		h.fail(w, 403, protocol.ErrorCodeFORBIDDEN, corr, op, "", false)
		return
	}
	identity, err := h.auth.AuthorizeHTTP(r.Context(), r, h.authority, "", "")
	if err != nil {
		h.failure(w, err, corr, op, "", false)
		return
	}
	route, ok := protocol.MatchRoute(r.Method, r.URL.Path)
	if !ok {
		h.fail(w, 501, protocol.ErrorCodeCAPABILITYUNAVAILABLE, corr, op, "", false)
		return
	}
	op = route.ID
	if r.URL.EscapedPath() != r.URL.Path {
		h.fail(w, 400, protocol.ErrorCodeINVALIDINPUT, corr, op, "", false)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || protocol.ValidateRouteParameters(route, query) != nil {
		h.fail(w, 400, protocol.ErrorCodeINVALIDINPUT, corr, op, "", false)
		return
	}
	action := route.Action
	if action == "authenticated" {
		action = ""
	}
	ws := route.Parameters["workspaceId"]
	installationScope := ws == "" || route.ID == "relinkWorkspace"
	a := identity
	if action != "" {
		a, err = h.auth.AuthorizeHTTP(r.Context(), r, h.authority, ws, action)
		if err != nil {
			h.failure(w, err, corr, op, "", installationScope)
			return
		}
	}
	if len(r.Header.Values("Content-Encoding")) != 0 || (r.Header.Get("Content-Length") != "" && r.ContentLength < 0) {
		h.fail(w, 400, protocol.ErrorCodeINVALIDINPUT, corr, op, "", installationScope)
		return
	}
	body := io.Reader(strings.NewReader(""))
	if r.Body != nil {
		body = r.Body
	}
	raw, err := io.ReadAll(io.LimitReader(body, protocol.RequestMaxBytes+1))
	if len(raw) > protocol.RequestMaxBytes {
		h.fail(w, 413, protocol.ErrorCodeREQUESTTOOLARGE, corr, op, "", installationScope)
		return
	}
	if err != nil {
		h.fail(w, 400, protocol.ErrorCodeINVALIDINPUT, corr, op, "", installationScope)
		return
	}
	if len(raw) > 0 && (len(r.Header.Values("Content-Type")) != 1 || r.Header.Get("Content-Type") != "application/json") {
		h.fail(w, 400, protocol.ErrorCodeINVALIDINPUT, corr, op, "", installationScope)
		return
	}
	if protocol.ValidateOperationBody(op, raw) != nil {
		h.fail(w, 400, protocol.ErrorCodeINVALIDINPUT, corr, op, "", installationScope)
		return
	}
	h.dispatch(w, r, a, route, query, raw, corr)
}
