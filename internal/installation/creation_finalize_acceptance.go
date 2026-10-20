//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

const finalizeProbeEnvironment = "ABOARD_ACCEPTANCE_FINALIZE"

var errFinalizeProbe = errors.New("acceptance finalize fixture rejected")

type finalizeProbe struct {
	Version              int `json:"version"`
	Fixture, OperationID string
	Ordinal              int
}
type finalizeReport struct {
	Version                                          int    `json:"version"`
	PID                                              int    `json:"pid"`
	OperationID                                      string `json:"operationId"`
	Ordinal, Calls, Injected, NativeBOOL, SQLiteCode int
	Unknown, NativeControlFailed, CallbackDrained    bool
}
type finalizeFlushState struct {
	mu                          sync.Mutex
	active                      bool
	config                      finalizeProbe
	handle                      win.Handle
	databasePath                string // exact retained workspace DB for save probes; empty preserves creation target
	calls, injected, nativeBOOL int
	midpoint                    *saveNativeMidpoint // immutable owner published before CAS; only callback mutates evidence under mu
}

var armedFinalize atomic.Pointer[finalizeFlushState]

// This Go funcvalue is a process-lifetime GC root: the uintptr held by generated
// C is NOT a Go root. Never replace/restore the VFS pointer while main is live.
var finalizeFlushFunction func(*libc.TLS, uintptr) int32 = finalizeFlushDelegate
var finalizeDelegateError error

func init() { finalizeDelegateError = installFinalizeDelegate() }
func installFinalizeDelegate() error {
	tls := libc.NewTLS()
	defer tls.Close()
	name, e := libc.CString("win32")
	if e != nil {
		return e
	}
	defer libc.Xfree(tls, name)
	vfs := sqlite3.Xsqlite3_vfs_find(tls, name)
	if vfs == 0 {
		return errFinalizeProbe
	}
	var view sqlite3.Tsqlite3_vfs
	libc.Xmemcpy(tls, uintptr(unsafe.Pointer(&view)), vfs, libc.Tsize_t(unsafe.Sizeof(view)))
	if view.FiVersion < 3 || view.FxSetSystemCall == 0 || view.FxGetSystemCall == 0 {
		return errFinalizeProbe
	}
	call, e := libc.CString("FlushFileBuffers")
	if e != nil {
		return e
	}
	defer libc.Xfree(tls, call)
	get := *(*func(*libc.TLS, uintptr, uintptr) uintptr)(unsafe.Pointer(&view.FxGetSystemCall))
	set := *(*func(*libc.TLS, uintptr, uintptr, uintptr) int32)(unsafe.Pointer(&view.FxSetSystemCall))
	if get(tls, vfs, call) == 0 {
		return errFinalizeProbe
	}
	pointer := *(*uintptr)(unsafe.Pointer(&finalizeFlushFunction))
	if set(tls, vfs, call, pointer) != sqlite3.SQLITE_OK || get(tls, vfs, call) != pointer {
		return errFinalizeProbe
	}
	return nil
}
func finalizeHandlePath(h win.Handle) string {
	var buffer [32768]uint16
	n, e := win.GetFinalPathNameByHandle(h, &buffer[0], uint32(len(buffer)), 0)
	if e != nil || n == 0 || n >= uint32(len(buffer)) {
		return ""
	}
	return strings.TrimPrefix(win.UTF16ToString(buffer[:n]), `\\?\`)
}
func finalizeFlushDelegate(tls *libc.TLS, actual uintptr) int32 {
	state := armedFinalize.Load()
	if state == nil {
		return libc.XFlushFileBuffers(tls, actual)
	}
	// Immutable exact-path filtering MUST precede mu: held A must never stall B.
	// A loaded state remains GC-rooted even after CAS disarm; these fields never change.
	path := finalizeHandlePath(win.Handle(actual))
	database := state.databasePath
	if database == "" {
		database = filepath.Join(state.config.Fixture, "installation", "installation.sqlite")
	}
	if path != database && path != database+"-journal" {
		return libc.XFlushFileBuffers(tls, actual)
	}
	state.mu.Lock()
	defer state.mu.Unlock() // also preserves the existing disarm + callback join on panic
	if state.active {
		state.calls++
		if state.midpoint != nil {
			result := libc.XFlushFileBuffers(tls, actual) // SAME actual native handle, ONCE, result unchanged
			state.midpoint.afterSync(path, database, int(result))
			return result
		}
		if state.config.Ordinal > 0 && state.calls == state.config.Ordinal {
			state.injected++
			result := libc.XFlushFileBuffers(tls, uintptr(state.handle)) // actual native BOOL, never fabricated
			state.nativeBOOL = int(result)
			return result
		}
	}
	return libc.XFlushFileBuffers(tls, actual)
}
func readFinalizeProbe(c *WorkspaceCreator, r CreateWorkspaceRequest) (finalizeProbe, bool, error) {
	var out finalizeProbe
	encoded := os.Getenv(finalizeProbeEnvironment)
	if encoded == "" {
		return out, false, nil
	}
	if os.Getenv(creationProbeEnvironment) != "" || len(encoded) > 8192 {
		return out, false, errFinalizeProbe
	}
	raw, e := base64.RawURLEncoding.DecodeString(encoded)
	if e != nil {
		return out, false, errFinalizeProbe
	}
	value, e := protocol.ParseJSON(raw)
	if e != nil {
		return out, false, errFinalizeProbe
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 4 {
		return out, false, errFinalizeProbe
	}
	version, vok := fields["version"].(json.Number)
	ordinal, nok := fields["Ordinal"].(json.Number)
	if !vok || string(version) != "1" || !nok {
		return out, false, errFinalizeProbe
	}
	n, e := strconv.Atoi(string(ordinal))
	if e != nil || n < 0 || n > 8 || strconv.Itoa(n) != string(ordinal) {
		return out, false, errFinalizeProbe
	}
	for _, k := range []string{"Fixture", "OperationID"} {
		if _, ok := fields[k].(string); !ok {
			return out, false, errFinalizeProbe
		}
	}
	if json.Unmarshal(raw, &out) != nil || !creationProbeRoot(out.Fixture) {
		return out, false, errFinalizeProbe
	}
	if json.Unmarshal([]byte(`"`+out.OperationID+`"`), new(protocol.UUID)) != nil {
		return out, false, errFinalizeProbe
	}
	if c == nil || c.auth == nil || c.auth.store == nil || c.auth.store.root != filepath.Join(out.Fixture, "installation") {
		return out, false, errFinalizeProbe
	}
	if r.OperationID != out.OperationID {
		return out, false, nil
	}
	if r.RootLocator != filepath.Join(out.Fixture, "allowed", "finalize-target") || finalizeDelegateError != nil {
		return out, false, errFinalizeProbe
	}
	return out, true, nil
}

// Owned error is retained by the existing workspaceCreation/Store native fence;
// failed native Close is sticky and is NEVER retried using a numeric handle.
type finalizeCloseError struct {
	cause error
	owner any
}

func (e *finalizeCloseError) Error() string { return platform.ErrPhysicalCloseUnknown.Error() }
func (e *finalizeCloseError) Unwrap() []error {
	return []error{platform.ErrPhysicalCloseUnknown, e.cause}
}

// Cleanup runs on adapter panic as well. A panic supplies no receipt/evidence,
// so it is conservatively unknown with NO fabricated native/SQLite error code.
func callFinalizeProbe(state *finalizeFlushState, apply func() error, closeStimulus func() error) (result error, closeErr error) {
	completed := false
	defer func() {
		if !completed {
			_ = recover() // also handles legacy GODEBUG=panicnil=1
			result = errors.Join(sqlite.ErrUnknown, errFinalizeProbe)
		}
		armedFinalize.CompareAndSwap(state, nil)
		state.mu.Lock() // join every callback already using the stimulus
		defer state.mu.Unlock()
		state.active = false
		closeErr = closeStimulus()
	}()
	result = apply()
	completed = true
	return
}
func writeFinalizeReport(root string, report finalizeReport) error {
	raw, e := json.Marshal(report)
	if e != nil || len(raw) > 4096 {
		return errFinalizeProbe
	}
	file, e := os.OpenFile(filepath.Join(root, "finalize-evidence.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, writeErr := file.Write(raw)
	if closeErr := file.Close(); closeErr != nil {
		return errors.Join(writeErr, &finalizeCloseError{closeErr, file})
	}
	return writeErr
}

// Called only inside the ORIGINAL Store.mu boundary after current authorization
// and successful source DB Close. No SQL, scope, transaction or owner is replaced.
func creationFinalize(c *WorkspaceCreator, _ *Authorization, o sqlite.DirectoryOperation, reconcile bool, apply func() error) error {
	config, matched, e := readFinalizeProbe(c, CreateWorkspaceRequest{OperationID: o.OperationID, RootLocator: o.TargetLocator})
	if e != nil {
		return e
	}
	if !matched {
		return apply()
	}
	s := c.auth.store
	w := s.workspaceCreations[o.OperationID]
	if reconcile || o.Kind != "create" || o.Phase != string(sqlite.DescriptorPublished) || o.Outcome != string(sqlite.InstallationAccepted) || s.closed || s.openFailure || s.nativeFailure != nil || s.authUncertain || s.workspaceStopping || s.pendingDeliveries != 0 || s.pendingWorkspaceCreations != 1 || s.liveWorkspaceHandles != 0 || len(s.workspaceOpenOwners) != 0 || len(s.inspections) != 0 || len(s.workspaceCreations) != 1 || w == nil || !w.active || w.db != nil || w.openUnknown || w.nativeFailure != nil {
		return errFinalizeProbe
	}
	native, e := win.UTF16PtrFromString(filepath.Join(config.Fixture, "config.json"))
	if e != nil {
		return e
	}
	handle, e := win.CreateFile(native, win.GENERIC_READ, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE|win.FILE_SHARE_DELETE, nil, win.OPEN_EXISTING, win.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		return e
	}
	state := &finalizeFlushState{active: true, config: config, handle: handle}
	closeStimulus := func() error {
		if err := win.CloseHandle(handle); err != nil {
			return &finalizeCloseError{err, state}
		}
		return nil
	}
	kind, typeErr := win.GetFileType(handle)
	var info win.ByHandleFileInformation
	if typeErr != nil || kind != win.FILE_TYPE_DISK || win.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&(win.FILE_ATTRIBUTE_DIRECTORY|win.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 || finalizeHandlePath(handle) != filepath.Join(config.Fixture, "config.json") {
		return errors.Join(errFinalizeProbe, closeStimulus())
	}
	control := win.FlushFileBuffers(handle)
	if !errors.Is(control, win.ERROR_ACCESS_DENIED) {
		return errors.Join(errFinalizeProbe, closeStimulus())
	}
	if !armedFinalize.CompareAndSwap(nil, state) {
		return errors.Join(errFinalizeProbe, closeStimulus())
	}
	// apply is the unmodified original FinalizeCreation on the same connection.
	result, closeErr := callFinalizeProbe(state, apply, closeStimulus)
	state.mu.Lock()
	report := finalizeReport{Version: 1, PID: os.Getpid(), OperationID: o.OperationID, Ordinal: config.Ordinal, Calls: state.calls, Injected: state.injected, NativeBOOL: state.nativeBOOL, Unknown: errors.Is(result, sqlite.ErrUnknown), NativeControlFailed: true, CallbackDrained: true}
	state.mu.Unlock()
	var coded interface{ Code() int }
	if errors.As(result, &coded) {
		report.SQLiteCode = coded.Code()
	}
	return errors.Join(result, closeErr, writeFinalizeReport(config.Fixture, report))
}
