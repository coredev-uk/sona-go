// Package media is what the native UI needs of the system's media stack and
// MyGo lacks: a decoder for Apple's motion artwork (H.264 or HEVC in MP4;
// MyGo decodes no video codec, GStreamer does it on Linux, macOS and Windows, see
// motion_gst.go) and the conversion of the music video pictures the CDM host
// decodes (I420, internal/playback/video) to the RGBA a MyGo bitmap takes.
//
// Nothing here draws: the kit's MotionCover, TileMotion and VideoSurface
// (internal/ui/kit) show what this package decodes, and the kit's ArtLoader
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
