//go:build !linux && !darwin

package procmem

import "errors"

// Of is not available here: it fails with errors.ErrUnsupported.
func Of(int) (Usage, error) { return Usage{}, errors.ErrUnsupported }
