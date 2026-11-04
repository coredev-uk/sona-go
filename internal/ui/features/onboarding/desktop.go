package onboarding

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// desktopStep is Desktop: what closing the window does, starting at login,
// and what Sona does while music plays, each where the platform can. What it
// can't do yet is named in one line at the foot, so nothing goes missing
// quietly (DesktopStep.tsx).
func (o *onboarding) desktopStep(c *ui.Context) {
	settings, caps := o.settings(), o.caps()
	switch {
	case settings.Err != nil:
		o.settingsError(c, settings)
		return
	case !settings.HasData || !caps.HasData:
		loading(c, 288)
		return
	}
	s, cp := settings.Data, caps.Data
	d := s.Desktop
	desktop := func(patch func(*config.Desktop)) {
		o.save(func(s *config.Settings) { patch(&s.Desktop) })
	}
	integrations := func(patch func(*config.Integrations)) {
		o.save(func(s *config.Settings) { patch(&s.Integrations) })
	}
	var missing []string
	if !cp.Autostart {
		missing = append(missing, i18n.T("starting at login"))
	}
	if !cp.Notifications {
		missing = append(missing, i18n.T("track notifications"))
	}

	ui.Column(c.Key("desktop")).FillWidth().Gap(20).Children(func() {
		if cp.Tray {
			stepGroup(c, "close", i18n.T("When you close the window"), func() {
				choiceCards(c, "close", i18n.T("When you close the window"), d.CloseAction, []choice{
					{value: config.CloseToBackground, label: i18n.T("Keep playing"), hint: i18n.T("Sona stays in the system tray.")},
					{value: config.CloseQuits, label: i18n.T("Quit Sona"), hint: i18n.T("The music stops with it.")},
				}, false, func(v string) {
					desktop(func(d *config.Desktop) { d.CloseAction = v })
				})
			})
		}
		var rows []func()
		if cp.Autostart {
			rows = append(rows, func() {
				switchRow(c, "autostart", i18n.T("Start Sona when you log in"), i18n.T("Adds Sona to your desktop's startup applications."),
					d.Autostart, false, func(on bool) { desktop(func(d *config.Desktop) { d.Autostart = on }) }, nil)
			})
		}
		// Starting without a window needs the tray to open it from, and only applies at login.
		if cp.Autostart && cp.Tray && d.Autostart {
			rows = append(rows, func() {
				switchRow(c, "start-hidden", i18n.T("Start in the tray"), i18n.T("At login, Sona starts without a window, ready for your media keys."),
					d.StartHidden, false, func(on bool) { desktop(func(d *config.Desktop) { d.StartHidden = on }) }, nil)
			})
		}
		if cp.Notifications {
			rows = append(rows, func() {
				switchRow(c, "notifications", i18n.T("Track notifications"), i18n.T("The song and artist when a new track starts, while Sona is in the background."),
					s.Integrations.Notifications, false, func(on bool) { integrations(func(i *config.Integrations) { i.Notifications = on }) }, nil)
			})
		}
		if cp.SleepInhibit {
			rows = append(rows, func() {
				switchRow(c, "inhibit-sleep", i18n.T("Stay awake while playing"), i18n.T("The computer doesn't suspend on its own while music plays."),
					s.Integrations.InhibitSleep, false, func(on bool) { integrations(func(i *config.Integrations) { i.InhibitSleep = on }) }, nil)
			})
		}
		list(c, "desktop", rows)

		if len(missing) > 0 || !cp.Tray {
			var lines []string
			if len(missing) > 0 {
				lines = append(lines, i18n.T("Not on this platform yet: {missingList}.", "missingList", i18n.List(missing)))
			}
			if !cp.Tray {
				lines = append(lines, i18n.T("Closing the window quits Sona: playing on in the background needs a system tray, which Sona can't use here yet."))
			}
			text(c.Key("missing"), strings.Join(lines, " "), "sub").TextColor(theme.C("ink-muted"))
		}
	})
}
