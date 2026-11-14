package settings

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// accentDefault is the radio value of "Theme default": no accent chosen, the
// theme's own colour. No accent ID is spelt so.
const accentDefault = "default"

// accentSwatch is the diameter of an accent's colour disc.
const accentSwatch = 20

// The accents sit in a grid of equal cells, three to a row (six choices make
// two even rows; high contrast's seven end a row short), each at most
// accentCellMax wide.
const (
	accentColumns = 3
	accentGap     = 8
	accentCellMax = 168
)

// accentName is an accent's name in the user's language. Each is a literal
// msgid, so it is looked up here, not in a table of strings.
func accentName(id string) string {
	switch id {
	case "":
		return i18n.T("Theme default")
	case "amber":
		return i18n.T("Amber")
	case "gold":
		return i18n.T("Gold")
	case "lime":
		return i18n.T("Lime")
	case "sky":
		return i18n.T("Sky")
	case "teal":
		return i18n.T("Teal")
	case "rose":
		return i18n.T("Rose")
	}
	return id
}

// accentRow is the Accent row under the theme gallery: the colour that marks
// what plays, for the themes that offer a choice of it (Sona's own). It shows
// nothing for the others, whose spot colour is their palette's.
func (s *settings) accentRow(c *ui.Context, off bool) {
	th := theme.Active()
	if len(th.Accents()) == 0 {
		return
	}
	s.row(c, rowOpts{
		ID: "accent", Title: i18n.T("Accent"), Stacked: true,
		Desc: i18n.T("The colour that marks what plays: the play button, the progress and the song that is playing. Each theme keeps its own."),
	}, func() ui.Element { return s.accentChoice(c, th, theme.ActiveAccent(), off) })
}

// accentChoice is the accents as swatches with their names, all in view at
// once: radio buttons, so Tab reaches the chosen one and the arrow keys
// choose (each choice saves at once), as in the gallery above. The chosen one
// carries a check in its disc and a ring, so colour alone never says which.
// It returns the group, which Find focuses.
func (s *settings) accentChoice(c *ui.Context, th *theme.Theme, current string, disabled bool) ui.Element {
	dark := theme.Dark()
	ids := append([]string{""}, th.Accents()...)
	sel, changed := accentValue(current), false
	group := ui.RadioGroup(c.Key("accents"), func() {
		for start := 0; start < len(ids); start += accentColumns {
			ui.Row(c.Key("row:" + strconv.Itoa(start))).FillWidth().Gap(accentGap).Children(func() {
				for _, id := range ids[start:min(start+accentColumns, len(ids))] {
					accentChip(c, th, id, &sel, &changed, dark, disabled)
				}
				// A short last row keeps the cells' width, so the set reads as
				// one grid with no orphan stretched across the row.
				for i := len(ids) - start; i < accentColumns; i++ {
					ui.Box(c.Key("pad:" + strconv.Itoa(start+i))).Grow(1).Basis(0).MinWidth(0)
				}
			})
		}
	}).Column().Gap(accentGap).FillWidth().MaxWidth(accentColumns*accentCellMax + (accentColumns-1)*accentGap).Label(i18n.T("Accent"))
	if changed && sel != accentValue(current) {
		next, id := sel, th.ID
		if next == accentDefault {
			next = ""
		}
		s.save(func(n *config.Settings) {
			if next == "" {
				delete(n.Accents, id)
				return
			}
			if n.Accents == nil {
				n.Accents = map[string]string{}
			}
			n.Accents[id] = next
		})
	}
	return group
}

// accentValue is an accent's radio value: its ID, or accentDefault for none.
func accentValue(id string) string {
	if id == "" {
		return accentDefault
	}
	return id
}

// accentChip is one accent: a disc of its colour (with a check when chosen)
// and its name, in a cell of the grid.
func accentChip(c *ui.Context, th *theme.Theme, id string, sel *string, changed *bool, dark, disabled bool) {
	tk := theme.Current()
	{
		{
			v, name := accentValue(id), accentName(id)
			checked := *sel == v
			fill, glyph := th.AccentColors(id, dark)
			r := ui.RadioBase(c.Key("accent:"+v), sel, v).Row().Gap(8).AlignItems(ui.Center).Padding(6, 10, 6, 8).
				Grow(1).Basis(0).MinWidth(0).Radius(tk.Radius["lg"]).Background(theme.C("surface")).Label(name).Cursor(ui.CursorPointer)
			switch {
			case disabled:
				r.Disabled(true).Opacity(0.5)
			case r.Hovered():
				r.Background(theme.C("hover"))
			}
			if checked {
				r.Shadow(0, 0, 0, 2, theme.C("ink"))
			}
			if r.Changed() {
				*changed = true
			}
			r.Children(func() {
				ui.Row(c.Key("disc")).Width(accentSwatch).Height(accentSwatch).Radius(accentSwatch / 2).Background(fill).
					Justify(ui.Center).AlignItems(ui.Center).Shrink(0).Children(func() {
					if checked {
						ui.Icon(c.Key("check"), theme.Icon("check")).FontSize(14).TextColor(glyph)
					}
				})
				weight := 500
				if checked {
					weight = 600
				}
				theme.Text(c, name, "ui").FontWeight(weight).TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
			})
		}
	}
}
