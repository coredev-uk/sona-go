package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// appearanceStep is Appearance: the theme as pictures, then reduce effects
// and motion artwork. Each applies at once, to the app behind the setup too
// (AppearanceStep.tsx).
func (o *onboarding) appearanceStep(c *ui.Context) {
	settings := o.settings()
	// Without the power state, motion artwork can't pause on battery.
	followsBattery := true
	if caps := o.capsData(); caps != nil {
		followsBattery = caps.PowerState
	}
	s := settings.Data
	reduce := settings.HasData && s.ReduceEffects
	ui.Column(c.Key("appearance")).FillWidth().Gap(20).Children(func() {
		o.themePicker(c, settings.HasData, s.Theme)
		motionHint := i18n.T("Plays the looping covers some albums have.")
		switch {
		case reduce:
			motionHint = i18n.T("Off while Reduce effects is on.")
		case followsBattery:
			motionHint = i18n.T("Plays the looping covers some albums have. Paused on battery power.")
		}
		list(c, "appearance", []func(){
			func() {
				switchRow(c, "reduce-effects", i18n.T("Reduce effects"),
					i18n.T("Solid surfaces and no animation. Smoother on software rendering."),
					reduce, !settings.HasData, func(on bool) {
						o.save(func(s *config.Settings) { s.ReduceEffects = on })
					}, nil)
			},
			func() {
				switchRow(c, "motion-artwork", i18n.T("Motion artwork"), motionHint,
					settings.HasData && s.MotionArtwork.Enabled && !reduce, !settings.HasData || reduce, func(on bool) {
						o.save(func(s *config.Settings) { s.MotionArtwork.Enabled = on })
					}, nil)
			},
		})
	})
}

// themePicker is the theme as three pictures to choose from (Settings →
// Appearance has the same setting as a list). The app behind the setup
// changes at once (ThemePicker). An unknown saved theme reads as System.
func (o *onboarding) themePicker(c *ui.Context, loaded bool, saved string) {
	value := config.ThemeSystem
	if saved == config.ThemeLight || saved == config.ThemeDark {
		value = saved
	}
	preview := func(mode string) func(c *ui.Context) {
		return func(c *ui.Context) { themePreview(c, mode) }
	}
	stepGroup(c, "theme", i18n.T("Theme"), func() {
		choiceCards(c, "theme", i18n.T("Theme"), value, []choice{
			{value: config.ThemeSystem, label: i18n.T("System"), hint: i18n.T("Follows your desktop"), picture: preview(config.ThemeSystem)},
			{value: config.ThemeLight, label: i18n.T("Light"), hint: i18n.T("Always light"), picture: preview(config.ThemeLight)},
			{value: config.ThemeDark, label: i18n.T("Dark"), hint: i18n.T("Always dark"), picture: preview(config.ThemeDark)},
		}, !loaded, func(next string) {
			o.save(func(s *config.Settings) { s.Theme = next })
		})
		theme.Text(c, i18n.T("More themes, including OLED and community palettes, are in Settings → Appearance."), "meta").
			TextColor(theme.C("ink-muted"))
	})
}

// themePreview is the theme's picture: Sona's window in miniature in the
// light palette, the dark one, or, for System, the two halved. Decorative.
func themePreview(c *ui.Context, mode string) {
	e := ui.Box(c.Key("preview:" + mode)).FillWidth().AspectRatio(1.6).Radius(theme.Current().Radius["md"]).Clip()
	ink := theme.C("ink")
	e.Draw(func(p *ui.Painter, r ui.Rect) {
		switch mode {
		case config.ThemeLight:
			kit.MiniWindow(p, r, theme.TokensFor(false))
		case config.ThemeDark:
			kit.MiniWindow(p, r, theme.TokensFor(true))
		default:
			kit.MiniWindow(p, r, theme.TokensFor(false))
			p.Clip(ui.Rect{X: r.X + r.W/2, Y: r.Y, W: r.W - r.W/2, H: r.H}, 0, func() {
				kit.MiniWindow(p, r, theme.TokensFor(true))
			})
		}
		// The edge, over the picture so a dark window still shows on a dark card.
		p.Stroke(r, ink.Alpha(0.2), theme.Current().Radius["md"], 1)
	})
}
