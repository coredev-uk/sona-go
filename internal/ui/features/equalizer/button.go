package equalizer

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// controlOrder is the button's place among the player bar's controls: after
// the queue button and the Mini player button (v0.2.0's EqualizerButton).
const controlOrder = module.ControlsAfterQueue + 20

// button is the player bar's way to the equaliser, a dot under the glyph
// while it is on (v0.2.0's EqualizerButton). Absent where the platform's
// playback has no equaliser.
func (e *equalizer) button(c *ui.Context) {
	if !e.supported() {
		return
	}
	on := false
	if s := e.settings(); s.HasData {
		on = s.Data.Audio.Equalizer.Enabled
	}
	name := i18n.T("Equaliser")
	if on {
		name = i18n.T("Equaliser (on)")
	}
	ui.Box(c.Key("equalizer")).Children(func() {
		if kit.Button(c, "", kit.ButtonOpts{
			Variant: kit.Ghost, Size: kit.SizeSM, Icon: "sliders-vertical", Muted: !on, A11yLabel: name,
			Keys: e.env.KeysOf("equalizer.open"),
		}).Clicked() {
			e.env.Nav.Push(path)
		}
		if on {
			ui.Row(c.Key("on")).Absolute().Left(0).Right(0).Bottom(2).Center().PassThrough().Children(func() {
				ui.Box(c).Size(4, 4).Radius(2).Background(theme.C("ink"))
			})
		}
	})
}
