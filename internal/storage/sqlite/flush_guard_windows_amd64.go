package sqlite

import (
	"fmt"
	"sync"

	"modernc.org/libc"
)

// An ordinary build that omitted the compatibility overlay must not silently
// accept unsafe durability semantics. InvalidHandle never touches a real file.
var verifyFlushBridge = sync.OnceValue(func() error {
	tls := libc.NewTLS()
	defer tls.Close()
	if result := libc.XFlushFileBuffers(tls, ^uintptr(0)); result != 0 {
		return fmt.Errorf("%w: Windows flush failure returned BOOL=%d; required compatibility fix missing", ErrProfile, result)
	}
	return nil
})
