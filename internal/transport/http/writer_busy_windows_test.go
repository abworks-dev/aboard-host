//go:build windows && amd64

package httptransport

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	win "golang.org/x/sys/windows"
)

// Classification policy only. The external V012 matrix supplies genuine native
// contention; this table must not turn arbitrary path/ACL failures into busy or
// let a joined persistence/physical unknown masquerade as a known rejection.
func TestHTTPNativeWriterContentionClassification(t *testing.T) {
	cases := []struct {
		err    error
		code   protocol.ErrorCode
		status int
	}{
		{win.ERROR_SHARING_VIOLATION, protocol.ErrorCodeWRITERBUSY, 409},
		{win.ERROR_LOCK_VIOLATION, protocol.ErrorCodeWRITERBUSY, 409},
		{win.STATUS_SHARING_VIOLATION, protocol.ErrorCodeWRITERBUSY, 409},
		{errors.Join(win.STATUS_SHARING_VIOLATION, workspace.ErrOpenUncertain), protocol.ErrorCodeCOMMITUNKNOWN, 503},
		{win.ERROR_ACCESS_DENIED, protocol.ErrorCodeRECOVERYREQUIRED, 409},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		(&Handler{}).failure(w, c.err, "writer-busy-policy", "openWorkspace", "", false)
		var view protocol.ErrorView
		if w.Code != c.status || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.Code != c.code {
			t.Fatal("native contention classification changed", view.Code)
		}
	}
}
