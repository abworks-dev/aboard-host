//go:build windows && amd64

package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// The production matrix and attributed observation use the SAME fixed specimens.
// Fixture mutations run only while A is closed, outside the measured window.
type badDirectoryCase struct {
	Name string
	Want protocol.ErrorCode
	SQL  bool
}

func badDirectoryCases() []badDirectoryCase {
	return []badDirectoryCase{
		{"missing-database", "MISSING_DATABASE", false}, {"zero-database", "CORRUPT_DATABASE", false}, {"corrupt-header", "CORRUPT_DATABASE", false},
		{"future-format", "UNSUPPORTED_FORMAT", false}, {"legacy-v1", "UNSUPPORTED_FORMAT", false}, {"future-digest-version", "UNSUPPORTED_FORMAT", false}, {"digest-mismatch", "DIGEST_MISMATCH", false}, {"forged-workspace", "IDENTITY_CONFLICT", false},
		{"unknown-field", "INCOMPLETE_DIRECTORY", false}, {"duplicate-field", "INSPECTION_UNAVAILABLE", false}, {"malformed-descriptor", "INSPECTION_UNAVAILABLE", false}, {"foreign-descriptor", "FORBIDDEN", false},
		{"missing-descriptor", "INSPECTION_UNAVAILABLE", false}, {"missing-lock", "INSPECTION_UNAVAILABLE", false},
		{"ledger-missing", "CORRUPT_DATABASE", true}, {"ledger-checksum", "CORRUPT_DATABASE", true}, {"ledger-order", "CORRUPT_DATABASE", true}, {"ledger-state", "CORRUPT_DATABASE", true}, {"ledger-version", "CORRUPT_DATABASE", true},
		{"user-version-future", "UNSUPPORTED_SCHEMA", true}, {"user-version-old", "UNSUPPORTED_SCHEMA", true}, {"schema-metadata", "CORRUPT_DATABASE", true}, {"business-unavailable", "", true},
		{"database-foreign-tenant", "FORBIDDEN", true}, {"database-forged-id", "IDENTITY_CONFLICT", true}, {"foreign-key-violation", "CORRUPT_DATABASE", true}, {"immutable-content-corrupt", "CORRUPT_DATABASE", true},
		{"journal-residue", "RECOVERY_REQUIRED", false}, {"wal-residue", "RECOVERY_REQUIRED", false}, {"shm-residue", "RECOVERY_REQUIRED", false},
	}
}
func badDirectoryOpenCode(r badDirectoryCase) protocol.ErrorCode {
	if strings.HasPrefix(r.Name, "initialization-") {
		if r.Want == "RECOVERY_REQUIRED" {
			return protocol.ErrorCodeRECOVERYREQUIRED
		}
		return protocol.ErrorCodeUNSUPPORTEDSCHEMA
	}
	if r.SQL || r.Name == "zero-database" {
		return protocol.ErrorCodeUNSUPPORTEDSCHEMA
	}
	switch r.Name {
	case "future-format", "legacy-v1", "future-digest-version":
		return protocol.ErrorCodeUNSUPPORTEDFORMAT
	case "digest-mismatch":
		return protocol.ErrorCodeDIGESTMISMATCH
	case "unknown-field", "duplicate-field", "malformed-descriptor":
		return protocol.ErrorCodeINCOMPLETEDIRECTORY
	default:
		return protocol.ErrorCodeRECOVERYREQUIRED
	}
}
func applyBadDirectoryCase(t *testing.T, f *fixture, root string, r badDirectoryCase) {
	t.Helper()
	db := filepath.Join(root, "data", "workspace.sqlite")
	switch {
	case r.Name == "healthy":
		return
	case r.SQL:
		mutateBadSQL(t, f, root, "workspace", r.Name)
	case r.Name == "missing-database":
		if err := os.Remove(db); err != nil {
			t.Fatal(err)
		}
	case r.Name == "zero-database":
		if err := os.WriteFile(db, nil, 0600); err != nil {
			t.Fatal(err)
		}
	case r.Name == "corrupt-header":
		if err := os.WriteFile(db, []byte("synthetic invalid SQLite header"), 0600); err != nil {
			t.Fatal(err)
		}
	case r.Name == "missing-descriptor":
		if err := os.Remove(filepath.Join(root, "aboard.workspace.json")); err != nil {
			t.Fatal(err)
		}
	case r.Name == "missing-lock":
		if err := os.Remove(filepath.Join(root, ".aboard", "locks", "writer.lock")); err != nil {
			t.Fatal(err)
		}
	case strings.HasSuffix(r.Name, "-residue"):
		suffix := map[string]string{"journal-residue": "-journal", "wal-residue": "-wal", "shm-residue": "-shm"}[r.Name]
		if err := os.WriteFile(db+suffix, []byte("synthetic nonrecoverable sidecar; not genuine hot journal"), 0600); err != nil {
			t.Fatal(err)
		}
	default:
		mutateBadDescriptor(t, root, r.Name)
	}
}
