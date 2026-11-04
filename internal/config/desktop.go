package config

import (
	"slices"
	"strings"
)

// Close actions for [Desktop.CloseAction].
const (
	// CloseToBackground hides the window and keeps playing; the tray, the
	// launcher (single instance) and MPRIS Raise bring it back.
	CloseToBackground = "background"
	// CloseQuits exits Sona when its window is closed.
	CloseQuits = "quit"
)

// StartPages are the routes [Desktop.StartPage] may name: the sidebar's top
// level pages. The UI labels them.
var StartPages = []string{
	"/",
	"/new",
	"/charts",
	"/radio",
	"/library/recently-added",
	"/library/artists",
	"/library/albums",
	"/library/songs",
	"/library/playlists",
}

// maxRouteLen bounds [Desktop.LastRoute]: a route with its search string,
// never a document.
const maxRouteLen = 1024

// Desktop is how Sona behaves as a desktop app: what closing the window
// does, starting at login, and where the main window opens.
type Desktop struct {
	// CloseAction is "background" (default: keep playing, hidden, with
	// the tray) or "quit".
	CloseAction string `json:"closeAction"`
	// Autostart mirrors the XDG autostart entry
	// ($XDG_CONFIG_HOME/autostart/dev.coredev.Sona.desktop): set, the
	// entry exists. The file is the truth at start-up, so removing it
	// from the desktop's own settings turns this off.
	Autostart bool `json:"autostart"`
	// StartHidden makes the autostart entry start Sona in the tray, with
	// no window (--hidden).
	StartHidden bool `json:"startHidden"`
	// StartPage is the route the main window opens on (one of
	// [StartPages]).
	StartPage string `json:"startPage"`
	// ResumeLastRoute opens the main window on LastRoute instead.
	ResumeLastRoute bool `json:"resumeLastRoute"`
	// LastRoute is the main window's route ("/album/123?song=4"), saved
	// by the UI as it navigates.
	LastRoute string `json:"lastRoute"`
}

func defaultDesktop() Desktop {
	return Desktop{CloseAction: CloseToBackground, StartHidden: true, StartPage: "/"}
}

// normalized replaces an unknown close action with "background" and an
// unknown start page with "/", and drops a last route that is not an
// in-app path.
func (d Desktop) normalized() Desktop {
	if d.CloseAction != CloseToBackground && d.CloseAction != CloseQuits {
		d.CloseAction = CloseToBackground
	}
	if !slices.Contains(StartPages, d.StartPage) {
		d.StartPage = "/"
	}
	if !validRoute(d.LastRoute) {
		d.LastRoute = ""
	}
	return d
}

// validRoute reports whether r is an in-app route: absolute, without a
// scheme, host, fragment or control characters, and short.
func validRoute(r string) bool {
	if r == "" || len(r) > maxRouteLen || !strings.HasPrefix(r, "/") || strings.HasPrefix(r, "//") {
		return false
	}
	return !strings.ContainsFunc(r, func(c rune) bool { return c < 0x20 || c == 0x7f || c == '#' })
}

// StartRoute is the route the main window opens on: the last route when
// resuming and one is saved, else the start page.
func (d Desktop) StartRoute() string {
	if d.ResumeLastRoute && d.LastRoute != "" {
		return d.LastRoute
	}
	return d.StartPage
}

// The main window's default and minimum sizes, in logical pixels.
const (
	DefaultWindowWidth  = 1280
	DefaultWindowHeight = 820
	MinWindowWidth      = 940
	MinWindowHeight     = 600
	// maxWindowEdge rejects nonsense from a hand-edited file.
	maxWindowEdge = 16384
)

// Window is the main window's remembered state. Wayland gives clients no
// position, so only the size and the maximised state are kept. Width and
// Height are the size when not maximised, so un-maximising after a start
// restores it.
type Window struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

func defaultWindow() Window {
	return Window{Width: DefaultWindowWidth, Height: DefaultWindowHeight}
}

// normalized resets a missing size to the default and clamps the rest to
// [minimum, maxWindowEdge].
func (w Window) normalized() Window {
	if w.Width <= 0 || w.Height <= 0 {
		d := defaultWindow()
		w.Width, w.Height = d.Width, d.Height
	}
	w.Width = min(max(w.Width, MinWindowWidth), maxWindowEdge)
	w.Height = min(max(w.Height, MinWindowHeight), maxWindowEdge)
	return w
}
