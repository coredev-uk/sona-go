package app

import (
	"errors"
	"sync"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

var _ services.Windows = (*Windows)(nil)

// Windows is the main window as the services see it (services.Windows).
// main creates it before the services, which keep it, and New attaches the
// window to it when MyGo is ready; until then every call finds no window
// and does nothing, which is also what a call after the window closed does.
//
// The mini player, the tray, the title bar's own buttons and the
// close-to-background behaviour are phase 3 of the migration: their calls
// are no-ops here, and no control of the interface reaches them.
type Windows struct {
	dev bool

	mu      sync.Mutex
	win     *mygo.Window
	mem     *windowMemory
	state   windowchrome.WindowState
	watches []func(windowchrome.WindowState)
}

// NewWindows is the window control for services.Deps.Windows. dev tells
// that the inspector may be shown (a -tags dev build).
func NewWindows(dev bool) *Windows { return &Windows{dev: dev} }

// attach connects the main window; its size and state events feed the
// remembered window state and the watchers.
func (w *Windows) attach(win *mygo.Window, mem *windowMemory) {
	w.mu.Lock()
	w.win, w.mem = win, mem
	w.mu.Unlock()
	observe := func() {
		width, height := win.Size()
		maxed, full := win.IsMaximized(), win.IsFullScreen()
		mem.observe(width, height, maxed, full)
		w.setState(windowchrome.WindowState{Maximised: maxed, Fullscreen: full})
	}
	win.OnResize(observe)
	win.OnMaximize(observe)
	win.OnUnmaximize(observe)
	win.OnEnterFullScreen(observe)
	win.OnLeaveFullScreen(observe)
}

func (w *Windows) setState(st windowchrome.WindowState) {
	w.mu.Lock()
	changed := st != w.state
	w.state = st
	watches := append([]func(windowchrome.WindowState){}, w.watches...)
	w.mu.Unlock()
	if !changed {
		return
	}
	for _, fn := range watches {
		fn(st)
	}
}

func (w *Windows) window() *mygo.Window {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.win
}

// ShowMain raises the main window.
func (w *Windows) ShowMain() {
	win := w.window()
	if win == nil {
		return
	}
	win.Show()
	win.Restore()
	win.Focus()
}

// Focused reports whether the main window is shown and has the focus
// (services.Deps.Focused).
func (w *Windows) Focused() bool {
	win := w.window()
	return win != nil && win.IsVisible() && win.IsFocused()
}

// ToggleMini is the mini player's, phase 3.
func (*Windows) ToggleMini() {}

// CloseMini is the mini player's, phase 3.
func (*Windows) CloseMini() {}

// ResetWindowSize returns the main window to its default size, now and for
// the next start.
func (w *Windows) ResetWindowSize() {
	w.mu.Lock()
	win, mem := w.win, w.mem
	w.mu.Unlock()
	if win == nil {
		return
	}
	win.Unmaximize()
	win.SetSize(config.DefaultWindowWidth, config.DefaultWindowHeight)
	mem.reset()
}

// MinimiseMain minimises the main window.
func (w *Windows) MinimiseMain() {
	if win := w.window(); win != nil {
		win.Minimize()
	}
}

// ToggleMaximiseMain maximises the main window or restores it.
func (w *Windows) ToggleMaximiseMain() {
	if win := w.window(); win != nil {
		win.ToggleMaximize()
	}
}

// CloseMain closes the main window, which ends the app: keeping it running
// in the background is phase 3's.
func (w *Windows) CloseMain() {
	if win := w.window(); win != nil {
		win.Close()
	}
}

// ShowWindowMenu is unsupported: MyGo has no call for the desktop's window
// menu.
func (*Windows) ShowWindowMenu() error { return errors.ErrUnsupported }

// DeveloperTools reports whether the inspector is on this run.
func (w *Windows) DeveloperTools() bool { return w.dev && inspectorRequested() }

// OpenDeveloperTools shows the inspector of a development build run with
// SONA_INSPECTOR=1; everywhere else it is unsupported.
func (w *Windows) OpenDeveloperTools() error {
	win := w.window()
	if win == nil || !w.DeveloperTools() {
		return errors.ErrUnsupported
	}
	win.Page().ToggleDevTools()
	return nil
}

// MainState is the main window's maximised and full-screen state.
func (w *Windows) MainState() windowchrome.WindowState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// WatchMain follows changes of MainState.
func (w *Windows) WatchMain(fn func(windowchrome.WindowState)) {
	w.mu.Lock()
	w.watches = append(w.watches, fn)
	w.mu.Unlock()
}

// WatchNavigate follows the mouse's back and forward buttons. MyGo's
// router takes them itself, so the services have nothing to follow.
func (*Windows) WatchNavigate(func(bool)) {}
