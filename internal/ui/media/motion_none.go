//go:build !((linux || darwin) && cgo)

package media

import (
	"errors"

	"github.com/egoist/mygo/ui"
)

// Supported is whether this build decodes motion artwork: not here. MyGo has
// no video codec of its own. A Windows decoder would drive Media Foundation's
// H.264 transform (a COM MFT with sample and media-type plumbing) through raw
// syscalls, which cannot be written or tested without a Windows machine, so
// the platform reports Capabilities().MotionArtwork = false and covers show
// their stills.
const Supported = false

// Codec is "": nothing decodes motion artwork here.
func Codec() string { return "" }

// NewMotionDecoder has no decoder on this platform.
func NewMotionDecoder(string) (ui.VideoDecoder, error) { return nil, errors.ErrUnsupported }
