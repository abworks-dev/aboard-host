//go:build windows && amd64

package security

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/protocol"
	win "golang.org/x/sys/windows"
)

func controlTestRequest() PreconditionRequest {
	return PreconditionRequest{Version: 1, Kind: "registration", Bearer: strings.Repeat("a", 43), WorkspaceID: "10000000-0000-4000-8000-000000000001", IntendedUse: "open"}
}
func controlTestReply(r PreconditionRequest) PreconditionReply {
	return PreconditionReply{Version: 1, Code: "ok", Kind: r.Kind, InstallationID: "10000000-0000-4000-8000-000000000002", TenantID: "10000000-0000-4000-8000-000000000003", WorkspaceID: protocol.UUID(r.WorkspaceID), RegistrationVersion: "1"}
}
func TestControlPreconditionStrictFrames(t *testing.T) {
	r := controlTestRequest()
	good, _ := json.Marshal(r)
	var decoded PreconditionRequest
	if err := decodeControl(good, &decoded); err != nil || validControlRequest(decoded) != nil {
		t.Fatal("valid private request refused", err)
	}
	cases := map[string]string{
		"case-alias":         strings.Replace(string(good), `"version"`, `"Version"`, 1),
		"semantic-duplicate": strings.TrimSuffix(string(good), "}") + `,"Kind":"registration"}`,
		"duplicate-key":      strings.TrimSuffix(string(good), "}") + `,"kind":"registration"}`,
		"null-forbidden":     strings.TrimSuffix(string(good), "}") + `,"operationId":null}`,
		"empty-forbidden":    strings.TrimSuffix(string(good), "}") + `,"operationId":""}`,
		"null-required":      strings.Replace(string(good), `"open"`, `null`, 1),
		"null-version":       strings.Replace(string(good), `"version":1`, `"version":null`, 1),
		"exponent-version":   strings.Replace(string(good), `"version":1`, `"version":1e0`, 1),
		"unknown-null":       strings.TrimSuffix(string(good), "}") + `,"unknown":null}`,
		"missing-field":      `{"version":1,"kind":"registration","bearer":"` + r.Bearer + `","workspaceId":"` + r.WorkspaceID + `"}`,
		"invalid-utf8":       string(append(append([]byte{}, good...), 0xff)),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			var v PreconditionRequest
			if err := decodeControl([]byte(raw), &v); err == nil {
				t.Fatal("non-strict private request accepted")
			}
		})
	}
	reply := controlTestReply(r)
	raw, _ := json.Marshal(reply)
	badReplies := map[string]string{
		"case-alias":         strings.Replace(string(raw), `"workspaceId"`, `"WorkspaceId"`, 1),
		"semantic-duplicate": strings.TrimSuffix(string(raw), "}") + `,"Kind":"registration"}`,
		"null-forbidden":     strings.TrimSuffix(string(raw), "}") + `,"expectedPhase":null}`,
		"error-with-data":    `{"version":1,"code":"denied","kind":"registration"}`,
		"null-error":         `{"version":1,"code":null}`,
	}
	for name, b := range badReplies {
		t.Run("reply-"+name, func(t *testing.T) {
			var v PreconditionReply
			if err := decodeControl([]byte(b), &v); err == nil {
				t.Fatal("non-strict private reply accepted")
			}
		})
	}
	for _, bad := range []PreconditionRequest{{Version: 2}, {Version: 1, Kind: "SQL", Bearer: r.Bearer}} {
		if validControlRequest(bad) == nil {
			t.Fatal("private protocol gained unsupported operation")
		}
	}
	if err := decodeControl(make([]byte, controlFrameLimit+1), &decoded); err == nil {
		t.Fatal("unbounded frame accepted")
	}
}

type controlReaderFunc func(context.Context, PreconditionRequest) (PreconditionReply, func(context.Context) error, error)

func (f controlReaderFunc) ReadControlPrecondition(ctx context.Context, r PreconditionRequest) (PreconditionReply, func(context.Context) error, error) {
	return f(ctx, r)
}
func TestControlPreconditionNativeLateAuthorityAndServerIdentity(t *testing.T) {
	for _, wrongPID := range []bool{false, true} {
		t.Run(map[bool]string{false: "late-denial", true: "wrong-live-pid"}[wrongPID], func(t *testing.T) {
			name := pipeName(t)
			pipe, err := NewControlPipe(name)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			called := false
			checked := false
			reader := controlReaderFunc(func(_ context.Context, r PreconditionRequest) (PreconditionReply, func(context.Context) error, error) {
				called = true
				return controlTestReply(r), func(context.Context) error { checked = true; return ErrUnauthorized }, nil
			})
			done := make(chan error, 1)
			go func() { done <- pipe.ServeControl(ctx, NewSessions(), Grant{}, time.Minute, reader) }()
			pid := ProcessID()
			if wrongPID {
				pid++
			}
			reply, clientErr := ReceivePrecondition(ctx, name, pid, controlTestRequest())
			serverErr := <-done
			if err = pipe.Close(); err != nil {
				t.Error(err)
			}
			if !errors.Is(clientErr, ErrUnauthorized) || reply.InstallationID != "" {
				t.Fatal("private response bypassed identity/current authority", clientErr)
			}
			if wrongPID {
				if called || checked || serverErr == nil {
					t.Fatal("unverified server received bearer request")
				}
			} else if !called || !checked || serverErr != nil {
				t.Fatal("late current authorization not checked", serverErr)
			}
		})
	}
}
func TestControlPreconditionNativeCanceledPartialFrameDrains(t *testing.T) {
	name := pipeName(t)
	pipe, err := NewControlPipe(name)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := false
	reader := controlReaderFunc(func(context.Context, PreconditionRequest) (PreconditionReply, func(context.Context) error, error) {
		called = true
		return PreconditionReply{}, nil, ErrUnauthorized
	})
	done := make(chan error, 1)
	go func() { done <- pipe.ServeControl(ctx, NewSessions(), Grant{}, time.Minute, reader) }()
	path, _ := localPipeName(name)
	p, _ := win.UTF16PtrFromString(path)
	h, err := win.CreateFile(p, win.GENERIC_READ|win.GENERIC_WRITE, 0, nil, win.OPEN_EXISTING, win.FILE_FLAG_OVERLAPPED|win.SECURITY_SQOS_PRESENT|win.SECURITY_IMPERSONATION, 0)
	if err != nil {
		cancel()
		<-done
		pipe.Close()
		t.Fatal(err)
	}
	ioctx, end := context.WithTimeout(context.Background(), 3*time.Second)
	defer end()
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], 100)
	if err = writeExact(ioctx, h, []byte{5}); err == nil {
		err = writeExact(ioctx, h, header[:])
	}
	cancel()
	serverErr := <-done
	clientClose := win.CloseHandle(h)
	pipeClose := pipe.Close()
	if err != nil || !errors.Is(serverErr, context.Canceled) || called || clientClose != nil || pipeClose != nil {
		t.Fatal("partial frame cancellation did not physically drain", err, serverErr, called, clientClose, pipeClose)
	}
	replacement, err := NewControlPipe(name)
	if err != nil {
		t.Fatal("canceled reader retained the actual first instance", err)
	}
	if err = replacement.Close(); err != nil {
		t.Fatal(err)
	}
}
