package workspace

import (
	"encoding/json"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

var (
	ErrDirectoryDescriptor        = errors.New("directory descriptor invalid")
	ErrUnsupportedDirectoryFormat = errors.New("directory format unsupported")
	ErrDirectoryDigestMismatch    = errors.New("directory identity digest mismatch")
	ErrDirectoryIdentityMismatch  = errors.New("directory identity differs from database")
	ErrDirectoryTenantMismatch    = errors.New("directory database tenant scope denied")
)

// DirectoryDescriptor is the single in-process representation of the six
// identity-only fields. It is neither a permission nor a business-state owner.
type DirectoryDescriptor struct {
	FormatVersion         int    `json:"formatVersion"`
	WorkspaceID           string `json:"workspaceId"`
	TenantID              string `json:"tenantId"`
	DatabaseRelativePath  string `json:"databaseRelativePath"`
	IdentityDigestVersion int    `json:"identityDigestVersion"`
	IdentityDigest        string `json:"identityDigest"`
}

func NewDirectoryDescriptor(workspaceID, tenantID string) (DirectoryDescriptor, error) {
	h, err := digest.Identity(digest.IdentityInput{FormatVersion: 2, WorkspaceID: protocol.UUID(workspaceID), TenantID: protocol.UUID(tenantID), DatabaseRelativePath: "data/workspace.sqlite"})
	if err != nil {
		return DirectoryDescriptor{}, ErrDirectoryDescriptor
	}
	return DirectoryDescriptor{2, workspaceID, tenantID, "data/workspace.sqlite", 1, string(h)}, nil
}

func (d DirectoryDescriptor) Validate() error {
	if d.FormatVersion != 2 || d.IdentityDigestVersion != 1 {
		return ErrUnsupportedDirectoryFormat
	}
	want, err := NewDirectoryDescriptor(d.WorkspaceID, d.TenantID)
	if err != nil || d.DatabaseRelativePath != want.DatabaseRelativePath {
		return ErrDirectoryDescriptor
	}
	if d.IdentityDigest != want.IdentityDigest {
		return ErrDirectoryDigestMismatch
	}
	return nil
}

func ParseDirectoryDescriptor(data []byte) (DirectoryDescriptor, error) {
	// ParseJSON rejects duplicate keys, invalid UTF-8, BOM, lone surrogates,
	// NUL, excess nesting/size and trailing content before ordinary decoding.
	value, err := protocol.ParseJSON(data)
	if err != nil {
		return DirectoryDescriptor{}, ErrDirectoryDescriptor
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 6 {
		return DirectoryDescriptor{}, ErrDirectoryDescriptor
	}
	for _, key := range []string{"formatVersion", "workspaceId", "tenantId", "databaseRelativePath", "identityDigestVersion", "identityDigest"} {
		if _, exists := object[key]; !exists {
			return DirectoryDescriptor{}, ErrDirectoryDescriptor
		}
	}
	var d DirectoryDescriptor
	if err = json.Unmarshal(data, &d); err != nil {
		return DirectoryDescriptor{}, ErrDirectoryDescriptor
	}
	if err = d.Validate(); err != nil {
		return DirectoryDescriptor{}, err
	}
	return d, nil
}

func EncodeDirectoryDescriptor(d DirectoryDescriptor) ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(d)
}
