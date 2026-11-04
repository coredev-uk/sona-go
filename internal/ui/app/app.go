// Package app is the shell of Sona's native UI: it builds the shared Env
// (the event bus, cached queries, the playback mirror, the artwork loader),
// the modules registered in modules.go, the router, and the main window
// with its sidebar, top bar, page column and the slots modules build into
// (docs/ui-architecture.md, "The app shell").
package app

import (
	"cmp"
	"errors"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/media"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// windowTitle is the main window's title.
const windowTitle = "Sona"

// Options are what New needs from main.
type Options struct {
	// Core is the services; features look theirs up in it.
	Core *services.Core
	// Settings is the user's settings: the theme, the language, the start
	// page, the remembered window size.
	Settings *config.Store
	// Bus is the event bus the services were given as services.Deps.Emit;
	// main makes it before the core so no event is lost, and New connects
	// it to the window. Nil makes one (tests).
	Bus *store.Bus
	// Windows is the window control the services were given; New attaches
	// the main window to it. Nil: none (tests).
	Windows *Windows
	// Art is the artwork cache the covers are decoded from.
	Art *artwork.Cache
	// Motion serves the motion artwork files (the motion cache); nil: no
	// motion plays.
	Motion media.MotionFiles
	// Video is where the playing music video's pictures come from (the
	// engine's *video.Surface); nil: no video plays.
	Video media.FrameSource
	// Poster, when set, replaces Window.Update as how work gets to the
	// main thread (tests).
	Poster store.Poster
	Log    *slog.Logger
	// Version is the app version.
	Version string
	// Dev is a -tags dev build.
	Dev bool
	// Now is the clock the playback mirror extrapolates the position with;
	// nil is time.Now. Tests over uitest pass its fake clock.
	Now func() time.Time
	// StartPath overrides the page the window opens on (the settings'
	// start page, else the first sidebar link).
	StartPath string
	// FirstRun opens the window on the first-run setup's route
	// (services.OnboardingRoute) when a module registered it: a first start
	// (services.StartRoute). Unmatched paths still land on the start page.
	FirstRun bool
	// Hidden starts with no window shown (--hidden, a login autostart):
	// playback, MPRIS and the tray only, until something asks for a
	// window (the tray, a second launch, MPRIS Raise). Without a tray the
	// window shows anyway: nothing could bring it back.
	Hidden bool
	// Icon is the window icon, a PNG.
	Icon []byte
	// TrayIcon is the tray's icon, a small PNG; TrayTemplate, when set
	// (macOS), a template image the menu bar tints, used instead.
	TrayIcon, TrayTemplate []byte
	// OpenURL opens a web page in the system browser (Env.OpenURL); nil
	// reports an error.
	OpenURL func(url string) error
	// Copy puts text on the system clipboard (Env.Copy); nil reports failure.
	Copy func(text string) bool
}

// App is the native UI: the main window and what it shows.
type App struct {
	opts Options
	log  *slog.Logger
	env  *module.Env
	// system is SystemService, whose FirstPaint opens the network; nil
	// without services.
	system interface{ FirstPaint() }
	// playback seeds the mirror; nil without services.
	playback *services.PlaybackService
	// firstPaint runs once, on the first frame drawn.
	firstPaint func()

	// Motion artwork: the Motion artwork setting as last seen, the release
	// of the policy's Cover while something covers the page, and what has
	// been sent to MotionService.Report (nil without services). Main thread.
	motionOn   bool
	uncover    func()
	sendReport func(services.MotionReport)
	reportAt   time.Time
	reported   bool
	lastReport services.MotionReport

	mods   []module.Module
	routes []module.Route
	nav    [3][]module.NavItem
	keys   *dispatcher
	router *ui.Router
	start  string

	mem *windowMemory

	// platform is PlatformService (the title bar, the tray capability);
	// nil without services.
	platform *services.PlatformService
	// mini is the module's mini player content (module.Slots.Mini; nil:
	// none), tray the tray icon (nil: none). Main thread.
	mini func(c *ui.Context)
	tray *tray
	// drawnTitleBar: the main window has a hidden title bar, so the top
	// bar and the sidebar's header drag it and keep room for its buttons.
	drawnTitleBar bool
	// lastRoute is the route last handed to DesktopService.SetLastRoute;
	// routeSeen the route shown since routeAt (main thread).
	lastRoute string
	routeSeen string
	routeAt   time.Time
	// banner is the status banner's (main thread).
	banner banner

	mu        sync.Mutex
	win       *mygo.Window
	miniWin   *mygo.Window
	miniShown bool

	locale *localizer

	paintOnce sync.Once
	sidebar   bool // shown; Ctrl+B
}

// New builds the UI, puts the tray icon up and opens the main window
// (hidden with Options.Hidden and a tray). Call it in MyGo's WhenReady,
// which is on the main thread.
func New(opts Options) *App {
	a := newApp(opts, factories)
	if opts.Windows != nil {
		icon, template := opts.TrayIcon, false
		if len(opts.TrayTemplate) > 0 {
			icon, template = opts.TrayTemplate, true
		}
		a.tray = a.newTray(icon, template)
		opts.Windows.setTray(a.tray != nil)
	}
	a.openWindow()
	return a
}

// newApp builds everything but the window.
func newApp(opts Options, factories []module.Factory) *App {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	if opts.Bus == nil {
		opts.Bus = store.NewBus(opts.Log)
	}
	a := &App{opts: opts, log: opts.Log, sidebar: true}
	// The interface font, before any window: text built earlier would use
	// the system's sans.
	if err := theme.RegisterFonts(); err != nil {
		a.log.Error("interface font", "err", err)
	}
	if opts.Core != nil {
		a.system = services.Service[services.SystemService](opts.Core)
		a.firstPaint = func() { go a.system.FirstPaint() }
		a.playback = services.Service[services.PlaybackService](opts.Core)
		a.platform = services.Service[services.PlatformService](opts.Core)
	}
	if opts.Settings != nil {
		a.mem = newWindowMemory(opts.Settings, a.log)
	}

	if opts.OpenURL == nil {
		opts.OpenURL = func(string) error { return errors.New("no way to open a web page") }
	}
	if opts.Copy == nil {
		opts.Copy = func(string) bool { return false }
	}
	post := a.postFunc()
	win := store.NewWindow()
	a.env = &module.Env{
		Core:     opts.Core,
		Bus:      opts.Bus,
		Queries:  store.NewClient(post),
		Play:     store.NewPlayback(opts.Bus, opts.Now),
		Art:      kit.NewArtLoader(opts.Art, post),
		Actions:  kit.NewActionRegistry(),
		Players:  module.NewPlayers(opts.Core, opts.Now),
		Window:   win,
		Video:    store.NewVideoView(opts.Video, win),
		Post:     post,
		Surfaces: store.NewSurfaces(),
		OpenURL:  opts.OpenURL,
		Copy:     opts.Copy,
		Log:      a.log,
		Version:  opts.Version,
		Dev:      opts.Dev,
	}
	a.env.Motion = a.newMotion(post)
	if opts.Core != nil {
		a.env.Queue = kit.NewQueueCommandsFor(opts.Core, a.env.Play, post)
		a.env.ItemStates = kit.NewItemStates(a.env.Queries, post, services.Service[services.LibraryService](opts.Core))
		a.env.ItemStates.Follow(opts.Bus)
	}
	a.env.HasRoute = a.hasRoute
	for _, f := range factories {
		m := f(a.env)
		if slices.ContainsFunc(a.mods, func(o module.Module) bool { return o.ID == m.ID }) {
			panic("module " + m.ID + " registered twice")
		}
		a.mods = append(a.mods, m)
		a.env.Actions.Register(m.Actions...)
		a.routes = append(a.routes, m.Routes...)
		for _, n := range m.Nav {
			a.nav[n.Section] = append(a.nav[n.Section], n)
		}
		if a.mini == nil && m.Slots.Mini != nil {
			a.mini = m.Slots.Mini
		}
	}
	for i := range a.nav {
		slices.SortStableFunc(a.nav[i], func(x, y module.NavItem) int { return cmp.Compare(x.Order, y.Order) })
	}
	a.start = a.startPath()
	initial := a.start
	if r := a.resumeRoute(); r != "" {
		initial = r
	}
	if opts.FirstRun && slices.ContainsFunc(a.routes, func(r module.Route) bool { return r.Pattern == services.OnboardingRoute }) {
		initial = services.OnboardingRoute
	}
	a.lastRoute = initial
	a.router = ui.NewRouter(initial)
	a.env.Nav = a.router
	a.env.Actions.SetNavigator(a.router.Push)
	a.env.Actions.SetPoster(post)
	kit.SetToastPoster(post)

	var list []module.Shortcut
	list = append(list, navShortcuts(a.nav, a.router.Push)...)
	list = append(list, a.shellShortcuts()...)
	for _, m := range a.mods {
		list = append(list, m.Shortcuts...)
	}
	var overrides config.Shortcuts
	if opts.Settings != nil {
		overrides = opts.Settings.Get().Shortcuts
	}
	a.keys = newDispatcher(list, overrides)
	a.env.Shortcuts = a.keys.bindings

	i18n.SetPoster(post)
	i18n.SetLogger(a.log)
	return a
}

// startPath is where the window opens: the start page of the settings when
// a module registered it, else the first of the pages a start page may be
// (config.StartPages, v0.2.0's choices, Home first) that one did, else the
// first sidebar link.
func (a *App) startPath() string {
	if a.opts.StartPath != "" {
		return a.opts.StartPath
	}
	if a.opts.Settings != nil {
		page := a.opts.Settings.Get().Desktop.StartPage
		if slices.ContainsFunc(a.routes, func(r module.Route) bool { return r.Pattern == page }) {
			return page
		}
	}
	for _, page := range config.StartPages {
		if slices.ContainsFunc(a.routes, func(r module.Route) bool { return r.Pattern == page }) {
			return page
		}
	}
	for _, list := range a.nav {
		if len(list) > 0 {
			return list[0].To
		}
	}
	return "/"
}

// resumeRoute is the route the window was last on, when Settings → General →
// Open on is "The page you were last on" and a module still shows that page;
// "" otherwise.
func (a *App) resumeRoute() string {
	if a.opts.StartPath != "" || a.opts.Settings == nil {
		return ""
	}
	d := a.opts.Settings.Get().Desktop
	if !d.ResumeLastRoute || d.LastRoute == "" || !a.hasRoute(d.LastRoute) {
		return ""
	}
	return d.LastRoute
}

// shellShortcuts are the shell's own: the sidebar. (Back and forward are
// MyGo's router's: Alt+Left and Alt+Right, the mouse's side buttons.)
func (a *App) shellShortcuts() []module.Shortcut {
	return []module.Shortcut{{
		ID:    "shell.sidebar",
		Label: func() string { return i18n.T("Show or hide the sidebar") },
		Group: "view",
		Keys:  []string{"Ctrl+B"},
		Scope: "global",
		Icon:  "panel-left",
		Run:   func() { a.sidebar = !a.sidebar },
	}}
}

// inspectorRequested is SONA_INSPECTOR=1.
func inspectorRequested() bool { return os.Getenv("SONA_INSPECTOR") == "1" }

// openWindow creates the main window and connects everything to it.
func (a *App) openWindow() {
	w := config.Window{Width: config.DefaultWindowWidth, Height: config.DefaultWindowHeight}
	if a.mem != nil {
		w = a.mem.current()
	}
	// The inspector is only in development builds, and only with
	// SONA_INSPECTOR=1: windows of any other build never offer it.
	devtools := mygo.DevToolsDisabled
	if a.opts.Dev && inspectorRequested() {
		devtools = mygo.DevToolsEnabled
	}
	hidden := a.hidden()
	opts := mygo.WindowOptions{
		Title:     windowTitle,
		Width:     w.Width,
		Height:    w.Height,
		Maximized: w.Maximised,
		MinWidth:  config.MinWindowWidth,
		MinHeight: config.MinWindowHeight,
		Hidden:    hidden,
		Content:   ui.View(a.view),
		Page:      mygo.PageOptions{DevTools: devtools},
	}
	a.chrome(&opts)
	win := mygo.NewWindow(opts)
	if len(a.opts.Icon) > 0 {
		if err := win.SetIcon(a.opts.Icon); err != nil {
			a.log.Warn("window icon", "err", err)
		}
	}
	a.mu.Lock()
	a.win = win
	a.mu.Unlock()
	if a.opts.Windows != nil {
		hooks := windowHooks{
			closeQuits: func() bool {
				return a.opts.Settings != nil && a.opts.Settings.Get().Desktop.CloseAction == config.CloseQuits
			},
			miniShown: a.miniShownChanged,
			devTools:  func() { win.Page().ToggleDevTools() },
		}
		if a.mini != nil {
			hooks.newMini = a.newMiniWindow
		}
		a.opts.Windows.attach(win, a.mem, hooks)
	}
	a.followVisibility(win)
	a.followWindow(win)
	if hidden {
		// No window shows, so no first frame comes: the network opens now,
		// and the views that animate wait for the window.
		a.env.Motion.SetVisible(false)
		a.env.Window.SetFocused(false)
		a.paintOnce.Do(a.painted)
		a.log.Info("startup: hidden, in the tray")
	}
	a.connect()
	a.devHook(win)
	if a.opts.Dev && inspectorRequested() {
		win.OnReadyToShow(func() { win.Page().ToggleDevTools() })
	}
}

// followWindow binds Env.Window to the main window: its full screen, and its
// keyboard focus from the focus and blur events, with a redraw on each, so
// views that hold while the window is unfocused build again.
func (a *App) followWindow(win *mygo.Window) {
	a.env.Window.Bind(win.IsFullScreen, win.SetFullScreen)
	set := func(on bool) func() {
		return func() {
			a.env.Window.SetFocused(on)
			a.post(func() {})
		}
	}
	win.OnFocus(set(true))
	win.OnBlur(set(false))
}

// connect is the shell connecting to Go: the settings the window follows,
// the playback snapshots, then every module's Init. It runs on the main
// thread.
func (a *App) connect() {
	a.opts.Bus.SetPoster(a.postFunc())
	a.followSettings()
	a.followOpens()
	if a.tray != nil {
		a.tray.follow(a.opts.Bus)
	}
	if a.playback != nil {
		a.env.Play.Seed(a.playback.State(), a.playback.Queue())
	}
	for _, m := range a.mods {
		if m.Init != nil {
			m.Init()
		}
	}
}

// languageChanged writes the language shown into what no frame builds: the
// tray's menu and the mini player's title.
func (a *App) languageChanged() {
	if a.tray != nil {
		a.tray.render()
	}
	a.mu.Lock()
	m := a.miniWin
	a.mu.Unlock()
	if m != nil {
		m.SetTitle(i18n.T("Sona mini player"))
	}
}

// postFunc is the Poster: Options.Poster or the window's.
func (a *App) postFunc() store.Poster {
	if a.opts.Poster != nil {
		return a.opts.Poster
	}
	return a.post
}

// post runs fn on the main thread and redraws the windows (store.Poster):
// the main window, and the mini player while it shows.
func (a *App) post(fn func()) {
	a.mu.Lock()
	win, mini := a.win, a.miniWin
	if !a.miniShown {
		mini = nil
	}
	a.mu.Unlock()
	if win == nil {
		fn()
		return
	}
	win.Update(fn)
	if mini != nil {
		mini.Update(func() {})
	}
}

// Window is the main window (nil before New opened it).
func (a *App) Window() *mygo.Window {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.win
}

// Close releases what the UI holds; call it after MyGo's Run returns.
func (a *App) Close() {
	a.env.Motion.Close()
	a.env.Art.Close()
	if a.mem != nil {
		a.mem.flush()
	}
}

// followSettings applies the theme and the effects setting and follows
// their changes, and the language with them.
func (a *App) followSettings() {
	if a.opts.Settings == nil {
		return
	}
	apply := func(s config.Settings) {
		theme.SetMode(themeMode(s.Theme))
		theme.SetReduceEffects(s.ReduceEffects)
		a.keys.rebind(s.Shortcuts)
		a.motionOn = s.MotionArtwork.Enabled
	}
	apply(a.opts.Settings.Get())
	store.On(a.opts.Bus, services.EventSettingsChanged, func(s config.Settings) {
		// Every feature's copy of the settings (store.UseSettings) follows Go:
		// a save builds the whole settings from it.
		store.Set(a.env.Queries, store.KeySettings, s)
		apply(s)
		a.locale.apply()
	})
	a.initLocale()
}

// themeMode maps config.Settings.Theme to the theme's mode.
func themeMode(s string) theme.Mode {
	switch s {
	case config.ThemeLight:
		return theme.ModeLight
	case config.ThemeDark:
		return theme.ModeDark
	}
	return theme.ModeSystem
}
