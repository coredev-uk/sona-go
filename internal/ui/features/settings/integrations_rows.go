package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// needsAppBundle is why a control macOS ties to Sona.app is unavailable in a
// source build, in place of "not available on macOS yet".
func needsAppBundle() string {
	return i18n.T("Only when Sona runs from Sona.app, not from a source build.")
}

// playingRows are track notifications and the suspend inhibitor, where the
// platform has them.
func (s *settings) playingRows(c *ui.Context, cfg config.Settings, caps services.Capabilities) {
	status := s.integrationsStatus().Data
	on := cfg.Integrations

	title := i18n.T("Track notifications")
	if !caps.Notifications {
		reason := ""
		if caps.NeedsAppBundle {
			reason = needsAppBundle()
		}
		s.unavailableRow(c, "track-notifications", title, reason)
	} else {
		s.switchRow(c, rowOpts{ID: "track-notifications", Title: title, Desc: s.notificationsDesc(caps, on.Notifications, status.Notifications)}, on.Notifications, false, func(v bool) {
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

// notificationsDesc says what a track notification shows on this platform
// and, once turned on, why none appears.
func (s *settings) notificationsDesc(caps services.Capabilities, enabled bool, status string) string {
	var desc string
	if caps.PlainNotifications {
		desc = i18n.T("Shows the song and artist when a new track starts; click it to open Sona. Not shown while Sona's window is in front.")
	} else {
		desc = i18n.T("Shows the song, artist and artwork when a new track starts, with a Next button. Not shown while Sona's window is in front.")
	}
	switch {
	case enabled && status == services.IntegrationDenied:
		return desc + " " + i18n.T("{platform} isn't letting Sona show notifications: allow them in System Settings → Notifications.", "platform", s.platformName())
	case enabled && status == services.IntegrationUnavailable:
		return desc + " " + i18n.T("No notification service is running on this desktop.")
	}
	return desc
}
