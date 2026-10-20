//go:build windows && amd64 && aboard_acceptance

package inspect

// AcceptanceScratchIO exposes actual successful-copy accounting only to the
// instrumented test build. It is not a production API, inferred file size, or
// accounting for metadata/failed write attempts.
func (r Report) AcceptanceScratchIO() (calls, bytes uint64) { return r.scratchWrites, r.scratchBytes }
