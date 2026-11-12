package radio

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// liveNow is what's on air: Apple Music's live stations as the programme
// guide, then the live shelves of Apple's page (the local broadcasters), each
// under a ruled head. The guide's own heading sits over its station column.
func (r *radio) liveNow(c *ui.Context, live store.Result[[]services.Station], sections []services.Section, ctx kit.ItemsCtx) {
	pg := &r.radioPg
	title := i18n.T("Live now")
	ui.Column(c.Key("live-now")).Role(ui.RoleGroup).Label(title).Children(func() {
		head := railOpts{key: "live-state", title: title, style: "page", first: true}
		switch {
		case pending(live):
			r.onAirSkeleton(c, &pg.loadingBoard)
		case failed(live):
			pg.rail(c, head, func() {
				kit.ErrorBlock(c, i18n.T("Couldn't load the live stations"), live.Err, kit.ErrorBlockOpts{
					OnRetry: func() { r.env.Queries.Invalidate(keyLive) }, Retrying: live.Fetching,
				})
			})
		case len(live.Data) == 0:
			pg.rail(c, head, func() {
				kit.EmptyState(c, i18n.T("No live stations"), i18n.T("Apple Music lists no live radio for your storefront right now."), kit.EmptyOpts{})
			})
		default:
			r.onAirBoard(c, &pg.board, live.Data, ctx, pg.menus)
		}
		var other []services.Section
		for _, s := range sections {
			if s.Kind == services.SectionStations {
				r.broadcasters(c, s, ctx)
				continue
			}
			other = append(other, s)
		}
		if len(other) > 0 {
			ui.Column(c.Key("live-sections")).Margin(40, 0, 0, 0).Children(func() {
				kit.EditorialSections(c, &pg.live, other, ctx, kit.EditorialOpts{})
			})
		}
	})
}

// broadcasters is a station section of Apple's page (the local broadcasters)
// as a ruled block: its heading in the rail, "See all" under it, and the
// shared station list beside them.
func (r *radio) broadcasters(c *ui.Context, s services.Section, ctx kit.ItemsCtx) {
	pg := &r.radioPg
	if pg.stations == nil {
		pg.stations = map[string]*kit.StationListState{}
	}
	if pg.stations[s.ID] == nil {
		pg.stations[s.ID] = &kit.StationListState{}
	}
	var stations []services.Station
	for i := range s.Items {
		if s.Items[i].Station != nil {
			stations = append(stations, *s.Items[i].Station)
		}
	}
	var more func()
	if path := kit.RoomLink(s); path != "" {
		more = func() { r.env.Nav.Push(path) }
	}
	pg.rail(c, railOpts{key: "live-" + s.ID, title: s.Title, style: "heading", gap: 40, pad: 20, group: true, more: more}, func() {
		kit.StationList(c, pg.stations[s.ID], stations, ctx, kit.StationListOpts{
			Label: s.Title,
			OnPlay: func(station services.Station) {
				// Playing a station plays the section's item for it.
				at := slices.IndexFunc(s.Items, func(it services.Item) bool {
					return it.Station != nil && it.Station.ID == station.ID
				})
				if at >= 0 {
					ctx.Players.PlayItem(s.Items[at])
				}
			},
		})
	})
}

// railOpts are the options of rail.
type railOpts struct {
	key, title string
	// style is the heading's type role: "page" for a band, "heading" for a
	// section of one.
	style string
	// first: nothing above it, so no rule and no space.
	first bool
	// gap is the space above the rule, pad the space under it.
	gap, pad float32
	// group names the block for assistive technology.
	group bool
	// more is a "See all" under the heading.
	more func()
	// head replaces the heading (a loading frame's bar).
	head func()
}

// railed is true when the page is wide enough to hang headings in a rail.
func (f *frame) railed() bool {
	if f.width <= 0 {
		return true // the first frame; the next one knows the width
	}
	m := theme.Current().Metric
	return f.width-2*f.gutter()-m["scrollbar"] >= railMin
}

// rail lays out a block of the page as the guide does: a rule across the
// column, the heading hung in a rail at the left (as wide as the guide's
// station column) and the content beside it, starting where the guide's track
// starts. Where the window is too narrow for the rail the heading stands over
// the content.
func (pg *radioPage) rail(c *ui.Context, o railOpts, content func()) {
	box := ui.Column(c.Key(o.key))
	if o.group {
		box.Role(ui.RoleGroup).Label(o.title)
	}
	if !o.first {
		box.Margin(o.gap, 0, 0, 0).Padding(o.pad, 0, 0, 0)
	}
	railed := pg.railed()
	heading := func() {
		if o.head != nil {
			o.head()
			return
		}
		text(c, o.title, o.style).TextColor(theme.C("ink")).MaxLines(2).Role(ui.RoleHeading).Level(2)
		if o.more == nil {
			return
		}
		see := ui.ButtonBase(c.Key("more")).Role(ui.RoleLink).Height(28).PaddingX(6).Margin(6, 0, 0, -6).Gap(2).
			Radius(theme.Current().Radius["md"]).AlignSelf(ui.Start)
		ink := theme.C("ink-muted")
		if see.Hovered() {
			ink = theme.C("ink")
			see.Background(theme.C("hover"))
		}
		see.Children(func() {
			text(c, i18n.T("See all"), "sub").TextColor(ink).SingleLine()
			theme.Glyph(c, "chevron-right", 14, theme.GlyphOpts{Color: ink, Inherit: true})
		})
		if see.Clicked() {
			o.more()
		}
	}
	box.Children(func() {
		if !o.first {
			ui.Box(c.Key("rule")).Absolute().Top(0).Left(0).Right(0).Height(1).Background(theme.C("line"))
		}
		if !railed {
			ui.Column(c.Key("head")).Margin(0, 0, headingGap, 0).Children(heading)
			content()
			return
		}
		ui.Row(c.Key("rail")).FillWidth().AlignItems(ui.Start).Children(func() {
			ui.Column(c.Key("head")).Width(railW).Shrink(0).PaddingEnd(16).Sticky().Top(16).Children(heading)
			ui.Column(c.Key("content")).Grow(1).Basis(0).MinWidth(0).Children(content)
		})
	})
}

// band is a band of the page: a rule across the column, its heading in the
// rail, then its sections. lead builds what comes first (the user's
// stations).
func (r *radio) band(c *ui.Context, b band, lead bool, yours func(), ctx kit.ItemsCtx) {
	title := bandTitle(b.id)
	r.radioPg.rail(c, railOpts{key: "band-" + string(b.id), title: title, style: "page", gap: bandGap, pad: bandPad, group: true}, func() {
		if lead {
			ui.Column(c.Key("lead")).Children(yours)
		}
		if len(b.sections) > 0 {
			kit.EditorialSections(c, r.radioPg.bandState(b.id), b.sections, ctx, kit.EditorialOpts{First: !lead})
		}
	})
}

// yours is the Stations band's lead: the user's own stations as a shelf, or,
// signed out, an empty state with the way in.
func (r *radio) yours(c *ui.Context, state string, recent store.Result[[]services.Station]) {
	pg := &r.radioPg
	if account.HasAccount(state) && recent.HasData && len(recent.Data) > 0 {
		tiles := make([]kit.CoverItem, len(recent.Data))
		for i, s := range recent.Data {
			tiles[i] = r.stationCover(s)
		}
		kit.CoverShelf(c, &pg.yours, i18n.T("Your stations"), tiles, kit.CoverShelfOpts{
			CoverOpts: kit.CoverOpts{Art: r.env.Art, Motion: r.env.Motion, Menus: pg.yoursMenus},
			First:     true,
		})
		return
	}
	var action *kit.EmptyAction
	if r.env.Surfaces != nil {
		action = &kit.EmptyAction{Label: i18n.T("Sign in"), Run: r.env.Surfaces.OpenAccount}
	}
	kit.EmptyState(c, i18n.T("Your stations"), i18n.T("Sign in to see the stations you play in Apple Music on any of your devices."),
		kit.EmptyOpts{Action: action})
}

// bandsSkeleton is the frame of Apple's radio page while it loads: the local
// broadcasters' ruled head and rows, then the first band's rule, heading and
// shelves, each at the place it will land.
func (r *radio) bandsSkeleton(c *ui.Context) {
	pg := &r.radioPg
	hover := theme.C("hover")
	sm := theme.Current().Radius["sm"]
	bar := func(w, h float32) ui.Element {
		return ui.Box(c).Width(w).Height(h).Radius(sm).Background(hover)
	}
	headBar := func(w, h float32) func() { return func() { bar(w, h) } }
	ui.Column(c.Key("bands-skeleton")).Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		pg.rail(c, railOpts{key: "broadcasters", gap: 40, pad: 20, head: headBar(128, 22)}, func() {
			root := ui.Grid(c.Key("stations")).GapX(16).GapY(4).MarginX(-8)
			width := pg.loadingStations.track(c, root)
			if width > 0 {
				root.Columns(max(1, int((width+16)/(260+16))))
			}
			root.Children(func() {
				if width <= 0 {
					ui.Box(c).Height(1)
					return
				}
				for i := range 8 {
					ui.Row(c.Key(i)).Height(64).PaddingX(8).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(48, 48).Radius(sm).Background(hover).Shrink(0)
						bar(112, 14)
					})
				}
			})
		})
		pg.rail(c, railOpts{key: "first-band", gap: bandGap, pad: bandPad, head: headBar(112, 30)}, func() {
			ui.Column(c.Key("shelves")).Gap(40).Children(func() {
				for s := range 2 {
					ui.Column(c.Key(s)).Children(func() {
						ui.Row(c).Height(28).Margin(0, 0, 12, 0).AlignItems(ui.Center).Children(func() { bar(160, 16) })
						root := ui.Row(c.Key("tiles")).Gap(theme.Current().Metric["tile-gap-x"]).FillWidth().Clip()
						width := pg.loadingTiles.track(c, root)
						root.Children(func() {
							if width <= 0 {
								ui.Box(c).Height(1)
								return
							}
							cols, tile := kit.CoverColumns(width, theme.Current().Metric["tile-min"])
							for i := range cols {
								ui.Column(c.Key(i)).Width(tile).Children(func() {
									ui.Box(c).Size(tile, tile).Radius(sm).Background(hover)
									ui.Box(c).WidthPercent(75).Height(14).Margin(8, 0, 0, 0).Radius(sm).Background(hover)
									ui.Box(c).WidthPercent(50).Height(12).Margin(6, 0, 0, 0).Radius(sm).Background(hover)
								})
							}
						})
					})
				}
			})
		})
	})
}
