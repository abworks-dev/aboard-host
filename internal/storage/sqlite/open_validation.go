package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

// ValidateOpenHistory only validates an existing, caller-owned workspace view.
// The caller owns authorization, registration, physical identity, exclusion and
// any explicitly authorized SQLite recovery. This method never repairs or replays
// business work. pending means retained accepted/unknown operations require an
// investigation fence, NOT rollback evidence or permission to submit them again.
// Save input digests are cross-checked relationally; their installation ID is not
// stored in this database, so this method cannot independently recompute them.
func (d *Database) ValidateOpenHistory(ctx context.Context, descriptor workspace.DirectoryDescriptor) (pending bool, err error) {
	if d == nil || d.h == nil || d.h.Conn == nil || d.kind != WorkspaceDatabase {
		return false, ErrSchema
	}
	if err = d.ValidateDirectoryDescriptor(ctx, descriptor); err != nil {
		return false, err
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, errors.Join(ErrSchema, err)
	}
	defer func() {
		err = errors.Join(err, tx.Rollback())
		if err != nil {
			pending = false
		}
	}()
	// Recheck in the same snapshot as the additional business invariants. The
	// existing validator verifies DDL, ledger, FKs and EVERY immutable content hash.
	if err = verifySchema(ctx, tx, WorkspaceDatabase); err != nil {
		return false, err
	}
	var w, tenant, identity, state string
	var format int
	if err = tx.QueryRowContext(ctx, `SELECT workspace_id,tenant_id,identity_digest,format_version,business_state FROM workspace_identity WHERE singleton=1`).Scan(&w, &tenant, &identity, &format, &state); err != nil {
		return false, errors.Join(ErrSchema, err)
	}
	if w != descriptor.WorkspaceID || tenant != descriptor.TenantID || identity != descriptor.IdentityDigest || format != descriptor.FormatVersion || state != "ready" {
		return false, schemaError("open identity or business readiness mismatch")
	}
	for _, query := range []string{
		// Explicit joins avoid NULL/empty-subquery comparisons silently passing.
		`SELECT count(*) FROM text_record r LEFT JOIN text_version v ON v.workspace_id=r.workspace_id AND v.resource_id=r.resource_id AND v.version=r.current_version WHERE v.version IS NULL`,
		`SELECT count(*) FROM commit_receipt r JOIN candidate c USING(workspace_id,candidate_id) JOIN text_version v ON v.workspace_id=r.workspace_id AND v.operation_id=r.operation_id WHERE c.principal_id!=r.principal_id OR c.title!=v.title OR c.body!=v.body OR c.base_revision!=r.new_revision-1 OR (r.new_version=1 AND c.kind!='create-text') OR (r.new_version>1 AND c.kind!='update-text')`,
		`SELECT count(*) FROM commit_receipt r JOIN candidate c USING(workspace_id,candidate_id) LEFT JOIN text_version p ON p.workspace_id=r.workspace_id AND p.resource_id=r.resource_id AND p.version=r.new_version-1 WHERE r.new_version>1 AND (p.version IS NULL OR p.content_digest!=c.base_digest OR p.workspace_revision>=r.new_revision)`,
		`SELECT count(*) FROM candidate c WHERE c.state='applied' AND (SELECT count(*) FROM commit_receipt r WHERE r.workspace_id=c.workspace_id AND r.candidate_id=c.candidate_id)!=1`,
		`SELECT count(*) FROM save_operation o JOIN candidate c USING(workspace_id,candidate_id) WHERE o.admission_epoch='' OR o.admission_epoch!=c.preview_epoch OR (o.state='rejected' AND (o.failure_code IS NULL OR o.failure_code='')) OR (o.state!='rejected' AND o.failure_code IS NOT NULL)`,
		`SELECT count(*) FROM audit_entry a JOIN save_operation o USING(workspace_id,operation_id) LEFT JOIN commit_receipt r USING(workspace_id,operation_id) WHERE (a.outcome_code='committed' AND (r.operation_id IS NULL OR a.action!='save' OR a.principal_id!=r.principal_id OR a.revision!=r.new_revision OR a.sequence!=r.new_revision)) OR (r.operation_id IS NOT NULL AND (a.action!='save' OR a.outcome_code!='committed' OR a.principal_id!=r.principal_id OR a.revision!=r.new_revision))`,
	} {
		var bad int64
		if err = tx.QueryRowContext(ctx, query).Scan(&bad); err != nil || bad != 0 {
			return false, errors.Join(schemaError("contradictory open history"), err)
		}
	}
	// Only applied candidates are formal evidence. Invalid/expired unconfirmed
	// previews do not themselves corrupt an otherwise valid formal workspace.
	rows, err := tx.QueryContext(ctx, `SELECT workspace_id,candidate_id,principal_id,resource_id,kind,base_revision,base_digest,title,body,content_digest,candidate_digest,preview_version FROM candidate WHERE state='applied'`)
	if err != nil {
		return false, errors.Join(ErrSchema, err)
	}
	for rows.Next() {
		var c Candidate
		var base sql.NullString
		if err = rows.Scan(&c.WorkspaceID, &c.CandidateID, &c.PrincipalID, &c.ResourceID, &c.Kind, &c.BaseRevision, &base, &c.Title, &c.Body, &c.ContentDigest, &c.CandidateDigest, &c.PreviewVersion); err != nil {
			break
		}
		if base.Valid {
			c.BaseDigest = &base.String
		}
		content, candidate, hashErr := candidateHashes(c)
		if hashErr != nil || content != c.ContentDigest || candidate != c.CandidateDigest {
			err = schemaError("applied candidate digest mismatch")
			break
		}
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return false, errors.Join(ErrSchema, err)
	}
	var unresolved int64
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM save_operation WHERE state IN ('accepted','unknown')`).Scan(&unresolved); err != nil {
		return false, errors.Join(ErrSchema, err)
	}
	return unresolved != 0, nil
}
