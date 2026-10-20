package workspace

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrOpen          = errors.New("workspace existing-open validation rejected")
	ErrOpenUncertain = errors.New("workspace retained resources or persistence require investigation")
	ErrStaleHandle   = errors.New("workspace epoch is no longer current")
)

// OpenResources is a trusted adapter, not a client capability. Close must wait
// for physical completion and may not release exclusion while results are unknown.
type OpenResources interface {
	TextFacts
	Validate() error
	Close() error
}
type OpenEvidence struct {
	Resources          OpenResources
	Snapshot           Snapshot
	NeedsInvestigation bool
	// All durable save IDs from the same validated history, not result or rights data.
	SaveOperationIDs []string
}
type OpenView struct {
	Scope              Scope
	Snapshot           Snapshot
	Epoch              string
	NeedsInvestigation bool
}

// OpenOwner retains the same lifecycle used by directory binding. Its mutex is
// workspace-local: neither slow A IO nor cleanup holds installation admission.
// No SQL/path/lease is returned to callers or selected from a current-workspace global.
type OpenOwner struct {
	mu        sync.Mutex
	scope     Scope
	lifecycle *Lifecycle
	resources OpenResources
	view      OpenView
	queue     *operationQueue
	closeMu   sync.Mutex
	closes    map[string]*closeOperation
	// Denial-only kind index, guarded by closeMu. Persistence still owns results.
	saveKeys map[string]struct{}
	closing  *closeOperation
}

func NewOpenOwner(scope Scope, lifecycle *Lifecycle) (*OpenOwner, error) {
	return NewOpenOwnerWithCapacity(scope, lifecycle, NewOperationCapacity())
}
func NewOpenOwnerWithCapacity(scope Scope, lifecycle *Lifecycle, capacity *OperationCapacity) (*OpenOwner, error) {
	if scope.Validate() != nil || lifecycle == nil || capacity == nil {
		return nil, ErrScope
	}
	return &OpenOwner{scope: scope, lifecycle: lifecycle, queue: newOperationQueue(capacity), closes: make(map[string]*closeOperation), saveKeys: make(map[string]struct{})}, nil
}
func newEpoch() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// Open calls the trusted existing-only factory after admission. The factory must
// return retained resources even on failure if physical cleanup is uncertain.
// authorize is rechecked before both starting IO and publishing a fresh epoch.
func (o *OpenOwner) Open(ctx context.Context, scope Scope, authorize func(context.Context) error, factory func(context.Context, string) (OpenEvidence, error)) (OpenView, error) {
	ticket, admissionErr := o.beginOperation(ctx, scope, authorize, false, true)
	if admissionErr != nil {
		return OpenView{}, admissionErr
	}
	defer ticket.finish()
	o.mu.Lock()
	defer o.mu.Unlock()
	if scope != o.scope {
		return OpenView{}, ErrScope
	}
	if authorize == nil || factory == nil {
		return OpenView{}, ErrOpen
	}
	if err := authorize(ctx); err != nil {
		return OpenView{}, err
	}
	if err := ctx.Err(); err != nil {
		return OpenView{}, err
	}
	if err := o.lifecycle.BeginOpen(scope); err != nil {
		return OpenView{}, err
	}
	epoch, err := newEpoch()
	var evidence OpenEvidence
	if err == nil {
		evidence, err = factory(ctx, epoch)
	}
	o.resources = evidence.Resources
	if err == nil && (evidence.Resources == nil || evidence.Snapshot.Identity.WorkspaceID != scope.WorkspaceID || evidence.Snapshot.Identity.TenantID != scope.TenantID || evidence.Snapshot.Identity.BusinessState != "ready") {
		err = ErrOpen
	}
	if err == nil {
		err = evidence.Resources.Validate()
	}
	if err == nil {
		err = authorize(ctx)
	}
	if err == nil {
		err = ctx.Err()
	}
	var keys map[string]struct{}
	if err == nil {
		keys, err = o.validatedSaveKeys(evidence.SaveOperationIDs)
	}
	if err != nil {
		_ = o.lifecycle.BeginDrain(scope)
		var end error
		if o.resources != nil {
			end = o.resources.Close()
		}
		outcome := DrainSucceeded
		if end != nil {
			ticket.retain("open")
			outcome = DrainUnknown
		} else {
			o.resources = nil
		}
		finish := o.lifecycle.FinishDrain(scope, outcome)
		return OpenView{}, errors.Join(err, end, finish)
	}
	if err = o.lifecycle.MarkOpen(scope); err != nil {
		return OpenView{}, err
	}
	o.view = OpenView{Scope: scope, Snapshot: evidence.Snapshot, Epoch: epoch, NeedsInvestigation: evidence.NeedsInvestigation}
	o.closeMu.Lock()
	o.saveKeys = keys
	o.closeMu.Unlock()
	o.queue.mu.Lock()
	o.queue.epoch = epoch
	// An explicit open can have queued before a cold investigation physically
	// retired its predecessor. Publish a new generation's flags here as well as
	// at enqueue; never undo process shutdown's irreversible admission cutoff.
	o.queue.closed = false
	if !o.queue.stopping {
		o.queue.closing = false
	}
	o.queue.mu.Unlock()
	return o.view, nil
}

// DrainOpen is the physical primitive, not a public close operation receipt.
// T021 adds queue/accepted-operation coordination; uncertain results retain all
// exclusion resources, rather than laundering connection closure into rollback.
func (o *OpenOwner) DrainOpen(ctx context.Context, scope Scope, authorize func(context.Context) error) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if scope != o.scope {
		return ErrScope
	}
	if authorize == nil {
		return ErrOpen
	}
	if err := authorize(ctx); err != nil {
		return err
	}
	return o.drainOwnedResources(scope)
}

// drainOwnedResources requires o.mu and a previously checked current authority.
// It is shared by explicit close and the narrow cold-investigation handoff; an
// empty owner never gains authority to clear another coordinator's fence.
func (o *OpenOwner) drainOwnedResources(scope Scope) error {
	// A binding coordinator can fence the shared lifecycle while retaining its
	// own resources. An empty open owner has no evidence to clear that fence.
	if o.resources == nil {
		return ErrLifecycle
	}
	if err := o.lifecycle.BeginDrain(scope); err != nil {
		return err
	}
	var err error
	if o.resources != nil {
		err = o.resources.Close()
	}
	if err != nil {
		return errors.Join(err, o.lifecycle.FinishDrain(scope, DrainUnknown))
	}
	o.resources = nil
	o.view = OpenView{}
	if err := o.lifecycle.FinishDrain(scope, DrainSucceeded); err != nil {
		return err
	}
	o.queue.mu.Lock()
	o.queue.epoch = ""
	o.queue.mu.Unlock()
	o.queue.ended()
	return nil
}
