// Package shell is Sona's desktop shell around the Wails app: the main
// window (restored size, close-to-background, its title bar), the mini
// player window, and the StatusNotifierItem tray with transport controls.
//
// Windows are shown and hidden, never destroyed, until the app quits: a
// hidden main window keeps its page (and the playback mirror) alive, so
// bringing it back is instant.
package shell

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/diag"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// Window names and documents.
const (
	mainName = "main"
	miniName = "mini"
	miniURL  = "/miniplayer.html"
)

// The mini player is a fixed-size window. Compositors float fixed-size
// toplevels (Hyprland, sway, GNOME), so it is not tiled.
const (
	miniWidth  = 420
	miniHeight = 132
)

// saveDelay debounces window-size writes while the user drags an edge.
const saveDelay = time.Second

// Options are what the shell needs from main.
type Options struct {
	App      *application.App
	Settings *config.Store
	// Coord drives the tray's transport items and their state.
	Coord  *coordinator.Coordinator
	Logger *slog.Logger
	// Private is private listening, a checkbox in the tray (nil: none).
	Private *privatemode.Mode
	// WindowIcon is the windows' icon; TrayIcon the tray's (a small PNG).
	WindowIcon []byte
	TrayIcon   []byte
	// Background is the colour painted before the first frame.
	Background application.RGBA
	// Hidden starts with no window shown (--hidden): playback and the
	// tray only, until the user asks for a window.
	Hidden bool
	// TitleBar is how title bars work on this platform; with the user's
	// choice (Settings → Desktop → Title bar) it decides whether the main
	// window is frameless (titlebar.go).
	TitleBar windowchrome.Platform
}

// Shell owns the windows and the tray.
type Shell struct {
	o    Options
	main *application.WebviewWindow
	tray *tray

	mu       sync.Mutex
	mini     *application.WebviewWindow // nil until first opened
	stopTray func()                     // ends the tray's subscription

	quitting atomic.Bool

	saveMu    sync.Mutex
	saveTimer *time.Timer
	window    config.Window // the remembered state, as last observed

	stateMu  sync.Mutex
	state    windowchrome.WindowState         // the main window's, as last observed
	watchers []func(windowchrome.WindowState) // WatchMain's
}

// New creates the main window (shown when the app runs, unless
// o.Hidden) and the tray. Call it before App.Run.
func New(o Options) *Shell {
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	s := &Shell{o: o, stopTray: func() {}}
	cur := o.Settings.Get()
	s.window = cur.Window

	start := application.WindowStateNormal
	if cur.Window.Maximised {
		start = application.WindowStateMaximised
	}
	// The title bar setting applies from the next start: GTK doesn't bring
	// its own title bar back to a window that ran without one.
	kind, _ := o.TitleBar.Resolve(cur.TitleBar.Style)
	s.main = o.App.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             mainName,
		Title:            windowTitle,
		Width:            cur.Window.Width,
		Height:           cur.Window.Height,
		MinWidth:         config.MinWindowWidth,
		MinHeight:        config.MinWindowHeight,
		StartState:       start,
		Hidden:           o.Hidden,
		BackgroundColour: o.Background,
		// Sona's own title bar (Windows, Linux stacking desktops): no
		// frame; the top bar drags, and the web runtime resizes from the
		// edges.
		Frameless: kind == windowchrome.KindDrawn,
		// The hash router starts on the chosen page; settings are read
		// before the first paint, nothing else.
		URL:   "/#" + cur.Desktop.StartRoute(),
		Linux: application.LinuxWindow{Icon: o.WindowIcon},
		Mac:   macWindow(kind),
	})
	s.main.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if s.quitting.Load() {
			return
		}
		e.Cancel()
		s.close(s.main)
	})
	for _, ev := range []events.WindowEventType{
		events.Common.WindowDidResize,
		events.Common.WindowMaximise,
		events.Common.WindowUnMaximise,
		events.Common.WindowFullscreen,
		events.Common.WindowUnFullscreen,
	} {
		s.main.OnWindowEvent(ev, func(*application.WindowEvent) { s.observe() })
	}

	s.tray = newTray(s, o.TrayIcon)
	o.App.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		guardKeyboard(s.main)
		trackSecondaryPresses(s.main)
		if kind == windowchrome.KindDrawn {
			setFramelessTitle(s.main, windowTitle)
		}
		snaps, cancel := o.Coord.Subscribe()
		s.mu.Lock()
		s.stopTray = cancel
		s.mu.Unlock()
		diag.Go(o.Logger, "tray: follow playback", func() { s.tray.follow(snaps) })
	})
	return s
}

// close handles a window's close button: hide it and keep playing, or
// quit, as Settings → Desktop says.
func (s *Shell) close(w *application.WebviewWindow) {
	if s.o.Settings.Get().Desktop.CloseAction == config.CloseQuits {
		s.Quit()
		return
	}
	w.Hide()
	if w == s.miniWindow() {
		s.tray.miniShown(false)
	}
	s.o.Logger.Info("shell: window closed to the background", "window", w.Name())
}

// ShowMain brings the main window to the front and closes the mini player:
// the tray's Show, MPRIS Raise, a second launch, the mini player's
// expand button.
func (s *Shell) ShowMain() {
	application.InvokeAsync(func() {
		if m := s.miniWindow(); m != nil && m.IsVisible() {
			m.Hide()
			s.tray.miniShown(false)
		}
		s.main.UnMinimise()
		s.main.Show()
		s.main.Focus()
	})
}

// ToggleMini opens the mini player in place of the main window, or closes
// it and returns to the main window.
func (s *Shell) ToggleMini() {
	application.InvokeAsync(func() {
		if m := s.miniWindow(); m != nil && m.IsVisible() {
			s.ShowMain()
			return
		}
		m := s.ensureMini()
		m.Show()
		m.Focus()
		s.main.Hide()
		s.tray.miniShown(true)
	})
}

// CloseMini is the mini player's own close button: the same as closing
// the main window.
func (s *Shell) CloseMini() {
	if m := s.miniWindow(); m != nil {
		s.close(m)
	}
}

// MainFocused reports whether the main window is shown and has focus.
func (s *Shell) MainFocused() bool {
	return s.main.IsVisible() && s.main.IsFocused()
}

// Quit exits the app; windows close for real from here on.
func (s *Shell) Quit() {
	if s.quitting.Swap(true) {
		return
	}
	s.o.Logger.Info("shell: quit requested")
	// Off the caller's goroutine: quitting tears down what may have
	// delivered the request (a tray click, a D-Bus call).
	go s.o.App.Quit()
}

// ResetWindowSize returns the main window to its default size, now and
// for the next start.
func (s *Shell) ResetWindowSize() {
	application.InvokeAsync(func() {
		s.main.UnMaximise()
		s.main.SetSize(config.DefaultWindowWidth, config.DefaultWindowHeight)
	})
	s.saveMu.Lock()
	s.window = config.Window{Width: config.DefaultWindowWidth, Height: config.DefaultWindowHeight}
	s.saveMu.Unlock()
	s.scheduleSave()
}

// Close stops following playback and writes a pending window size. Call
// it before the coordinator closes.
func (s *Shell) Close() {
	s.mu.Lock()
	stop := s.stopTray
	s.mu.Unlock()
	stop()
	s.saveMu.Lock()
	pending := s.saveTimer != nil && s.saveTimer.Stop()
	s.saveMu.Unlock()
	if pending {
		s.save()
	}
}

func (s *Shell) miniWindow() *application.WebviewWindow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mini
}

// ensureMini creates the mini player window on first use; it lives,
// hidden when closed, until the app quits.
func (s *Shell) ensureMini() *application.WebviewWindow {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mini != nil {
		return s.mini
	}
	const title = "Sona mini player"
	// Before its web view exists: WebKitGTK sizes a second view's page
	// from the font DPI.
	pinFontDPI()
	m := s.o.App.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             miniName,
		Title:            title,
		Width:            miniWidth,
		Height:           miniHeight,
		MinWidth:         miniWidth,
		MinHeight:        miniHeight,
		MaxWidth:         miniWidth,
		MaxHeight:        miniHeight,
		DisableResize:    true,
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		BackgroundColour: s.o.Background,
		URL:              miniURL,
		Linux:            application.LinuxWindow{Icon: s.o.WindowIcon},
	})
	// Before it is first shown, so window rules match it as it maps.
	setFramelessTitle(m, title)
	guardKeyboard(m)
	m.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if s.quitting.Load() {
			return
		}
		e.Cancel()
		s.close(m)
	})
	s.mini = m
	return m
}

// observe records the main window's size and maximised state.
func (s *Shell) observe() {
	w, h := s.main.Size()
	maximised, fullscreen := s.main.IsMaximised(), s.main.IsFullscreen()
	next := observeWindow(s.currentWindow(), w, h, maximised, fullscreen)
	s.saveMu.Lock()
	changed := next != s.window
	s.window = next
	s.saveMu.Unlock()
	if changed {
		s.scheduleSave()
	}
	s.setState(windowchrome.WindowState{Maximised: maximised, Fullscreen: fullscreen})
}

func (s *Shell) currentWindow() config.Window {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	return s.window
}

func (s *Shell) scheduleSave() {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	s.saveTimer = time.AfterFunc(saveDelay, s.save)
}

func (s *Shell) save() {
	w := s.currentWindow()
	if _, err := s.o.Settings.Update(func(st *config.Settings) { st.Window = w }); err != nil {
		s.o.Logger.Warn("shell: save window size", "err", err)
	}
}
