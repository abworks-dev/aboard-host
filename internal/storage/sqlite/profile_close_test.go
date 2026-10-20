package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// This database/sql lifecycle control verifies error preservation, not SQLite
// durability or an actual SQLite close failure. The required real-SQLite gate
// still runs in full, including native COMMIT errors and process interruption.
type closeFailureControl struct {
	calls   atomic.Int32
	failure error
}

func (c *closeFailureControl) Connect(context.Context) (driver.Conn, error) {
	return &closeFailureConn{c}, nil
}
func (c *closeFailureControl) Driver() driver.Driver { return closeControlDriver{} }

type closeControlDriver struct{}

func (closeControlDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("unused lifecycle control")
}

type closeFailureConn struct{ c *closeFailureControl }

func (c *closeFailureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("no SQL in lifecycle control")
}
func (c *closeFailureConn) Begin() (driver.Tx, error) {
	return nil, errors.New("no transaction in lifecycle control")
}
func (c *closeFailureConn) Close() error { c.c.calls.Add(1); return c.c.failure }

func TestProfileGatePhysicalCloseErrorPreserved(t *testing.T) {
	cause := errors.New("synthetic driver close failed")
	ctl := &closeFailureControl{failure: cause}
	db := sql.OpenDB(ctl)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h := &Handle{Conn: conn, db: db}
	for attempt := 0; attempt < 3; attempt++ {
		err = h.Close()
		if !errors.Is(err, ErrPhysicalCloseUnknown) || !errors.Is(err, cause) {
			t.Fatalf("attempt %d lost unknown close cause: %v", attempt, err)
		}
	}
	if ctl.calls.Load() != 1 {
		t.Fatal("Close retried the discarded driver")
	}
	// database/sql reports zero even though the driver returned a failure. This
	// is why pool counters and a second nil Close must not certify physical drain.
	if db.Stats().OpenConnections != 0 {
		t.Fatal("control did not exercise discarded connection")
	}
	if physicalCloseResult(nil) != nil {
		t.Fatal("successful close marked unknown")
	}
}

func TestProfileGateFactoryErrorRetainsOriginalOwner(t *testing.T) {
	owner := &Handle{}
	cause := errors.New("controlled factory-close cause")
	err := retainCloseOwner(physicalCloseResult(cause), owner)
	var retained *retainedCloseOwner
	if !errors.As(err, &retained) || retained.owner != owner || !errors.Is(err, ErrPhysicalCloseUnknown) || !errors.Is(err, cause) {
		t.Fatal("nil-result factory error lost original owner/cause", err)
	}
	if got := retainCloseOwner(cause, owner); got != cause {
		t.Fatal("ordinary failure quarantined")
	}
}

func TestProfileGateRepeatedRealCloseAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "real-close.sqlite")
	h := openGate(t, path, CreateNew)
	execGate(t, h, "CREATE TABLE durable(value TEXT NOT NULL)")
	execGate(t, h, "INSERT INTO durable VALUES('合成 close')")
	closeGate(t, h)
	closeGate(t, h)
	reopened := openGate(t, path, ReadWriteExisting)
	defer closeGate(t, reopened)
	var got string
	if err := reopened.Conn.QueryRowContext(context.Background(), "SELECT value FROM durable").Scan(&got); err != nil || got != "合成 close" {
		t.Fatalf("real close/reopen changed data: %q %v", got, err)
	}
}
