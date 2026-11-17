//go:build !linux && !darwin && !windows

package sandbox

// Start fails: this operating system has no sandbox for add-ons.
func Start(Spec) (*Child, error) {
	return nil, fail(ReasonUnsupported, "no sandbox for add-ons on this system")
}
