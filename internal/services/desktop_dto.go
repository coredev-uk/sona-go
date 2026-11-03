package services

import (
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// EventWindowState carries the main window's WindowState when it changes.
const EventWindowState = "window:state"

// EventNavigate carries NavigateBack or NavigateForward when the mouse's
// back or forward button is pressed on the main window; the page goes
// through its history as its own back and forward controls do.
const EventNavigate = "window:navigate"

// The directions EventNavigate carries.
const (
	NavigateBack    = "back"
	NavigateForward = "forward"
)

// WindowState is the main window's state that Sona's title bar follows:
// the maximise button shows Restore while Maximised, and on macOS the
// room for the system's buttons goes while Fullscreen (they hide there).
type WindowState struct {
	Maximised  bool `json:"maximised"`
	Fullscreen bool `json:"fullscreen"`
}

func windowState(st windowchrome.WindowState) WindowState {
	return WindowState{Maximised: st.Maximised, Fullscreen: st.Fullscreen}
}
