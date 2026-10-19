package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// Shared fixed insert, not arbitrary SQL or a second candidate persistence path.
func insertCandidate(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, c Candidate) error {
	_, err := q.ExecContext(ctx, `INSERT INTO candidate(workspace_id,candidate_id,principal_id,resource_id,kind,base_revision,base_digest,title,body,content_digest,candidate_digest,preview_version,preview_epoch,expires_at,state) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'previewed')`, c.WorkspaceID, c.CandidateID, c.PrincipalID, c.ResourceID, c.Kind, c.BaseRevision, c.BaseDigest, c.Title, c.Body, c.ContentDigest, c.CandidateDigest, c.PreviewVersion, c.PreviewEpoch, c.ExpiresAt.UTC().Format(time.RFC3339Nano))
	return err
}

// PutPreview shares the same gate/connection as formal facts/commits. Unlike the
// lower-level fixture primitive PutCandidate, the business preview revalidates
// the formal baseline inside the same transaction as the candidate INSERT.
func (f *WorkspaceFacts) PutPreview(ctx context.Context, scope workspace.Scope, c workspace.Candidate) error {
	if f == nil || scope != f.scope || c.WorkspaceID != scope.WorkspaceID {
		return workspace.ErrScope
	}
	g := f.gate
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.fenced {
		return ErrFenced
	}
	if g.db.workspaceID != scope.WorkspaceID || g.db.tenantID != scope.TenantID {
		return workspace.ErrScope
	}
	content, hash, err := candidateHashes(c)
	if err != nil || content != c.ContentDigest || hash != c.CandidateDigest || c.PreviewEpoch != g.epoch || c.PreviewVersion != 1 || !time.Now().Before(c.ExpiresAt) {
		return ErrConflict
	}
	fence := func(err error) error {
		g.fenced = true
		return errors.Join(workspace.ErrOpenUncertain, ErrUnknown, err)
	}
	tx, err := g.db.h.Conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = baseline(ctx, tx, c); err != nil {
		if end := tx.Rollback(); end != nil {
			return fence(errors.Join(err, end))
		}
		if errors.Is(err, ErrConflict) {
			return workspace.ErrBaseline
		}
		return err
	}
	if err = insertCandidate(ctx, tx, c); err != nil {
		if end := tx.Rollback(); end != nil {
			return fence(errors.Join(err, end))
		}
		return err
	}
	if err = tx.Commit(); err != nil {
		return fence(err)
	}
	return nil
}

var _ workspace.PreviewFacts = (*WorkspaceFacts)(nil)
