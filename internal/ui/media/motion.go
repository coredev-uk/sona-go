package media

import (
	"context"
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"
)

// ErrNoDecoder is returned where nothing plays motion artwork: the platform
// has no decoder (not Supported), GStreamer lacks the plugins for H.264 and
// HEVC (Linux), or the system has no video framework (a Windows N edition
// without the Media Feature Pack).
var ErrNoDecoder = fmt.Errorf("media: no motion artwork decoder: %w", errors.ErrUnsupported)

// ErrDecode wraps what OpenMotion fails with after it has the file: the
// player did not open (a plugin is missing, the file is not a video). A
// failure before that, the file's download, is not one. A video that opens
// and then fails to decode is the player's Err.
var ErrDecode = errors.New("media: motion artwork did not decode")

// OpenMotion makes a looping player of the motion artwork urlPath names (a
// DTO's "/motion/<key>/<n>.mp4"), the rendition for px device pixels.
// It fetches the file from the motion cache (downloading it when it is not
// there) and opens a player on it, so it blocks: call it off the main thread.
//
// On Linux the player decodes with GStreamer in this process and has its
// first frame when it returns. On macOS and Windows the system's own video
// framework plays the file (ui/video, muted): the player returns at once,
// loads in the background, and is not Ready, with no Size, until its first
// picture came, which only a ui.Video that shows it builds. Its Err says what
// stopped it. The player belongs to the main thread from then on (ui.Video
// shows it; Close releases the decoder).
func OpenMotion(ctx context.Context, files MotionFiles, urlPath string, px int) (*ui.VideoPlayer, error) {
	codec := Codec()
	if codec == "" {
		return nil, ErrNoDecoder
	}
	path, err := files.Local(ctx, urlPath, px, codec)
	if err != nil {
		return nil, err
	}
	return openPlayer(path)
}
