package services

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// EventWindowState carries the main window's WindowState when it changes.
const EventWindowState = "window:state"

func init() { application.RegisterEvent[WindowState](EventWindowState) }

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
