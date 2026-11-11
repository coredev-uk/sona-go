package concerts

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// rowHeight is a concert row's height (ConcertRow's min-h-[72px]).
const rowHeight = 72

// text is a line of text in a type style and ink colour.
func text(c *ui.Context, s, style, ink string) ui.Element {
	t := theme.Current().Type[style]
	e := ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).TextColor(theme.C(ink))
	if t.Tracking != 0 {
		e.LetterSpacing(t.Tracking)
	}
	return e
}

// dateBlock is the day as a ledger's date column: the weekday over the day
// number (and the month, off a day heading).
func dateBlock(c *ui.Context, con services.Concert, month bool) ui.Element {
	return ui.Column(c).Width(56).Shrink(0).Children(func() {
		text(c, showWeekday(con), "meta", "ink-muted").FontFeatures("tnum").SingleLine()
		text(c, showDate(con), "title", "ink").FontFeatures("tnum").SingleLine()
		if month {
			text(c, showMonthShort(con), "meta", "ink-muted").FontFeatures("tnum").SingleLine()
		}
	})
}

// menuShow draws a tickets menu once the list is built: its panel is an
// overlay, which stays out of the agenda's focus group.
type menuShow func(c *ui.Context)

// rowOpts shape a concert row.
type rowOpts struct {
	// date puts the day's date column first, for lists that don't sit
	// under a day heading.
	date bool
	// artist is a row on an artist's own page: the portrait and the name
	// are left out, since the page is about them; the venue leads, and the
	// line-up only when others play too.
	artist bool
	// rov and row make the row's controls stops of the agenda's roving
	// focus; nil rov leaves them as Tab stops.
	rov *roving
	row int
	// menus collects tickets menus to show after the list; nil shows them
	// in place.
	menus *[]menuShow
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

// ticketButton is a ghost small button with its arrow after the label
// (button.tsx's ghost sm: the kit's Button puts icons first).
func ticketButton(c *ui.Context, label, a11y, icon string) ui.Element {
	e := ui.ButtonBase(c).Height(32).Gap(6).PaddingX(12).Radius(theme.Current().Radius["md"]).Label(a11y)
	bg := ui.Transparent
	if e.Hovered() {
		bg = theme.C("accent-soft")
	}
	e.Background(bg).TextColor(theme.C("ink"))
	if !theme.ReducedMotion() {
		m := theme.Current().Motion["feedback"]
		e.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
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
		btn := ticketButton(c.Key("tickets"), label, a11y, "arrow-up-right")
		if o.rov != nil {
			o.rov.stop(btn, o.row)
		}
		if btn.Clicked() {
			open(env, sellers[0].URL)
		}
		return
	}
	a11y := i18n.T("Get tickets for {lineup}", "lineup", con.Lineup)
	btn := ticketButton(c.Key("tickets"), label, a11y, "chevron-down")
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

// concertRow is one show: the headliner's portrait, the line-up (a link to
// the artist), the venue, the start at the venue's clock and how far it is,
// and tickets (ConcertRow.tsx).
func concertRow(c *ui.Context, env *module.Env, con services.Concert, o rowOpts) {
	var lead *services.ConcertArtist
	if len(con.Artists) > 0 {
		lead = &con.Artists[0]
	}
	meta := showTime(con)
	if d := distanceLabel(con.DistanceKm); d != "" {
		meta += " · " + d
	}
	title, place, others := con.Lineup, venueLine(con), ""
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
		if o.date {
			dateBlock(c.Key("date"), con, true).Margin(0, 4, 0, 0)
		}
		if !o.artist {
			art := ""
			if lead != nil {
				art = lead.Art.Medium
			}
			kit.Artwork(c.Key("art"), env.Art, art, 56, kit.ArtOpts{Round: true, Edge: true})
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if !o.artist && lead != nil && lead.ID != "" && env.HasRoute != nil && env.HasRoute("/artist/"+lead.ID) {
				t := theme.Current().Type["label"]
				link := ui.Link(c.Key("name"), title, "/artist/"+lead.ID).FontSize(t.Size).FixedLineHeight(t.LineHeight).
					FontWeight(int(t.Weight)).TextColor(theme.C("ink")).SingleLine().MinWidth(0)
				if o.rov != nil {
					o.rov.stop(link, o.row)
				}
			} else {
				text(c, title, "label", "ink").SingleLine().MinWidth(0)
			}
			if place != "" {
				text(c, place, "secondary", "ink-muted").SingleLine().MinWidth(0)
			}
			if others != "" {
				text(c, others, "secondary", "ink-muted").SingleLine().MinWidth(0)
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
		})
		tickets(c, env, con, o)
	})
}

// skeleton is the agenda's frame while it loads: a month heading and rows
// at their real height (AgendaSkeleton).
func skeleton(c *ui.Context, rows int) ui.Element {
	e := ui.Column(c.Key("skeleton")).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	e.Children(func() {
		kit.Skeleton(c.Key("month"), 128, 16).Margin(0, 0, 12, 0)
		for i := range rows {
			ui.Row(c.Key(i)).MinHeight(rowHeight).Gap(16).Padding(8, 8).AlignItems(ui.Center).Children(func() {
				kit.Skeleton(c.Key("date"), 56, 40)
				kit.Skeleton(c.Key("art"), 56, 56).Radius(28)
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
