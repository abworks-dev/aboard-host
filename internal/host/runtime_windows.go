//go:build windows && amd64

package host

import (
	"context"
	"errors"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/abworks-dev/aboard-host/internal/installation"
	"github.com/abworks-dev/aboard-host/internal/installation/security"
	httptransport "github.com/abworks-dev/aboard-host/internal/transport/http"
	inspection "github.com/abworks-dev/aboard-host/internal/workspace/inspect"
)

var ErrService = errors.New("Host service unavailable")

type Event struct {
	Event       string                   `json:"event"`
	Authority   string                   `json:"authority,omitempty"`
	PID         int                      `json:"pid,omitempty"`
	HTTP        *httptransport.Event     `json:"http,omitempty"`
	Pipe        *security.PipeDiagnostic `json:"pipe,omitempty"`
	PipeAttempt uint64                   `json:"pipeAttempt,omitempty"`
	PipeDropped uint64                   `json:"pipeDropped,omitempty"`
}

func emit(log func(Event), event string) {
	if log != nil {
		log(Event{Event: event})
	}
}

// A physical-close failure has no permission to turn into process exit. Keep
// the actual owner live, report only a fixed fenced status, and await deliberate
// external process termination. This is not recovery or a claim of safe closure.
func retain(store *installation.Store, log func(Event)) {
	emit(log, "retained")
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		runtime.KeepAlive(store)
		emit(log, "retained")
	}
}
func cleanupStartup(store *installation.Store, log func(Event)) {
	if store != nil {
		if err := store.Close(); err != nil {
			retain(store, log)
		}
	}
}
func Initialize(ctx context.Context, c Config, log func(Event)) error {
	if err := c.Validate(); err != nil {
		return err
	}
	store, err := installation.Initialize(ctx, c.InstallationRoot)
	if err != nil {
		cleanupStartup(store, log)
		return ErrService
	}
	if err = store.CheckShutdownReady(ctx); err != nil {
		retain(store, log)
	}
	if err = store.Close(); err != nil {
		retain(store, log)
	}
	emit(log, "initialized")
	return nil
}

// Run only opens the configured existing installation. Cancellation requests
// graceful shutdown; it never cancels accepted work or releases unknown owners.
func Run(shutdown context.Context, c Config, log func(Event)) error {
	if c.Validate() != nil {
		return ErrConfiguration
	}
	store, err := installation.Open(context.Background(), c.InstallationRoot)
	if err != nil {
		cleanupStartup(store, log)
		return ErrService
	}
	auth, err := installation.NewAuthenticator(store, c.AllowedRoots)
	if err != nil {
		cleanupStartup(store, log)
		return ErrConfiguration
	}
	opener, err := installation.NewWorkspaceOpener(auth)
	if err != nil {
		cleanupStartup(store, log)
		return ErrService
	}
	pipe, err := security.NewControlPipe(c.PipeName)
	if err != nil {
		cleanupStartup(store, log)
		return ErrService
	}
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		if pipe.Close() != nil {
			retain(store, log)
		}
		cleanupStartup(store, log)
		return ErrService
	}
	handler, err := httptransport.New(auth, store, c.InstallationRoot, listener.Addr().String(), func(e httptransport.Event) {
		if log != nil {
			log(Event{Event: "http", HTTP: &e})
		}
	})
	if err != nil {
		listener.Close()
		if pipe.Close() != nil {
			retain(store, log)
		}
		cleanupStartup(store, log)
		return ErrService
	}
	server := &http.Server{Handler: handler, ErrorLog: stdlog.New(io.Discard, "", 0), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	served := make(chan struct{})
	var serveErr error
	go func() { serveErr = server.Serve(listener); close(served) }()
	control := newOperator(auth, pipe, store.Identity().OwnerPrincipalID)
	control.observe = func(attempt uint64, d security.PipeDiagnostic) {
		if log != nil {
			log(Event{Event: "operator", Pipe: &d, PipeAttempt: attempt, PipeDropped: control.observationDropped})
		}
	}
	go control.run()
	if log != nil {
		log(Event{Event: "ready", Authority: listener.Addr().String(), PID: os.Getpid()})
	}
	unexpected := false
	select {
	case <-shutdown.Done():
	case <-served:
		unexpected = true
		emit(log, "http_unavailable")
	}
	emit(log, "draining")
	// Cut off existing queues promptly, BEFORE waiting for slow HTTP bodies or
	// handlers. Store.workspaceStopping forbids late creation of an open owner.
	// Existing installation handlers may still finish their original intent, but
	// cannot create a late writable owner outside this stable shutdown snapshot.
	handler.FreezeNewWork()
	workspaces := opener.BeginShutdownWorkspaces()
	if handler.BeginDrain(context.Background()) != nil {
		retain(store, log)
	}
	<-workspaces
	reported := false
	for {
		changed := handler.Changes()
		cleanupErr := store.DrainInspections()
		var cleanupRetry <-chan time.Time
		if errors.Is(cleanupErr, inspection.ErrScratchCleanupPending) {
			cleanupRetry = time.After(time.Second)
		}
		if err = store.CheckShutdownReady(context.Background()); err != nil {
			if !reported {
				emit(log, "draining_unknown")
				reported = true
			}
			select {
			case <-changed:
			case <-control.progress:
			case <-cleanupRetry: // completed deletion refusal only, never native-close retry
			}
			continue
		}
		// Final quiescence: do not permanently close the only investigation channels
		// until HTTP and native issuance (including revoke/ACK cleanup) have joined.
		if handler.Pause(context.Background()) != nil {
			retain(store, log)
		}
		if control.pause(context.Background()) != nil {
			retain(store, log)
		}
		if store.CheckShutdownReady(context.Background()) == nil {
			break
		}
		handler.ResumeInvestigation()
		control.resume()
		emit(log, "draining_unknown")
		reported = true
	}
	// This stage has no unknown business resources. Network close still waits for
	// real handlers; no http.Server.Close or timeout-forced lease release is used.
	if server.Shutdown(context.Background()) != nil {
		retain(store, log)
	}
	<-served
	control.stop()
	control.flushObservations() // no stdout diagnostic while an inspector may measure
	if pipe.Close() != nil {
		retain(store, log)
	}
	if store.Close() != nil {
		retain(store, log)
	}
	emit(log, "stopped")
	if unexpected && !errors.Is(serveErr, http.ErrServerClosed) {
		return ErrService
	}
	return nil
}
