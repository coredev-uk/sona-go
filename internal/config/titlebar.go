package config

// Title bar styles for [TitleBar.Style] (Settings → Desktop → Title bar;
// not offered on macOS, where the window always keeps the system's own
// buttons inside Sona's top bar).
const (
	// TitleBarAuto lets Sona choose: its own title bar on Windows and on
	// the stacking Linux desktops it knows (GNOME, KDE Plasma, Cinnamon,
	// …), the desktop's elsewhere (tiling compositors, unknown desktops).
	TitleBarAuto = "auto"
	// TitleBarSona is Sona's own title bar (a frameless window) with its
	// own minimise, maximise and close buttons, laid out as the desktop
	// lays out its own.
	TitleBarSona = "sona"
	// TitleBarSonaNoButtons is Sona's own title bar without window
	// buttons: for tiling window managers Sona did not recognise.
	TitleBarSonaNoButtons = "sona-no-buttons"
	// TitleBarDesktop leaves the title bar to the desktop: its own title
	// bar and frame above Sona's top bar, or none on a tiling compositor.
	TitleBarDesktop = "desktop"
)

// TitleBar is who draws the main window's title bar and buttons.
type TitleBar struct {
	// Style is one of the TitleBar* constants; "auto" by default.
	Style string `json:"style"`
}

// normalized replaces an unknown style with "auto".
func (t TitleBar) normalized() TitleBar {
	switch t.Style {
	case TitleBarAuto, TitleBarSona, TitleBarSonaNoButtons, TitleBarDesktop:
	default:
		t.Style = TitleBarAuto
	}
	return t
}
