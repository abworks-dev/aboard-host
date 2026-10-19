package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/abworks-dev/aboard-host/internal/digest"
	"github.com/abworks-dev/aboard-host/internal/protocol"
)

// ReadCurrentInstallationOperation reads the common dispatch fact without a
// create-specific join. Authorization and original actor are checked within the
// same read transaction. A row (or its absence) never authorizes replay or IO.
// Locators are private evidence: the business owner MUST apply current allowed
// location policy before exposing its path-free view to a caller.
func (d *Database) ReadCurrentInstallationOperation(ctx context.Context, hash string, version int64, installationID, principalID, operationID string) (out InstallationOperation, err error) {
	if d == nil || d.kind != InstallationDatabase || !validUUID(installationID) || !validUUID(principalID) || !validUUID(operationID) {
		return out, ErrInstallationOperation
	}
	tx, err := d.h.Conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer func() {
		err = errors.Join(err, tx.Rollback())
		if err != nil {
			out = InstallationOperation{}
		}
	}()
	if err = creationOperator(ctx, tx, hash, version, installationID, principalID); err != nil {
		return out, err
	}
	out, err = readInstallationOperation(ctx, tx, installationID, operationID)
	if err != nil {
		return out, err
	}
	if out.PrincipalID != principalID {
		return out, ErrAuthorization
	}
	expected, digestErr := digest.Install(digest.InstallInput{InstallationID: protocol.UUID(out.InstallationID), PrincipalID: protocol.UUID(out.PrincipalID), OperationID: protocol.UUID(out.OperationID), Kind: out.Kind, WorkspaceID: protocol.UUID(out.WorkspaceID), CanonicalLocator: out.TargetLocator})
	if digestErr != nil || out.InputDigest != string(expected) {
		return out, ErrInstallationOperation
	}
	return out, nil
}
