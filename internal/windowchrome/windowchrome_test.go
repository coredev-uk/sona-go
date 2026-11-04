package windowchrome

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		name string
		vars map[string]string
		want Desktop
	}{
		{"GNOME", map[string]string{"XDG_CURRENT_DESKTOP": "GNOME"}, Desktop{Name: "GNOME", Stacking: true}},
		{"Ubuntu's GNOME", map[string]string{"XDG_CURRENT_DESKTOP": "ubuntu:GNOME"}, Desktop{Name: "GNOME", Stacking: true}},
		{"KDE Plasma", map[string]string{"XDG_CURRENT_DESKTOP": "KDE"}, Desktop{Name: "KDE Plasma", Stacking: true, KDE: true}},
		{"KDE Plasma on Wayland", map[string]string{"XDG_CURRENT_DESKTOP": "KDE", "WAYLAND_DISPLAY": "wayland-0"}, Desktop{Name: "KDE Plasma", Stacking: true, KDE: true, ServerDecorated: true}},
		{"KDE Plasma on Wayland, GTK on X11", map[string]string{"XDG_CURRENT_DESKTOP": "KDE", "WAYLAND_DISPLAY": "wayland-0", "GDK_BACKEND": "x11"}, Desktop{Name: "KDE Plasma", Stacking: true, KDE: true}},
		{"GNOME on Wayland", map[string]string{"XDG_CURRENT_DESKTOP": "GNOME", "WAYLAND_DISPLAY": "wayland-0"}, Desktop{Name: "GNOME", Stacking: true}},
		{"Cinnamon", map[string]string{"XDG_CURRENT_DESKTOP": "X-Cinnamon"}, Desktop{Name: "Cinnamon", Stacking: true}},
		{"Xfce", map[string]string{"XDG_CURRENT_DESKTOP": "XFCE"}, Desktop{Name: "Xfce", Stacking: true}},
		{"Budgie from the session name", map[string]string{"XDG_SESSION_DESKTOP": "budgie-desktop"}, Desktop{Name: "Budgie", Stacking: true}},
		{"session file path", map[string]string{"DESKTOP_SESSION": "/usr/share/xsessions/mate"}, Desktop{Name: "MATE", Stacking: true}},
		{"sway by name", map[string]string{"XDG_CURRENT_DESKTOP": "sway"}, Desktop{Name: "sway", Tiling: true}},
		{"Hyprland by name", map[string]string{"XDG_CURRENT_DESKTOP": "Hyprland"}, Desktop{Name: "Hyprland", Tiling: true}},
		{"river", map[string]string{"XDG_CURRENT_DESKTOP": "river"}, Desktop{Name: "river", Tiling: true}},
		{"bspwm on X11", map[string]string{"DESKTOP_SESSION": "bspwm"}, Desktop{Name: "bspwm", Tiling: true}},
		// A compositor's own socket wins over a desktop name inherited
		// from the session that launched it.
		{"sway socket under a GNOME name", map[string]string{"XDG_CURRENT_DESKTOP": "GNOME", "SWAYSOCK": "/run/user/1000/sway-ipc.sock"}, Desktop{Name: "sway", Tiling: true}},
		{"Hyprland socket", map[string]string{"HYPRLAND_INSTANCE_SIGNATURE": "abc"}, Desktop{Name: "Hyprland", Tiling: true}},
		{"niri socket", map[string]string{"NIRI_SOCKET": "/run/user/1000/niri.sock"}, Desktop{Name: "niri", Tiling: true}},
		{"i3 socket", map[string]string{"I3SOCK": "/run/user/1000/i3/ipc-socket"}, Desktop{Name: "i3", Tiling: true}},
		{"unknown", map[string]string{"XDG_CURRENT_DESKTOP": "mywm"}, Desktop{}},
		{"nothing set", nil, Desktop{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(env(tc.vars)); got != tc.want {
				t.Errorf("Detect = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// Sona draws its own title bar only where it recognises a stacking
// desktop; tiling and unknown desktops keep the desktop's.
func TestAutomatic(t *testing.T) {
	for d, want := range map[Desktop]string{
		{Name: "GNOME", Stacking: true}: config.TitleBarSona,
		{Name: "sway", Tiling: true}:    config.TitleBarDesktop,
		// KWin keeps its title bar over a hidden one.
		{Name: "KDE Plasma", Stacking: true, KDE: true, ServerDecorated: true}: config.TitleBarDesktop,
		{Name: "KDE Plasma", Stacking: true, KDE: true}:                        config.TitleBarSona,
		{}:                              config.TitleBarDesktop,
	} {
		if got := d.Automatic(); got != want {
			t.Errorf("%+v.Automatic() = %q, want %q", d, got, want)
		}
	}
}

func TestParseGTK(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Layout
	}{
		// GNOME's default: Close only, at the end.
		{"appmenu:close", Layout{End: []string{Close}}},
		// GTK's default.
		{"menu:minimize,maximize,close", Layout{End: []string{Minimize, Maximize, Close}}},
		// macOS-like and elementary's layouts.
		{"close,minimize,maximize:", Layout{Start: []string{Close, Minimize, Maximize}}},
		{"close:maximize", Layout{Start: []string{Close}, End: []string{Maximize}}},
		// Icons, menus and spacers are dropped; a button counts once.
		{"icon,spacer,close:menu,close,minimize", Layout{Start: []string{Close}, End: []string{Minimize}}},
		{" minimize , maximize : close ", Layout{Start: []string{Minimize, Maximize}, End: []string{Close}}},
		// No colon: everything is at the start, as GTK reads it.
		{"close", Layout{Start: []string{Close}}},
		{"", Layout{}},
	} {
		if got := ParseGTK(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseGTK(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestParseKWin(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		want        Layout
	}{
		{"MS", "HIAX", Layout{End: []string{Minimize, Maximize, Close}}},
		{"XIA", "", Layout{Start: []string{Close, Minimize, Maximize}}},
		{"X", "FIA_", Layout{Start: []string{Close}, End: []string{Minimize, Maximize}}},
		{"", "XX", Layout{End: []string{Close}}},
	} {
		if got := ParseKWin(tc.left, tc.right); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseKWin(%q, %q) = %+v, want %+v", tc.left, tc.right, got, tc.want)
		}
	}
}

func TestReadKWin(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	custom := write("custom", "[Desktops]\nNumber=4\nButtonsOnLeft=I\n\n[org.kde.kdecoration2]\n# close on the left\nButtonsOnLeft[$i]=XM\nButtonsOnRight=IA\nlibrary=org.kde.breeze\n")
	if got, err := ReadKWin(custom); err != nil || !reflect.DeepEqual(got, Layout{Start: []string{Close}, End: []string{Minimize, Maximize}}) {
		t.Errorf("custom kwinrc = %+v, %v", got, err)
	}

	// Keys KWin leaves out take its defaults ("MS", "HIAX").
	defaults := write("defaults", "[org.kde.kdecoration2]\nlibrary=org.kde.breeze\n")
	if got, err := ReadKWin(defaults); err != nil || !reflect.DeepEqual(got, Layout{End: []string{Minimize, Maximize, Close}}) {
		t.Errorf("default kwinrc = %+v, %v", got, err)
	}

	if _, err := ReadKWin(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing kwinrc: err = %v, want os.ErrNotExist", err)
	}
}

func TestKWinRC(t *testing.T) {
	if got := KWinRC(env(map[string]string{"XDG_CONFIG_HOME": "/cfg", "HOME": "/home/u"})); got != "/cfg/kwinrc" {
		t.Errorf("with XDG_CONFIG_HOME: %q", got)
	}
	// A relative XDG_CONFIG_HOME is invalid and ignored (XDG spec).
	if got := KWinRC(env(map[string]string{"XDG_CONFIG_HOME": "cfg", "HOME": "/home/u"})); got != "/home/u/.config/kwinrc" {
		t.Errorf("with a relative XDG_CONFIG_HOME: %q", got)
	}
	if got := KWinRC(env(nil)); got != "" {
		t.Errorf("with nothing set: %q", got)
	}
}

func TestResolve(t *testing.T) {
	stacking := Platform{Native: NativeFrame, Automatic: config.TitleBarSona}
	tiling := Platform{Native: NativeFrame, Automatic: config.TitleBarDesktop}
	mac := Platform{Native: NativeInset, Automatic: config.TitleBarDesktop}
	type out struct {
		kind    string
		buttons bool
	}
	for _, tc := range []struct {
		name  string
		p     Platform
		style string
		want  out
	}{
		{"stacking, automatic", stacking, config.TitleBarAuto, out{KindDrawn, true}},
		{"stacking, unset", stacking, "", out{KindDrawn, true}},
		{"stacking, the desktop's", stacking, config.TitleBarDesktop, out{KindNone, false}},
		{"stacking, without buttons", stacking, config.TitleBarSonaNoButtons, out{KindDrawn, false}},
		{"tiling, automatic", tiling, config.TitleBarAuto, out{KindNone, false}},
		{"tiling, Sona's", tiling, config.TitleBarSona, out{KindDrawn, true}},
		{"tiling, without buttons", tiling, config.TitleBarSonaNoButtons, out{KindDrawn, false}},
		// macOS always keeps its own buttons, whatever the setting says.
		{"macOS, automatic", mac, config.TitleBarAuto, out{KindNativeInset, false}},
		{"macOS, Sona's", mac, config.TitleBarSona, out{KindNativeInset, false}},
	} {
		kind, buttons := tc.p.Resolve(tc.style)
		if got := (out{kind, buttons}); got != tc.want {
			t.Errorf("%s: Resolve = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
