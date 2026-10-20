//go:build windows && amd64

package acceptance

import "testing"

// A second call returning early from the native owner's attempted/done guard
// is NOT evidence the first physical finish succeeded. Only its first normal
// return may authorize fixture restoration.
func finishInspectionOnce(attempted, joined *bool, finish func()) {
	if *attempted {
		return
	}
	*attempted = true
	finish()
	*joined = true
}
func TestInspectionFinishProofRequiresFirstNormalReturn(t *testing.T) {
	for _, fails := range []bool{false, true} {
		attempted, joined, calls := false, false, 0
		finish := func() {
			finishInspectionOnce(&attempted, &joined, func() {
				calls++
				if fails {
					panic("policy-only unwinding")
				}
			})
		}
		func() { defer func() { _ = recover() }(); defer finish(); finish() }()
		finish()
		if !attempted || calls != 1 || joined == fails {
			t.Fatal("later no-op falsely established physical join")
		}
	}
	// Panic policy only, not a native Close/Goexit fault observation.
}
