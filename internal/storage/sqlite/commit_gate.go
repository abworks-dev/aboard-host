package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

var (
	ErrConflict = errors.New("candidate baseline, ownership or admission conflict")
	ErrKeyReuse = workspace.ErrSaveKeyReuse
	ErrFenced   = errors.New("workspace writes fenced pending investigation")
)

// Candidate is the workspace use-case value; storage does not own a second model.
type Candidate = workspace.Candidate
type SaveRequest = workspace.SaveRequest
type Receipt = workspace.Receipt
type PersistenceStatus = workspace.PersistenceStatus

const (
	Confirmed    = workspace.SaveConfirmed
	Accepted     = workspace.SaveAccepted
	Unknown      = workspace.SaveUnknown
	Rejected     = workspace.SaveRejected
	NotCommitted = workspace.SaveNotCommitted
)

type SaveResult = workspace.SaveResult

// Admission is a single-use capability returned ONLY by a confirmed new
// admission. A reopened/existing accepted operation cannot mint one or replay.
type Admission struct {
	gate    *CommitGate
	request SaveRequest
	input   string
	used    bool
}

// CommitGate serializes this adapter's calls, not the whole directory lifetime.
// The owner must hold the OS lease and reauthorize every call (and again before
// CommitAccepted). It must keep that lease while this gate is unknown/draining.
// Context must be the owner's operation context, NOT a disconnected HTTP waiter.
// No Host, grant cache, lifecycle epoch generator or retry loop is implemented.
type CommitGate struct {
	mu             sync.Mutex
	db             *Database
	epoch          string
	fenced, closed bool
}

func NewCommitGate(db *Database, epoch string) (*CommitGate, error) {
	if db == nil || db.kind != WorkspaceDatabase || epoch == "" {
		return nil, ErrConflict
	}
	var unresolved int
	if err := db.h.Conn.QueryRowContext(context.Background(), "SELECT count(*) FROM save_operation WHERE state IN ('accepted','unknown')").Scan(&unresolved); err != nil {
		return nil, errors.Join(ErrUnknown, err)
	}
	// Reopening cannot resurrect an in-memory admission capability. Retained
	// operations are investigation-only; a fresh result is not auto-unfencing.
	return &CommitGate{db: db, epoch: epoch, fenced: unresolved != 0}, nil
}
func candidateHashes(c Candidate) (string, string, error) {
	content, err := digest.Content(digest.ContentInput{WorkspaceID: protocol.UUID(c.WorkspaceID), ResourceID: protocol.UUID(c.ResourceID), Title: protocol.Title(c.Title), Body: protocol.Body(c.Body)})
	if err != nil {
		return "", "", err
	}
	var base *protocol.Digest
	if c.BaseDigest != nil {
		v := protocol.Digest(*c.BaseDigest)
		base = &v
	}
	candidate, err := digest.Candidate(digest.CandidateInput{WorkspaceID: protocol.UUID(c.WorkspaceID), PrincipalID: protocol.UUID(c.PrincipalID), CandidateID: protocol.UUID(c.CandidateID), ResourceID: protocol.UUID(c.ResourceID), Kind: protocol.PreviewRecordKind(c.Kind), BaseRevision: protocol.Revision(strconv.FormatInt(c.BaseRevision, 10)), BaseDigest: base, ContentDigest: content, PreviewVersion: protocol.PositiveRevision(strconv.FormatInt(c.PreviewVersion, 10))})
	return string(content), string(candidate), err
}
func (g *CommitGate) PutCandidate(ctx context.Context, c Candidate) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.fenced {
		return ErrFenced
	}
	content, candidate, err := candidateHashes(c)
	if err != nil {
		return err
	}
	if c.WorkspaceID != g.db.workspaceID || c.PreviewEpoch != g.epoch || c.ContentDigest != content || c.CandidateDigest != candidate {
		return ErrConflict
	}
	// Preview is non-formal; a failure is still conservatively fenced because
	// even this persistence result may be ambiguous. It never changes Head.
	err = insertCandidate(ctx, g.db.h.Conn, c)
	if err != nil {
		g.fenced = true
		return errors.Join(ErrUnknown, err)
	}
	return nil
}
func (g *CommitGate) input(r SaveRequest) (string, error) {
	if r.WorkspaceID != g.db.workspaceID || r.TenantID != g.db.tenantID {
		return "", ErrConflict
	}
	d, err := digest.Save(digest.SaveInput{InstallationID: protocol.UUID(r.InstallationID), TenantID: protocol.UUID(r.TenantID), PrincipalID: protocol.UUID(r.PrincipalID), WorkspaceID: protocol.UUID(r.WorkspaceID), OperationID: protocol.UUID(r.OperationID), CandidateID: protocol.UUID(r.CandidateID), CandidateDigest: protocol.Digest(r.CandidateDigest)})
	return string(d), err
}
func lookup(ctx context.Context, q schemaReader, r SaveRequest, input string) (SaveResult, bool, error) {
	var actor, candidate, stored, state string
	var failure sql.NullString
	err := q.QueryRowContext(ctx, "SELECT principal_id,candidate_id,input_digest,state,failure_code FROM save_operation WHERE workspace_id=? AND operation_id=?", r.WorkspaceID, r.OperationID).Scan(&actor, &candidate, &stored, &state, &failure)
	if errors.Is(err, sql.ErrNoRows) {
		return SaveResult{Status: Unknown}, false, nil
	}
	if err != nil {
		return SaveResult{Status: Unknown}, true, err
	}
	if actor != r.PrincipalID || candidate != r.CandidateID || stored != input {
		return SaveResult{Status: Rejected}, true, ErrKeyReuse
	}
	receipt := &Receipt{WorkspaceID: r.WorkspaceID, OperationID: r.OperationID, InputDigest: input, PrincipalID: actor, CandidateID: candidate}
	err = q.QueryRowContext(ctx, "SELECT resource_id,new_version,new_revision,content_digest FROM commit_receipt WHERE workspace_id=? AND operation_id=?", r.WorkspaceID, r.OperationID).Scan(&receipt.ResourceID, &receipt.NewVersion, &receipt.NewRevision, &receipt.ContentDigest)
	if err == nil {
		if state != "committed" {
			return SaveResult{Status: Unknown}, true, ErrSchema
		}
		return SaveResult{Status: Confirmed, Receipt: receipt}, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SaveResult{Status: Unknown}, true, err
	}
	if state == "rejected" {
		// A positive durable rejection record is evidence, not an inference from
		// receipt absence. The adapter never executes this key again.
		if !failure.Valid || failure.String == "" {
			return SaveResult{Status: Unknown}, true, ErrSchema
		}
		if failure.String == "NOT_COMMITTED" {
			return SaveResult{Status: NotCommitted, FailureCode: failure.String}, true, nil
		}
		return SaveResult{Status: Rejected, FailureCode: failure.String}, true, nil
	}
	// Missing receipt on any ordinary/current/old snapshot is NEVER rollback
	// evidence. An accepted/unknown record remains investigation-only.
	return SaveResult{Status: Unknown}, true, nil
}
func loadCandidate(ctx context.Context, q schemaReader, r SaveRequest) (Candidate, error) {
	c := Candidate{WorkspaceID: r.WorkspaceID, CandidateID: r.CandidateID}
	var expiry, state string
	var base sql.NullString
	err := q.QueryRowContext(ctx, `SELECT principal_id,resource_id,kind,base_revision,base_digest,title,body,content_digest,candidate_digest,preview_version,preview_epoch,expires_at,state FROM candidate WHERE workspace_id=? AND candidate_id=?`, r.WorkspaceID, r.CandidateID).Scan(&c.PrincipalID, &c.ResourceID, &c.Kind, &c.BaseRevision, &base, &c.Title, &c.Body, &c.ContentDigest, &c.CandidateDigest, &c.PreviewVersion, &c.PreviewEpoch, &expiry, &state)
	if err != nil {
		return c, err
	}
	if base.Valid {
		c.BaseDigest = &base.String
	}
	c.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiry)
	if err != nil || state != "previewed" || c.PrincipalID != r.PrincipalID || c.CandidateDigest != r.CandidateDigest {
		return c, ErrConflict
	}
	content, hash, err := candidateHashes(c)
	if err != nil || content != c.ContentDigest || hash != c.CandidateDigest {
		return c, ErrConflict
	}
	return c, nil
}
func baseline(ctx context.Context, q schemaReader, c Candidate) (int64, error) {
	var head int64
	if err := q.QueryRowContext(ctx, "SELECT revision FROM workspace_head WHERE workspace_id=?", c.WorkspaceID).Scan(&head); err != nil {
		return 0, err
	}
	if head != c.BaseRevision || head == math.MaxInt64 {
		return 0, ErrConflict
	}
	var version int64
	var content string
	err := q.QueryRowContext(ctx, `SELECT r.current_version,v.content_digest FROM text_record r JOIN text_version v ON v.workspace_id=r.workspace_id AND v.resource_id=r.resource_id AND v.version=r.current_version WHERE r.workspace_id=? AND r.resource_id=?`, c.WorkspaceID, c.ResourceID).Scan(&version, &content)
	if c.Kind == "create-text" {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, ErrConflict
		}
		return 1, nil
	}
	if err != nil || c.BaseDigest == nil || content != *c.BaseDigest || version == math.MaxInt64 {
		return 0, ErrConflict
	}
	return version + 1, nil
}
func (g *CommitGate) unknown(err error) (SaveResult, error) {
	g.fenced = true
	return SaveResult{Status: Unknown}, errors.Join(ErrUnknown, err)
}
func (g *CommitGate) abort(tx *sql.Tx, cause error) (SaveResult, error) {
	if err := tx.Rollback(); err != nil {
		return g.unknown(errors.Join(cause, err))
	}
	return SaveResult{Status: Rejected}, cause
}

// rejectAccepted records a proven rollback of THIS admitted business attempt.
// This is not reconciliation or a retry: only its still-live, single-use
// admission can reach here, and no formal business statement is re-executed.
func (g *CommitGate) rejectAccepted(ctx context.Context, tx *sql.Tx, a *Admission, code string, cause error) (SaveResult, error) {
	if err := tx.Rollback(); err != nil {
		return g.unknown(errors.Join(cause, err))
	}
	record, err := g.db.h.Conn.BeginTx(ctx, nil)
	if err != nil {
		return g.unknown(errors.Join(cause, err))
	}
	r := a.request
	updated, err := record.ExecContext(ctx, `UPDATE save_operation SET state='rejected',failure_code=? WHERE workspace_id=? AND operation_id=? AND principal_id=? AND candidate_id=? AND input_digest=? AND state='accepted' AND NOT EXISTS(SELECT 1 FROM commit_receipt WHERE workspace_id=? AND operation_id=?)`, code, r.WorkspaceID, r.OperationID, r.PrincipalID, r.CandidateID, a.input, r.WorkspaceID, r.OperationID)
	if err == nil {
		var count int64
		count, err = updated.RowsAffected()
		if err == nil && count != 1 {
			err = ErrConflict
		}
	}
	if err != nil {
		return g.unknown(errors.Join(cause, err, record.Rollback()))
	}
	if err := record.Commit(); err != nil {
		return g.unknown(errors.Join(cause, err))
	}
	return SaveResult{Status: Rejected, FailureCode: code}, cause
}

func (g *CommitGate) Admit(ctx context.Context, r SaveRequest, now time.Time) (SaveResult, *Admission, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	input, err := g.input(r)
	if err != nil {
		return SaveResult{Status: Rejected}, nil, err
	}
	if g.closed {
		return SaveResult{Status: Unknown}, nil, ErrFenced
	}
	tx, err := g.db.h.Conn.BeginTx(ctx, nil)
	if err != nil {
		result, e := g.unknown(err)
		return result, nil, e
	}
	// Existing key investigation comes BEFORE epoch/expiry/applied/base and even
	// the new-write fence. The owner has already checked CURRENT authorization.
	result, found, err := lookup(ctx, tx, r, input)
	if found || err != nil {
		end := tx.Rollback()
		if err != nil || end != nil {
			if errors.Is(err, ErrKeyReuse) && end == nil {
				return SaveResult{Status: Rejected}, nil, err
			}
			result, e := g.unknown(errors.Join(err, end))
			return result, nil, e
		}
		if result.Status == Unknown {
			g.fenced = true
		}
		return result, nil, nil
	}
	if g.fenced {
		_ = tx.Rollback()
		return SaveResult{Status: Unknown}, nil, ErrFenced
	}
	if r.Epoch != g.epoch {
		result, err := g.abort(tx, workspace.ErrStaleHandle)
		return result, nil, err
	}
	c, err := loadCandidate(ctx, tx, r)
	if err == nil && c.PreviewEpoch != g.epoch {
		err = workspace.ErrStaleHandle
	}
	if err == nil && (r.PreviewVersion != c.PreviewVersion || !now.Before(c.ExpiresAt)) {
		err = ErrConflict
	}
	if err == nil {
		_, err = baseline(ctx, tx, c)
	}
	if err != nil {
		result, e := g.abort(tx, err)
		return result, nil, e
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO save_operation VALUES(?,?,?,?,?,?,'accepted',NULL)", r.WorkspaceID, r.OperationID, r.PrincipalID, r.CandidateID, input, r.Epoch)
	if err != nil {
		result, e := g.abort(tx, err)
		return result, nil, e
	}
	if err := tx.Commit(); err != nil {
		result, e := g.unknown(err)
		return result, nil, e
	}
	return SaveResult{Status: Accepted}, &Admission{gate: g, request: r, input: input}, nil
}

func (g *CommitGate) CommitAccepted(ctx context.Context, a *Admission) (SaveResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if a == nil || a.gate != g {
		return SaveResult{Status: Rejected}, ErrConflict
	}
	if g.closed {
		return SaveResult{Status: Unknown}, ErrFenced
	}
	tx, err := g.db.h.Conn.BeginTx(ctx, nil)
	if err != nil {
		return g.unknown(err)
	}
	result, found, err := lookup(ctx, tx, a.request, a.input)
	if err != nil {
		return g.unknown(errors.Join(err, tx.Rollback()))
	}
	if found && (result.Status == Confirmed || result.Status == Rejected) {
		end := tx.Rollback()
		if end != nil {
			return g.unknown(end)
		}
		return result, nil
	}
	if !found || a.used || g.fenced {
		g.fenced = true
		_ = tx.Rollback()
		return SaveResult{Status: Unknown}, ErrFenced
	}
	a.used = true // never mint another business attempt for this admission
	c, err := loadCandidate(ctx, tx, a.request)
	if err != nil {
		return g.rejectAccepted(ctx, tx, a, "INVALID_CANDIDATE", err)
	}
	version, err := baseline(ctx, tx, c)
	if err != nil {
		return g.rejectAccepted(ctx, tx, a, "STALE_BASELINE", err)
	}
	receipt := &Receipt{WorkspaceID: c.WorkspaceID, OperationID: a.request.OperationID, InputDigest: a.input, PrincipalID: c.PrincipalID, CandidateID: c.CandidateID, ResourceID: c.ResourceID, NewVersion: version, NewRevision: c.BaseRevision + 1, ContentDigest: c.ContentDigest}
	statements := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO text_version VALUES(?,?,?,?,?,?,?,?,?)", []any{c.WorkspaceID, c.ResourceID, version, receipt.NewRevision, c.Title, c.Body, c.ContentDigest, a.request.OperationID, c.PrincipalID}},
		{"INSERT INTO text_record VALUES(?,?,?,?) ON CONFLICT(workspace_id,resource_id) DO UPDATE SET current_version=excluded.current_version", []any{c.WorkspaceID, g.db.tenantID, c.ResourceID, version}},
		{"UPDATE workspace_head SET revision=? WHERE workspace_id=?", []any{receipt.NewRevision, c.WorkspaceID}},
		{"INSERT INTO commit_receipt VALUES(?,?,?,?,?,?,?,?,?)", []any{c.WorkspaceID, a.request.OperationID, a.input, c.PrincipalID, c.CandidateID, c.ResourceID, version, receipt.NewRevision, c.ContentDigest}},
		{"UPDATE save_operation SET state='committed',failure_code=NULL WHERE workspace_id=? AND operation_id=?", []any{c.WorkspaceID, a.request.OperationID}},
		{"UPDATE candidate SET state='applied' WHERE workspace_id=? AND candidate_id=?", []any{c.WorkspaceID, c.CandidateID}},
		{"INSERT INTO audit_entry VALUES(?,?,?,?,?,?,?)", []any{c.WorkspaceID, receipt.NewRevision, c.PrincipalID, a.request.OperationID, "save", "committed", receipt.NewRevision}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			return g.rejectAccepted(ctx, tx, a, "WRITE_FAILED", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return g.unknown(err)
	}
	return SaveResult{Status: Confirmed, Receipt: receipt}, nil
}

// Investigate only reads and never starts a business submission. A positive
// receipt can be observed in a trustworthy existing view; absence stays unknown.
func (g *CommitGate) Investigate(ctx context.Context, r SaveRequest) (SaveResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	input, err := g.input(r)
	if err != nil {
		return SaveResult{Status: Rejected}, err
	}
	if g.closed {
		return SaveResult{Status: Unknown}, ErrFenced
	}
	tx, err := g.db.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return g.unknown(err)
	}
	result, found, err := lookup(ctx, tx, r, input)
	end := tx.Rollback()
	if err != nil || end != nil {
		if errors.Is(err, ErrKeyReuse) && end == nil {
			return SaveResult{Status: Rejected}, err
		}
		return g.unknown(errors.Join(err, end))
	}
	if found && result.Status == Unknown {
		g.fenced = true
	}
	return result, nil
}

// EndPhysical waits for all adapter calls (including canceled driver work) and
// closes the physical connection/pool. It does not release the caller's OS lease
// and DOES NOT by itself prove committed, rolled back, or safe to move.
func (g *CommitGate) EndPhysical() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.fenced = true
	if err := g.db.Close(); err != nil {
		return errors.Join(ErrUnknown, err)
	}
	g.closed = true
	return nil
}

// InvestigateFresh requires EndPhysical and a mandatory owner callback proving
// exclusion, unchanged physical identity and SAME UNREPLACED history. Callback
// must retain that lease through this call. Storage cannot infer these facts
// from a path, UUID, PID, context timeout, or database/sql transaction lock.
// It opens a fresh checked view, verifies all formal history, and never reseeds
// or replays. NotCommitted is evidence only, never permission to reuse this key.
func (g *CommitGate) InvestigateFresh(ctx context.Context, r SaveRequest, verifyExclusiveHistory func() error) (SaveResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closed || verifyExclusiveHistory == nil {
		return SaveResult{Status: Unknown}, ErrFenced
	}
	if err := verifyExclusiveHistory(); err != nil {
		return SaveResult{Status: Unknown}, err
	}
	input, err := g.input(r)
	if err != nil {
		return SaveResult{Status: Rejected}, err
	}
	d, err := OpenDatabase(ctx, g.db.path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
	if err != nil {
		return SaveResult{Status: Unknown}, errors.Join(ErrUnknown, err)
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SaveResult{Status: Unknown}, errors.Join(ErrUnknown, err, d.Close())
	}
	err = verifySchema(ctx, tx, WorkspaceDatabase)
	result := SaveResult{Status: Unknown}
	var found bool
	if err == nil {
		result, found, err = lookup(ctx, tx, r, input)
	}
	if err == nil && found && result.Status == Unknown {
		result.Status = NotCommitted
	}
	// No admission record is not enough to attribute a missing result to this
	// history. Admission-unknown remains unknown rather than invented rejection.
	err = errors.Join(err, tx.Rollback(), d.Close())
	if err != nil {
		return SaveResult{Status: Unknown}, errors.Join(ErrUnknown, err)
	}
	return result, nil
}
