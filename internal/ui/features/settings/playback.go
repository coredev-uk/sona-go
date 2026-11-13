package settings

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

var idleChoices = []int{5, 10, 15, 30, 60}

// playback is Settings → Playback (Playback.tsx): Widevine's state first,
// then Sound, Listening, Lyrics and Advanced. Rows for what this
// platform's playback can't do say so in their place; without any
// playback one line says so and the lyrics stay. The volume is the player
// bar's, never a row here.
func (s *settings) playback(c *ui.Context) {
	if r := s.cfg(); r.Err != nil && !r.HasData {
		kit.QueryFailed(c, s.env.Queries, i18n.T("Couldn't read Sona's settings"), r.Err, store.KeySettings)
		return
	}
	st, caps, ok := s.ready()
	if !ok {
		s.loading(c, 5)
		return
	}
	pb := caps.Playback
	if !pb.Available {
		s.stack(c, func() {
			s.bare(c.Key("panel"), func() {
				s.unavailableRow(c, "playback", i18n.T("Playback isn't available on this platform yet"),
					i18n.T("Browsing Apple Music and your library works as usual."))
			})
			s.lyricsGroup(c, st)
		})
		return
	}
	cdm := pb.Engine == "cdm"
	s.stack(c, func() {
		if cdm {
			s.bare(c.Key("cdm"), func() { s.cdmRow(c) })
		}
		s.group(c, "playback-sound", i18n.T("Sound"), "", func() {
			s.qualityRow(c, st)
			s.audioRows(c, st, caps)
			s.crossfadeRows(c, st, caps)
		})
		s.group(c, "playback-listening", i18n.T("Listening"), "", func() {
			if pb.PrivateListening {
				s.privateRow(c)
			} else {
				// translators: Apple Music's own name for the mode: plays aren't reported to Apple, so they don't shape recommendations or Replay.
				// translators: Setting name. No Apple Music web string; Apple's apps offer the inverse, 'Use Listening History'.
				// translators: Switch name. No Apple Music web string; Apple's apps offer the inverse, 'Use Listening History'.
				s.unavailableRow(c, "private-listening", i18n.T("Private listening"),
					i18n.T("This platform's player reports plays to Apple itself, so Sona can't hold them back."))
			}
			s.queueRow(c, st)
		})
		s.lyricsGroup(c, st)
		if cdm {
			s.group(c, "playback-advanced", i18n.T("Advanced"), "", func() { s.idleRow(c, st) })
		}
	})
}

// idleRow is Free memory when idle: how long the playback host stays
// alive with nothing queued.
func (s *settings) idleRow(c *ui.Context, st config.Settings) {
	choices := slices.Clone(idleChoices)
	if !slices.Contains(choices, st.EngineIdleMinutes) {
		choices = append(choices, st.EngineIdleMinutes)
		slices.Sort(choices)
	}
	items := make([]kit.SelectItem, len(choices))
	for i, m := range choices {
		items[i] = kit.SelectOption(strconv.Itoa(m), i18n.T("After {m, plural, one {# minute} other {# minutes}}", "m", m))
	}
	s.selectRow(c, rowOpts{
		ID:    "idle",
		Title: i18n.T("Free memory when idle"),
		Desc:  i18n.T("With nothing queued, Sona stops its playback host (Widevine and the audio pipeline) after this long. The next song then takes a few seconds longer to start. Applies the next time the host starts."),
	}, strconv.Itoa(st.EngineIdleMinutes), items, false, func(v string) {
		n, _ := strconv.Atoi(v)
		s.save(func(x *config.Settings) { x.EngineIdleMinutes = n })
	})
}
