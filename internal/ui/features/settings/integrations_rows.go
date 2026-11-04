package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// playingRows are track notifications and the suspend inhibitor, where the
// platform has them.
func (s *settings) playingRows(c *ui.Context, cfg config.Settings, caps services.Capabilities) {
	status := s.integrationsStatus().Data
	on := cfg.Integrations

	title := i18n.T("Track notifications")
	if !caps.Notifications {
		s.unavailableRow(c, "track-notifications", title, "")
	} else {
		desc := i18n.T("Shows the song, artist and artwork when a new track starts, with a Next button. Not shown while Sona's window is in front.")
		if on.Notifications && status.Notifications == "unavailable" {
			desc = i18n.T("Shows the song, artist and artwork when a new track starts, with a Next button. Not shown while Sona's window is in front. No notification service is running on this desktop.")
		}
		s.switchRow(c, rowOpts{ID: "track-notifications", Title: title, Desc: desc}, on.Notifications, false, func(v bool) {
			s.save(func(st *config.Settings) { st.Integrations.Notifications = v })
		})
	}

	title = i18n.T("Stay awake while playing")
	if !caps.SleepInhibit {
		s.unavailableRow(c, "inhibit-sleep", title, "")
		return
	}
	desc := i18n.T("Stops the computer from suspending on its own while music plays. The screen can still lock and turn off.")
	if on.InhibitSleep && status.Inhibit == "unavailable" {
		desc = i18n.T("Stops the computer from suspending on its own while music plays. The screen can still lock and turn off. Neither logind nor the desktop portal answered, so sleep can't be blocked.")
	}
	s.switchRow(c, rowOpts{ID: "inhibit-sleep", Title: title, Desc: desc}, on.InhibitSleep, false, func(v bool) {
		s.save(func(st *config.Settings) { st.Integrations.InhibitSleep = v })
	})
}
