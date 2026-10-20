//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"errors"
	"os"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// These false fields are emitted explicitly: readiness is NOT callback/process end.
type saveNativeEvidence struct {
	JournalCalls, JournalSuccesses, DatabaseCalls, NativeBOOL int
	CommitCalls                                               int
	InFlight, CommitReturned, CallbackDrained, PhysicalEnded  bool
}
type saveNativeMidpoint struct {
	root                    string
	marker                  saveCheckpoint
	observation             *sqlite.AcceptanceCommitObservation
	evidence                saveNativeEvidence
	disqualified, published bool
	failure                 error
}

// Called ONLY under finalizeFlushState.mu AFTER the original native call. Does
// no gate/owner/Store/SQL work. Diagnostic failure never changes that native BOOL.
func (m *saveNativeMidpoint) afterSync(path, database string, nativeBOOL int) {
	completed := false
	defer func() {
		if !completed {
			_ = recover()
			m.failure = errors.Join(m.failure, errSaveProbe)
			m.disqualified = true
		}
	}()
	snapshot := m.observation.Snapshot()
	if !snapshot.InFlight || snapshot.Returned || snapshot.Calls != 1 {
		m.disqualified = true
	}
	if path == database {
		m.evidence.DatabaseCalls++
		m.disqualified = true
	} else {
		m.evidence.JournalCalls++
		if nativeBOOL != 0 {
			m.evidence.JournalSuccesses++
		} else {
			m.disqualified = true
		}
	}
	if !m.disqualified && !m.published && m.evidence.JournalCalls == 2 && m.evidence.JournalSuccesses == 2 && m.evidence.DatabaseCalls == 0 {
		m.evidence.NativeBOOL = nativeBOOL
		m.evidence.CommitCalls, m.evidence.InFlight, m.evidence.CommitReturned = snapshot.Calls, snapshot.InFlight, snapshot.Returned
		m.marker.Native = &m.evidence
		m.published = true
		m.failure = publishSaveCheckpoint(m.root, m.marker)
	}
	completed = true
}

func runSaveMidpointProbe(r *openResources, request workspace.SaveRequest, admission *sqlite.Admission, cfg saveProbe, apply func() (workspace.SaveResult, error)) (result workspace.SaveResult, err error) {
	result.Status = workspace.SaveUnknown
	if e := saveProbeQuiet(r, request); e != nil {
		return result, e
	}
	if e := installFinalizeDelegate(); e != nil {
		return result, e
	}
	return r.gate.AcceptanceObserveCommit(admission, request, func(observation *sqlite.AcceptanceCommitObservation) (workspace.SaveResult, error) {
		identity := cfg.Fixture + "|" + cfg.WorkspaceID + "|" + cfg.OperationID + "|" + cfg.Phase
		if _, used := consumedSaveProbes.LoadOrStore(identity, struct{}{}); used {
			return result, errSaveProbe
		}
		midpoint := &saveNativeMidpoint{root: cfg.Fixture, observation: observation, marker: saveCheckpoint{
			Version: 1, PID: os.Getpid(), InstallationID: request.InstallationID, TenantID: request.TenantID, WorkspaceID: request.WorkspaceID, PrincipalID: request.PrincipalID, OperationID: request.OperationID, CandidateID: request.CandidateID, CandidateDigest: request.CandidateDigest, Epoch: request.Epoch, Phase: cfg.Phase, Status: "accepted",
		}}
		state := &finalizeFlushState{active: true, databasePath: r.databasePin.Path(), midpoint: midpoint}
		if !armedFinalize.CompareAndSwap(nil, state) {
			return result, errSaveProbe
		}
		original, closeErr := callFinalizeProbe(state, func() error { var e error; result, e = apply(); return e }, func() error { return nil })
		// CAS-disarm + mu have joined the callback before reading its failure/owners.
		state.mu.Lock()
		diagnostic := midpoint.failure
		state.mu.Unlock()
		var owned *finalizeCloseError
		if errors.As(diagnostic, &owned) {
			diagnostic = &finalizeCloseError{cause: diagnostic, owner: state}
		} // retain the COMPLETE graph; no close retry
		combined := errors.Join(original, closeErr, diagnostic)
		retainSaveProbeFailure(r, combined)
		if result.Status != workspace.SaveConfirmed && (original != nil || diagnostic != nil || closeErr != nil) {
			result = workspace.SaveResult{Status: workspace.SaveUnknown}
			combined = errors.Join(combined, sqlite.ErrUnknown)
		}
		return result, combined
	})
}
