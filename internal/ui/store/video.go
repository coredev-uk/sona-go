package store

import (
	"github.com/egoist/mygo/ui"

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

	// opener is the control that opened the view, which the focus goes back
	// to when it closes; nil: none known. restoring is the one a return was
	// asked of and that is not answered yet.
	opener    *ui.Handle
	restoring *ui.Handle
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

// Open shows the video view. Nothing is known of what opened it, so closing
// it leaves the focus with the page (see ReturnFocus).
func (v *VideoView) Open() {
	if !v.open {
		v.opener = nil
	}
	v.open = true
}

// OpenFrom shows the video view, opened by the control bound to opener
// (Element.Bind): closing the view, however it closes, puts the focus back on
// that control (v0.2.0's useReturnFocus). A view that is already open keeps
// the opener it has.
func (v *VideoView) OpenFrom(opener *ui.Handle) {
	if !v.open {
		v.opener = opener
	}
	v.open = true
}

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
func (v *VideoView) Toggle() { v.ToggleFrom(nil) }

// ToggleFrom opens the view from the control bound to opener, or closes it
// when it is open (see OpenFrom).
func (v *VideoView) ToggleFrom(opener *ui.Handle) {
	if v.open {
		v.Close()
		return
	}
	v.OpenFrom(opener)
}

// ReturnFocus is the view's layer's to call in every frame it builds while
// the view is closed. The first call after the view closed asks for the
// focus on the control that opened it (Handle.Focus: MyGo gives it at the
// end of that frame, to the control as built in that frame). The next call
// takes the request back, so a control that was not built then (it is gone)
// never takes the focus when it comes back much later. With no opener, or
// none left, nothing is focused and Tab starts at the page. It reports
// whether it asked for a focus.
func (v *VideoView) ReturnFocus(c *ui.Context) bool {
	if v.restoring != nil {
		v.restoring.CancelFocus(c)
		v.restoring = nil
	}
	if v.open || v.opener == nil {
		return false
	}
	v.restoring, v.opener = v.opener, nil
	v.restoring.Focus(c)
	return true
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
