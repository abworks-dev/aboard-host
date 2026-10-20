//go:build windows && amd64

package host

import (
	"context"
	"sync"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/installation/security"
)

type operatorServer struct {
	mu                      sync.Mutex
	auth                    *installation.Authenticator
	pipe                    *security.ControlPipe
	grant                   security.Grant
	paused, stopped, active bool
	cancel                  context.CancelFunc
	changed                 chan struct{}
	done                    chan struct{}
	progress                chan struct{}
	observe                 func(uint64, security.PipeDiagnostic) // consumed only after stop; never request/bearer
	observations            []operatorObservation                 // sole run writer; read only after done
	observationDropped      uint64
}

func newOperator(auth *installation.Authenticator, pipe *security.ControlPipe, principal string) *operatorServer {
	return &operatorServer{auth: auth, pipe: pipe, grant: security.Grant{PrincipalID: principal, Action: "installation.operator"}, changed: make(chan struct{}), done: make(chan struct{}), progress: make(chan struct{}, 1)}
}
func (o *operatorServer) signal() {
	close(o.changed)
	o.changed = make(chan struct{})
	select {
	case o.progress <- struct{}{}:
	default:
	}
}
func (o *operatorServer) run() {
	defer close(o.done)
	var attempt uint64
	for {
		o.mu.Lock()
		if o.stopped {
			o.mu.Unlock()
			return
		}
		if o.paused {
			changed := o.changed
			o.mu.Unlock()
			<-changed
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		o.cancel = cancel
		o.active = true
		o.signal()
		o.mu.Unlock()
		attempt++
		started := time.Now()
		err := o.auth.ServeOperator(ctx, o.pipe, o.grant, 5*time.Minute)
		o.recordObservation(attempt, security.DiagnosePipe(ctx, err, time.Since(started)))
		cancel()
		o.mu.Lock()
		o.cancel = nil
		o.active = false
		o.signal()
		paused := o.paused || o.stopped
		o.mu.Unlock()
		// A denied/disconnected client is not a new pipe or a retry of its session.
		// Bound accept-error spinning without weakening the original first instance.
		if err != nil && !paused {
			timer := time.NewTimer(50 * time.Millisecond)
			<-timer.C
		}
	}
}
func (o *operatorServer) pause(ctx context.Context) error {
	o.mu.Lock()
	o.paused = true
	if o.cancel != nil {
		o.cancel()
	}
	o.signal()
	o.mu.Unlock()
	for {
		o.mu.Lock()
		active := o.active
		changed := o.changed
		o.mu.Unlock()
		if !active {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
func (o *operatorServer) resume() { o.mu.Lock(); o.paused = false; o.signal(); o.mu.Unlock() }

type operatorObservation struct {
	attempt    uint64
	diagnostic security.PipeDiagnostic
}

// Recording must perform no IO: even a delayed session completion or idle accept
// timeout may overlap the independent inspector's whole-process IO measurement.
func (o *operatorServer) recordObservation(attempt uint64, d security.PipeDiagnostic) {
	if len(o.observations) == 64 {
		copy(o.observations, o.observations[1:])
		o.observations = o.observations[:63]
		o.observationDropped++
	}
	o.observations = append(o.observations, operatorObservation{attempt, d})
}

// Called by Run only after HTTP/inspection drainage and actual operator stop.
func (o *operatorServer) flushObservations() {
	select {
	case <-o.done:
	default:
		return
	} // cannot expose an active writer/race
	if o.observe != nil {
		for _, v := range o.observations {
			o.observe(v.attempt, v.diagnostic)
		}
	}
	o.observations = nil
}
func (o *operatorServer) stop() {
	o.mu.Lock()
	o.stopped = true
	if o.cancel != nil {
		o.cancel()
	}
	o.signal()
	o.mu.Unlock()
	<-o.done
}
