package store

import "testing"

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
