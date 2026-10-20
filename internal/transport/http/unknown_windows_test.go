//go:build windows && amd64

package httptransport

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// Policy guard only: mixed cleanup errors are real lower-level producers, but
// this does not manufacture conflicting admissions through the serialized API.
func TestHTTPUncertaintyOutranksBaselineAndCandidateCleanup(t *testing.T) {
	for _, uncertain := range []error{workspace.ErrOpenUncertain, sqlite.ErrUnknown, sqlite.ErrFenced} {
		for _, cause := range []error{workspace.ErrBaseline, sqlite.ErrConflict, workspace.ErrStaleHandle, workspace.ErrPreview, workspace.ErrSaveInput, workspace.ErrRecordQuery} {
			w := httptest.NewRecorder()
			(&Handler{}).failure(w, errors.Join(cause, uncertain), "mixed-unknown-proof", "commitRecord", "00112233-4455-4677-8899-aabbccddeeff", false)
			var view protocol.ErrorView
			if w.Code != 503 || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.Code != protocol.ErrorCodeCOMMITUNKNOWN || view.Outcome != "unknown" || view.RetryPolicy != "investigate" || view.OperationId == nil {
				t.Fatal("joined uncertainty projected as rejected/newPreview")
			}
		}
	}
	for _, known := range []error{workspace.ErrBaseline, sqlite.ErrConflict} {
		w := httptest.NewRecorder()
		(&Handler{}).failure(w, known, "known-baseline-proof", "commitRecord", "", false)
		var view protocol.ErrorView
		if w.Code != 409 || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.Code != protocol.ErrorCodeSTALEBASELINE || view.Outcome != "rejected" || view.RetryPolicy != "newPreview" {
			t.Fatal("known conflict behavior changed")
		}
	}
}

// Policy projection only. The independent V013 test supplies the actual native
// installation COMMIT failure; a workspace route must not relabel its owner.
func TestHTTPSharedAuthorizationUnknownAlwaysInstallationScope(t *testing.T) {
	for _, route := range []string{"boundary", "listRecords", "commitRecord"} {
		w := httptest.NewRecorder()
		(&Handler{}).failure(w, errors.Join(installation.ErrAuthorizationUnknown, workspace.ErrLifecycle), "shared-auth-scope", route, "", false)
		var v protocol.ErrorView
		if w.Code != 503 || json.Unmarshal(w.Body.Bytes(), &v) != nil || v.Code != protocol.ErrorCodeINSTALLATIONSTATEUNKNOWN || v.Scope == nil || *v.Scope != "installation" || v.Outcome != "unknown" || v.RetryPolicy != "investigate" {
			t.Fatal("shared authorization uncertainty mislabeled workspace scope")
		}
	}
}

func TestHTTPColdOpenUncertaintyOutranksLifecycleCleanup(t *testing.T) {
	for _, uncertain := range []error{workspace.ErrOpenUncertain, sqlite.ErrUnknown, sqlite.ErrFenced} {
		w := httptest.NewRecorder()
		h := &Handler{}
		h.failure(w, errors.Join(uncertain, workspace.ErrLifecycle), "native-open-proof", "openWorkspace", "", false)
		var view protocol.ErrorView
		if w.Code != 503 || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.Code != protocol.ErrorCodeCOMMITUNKNOWN || view.Outcome != "unknown" || view.RetryPolicy != "investigate" {
			t.Fatal("physical unknown projected as ordinary closed/rejected")
		}
	}
	w := httptest.NewRecorder()
	(&Handler{}).failure(w, workspace.ErrLifecycle, "known-closed", "listRecords", "", false)
	var view protocol.ErrorView
	if w.Code != 409 || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.Code != protocol.ErrorCodeWORKSPACECLOSED {
		t.Fatal("known lifecycle-only refusal changed")
	}
}
