package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// setPrivate is the switch's action; tests override it (seam:private).
func (s *settings) setPrivate(on bool) {
	if f := *stateOf[func(bool)](s, "seam:private"); f != nil {
		f(on)
		return
	}
	kit.SetPrivate(s.env.Queries, s.env.Core, on)
}

// privateRow is Settings → Playback's Private listening, the same switch
// as the player bar's: the description says where a state the user did
// not choose comes from.
func (s *settings) privateRow(c *ui.Context) {
	r := store.UsePrivate(s.env.Queries, s.env.Core)
	st := r.Data
	forced := r.HasData && store.PrivateForced(st)
	var desc string
	switch {
	case forced:
		desc = i18n.T("On because Sona was started with SONA_PRIVATE=1. Start Sona without it to turn private listening off.")
	case r.HasData && st.Source == "development" && st.Saved:
		desc = i18n.T("This development build of Sona starts with private listening on. Switching it here lasts until Sona quits; your saved choice (on) is unchanged.")
	case r.HasData && st.Source == "development":
		desc = i18n.T("This development build of Sona starts with private listening on. Switching it here lasts until Sona quits; your saved choice (off) is unchanged.")
	default:
		desc = i18n.T("Plays stay out of your Apple Music listening history and recommendations, and aren't scrobbled to Last.fm or ListenBrainz, shown on Discord or added to History. The same as the button in the player bar.")
	}
	s.switchRow(c, rowOpts{ID: "private-listening", Title: i18n.T("Private listening"), Desc: desc},
		r.HasData && st.On, !r.HasData || forced, s.setPrivate)
}
