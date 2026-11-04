package settings

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// desktopAPI is what General asks of DesktopService.
type desktopAPI interface {
	ResetWindowSize()
}

// desktopSeam is a test's replacement of DesktopService.
type desktopSeam struct{ api desktopAPI }

func (s *settings) desktop() desktopAPI {
	if v := stateOf[desktopSeam](s, "seam:desktop").api; v != nil {
		return v
	}
	return services.Service[services.DesktopService](s.env.Core)
}

// startupRows are Start at login and, once it is on, Start in the tray.
func (s *settings) startupRows(c *ui.Context, cfg config.Settings, caps services.Capabilities) {
	d := cfg.Desktop
	title := i18n.T("Start Sona when you log in")
	if !caps.Autostart {
		s.unavailableRow(c, "autostart", title, "")
		return
	}
	s.switchRow(c, rowOpts{ID: "autostart", Title: title, Desc: i18n.T("Adds Sona to your desktop's startup applications.")},
		d.Autostart, false, func(on bool) { s.saveDesktop(func(d *config.Desktop) { d.Autostart = on }) })
	// Starting without a window needs the tray to open it from, and only
	// applies at login.
	if caps.Tray && d.Autostart {
		s.switchRow(c, rowOpts{
			ID: "start-hidden", Title: i18n.T("Start in the tray"),
			Desc: i18n.T("At login, Sona starts without a window, ready to play from the tray or your media keys."),
		}, d.StartHidden, false, func(on bool) { s.saveDesktop(func(d *config.Desktop) { d.StartHidden = on }) })
	}
}

// closeActionRow is what closing the window does; it needs the tray.
func (s *settings) closeActionRow(c *ui.Context, cfg config.Settings, caps services.Capabilities) {
	title := i18n.T("When the window is closed")
	if !caps.Tray {
		s.unavailableRow(c, "close-action", title,
			i18n.T("Sona quits: it can't stay in the system tray on {platform} yet.", "platform", s.platformName()))
		return
	}
	items := []kit.SelectItem{
		kit.SelectOption(config.CloseToBackground, i18n.T("Keep playing in the background")),
		kit.SelectOption(config.CloseQuits, i18n.T("Quit Sona")),
	}
	s.selectRow(c, rowOpts{
		ID: "close-action", Title: title,
		Desc: i18n.T("In the background, music keeps playing and Sona stays in the system tray. Open it again from the tray, your media keys' player or the app launcher."),
	}, cfg.Desktop.CloseAction, items, false, func(v string) {
		s.saveDesktop(func(d *config.Desktop) { d.CloseAction = v })
	})
}

// titleBarRow says who draws the title bar (Windows and Linux).
func (s *settings) titleBarRow(c *ui.Context, cfg config.Settings, caps services.Capabilities) {
	controls := caps.WindowControls
	desktop := controls.Desktop
	var hint string
	switch {
	case controls.Automatic == "sona" && desktop != "":
		hint = i18n.T("Automatic uses Sona's title bar on {desktop}, with the window buttons where your desktop puts them.", "desktop", desktop)
	case controls.Automatic == "sona":
		hint = i18n.T("Automatic uses Sona's title bar, with the window buttons where your system puts them.")
	case desktop != "":
		hint = i18n.T("Automatic leaves the title bar to {desktop}.", "desktop", desktop)
	default:
		hint = i18n.T("Automatic leaves the title bar to your desktop, which Sona doesn't recognise.")
	}
	value := cfg.TitleBar.Style
	// The window keeps the title bar it was created with until Sona restarts.
	if value != controls.Style {
		hint += " " + i18n.T("Applies the next time Sona starts.")
	}
	items := []kit.SelectItem{
		kit.SelectOption(config.TitleBarAuto, i18n.T("Automatic")),
		kit.SelectOption("sona", i18n.T("Sona's, with window buttons")),
		kit.SelectOption("sona-no-buttons", i18n.T("Sona's, without window buttons")),
		kit.SelectOption("desktop", i18n.T("The desktop's")),
	}
	s.selectRow(c, rowOpts{ID: "title-bar", Title: i18n.T("Title bar"), Desc: hint}, value, items, false, func(v string) {
		s.save(func(st *config.Settings) { st.TitleBar.Style = v })
	})
}

// windowSizeRow shows the saved size and resets it.
func (s *settings) windowSizeRow(c *ui.Context, cfg config.Settings) {
	w := cfg.Window
	var desc string
	if w.Maximised {
		desc = i18n.T("Sona opens at the size you left it, maximised: <0>{width} × {height}</0>.", "width", w.Width, "height", w.Height)
	} else {
		desc = i18n.T("Sona opens at the size you left it: <0>{width} × {height}</0>.", "width", w.Width, "height", w.Height)
	}
	desc = stripTags(desc)
	s.row(c, rowOpts{ID: "window-size", Title: i18n.T("Window size"), Desc: desc}, func() ui.Element {
		b := kit.Button(c.Key("reset"), i18n.T("Reset size"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM})
		if b.Clicked() {
			d := s.desktop()
			go func() {
				defer func() {
					if r := recover(); r != nil {
						s.env.Post(func() { kit.ToastErrorf(i18n.T("Couldn't reset the window"), "") })
					}
				}()
				d.ResetWindowSize()
			}()
		}
		return b
	})
}

// stripTags drops the <0> markers of a message's rich text.
func stripTags(t string) string { return tagMarkers.Replace(t) }

var tagMarkers = strings.NewReplacer("<0>", "", "</0>", "")
