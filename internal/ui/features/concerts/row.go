package concerts

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// rowHeight is a concert row's least height.
const rowHeight = 72

// The widths of a row's fixed parts: the clock at the agenda's start of a
// row, the portrait, and the date column of a shelf's row.
const (
	timeWidth = 72
	portrait  = 56
	dateWidth = 44
)

// text is a line of text in a type style and ink colour.
func text(c *ui.Context, s, style, ink string) ui.Element {
	return theme.Text(c, s, style).TextColor(theme.C(ink))
}

// rule is the 1px line of a stub, the day's date on one side and its shows
// on the other: h high, or as high as its row when h is 0.
func rule(c *ui.Context, h float32) ui.Element {
	e := ui.Box(c).Width(1).Shrink(0).Role(ui.RoleNone).Background(theme.C("line"))
	if h > 0 {
		e.Height(h)
	}
	return e
}

// dateBlock is the day on a shelf's row, which has no day heading above it:
// the weekday over the day number and the month.
func dateBlock(c *ui.Context, con services.Concert) ui.Element {
	return ui.Column(c).Width(dateWidth).Shrink(0).Children(func() {
		text(c, showWeekday(con), "meta", "ink-muted").FontFeatures("tnum").SingleLine()
		text(c, showDate(con), "title", "ink").FontFeatures("tnum").SingleLine()
		text(c, showMonthShort(con), "meta", "ink-muted").FontFeatures("tnum").SingleLine()
	})
}

// menuShow draws a tickets menu once the list is built: its panel is an
// overlay, which stays out of the agenda's focus group.
type menuShow func(c *ui.Context)

// rowOpts shape a concert row.
type rowOpts struct {
	// date puts the day's date column and its rule first, for lists that
	// don't sit under a day's stub; such a row (a shelf's) is the compact
	// one, its start, distance and mark under the venue.
	date bool
	// artist is a row on an artist's own page: the portrait and the name
	// are left out, since the page is about them; the venue leads, and the
	// line-up only when others play too.
	artist bool
	// facts are the row's words, made once for the listing; nil makes them
	// here.
	facts *facts
	// rov and row make the row's controls stops of the agenda's roving
	// focus; nil rov leaves them as Tab stops.
	rov *roving
	row int
	// menus collects tickets menus to show after the list; nil shows them
	// in place.
	menus *[]menuShow
}

// wide is the agenda's row: the start leads, and the distance sits by the
// venue. The others are a shelf's.
func (o rowOpts) wide() bool { return !o.date && !o.artist }

// facts are the words a row sets beside the line-up. The agenda makes them
// once for each listing, not in every frame.
type facts struct {
	// clock is the start at the venue's clock, away how far it is ("" when
	// unknown), place "Example Hall · London".
	clock, away, place string
}

func factsOf(c services.Concert) facts {
	return facts{clock: showTime(c), away: distanceLabel(c.DistanceKm), place: venueLine(c)}
}

// open opens a ticket seller's page in the browser; Go let through only
// https links.
func open(env *module.Env, url string) {
	if env.OpenURL == nil {
		return
	}
	if err := env.OpenURL(url); err != nil && env.Log != nil {
		env.Log.Warn("concerts: open tickets", "err", err)
	}
}

// ticketButton is the tickets control, its arrow after the label (the kit's
// Button puts icons first): outlined on the agenda, where it is every row's
// one action, a ghost button on a shelf.
func ticketButton(c *ui.Context, label, a11y, icon string, outlined bool) ui.Element {
	e := ui.ButtonBase(c).Height(32).Gap(6).PaddingX(12).Radius(theme.Current().Radius["md"]).Label(a11y)
	bg := ui.Transparent
	if e.Hovered() {
		bg = theme.C("accent-soft")
	}
	e.Background(bg).TextColor(theme.C("ink"))
	if outlined {
		e.Border(1, theme.C("control-line"))
	}
	theme.Fade(e, "feedback")
	e.Children(func() {
		text(c, label, "label", "ink").SingleLine()
		theme.Glyph(c, icon, 14, theme.GlyphOpts{Color: theme.C("ink"), Inherit: true})
	})
	return e
}

// tickets is the tickets control: one seller is a button, several a menu
// of them, none nothing.
func tickets(c *ui.Context, env *module.Env, con services.Concert, o rowOpts) {
	sellers := con.Tickets
	if len(sellers) == 0 {
		return
	}
	label := i18n.T("Get tickets")
	if len(sellers) == 1 {
		a11y := i18n.T("Get tickets for {lineup} from {seller}, opens in your browser", "lineup", con.Lineup, "seller", sellers[0].Vendor)
		btn := ticketButton(c.Key("tickets"), label, a11y, "arrow-up-right", o.wide())
		if o.rov != nil {
			o.rov.stop(btn, o.row)
		}
		if btn.Clicked() {
			open(env, sellers[0].URL)
		}
		return
	}
	a11y := i18n.T("Get tickets for {lineup}", "lineup", con.Lineup)
	btn := ticketButton(c.Key("tickets"), label, a11y, "chevron-down", o.wide())
	if o.rov != nil {
		o.rov.stop(btn, o.row)
	}
	menu := ui.Local(btn, "menu", func() kit.Menu { return kit.Menu{} })
	btn.Role(ui.RoleMenuButton).HasPopup(ui.PopupMenu).Expanded(menu.IsOpen())
	if btn.Clicked() {
		if menu.IsOpen() {
			menu.Close()
		} else {
			menu.OpenFrom(btn.FocusVisible())
		}
	}
	items := make([]kit.MenuItem, len(sellers))
	for i, s := range sellers {
		items[i] = kit.MenuItem{
			Key: s.URL, Label: s.Vendor, Icon: "arrow-up-right", Description: i18n.T("Opens in your browser"),
			Run: func() { open(env, s.URL) },
		}
	}
	show := menuShow(func(c *ui.Context) {
		menu.Show(c, kit.MenuOpts{ID: "tickets:" + con.ID, Label: a11y}, btn, items)
	})
	if o.menus != nil {
		*o.menus = append(*o.menus, show)
		return
	}
	show(c)
}

// lineupLink is the line-up as a link to the headliner's page when the page
// exists, else plain text, in the type style named.
func lineupLink(c *ui.Context, env *module.Env, con services.Concert, lead *services.ConcertArtist, title, style string, o rowOpts) {
	if lead != nil && lead.ID != "" && env.HasRoute != nil && env.HasRoute("/artist/"+lead.ID) {
		t := theme.Current().Type[style]
		link := ui.Link(c.Key("name"), title, "/artist/"+lead.ID).FontSize(t.Size).FixedLineHeight(t.LineHeight).
			FontWeight(int(t.Weight)).TextColor(theme.C("ink")).SingleLine().MinWidth(0)
		if o.rov != nil {
			o.rov.stop(link, o.row)
		}
		return
	}
	text(c, title, style, "ink").SingleLine().MinWidth(0)
}

// concertRow is one show. On the agenda it reads left to right as a
// timetable: the start at the venue's clock, the headliner's portrait, the
// line-up (a link to the artist) over the venue and how far it is, and
// tickets at the row's end. A shelf's row (o.date) puts its date and rule
// first and its start and distance under the venue; an artist's own (o.artist)
// leads with the venue.
func concertRow(c *ui.Context, env *module.Env, con services.Concert, o rowOpts) {
	var lead *services.ConcertArtist
	if len(con.Artists) > 0 {
		lead = &con.Artists[0]
	}
	f := o.facts
	if f == nil {
		v := factsOf(con)
		f = &v
	}
	wide := o.wide()
	title, place, others := con.Lineup, f.place, ""
	if o.artist {
		title, place = con.Lineup, ""
		if con.Venue != nil {
			title, place = con.Venue.Name, con.Venue.City
		}
		if lead != nil && con.Lineup != lead.Name {
			others = con.Lineup
		}
	}

	row := ui.Row(c.Key(con.ID)).MinHeight(rowHeight).Gap(16).Padding(8, 8).AlignItems(ui.Center).
		Radius(theme.Current().Radius["md"]).Role(ui.RoleListItem)
	if row.Hovered() {
		row.Background(theme.C("hover"))
	}
	row.Children(func() {
		switch {
		case wide:
			text(c.Key("time"), f.clock, "label", "ink").Width(timeWidth).Shrink(0)
		case o.date:
			dateBlock(c.Key("date"), con)
			rule(c.Key("rule"), 48)
		}
		if !o.artist {
			art := ""
			if lead != nil {
				art = lead.Art.Medium
			}
			kit.Artwork(c.Key("art"), env.Art, art, portrait, kit.ArtOpts{Round: true, Edge: true})
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if o.artist {
				text(c, title, "label", "ink").SingleLine().MinWidth(0)
			} else if wide {
				lineupLink(c, env, con, lead, title, "heading", o)
			} else {
				lineupLink(c, env, con, lead, title, "label", o)
			}
			switch {
			case wide:
				ui.Row(c).MinWidth(0).Gap(8).Children(func() {
					if place != "" {
						text(c, place, "secondary", "ink-muted").SingleLine().MinWidth(0)
					}
					if f.away != "" {
						text(c, f.away, "meta", "ink-faint").SingleLine().Shrink(0).Margin(1, 0, 0, 0)
					}
				})
				if con.YourArtist {
					text(c, i18n.T("Your artist"), "meta", "ink-muted").SingleLine()
				}
			default:
				if place != "" {
					text(c, place, "secondary", "ink-muted").SingleLine().MinWidth(0)
				}
				if others != "" {
					text(c, others, "secondary", "ink-muted").SingleLine().MinWidth(0)
				}
				meta := f.clock
				if f.away != "" {
					meta += " · " + f.away
				}
				ui.Row(c).MinWidth(0).Gap(4).Children(func() {
					text(c, meta, "meta", "ink-faint").SingleLine()
					if con.YourArtist {
						if meta != "" {
							text(c, "·", "meta", "ink-faint").SingleLine()
						}
						text(c, i18n.T("Your artist"), "meta", "ink-muted").SingleLine()
					}
				})
			}
		})
		tickets(c, env, con, o)
	})
}

// skeleton is the agenda's frame while it loads: a month heading, then days
// with their date column, rule and rows at their real height, the first
// holding two shows. A shelf has no days: see shelfSkeleton.
func skeleton(c *ui.Context) ui.Element {
	e := ui.Column(c.Key("skeleton")).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	e.Children(func() {
		kit.Skeleton(c.Key("month"), 168, 24).Margin(0, 0, 12, 0)
		for i, shows := range []int{2, 1, 2} {
			ui.Row(c.Key(i)).FillWidth().AlignItems(ui.Stretch).Padding(4, 0).Children(func() {
				ui.Column(c.Key("date")).Width(stubWidth).Shrink(0).Padding(10, 0, 0, 0).Gap(4).Children(func() {
					kit.Skeleton(c.Key("weekday"), 28, 12)
					kit.Skeleton(c.Key("day"), 40, 36)
				})
				rule(c.Key("rule"), 0)
				ui.Column(c.Key("rows")).Grow(1).MinWidth(0).Margin(0, 0, 0, 8).Children(func() {
					for j := range shows {
						ui.Row(c.Key(j)).MinHeight(rowHeight).Gap(16).Padding(8, 8).AlignItems(ui.Center).Children(func() {
							kit.Skeleton(c.Key("time"), 56, 14).Margin(0, 24, 0, 0)
							kit.Skeleton(c.Key("art"), portrait, portrait).Radius(portrait / 2)
							ui.Column(c).Grow(1).Gap(8).Children(func() {
								kit.Skeleton(c.Key("a"), 200, 16)
								kit.Skeleton(c.Key("b"), 140, 12)
							})
							kit.Skeleton(c.Key("btn"), 112, 32)
						})
					}
				})
			})
		}
	})
	return e
}

// shelfSkeleton is a shelf's rows while they load.
func shelfSkeleton(c *ui.Context, rows int) ui.Element {
	e := ui.Column(c.Key("skeleton")).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	e.Children(func() {
		for i := range rows {
			ui.Row(c.Key(i)).MinHeight(rowHeight).Gap(16).Padding(8, 8).AlignItems(ui.Center).Children(func() {
				kit.Skeleton(c.Key("date"), dateWidth, 40)
				rule(c.Key("rule"), 48)
				kit.Skeleton(c.Key("art"), portrait, portrait).Radius(portrait / 2)
				ui.Column(c).Grow(1).Gap(8).Children(func() {
					kit.Skeleton(c.Key("a"), 200, 14)
					kit.Skeleton(c.Key("b"), 140, 12)
				})
				kit.Skeleton(c.Key("btn"), 80, 32)
			})
		}
	})
	return e
}
