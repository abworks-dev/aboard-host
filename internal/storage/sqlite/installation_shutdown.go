package sqlite

import "context"

// HasUnresolvedInstallationOperations is a lifecycle read under the installation
// owner's exclusion. Every actor and every kind matters; a missing joined intent
// must not hide unresolved history. The verified owner supplies installationID.
func (d *Database) HasUnresolvedInstallationOperations(ctx context.Context, installationID string) (bool, error) {
	if d == nil || d.kind != InstallationDatabase || !validUUID(installationID) {
		return false, ErrInstallationOperation
	}
	var pending bool
	err := d.h.Conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM installation_operation WHERE installation_id=? AND outcome IN ('accepted','unknown'))`, installationID).Scan(&pending)
	return pending, err
}
