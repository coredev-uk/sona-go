//go:build linux || (cgo && (darwin || windows))

package cdmhost

import (
	"context"
	"fmt"
	"time"
)

// Music videos. The parent loads a video as a track: its audio rendition
// goes through Load and Feed like a song's (decrypted with the CDM's
// Decrypt, decoded and played in the child), and its video rendition
// through LoadVideo and FeedVideo under the same track generation. The
// child decrypts and decodes the video with the CDM's own decoder (the
// only use its SW_SECURE_DECODE keys allow), holds the decoded pictures
// and sends each to the parent (VideoFrame) when the audio being heard
// reaches it, so the picture follows the audio clock. Decoded pictures are
// the only part of a video that leaves the child: never audio, never the
// compressed or decrypted bitstream.

// VideoFrame is one decoded picture of track Track, sent when it is due.
type VideoFrame struct {
	Track uint64
	// Position is the picture's place on the track's timeline (the
	// audio's: 0 is the first audible sample).
	Position      time.Duration
	Width, Height int
	// Data holds the planes packed as I420: Width×Height luma, then the
	// two chroma planes of ⌈Width/2⌉×⌈Height/2⌉ (U, then V).
	Data []byte
}

// MaxVideoSize bounds the picture size the parent accepts from the child:
// larger pictures (or a Data that does not match the size) are dropped.
const MaxVideoSize = 1920

// I420Size is the size of a packed I420 picture of w×h.
func I420Size(w, h int) int {
	cw, ch := (w+1)/2, (h+1)/2
	return w*h + 2*cw*ch
}

// valid reports whether f is a picture the parent may show.
func (f *VideoFrame) valid() bool {
	return f.Width > 0 && f.Height > 0 && f.Width <= MaxVideoSize && f.Height <= MaxVideoSize &&
		len(f.Data) == I420Size(f.Width, f.Height)
}

// VideoMedia is a music video's video rendition for LoadVideo.
type VideoMedia struct {
	// Init is the rendition's fMP4 init segment (encv/avc1 with avcC);
	// nil on a seek keeps the loaded one.
	Init []byte
	// KID is the 16-byte key ID to decrypt with, from the playlist's
	// Widevine key: Apple's video init segments carry an all-zero
	// default key ID.
	KID []byte
	// Shift is subtracted from the video's presentation times to put
	// them on the track's timeline (the audio's: Apple's renditions
	// start their timestamps at the playlist's offset, and the audio's
	// position leaves out its encoder priming).
	Shift time.Duration
}

// LoadVideo adds video to track, which Load just made current (a video's
// audio rendition): the child (re)initialises the CDM's video decoder for
// m (on a seek, with m.Init nil, it only flushes it) and shows pictures
// from start on. A Load of another track drops the video.
func (h *Host) LoadVideo(ctx context.Context, track uint64, m VideoMedia, start time.Duration) error {
	if len(m.KID) != 16 {
		return fmt.Errorf("cdmhost: video key ID of %d bytes", len(m.KID))
	}
	_, err := h.call(ctx, request{Op: opVideoLoad, Track: track, Data: m.Init, KID: m.KID, Start: start, Shift: m.Shift})
	return err
}

// FeedVideo sends one encrypted fragment (moof + mdat) of track's video.
// It returns once the child has queued it for the decoder, which blocks
// while the decoder is far enough ahead; the audio's Feed never waits for
// it. ErrStale means a later Load superseded track.
func (h *Host) FeedVideo(ctx context.Context, track uint64, fragment []byte) error {
	_, err := h.call(ctx, request{Op: opVideoFeed, Track: track, Data: fragment})
	return err
}

// EndOfVideo marks track's last video fragment: the decoder hands out the
// pictures it still holds.
func (h *Host) EndOfVideo(ctx context.Context, track uint64) error {
	_, err := h.call(ctx, request{Op: opVideoEnd, Track: track})
	return err
}
