package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// appearance is Settings → Appearance (AppearanceSection.tsx): the colours
// (the mode, the theme gallery, the folder of the user's own themes), then
// Reduce effects and Motion artwork. Each control saves at once; kit's
// UpdateSettings applies the theme, the mode and Reduce effects as it does,
// so the window follows before Go has answered.
func (s *settings) appearance(c *ui.Context) {
	st := s.cfg()
	// Until the settings are there the rows keep their places, disabled.
	off := !st.HasData
	s.stack(c.Key("panel"), func() {
		s.group(c, "appearance-colours", i18n.T("Colours"), "", func() {
			s.modeRow(c, st.Data.Theme, off)
			s.row(c, rowOpts{
				ID: "theme-gallery", Title: i18n.T("Theme"), Stacked: true,
				Desc:  i18n.T("The colours of the whole window. A theme also picks the colour that marks what plays."),
				Notes: func() { themeFallbackNote(c) },
			}, func() ui.Element { return s.themeGallery(c, off) })
			s.themesFolderRow(c)
		})
		s.group(c, "appearance-effects", i18n.T("Effects"), "", func() {
			s.switchRow(c, rowOpts{
				ID: "reduce-effects", Title: i18n.T("Reduce effects"),
				Desc: i18n.T("Solid surfaces instead of artwork colour washes, and no animation. Smoother on software rendering."),
			}, st.Data.ReduceEffects, off, func(on bool) {
				s.save(func(n *config.Settings) { n.ReduceEffects = on })
			})
			s.motionArtwork(c, st.Data, off)
		})
	})
}

// modeRow is the appearance: light, dark, or the desktop's. A theme with one
// look only holds it, and the row says so.
func (s *settings) modeRow(c *ui.Context, value string, off bool) {
	active := theme.Active()
	desc := i18n.T("System follows your desktop's light or dark preference.")
	dark, pinned := active.Pinned()
	if pinned {
		if dark {
			desc = i18n.T("{theme} has a dark look only, so Sona stays dark with it.", "theme", themeName(active))
		} else {
			desc = i18n.T("{theme} has a light look only, so Sona stays light with it.", "theme", themeName(active))
		}
	}
	s.row(c, rowOpts{ID: "theme", Title: i18n.T("Mode"), Desc: desc},
		func() ui.Element { return s.themeChoice(c, value, off || pinned) })
}

// themeChoice is the mode as three segments, all in view at once (radio
// buttons: ←/→ move and choose, one Tab stop), instead of a menu that hides
// two of three choices. It returns the group, which Find focuses.
func (s *settings) themeChoice(c *ui.Context, value string, disabled bool) ui.Element {
	choices := [...]struct{ value, label string }{
		{config.ThemeSystem, i18n.T("System")},
		{config.ThemeLight, i18n.T("Light")},
		{config.ThemeDark, i18n.T("Dark")},
	}
	if value == "" {
		value = config.ThemeSystem
	}
	tk := theme.Current()
	sel := value
	changed := false
	group := ui.RadioGroup(c.Key("theme"), func() {
		for _, ch := range choices {
			on := sel == ch.value
			r := ui.RadioBase(c.Key(ch.value), &sel, ch.value).Label(ch.label).MinWidth(64).Padding(0, 12).
				Justify(ui.Center).AlignItems(ui.Center).Radius(4).Cursor(ui.CursorPointer)
			fg, weight := theme.C("ink-muted"), 400
			switch {
			case on:
				r.Background(theme.C("active"))
				fg, weight = theme.C("ink"), 500
			case r.Hovered():
				fg = theme.C("ink")
			}
			if disabled {
				r.Disabled(true).Opacity(0.5)
			}
			if r.Changed() {
				changed = true
			}
			r.Children(func() { theme.Text(c, ch.label, "secondary").FontWeight(weight).TextColor(fg).SingleLine() })
		}
	}).Row().Height(32).Padding(2).AlignItems(ui.Stretch).Radius(tk.Radius["md"]).
		Border(1, theme.C("control-line")).Label(i18n.T("Mode"))
	if changed && sel != value {
		next := sel
		s.save(func(n *config.Settings) { n.Theme = next })
	}
	return group
}
