//go:build windows && amd64

package inspect

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	win "golang.org/x/sys/windows"
)

var ErrUnsupportedSchema = errors.New("workspace schema unsupported")
var ErrInspectionScope = errors.New("directory tenant scope denied")

// DirectoryReport is internal disk evidence, never an authorization or DTO.
// ScopeEstablished does not outlive this request's current policy rechecks.
type DirectoryReport struct {
	Storage          Report
	Descriptor       workspace.DirectoryDescriptor
	ScopeEstablished bool
}

func frozenDescriptorBytes(ctx context.Context, source *platform.File) (data []byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = source.Validate(); err != nil {
		return nil, err
	}
	var duplicate win.Handle
	process := win.CurrentProcess()
	if err = win.DuplicateHandle(process, source.Handle(), process, &duplicate, 0, false, win.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "frozen-descriptor")
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	data, err = io.ReadAll(io.LimitReader(file, int64(protocol.RequestMaxBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > protocol.RequestMaxBytes {
		return nil, workspace.ErrDirectoryDescriptor
	}
	return data, source.Validate()
}

// InspectDirectory requires both a trusted expected tenant and a mandatory
// current-authorization recheck. It cannot opt out of the metadata scope gate.
func InspectDirectory(ctx context.Context, root, scratchParent, expectedTenant string, checkCurrent func(context.Context) error) (DirectoryReport, error) {
	var descriptor workspace.DirectoryDescriptor
	known := false
	if _, err := workspace.NewDirectoryDescriptor(expectedTenant, expectedTenant); err != nil || checkCurrent == nil {
		return DirectoryReport{}, ErrInspectionScope
	}
	if err := checkCurrent(ctx); err != nil {
		return DirectoryReport{}, err
	}
	report, err := inspectValidated(ctx, root, scratchParent, os.RemoveAll, func(ctx context.Context, file *platform.File) error {
		data, err := frozenDescriptorBytes(ctx, file)
		if err != nil {
			return err
		}
		value, err := protocol.ParseJSON(data)
		if err != nil {
			return workspace.ErrDirectoryDescriptor
		}
		object, ok := value.(map[string]any)
		if !ok {
			return workspace.ErrDirectoryDescriptor
		}
		tenant, ok := object["tenantId"].(string)
		if !ok {
			return workspace.ErrDirectoryDescriptor
		}
		if _, err = workspace.NewDirectoryDescriptor(tenant, tenant); err != nil {
			return workspace.ErrDirectoryDescriptor
		}
		// Match the precise top-level key before digest/format diagnostics: even
		// a foreign descriptor with a bad digest must expose no health summary.
		if tenant != expectedTenant {
			return ErrInspectionScope
		}
		known = true
		if err = checkCurrent(ctx); err != nil {
			return err
		}
		descriptor, err = workspace.ParseDirectoryDescriptor(data)
		return err
	}, func(ctx context.Context, scratch string, conn *sql.Conn) error {
		data, err := os.ReadFile(filepath.Join(scratch, "descriptor.json"))
		if err != nil {
			return err
		}
		copied, err := workspace.ParseDirectoryDescriptor(data)
		if err != nil {
			return err
		}
		if copied != descriptor {
			return workspace.ErrDirectoryIdentityMismatch
		}
		// Only tenant metadata is queried before any integrity/schema diagnosis.
		// A forged same-tenant descriptor cannot grant access to foreign DB data.
		if err = sqlite.ValidateDirectoryTenant(ctx, conn, expectedTenant); err != nil {
			return err
		}
		var schemaVersion int
		if err = conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&schemaVersion); err != nil {
			return err
		}
		if schemaVersion != 1 {
			return ErrUnsupportedSchema
		}
		return sqlite.ValidateDirectorySnapshot(ctx, conn, descriptor)
	})
	if checkErr := checkCurrent(ctx); checkErr != nil {
		return DirectoryReport{Storage: report, ScopeEstablished: known}, checkErr
	}
	if err != nil {
		return DirectoryReport{Storage: report, ScopeEstablished: known}, err
	}
	return DirectoryReport{Storage: report, Descriptor: descriptor, ScopeEstablished: known}, nil
}
