package concerts

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry, from AppShell's main and PageHeader (DIPs).
const (
	padTop    = 28 // pt-7
	padBottom = 48 // pb-12
)

// anyChoice stands for no filter in a select: a select holds no empty value.
const anyChoice = "any"

// dayFormat is how the date fields are written and read: "2026-11-14", the
// URL's own form.
const dayFormat = "2006-01-02"

// dateField is a From or To field: the text being typed, and the URL's day
// it follows (MyGo has no date input that may stand empty with a first and
// a last day, so the field is text in the URL's own form, "2026-11-14").
type dateField struct {
	text, url string
	init      bool
}

// follow makes the text the URL's when the URL changed under it.
func (f *dateField) follow(url string) {
	if !f.init || f.url != url {
		f.text, f.url, f.init = url, url, true
	}
}

// day is text as a calendar day; false when it is none.
func parseDay(s string) (time.Time, bool) {
	t, err := time.Parse(dayFormat, s)
	return t, err == nil
}

// validDay is whether s is a day within lo and hi (either may be ""):
// the field's own check that a native date input's min and max made.
func validDay(s, lo, hi string) bool {
	if _, ok := parseDay(s); !ok {
		return false
	}
	return (lo == "" || s >= lo) && (hi == "" || s <= hi)
}

// concertsPage is ConcertsPage: the header with the location button, the
// filters, and the agenda, in one scroll container.
func (s *concerts) concertsPage(c *ui.Context, r *ui.Route) {
	p := paramsOf(r)
	now := s.today()
	rng, hasRange := rangeFor(p, now)
	filterKey := rng.from + "|" + rng.to + "|" + p.genre
	pg := &s.page
	if pg.more.key != filterKey {
		pg.more.key, pg.more.limit = filterKey, pageSize
	}
	limit := pg.more.limit

	q := s.concerts(requestOf(rng, hasRange, p.genre, limit))
	fallback := s.location()
	// A change of filter keeps the list on screen until the new one
	// arrives, so the page never flashes empty between choices.
	var page *services.ConcertsPage
	placeholder := false
	switch {
	case q.HasData:
		d := q.Data
		page, pg.prev = &d, &d
	case q.Err == nil && pg.prev != nil:
		page, placeholder = pg.prev, true
	}
	var loc *services.ConcertLocation
	switch {
	case page != nil:
		loc = &page.Location
	case fallback.HasData:
		loc = &fallback.Data
	}

	yoursAvailable := account.HasAccount(s.authState()) && page != nil && page.YourArtists > 0
	onlyYours := p.yours && yoursAvailable
	var shown []services.Concert
	if page != nil {
		for _, con := range page.Concerts {
			if !onlyYours || con.YourArtist {
				shown = append(shown, con)
			}
		}
	}
	filtered := hasRange || p.genre != "" || onlyYours
	if q.HasData {
		s.announce(c, filterKey, onlyYours, len(shown), page.Location.Name, filtered)
	}

	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	devstate.Scroll(c, &pg.pos) // dev builds: the offset survives a restart
	sc := ui.Scroll(c).TrackScroll(&pg.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		ui.Column(c).Padding(padTop, gutter+tk.Metric["scrollbar"], padBottom, gutter).Children(func() {
			s.header(c, page, loc)
			s.filters(c, p, page, yoursAvailable, now)
			switch {
			case q.Err != nil && !q.HasData:
				kit.ErrorBlock(c, i18n.T("Couldn't load the concerts"), q.Err, kit.ErrorBlockOpts{
					OnRetry:  func() { s.env.Queries.Invalidate(store.Key{"concerts", "page"}) },
					Retrying: q.Fetching,
				})
			case page == nil:
				skeleton(c, 5)
			case len(shown) == 0:
				s.nothing(c, page.Location.Name, filtered)
			default:
				if placeholder {
					busy(c) // the last filters' list stays meanwhile
				}
				s.agenda(c, shown)
				if page.More && !onlyYours {
					busy := q.Fetching || placeholder
					ui.Row(c.Key("more")).FillWidth().Justify(ui.Center).Margin(32, 0, 0, 0).Children(func() {
						label := i18n.T("Show more concerts")
						if busy {
							label = i18n.T("Loading…")
						}
						if kit.Button(c, label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: busy}).Clicked() {
							pg.more.limit += pageSize
						}
					})
				}
			}
		})
	})
	s.where.show(c, s.data, loc)
}

// announce reads out what a changed filter lists, when its listing arrives
// (v0.2.0 only marked the old one busy): how many concerts, or that there
// are none. key names the filters; the listing the page opens with is only
// noted, as opening the page announces itself.
func (s *concerts) announce(c *ui.Context, key string, onlyYours bool, n int, city string, filtered bool) {
	pg := &s.page
	if pg.heardSet && pg.heard == key && pg.heardYours == onlyYours {
		return
	}
	first := !pg.heardSet
	pg.heard, pg.heardYours, pg.heardSet = key, onlyYours, true
	if first {
		return
	}
	switch {
	case n > 0:
		c.Announce(i18n.T("{count, plural, one {# concert} other {# concerts}}", "count", n))
	case filtered:
		c.Announce(i18n.T("No concerts match"))
	default:
		c.Announce(i18n.T("No concerts near {city}", "city", city))
	}
}

// busy marks the agenda while the next listing loads and the last one stays
// on screen (v0.2.0's aria-busy): a status of no size, read as "Loading".
func busy(c *ui.Context) {
	ui.Box(c.Key("busy")).Size(0, 0).Role(ui.RoleStatus).Label(i18n.T("Loading"))
}

// header is PageHeader: the title, what the list is about, and the location
// button.
func (s *concerts) header(c *ui.Context, page *services.ConcertsPage, loc *services.ConcertLocation) {
	subline := ""
	if loc != nil {
		subline = i18n.T("Near {city}", "city", loc.Name)
		if loc.IsDefault {
			subline = i18n.T("Near {city}, the main city of your Apple Music storefront", "city", loc.Name)
		}
		if page != nil && page.FetchedAt > 0 {
			when := i18n.RelativeTime(time.UnixMilli(page.FetchedAt), time.Now())
			if page.Stale {
				subline += " · " + i18n.T("Couldn't reach Apple Music · showing the copy from {when}", "when", when)
			} else {
				subline += " · " + i18n.T("Updated {when}", "when", when)
			}
		}
	}
	kit.PageHeader(c, i18n.T("Concerts"), subline, func() {
		label := i18n.T("Location")
		if loc != nil {
			label = loc.Name
			if loc.IsDefault {
				label = i18n.T("Set location")
			}
		}
		if kit.Button(c.Key("location"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "map-pin"}).Clicked() {
			s.where.Show()
		}
	})
}

// set is the URL with the parameters changed, in place of the page's entry.
func (s *concerts) set(p params) { s.env.Nav.Replace(p.location()) }

// filters is the group labelled "Filter concerts": the Dates select with
// From and To fields, the Genres select, and the Your artists switch.
func (s *concerts) filters(c *ui.Context, p params, page *services.ConcertsPage, yoursAvailable bool, now time.Time) {
	ui.Row(c.Key("filters")).Wrap().GapX(12).GapY(8).AlignItems(ui.Center).Margin(0, 0, 24, 0).FillWidth().
		Role(ui.RoleGroup).Label(i18n.T("Filter concerts")).Children(func() {
		when := p.when
		if when == "" {
			when = anyChoice
		}
		sel := kit.Select(c.Key("dates"), &when, []kit.SelectItem{
			kit.SelectOption(anyChoice, i18n.T("Any date")),
			kit.SelectOption(whenWeek, i18n.T("This week")),
			kit.SelectOption(whenWeekend, i18n.T("This weekend")),
			kit.SelectOption(whenMonth, i18n.T("Next 30 days")),
			kit.SelectOption(whenDates, i18n.T("Choose dates…")),
		}, kit.SelectOpts{Small: true, Width: 176, A11yLabel: i18n.T("Dates")})
		if sel.Changed() {
			next := p
			switch when {
			case anyChoice:
				next.when, next.from, next.to = "", "", ""
			case whenDates:
				next.when = when
			default:
				next.when, next.from, next.to = when, "", ""
			}
			s.set(next)
		}

		if p.when == whenDates {
			pg := &s.page
			pg.from.follow(p.from)
			pg.to.follow(p.to)
			s.dateInput(c.Key("from"), i18n.T("From"), &pg.from, isoDay(now), p.to, func(v string) { next := p; next.from = v; s.set(next) })
			s.dateInput(c.Key("to"), i18n.T("To"), &pg.to, p.from, "", func(v string) { next := p; next.to = v; s.set(next) })
			if _, ok := rangeFor(p, now); !ok {
				text(c, i18n.T("Choose both dates"), "secondary", "ink-muted")
			}
		}

		if page != nil && len(page.Genres) > 1 {
			genre := p.genre
			if genre == "" {
				genre = anyChoice
			}
			items := make([]kit.SelectItem, len(page.Genres))
			for i, g := range page.Genres {
				if g.ID == "" {
					items[i] = kit.SelectOption(anyChoice, i18n.T("All genres"))
				} else {
					items[i] = kit.SelectOption(g.ID, g.Name)
				}
			}
			gs := kit.Select(c.Key("genres"), &genre, items, kit.SelectOpts{Small: true, Width: 192, A11yLabel: i18n.T("Genres")})
			if gs.Changed() {
				next := p
				next.genre = genre
				if genre == anyChoice {
					next.genre = ""
				}
				s.set(next)
			}
		}

		if yoursAvailable {
			yours := p.yours
			label := i18n.T("Your artists ({yours})", "yours", i18n.FormatNumber(float64(page.YourArtists)))
			ui.Row(c.Key("yours")).Gap(8).AlignItems(ui.Center).Margin(0, 0, 0, 4).Children(func() {
				sw := kit.Switch(c, &yours, kit.SwitchOpts{Small: true, A11yLabel: label})
				// v0.2.0 wraps the switch and its text in a <label>: the text toggles it.
				txt := text(c, label, "body", "ink").SingleLine()
				if txt.Clicked() {
					yours = !yours
					next := p
					next.yours = yours
					s.set(next)
				}
				if sw.Changed() {
					next := p
					next.yours = yours
					s.set(next)
				}
			})
		}
	})
}

// dateInput is a labelled day field; lo and hi bound the days it takes
// ("" for none). A day outside them, or half typed, shows invalid and
// changes nothing; a valid one, or none, is handed to set.
func (s *concerts) dateInput(c *ui.Context, label string, f *dateField, lo, hi string, set func(string)) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		text(c, label, "secondary", "ink-muted")
		in := kit.Input(c.Key("in"), &f.text, kit.InputOpts{
			Placeholder: dayHint, A11yLabel: label, Invalid: f.text != "" && !validDay(f.text, lo, hi),
			Error: i18n.T("Enter a day as {form}", "form", dayHint),
		}).Width(160).Height(32)
		if in.Changed() {
			switch {
			case f.text == "":
				set("")
			case validDay(f.text, lo, hi):
				set(f.text)
			}
		}
	})
}

// dayHint is the form a day is typed in.
const dayHint = "YYYY-MM-DD"

// nothing is the empty page: nothing matches the filters, or nothing is
// listed near the city.
func (s *concerts) nothing(c *ui.Context, city string, filtered bool) {
	if filtered {
		kit.EmptyState(c, i18n.T("No concerts"),
			i18n.T("Apple Music lists nothing near {city} for these filters. Widen the dates or choose another genre.", "city", city),
			kit.EmptyOpts{Action: &kit.EmptyAction{Label: i18n.T("Clear filters"), Run: func() { s.set(params{}) }}})
		return
	}
	kit.EmptyState(c, i18n.T("No concerts"),
		i18n.T("No upcoming concerts. Please check back later.")+" "+i18n.T("A larger city nearby may have some."),
		kit.EmptyOpts{Action: &kit.EmptyAction{Label: i18n.T("Set location"), Run: s.where.Show}})
}

// agenda is the shows by month, then by the venue's day: a date column and
// that day's rows. The whole list is one Tab stop (roving.go).
func (s *concerts) agenda(c *ui.Context, shows []services.Concert) {
	groups := groupShows(shows)
	rov := &s.page.rov
	rov.begin()
	var menus []menuShow
	list := ui.Column(c.Key("agenda")).FocusGroup(ui.Horizontal | ui.Vertical)
	list.Children(func() {
		for i, m := range groups {
			sec := ui.Column(c.Key(m.key)).Role(ui.RoleGroup).Label(m.title)
			if i > 0 {
				sec.Margin(40, 0, 0, 0)
			}
			sec.Children(func() {
				text(c, m.title, "heading", "ink").Role(ui.RoleHeading).Level(2).Margin(0, 0, 8, 0)
				for _, day := range m.days {
					ui.Row(c.Key(day.day)).FillWidth().Gap(16).Padding(4, 0).AlignItems(ui.Start).Children(func() {
						dateBlock(c.Key("date"), day.shows[0], false).Padding(12, 8, 0, 8)
						ui.Column(c.Key("rows")).Grow(1).MinWidth(0).Role(ui.RoleList).Label(showDayLong(day.shows[0])).Children(func() {
							for _, con := range day.shows {
								concertRow(c, s.env, con, rowOpts{rov: rov, row: rov.row(), menus: &menus})
							}
						})
					})
				}
			})
		}
	})
	rov.end(c, list)
	for _, show := range menus {
		show(c)
	}
}
