package shell

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// windowTitle is the main window's title (task switchers, window rules).
const windowTitle = "Sona"

// macWindow is the main window's macOS options: with the system's buttons
// inset into Sona's top bar, a hidden title bar over full-size content and
// AppKit's inset button position (centred in a unified toolbar's height,
// which Sona's top bar matches). Dragging and double-click zoom go through
// the top bar's drag region (--wails-draggable), so the top bar's own
// controls stay clickable.
func macWindow(kind string) application.MacWindow {
	if kind != windowchrome.KindNativeInset {
		return application.MacWindow{}
	}
	return application.MacWindow{TitleBar: application.MacTitleBarHiddenInset}
}

// MinimiseMain is Sona's title bar's Minimise button.
func (s *Shell) MinimiseMain() {
	application.InvokeAsync(func() { s.main.Minimise() })
}

// ToggleMaximiseMain is Sona's title bar's Maximise/Restore button and a
// double click on the title bar.
func (s *Shell) ToggleMaximiseMain() {
	application.InvokeAsync(func() { s.main.ToggleMaximise() })
}

// CloseMain is Sona's title bar's Close button: what closing the window
// does (Settings → General: keep playing in the background, or quit).
func (s *Shell) CloseMain() {
	s.close(s.main)
}

// ShowWindowMenu opens the desktop's window menu for the main window at
// the pointer, for a right click on Sona's title bar. It fails with
// errors.ErrUnsupported where the platform has no such menu or Sona can't
// open it (macOS, Windows), and when no right click was seen.
func (s *Shell) ShowWindowMenu() error {
	if !s.o.TitleBar.WindowMenu {
		return errors.ErrUnsupported
	}
	return showWindowMenu(s.main)
}

// MainState is the main window's state as last observed.
func (s *Shell) MainState() windowchrome.WindowState {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.state
}

// WatchMain calls fn with each change of the main window's state.
func (s *Shell) WatchMain(fn func(windowchrome.WindowState)) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.watchers = append(s.watchers, fn)
}

func (s *Shell) setState(st windowchrome.WindowState) {
	s.stateMu.Lock()
	if st == s.state {
		s.stateMu.Unlock()
		return
	}
	s.state = st
	watchers := s.watchers
	s.stateMu.Unlock()
	for _, fn := range watchers {
		fn(st)
	}
}
