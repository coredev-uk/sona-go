//go:build !linux

package addons

import (
	"errors"
	"os"
)

// CanRunVerified is false where Sona can't yet run an add-on from the file it
// checked: `sona addons dev` needs it and refuses to run there.
const CanRunVerified = false

// OpenVerified is not available here.
func OpenVerified(dir, exe string) (*os.File, error) {
	return nil, errors.ErrUnsupported
}
