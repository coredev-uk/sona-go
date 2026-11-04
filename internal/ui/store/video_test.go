package store

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestVideoViewOpensAndCloses(t *testing.T) {
	v := NewVideoView(nil, nil)
	if v.IsOpen() {
		t.Fatal("a new view is open")
	}
	if v.Frames() != nil {
		t.Error("a view of no source has one")
	}
	v.Open()
	if !v.IsOpen() {
		t.Fatal("Open did not open")
	}
	v.Toggle()
	if v.IsOpen() {
		t.Fatal("Toggle did not close an open view")
	}
	v.Toggle()
	if !v.IsOpen() {
		t.Fatal("Toggle did not open a closed view")
	}
	v.Close()
	v.Close() // closing a closed view is nothing
	if v.IsOpen() {
		t.Fatal("Close did not close")
	}
}

// fakeWindow is a window with a full-screen state.
type fakeWindow struct {
	full  bool
	calls []bool
}

// view is a VideoView going full screen in w.
func (w *fakeWindow) view() *VideoView {
	win := NewWindow()
	win.Bind(func() bool { return w.full }, func(on bool) { w.full = on; w.calls = append(w.calls, on) })
	return NewVideoView(nil, win)
}

func TestVideoViewLeavesFullScreenItEntered(t *testing.T) {
	w := &fakeWindow{}
	v := w.view()
	v.Open()
	v.SetFullscreen(true)
	if !v.Fullscreen() {
		t.Fatal("the window is not full screen")
	}
	v.Close()
	if w.full {
		t.Error("closing the view left the window full screen")
	}
}

func TestVideoViewLeavesAFullScreenOfTheUsersAlone(t *testing.T) {
	w := &fakeWindow{full: true} // the user pressed F11 before
	v := w.view()
	v.Open()
	if !v.Fullscreen() {
		t.Fatal("the view does not see the window's full screen")
	}
	v.Close()
	if !w.full || len(w.calls) != 0 {
		t.Errorf("closing the view touched the user's full screen: full %v calls %v", w.full, w.calls)
	}
}

func TestVideoViewDoesNotLeaveAFullScreenItLeftItself(t *testing.T) {
	w := &fakeWindow{}
	v := w.view()
	v.Open()
	v.SetFullscreen(true)
	v.SetFullscreen(false) // the button again
	w.full = true          // the user goes full screen afterwards
	v.Close()
	if !w.full {
		t.Error("closing the view took the user's later full screen away")
	}
}

func TestVideoViewWithoutAWindowHasNoFullScreen(t *testing.T) {
	v := NewVideoView(nil, nil)
	v.SetFullscreen(true)
	if v.Fullscreen() {
		t.Error("a view with no window is full screen")
	}
	v.Close()
}

func TestVideoViewRemembersTheOpenerOfAClosedViewOnly(t *testing.T) {
	var a, b ui.Handle
	v := NewVideoView(nil, nil)
	v.OpenFrom(&a)
	if v.opener != &a {
		t.Fatal("OpenFrom did not remember the opener")
	}
	v.OpenFrom(&b) // already open: the view keeps the control that opened it
	v.Open()
	if v.opener != &a {
		t.Error("a view that was open changed its opener")
	}
	v.Close()
	v.Open() // opened by something that is not a control: no opener
	if v.opener != nil {
		t.Error("Open kept the opener of the view before")
	}
	v.Toggle()
	v.ToggleFrom(&b)
	if !v.IsOpen() || v.opener != &b {
		t.Errorf("ToggleFrom: open %v opener %v", v.IsOpen(), v.opener == &b)
	}
	v.ToggleFrom(&a)
	if v.IsOpen() {
		t.Error("ToggleFrom did not close an open view")
	}
}

func TestVideoViewReturnsNoFocusWithoutAnOpenerOrWhileOpen(t *testing.T) {
	var h ui.Handle
	v := NewVideoView(nil, nil)
	if v.ReturnFocus(nil) {
		t.Error("a view nothing opened asked for a focus")
	}
	v.OpenFrom(&h)
	if v.ReturnFocus(nil) {
		t.Error("an open view asked for a focus")
	}
}
