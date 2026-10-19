// Package digest implements specs/001-host-workspace-foundation/data-model.md §4.
// These inputs are internal hash layouts, not a second public HTTP contract.
// Hashing establishes byte identity only, never authorization, filesystem safety,
// database integrity or proof of an operation's commit/rollback.
package digest

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

type IdentityInput struct {
	FormatVersion        uint64        `json:"formatVersion"`
	WorkspaceID          protocol.UUID `json:"workspaceId"`
	TenantID             protocol.UUID `json:"tenantId"`
	DatabaseRelativePath string        `json:"databaseRelativePath"`
}
type ContentInput struct {
	WorkspaceID protocol.UUID  `json:"workspaceId"`
	ResourceID  protocol.UUID  `json:"resourceId"`
	Title       protocol.Title `json:"title"`
	Body        protocol.Body  `json:"body"`
}
type CandidateInput struct {
	WorkspaceID    protocol.UUID              `json:"workspaceId"`
	PrincipalID    protocol.UUID              `json:"principalId"`
	CandidateID    protocol.UUID              `json:"candidateId"`
	ResourceID     protocol.UUID              `json:"resourceId"`
	Kind           protocol.PreviewRecordKind `json:"kind"`
	BaseRevision   protocol.Revision          `json:"baseRevision"`
	BaseDigest     *protocol.Digest           `json:"baseDigest,omitempty"`
	ContentDigest  protocol.Digest            `json:"contentDigest"`
	PreviewVersion protocol.PositiveRevision  `json:"previewVersion"`
}
type SaveInput struct {
	InstallationID  protocol.UUID   `json:"installationId"`
	TenantID        protocol.UUID   `json:"tenantId"`
	PrincipalID     protocol.UUID   `json:"principalId"`
	WorkspaceID     protocol.UUID   `json:"workspaceId"`
	OperationID     protocol.UUID   `json:"operationId"`
	CandidateID     protocol.UUID   `json:"candidateId"`
	CandidateDigest protocol.Digest `json:"candidateDigest"`
}
type InstallInput struct {
	InstallationID      protocol.UUID      `json:"installationId"`
	PrincipalID         protocol.UUID      `json:"principalId"`
	OperationID         protocol.UUID      `json:"operationId"`
	Kind                string             `json:"kind"`
	WorkspaceID         protocol.UUID      `json:"workspaceId"`
	CanonicalLocator    string             `json:"canonicalLocator"`
	RegistrationVersion *protocol.Revision `json:"registrationVersion,omitempty"`
}

type encoder struct {
	bytes []byte
	err   error
}

func domain(s string) *encoder { return &encoder{bytes: append([]byte(s), 0)} }
func (e *encoder) fail() {
	if e.err == nil {
		e.err = fmt.Errorf("invalid digest input")
	}
}
func valid(schema, s string) bool {
	// Marshal cannot be trusted to preserve invalid UTF-8: check before encoding.
	if !utf8.ValidString(s) || strings.ContainsRune(s, 0) || strings.HasPrefix(s, "\ufeff") {
		return false
	}
	b, err := json.Marshal(s)
	return err == nil && protocol.ValidateJSON(schema, b) == nil
}
func (e *encoder) u(n uint64) {
	if n > uint64(1<<63-1) {
		e.fail()
		return
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], n)
	e.bytes = append(e.bytes, b[:]...)
}
func (e *encoder) revision(r protocol.Revision, positive bool) {
	n, err := protocol.RevisionUint64(r)
	if err != nil || (positive && n == 0) {
		e.fail()
		return
	}
	e.u(n)
}
func (e *encoder) s(s string) {
	if !utf8.ValidString(s) || strings.ContainsRune(s, 0) || strings.HasPrefix(s, "\ufeff") {
		e.fail()
		return
	}
	e.u(uint64(len(s)))
	e.bytes = append(e.bytes, []byte(s)...)
}
func (e *encoder) uuid(id protocol.UUID) {
	if !valid("UUID", string(id)) {
		e.fail()
		return
	}
	b, err := hex.DecodeString(strings.ReplaceAll(string(id), "-", ""))
	if err != nil {
		e.fail()
		return
	}
	e.bytes = append(e.bytes, b...)
}
func (e *encoder) d(d protocol.Digest) {
	if !valid("Digest", string(d)) {
		e.fail()
		return
	}
	b, err := hex.DecodeString(string(d))
	if err != nil {
		e.fail()
		return
	}
	e.bytes = append(e.bytes, b...)
}
func (e *encoder) result() ([]byte, error) {
	if e.err != nil {
		return nil, e.err
	}
	return e.bytes, nil
}
func sum(b []byte, err error) (protocol.Digest, error) {
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return protocol.Digest(hex.EncodeToString(h[:])), nil
}

func IdentityBytes(in IdentityInput) ([]byte, error) {
	e := domain("ABOARD/IDENTITY/1")
	if in.DatabaseRelativePath != "data/workspace.sqlite" {
		e.fail()
	}
	e.u(in.FormatVersion)
	e.uuid(in.WorkspaceID)
	e.uuid(in.TenantID)
	e.s(in.DatabaseRelativePath)
	return e.result()
}
func Identity(in IdentityInput) (protocol.Digest, error) { return sum(IdentityBytes(in)) }
func ContentBytes(in ContentInput) ([]byte, error) {
	e := domain("ABOARD/TEXT/1")
	if !valid("Title", string(in.Title)) || !valid("Body", string(in.Body)) {
		e.fail()
	}
	e.uuid(in.WorkspaceID)
	e.uuid(in.ResourceID)
	e.s(string(in.Title))
	e.s(string(in.Body))
	return e.result()
}
func Content(in ContentInput) (protocol.Digest, error) { return sum(ContentBytes(in)) }
func CandidateBytes(in CandidateInput) ([]byte, error) {
	e := domain("ABOARD/CANDIDATE/1")
	if !valid("PreviewRecordKind", string(in.Kind)) {
		e.fail()
	}
	if (in.Kind == "create-text" && in.BaseDigest != nil) || (in.Kind == "update-text" && in.BaseDigest == nil) {
		e.fail()
	}
	e.uuid(in.WorkspaceID)
	e.uuid(in.PrincipalID)
	e.uuid(in.CandidateID)
	e.uuid(in.ResourceID)
	e.s(string(in.Kind))
	e.revision(in.BaseRevision, false)
	if in.BaseDigest == nil {
		e.bytes = append(e.bytes, 0)
	} else {
		e.bytes = append(e.bytes, 1)
		e.d(*in.BaseDigest)
	}
	e.d(in.ContentDigest)
	e.revision(protocol.Revision(in.PreviewVersion), true)
	return e.result()
}
func Candidate(in CandidateInput) (protocol.Digest, error) { return sum(CandidateBytes(in)) }
func SaveBytes(in SaveInput) ([]byte, error) {
	e := domain("ABOARD/SAVE/1")
	e.uuid(in.InstallationID)
	e.uuid(in.TenantID)
	e.uuid(in.PrincipalID)
	e.uuid(in.WorkspaceID)
	e.uuid(in.OperationID)
	e.uuid(in.CandidateID)
	e.d(in.CandidateDigest)
	return e.result()
}
func Save(in SaveInput) (protocol.Digest, error) { return sum(SaveBytes(in)) }
func InstallBytes(in InstallInput) ([]byte, error) {
	e := domain("ABOARD/INSTALL-OP/1")
	if in.Kind != "create" && in.Kind != "register" && in.Kind != "relink" {
		e.fail()
	}
	if in.CanonicalLocator == "" {
		e.fail()
	}
	e.uuid(in.InstallationID)
	e.uuid(in.PrincipalID)
	e.uuid(in.OperationID)
	e.s(in.Kind)
	e.uuid(in.WorkspaceID)
	e.s(in.CanonicalLocator)
	if in.RegistrationVersion == nil {
		e.bytes = append(e.bytes, 0)
	} else {
		e.bytes = append(e.bytes, 1)
		e.revision(*in.RegistrationVersion, false)
	}
	return e.result()
}
func Install(in InstallInput) (protocol.Digest, error) { return sum(InstallBytes(in)) }
