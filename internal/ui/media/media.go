// Package media is what the native UI needs of the system's media stack and
// MyGo lacks: the player of Apple's motion artwork (H.264 in MP4: MyGo's
// ui/video plays it with AVFoundation on macOS and Media Foundation on
// Windows; on Linux MyGo has no decoder that fits and GStreamer decodes it in
// process, see motion_gst.go) and the conversion of the music video pictures
// the CDM host decodes (I420, internal/playback/video) to the RGBA a MyGo
// bitmap takes.
//
// Nothing here draws: the kit's MotionCover, TileMotion and VideoSurface
// (internal/ui/kit) show what this package opens, and the kit's ArtLoader
// is the cache of decoded bitmaps.
package media

import "context"

// MotionFiles is where motion artwork files come from: *motion.Cache is one.
// Local returns the file of the rendition of urlPath (a DTO's
// "/motion/<key>/<n>.mp4") at px device pixels in codec ("avc" or "hevc"),
// downloading it first when it is not on disk. No HTTP is involved: the file
// is the one the cache would serve.
type MotionFiles interface {
	Local(ctx context.Context, urlPath string, px int, codec string) (string, error)
}
