//go:build windows && amd64

package httptransport

import (
	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	"strconv"
	"time"
)

func revision(n int64) protocol.Revision { return protocol.Revision(strconv.FormatInt(n, 10)) }
func positive(n int64) protocol.PositiveRevision {
	return protocol.PositiveRevision(strconv.FormatInt(n, 10))
}
func recordView(r workspace.TextRecord) protocol.TextRecordView {
	return protocol.TextRecordView{WorkspaceId: protocol.UUID(r.WorkspaceID), ResourceId: protocol.UUID(r.ResourceID), Version: positive(r.Version), WorkspaceRevision: positive(r.Revision), Title: protocol.Title(r.Title), Body: protocol.Body(r.Body), ContentDigest: protocol.Digest(r.ContentDigest), OperationId: protocol.UUID(r.OperationID)}
}
func candidateView(p workspace.Preview) protocol.CandidateView {
	c := p.Candidate
	v := protocol.CandidateView{WorkspaceId: protocol.UUID(c.WorkspaceID), CandidateId: protocol.UUID(c.CandidateID), ResourceId: protocol.UUID(c.ResourceID), Kind: protocol.CandidateViewKind(c.Kind), BaseRevision: revision(c.BaseRevision), Title: protocol.Title(c.Title), Body: protocol.Body(c.Body), ContentDigest: protocol.Digest(c.ContentDigest), CandidateDigest: protocol.Digest(c.CandidateDigest), PreviewVersion: positive(c.PreviewVersion), ExpiresAt: c.ExpiresAt.UTC().Format(time.RFC3339Nano), Validation: protocol.CandidateValidation{State: "valid"}, Diff: protocol.Diff{AfterTitle: protocol.Title(c.Title), AfterBody: protocol.Body(c.Body)}}
	if c.BaseDigest != nil {
		d := protocol.Digest(*c.BaseDigest)
		v.BaseDigest = &d
	}
	if p.Before != nil {
		n, t, b := positive(p.Before.Version), protocol.Title(p.Before.Title), protocol.Body(p.Before.Body)
		v.Diff.BeforeVersion = &n
		v.Diff.BeforeTitle = &t
		v.Diff.BeforeBody = &b
	}
	return v
}
func receiptView(r *workspace.Receipt) *protocol.Receipt {
	if r == nil {
		return nil
	}
	return &protocol.Receipt{WorkspaceId: protocol.UUID(r.WorkspaceID), OperationId: protocol.UUID(r.OperationID), CandidateId: protocol.UUID(r.CandidateID), ResourceId: protocol.UUID(r.ResourceID), NewVersion: positive(r.NewVersion), NewRevision: positive(r.NewRevision), ContentDigest: protocol.Digest(r.ContentDigest)}
}
func workspaceView(scope workspace.Scope, s workspace.Status) protocol.WorkspaceView {
	v := protocol.WorkspaceView{WorkspaceId: protocol.UUID(scope.WorkspaceID), TenantId: protocol.UUID(scope.TenantID), LocationLabel: "registered workspace", BusinessState: "unavailable", DirectoryLifecycle: "registeredClosed", AccessMode: "none", Integrity: "unchecked", PersistenceHealth: protocol.PersistenceHealth{State: "unavailable", Scope: "workspace"}, AvailableCapabilities: []protocol.Capability{}}
	switch s.State {
	case workspace.LifecycleOpening:
		v.DirectoryLifecycle = "opening"
	case workspace.LifecycleOpen:
		v.DirectoryLifecycle = "open"
	case workspace.LifecycleDraining:
		v.DirectoryLifecycle = "draining"
	case workspace.LifecycleUnknown:
		v.DirectoryLifecycle = "rejected"
		v.PersistenceHealth.State = "unknown"
	}
	if s.NeedsInvestigation {
		v.PersistenceHealth.State = "unknown"
	}
	if s.Snapshot != nil && s.State == workspace.LifecycleOpen && !s.NeedsInvestigation {
		v.BusinessState = "ready"
		v.Integrity = "valid"
		v.AccessMode = "writable"
		v.PersistenceHealth.State = "known"
		n := revision(s.Snapshot.Revision)
		v.Revision = &n
	}
	if s.Epoch != "" {
		e := protocol.UUID(s.Epoch)
		v.OpenEpoch = &e
	}
	return v
}

// A persisted logical outcome does not prove physical pin/lease cleanup. Until
// the business owner supplies positive physical evidence this axis stays fenced.
func installationOperation(v installation.InstallationOperationView, corr protocol.CorrelationId) protocol.OperationView {
	result := protocol.OperationView{OperationId: protocol.UUID(v.OperationID), Scope: "installation", Kind: protocol.OperationViewKind(v.Kind), LogicalState: "unknown", PhysicalState: "reconciling", PersistenceState: "unknown", CorrelationId: corr}
	if v.WorkspaceID != "" {
		id := protocol.UUID(v.WorkspaceID)
		result.WorkspaceId = &id
	}
	switch string(v.Outcome) {
	case "committed":
		result.LogicalState = "committed"
		result.PersistenceState = "known"
	case "rejected":
		result.LogicalState = "rejected"
		result.PersistenceState = "known"
	case "accepted":
		result.LogicalState = "accepted"
	}
	return result
}
func closeView(v workspace.CloseView, corr protocol.CorrelationId) protocol.OperationView {
	id := protocol.UUID(v.Scope.WorkspaceID)
	result := protocol.OperationView{OperationId: protocol.UUID(v.OperationID), Scope: "workspace", WorkspaceId: &id, Kind: "close", LogicalState: "accepted", PhysicalState: "executing", PersistenceState: "unavailable", CorrelationId: corr}
	if v.Closed {
		result.LogicalState = "committed"
		result.PhysicalState = "drained"
	} else if v.State == workspace.LifecycleUnknown {
		result.LogicalState = "unknown"
		result.PhysicalState = "reconciling"
	}
	return result
}
func saveView(ws, key string, e workspace.OperationEvidence, corr protocol.CorrelationId) protocol.OperationView {
	id := protocol.UUID(ws)
	v := protocol.OperationView{OperationId: protocol.UUID(key), Scope: "workspace", WorkspaceId: &id, Kind: "save", LogicalState: "unknown", PhysicalState: "reconciling", PersistenceState: "unknown", CorrelationId: corr, Receipt: receiptView(e.Result.Receipt)}
	switch e.Result.Status {
	case workspace.SaveConfirmed:
		v.LogicalState = "committed"
		v.PersistenceState = "known"
	case workspace.SaveRejected, workspace.SaveNotCommitted:
		v.LogicalState = "rejected"
		v.PersistenceState = "known"
	case workspace.SaveAccepted:
		v.LogicalState = "accepted"
	}
	if e.PhysicalEnded {
		v.PhysicalState = "drained"
	}
	if e.Result.ViewDeliveryFailed {
		reason := protocol.ErrorCodeVIEWDELIVERYFAILED
		v.ReasonCode = &reason
	}
	return v
}
