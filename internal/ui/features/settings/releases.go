package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// releaseRows are the New Releases group's two switches
// (config.ArtistTracker): whether Sona watches the favourite artists'
// releases, and whether a release (a reminder that came due, or what the
// tracker found) also shows a desktop notification. Either way it is in the
// notification centre. The desktop row stays where the platform has no
// notifications and says why. Track notifications are Playback's own setting.
func (s *settings) releaseRows(c *ui.Context, cfg config.Settings, caps services.Capabilities) {
	t := cfg.ArtistTracker
	s.switchRow(c, rowOpts{
		ID:    "artist-tracker",
		Title: i18n.T("Track new releases from favourite artists"),
		Desc:  i18n.T("Once a day Sona looks at the artists you've favourited and lists their new albums, EPs and singles under New releases in the sidebar. Turning it on lists nothing that is out already, only what comes out afterwards. Sona only reads Apple Music's catalogue: nothing counts as a play."),
	}, t.Enabled, false, func(v bool) {
		s.save(func(st *config.Settings) { st.ArtistTracker.Enabled = v })
	})

	title := i18n.T("Also show releases as desktop notifications")
	if !caps.Notifications {
		reason := ""
		if caps.NeedsAppBundle {
			reason = needsAppBundle()
		}
		s.unavailableRow(c, "artist-tracker-notify", title, reason)
		return
	}
	s.switchRow(c, rowOpts{
		ID:    "artist-tracker-notify",
		Title: title,
		Desc:  i18n.T("A reminder that comes due and new releases from your artists are always in Notifications, under the bell. This also shows them on your desktop: one for each release, or one summary when several arrive together."),
	}, t.Desktop, false, func(v bool) {
		s.save(func(st *config.Settings) { st.ArtistTracker.Desktop = v })
	})
}
