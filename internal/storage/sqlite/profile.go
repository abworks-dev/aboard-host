// Package sqlite is the fixed M1 storage adapter. It is not an authorization or
// directory-lock boundary: callers must own the approved root and writer lock.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

const EngineVersion = "3.53.4"

// Mode never accepts caller-supplied DSNs, pragmas, VFSes or SQL.
type Mode uint8

const (
	CreateNew Mode = iota + 1
	ReadWriteExisting
	// ReadOnlyScratch is only for a private, frozen copy; not a live source DB.
	ReadOnlyScratch
)

var ErrProfile = errors.New("unsupported SQLite profile")

// ErrPhysicalCloseUnknown means failed cleanup has not proved physical release.
// Factory callers must retain their OS lease and quarantine this owner until
// process termination; a nil returned database does not prove successful drain.
var ErrPhysicalCloseUnknown = errors.New("storage physical close not confirmed")

// Handle owns exactly one checked physical connection until Close. No pool
// connection may be used unchecked. All SQL access is internal and serialized
// by the caller; the adapter does not claim OS exclusion or atomic creation.
type Handle struct {
	Conn           *sql.Conn
	db             *sql.DB
	closeMu        sync.Mutex
	closeAttempted bool
	closeResult    error
}

type retainedCloseOwner struct {
	cause error
	owner any
}

func (e *retainedCloseOwner) Error() string { return e.cause.Error() }
func (e *retainedCloseOwner) Unwrap() error { return e.cause }
func retainCloseOwner(err error, owner any) error {
	if errors.Is(err, ErrPhysicalCloseUnknown) {
		return &retainedCloseOwner{err, owner}
	}
	return err
}

func physicalCloseResult(errs ...error) error {
	if err := errors.Join(errs...); err != nil {
		return errors.Join(ErrPhysicalCloseUnknown, err)
	}
	return nil
}

func (h *Handle) Close() error {
	h.closeMu.Lock()
	defer h.closeMu.Unlock()
	if h.closeAttempted {
		return h.closeResult
	}
	// database/sql may discard a driver after a failed Close. A second nil
	// result would not prove that driver's physical release, so retain the
	// original failure rather than laundering it into a successful drain.
	h.closeAttempted = true
	h.closeResult = physicalCloseResult(h.Conn.Close(), h.db.Close())
	return h.closeResult
}

func fixedURI(path string, mode Mode) (string, error) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return "", ErrProfile
	}
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) || strings.HasPrefix(path, `\\`) {
		return "", ErrProfile
	}
	m := "rw"
	switch mode {
	case CreateNew, ReadWriteExisting:
	case ReadOnlyScratch:
		m = "ro"
	default:
		return "", ErrProfile
	}
	u := url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(filepath.Clean(path))}
	q := url.Values{"mode": {m}, "vfs": {"win32"}}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Open never creates on an existing-open path and never recovers a source in
// inspect mode. CreateNew's exclusive empty file is deliberate; a failed open
// leaves its artifact for explicit owner reconciliation, never delete/reseed.
// Path/ancestor/handle stability and ACLs are enforced by the Windows owner.
func Open(ctx context.Context, path string, mode Mode) (*Handle, error) {
	dsn, err := fixedURI(path, mode)
	if err != nil {
		return nil, err
	}
	if err := verifyFlushBridge(); err != nil {
		return nil, err // fail closed before even creating a new empty file
	}
	if mode == CreateNew {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, retainCloseOwner(physicalCloseResult(err), f)
		}
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, retainCloseOwner(errors.Join(err, physicalCloseResult(db.Close())), db)
	}
	h := &Handle{Conn: conn, db: db}
	if err := configure(ctx, conn, mode); err != nil {
		return nil, retainCloseOwner(errors.Join(err, h.Close()), h)
	}
	return h, nil
}

func configure(ctx context.Context, conn *sql.Conn, mode Mode) error {
	var initialJournal string
	query := "PRAGMA journal_mode"
	if mode == CreateNew {
		query = "PRAGMA journal_mode=DELETE"
	}
	if err := conn.QueryRowContext(ctx, query).Scan(&initialJournal); err != nil {
		return err
	}
	if initialJournal != "delete" {
		return ErrProfile // existing mode is rejected, never silently converted
	}
	for _, statement := range []string{
		"PRAGMA synchronous=EXTRA",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=1000",
		"PRAGMA trusted_schema=OFF",
		"PRAGMA mmap_size=0",
	} {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	queryOnly := 0
	if mode == ReadOnlyScratch {
		if _, err := conn.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
			return err
		}
		queryOnly = 1
	}
	var version, journal string
	if err := conn.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		return err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
		return err
	}
	if version != EngineVersion || journal != "delete" {
		return fmt.Errorf("%w: engine=%s journal=%s", ErrProfile, version, journal)
	}
	for _, setting := range []struct {
		query string
		want  int
	}{
		{"PRAGMA synchronous", 3},
		{"PRAGMA foreign_keys", 1},
		{"PRAGMA busy_timeout", 1000},
		{"PRAGMA trusted_schema", 0},
		{"PRAGMA mmap_size", 0},
		{"PRAGMA query_only", queryOnly},
	} {
		var got int
		if err := conn.QueryRowContext(ctx, setting.query).Scan(&got); err != nil {
			return err
		}
		if got != setting.want {
			return fmt.Errorf("%w: %s=%d", ErrProfile, setting.query, got)
		}
	}
	return nil
}
