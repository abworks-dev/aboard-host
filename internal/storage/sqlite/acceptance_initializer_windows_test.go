//go:build windows && amd64

package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	win "golang.org/x/sys/windows"
)

const acceptanceInitializerEnvironment = "ABOARD_ACCEPTANCE_INITIALIZER"

var errAcceptanceInitializer = errors.New("acceptance initializer fixture rejected")

type acceptanceInitializerRequest struct {
	Fixture string
	Root    string
	Kind    DatabaseKind
	Stage   int
}

// This owner exists only in the test binary. In particular, neither a failed
// Close nor an unexpected stdin EOF may turn physical uncertainty into release.
// Retained errors also retain resources returned only through factory errors.
type acceptanceInitializerOwner struct {
	lease       *platform.Lease
	pin         *platform.File
	h           *Handle
	tx          *sql.Tx
	config      *os.File
	quarantined error
	held        bool
}

var acceptanceInitializerRetained []*acceptanceInitializerOwner

func (o *acceptanceInitializerOwner) failed(err error) error {
	if errors.Is(err, ErrPhysicalCloseUnknown) || errors.Is(err, platform.ErrPhysicalCloseUnknown) {
		o.quarantined = errors.Join(o.quarantined, err)
	}
	return err
}

// Only setup failures use rollback/Close. Stop at the first unconfirmed physical
// end, retaining that object and every enclosing lease; never retry raw handles.
func (o *acceptanceInitializerOwner) closeSetup() error {
	if o.quarantined != nil {
		return o.quarantined
	}
	if o.config != nil {
		if err := o.config.Close(); err != nil {
			o.quarantined = errors.Join(ErrPhysicalCloseUnknown, err)
			return o.quarantined
		}
		o.config = nil
	}
	if o.tx != nil {
		if err := o.tx.Rollback(); err != nil {
			o.quarantined = errors.Join(ErrPhysicalCloseUnknown, err)
			return o.quarantined
		}
		o.tx = nil
	}
	if o.h != nil {
		if err := o.h.Close(); err != nil {
			return o.failed(err)
		}
		o.h = nil
	}
	if o.lease != nil {
		// This includes keys retained internally by an unsuccessful pin factory.
		if err := o.lease.ClosePinnedFiles(); err != nil {
			o.quarantined = err
			return err
		}
		o.pin = nil
		if err := o.lease.Close(); err != nil {
			o.quarantined = err
			return err
		}
		o.lease = nil
	}
	return nil
}

func acceptanceInitializerLocalPath(path string) bool {
	return len(path) > 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) &&
		path[1:3] == `:\` && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path[2:], "/:\x00")
}

func readAcceptanceInitializer(encoded string) (acceptanceInitializerRequest, error) {
	var out acceptanceInitializerRequest
	if len(encoded) == 0 || len(encoded) > 8192 {
		return out, errAcceptanceInitializer
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != encoded {
		return out, errAcceptanceInitializer
	}
	value, err := protocol.ParseJSON(raw)
	if err != nil {
		return out, errAcceptanceInitializer
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 4 {
		return out, errAcceptanceInitializer
	}
	for _, key := range []string{"Fixture", "Root", "Kind"} {
		if _, ok := fields[key].(string); !ok {
			return out, errAcceptanceInitializer
		}
	}
	number, ok := fields["Stage"].(json.Number)
	if !ok {
		return out, errAcceptanceInitializer
	}
	stage, err := strconv.Atoi(string(number))
	if err != nil || stage < 0 || stage > 6 || strconv.Itoa(stage) != string(number) {
		return out, errAcceptanceInitializer
	}
	out = acceptanceInitializerRequest{fields["Fixture"].(string), fields["Root"].(string), DatabaseKind(fields["Kind"].(string)), stage}
	_, source, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(source) {
		return out, errAcceptanceInitializer
	}
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
	validation := filepath.Join(project, ".data", "logs", "001-validation")
	if !acceptanceInitializerLocalPath(out.Fixture) || filepath.Dir(out.Fixture) != validation ||
		!strings.HasPrefix(filepath.Base(out.Fixture), "t023-") || len(filepath.Base(out.Fixture)) <= len("t023-") {
		return out, errAcceptanceInitializer
	}
	want := filepath.Join(out.Fixture, "installation")
	switch out.Kind {
	case InstallationDatabase:
	case WorkspaceDatabase:
		want = filepath.Join(out.Fixture, "allowed", "observed-source")
	default:
		return out, errAcceptanceInitializer
	}
	if !acceptanceInitializerLocalPath(out.Root) || out.Root != want {
		return out, errAcceptanceInitializer
	}
	return out, nil
}

// Do not import host: host composes installation, which imports this package.
// Parse precisely its four-field Config shape, with decoded key/type checks,
// and require the exact fixture installation and singleton allowed root.
func (o *acceptanceInitializerOwner) checkConfig(fixture string) error {
	path := filepath.Join(fixture, "config.json")
	if _, err := acceptanceInitializerPlainFile(path); err != nil {
		return err
	}
	var err error
	o.config, err = os.Open(path)
	if err != nil {
		return err
	}
	raw, readErr := io.ReadAll(io.LimitReader(o.config, 8193))
	if err = o.config.Close(); err != nil {
		o.quarantined = errors.Join(ErrPhysicalCloseUnknown, err)
		return o.quarantined
	}
	o.config = nil
	if readErr != nil || len(raw) > 8192 {
		return errAcceptanceInitializer
	}
	value, err := protocol.ParseJSON(raw)
	if err != nil {
		return errAcceptanceInitializer
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 4 {
		return errAcceptanceInitializer
	}
	installation, iok := fields["installationRoot"].(string)
	allowed, aok := fields["allowedRoots"].([]any)
	listen, lok := fields["listen"].(string)
	pipe, pok := fields["pipeName"].(string)
	if !iok || !aok || !lok || !pok || installation != filepath.Join(fixture, "installation") || len(allowed) != 1 {
		return errAcceptanceInitializer
	}
	if root, ok := allowed[0].(string); !ok || root != filepath.Join(fixture, "allowed") {
		return errAcceptanceInitializer
	}
	host, port, err := net.SplitHostPort(listen)
	ip := net.ParseIP(host)
	n, portErr := strconv.ParseUint(port, 10, 16)
	if err != nil || portErr != nil || strconv.FormatUint(n, 10) != port || ip == nil || !ip.IsLoopback() ||
		!strings.HasPrefix(pipe, "aboard-g5-") || len(pipe) > 100 || strings.ContainsAny(pipe, `\/:`+"\x00") {
		return errAcceptanceInitializer
	}
	return o.lease.Validate()
}

func acceptanceInitializerPlainFile(path string) (os.FileInfo, error) {
	if !acceptanceInitializerLocalPath(path) {
		return nil, errAcceptanceInitializer
	}
	name, err := win.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	attributes, err := win.GetFileAttributes(name)
	if err != nil || attributes&(win.FILE_ATTRIBUTE_REPARSE_POINT|win.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return nil, errAcceptanceInitializer
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errAcceptanceInitializer
	}
	return info, nil
}

func (o *acceptanceInitializerOwner) acquire(r acceptanceInitializerRequest) error {
	scope := platform.Installation
	if r.Kind == WorkspaceDatabase {
		scope = platform.Workspace
	}
	var err error
	// Existing fixed lock only. Native relative opens pin every ancestor and
	// reject reparses, aliases, non-NTFS paths and inherited lock handles.
	o.lease, err = platform.Acquire(r.Root, scope, false, false)
	if err != nil {
		return o.failed(err)
	}
	if o.lease.RootPath() != r.Root {
		return errAcceptanceInitializer
	}
	if r.Kind == WorkspaceDatabase {
		// Retain data-directory protection without closing a tracked DB key:
		// closing a key poisons a lease and must never be followed by reuse.
		if err = o.lease.WorkspaceDataHasOnlyDatabase(); err != nil {
			return o.failed(err)
		}
	}
	return o.checkConfig(r.Fixture)
}

func (o *acceptanceInitializerOwner) pinDatabase(kind DatabaseKind, path string) error {
	var err error
	if kind == WorkspaceDatabase {
		o.pin, err = o.lease.PinFile(`data\workspace.sqlite`, false)
	} else {
		o.pin, err = o.lease.PinInstallationFile("installation.sqlite")
	}
	if err != nil {
		return o.failed(err)
	}
	if o.pin.Path() != path {
		return errAcceptanceInitializer
	}
	return o.lease.Validate()
}

func acceptanceInitializerNoSidecars(path string) error {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	prefix := strings.ToLower(filepath.Base(path)) + "-"
	for _, entry := range entries {
		if strings.HasPrefix(strings.ToLower(entry.Name()), prefix) {
			return errAcceptanceInitializer
		}
	}
	return nil
}

func (o *acceptanceInitializerOwner) prepare(r acceptanceInitializerRequest) error {
	ctx := context.Background()
	path := filepath.Join(r.Root, "installation.sqlite")
	if r.Kind == WorkspaceDatabase {
		path = filepath.Join(r.Root, "data", "workspace.sqlite")
	}
	if err := o.acquire(r); err != nil {
		return err
	}
	if err := o.pinDatabase(r.Kind, path); err != nil {
		return err
	}
	if err := acceptanceInitializerNoSidecars(path); err != nil {
		return err
	}
	original, err := acceptanceInitializerPlainFile(path)
	if err != nil {
		return err
	}
	// Populate any lazily loaded Windows file IDs while the original native DB
	// pin is still held, rather than resolving both FileInfos after release.
	originalCheck, err := acceptanceInitializerPlainFile(path)
	if err != nil || !os.SameFile(original, originalCheck) {
		return errAcceptanceInitializer
	}
	o.h, err = Open(ctx, path, ReadWriteExisting)
	if err != nil {
		return o.failed(err)
	}
	// Own the verification transaction too: failed original-view teardown must
	// not leave an untracked sql.Tx or release its enclosing native exclusion.
	o.tx, err = o.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	if err = verifySchema(ctx, o.tx, r.Kind); err != nil {
		return err
	}
	g := Genesis{Kind: r.Kind}
	var identity string
	if r.Kind == WorkspaceDatabase {
		err = o.tx.QueryRowContext(ctx, "SELECT workspace_id,tenant_id,identity_digest,display_name FROM workspace_identity WHERE singleton=1 AND format_version=2 AND schema_version=1 AND business_state='ready'").Scan(&g.WorkspaceID, &g.TenantID, &identity, &g.DisplayName)
	} else {
		err = o.tx.QueryRowContext(ctx, "SELECT installation_id,tenant_id,owner_principal_id,windows_sid,secret_reference FROM installation_identity WHERE singleton=1 AND schema_version=1").Scan(&g.InstallationID, &g.TenantID, &g.OwnerPrincipalID, &g.WindowsSID, &g.SecretReference)
	}
	if err != nil {
		return err
	}
	steps, err := migrations(r.Kind)
	if err != nil || len(steps) != 1 || steps[0].sequence != 1 || steps[0].version != 1 {
		return errAcceptanceInitializer
	}
	rootID, lockID := o.lease.RootIdentity(), o.lease.LockIdentity()
	if err = o.lease.Validate(); err != nil {
		return err
	}
	// The original connection, DB pin AND lease must all physically end before
	// reacquisition. A failed Close leaves them quarantined, not retried.
	if err = o.closeSetup(); err != nil {
		return err
	}
	if err = o.acquire(r); err != nil {
		return err
	}
	if o.lease.RootIdentity() != rootID || o.lease.LockIdentity() != lockID {
		return errAcceptanceInitializer
	}
	if err = acceptanceInitializerNoSidecars(path); err != nil {
		return err
	}
	current, err := acceptanceInitializerPlainFile(path)
	if err != nil || !os.SameFile(original, current) {
		return errAcceptanceInitializer
	}
	// Verify the resolved absolute deletion target immediately before the sole
	// deletion. No recursive cleanup, sidecar deletion, lock replacement or
	// fallback pathname is permitted. The native parent pins remain held.
	want := filepath.Join(r.Fixture, "installation", "installation.sqlite")
	if r.Kind == WorkspaceDatabase {
		want = filepath.Join(r.Fixture, "allowed", "observed-source", "data", "workspace.sqlite")
	}
	if !acceptanceInitializerLocalPath(path) || path != want {
		return errAcceptanceInitializer
	}
	if err = o.lease.Validate(); err != nil {
		return err
	}
	if err = os.Remove(path); err != nil {
		return err
	}
	o.h, err = Open(ctx, path, CreateNew)
	if err != nil {
		return o.failed(err)
	}
	if err = o.pinDatabase(r.Kind, path); err != nil {
		return err
	}
	if r.Stage == 0 {
		return nil // actual new empty DB and fixed connection profile, no tx yet
	}
	if _, err = o.h.Conn.ExecContext(ctx, "PRAGMA cache_size=2"); err != nil {
		return err
	}
	var cache int
	if err = o.h.Conn.QueryRowContext(ctx, "PRAGMA cache_size").Scan(&cache); err != nil || cache != 2 {
		return errAcceptanceInitializer
	}
	o.tx, err = o.h.Conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	statements := []struct {
		s string
		a []any
	}{
		{steps[0].sql, nil},
		{"INSERT INTO migration_ledger(sequence,version,checksum,state) VALUES(?,?,?,'applied')", []any{steps[0].sequence, steps[0].version, steps[0].checksum}},
		{"INSERT INTO schema_metadata VALUES(1,?,1)", []any{string(r.Kind)}},
		{"PRAGMA user_version=1", nil},
	}
	if r.Kind == WorkspaceDatabase {
		statements = append(statements, struct {
			s string
			a []any
		}{"INSERT INTO workspace_identity VALUES(1,?,?,2,1,?,?,'ready')", []any{g.WorkspaceID, g.TenantID, identity, g.DisplayName}}, struct {
			s string
			a []any
		}{"INSERT INTO workspace_head VALUES(?,0)", []any{g.WorkspaceID}})
	} else {
		statements = append(statements, struct {
			s string
			a []any
		}{"INSERT INTO installation_identity VALUES(1,?,?,?,?,1,?)", []any{g.InstallationID, g.TenantID, g.OwnerPrincipalID, g.WindowsSID, g.SecretReference}}, struct {
			s string
			a []any
		}{"INSERT INTO principal VALUES(?,?,1,0)", []any{g.OwnerPrincipalID, g.TenantID}})
	}
	for i, statement := range statements {
		if _, err = o.tx.ExecContext(ctx, statement.s, statement.a...); err != nil {
			return err
		}
		if r.Stage == i+1 {
			// Even phase 6 is still INSIDE this same live transaction. There is
			// deliberately no Commit call anywhere in this helper.
			return nil
		}
	}
	return errAcceptanceInitializer
}

func (o *acceptanceInitializerOwner) ready(r acceptanceInitializerRequest) error {
	if o.quarantined != nil || o.h == nil || o.pin == nil || (r.Stage == 0) != (o.tx == nil) {
		return errAcceptanceInitializer
	}
	if err := o.lease.Validate(); err != nil {
		return err
	}
	marker, err := json.Marshal(struct {
		Version int
		PID     int
		Kind    DatabaseKind
		Stage   int
	}{1, os.Getpid(), r.Kind, r.Stage})
	if err != nil {
		return err
	}
	line := append([]byte("ABOARD_INITIALIZER_READY"), marker...)
	line = append(line, '\n')
	if len(line) > 256 {
		return errAcceptanceInitializer
	}
	if n, err := os.Stdout.Write(line); err != nil || n != len(line) {
		return errAcceptanceInitializer
	}
	o.held = true
	acceptanceInitializerRetained = append(acceptanceInitializerRetained, o)
	// Parent hard-kills this process, then Waits. Never roll back or close a
	// ready owner, even if a broken caller writes again or closes stdin.
	var next [1]byte
	_, _ = os.Stdin.Read(next[:])
	select {}
}

func TestAcceptanceInitializerStrictScope(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source anchor unavailable")
	}
	fixture := filepath.Join(filepath.Dir(source), "..", "..", "..", ".data", "logs", "001-validation", "t023-guard")
	fixture = filepath.Clean(fixture)
	root := filepath.Join(fixture, "allowed", "observed-source")
	valid, _ := json.Marshal(acceptanceInitializerRequest{fixture, root, WorkspaceDatabase, 6})
	encode := func(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }
	if _, err := readAcceptanceInitializer(encode(valid)); err != nil {
		t.Fatal("canonical fixed scope rejected")
	}
	for name, raw := range map[string][]byte{
		"duplicate":        append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"Stage":6}`)...),
		"case":             []byte(strings.Replace(string(valid), `"Stage"`, `"stage"`, 1)),
		"null":             []byte(strings.Replace(string(valid), `"Stage":6`, `"Stage":null`, 1)),
		"exponent":         []byte(strings.Replace(string(valid), `"Stage":6`, `"Stage":6e0`, 1)),
		"out-of-range":     []byte(strings.Replace(string(valid), `"Stage":6`, `"Stage":7`, 1)),
		"different-target": []byte(strings.Replace(string(valid), `observed-source`, `other-source`, 1)),
		"outside-prefix":   []byte(strings.ReplaceAll(string(valid), `t023-guard`, `user-data`)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readAcceptanceInitializer(encode(raw)); err == nil {
				t.Fatal("unsafe helper scope admitted")
			}
		})
	}
	for _, bad := range []string{"", strings.Repeat("x", 8193), encode(valid) + "="} {
		if _, err := readAcceptanceInitializer(bad); err == nil {
			t.Fatal("invalid bounded framing admitted")
		}
	}
}

func TestAcceptanceInitializerHelper(t *testing.T) {
	encoded := os.Getenv(acceptanceInitializerEnvironment)
	if encoded == "" {
		t.Skip("separate synthetic initializer process only")
	}
	r, err := readAcceptanceInitializer(encoded)
	if err != nil {
		t.Fatal("acceptance initializer configuration rejected")
	}
	// Parent launch-pins the executable and birth identity before sending this
	// literal byte (0x01). No fixture open/configuration/SQL mutation precedes it.
	var permit [1]byte
	if _, err = io.ReadFull(os.Stdin, permit[:]); err != nil || permit[0] != 1 {
		t.Fatal("acceptance initializer permit rejected")
	}
	o := &acceptanceInitializerOwner{}
	defer func() {
		if !o.held {
			if err := o.closeSetup(); err != nil {
				acceptanceInitializerRetained = append(acceptanceInitializerRetained, o)
				t.Error("acceptance initializer setup cleanup unconfirmed")
			}
		}
	}()
	if err = o.prepare(r); err == nil {
		err = o.ready(r)
	}
	_ = o.failed(err)
	// Never print SQL parameters, paths, Genesis, SID or secret references.
	t.Fatal("acceptance initializer setup failed")
}
