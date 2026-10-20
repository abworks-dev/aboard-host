//go:build windows && amd64

package installation

import (
	"context"
	"errors"
	"sync"

	"github.com/abworks-dev/aboard-host/internal/workspace"
	inspection "github.com/abworks-dev/aboard-host/internal/workspace/inspect"
)

var ErrInspectionPending = errors.New("inspection cleanup not drained; installation lease retained")

// Not an operation-result cache: inspection never writes formal state. This
// owner retains the original accounting and native cleanup until actual end.
type inspectionOwner struct {
	mu          sync.Mutex
	store       *Store
	resources   *inspection.Resources
	failure     error // captured factory close-unknown without a returned resource
	reservation *workspace.OperationReservation
}

func (i *DirectoryInspector) beginInspection(ctx context.Context, a *Authorization) (*inspectionOwner, error) {
	reservation, err := i.store.ReserveOperation(ctx, a)
	if err != nil {
		return nil, err
	}
	owner := &inspectionOwner{store: i.store, reservation: reservation}
	owner.mu.Lock() // publish only with the IO owner held; drain cannot race its run
	i.store.mu.Lock()
	if err = a.checkLocked(ctx); err == nil && i.store.workspaceStopping {
		err = workspace.ErrLifecycle
	}
	if err == nil {
		if i.store.inspections == nil {
			i.store.inspections = make(map[*inspectionOwner]struct{})
		}
		i.store.inspections[owner] = struct{}{}
	}
	i.store.mu.Unlock()
	if err != nil {
		owner.mu.Unlock()
		reservation.Release()
		return nil, err
	}
	return owner, nil
}

// Caller holds owner.mu, never Store.mu across native Close/deletion.
func (o *inspectionOwner) closeLocked() error {
	if o.failure != nil {
		return errors.Join(ErrInspectionPending, o.failure)
	}
	if err := o.resources.Close(); err != nil {
		return errors.Join(ErrInspectionPending, err)
	}
	o.resources = nil
	o.store.mu.Lock()
	delete(o.store.inspections, o)
	o.reservation.Release()
	o.reservation = nil
	o.store.mu.Unlock()
	return nil
}

// DrainInspections is trusted Host teardown, not a public repair API. Repeats
// only the original cleanup chain (never source SQL/recovery/business replay).
// Sticky native/driver failure remains fenced; a known deletion refusal may end
// after the blocker is removed. A waiter cannot close a still-running inspector.
func (s *Store) DrainInspections() error {
	s.mu.Lock()
	owners := make([]*inspectionOwner, 0, len(s.inspections))
	for owner := range s.inspections {
		owners = append(owners, owner)
	}
	s.mu.Unlock()
	var errs []error
	for _, owner := range owners {
		owner.mu.Lock()
		errs = append(errs, owner.closeLocked())
		owner.mu.Unlock()
	}
	return errors.Join(errs...)
}
