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
