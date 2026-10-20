//go:build windows && amd64

package windows

import "errors"

// ErrPhysicalCloseUnknown means a native close failed to prove physical release.
// The returned error retains the affected resources. Its owner must retain that
// error and quarantine the resources until process exit, not retry their close.
var ErrPhysicalCloseUnknown = errors.New("Windows physical close not confirmed")

type physicalCloseError struct {
	cause    error
	retained []any
}

func (e *physicalCloseError) Error() string {
	return ErrPhysicalCloseUnknown.Error() + ": " + e.cause.Error()
}
func (e *physicalCloseError) Unwrap() []error {
	return []error{ErrPhysicalCloseUnknown, e.cause}
}

// physicalCloseUnknown is used only for an actual failed native close.
func physicalCloseUnknown(err error, retained ...any) error {
	if err == nil {
		return nil
	}
	return &physicalCloseError{cause: err, retained: retained}
}

// retainPhysicalClose adds ownership evidence without classifying ordinary
// validation failures with successful cleanup as an unknown native close.
func retainPhysicalClose(err error, retained ...any) error {
	if !errors.Is(err, ErrPhysicalCloseUnknown) {
		return err
	}
	return physicalCloseUnknown(err, retained...)
}

// closePins stops at the first failed close, preserving every enclosing pin.
func closePins(pins []*File) error {
	for i := len(pins) - 1; i >= 0; i-- {
		if err := pins[i].Close(); err != nil {
			return retainPhysicalClose(err, pins)
		}
	}
	return nil
}

// unwindPins must not release enclosing pins after a nested resource's close
// already became unknown. Retaining pointers alone would not preserve exclusion.
func unwindPins(err error, pins []*File, retained ...any) error {
	if !errors.Is(err, ErrPhysicalCloseUnknown) {
		err = errors.Join(err, closePins(pins))
	}
	return retainPhysicalClose(err, append(retained, pins)...)
}
