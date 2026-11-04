package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// motionArtwork is the Motion artwork row (MotionArtworkRows.tsx): Apple's
// looping covers. It is forced off, and disabled, while Reduce effects is
// on; without the power state it can't pause on battery, so it doesn't say
// it does.
func (s *settings) motionArtwork(c *ui.Context, st config.Settings, disabled bool) {
	reduce := st.ReduceEffects
	// Without the power state, motion artwork can't pause on battery.
	caps := s.caps()
	followsBattery := !caps.HasData || caps.Data.PowerState
	desc := i18n.T("Plays the looping covers some albums and playlists have, in their headers and in Now Playing.")
	switch {
	case reduce:
		desc = i18n.T("Off while Reduce effects is on.")
	case followsBattery:
		desc = i18n.T("Plays the looping covers some albums and playlists have, in their headers and in Now Playing. Paused on battery power.")
	}
	s.switchRow(c, rowOpts{ID: "motion-artwork", Title: i18n.T("Motion artwork"), Desc: desc},
		st.MotionArtwork.Enabled && !reduce, disabled || reduce, func(on bool) {
			s.save(func(n *config.Settings) { n.MotionArtwork.Enabled = on })
		})
}
