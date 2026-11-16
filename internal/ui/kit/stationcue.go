package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// StationOn is true for a station with no page that the queue is on, playing
// or paused.
func StationOn(it services.Item, ctx ItemsCtx) bool {
	return it.Station != nil && ctx.Current != nil && ctx.Current.ID == it.Station.ID && PlaysWhenOpened(it, ctx.Players)
}

// StationPlaying is true for a station with no page that is playing now: its
// one control reads "Pause" and a press pauses it.
func StationPlaying(it services.Item, ctx ItemsCtx) bool {
	return ctx.Playing && StationOn(it, ctx)
}

// StationLabel is the accessible name of the one control of a station with no
// page: its text, then what a press does ("Play", "Pause").
func StationLabel(it services.Item, ctx ItemsCtx) string {
	return it.Name + " " + StationAction(StationPlaying(it, ctx))
}

// StationAction is the word a station's control ends its name with.
func StationAction(playing bool) string {
	if playing {
		return i18n.T("Pause")
	}
	return i18n.T("Play")
}

// StationCue is the mark of the one control of a station with no page: the
// play triangle (the pause bars while it plays), 16 DIPs, shown on hover or
// focus. It takes no input; the control it sits in is named by StationLabel.
func StationCue(c *ui.Context, playing, show bool) ui.Element {
	icon := "play"
	if playing {
		icon = "pause"
	}
	mark := ui.Row(c.Key("cue")).Size(16, 16).Shrink(0).Center().IgnoreInput(true).Role(ui.RoleNone).TextColor(theme.C("ink-muted"))
	Fade(mark, show)
	return mark.Children(func() {
		theme.Glyph(c, icon, 16, theme.GlyphOpts{Color: theme.C("ink-muted"), Inherit: true, Filled: true})
	})
}
