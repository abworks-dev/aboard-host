//go:build windows && amd64

package installation

import (
	"context"
	"errors"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	"github.com/abworks-dev/aboard-host/internal/workspace"
	inspection "github.com/abworks-dev/aboard-host/internal/workspace/inspect"
	win "golang.org/x/sys/windows"
)

// InspectionFailure carries a closed contract code, never a native error/path.
// Internal low-level diagnostics must not be serialized as a browser DTO.
type InspectionFailure struct{ Code protocol.ErrorCode }

func (e *InspectionFailure) Error() string { return "workspace inspection rejected: " + string(e.Code) }

type DirectoryInspection struct {
	WorkspaceID, TenantID string
	FormatVersion         int
	ScratchCleanupFailed  bool
}

// DirectoryInspector uses host configuration for scratch, never request input.
// It has no write, registration or runtime activation capability.
type DirectoryInspector struct {
	store         *Store
	scratchParent string
}

func NewDirectoryInspector(store *Store, privateScratchParent string) (*DirectoryInspector, error) {
	if store == nil || platform.CheckPrivateDirectory(privateScratchParent) != nil {
		return nil, &InspectionFailure{protocol.ErrorCodeINSPECTIONUNAVAILABLE}
	}
	return &DirectoryInspector{store, privateScratchParent}, nil
}

func sharingFailure(err error) bool {
	if errors.Is(err, win.ERROR_SHARING_VIOLATION) {
		return true
	}
	var status win.NTStatus
	return errors.As(err, &status) && status == win.STATUS_SHARING_VIOLATION
}

func inspectionCode(err error) protocol.ErrorCode {
	switch {
	case errors.Is(err, workspace.ErrUnsupportedDirectoryFormat):
		return protocol.ErrorCodeUNSUPPORTEDFORMAT
	case errors.Is(err, workspace.ErrDirectoryDigestMismatch):
		return protocol.ErrorCodeDIGESTMISMATCH
	case errors.Is(err, workspace.ErrDirectoryIdentityMismatch):
		return protocol.ErrorCodeIDENTITYCONFLICT
	case errors.Is(err, workspace.ErrDirectoryDescriptor), errors.Is(err, inspection.ErrIncompleteDirectory):
		return protocol.ErrorCodeINCOMPLETEDIRECTORY
	case errors.Is(err, inspection.ErrMissingDatabase):
		return protocol.ErrorCodeMISSINGDATABASE
	case errors.Is(err, inspection.ErrRecoveryRequired):
		return protocol.ErrorCodeRECOVERYREQUIRED
	case errors.Is(err, inspection.ErrUnsupportedSchema):
		return protocol.ErrorCodeUNSUPPORTEDSCHEMA
	case errors.Is(err, inspection.ErrCorruptDatabase), errors.Is(err, sqlite.ErrSchema):
		return protocol.ErrorCodeCORRUPTDATABASE
	case sharingFailure(err):
		return protocol.ErrorCodeWRITERBUSY
	}
	var sqliteCode interface{ Code() int }
	if errors.As(err, &sqliteCode) && (sqliteCode.Code()&255 == 11 || sqliteCode.Code()&255 == 26) {
		return protocol.ErrorCodeCORRUPTDATABASE
	}
	return protocol.ErrorCodeINSPECTIONUNAVAILABLE
}

func (i *DirectoryInspector) Inspect(ctx context.Context, operator *Authorization, root string) (DirectoryInspection, error) {
	if i == nil || operator == nil || operator.auth == nil || operator.auth.store != i.store {
		return DirectoryInspection{}, ErrUnauthorized
	}
	// Reject before touching even an unregistered source. A path never grants
	// the caller identity, installation.operator or an allowed-root capability.
	if err := operator.CheckLocation(ctx, root); err != nil {
		return DirectoryInspection{}, err
	}
	observed, err := inspection.InspectDirectory(ctx, root, i.scratchParent, operator.tenantID, func(ctx context.Context) error { return operator.CheckLocation(ctx, root) })
	if checkErr := operator.CheckLocation(ctx, root); checkErr != nil {
		return DirectoryInspection{}, checkErr
	}
	if errors.Is(err, inspection.ErrInspectionScope) || errors.Is(err, workspace.ErrDirectoryTenantMismatch) {
		return DirectoryInspection{}, ErrUnauthorized
	}
	if !observed.ScopeEstablished {
		// No tenant proof: never expose existence, DB health, sharing state or
		// cleanup detail. The low-level internal report retains real cleanup data.
		return DirectoryInspection{}, &InspectionFailure{protocol.ErrorCodeINSPECTIONUNAVAILABLE}
	}
	if err != nil {
		return DirectoryInspection{ScratchCleanupFailed: observed.Storage.ScratchCleanupFailed}, &InspectionFailure{inspectionCode(err)}
	}
	if observed.Descriptor.TenantID != operator.tenantID {
		return DirectoryInspection{}, ErrUnauthorized
	}
	return DirectoryInspection{observed.Descriptor.WorkspaceID, observed.Descriptor.TenantID, observed.Descriptor.FormatVersion, observed.Storage.ScratchCleanupFailed}, nil
}
