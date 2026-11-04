package concerts

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// locationDialog is where to look for concerts (LocationDialog.tsx): a
// search over Sona's own list of cities, run on this computer. The choice
// is saved here; Apple is told only the area's four-character code with
// each listing.
type locationDialog struct {
	open   bool
	query  string
	saving bool
	// last is the cities of the last answer, shown while a new search loads
	// (keepPreviousData).
	last []services.ConcertPlace
	// announced is the term whose "no cities match" was announced.
	announced string
	// cur is the city button that had the focus last; want one a key asked for.
	cur, want int
	wanting   bool
	// focus gives the field the focus once the dialog is up.
	focus kit.FocusStep
}

// Show opens the dialog.
func (l *locationDialog) Show() { l.open = true }

// show builds the dialog while it is open; loc is where concerts are
// looked for now (nil until known).
func (l *locationDialog) show(c *ui.Context, d *data, loc *services.ConcertLocation) {
	if !l.open {
		l.query, l.announced, l.wanting, l.focus = "", "", false, 0
		return
	}
	term := strings.TrimSpace(l.query)
	res := d.places(term)
	if res.HasData {
		l.last = res.Data
	}
	items := l.last
	kit.Dialog(c, &l.open, kit.DialogOpts{
		Title:       i18n.T("Location"),
		Description: i18n.T("Concerts are listed near the city you choose. Sona keeps your choice on this computer and tells Apple Music only the area it covers."),
		MaxWidth:    448,
		Gap:         16,
	}, func() {
		ui.Column(c.Key("find")).Children(func() {
			in := kit.Input(c.Key("query"), &l.query, kit.InputOpts{
				Placeholder: i18n.T("Search cities"), A11yLabel: i18n.T("Search cities"),
			})
			kit.FocusWhenShown(c, in, &l.focus)
			switch {
			case in.Shortcut(0, ui.KeyDown) && len(items) > 0:
				l.cur, l.want, l.wanting = 0, 0, true
			case in.Submitted() && term != "" && len(items) > 0:
				l.pick(d, items[0].ID)
			}
			none := term != "" && len(items) == 0
			if none && !res.Fetching && l.announced != term {
				l.announced = term
				c.Announce(i18n.T("No cities found"))
			} else if !none {
				l.announced = ""
			}
			switch {
			case len(items) > 0:
				l.list(c, d, in, items, term)
			case none:
				text(c.Key("none"), i18n.T("No cities found")+". "+i18n.T("Try the name of a bigger city nearby."), "secondary", "ink-muted").
					Margin(12, 0, 0, 0).Wrap()
			}
		})
		if loc != nil && !loc.IsDefault {
			ui.Row(c.Key("footer")).Children(func() {
				b := kit.Button(c.Key("reset"), i18n.T("Use my storefront’s main city"), kit.ButtonOpts{
					Variant: kit.Ghost, Size: kit.SizeSM, Disabled: l.saving,
				})
				if b.Clicked() {
					l.pick(d, "")
				}
			})
		}
	})
}

// pick saves the city and closes the dialog; a failure toasts and leaves it open.
func (l *locationDialog) pick(d *data, id string) {
	if l.saving {
		return
	}
	l.saving = true
	d.setLocation(id, func(ok bool) {
		l.saving = false
		if ok {
			l.open = false
		}
	})
}

// list is the cities: Up and Down, Home and End move among them, Up from
// the first returns to the field.
func (l *locationDialog) list(c *ui.Context, d *data, in ui.Element, items []services.ConcertPlace, term string) {
	label := i18n.T("Suggestions")
	if term != "" {
		label = i18n.T("Cities that match")
	}
	list := ui.Scroll(c.Key("cities")).MaxHeight(288).Margin(12, 0, 0, 0).Role(ui.RoleList).Label(label)
	list.Children(func() {
		for i, p := range items {
			l.place(c, d, p, i)
		}
	})
	last := len(items) - 1
	to, ok := l.cur, false
	switch {
	case list.Shortcut(0, ui.KeyDown) && l.cur < last:
		to, ok = l.cur+1, true
	case list.Shortcut(0, ui.KeyUp):
		if l.cur > 0 {
			to, ok = l.cur-1, true
		} else {
			in.Focus()
		}
	case list.Shortcut(0, ui.KeyHome):
		to, ok = 0, true
	case list.Shortcut(0, ui.KeyEnd):
		to, ok = last, true
	}
	if ok {
		l.want, l.wanting = to, true
		c.Invalidate()
	}
}

// place is a city's row: "Name, Country", a check on the chosen one.
func (l *locationDialog) place(c *ui.Context, d *data, p services.ConcertPlace, i int) {
	country := countryName(p.CountryCode)
	row := ui.ButtonBase(c.Key(p.ID)).Height(40).FillWidth().PaddingX(8).Gap(12).Radius(theme.Current().Radius["md"]).
		AlignItems(ui.Center).Label(p.Name + ", " + country).Disabled(l.saving)
	switch {
	case p.Selected:
		row.Background(theme.C("active")).Description(i18n.T("Current"))
	case row.Hovered() || row.Focused():
		row.Background(theme.C("hover"))
	}
	if l.wanting && l.want == i {
		row.Focus().ScrollIntoView()
		l.wanting = false
	}
	if row.Focused() {
		l.cur = i
	}
	row.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			text(c, p.Name, "body", "ink").SingleLine().Shrink(0)
			text(c, ", "+country, "body", "ink-muted").SingleLine().Shrink(1).MinWidth(0)
		})
		if p.Selected {
			ui.Icon(c, theme.Icon("check")).FontSize(16).Shrink(0)
		}
	})
	if row.Clicked() {
		l.pick(d, p.ID)
	}
}
