//go:build windows && amd64

package acceptance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/host"
	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

const badFixtureEnvironment = "ABOARD_ACCEPTANCE_BAD_FIXTURE"

type badFixtureInput struct{ Fixture, Root, Kind, Variant string }

// These are synthetic corruption constructors, never commands supplied to Host.
// Only fixed statements/variants are accepted, under the real exclusion adapter.
func badFixtureStatements(kind, variant string) []string {
	common := map[string][]string{
		"ledger-missing":      {"DELETE FROM migration_ledger"},
		"ledger-checksum":     {"UPDATE migration_ledger SET checksum='" + strings.Repeat("0", 64) + "'"},
		"ledger-order":        {"UPDATE migration_ledger SET sequence=2"},
		"ledger-state":        {"PRAGMA ignore_check_constraints=ON", "UPDATE migration_ledger SET state='pending'"},
		"ledger-version":      {"UPDATE migration_ledger SET version=2"},
		"user-version-future": {"PRAGMA user_version=2"},
		"user-version-old":    {"PRAGMA user_version=0"},
		"schema-metadata":     {"PRAGMA ignore_check_constraints=ON", "UPDATE schema_metadata SET version=2"},
	}
	if q := common[variant]; q != nil && (kind == "installation" || kind == "workspace") {
		return q
	}
	if kind != "workspace" {
		return nil
	}
	switch variant {
	case "business-unavailable":
		return []string{"UPDATE workspace_identity SET business_state='unavailable'"}
	case "database-foreign-tenant":
		return []string{"PRAGMA foreign_keys=OFF", "UPDATE workspace_identity SET tenant_id='5abfa6ad-0611-4dd9-adbb-04746bacaa11'"}
	case "database-forged-id":
		return []string{"PRAGMA foreign_keys=OFF", "UPDATE workspace_identity SET workspace_id='5abfa6ad-0611-4dd9-adbb-04746bacaa12'"}
	case "foreign-key-violation":
		return []string{"PRAGMA foreign_keys=OFF", "UPDATE text_record SET current_version=99"}
	case "immutable-content-corrupt":
		return []string{"DROP TRIGGER immutable_text_version_update", "UPDATE text_version SET body='synthetic corrupted formal bytes'", "CREATE TRIGGER immutable_text_version_update BEFORE UPDATE ON text_version BEGIN SELECT RAISE(ABORT,'immutable version'); END"}
	}
	return nil
}
func TestAcceptanceBadFixtureHelper(t *testing.T) {
	encoded := os.Getenv(badFixtureEnvironment)
	if encoded == "" {
		t.Skip("synthetic fixture helper only")
	}
	if len(encoded) > 16384 {
		t.Fatal("oversized synthetic fixture")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	var in badFixtureInput
	if err != nil || json.Unmarshal(raw, &in) != nil || requireSyntheticRoot(in.Fixture) != nil {
		t.Fatal("bad fixture scope")
	}
	config, err := host.LoadConfig(filepath.Join(in.Fixture, "config.json"))
	if err != nil || !unreparsed(in.Root) {
		t.Fatal("bad fixture config/target")
	}
	kind, name := platform.Workspace, "data/workspace.sqlite"
	if in.Kind == "installation" {
		kind, name = platform.Installation, "installation.sqlite"
		if in.Root != config.InstallationRoot {
			t.Fatal("installation target mismatch")
		}
	} else if in.Kind != "workspace" || len(config.AllowedRoots) != 1 || !within(config.AllowedRoots[0], in.Root) {
		t.Fatal("workspace target mismatch")
	}
	statements := badFixtureStatements(in.Kind, in.Variant)
	if len(statements) == 0 {
		t.Fatal("unsupported fixed mutation")
	}
	lease, err := platform.Acquire(in.Root, kind, false, false)
	if err != nil {
		t.Fatal("fixture requires native exclusion")
	}
	var pin *platform.File
	if in.Kind == "installation" {
		pin, err = lease.PinInstallationFile(name)
	} else {
		pin, err = lease.PinFile(filepath.FromSlash(name), false)
	}
	if err != nil {
		t.Fatal("fixture pin refused")
	}
	handle, err := sqlite.Open(context.Background(), filepath.Join(in.Root, filepath.FromSlash(name)), sqlite.ReadWriteExisting)
	if err != nil {
		t.Fatal("fixture existing SQLite refused")
	}
	for _, q := range statements {
		if _, err = handle.Conn.ExecContext(context.Background(), q); err != nil {
			t.Fatal("fixed mutation failed", in.Variant)
		}
	}
	if handle.Close() != nil || pin.Close() != nil || lease.Close() != nil {
		t.Fatal("fixture physical close failed")
	}
}
func mutateBadSQL(t *testing.T, f *fixture, root, kind, variant string) {
	t.Helper()
	if badFixtureStatements(kind, variant) == nil {
		t.Fatal("unrecognized mutation")
	}
	raw, _ := json.Marshal(badFixtureInput{f.root, root, kind, variant})
	image, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, image, "-test.run=^TestAcceptanceBadFixtureHelper$", "-test.v")
	cmd.Dir = projectRoot()
	cmd.Env = append(os.Environ(), badFixtureEnvironment+"="+base64.RawURLEncoding.EncodeToString(raw))
	var out boundedCapture
	cmd.Stdout = &out
	cmd.Stderr = &out
	if cmd.Run() != nil || ctx.Err() != nil {
		b, _ := out.snapshot()
		t.Fatalf("fixed bad fixture helper failed: %s", b)
	}
}

type inspectionBytes map[string][]byte

func saveInspectionBytes(t *testing.T, root string) inspectionBytes {
	t.Helper()
	result := inspectionBytes{}
	if requireSyntheticRoot(filepath.Dir(filepath.Dir(root))) != nil && !within(logRoot(), root) {
		t.Fatal("unowned snapshot")
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			t.Fatal("fixture reparse")
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		b, e := os.ReadFile(path)
		result[rel] = b
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func restoreInspectionBytes(t *testing.T, root string, saved inspectionBytes) {
	t.Helper()
	if !within(logRoot(), root) || !unreparsed(root) {
		t.Fatal("unowned restore")
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if _, ok := saved[rel]; !ok {
			if !within(root, path) {
				t.Fatal("unowned removal")
			}
			return os.Remove(path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for rel, b := range saved {
		path := filepath.Join(root, rel)
		if !within(root, path) {
			t.Fatal("unowned restore target")
		}
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func mutateBadDescriptor(t *testing.T, root, variant string) {
	t.Helper()
	path := filepath.Join(root, "aboard.workspace.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d workspace.DirectoryDescriptor
	if json.Unmarshal(raw, &d) != nil {
		t.Fatal("descriptor fixture")
	}
	switch variant {
	case "future-format":
		d.FormatVersion = 99
	case "legacy-v1":
		d.FormatVersion = 1
	case "future-digest-version":
		d.IdentityDigestVersion = 2
	case "digest-mismatch":
		d.IdentityDigest = strings.Repeat("0", 64)
	case "forged-workspace":
		d, err = workspace.NewDirectoryDescriptor(string(newID(t)), d.TenantID)
	case "foreign-descriptor":
		d, err = workspace.NewDirectoryDescriptor(d.WorkspaceID, "5abfa6ad-0611-4dd9-adbb-04746bacaa11")
	case "unknown-field":
		raw = append(raw[:len(raw)-1], []byte(`,"seal":"not-authority"}`)...)
	case "duplicate-field":
		raw = append(raw[:len(raw)-1], []byte(`,"tenantId":"`+d.TenantID+`"}`)...)
	case "malformed-descriptor":
		raw = []byte(`{"tenantId":`)
	default:
		t.Fatal("unknown descriptor mutation")
	}
	if err != nil {
		t.Fatal(err)
	}
	if variant != "unknown-field" && variant != "duplicate-field" && variant != "malformed-descriptor" {
		raw, err = json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func newInspectionWorkspace(t *testing.T, c *client, name string) (string, protocol.WorkspaceView) {
	t.Helper()
	root := filepath.Join(c.process.fixture.config.AllowedRoots[0], name)
	if os.Mkdir(root, 0700) != nil {
		t.Fatal("synthetic directory creation")
	}
	var out protocol.WorkspaceView
	c.request(t, "POST", "/v1/workspace-creations", protocol.CreateWorkspace{OperationId: newID(t), Path: protocol.Path(root), DisplayName: "inspection synthetic"}, &out, 201)
	return root, out
}

func TestV009IndependentBadDirectoryMatrix(t *testing.T) {
	f := newFixture(t)
	f.initialize(t)
	p := f.start(t)
	c := p.connect(t)
	root, a := newInspectionWorkspace(t, c, "inspection-source")
	_, b := newInspectionWorkspace(t, c, "independent-B")
	opened := c.open(t, a.WorkspaceId)
	var candidate protocol.CandidateView
	prefix := "/v1/workspaces/" + string(a.WorkspaceId)
	c.request(t, "POST", prefix+"/record-previews", protocol.PreviewRecord{OpenEpoch: *opened.OpenEpoch, Kind: "create-text", BaseRevision: "0", Title: "preserved", Body: "synthetic formal evidence"}, &candidate, 201)
	var receipt protocol.Receipt
	c.request(t, "POST", prefix+"/record-commits", protocol.CommitRecord{OperationId: newID(t), OpenEpoch: *opened.OpenEpoch, CandidateId: candidate.CandidateId, CandidateDigest: candidate.CandidateDigest, PreviewVersion: candidate.PreviewVersion}, &receipt, 200)
	closePublic(t, c, opened)
	p.stop(t)
	baseline := saveInspectionBytes(t, root)
	baselineTree := tree(t, root)
	bRevision := 0
	rows := badDirectoryCases()
	type evidence struct {
		Name           string
		Inspect, Open  int
		Code, OpenCode string
		Before, After  map[string]string
	}
	results := []evidence{}
	reuseSafe := true
	for _, r := range rows {
		t.Run(r.Name, func(t *testing.T) {
			reuseSafe = false
			stopped, watchJoined := false, false
			defer func() {
				if !stopped || !watchJoined || t.Failed() {
					return
				} // preserve failed/unended source; no later reuse
				restoreInspectionBytes(t, root, baseline)
				if !reflect.DeepEqual(baselineTree, tree(t, root)) {
					t.Error("fixture did not restore original directory inventory/bytes")
					return
				}
				reuseSafe = true
			}()
			applyBadDirectoryCase(t, f, root, r)
			p = f.start(t)
			c = p.connect(t)
			c.open(t, b.WorkspaceId)
			before := tree(t, root)
			watch := startInspectionWatch(t, root)
			watchAttempted := false
			finishWatch := func() { finishInspectionOnce(&watchAttempted, &watchJoined, func() { watch.finish(t, false) }) }
			defer finishWatch() // first normal native completion only, before fixture restore
			var denied protocol.ErrorView
			var status int
			if r.Want == "" {
				// A valid unavailable business state is not corrupt storage. Inspect
				// proves identity/integrity only; writable open must still refuse it.
				var valid protocol.InspectionView
				status = c.request(t, "POST", "/v1/workspace-inspections", protocol.InspectWorkspace{Path: protocol.Path(root)}, &valid, 200)
				if valid.WorkspaceId != a.WorkspaceId || valid.Integrity != "valid" || valid.AccessMode != "none" {
					t.Fatal("read-only valid storage evidence fabricated business readiness")
				}
			} else {
				status = c.request(t, "POST", "/v1/workspace-inspections", protocol.InspectWorkspace{Path: protocol.Path(root)}, &denied, 0)
				if status < 400 || denied.Code != r.Want {
					t.Fatalf("inspection %s status=%d code=%s want=%s", r.Name, status, denied.Code, r.Want)
				}
			}
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("inspection changed bad source")
			}
			pre := registrationPrecondition(t, c, a.WorkspaceId, "open")
			var failure protocol.ErrorView
			openStatus := c.request(t, "POST", prefix+"/open", protocol.OpenWorkspace{ExpectedRegistrationVersion: protocol.PositiveRevision(pre.RegistrationVersion)}, &failure, 0)
			wantOpen := protocol.ErrorCodeRECOVERYREQUIRED
			if r.SQL || r.Name == "zero-database" {
				wantOpen = protocol.ErrorCodeUNSUPPORTEDSCHEMA
			}
			switch r.Name {
			case "future-format", "legacy-v1", "future-digest-version":
				wantOpen = protocol.ErrorCodeUNSUPPORTEDFORMAT
			case "digest-mismatch":
				wantOpen = protocol.ErrorCodeDIGESTMISMATCH
			case "unknown-field", "duplicate-field", "malformed-descriptor":
				wantOpen = protocol.ErrorCodeINCOMPLETEDIRECTORY
			}
			if openStatus != 409 || failure.Code != wantOpen {
				t.Fatalf("storage rejection not established: %s status=%d code=%s want=%s", r.Name, openStatus, failure.Code, wantOpen)
			}
			after := tree(t, root)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("open repaired/replaced source", r.Name)
			}
			finishWatch()
			var other protocol.WorkspaceView
			bPrefix := "/v1/workspaces/" + string(b.WorkspaceId)
			c.request(t, "GET", bPrefix, nil, &other, 200)
			if other.OpenEpoch == nil || other.Revision == nil || string(*other.Revision) != strconv.Itoa(bRevision) {
				t.Fatal("local bad directory fenced healthy B")
			}
			var bp protocol.CandidateView
			body := "independent B commit during " + r.Name
			c.request(t, "POST", bPrefix+"/record-previews", protocol.PreviewRecord{OpenEpoch: *other.OpenEpoch, Kind: "create-text", BaseRevision: *other.Revision, Title: "independent B", Body: protocol.Body(body)}, &bp, 201)
			var br protocol.Receipt
			c.request(t, "POST", bPrefix+"/record-commits", protocol.CommitRecord{OperationId: newID(t), OpenEpoch: *other.OpenEpoch, CandidateId: bp.CandidateId, CandidateDigest: bp.CandidateDigest, PreviewVersion: bp.PreviewVersion}, &br, 200)
			var record protocol.TextRecordView
			c.request(t, "GET", bPrefix+"/records/"+string(br.ResourceId), nil, &record, 200)
			bRevision++
			if string(record.Body) != body || record.OperationId != br.OperationId || string(br.NewRevision) != strconv.Itoa(bRevision) {
				t.Fatal("B did not independently save/read current formal bytes")
			}
			p.stop(t)
			stopped = true // ALL native product resources end before source restoration
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("Host shutdown rewrote rejected source")
			}
			results = append(results, evidence{r.Name, status, openStatus, string(denied.Code), string(failure.Code), before, after})
		})
		if !reuseSafe {
			t.Fatal("failed or unjoined specimen retained; refusing fixture reuse")
		}
	}
	// A real positive control after restoring FIXTURES, not product recovery.
	p = f.start(t)
	c = p.connect(t)
	reopened := c.open(t, a.WorkspaceId)
	if reopened.Revision == nil || *reopened.Revision != "1" {
		t.Fatal("fixture restore control")
	}
	var actual protocol.TextRecordView
	c.request(t, "GET", prefix+"/records/"+string(receipt.ResourceId), nil, &actual, 200)
	if actual.Body != "synthetic formal evidence" || actual.OperationId != receipt.OperationId {
		t.Fatal("formal bytes changed")
	}
	p.stop(t)
	matrixEvidence(t, f, "bad-directory-matrix", results)
	t.Log("external production Host classifications, source recursive bytes/inventory and open refusal; not yet source attempted-write/global external-call trace")
}

func TestV009IndependentInstallationLedgersRefuseStartup(t *testing.T) {
	for _, variant := range []string{"ledger-missing", "ledger-checksum", "ledger-order", "ledger-state", "ledger-version", "user-version-future", "user-version-old", "schema-metadata"} {
		t.Run(variant, func(t *testing.T) {
			f := newFixture(t)
			f.initialize(t)
			mutateBadSQL(t, f, f.config.InstallationRoot, "installation", variant)
			before := tree(t, f.config.InstallationRoot)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			out := runObservedCLI(ctx, f, "serve")
			if ctx.Err() != nil || !out.IdentityProven || !out.Unavailable || !out.SafeOutput || out.OutputBytes != 0 {
				t.Fatal("malformed installation became ready or unsafe", out)
			}
			if !reflect.DeepEqual(before, tree(t, f.config.InstallationRoot)) {
				t.Fatal("installation open repaired/reseeded invalid history")
			}
			matrixEvidence(t, f, "invalid-installation-ledger", out)
		})
	}
}
