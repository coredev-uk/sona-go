package concerts

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The agenda's geometry (DIPs): a day's date column, and a day of the strip.
const (
	stubWidth = 64
	cellMax   = 72
	cellDots  = 3
)

// viewKey is what a listing's grouped form depends on: the listing (what it
// was asked, when it was fetched, how long it is and where it starts and
// ends), the day the strip starts from, and the language the days are
// written in.
type viewKey struct {
	filter      string
	yours       bool
	fetched     int64
	n           int
	first, last string
	from, to    string
	locale      string
}

// agendaView is the listing as the page draws it: the shows by month and day,
// and the strip of the coming days over them. It is made when the listing
// changes, not in every frame.
type agendaView struct {
	key    viewKey
	months []monthGroup
	strip  []stripDay
}

// viewOf is the view of the shown shows, the one before when nothing it
// depends on changed. first is the strip's first day, to the range's last
// day ("" for no end).
func (s *concerts) viewOf(shown []services.Concert, filterKey string, onlyYours bool, fetched int64, first time.Time, to string) *agendaView {
	k := viewKey{
		filter: filterKey, yours: onlyYours, fetched: fetched, n: len(shown),
		from: isoDay(first), to: to, locale: i18n.Locales()[0],
	}
	if n := len(shown); n > 0 {
		k.first, k.last = shown[0].ID, shown[n-1].ID
	}
	v := &s.page.view
	if v.months != nil && v.key == k {
		return v
	}
	v.key, v.months = k, groupShows(shown)
	v.strip = comingDays(first, to, v.months)
	return v
}

// stripFrom is the strip's first day: today, or the first day of a range
// that starts later.
func stripFrom(now time.Time, rng dayRange, hasRange bool) time.Time {
	first := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if hasRange && rng.from > isoDay(first) {
		if d, ok := parseDay(rng.from); ok {
			first = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
		}
	}
	return first
}

// stripTo is the strip's last day: the range's, "" for none.
func stripTo(rng dayRange, hasRange bool) string {
	if hasRange {
		return rng.to
	}
	return ""
}

// jump is a day of the strip chosen: the agenda scrolls to the day and the
// focus lands on its first control.
func (s *concerts) jump(c *ui.Context, d stripDay) {
	pg := &s.page
	if at, ok := pg.rov.firstStop(d.row, d.count); ok {
		pg.rov.want, pg.rov.wanting = at, true
	}
	pg.jump = d.day
	c.Invalidate()
}

// strip is the coming days as a row of cells, the weekday over the day
// number: a day with shows is a button that jumps to it in the agenda, and
// says how many with dots (three at most), a day without is dim and inert.
// The cells are one Tab stop; the arrows move along them.
func (s *concerts) strip(c *ui.Context, days []stripDay) {
	ui.Row(c.Key("days")).FillWidth().Gap(4).FocusGroup(ui.Horizontal).
		Role(ui.RoleGroup).Label(i18n.T("Coming days")).Children(func() {
		for _, d := range days {
			s.cell(c, d)
		}
	})
}

// cell is a day of the strip.
func (s *concerts) cell(c *ui.Context, d stripDay) {
	on := d.count > 0
	var e ui.Element
	if on {
		e = ui.ButtonBase(c.Key(d.day)).Label(d.label)
		if e.Hovered() {
			e.Background(theme.C("hover"))
		}
		if !theme.ReducedMotion() {
			m := theme.Current().Motion["feedback"]
			e.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
		}
	} else {
		e = ui.Box(c.Key(d.day)).Role(ui.RoleNone)
	}
	e.Grow(1).Basis(0).MinWidth(0).MaxWidth(cellMax).Radius(theme.Current().Radius["md"])
	e.Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(6, 0).Children(func() {
			if on {
				text(c, d.weekday, "meta", "ink-muted").FontFeatures("tnum").SingleLine()
				text(c, d.num, "heading", "ink").FontFeatures("tnum").SingleLine()
			} else {
				text(c, d.weekday, "meta", "ink-faint").FontFeatures("tnum").SingleLine()
				text(c, d.num, "heading", "ink-faint").FontWeight(400).FontFeatures("tnum").SingleLine()
			}
			ui.Row(c).Gap(3).Height(4).Margin(4, 0, 0, 0).Children(func() {
				for range min(d.count, cellDots) {
					ui.Box(c).Size(4, 4).Radius(2).Background(theme.C("ink")).Role(ui.RoleNone)
				}
			})
		})
	})
	if on {
		kit.TooltipDescribed(c, e, d.label, "")
		if e.Clicked() {
			s.jump(c, d)
		}
	}
}

// agenda is the shows by month, then by the venue's day: each day a ticket
// stub, its weekday over its number, a rule, and that day's rows. The whole
// list is one Tab stop (roving.go). loading is whether the shows are the last
// filters' while the next ones load: the list is Busy. more is whether Apple
// has more shows than the list holds, which leaves its last month's count
// out, as it may be short. top is the space above the list.
func (s *concerts) agenda(c *ui.Context, v *agendaView, loading, more bool, top float32) {
	rov := &s.page.rov
	rov.begin()
	var menus []menuShow
	list := ui.Column(c.Key("agenda")).FocusGroup(ui.Horizontal|ui.Vertical).Busy(loading).Margin(top, 0, 0, 0)
	list.Children(func() {
		for i, m := range v.months {
			sec := ui.Column(c.Key(m.key)).Role(ui.RoleGroup).Label(m.title)
			if i > 0 {
				sec.Margin(48, 0, 0, 0)
			}
			sec.Children(func() {
				ui.Row(c).AlignItems(ui.Baseline).Gap(8).Margin(0, 0, 8, 0).Children(func() {
					text(c, m.title, "title", "ink").SingleLine().Role(ui.RoleHeading).Level(2)
					if !more || i < len(v.months)-1 {
						text(c, "·", "secondary", "ink-faint").Role(ui.RoleNone)
						text(c, m.counted, "secondary", "ink-muted").SingleLine()
					}
				})
				for _, day := range m.days {
					s.dayBlock(c, day, rov, &menus)
				}
			})
		}
	})
	rov.end(c, list)
	for _, show := range menus {
		show(c)
	}
	s.page.jump = ""
}

// dayBlock is a day: the stub (weekday over the day number), a 1px rule, and
// the day's shows as rows.
func (s *concerts) dayBlock(c *ui.Context, day dayGroup, rov *roving, menus *[]menuShow) {
	blk := ui.Row(c.Key(day.day)).FillWidth().AlignItems(ui.Stretch).Padding(4, 0)
	if s.page.jump == day.day {
		blk.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollStart})
	}
	blk.Children(func() {
		ui.Column(c.Key("date")).Width(stubWidth).Shrink(0).Padding(10, 0, 0, 0).Children(func() {
			text(c, day.weekday, "meta", "ink-muted").FontFeatures("tnum").SingleLine()
			text(c, day.number, "display", "ink").FontFeatures("tnum").SingleLine()
		})
		rule(c.Key("rule"), 0)
		ui.Column(c.Key("rows")).Grow(1).MinWidth(0).Margin(0, 0, 0, 8).Role(ui.RoleList).Label(day.long).Children(func() {
			for i, con := range day.shows {
				concertRow(c, s.env, con, rowOpts{facts: &day.facts[i], rov: rov, row: rov.row(), menus: menus})
			}
		})
	})
}
