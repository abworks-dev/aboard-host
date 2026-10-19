//go:build windows && amd64

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/workspace"
	"golang.org/x/sys/windows"
	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestWorkspacePreviewBaselineAndPersistence(t *testing.T) {
	ctx := context.Background()
	d, g := newTestGate(t)
	defer g.EndPhysical()
	f, err := NewWorkspaceFacts(g, factsScope())
	if err != nil {
		t.Fatal(err)
	}
	c, r := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
	foreign := factsScope()
	foreign.TenantID = testInstallation
	if err = f.PutPreview(ctx, foreign, c); !errors.Is(err, workspace.ErrScope) {
		t.Fatal(err)
	}
	if err = f.PutPreview(ctx, factsScope(), c); err != nil {
		t.Fatal(err)
	}
	var state, body, principal, epoch, expires, hash string
	var version int64
	if err = d.h.Conn.QueryRowContext(ctx, `SELECT state,body,principal_id,preview_epoch,expires_at,candidate_digest,preview_version FROM candidate WHERE workspace_id=? AND candidate_id=?`, testWorkspace, c.CandidateID).Scan(&state, &body, &principal, &epoch, &expires, &hash, &version); err != nil {
		t.Fatal(err)
	}
	if state != "previewed" || body != c.Body || principal != testPrincipal || epoch != g.epoch || expires != c.ExpiresAt.UTC().Format(time.RFC3339Nano) || hash != c.CandidateDigest || version != 1 {
		t.Fatal(state, body, principal, epoch, expires, hash, version)
	}
	if s, err := f.Snapshot(ctx, factsScope()); err != nil || s.Revision != 0 {
		t.Fatal(s, err)
	}
	if _, err = f.Current(ctx, factsScope(), testResource); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatal(err)
	}
	_, ticket, err := g.Admit(ctx, r, time.Now())
	if err != nil || ticket == nil {
		t.Fatal(err)
	}
	if result, err := g.CommitAccepted(ctx, ticket); err != nil || result.Status != Confirmed {
		t.Fatal(result, err)
	}
	stale, _ := candidateFixture(t, g, testPrincipal, testTenant, "create-text", 0, nil)
	if err = f.PutPreview(ctx, factsScope(), stale); !errors.Is(err, workspace.ErrBaseline) {
		t.Fatal("stale admitted", err)
	}
	var count int
	if err = d.h.Conn.QueryRowContext(ctx, "SELECT count(*) FROM candidate WHERE candidate_id=?", stale.CandidateID).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	base := c.ContentDigest
	update, _ := candidateFixture(t, g, testPrincipal, testResource, "update-text", 1, &base)
	if err = f.PutPreview(ctx, factsScope(), update); err != nil {
		t.Fatal(err)
	}
	formal, err := f.Current(ctx, factsScope(), testResource)
	if err != nil || formal.Body != c.Body || formal.Version != 1 || formal.Revision != 1 {
		t.Fatal(formal, err)
	}
	if err = g.EndPhysical(); err != nil {
		t.Fatal(err)
	}
	fresh, err := OpenDatabase(ctx, d.path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if err = fresh.h.Conn.QueryRowContext(ctx, "SELECT count(*) FROM candidate WHERE state='previewed'").Scan(&count); err != nil || count != 1 {
		t.Fatal("fresh preview", count, err)
	}
	t.Log("real SQLite candidate roundtrip, transactional recheck after intervening formal commit; no business commit/Host claim")
}

func TestWorkspacePreviewNativeCommitFaultFences(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "synthetic-preview-flush")
	if err := os.WriteFile(p, []byte("synthetic readonly native flush stimulus"), 0600); err != nil {
		t.Fatal(err)
	}
	stimulus, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer stimulus.Close()
	if err = windows.FlushFileBuffers(windows.Handle(stimulus.Fd())); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatal(err)
	}
	var count int32
	for ordinal := int32(0); ordinal == 0 || ordinal <= count; ordinal++ {
		t.Run(fmt.Sprint(ordinal), func(t *testing.T) {
			d, g := newTestGate(t)
			defer g.EndPhysical()
			f, err := NewWorkspaceFacts(g, factsScope())
			if err != nil {
				t.Fatal(err)
			}
			c, _ := candidateFixture(t, g, testCandidate, testResource, "create-text", 0, nil)
			flushFault.handle = stimulus.Fd()
			flushFault.at = ordinal
			flushFault.calls.Store(0)
			flushFault.failed.Store(0)
			restore := replaceFlushForGate(t)
			err = f.PutPreview(ctx, factsScope(), c)
			calls, failed := flushFault.calls.Load(), flushFault.failed.Load()
			restore()
			if ordinal == 0 {
				if err != nil || calls < 1 || calls > 8 {
					t.Fatal(err, calls)
				}
				count = calls
			} else {
				var native *driver.Error
				if !errors.Is(err, ErrUnknown) || !errors.Is(err, workspace.ErrOpenUncertain) || !errors.As(err, &native) || native.Code() != sqlite3.SQLITE_IOERR_FSYNC || failed != 1 {
					t.Fatal(err, calls, failed)
				}
				if err = f.PutPreview(ctx, factsScope(), c); !errors.Is(err, ErrFenced) {
					t.Fatal("unknown preview unfenced", err)
				}
			}
			if err = g.EndPhysical(); err != nil {
				t.Fatal(err)
			}
			fresh, err := OpenDatabase(ctx, d.path, ReadWriteExisting, Genesis{Kind: WorkspaceDatabase})
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			for _, table := range []string{"text_record", "text_version", "save_operation", "commit_receipt", "audit_entry"} {
				var n int
				if err = fresh.h.Conn.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil || n != 0 {
					t.Fatal(table, n, err)
				}
			}
			var head int
			if err = fresh.h.Conn.QueryRowContext(ctx, "SELECT revision FROM workspace_head").Scan(&head); err != nil || head != 0 {
				t.Fatal(head, err)
			}
		})
	}
	t.Log("all observed real SQLite preview COMMIT flush ordinals faulted; no formal effect, unknown never reported success; not native lease/drain or complete Host acceptance")
}
