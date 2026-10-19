//go:build !windows || !amd64

package sqlite

func verifyFlushBridge() error { return ErrProfile }
