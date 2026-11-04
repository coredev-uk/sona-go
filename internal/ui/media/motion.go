package media

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/egoist/mygo/ui"
)

// ErrNoDecoder is returned where nothing decodes motion artwork: the
// platform has no decoder (not Supported) or GStreamer lacks the plugins for
// H.264 and HEVC.
var ErrNoDecoder = fmt.Errorf("media: no motion artwork decoder: %w", errors.ErrUnsupported)

// OpenMotion makes a looping player of the motion artwork urlPath names (a
// DTO's "/motion/<key>/<n>.mp4"), the rendition for px device pixels.
// It fetches the file from the motion cache (downloading it when it is not
// there), opens a decoder on it and decodes the first frame, so it blocks:
// call it off the main thread. The player belongs to the main thread from
// then on (ui.Video shows it; Close releases the decoder).
func OpenMotion(ctx context.Context, files MotionFiles, urlPath string, px int) (*ui.VideoPlayer, error) {
	codec := Codec()
	if codec == "" {
		return nil, ErrNoDecoder
	}
	path, err := files.Local(ctx, urlPath, px, codec)
	if err != nil {
		return nil, err
	}
	dec, err := NewMotionDecoder(path)
	if err != nil {
		return nil, err
	}
	p, err := ui.NewVideoPlayer(dec)
	if err != nil {
		if c, ok := dec.(io.Closer); ok {
			_ = c.Close() // a no-op if the player did
		}
		return nil, err
	}
	return p.SetLoop(true), nil
}
