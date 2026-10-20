//go:build windows && amd64 && aboard_acceptance

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"time"
)

// This private live read-only connection exists only for a bounded busy fixture.
// It is not Open(ReadOnlyScratch), an advertised open mode or a second writer.
type acceptanceBusyReader struct {
	pool        *sql.DB
	conn        *sql.Conn
	tx          *sql.Tx
	attempted   bool
	closeErr    error
	readOnlyURI bool
	queryOnly   int
}

func (r *acceptanceBusyReader) open(ctx context.Context, path string, workspace string) (head int64, err error) {
	dsn, err := fixedURI(path, ReadWriteExisting)
	if err != nil {
		return 0, err
	}
	uri, err := url.Parse(dsn)
	if err != nil {
		return 0, err
	}
	q := uri.Query()
	q.Set("mode", "ro")
	uri.RawQuery = q.Encode()
	r.readOnlyURI = q.Get("mode") == "ro" && q.Get("vfs") == "win32"
	r.pool, err = sql.Open("sqlite", uri.String())
	if err != nil {
		return 0, err
	}
	r.pool.SetMaxOpenConns(1)
	r.pool.SetMaxIdleConns(1)
	r.conn, err = r.pool.Conn(ctx)
	if err != nil {
		return 0, err
	}
	// Only fixed connection settings and SELECTs. Never a business write.
	for _, s := range []string{"PRAGMA query_only=ON", "PRAGMA synchronous=EXTRA", "PRAGMA foreign_keys=ON", "PRAGMA trusted_schema=OFF", "PRAGMA mmap_size=0", "PRAGMA busy_timeout=1000"} {
		if _, err = r.conn.ExecContext(ctx, s); err != nil {
			return 0, err
		}
	}
	var version, journal string
	var queryOnly int
	if err = r.conn.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		return 0, err
	}
	if err = r.conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
		return 0, err
	}
	if err = r.conn.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly); err != nil {
		return 0, err
	}
	if version != EngineVersion || journal != "delete" || queryOnly != 1 {
		return 0, ErrProfile
	}
	r.queryOnly = queryOnly
	for _, s := range []struct {
		sql  string
		want int
	}{{"PRAGMA synchronous", 3}, {"PRAGMA foreign_keys", 1}, {"PRAGMA busy_timeout", 1000}, {"PRAGMA trusted_schema", 0}, {"PRAGMA mmap_size", 0}} {
		var got int
		if err = r.conn.QueryRowContext(ctx, s.sql).Scan(&got); err != nil {
			return 0, err
		}
		if got != s.want {
			return 0, ErrProfile
		}
	}
	// Independent lifetime: cancellation of a request must not silently release
	// SHARED while the original driver call is still running.
	r.tx, err = r.conn.BeginTx(context.WithoutCancel(ctx), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, err
	}
	err = r.tx.QueryRowContext(ctx, "SELECT revision FROM workspace_head WHERE workspace_id=?", workspace).Scan(&head)
	return
}
func (r *acceptanceBusyReader) close() (err error) {
	if r.attempted {
		return retainCloseOwner(r.closeErr, r)
	}
	r.attempted = true
	returned := false
	defer func() {
		recovered := recover()
		if !returned {
			_ = recovered
			r.closeErr = physicalCloseResult(errAcceptanceBusy)
		}
		err = retainCloseOwner(r.closeErr, r)
	}()
	// First uncertain child end is sticky; do not release its ancestors.
	if r.tx != nil {
		if e := r.tx.Rollback(); e != nil {
			r.closeErr = physicalCloseResult(e)
			returned = true
			return
		}
	}
	if r.conn != nil {
		if e := r.conn.Close(); e != nil {
			r.closeErr = physicalCloseResult(e)
			returned = true
			return
		}
	}
	if r.pool != nil {
		r.closeErr = physicalCloseResult(r.pool.Close())
	}
	returned = true
	return
}

type AcceptanceBusyEvidence struct {
	InputDigest                                                                             string
	ReaderStarted, SnapshotHeld, OriginalReturned, ReaderEnded, ReadOnlyURI, CommitReturned bool
	ReaderHead                                                                              int64
	QueryOnly, SQLiteCode, CommitCalls, CommitCode                                          int
}

var errAcceptanceBusy = errors.New("acceptance busy fixture unavailable")

// AcceptanceSaveBusy holds a genuine SHARED read transaction only AFTER the
// original durable admission. apply is the sole original CommitAccepted call.
func (g *CommitGate) AcceptanceSaveBusy(ctx context.Context, a *Admission, apply func() (SaveResult, error)) (result SaveResult, evidence AcceptanceBusyEvidence, err error) {
	result.Status = Unknown
	reader := &acceptanceBusyReader{}
	observation := &acceptanceBusyCommitState{}
	armed := false
	locked, completed := false, false
	defer func() {
		recovered := recover()
		if !completed {
			_ = recovered
			result = SaveResult{Status: Unknown}
			err = errors.Join(err, ErrUnknown, errAcceptanceBusy)
		}
		var coded interface{ Code() int }
		if errors.As(err, &coded) {
			evidence.SQLiteCode = coded.Code()
		}
		if !locked {
			g.mu.Lock()
			locked = true
		}
		defer g.mu.Unlock()
		var disarmErr error
		if armed && !acceptanceBusyCommits.CompareAndDelete(g, observation) {
			disarmErr = errAcceptanceBusy
		}
		observation.mu.Lock()
		evidence.CommitCalls = observation.calls
		evidence.CommitReturned = observation.returned
		evidence.CommitCode = observation.code
		observation.mu.Unlock()
		closeErr := errors.Join(disarmErr, reader.close())
		evidence.ReaderEnded = closeErr == nil
		if !completed || !evidence.OriginalReturned || closeErr != nil {
			g.fenced = true
			if result.Status != Confirmed {
				result = SaveResult{Status: Unknown}
			}
			err = errors.Join(err, ErrUnknown, closeErr)
		}
	}()
	g.mu.Lock()
	locked = true
	fail := func(e error) (SaveResult, AcceptanceBusyEvidence, error) {
		completed = true
		return result, evidence, errors.Join(ErrUnknown, e)
	}
	if g.closed || g.fenced || a == nil || a.gate != g || a.used || a.request.Epoch != g.epoch || apply == nil {
		return fail(errAcceptanceBusy)
	}
	evidence.InputDigest = a.input
	setup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	var e error
	evidence.ReaderHead, e = reader.open(setup, g.db.path, a.request.WorkspaceID)
	cancel()
	if e != nil {
		return fail(e)
	}
	evidence.ReaderStarted = true
	evidence.SnapshotHeld = true
	evidence.ReadOnlyURI = reader.readOnlyURI
	evidence.QueryOnly = reader.queryOnly
	if _, exists := acceptanceBusyCommits.LoadOrStore(g, observation); exists {
		return fail(errAcceptanceBusy)
	}
	armed = true
	g.mu.Unlock()
	locked = false
	result, err = apply()
	evidence.OriginalReturned = true
	completed = true
	return
}
