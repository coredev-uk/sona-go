package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
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
			miniWindow(p, r, theme.TokensFor(false))
		case config.ThemeDark:
			miniWindow(p, r, theme.TokensFor(true))
		default:
			miniWindow(p, r, theme.TokensFor(false))
			p.Clip(ui.Rect{X: r.X + r.W/2, Y: r.Y, W: r.W - r.W/2, H: r.H}, 0, func() {
				miniWindow(p, r, theme.TokensFor(true))
			})
		}
		// The edge, over the picture so a dark window still shows on a dark card.
		p.Stroke(r, ink.Alpha(0.2), theme.Current().Radius["md"], 1)
	})
}

// miniWindow draws Sona's window in miniature in one palette: the sidebar with
// the mark, the top bar, a few rows, and the player bar with its amber play
// control. Drawn from the tokens, so it follows any change to them.
func miniWindow(p *ui.Painter, r ui.Rect, tk *theme.Tokens) {
	col := func(name string) ui.Color { return tk.Color[name] }
	at := func(fx, fy, fw, fh float32) ui.Rect {
		return ui.Rect{X: r.X + fx*r.W, Y: r.Y + fy*r.H, W: fw * r.W, H: fh * r.H}
	}
	pill := func(rc ui.Rect, c ui.Color) { p.Fill(rc, c, rc.H/2) }

	p.Fill(r, col("ground"), 0)

	// The sidebar: the mark and three rows.
	side := at(0, 0, 0.28, 1)
	p.Fill(side, col("surface"), 0)
	d := 0.22 * side.W
	p.Fill(ui.Rect{X: side.X + 0.14*side.W, Y: r.Y + 0.08*r.H, W: d, H: d}, col("play"), d/2)
	faint := col("ink-faint").Alpha(0.4)
	for _, row := range [...]struct{ top, w float32 }{{0.30, 0.58}, {0.42, 0.44}, {0.54, 0.52}} {
		pill(ui.Rect{X: side.X + 0.14*side.W, Y: r.Y + row.top*r.H, W: row.w * side.W, H: 0.05 * r.H}, faint)
	}

	// The top bar, with its line under it.
	bar := at(0.28, 0, 0.72, 0.16)
	p.Fill(bar, col("surface"), 0)
	p.Fill(ui.Rect{X: bar.X, Y: bar.Y + bar.H - 1, W: bar.W, H: 1}, col("line"), 0)

	// A heading and a few rows.
	pill(at(0.34, 0.26, 0.30, 0.08), col("ink").Alpha(0.8))
	muted := col("ink-muted").Alpha(0.4)
	for _, row := range [...]struct{ top, w float32 }{{0.42, 0.50}, {0.52, 0.42}, {0.62, 0.46}} {
		pill(at(0.34, row.top, row.w, 0.05), muted)
	}

	// The player bar and its amber play control.
	foot := at(0, 0.82, 1, 0.18)
	p.Fill(foot, col("surface"), 0)
	p.Fill(ui.Rect{X: foot.X, Y: foot.Y, W: foot.W, H: 1}, col("line"), 0)
	disc := 0.48 * foot.H
	p.Fill(ui.Rect{X: foot.X + (foot.W-disc)/2, Y: foot.Y + (foot.H-disc)/2, W: disc, H: disc}, col("play"), disc/2)
}
