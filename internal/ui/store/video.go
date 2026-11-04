package store

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/playback/video"
)

// VideoFrames is where the pictures of the playing music video come from:
// *video.Surface, which the playback engine shows each picture on as it falls
// due. It is the same contract as media.FrameSource.
type VideoFrames interface {
	// Next returns the picture on show once it is newer than after,
	// waiting until it is or ctx ends; false when ctx ended or the surface
	// was cleared (the Frame's Seq is then the one to wait from).
	Next(ctx context.Context, after uint64) (video.Frame, bool)
	// Seq is the sequence number of the latest change of the picture on show.
	Seq() uint64
}

// VideoView is the state the music video view shares with the features that
// open it: v0.2.0's overlay "video" of player/ui.ts, with the window's full
// screen the view toggles and the source of its pictures. The player bar's
// "Show video", Now Playing's button and a music video's Play open it
// (Open), the view closes itself (Close), and the shell keeps the window
// behind it inert while it is open.
//
// Main thread only: it is read while a view builds and written by what the
// user did (a click, a key), never from a goroutine.
type VideoView struct {
	frames VideoFrames
	win    *Window
	open   bool

	// ours: the view put the window in full screen, so closing it leaves
	// again; a full screen the user entered elsewhere is left alone.
	ours bool
}

// NewVideoView is a VideoView, closed, whose pictures come from frames (nil:
// none, which a build without video playback has) and which goes full screen
// in win (the Env's Window; nil: a window of its own that is never bound).
func NewVideoView(frames VideoFrames, win *Window) *VideoView {
	if win == nil {
		win = NewWindow()
	}
	return &VideoView{frames: frames, win: win}
}

// Frames is the source of the playing music video's pictures; nil when none.
func (v *VideoView) Frames() VideoFrames { return v.frames }

// Open shows the video view.
func (v *VideoView) Open() { v.open = true }

// Close hides the video view, and leaves full screen if the view entered it.
func (v *VideoView) Close() {
	if !v.open && !v.ours {
		return
	}
	v.open = false
	if v.ours {
		v.SetFullscreen(false)
	}
}

// Toggle opens the view, or closes it when it is open.
func (v *VideoView) Toggle() {
	if v.open {
		v.Close()
		return
	}
	v.Open()
}

// IsOpen reports whether the video view is open.
func (v *VideoView) IsOpen() bool { return v.open }

// Fullscreen reports whether the window is full screen now, whoever put it
// there (the window manager's F11, a double click on the title bar).
func (v *VideoView) Fullscreen() bool { return v.win.FullScreen() }

// SetFullscreen puts the window in full screen, or takes it out. Entering is
// the view's own: closing the view leaves it again.
func (v *VideoView) SetFullscreen(on bool) {
	v.ours = on
	v.win.SetFullScreen(on)
}
