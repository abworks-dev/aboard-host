//go:build windows && amd64

package security

import (
	"context"
	"errors"
	"syscall"
	"time"
)

// PipeDiagnostic contains only finite implementation stages, coarse outcomes and
// native status numbers. It cannot contain names, SIDs, credentials, DTOs or SQL.
// This is internal observability, not a wire acknowledgement or authority.
type PipeDiagnostic struct {
	Stage         string `json:"stage"`
	Outcome       string `json:"outcome"`
	Context       string `json:"context"`
	Native        uint32 `json:"native,omitempty"`
	ElapsedMillis int64  `json:"elapsedMillis"`
	ResetOutcome  string `json:"resetOutcome,omitempty"`
	ResetNative   uint32 `json:"resetNative,omitempty"`
}
type pipeFailure struct {
	stage string
	cause error
}

func (e *pipeFailure) Error() string { return "protected pipe failure at " + e.stage }
func (e *pipeFailure) Unwrap() error { return e.cause }
func pipeFault(stage string, err error) error {
	if err == nil {
		return nil
	}
	var existing *pipeFailure
	if errors.As(err, &existing) {
		return err
	}
	return &pipeFailure{stage: stage, cause: err}
}

// DiagnosePipe never renders err or caller data. Stage is supplied only by this
// package's literal call sites; arbitrary wrapped errors reduce to unclassified.
func DiagnosePipe(ctx context.Context, err error, elapsed time.Duration) PipeDiagnostic {
	d := PipeDiagnostic{Stage: "unclassified", Outcome: "other", Context: "none", ElapsedMillis: elapsed.Milliseconds()}
	var failure *pipeFailure
	primary := err
	if errors.As(err, &failure) {
		d.Stage = failure.stage
		primary = failure.cause
	}
	switch {
	case err == nil:
		d.Stage = "complete"
		d.Outcome = "ok"
	case errors.Is(primary, context.DeadlineExceeded):
		d.Outcome = "deadline"
	case errors.Is(primary, context.Canceled):
		d.Outcome = "canceled"
	case errors.Is(primary, ErrUnauthorized):
		d.Outcome = "unauthorized"
	default:
		var native syscall.Errno
		if errors.As(primary, &native) {
			d.Outcome = "native"
		}
	}
	// A primary joined authority/native failure retains both facts, without
	// borrowing any code from the independently reported reset subtree.
	var native syscall.Errno
	if errors.As(primary, &native) {
		d.Native = uint32(native)
	}
	if ctx != nil {
		switch ctx.Err() {
		case context.Canceled:
			d.Context = "canceled"
		case context.DeadlineExceeded:
			d.Context = "deadline"
		}
	}
	if reset := resetPipeFailure(err, 0); reset != nil {
		d.ResetOutcome = "other"
		var native syscall.Errno
		if errors.As(reset.cause, &native) {
			d.ResetOutcome = "native"
			d.ResetNative = uint32(native)
		}
	}
	return d
}
func resetPipeFailure(err error, depth int) *pipeFailure {
	if err == nil || depth > 32 {
		return nil
	}
	if f, ok := err.(*pipeFailure); ok && f.stage == "server-reset" {
		return f
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if f := resetPipeFailure(cause, depth+1); f != nil {
				return f
			}
		}
	} else if cause := errors.Unwrap(err); cause != nil {
		return resetPipeFailure(cause, depth+1)
	}
	return nil
}
