//go:build darwin || (windows && (amd64 || arm64))

package media

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"
)

// Supported is whether this build plays motion artwork: it does, through the
// system's own video framework (ui/video: AVFoundation on macOS, Media
// Foundation on Windows), with no cgo and no GStreamer. The picture is
// decoded by the system, in hardware where it can, and reaches the window's
// renderer on the GPU. Capabilities().MotionArtwork reports it.
const Supported = true

// Codec is "avc": H.264 in MP4 plays on every Mac and Windows 10 and 11, and
// is Apple's smallest rendition to decode. ui/video cannot be asked what a
// system decodes, so HEVC (which only some systems have a decoder for) is
// never requested; a system with no H.264 at all (a Windows N edition without
// the Media Feature Pack) fails at open, with ErrNoDecoder, and the policy
// stops offering motion then.
func Codec() string { return "avc" }

// openPlayer makes the looping, muted player of the file at path, which the
// system opens in the background: it is not Ready, and has no Size, until
// its first picture came, which a ui.Video that shows it builds. A system with
// no media framework is ErrNoDecoder; any other failure to open is ErrDecode.
func openPlayer(path string) (*ui.VideoPlayer, error) {
	p, err := video.Open(path)
	switch {
	case errors.Is(err, video.ErrUnsupported):
		return nil, fmt.Errorf("%w: %w", ErrNoDecoder, err)
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrDecode, err)
	}
	return p.SetLoop(true).SetMuted(true), nil
}
