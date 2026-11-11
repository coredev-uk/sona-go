//go:build !((linux && cgo) || darwin || (windows && (amd64 || arm64)))

package media

import (
	"errors"

	"github.com/egoist/mygo/ui"
)

// Supported is whether this build plays motion artwork: not here. Linux plays
// it through GStreamer, linked with cgo, and macOS and Windows through the
// system's video framework; a build without cgo on Linux (the type-checks, a
// cross-compile with CGO_ENABLED=0) and the systems with neither report
// Capabilities().MotionArtwork = false and covers show their stills.
const Supported = false

// Codec is "": nothing plays motion artwork here.
func Codec() string { return "" }

// openPlayer has no decoder on this platform.
func openPlayer(string) (*ui.VideoPlayer, error) { return nil, errors.ErrUnsupported }
