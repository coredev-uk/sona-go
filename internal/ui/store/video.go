package store

import (
	"github.com/coredev-uk/sona-go/internal/ui/media"
)

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
	frames media.FrameSource
	win    *Window
	open   bool

	// ours: the view put the window in full screen, so closing it leaves
	// again; a full screen the user entered elsewhere is left alone.
	ours bool
}

// NewVideoView is a VideoView, closed, whose pictures come from frames (nil:
// none, which a build without video playback has) and which goes full screen
// in win (the Env's Window; nil: a window of its own that is never bound).
func NewVideoView(frames media.FrameSource, win *Window) *VideoView {
	if win == nil {
		win = NewWindow()
	}
	return &VideoView{frames: frames, win: win}
}

// Frames is the source of the playing music video's pictures; nil when none.
func (v *VideoView) Frames() media.FrameSource { return v.frames }

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
