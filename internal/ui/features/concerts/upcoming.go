package concerts

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

const (
	// artistRows are the rows an artist's shelf shows before "Show all".
	artistRows = 3
	// homeShows are the shows on Home's shelf, in two columns of three from
	// 900 DIPs wide.
	homeShows  = 6
	twoColumns = 900
)

// linkToConcerts is whether the Concerts page is registered: the shelves
// link to it only then.
func linkToConcerts(env *module.Env) bool {
	return env.HasRoute != nil && env.HasRoute(concertsPath)
}

// rows is the shows as rows with their date column: one column, or two
// filled top to bottom (v0.2.0's grid-flow-col with three rows). The list
// is named label.
func rows(c *ui.Context, env *module.Env, shows []services.Concert, columns int, artist bool, label string) {
	one := func(list []services.Concert) {
		for _, con := range list {
			concertRow(c, env, con, rowOpts{date: true, artist: artist})
		}
	}
	w, _ := c.Size()
	if columns < 2 || w < twoColumns {
		ui.Column(c.Key("rows")).Role(ui.RoleList).Label(label).Children(func() { one(shows) })
		return
	}
	half := min(homeShows/2, len(shows))
	ui.Row(c.Key("rows")).Gap(32).AlignItems(ui.Start).Role(ui.RoleList).Label(label).Children(func() {
		ui.Column(c.Key("left")).Grow(1).Basis(0).MinWidth(0).Children(func() { one(shows[:half]) })
		ui.Column(c.Key("right")).Grow(1).Basis(0).MinWidth(0).Children(func() { one(shows[half:]) })
	})
}

// ArtistConcerts is an artist's upcoming concerts, those near the chosen
// city first, as the shelf "Upcoming concerts" of the artist's page
// (Upcoming.tsx). Nothing is drawn while it loads, when there are none and
// when they can't be had: the page is about the artist, and it is complete
// without. first leaves out the space above, as for a page's first shelf.
func ArtistConcerts(c *ui.Context, env *module.Env, artistID string, first bool) {
	d := dataOf(env)
	q := d.artist(artistID)
	if !q.HasData {
		return
	}
	shows := append(append([]services.Concert(nil), q.Data.Nearby...), q.Data.Elsewhere...)
	if len(shows) == 0 {
		return
	}
	city := q.Data.Location.Name
	host := ui.Column(c.Key("concerts.artist." + artistID))
	all := ui.Local(host, "all", func() bool { return false })
	host.Children(func() {
		kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Upcoming concerts"), First: first, Actions: func() {
			if len(shows) <= artistRows {
				return
			}
			label := i18n.T("Show all {count}", "count", len(shows))
			if *all {
				label = i18n.T("Show fewer")
			}
			if kit.ShelfToggle(c, label, *all).Clicked() {
				*all = !*all
			}
		}}, func() {
			list := shows
			if !*all && len(list) > artistRows {
				list = list[:artistRows]
			}
			rows(c, env, list, 1, true, i18n.T("Upcoming concerts"))
			if linkToConcerts(env) {
				more := ui.Link(c.Key("more"), "", "").Margin(8, 0, 0, 8)
				more.Children(func() {
					text(c, i18n.T("More concerts near {city}", "city", city), "meta", "ink-faint").SingleLine()
				})
				if more.Clicked() {
					env.Nav.Push(concertsPath)
				}
			}
		})
	})
}

// ConcertsShelf is Home's concerts shelf: the next shows near the chosen
// city, with the agenda one link away. A failure leaves the shelf out (Home
// has plenty without); no shows says so, with the way to another city.
func ConcertsShelf(c *ui.Context, env *module.Env, first bool) {
	d := dataOf(env)
	q := d.concerts(requestOf(dayRange{}, false, "", pageSize))
	if q.Err != nil && !q.HasData {
		return
	}
	title, city := i18n.T("Concerts"), ""
	if q.HasData {
		city = q.Data.Location.Name
		title = i18n.T("Concerts in {city}", "city", city)
	}
	opts := kit.ShelfOpts{Title: title, First: first}
	if linkToConcerts(env) {
		opts.More = func() { env.Nav.Push(concertsPath) }
	}
	kit.Shelf(c, opts, func() {
		switch {
		case !q.HasData:
			shelfSkeleton(c, 3)
		case len(q.Data.Concerts) == 0:
			empty := kit.EmptyOpts{}
			if linkToConcerts(env) {
				empty.Action = &kit.EmptyAction{Label: i18n.T("Set location"), Run: func() { env.Nav.Push(concertsPath) }}
			}
			kit.EmptyState(c, i18n.T("No concerts"),
				i18n.T("No upcoming concerts. Please check back later."), empty)
		default:
			shows := q.Data.Concerts
			rows(c, env, shows[:min(len(shows), homeShows)], 2, false, title)
		}
	})
}
