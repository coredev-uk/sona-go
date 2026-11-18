package store

// Window is the main window as the features see it (module.Env.Window):
// whether it has the keyboard focus, whether it is full screen, and the call
// that puts it in full screen or takes it out. The shell binds it to the
// window once that exists and redraws the window on each change of either,
// so a view that reads them builds again when they change.
//
// Without a window (tests that build an Env by hand, before New opened it)
// it is focused and never full screen, and SetFullScreen does nothing.
//
// Main thread only.
type Window struct {
	unfocused bool
	isFull    func() bool
	setFull   func(bool)
	hold      func() (release func())
	release   func() // the hold taken by KeepScreenOn; nil while none
}

// BindScreen connects what keeps the screen on: hold takes the hold (with
// the reason the system lists) and returns the call that gives it back. The
// shell calls it once; the store cannot name the reason itself, as the
// translations live above it.
func (w *Window) BindScreen(hold func() (release func())) { w.hold = hold }

// KeepScreenOn keeps the display from turning off or a screen saver from
// starting while on, e.g. for lyrics shown on a TV; false gives it back.
// Asking for the state it is in does nothing. Without a window bound it
// does nothing at all.
func (w *Window) KeepScreenOn(on bool) {
	if on == (w.release != nil) || w.hold == nil {
		return
	}
	if !on {
		w.release()
		w.release = nil
		return
	}
	if w.release = w.hold(); w.release == nil {
		w.release = func() {}
	}
}

// NewWindow is a Window with no window bound: focused, not full screen.
func NewWindow() *Window { return &Window{} }

// Bind connects the main window: full reports whether it is full screen,
// set puts it in or takes it out. The shell calls it once.
func (w *Window) Bind(full func() bool, set func(on bool)) {
	w.isFull, w.setFull = full, set
}

// SetFocused records whether the main window has the keyboard focus; the
// shell calls it from the window's focus and blur events.
func (w *Window) SetFocused(on bool) { w.unfocused = !on }

// Focused reports whether the main window has the keyboard focus: false
// while another app (or Sona's mini player) has it, or the window is hidden.
func (w *Window) Focused() bool { return !w.unfocused }

// FullScreen reports whether the main window is full screen now, whoever
// put it there (the window manager's F11, a double click on the title bar,
// a feature).
func (w *Window) FullScreen() bool { return w.isFull != nil && w.isFull() }

// SetFullScreen puts the main window in full screen, or takes it out. A
// feature that enters it for a view of its own (the music video, full-screen
// lyrics) remembers that it did and leaves only then.
func (w *Window) SetFullScreen(on bool) {
	if w.setFull != nil {
		w.setFull(on)
	}
}
