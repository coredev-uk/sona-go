// Package windowchrome decides who draws the title bar of Sona's main
// window and where its buttons go:
//   - macOS keeps the system's own buttons (the traffic lights), inset into
//     Sona's top bar;
//   - Windows and the stacking Linux desktops (GNOME, KDE Plasma, Cinnamon,
//     Xfce, …) get Sona's own title bar, a window with a hidden title bar under
//     Sona's top bar and MyGo's window buttons laid out as the desktop lays
//     out its own;
//   - tiling compositors (sway, Hyprland, i3, niri, …) and desktops Sona
//     does not recognise keep the desktop's: its own title bar and frame,
//     or none at all on a tiling compositor.
//
// Settings → General → Title bar (config.TitleBar) overrides the automatic
// choice on Windows and Linux. The package is portable: the detection reads
// environment variables and files, and the GTK setting is read by
// MyGo on Linux.
package windowchrome

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/coredev-uk/sona-go/internal/config"
)

// The window buttons Sona draws, by the names GTK's decoration layout uses.
const (
	Minimize = "minimize"
	Maximize = "maximize"
	Close    = "close"
)

// How the platform's own title bar relates to Sona's window
// ([Platform.Native]).
const (
	// NativeInset: the system's buttons sit inside Sona's top bar, which
	// keeps room for them (macOS: hidden title bar, full-size content).
	NativeInset = "inset"
	// NativeFrame: the desktop draws a title bar and frame around Sona's
	// window unless Sona draws its own (Windows, Linux).
	NativeFrame = "frame"
)

// What a window ends up with ([Platform.Resolve]).
const (
	// KindNativeInset: the system's buttons over Sona's top bar (macOS).
	KindNativeInset = "native-inset"
	// KindDrawn: Sona's own title bar in a frameless window.
	KindDrawn = "drawn"
	// KindNone: Sona draws no title bar controls; the desktop's own title
	// bar (or none, on a tiling compositor) frames the window.
	KindNone = "none"
)

// Layout is the window buttons on each side of a title bar, in reading
// order: Start runs left to right from the window's left edge, End left to
// right ending at its right edge.
type Layout struct {
	Start []string
	End   []string
}

// MacLayout is the order of macOS's traffic lights at the window's start.
func MacLayout() Layout { return Layout{Start: []string{Close, Minimize, Maximize}} }

// DefaultLayout is Windows' caption buttons, and what a Linux desktop uses
// when its own layout can't be read: GTK's own default ("menu:minimize,maximize,close").
func DefaultLayout() Layout { return Layout{End: []string{Minimize, Maximize, Close}} }

// Platform is how title bars work for Sona on this OS and desktop. It is
// fixed for the process; the user's choice (config.TitleBar) is applied
// on top by [Platform.Resolve].
type Platform struct {
	// Native is NativeInset or NativeFrame.
	Native string
	// Automatic is what config.TitleBarAuto stands for here:
	// config.TitleBarSona or config.TitleBarDesktop.
	Automatic string
	// Desktop names the desktop or compositor Sona recognised ("GNOME",
	// "KDE Plasma", "sway"); "" when it recognised none.
	Desktop string
	// Layout returns the desktop's button layout. It may need the GUI
	// toolkit (GTK on Linux), so it is called only after the app started.
	Layout func() Layout
}

// Resolve applies a title bar style (config.TitleBar.Style) to the
// platform: the window's kind and whether Sona draws window buttons.
// macOS ignores the style.
func (p Platform) Resolve(style string) (kind string, buttons bool) {
	if p.Native == NativeInset {
		return KindNativeInset, false
	}
	if style == config.TitleBarAuto || style == "" {
		style = p.Automatic
	}
	switch style {
	case config.TitleBarSona:
		return KindDrawn, true
	case config.TitleBarSonaNoButtons:
		return KindDrawn, false
	}
	return KindNone, false
}

// Desktop is the Linux session's desktop, as far as title bars go.
type Desktop struct {
	// Name is the desktop's display name ("GNOME", "sway"); "" when
	// unknown.
	Name string
	// Tiling: a tiling compositor or window manager. Stacking: a stacking
	// desktop Sona knows. Neither: unknown, where Sona leaves the title
	// bar to the desktop.
	Tiling, Stacking bool
	// KDE: KDE Plasma, whose buttons are KWin's (kwinrc).
	KDE bool
	// ServerDecorated: KDE Plasma on Wayland, where KWin decorates a window
	// whose title bar is hidden, so Sona's own cannot replace it.
	ServerDecorated bool
}

// Automatic is what config.TitleBarAuto stands for on this desktop: Sona's
// own title bar on a stacking desktop it knows, else the desktop's (KWin's,
// on Plasma under Wayland, which keeps its title bar over a hidden one).
func (d Desktop) Automatic() string {
	if d.Stacking && !d.ServerDecorated {
		return config.TitleBarSona
	}
	return config.TitleBarDesktop
}

// desktopNames maps XDG_CURRENT_DESKTOP / XDG_SESSION_DESKTOP /
// DESKTOP_SESSION values (lower case) to display names and kinds.
var desktopNames = map[string]Desktop{
	// Stacking desktops.
	"gnome":          {Name: "GNOME", Stacking: true},
	"gnome-classic":  {Name: "GNOME", Stacking: true},
	"gnome-xorg":     {Name: "GNOME", Stacking: true},
	"kde":            {Name: "KDE Plasma", Stacking: true, KDE: true},
	"plasma":         {Name: "KDE Plasma", Stacking: true, KDE: true},
	"plasmawayland":  {Name: "KDE Plasma", Stacking: true, KDE: true},
	"x-cinnamon":     {Name: "Cinnamon", Stacking: true},
	"cinnamon":       {Name: "Cinnamon", Stacking: true},
	"xfce":           {Name: "Xfce", Stacking: true},
	"mate":           {Name: "MATE", Stacking: true},
	"budgie":         {Name: "Budgie", Stacking: true},
	"budgie-desktop": {Name: "Budgie", Stacking: true},
	"pantheon":       {Name: "Pantheon", Stacking: true},
	"unity":          {Name: "Unity", Stacking: true},
	"lxqt":           {Name: "LXQt", Stacking: true},
	"lxde":           {Name: "LXDE", Stacking: true},
	"deepin":         {Name: "Deepin", Stacking: true},
	"cosmic":         {Name: "COSMIC", Stacking: true},
	"enlightenment":  {Name: "Enlightenment", Stacking: true},
	"ukui":           {Name: "UKUI", Stacking: true},
	"labwc":          {Name: "labwc", Stacking: true},
	"wayfire":        {Name: "Wayfire", Stacking: true},
	"openbox":        {Name: "Openbox", Stacking: true},
	// Tiling compositors and window managers.
	"sway":          {Name: "sway", Tiling: true},
	"hyprland":      {Name: "Hyprland", Tiling: true},
	"i3":            {Name: "i3", Tiling: true},
	"river":         {Name: "river", Tiling: true},
	"niri":          {Name: "niri", Tiling: true},
	"bspwm":         {Name: "bspwm", Tiling: true},
	"dwm":           {Name: "dwm", Tiling: true},
	"dwl":           {Name: "dwl", Tiling: true},
	"awesome":       {Name: "awesome", Tiling: true},
	"qtile":         {Name: "Qtile", Tiling: true},
	"xmonad":        {Name: "xmonad", Tiling: true},
	"herbstluftwm":  {Name: "herbstluftwm", Tiling: true},
	"leftwm":        {Name: "LeftWM", Tiling: true},
	"spectrwm":      {Name: "spectrwm", Tiling: true},
	"miracle-wm":    {Name: "miracle-wm", Tiling: true},
	"jay":           {Name: "Jay", Tiling: true},
	"mango":         {Name: "Mango", Tiling: true},
	"pinnacle":      {Name: "Pinnacle", Tiling: true},
	"komorebi":      {Name: "komorebi", Tiling: true},
	"hyprland-uwsm": {Name: "Hyprland", Tiling: true},
}

// compositorSockets are environment variables only a tiling compositor
// sets for its clients; they win over the desktop names, which a session
// may inherit from elsewhere.
var compositorSockets = []struct{ env, name string }{
	{"SWAYSOCK", "sway"},
	{"HYPRLAND_INSTANCE_SIGNATURE", "Hyprland"},
	{"NIRI_SOCKET", "niri"},
	{"I3SOCK", "i3"},
}

// Detect recognises the Linux desktop from the environment: a tiling
// compositor's socket variable first, then the first known name in
// XDG_CURRENT_DESKTOP (a colon-separated list, "ubuntu:GNOME"),
// XDG_SESSION_DESKTOP and DESKTOP_SESSION (KDE Plasma with WAYLAND_DISPLAY set and
// GDK_BACKEND not x11 is ServerDecorated). An unknown desktop is neither
// tiling nor stacking.
func Detect(getenv func(string) string) Desktop {
	for _, s := range compositorSockets {
		if getenv(s.env) != "" {
			return Desktop{Name: s.name, Tiling: true}
		}
	}
	for _, env := range []string{"XDG_CURRENT_DESKTOP", "XDG_SESSION_DESKTOP", "DESKTOP_SESSION"} {
		for name := range strings.SplitSeq(getenv(env), ":") {
			// DESKTOP_SESSION may be a session file path.
			name = strings.ToLower(strings.TrimSpace(filepath.Base(name)))
			if d, ok := desktopNames[name]; ok {
				// GDK_BACKEND may list backends; the first is tried first.
				first, _, _ := strings.Cut(getenv("GDK_BACKEND"), ",")
				d.ServerDecorated = d.KDE && getenv("WAYLAND_DISPLAY") != "" && strings.TrimSpace(first) != "x11"
				return d
			}
		}
	}
	return Desktop{}
}

// ParseGTK reads a GTK decoration layout ("icon,menu:minimize,maximize,close",
// GTK's gtk-decoration-layout, GNOME's button-layout): the buttons before
// the colon go at the start, those after it at the end. Only the buttons
// Sona draws are kept, each once.
func ParseGTK(layout string) Layout {
	start, end, _ := strings.Cut(layout, ":")
	var l Layout
	seen := map[string]bool{}
	pick := func(side string) []string {
		var out []string
		for b := range strings.SplitSeq(side, ",") {
			b = strings.TrimSpace(b)
			if (b == Minimize || b == Maximize || b == Close) && !seen[b] {
				seen[b] = true
				out = append(out, b)
			}
		}
		return out
	}
	l.Start = pick(start)
	l.End = pick(end)
	return l
}

// KWin's defaults when kwinrc does not set the buttons.
const (
	kwinDefaultLeft  = "MS"
	kwinDefaultRight = "HIAX"
)

// ParseKWin reads KWin's ButtonsOnLeft and ButtonsOnRight ("MS", "HIAX"):
// I is minimise, A maximise, X close; the other letters (menu, on all
// desktops, help, keep above, shade, spacer) are buttons Sona doesn't draw.
func ParseKWin(left, right string) Layout {
	seen := map[string]bool{}
	pick := func(side string) []string {
		var out []string
		for _, c := range side {
			var b string
			switch c {
			case 'I':
				b = Minimize
			case 'A':
				b = Maximize
			case 'X':
				b = Close
			default:
				continue
			}
			if !seen[b] {
				seen[b] = true
				out = append(out, b)
			}
		}
		return out
	}
	return Layout{Start: pick(left), End: pick(right)}
}

// KWinRC is KWin's configuration file: $XDG_CONFIG_HOME/kwinrc, else
// ~/.config/kwinrc; "" when neither can be found.
func KWinRC(getenv func(string) string) string {
	if dir := getenv("XDG_CONFIG_HOME"); dir != "" && filepath.IsAbs(dir) {
		return filepath.Join(dir, "kwinrc")
	}
	if home := getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", "kwinrc")
	}
	return ""
}

// ReadKWin reads the button layout from a kwinrc file
// ([org.kde.kdecoration2] ButtonsOnLeft / ButtonsOnRight, KWin's defaults
// for a missing key). A missing file is an error wrapping os.ErrNotExist.
func ReadKWin(path string) (Layout, error) {
	f, err := os.Open(path)
	if err != nil {
		return Layout{}, err
	}
	defer func() { _ = f.Close() }()
	left, right, err := kwinButtons(f)
	if err != nil {
		return Layout{}, err
	}
	return ParseKWin(left, right), nil
}

// kwinButtons scans a KConfig file for the decoration's button keys.
func kwinButtons(r io.Reader) (left, right string, err error) {
	left, right = kwinDefaultLeft, kwinDefaultRight
	group := ""
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			group = line[1 : len(line)-1]
			continue
		}
		if group != "org.kde.kdecoration2" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		// KConfig flags such as "ButtonsOnLeft[$i]".
		key, _, _ = strings.Cut(strings.TrimSpace(key), "[")
		switch key {
		case "ButtonsOnLeft":
			left = strings.TrimSpace(value)
		case "ButtonsOnRight":
			right = strings.TrimSpace(value)
		}
	}
	return left, right, sc.Err()
}
