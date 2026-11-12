package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// fadeState is the crossfade length while the thumb moves: saved on release.
type fadeState struct {
	on bool
	v  float64
}

// crossfadeRows are Crossfade and, while it is on, its length (1-12 s).
func (s *settings) crossfadeRows(c *ui.Context, st config.Settings, caps services.Capabilities) {
	title := i18n.T("Crossfade")
	if !caps.Playback.Crossfade {
		s.unavailableRow(c, "crossfade", title, "")
		return
	}
	cf := st.Crossfade
	s.switchRow(c, rowOpts{
		ID:    "crossfade",
		Title: title,
		Desc:  i18n.T("Fades each song into the next instead of playing them back to back. Songs that follow each other on the same album always play without a gap, as they were mastered."),
	}, cf.Enabled, false, func(on bool) {
		s.save(func(x *config.Settings) { x.Crossfade.Enabled = on })
	})
	if !cf.Enabled {
		return
	}
	fs := stateOf[fadeState](s, "crossfade")
	s.row(c, rowOpts{
		ID:    "crossfade-seconds",
		Title: i18n.T("Crossfade length"),
		Desc:  i18n.T("How long the end of one song overlaps the start of the next. Applies from the next song."),
	}, func() ui.Element {
		v := float64(cf.Seconds)
		if fs.on {
			v = fs.v
		}
		secs := int(v)
		var sl ui.Element
		ui.Row(c.Key("fade")).Width(selectWidth).AlignItems(ui.Center).Gap(12).Children(func() {
			sl = kit.Slider(c, &v, config.MinCrossfadeSeconds, config.MaxCrossfadeSeconds, kit.SliderOpts{
				A11yLabel: i18n.T("Crossfade length"),
				ValueText: i18n.T("{seconds, plural, one {# second} other {# seconds}}", "seconds", secs),
				Step:      1,
				Range:     theme.C("ink-muted"),
			})
			theme.Text(c, i18n.T("{seconds} s", "seconds", int(v)), "meta").TextColor(theme.C("ink-muted")).Width(36).
				TextAlign(ui.End)
		})
		if sl.Changed() {
			fs.on, fs.v = true, v
		}
		if fs.on && !sl.Pressed() {
			fs.on = false
			if n := int(fs.v); n != cf.Seconds {
				s.save(func(x *config.Settings) { x.Crossfade.Seconds = n })
			}
		}
		return sl
	})
}
