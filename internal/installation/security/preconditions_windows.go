//go:build windows && amd64

package security

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/abworks-dev/aboard-host/internal/protocol"
	win "golang.org/x/sys/windows"
)

// This private, versioned read protocol supplies only otherwise-unavailable
// public CAS preconditions. It conveys no location, grant, credential response,
// filesystem command, or write operation. Snapshots are not permissions.
const controlFrameLimit = 8192

var ErrControlUnavailable = errors.New("protected control snapshot unavailable")

type PreconditionRequest struct {
	Version     int    `json:"version"`
	Kind        string `json:"kind"`
	Bearer      string `json:"bearer"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	OperationID string `json:"operationId,omitempty"`
	IntendedUse string `json:"intendedUse,omitempty"`
}
type PreconditionReply struct {
	Version             int                        `json:"version"`
	Code                string                     `json:"code"`
	Kind                string                     `json:"kind,omitempty"`
	InstallationID      protocol.UUID              `json:"installationId,omitempty"`
	TenantID            protocol.UUID              `json:"tenantId,omitempty"`
	WorkspaceID         protocol.UUID              `json:"workspaceId,omitempty"`
	RegistrationVersion protocol.PositiveRevision  `json:"registrationVersion,omitempty"`
	OperationID         protocol.UUID              `json:"operationId,omitempty"`
	ExpectedPhase       protocol.InstallationPhase `json:"expectedPhase,omitempty"`
}
type PreconditionReader interface {
	ReadControlPrecondition(context.Context, PreconditionRequest) (PreconditionReply, func(context.Context) error, error)
}

func validControlRequest(r PreconditionRequest) error {
	if r.Version != 1 || len(r.Bearer) != 43 {
		return ErrUnauthorized
	}
	switch r.Kind {
	case "registration":
		if r.OperationID != "" || (r.IntendedUse != "open" && r.IntendedUse != "relink") {
			return ErrUnauthorized
		}
		if _, err := json.Marshal(protocol.UUID(r.WorkspaceID)); err != nil {
			return ErrUnauthorized
		}
	case "installation-operation":
		if r.WorkspaceID != "" || r.IntendedUse != "" {
			return ErrUnauthorized
		}
		if _, err := json.Marshal(protocol.UUID(r.OperationID)); err != nil {
			return ErrUnauthorized
		}
	default:
		return ErrUnauthorized
	}
	return nil
}
func validControlReply(r PreconditionRequest, v PreconditionReply) error {
	if v.Version != 1 {
		return ErrControlUnavailable
	}
	if v.Code != "ok" {
		if v.Code != "denied" && v.Code != "unavailable" {
			return ErrControlUnavailable
		}
		if v.Kind != "" || v.InstallationID != "" || v.TenantID != "" || v.WorkspaceID != "" || v.RegistrationVersion != "" || v.OperationID != "" || v.ExpectedPhase != "" {
			return ErrControlUnavailable
		}
		return nil
	}
	if v.Kind != r.Kind {
		return ErrControlUnavailable
	}
	if _, err := json.Marshal(v.InstallationID); err != nil {
		return ErrControlUnavailable
	}
	if _, err := json.Marshal(v.TenantID); err != nil {
		return ErrControlUnavailable
	}
	switch r.Kind {
	case "registration":
		if string(v.WorkspaceID) != r.WorkspaceID || v.OperationID != "" || v.ExpectedPhase != "" {
			return ErrControlUnavailable
		}
		if _, err := json.Marshal(v.RegistrationVersion); err != nil {
			return ErrControlUnavailable
		}
	case "installation-operation":
		if string(v.OperationID) != r.OperationID || v.RegistrationVersion != "" {
			return ErrControlUnavailable
		}
		if _, err := json.Marshal(v.WorkspaceID); err != nil {
			return ErrControlUnavailable
		}
		if _, err := json.Marshal(v.ExpectedPhase); err != nil {
			return ErrControlUnavailable
		}
	default:
		return ErrControlUnavailable
	}
	return nil
}
func decodeControl(b []byte, out any) error {
	if len(b) == 0 || len(b) > controlFrameLimit {
		return ErrUnauthorized
	}
	parsed, err := protocol.ParseJSON(b)
	if err != nil {
		return ErrUnauthorized
	}
	object, ok := parsed.(map[string]any)
	if !ok {
		return ErrUnauthorized
	}
	version, ok := object["version"].(json.Number)
	if !ok || string(version) != "1" {
		return ErrUnauthorized
	}
	var keys []string
	switch out.(type) {
	case *PreconditionRequest:
		switch object["kind"] {
		case "registration":
			keys = []string{"version", "kind", "bearer", "workspaceId", "intendedUse"}
		case "installation-operation":
			keys = []string{"version", "kind", "bearer", "operationId"}
		default:
			return ErrUnauthorized
		}
	case *PreconditionReply:
		if object["code"] != "ok" {
			keys = []string{"version", "code"}
		} else {
			switch object["kind"] {
			case "registration":
				keys = []string{"version", "code", "kind", "installationId", "tenantId", "workspaceId", "registrationVersion"}
			case "installation-operation":
				keys = []string{"version", "code", "kind", "installationId", "tenantId", "workspaceId", "operationId", "expectedPhase"}
			default:
				return ErrUnauthorized
			}
		}
	default:
		return ErrUnauthorized
	}
	if len(object) != len(keys) {
		return ErrUnauthorized
	}
	for _, key := range keys {
		if key == "version" {
			continue
		}
		if _, ok := object[key].(string); !ok {
			return ErrUnauthorized
		}
	}
	// Exact spelling/type/presence is checked above: encoding/json's permissive
	// case-insensitive field matching and null-to-zero coercions cannot apply.
	if err = json.Unmarshal(b, out); err != nil {
		return ErrUnauthorized
	}
	return nil
}
func readControlFrame(ctx context.Context, h win.Handle) ([]byte, error) {
	var header [4]byte
	if err := readExact(ctx, h, header[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(header[:])
	if n == 0 || n > controlFrameLimit {
		return nil, ErrUnauthorized
	}
	b := make([]byte, int(n))
	if err := readExact(ctx, h, b); err != nil {
		clear(b)
		return nil, err
	}
	return b, nil
}
func writeControlFrame(ctx context.Context, h win.Handle, b []byte) error {
	if len(b) == 0 || len(b) > controlFrameLimit {
		return ErrControlUnavailable
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(b)))
	if err := writeExact(ctx, h, header[:]); err != nil {
		return err
	}
	return writeExact(ctx, h, b)
}
func (p *ControlPipe) servePrecondition(ctx context.Context, reader PreconditionReader) (err error) {
	stage := "server-precondition-request"
	defer func() { err = pipeFault(stage, err) }()
	raw, err := readControlFrame(ctx, p.handle)
	if err != nil {
		return err
	}
	defer clear(raw)
	stage = "server-precondition-shape"
	var request PreconditionRequest
	if err = decodeControl(raw, &request); err != nil {
		return err
	}
	if err = validControlRequest(request); err != nil {
		return err
	}
	reply, check, err := reader.ReadControlPrecondition(ctx, request)
	if err == nil {
		err = validControlReply(request, reply)
	}
	// Serialize first, then recheck current request authority immediately before
	// delivery; neither the installation mutex nor a DB transaction spans pipe IO.
	var payload []byte
	if err == nil {
		payload, err = json.Marshal(reply)
	}
	if err == nil {
		if check == nil {
			err = ErrUnauthorized
		} else {
			err = check(ctx)
		}
	}
	if err != nil {
		code := "unavailable"
		if errors.Is(err, ErrUnauthorized) {
			code = "denied"
		}
		payload, _ = json.Marshal(PreconditionReply{Version: 1, Code: code})
	}
	stage = "server-precondition-response"
	if err = writeControlFrame(ctx, p.handle, payload); err != nil {
		return err
	}
	stage = "server-precondition-ack"
	var ack [1]byte
	if err = readExact(ctx, p.handle, ack[:]); err != nil {
		return err
	}
	if ack[0] != 7 {
		return ErrUnauthorized
	}
	return nil
}

// ReceivePrecondition verifies the actual server SID, DACL and independently
// trusted live launch PID before sending any bearer bytes, just like sessions.
func ReceivePrecondition(ctx context.Context, name string, expectedPID uint32, request PreconditionRequest) (result PreconditionReply, err error) {
	stage := "precondition-input"
	defer func() {
		if stage == "precondition-open" && err == win.ERROR_PIPE_BUSY {
			return
		}
		err = pipeFault(stage, err)
	}()
	if err := validControlRequest(request); err != nil {
		return PreconditionReply{}, err
	}
	if expectedPID == 0 {
		return PreconditionReply{}, ErrUnauthorized
	}
	path, err := localPipeName(name)
	if err != nil {
		return PreconditionReply{}, err
	}
	p, _ := win.UTF16PtrFromString(path)
	stage = "precondition-open"
	h, err := win.CreateFile(p, win.GENERIC_READ|win.GENERIC_WRITE|win.READ_CONTROL, 0, nil, win.OPEN_EXISTING, win.FILE_FLAG_OVERLAPPED|win.SECURITY_SQOS_PRESENT|win.SECURITY_IMPERSONATION, 0)
	if err != nil {
		return PreconditionReply{}, err
	}
	defer win.CloseHandle(h)
	stage = "precondition-sid"
	sid, err := currentSID()
	if err != nil {
		return PreconditionReply{}, err
	}
	stage = "precondition-dacl"
	if err = checkPrivateObject(h, sid, false); err != nil {
		return PreconditionReply{}, err
	}
	stage = "precondition-server"
	process, err := verifyPipeServer(h, expectedPID, sid)
	if err != nil {
		return PreconditionReply{}, err
	}
	defer win.CloseHandle(process)
	stage = "precondition-encode"
	payload, err := json.Marshal(request)
	if err != nil {
		return PreconditionReply{}, ErrUnauthorized
	}
	defer clear(payload)
	stage = "precondition-hello"
	if err = writeExact(ctx, h, []byte{5}); err != nil {
		return PreconditionReply{}, err
	}
	stage = "precondition-request"
	if err = writeControlFrame(ctx, h, payload); err != nil {
		return PreconditionReply{}, err
	}
	stage = "precondition-response"
	raw, err := readControlFrame(ctx, h)
	if err != nil {
		return PreconditionReply{}, err
	}
	defer clear(raw)
	stage = "precondition-decode"
	var reply PreconditionReply
	if err = decodeControl(raw, &reply); err != nil {
		return PreconditionReply{}, ErrControlUnavailable
	}
	stage = "precondition-shape"
	if err = validControlReply(request, reply); err != nil {
		return PreconditionReply{}, err
	}
	stage = "precondition-ack"
	if err = writeExact(ctx, h, []byte{7}); err != nil {
		return PreconditionReply{}, err
	}
	stage = "precondition-result"
	switch reply.Code {
	case "ok":
		return reply, nil
	case "denied":
		return PreconditionReply{}, ErrUnauthorized
	default:
		return PreconditionReply{}, ErrControlUnavailable
	}
}

// String deliberately never renders credentials, even in accidental diagnostics.
func (r PreconditionRequest) String() string {
	return fmt.Sprintf("control precondition v%d", r.Version)
}
