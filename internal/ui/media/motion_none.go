//go:build !((linux || darwin || windows) && cgo)

package media

import (
	"errors"

	"github.com/egoist/mygo/ui"
)

// Supported is whether this build decodes motion artwork: not here. MyGo has
// no video codec of its own and GStreamer is linked through cgo, so a build
// without cgo (the type-checks, a cross-compile with CGO_ENABLED=0) and the
// systems GStreamer is not built for report Capabilities().MotionArtwork =
// false and covers show their stills.
const Supported = false

// Codec is "": nothing decodes motion artwork here.
func Codec() string { return "" }

// NewMotionDecoder has no decoder on this platform.
func NewMotionDecoder(string) (ui.VideoDecoder, error) { return nil, errors.ErrUnsupported }
