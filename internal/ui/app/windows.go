package app

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

var _ services.Windows = (*Windows)(nil)

// native is the part of a *mygo.Window the window control drives; the tests
// fake it.
type native interface {
	Show()
	Hide()
	Focus()
	Close()
	Restore()
	Minimize()
	ToggleMaximize()
	Unmaximize()
	SetSize(width, height int)
	IsVisible() bool
	IsFocused() bool
	IsMinimized() bool
}

// windowHooks are what the window control needs from the App.
type windowHooks struct {
	// closeQuits reports whether closing a window quits (Settings →
	// General → When the window is closed).
	closeQuits func() bool
	// newMini creates the mini player window, hidden; nil without a
	// module's mini player (module.Slots.Mini), which then never opens.
	newMini func() native
	// miniShown follows the mini player's shown state (the tray's item);
	// nil: nothing follows it.
	miniShown func(shown bool)
	// devTools toggles the main window's inspector.
	devTools func()
}

// Windows is Sona's windows as the services see them (services.Windows): the
// main window, the mini player shown in its place, and what closing either
// does. main creates it before the services, which keep it, and New attaches
// the window to it when MyGo is ready; until then every call finds no window
// and does nothing, which is also what a call after the app quit does.
//
// Windows are hidden, never destroyed, until the app quits: a hidden main
// window keeps its pages, so bringing it back is instant. Its calls come from
// any goroutine (MPRIS, the tray, a second launch, the services) and run on
// the main thread.
type Windows struct {
	dev bool
	// tray: Sona's tray icon is up (HasTray); set once, before the window.
	tray atomic.Bool
	// quitting: a quit started (ours, Cmd+Q, the menu's): windows close for
	// real from here on.
	quitting atomic.Bool
	// run runs fn on the main thread and waits (mygo.RunOnMain; the tests
	// run it in place).
	run func(fn func())
	// quit quits the app (mygo.App.Quit).
	quit func()

	mu      sync.Mutex
	win     native
	mini    native // nil until first opened
	hooks   windowHooks
	mem     *windowMemory
	state   windowchrome.WindowState
	watches []func(windowchrome.WindowState)
}

// NewWindows is the window control for services.Deps.Windows. dev tells
// that the inspector may be shown (a -tags dev build).
func NewWindows(dev bool) *Windows {
	return &Windows{dev: dev, run: mygo.RunOnMain, quit: mygo.App.Quit}
}

// attach connects the main window: its size and state events feed the
// remembered window state and the watchers, and its close button does what
// Settings says. A quit (Cmd+Q, the app menu's, SIGTERM) lets every window
// close.
func (w *Windows) attach(win *mygo.Window, mem *windowMemory, hooks windowHooks) {
	w.bind(win, mem, hooks)
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
	win.OnClose(func(e *mygo.CloseEvent) { w.closeEvent(win, e) })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { w.quitting.Store(true) })
	// The Dock icon (macOS) or the launcher with no window shown: the main
	// window comes back.
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			w.showMain()
		}
	})
}

// bind sets the main window and the hooks (attach, and the tests' fakes).
func (w *Windows) bind(win native, mem *windowMemory, hooks windowHooks) {
	w.mu.Lock()
	w.win, w.mem, w.hooks = win, mem, hooks
	w.mu.Unlock()
}

// closeEvent is a window's close request: let it close while quitting, else
// keep the window and do what closing it means.
func (w *Windows) closeEvent(win native, e *mygo.CloseEvent) {
	if w.quitting.Load() {
		return
	}
	e.PreventDefault()
	w.closeWindow(win)
}

// closeWindow hides win and keeps playing, or quits, as Settings → General
// says. Without a tray Sona quits: nothing could bring a hidden window back.
func (w *Windows) closeWindow(win native) {
	w.mu.Lock()
	hooks, mini := w.hooks, w.mini
	w.mu.Unlock()
	if !w.HasTray() || (hooks.closeQuits != nil && hooks.closeQuits()) {
		w.Quit()
		return
	}
	win.Hide()
	if win == mini && hooks.miniShown != nil {
		hooks.miniShown(false)
	}
}

// Quit quits Sona: every window closes for real. It returns at once:
// mygo.App.Quit only posts the quit to the main loop, so what asked for it (a
// tray item, a D-Bus call) is not waited on by the shutdown.
func (w *Windows) Quit() {
	if w.quitting.Swap(true) {
		return
	}
	w.quit()
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

func (w *Windows) window() native {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.win
}

// ShowMain brings the main window to the front and closes the mini player:
// the tray's Show Sona, MPRIS Raise, a second launch, the mini player's
// expand button.
func (w *Windows) ShowMain() { w.run(w.showMain) }

func (w *Windows) showMain() {
	w.mu.Lock()
	win, mini, hooks := w.win, w.mini, w.hooks
	w.mu.Unlock()
	if win == nil {
		return
	}
	if mini != nil && mini.IsVisible() {
		mini.Hide()
		if hooks.miniShown != nil {
			hooks.miniShown(false)
		}
	}
	win.Show()
	if win.IsMinimized() {
		win.Restore()
	}
	win.Focus()
}

// Focused reports whether the main window is shown and has the focus
// (services.Deps.Focused).
func (w *Windows) Focused() bool {
	win := w.window()
	return win != nil && win.IsVisible() && win.IsFocused()
}

// MiniShown reports whether the mini player is shown.
func (w *Windows) MiniShown() bool {
	w.mu.Lock()
	mini := w.mini
	w.mu.Unlock()
	return mini != nil && mini.IsVisible()
}

// ToggleMini opens the mini player in place of the main window, or closes it
// and brings the main window back. Without a mini player (no module builds
// one) the main window stays.
func (w *Windows) ToggleMini() {
	w.run(func() {
		if w.MiniShown() {
			w.showMain()
			return
		}
		w.mu.Lock()
		win, mini, hooks := w.win, w.mini, w.hooks
		if mini == nil && hooks.newMini != nil && win != nil {
			mini = hooks.newMini()
			w.mini = mini
		}
		w.mu.Unlock()
		if mini == nil {
			return
		}
		mini.Show()
		mini.Focus()
		win.Hide()
		if hooks.miniShown != nil {
			hooks.miniShown(true)
		}
	})
}

// CloseMini is the mini player's own close button: what closing the main
// window does (to the background, or quitting).
func (w *Windows) CloseMini() {
	w.run(func() {
		w.mu.Lock()
		mini := w.mini
		w.mu.Unlock()
		if mini != nil && mini.IsVisible() {
			w.closeWindow(mini)
		}
	})
}

// HasTray reports whether Sona's tray icon is up.
func (w *Windows) HasTray() bool { return w.tray.Load() }

// setTray records that the tray icon is up (New, before the window).
func (w *Windows) setTray(up bool) { w.tray.Store(up) }

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

// CloseMain closes the main window as its close button does (Settings →
// General: to the background, or quitting).
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
	w.mu.Lock()
	toggle := w.hooks.devTools
	w.mu.Unlock()
	if toggle == nil || !w.DeveloperTools() {
		return errors.ErrUnsupported
	}
	toggle()
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
