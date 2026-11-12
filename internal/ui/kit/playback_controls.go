package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// PlayPauseIcon is the icon and the name of the play control for a playback
// state (store.State…): the spinner while loading, pause while playing, play
// otherwise. A loading control spins (ButtonOpts.Spin).
func PlayPauseIcon(state string) (icon, name string) {
	switch state {
	case store.StateLoading:
		return "loader-circle", i18n.T("Play")
	case store.StatePlaying:
		return "pause", i18n.T("Pause")
	}
	return "play", i18n.T("Play")
}

// VolumeIcon is the icon and the name of the mute control at volume v (0…1):
// it unmutes at zero.
func VolumeIcon(v float64) (icon, name string) {
	switch {
	case v == 0:
		return "volume-x", i18n.T("Unmute")
	case v < 0.5:
		return "volume-1", i18n.T("Mute")
	}
	return "volume-2", i18n.T("Mute")
}

// LiveMarker is a live station's dot (lit while it plays) and "Live", in
// place of the seek bar's times.
func LiveMarker(c *ui.Context, playing bool) ui.Element {
	return ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		dot := theme.C("ink-faint")
		if playing {
			dot = theme.C("play")
		}
		ui.Box(c.Key("dot")).Size(6, 6).Radius(3).Background(dot).Shrink(0)
		theme.Text(c.Key("word"), i18n.T("Live"), "meta").FontWeight(500).TextColor(theme.C("ink")).SingleLine().MinWidth(0).Shrink(1)
	})
}

// PopupFor draws the menu a playing item's row opened, once a frame, and
// closes it when the item is gone (hasItem false). A nil m does nothing.
func (m *ActionMenus) PopupFor(c *ui.Context, hasItem bool) {
	switch {
	case m == nil:
	case !hasItem:
		m.Close()
	default:
		m.Popup(c)
	}
}
