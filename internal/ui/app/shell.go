package app

import (
	"net/url"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// routeSaveDelay is how long a route must stay before it is remembered:
// one write per page, not one per step through the history (v0.2.0's
// useRouteMemory).
const routeSaveDelay = time.Second

// The mini player window: fixed size, which compositors float rather than
// tile (Hyprland, sway, GNOME), as v0.2.0's.
const (
	miniWidth  = 420
	miniHeight = 132
)

// trafficLightsHeight is the height of macOS's window buttons, centred in
// the top bar.
const trafficLightsHeight = 14

// rememberRoute hands the main window's route to DesktopService.SetLastRoute
// once it has stayed routeSaveDelay, while Settings → General → Open on is
// "The page you were last on" (the service keeps nothing otherwise). The
// first-run setup's route is never remembered. It runs every frame, cheaply:
// a string comparison, and a frame asked for when the delay ends.
func (a *App) rememberRoute(c *ui.Context) {
	loc := a.router.Location()
	now := c.Now()
	if loc != a.routeSeen {
		a.routeSeen, a.routeAt = loc, now
	}
	if loc == a.lastRoute {
		return
	}
	if wait := routeSaveDelay - now.Sub(a.routeAt); wait > 0 {
		c.After(wait)
		return
	}
	a.lastRoute = loc
	if a.opts.Core == nil || a.opts.Settings == nil || !a.opts.Settings.Get().Desktop.ResumeLastRoute {
		return
	}
	if u, err := url.Parse(loc); err == nil && u.Path == services.OnboardingRoute {
		return
	}
	desktop := services.Service[services.DesktopService](a.opts.Core)
	go func() {
		// Losing one route is harmless: the next page saves again.
		if err := desktop.SetLastRoute(loc); err != nil {
			a.log.Warn("remember route", "route", loc, "err", err)
		}
	}()
}

// routeForOpen is the page an Apple Music link opens (v0.2.0's
// routeForOpen, app/open.ts): a song shared within its album opens the
// album with the song marked; "" for a kind Sona has no page for.
func routeForOpen(t services.OpenTarget) string {
	if t.ID == "" {
		return ""
	}
	id := url.PathEscape(t.ID)
	switch t.Kind {
	case "album", "library-album":
		return "/album/" + id
	case "song":
		if t.AlbumID != "" {
			return "/album/" + url.PathEscape(t.AlbumID) + "?song=" + url.QueryEscape(t.ID)
		}
		return "/song/" + id
	case "artist":
		return "/artist/" + id
	case "playlist", "library-playlist":
		return "/playlist/" + id
	case "music-video":
		return "/music-video/" + id
	case "station":
		return "/station/" + id
	case "curator":
		return "/curator/" + id
	}
	return ""
}

// followOpens opens the page of every Apple Music link Sona is launched
// with ("app:open": the command line, a second launch, the system's URL
// handler), when a module shows that page; others are ignored, as v0.2.0
// did.
func (a *App) followOpens() {
	store.On(a.opts.Bus, services.EventAppOpen, func(t services.OpenTarget) {
		to := routeForOpen(t)
		if to == "" || !a.hasRoute(to) {
			a.log.Info("open link: no page for it", "kind", t.Kind)
			return
		}
		a.router.Push(to)
	})
}

// chrome is the main window's title bar, from the platform and Settings →
// General → Title bar as the window is created (a change applies from the
// next start): macOS keeps its buttons inset into Sona's top bar; Linux and
// Windows either have the desktop's own title bar, or a hidden one under
// Sona's top bar with MyGo's window buttons in the desktop's layout (GTK's
// gtk-decoration-layout), or with none.
func (a *App) chrome(o *mygo.WindowOptions) {
	if a.platform == nil {
		return
	}
	wc := a.platform.Capabilities().WindowControls
	kind, buttons := windowchrome.Platform{Native: wc.Native, Automatic: wc.Automatic}.Resolve(wc.Style)
	bar := int(theme.Current().Metric["top-bar"])
	switch kind {
	case windowchrome.KindNativeInset:
		o.TitleBarStyle = mygo.TitleBarHidden
		o.TrafficLightPosition = &mygo.Point{X: 18, Y: (bar - trafficLightsHeight) / 2}
	case windowchrome.KindDrawn:
		o.TitleBarStyle = mygo.TitleBarHidden
		o.TitleBarHeight = bar
		o.HideWindowButtons = !buttons
	default:
		return
	}
	a.drawnTitleBar = true
}

// hidden reports whether the main window starts hidden: --hidden with a
// tray to bring it back.
func (a *App) hidden() bool {
	return a.opts.Hidden && a.opts.Windows != nil && a.opts.Windows.HasTray()
}

// newMiniWindow creates the mini player window, hidden: the module's content
// (module.Slots.Mini) in a fixed-size frameless window above the others. Its
// close button does what closing the main window does.
func (a *App) newMiniWindow() native {
	m := mygo.NewWindow(mygo.WindowOptions{
		Title:           i18n.T("Sona mini player"),
		Width:           miniWidth,
		Height:          miniHeight,
		MinWidth:        miniWidth,
		MinHeight:       miniHeight,
		MaxWidth:        miniWidth,
		MaxHeight:       miniHeight,
		DisableResize:   true,
		DisableMaximize: true,
		Frameless:       true,
		AlwaysOnTop:     true,
		Hidden:          true,
		Content:         ui.View(a.miniView),
	})
	if len(a.opts.Icon) > 0 {
		if err := m.SetIcon(a.opts.Icon); err != nil {
			a.log.Warn("mini player icon", "err", err)
		}
	}
	m.OnClose(func(e *mygo.CloseEvent) { a.opts.Windows.closeEvent(m, e) })
	a.mu.Lock()
	a.miniWin = m
	a.mu.Unlock()
	return m
}

// miniView is the mini player window's content: the theme, the dialogs and
// toasts layer, and the module's view.
func (a *App) miniView(c *ui.Context) {
	c.Root().Direction(direction())
	theme.Apply(c)
	ui.Column(c).Fill().Background(theme.C("surface")).Children(func() {
		if a.mini != nil {
			a.mini(c)
		}
	})
}

// miniShownChanged follows the mini player's shown state: the tray's item,
// and the redraws post asks of it.
func (a *App) miniShownChanged(shown bool) {
	a.mu.Lock()
	a.miniShown = shown
	a.mu.Unlock()
	if a.tray != nil {
		a.tray.setMini(shown)
	}
}
