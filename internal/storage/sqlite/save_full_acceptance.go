//go:build windows && amd64 && aboard_acceptance

package sqlite

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// Pager-local limits, not native disk exhaustion or a fabricated driver error.
type AcceptanceFullEvidence struct {
	PageSize, PageCount, FreePages, OldCap, EffectiveCap, RestoredCap int64
	InputDigest                                                       string
	Armed, OriginalReturned, Restored                                 bool
	SQLiteCode                                                        int
}

var errAcceptanceFull = errors.New("acceptance pager fixture unavailable")

// The owner retains the only writer and serializes this whole command. Only its
// minted admission and retained connection can enter; apply invokes the original
// CommitAccepted once. There is no caller SQL, schema/profile change or retry.
func (g *CommitGate) AcceptanceSaveFull(ctx context.Context, a *Admission, apply func() (SaveResult, error)) (result SaveResult, evidence AcceptanceFullEvidence, err error) {
	result.Status = Unknown
	locked, completed := false, false
	// Register ownership BEFORE any lock-held driver/setup/setter operation.
	defer func() {
		recovered := recover()
		if !completed {
			_ = recovered
			result = SaveResult{Status: Unknown}
			err = errors.Join(err, ErrUnknown, errAcceptanceFull)
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
		var restoreErr error
		if evidence.Armed {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			func() {
				restoredReturn := false
				defer func() {
					recovered := recover()
					if !restoredReturn {
						_ = recovered
						restoreErr = errAcceptanceFull
					}
				}()
				if g.closed {
					restoreErr = ErrFenced
				} else {
					restoreErr = g.db.h.Conn.QueryRowContext(cleanup, "PRAGMA main.max_page_count="+strconv.FormatInt(evidence.OldCap, 10)).Scan(&evidence.RestoredCap)
					if restoreErr == nil && evidence.RestoredCap != evidence.OldCap {
						restoreErr = errAcceptanceFull
					}
				}
				restoredReturn = true
			}()
			evidence.Restored = restoreErr == nil
		}
		if !completed || !evidence.OriginalReturned || restoreErr != nil {
			g.fenced = true
			if result.Status != Confirmed {
				result = SaveResult{Status: Unknown}
			}
			err = errors.Join(err, ErrUnknown, restoreErr)
		}
	}()
	g.mu.Lock()
	locked = true
	failSetup := func(cause error) (SaveResult, AcceptanceFullEvidence, error) {
		completed = true
		return result, evidence, errors.Join(ErrUnknown, cause)
	}
	if g.closed || g.fenced || a == nil || a.gate != g || a.used || a.request.Epoch != g.epoch || apply == nil {
		return failSetup(errAcceptanceFull)
	}
	evidence.InputDigest = a.input
	for _, q := range []struct {
		sql string
		out *int64
	}{
		{"PRAGMA main.page_size", &evidence.PageSize}, {"PRAGMA main.page_count", &evidence.PageCount},
		{"PRAGMA main.freelist_count", &evidence.FreePages}, {"PRAGMA main.max_page_count", &evidence.OldCap},
	} {
		if e := g.db.h.Conn.QueryRowContext(ctx, q.sql).Scan(q.out); e != nil {
			return failSetup(e)
		}
	}
	if evidence.PageSize < 512 || evidence.PageSize > 65536 || evidence.PageSize&(evidence.PageSize-1) != 0 || evidence.PageCount < 1 || evidence.FreePages != 0 || evidence.OldCap <= evidence.PageCount {
		return failSetup(errAcceptanceFull)
	}
	// A failed/panicking setter may have changed state: ownership is armed first.
	// Only this actual positive observed integer enters the fixed pragma.
	evidence.Armed = true
	if e := g.db.h.Conn.QueryRowContext(ctx, "PRAGMA main.max_page_count="+strconv.FormatInt(evidence.PageCount, 10)).Scan(&evidence.EffectiveCap); e != nil {
		return failSetup(e)
	}
	if evidence.EffectiveCap != evidence.PageCount {
		return failSetup(errAcceptanceFull)
	}
	g.mu.Unlock()
	locked = false
	result, err = apply()
	evidence.OriginalReturned = true
	completed = true
	return
}
