//go:build !linux && !darwin

package procmem

// Of is not available here.
func Of(int) (Usage, error) { return Usage{}, ErrUnsupported }
