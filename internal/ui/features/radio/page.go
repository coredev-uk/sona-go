package radio

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// radioPage is the Radio page's state: its scroll container, the one item
// menu its lists share, and what each list keeps (selection, "show all").
type radioPage struct {
	frame
	menus *kit.ActionMenus
	board board
	// loading is the width the loading frames' columns follow.
	loadingBoard, loadingStations, loadingTiles widthOf

	live  kit.EditorialState
	bands map[bandID]*kit.EditorialState
	yours kit.CoverShelfState
	// yoursMenus is the menu of "Your stations": its own selection.
	yoursMenus *kit.ActionMenus
}

// bandState is the state of a band's sections.
func (pg *radioPage) bandState(id bandID) *kit.EditorialState {
	if pg.bands == nil {
		pg.bands = map[bandID]*kit.EditorialState{}
	}
	if pg.bands[id] == nil {
		pg.bands[id] = &kit.EditorialState{}
	}
	return pg.bands[id]
}

// stationCover is a station as a cover tile (v0.2.0's RadioPage stationCover):
// its page and a play control, or, for a station with no page (a local
// broadcaster), the tile itself plays; its menu either way.
func (r *radio) stationCover(s services.Station) kit.CoverItem {
	item := kit.StationItem(s)
	sub := s.Tagline
	if sub == "" {
		sub = s.ShowName
	}
	if sub == "" {
		sub = s.Provider
	}
	tile := kit.CoverItem{Key: s.ID, Title: s.Name, Subtitle: sub, Art: s.Art.Medium, Motion: s.Motion, Item: &item}
	play := func() { r.playStation(s.ID, nil) }
	switch {
	case s.HasPage:
		tile.Open = func() { r.env.Nav.Push(stationURL(s.ID)) }
		// The tile names its play control "Play {label}".
		if s.Playable {
			tile.Play = &kit.CoverPlay{Label: s.Name, Run: play}
		}
	case s.Playable:
		tile.Open, tile.PlaysOnOpen = play, true
	}
	return tile
}

// allLive is true for a section Apple fills with its live stations, which the
// board above already lists.
func allLive(s services.Section) bool {
	if len(s.Items) == 0 {
		return false
	}
	for _, it := range s.Items {
		if it.Station == nil || it.Station.Kind != services.StationLive {
			return false
		}
	}
	return true
}

// failed is a query that has nothing to show and an error (TanStack's
// isError); pending one with neither.
func failed[T any](res store.Result[T]) bool  { return res.Err != nil && !res.HasData }
func pending[T any](res store.Result[T]) bool { return res.Err == nil && !res.HasData }

// radioView is RadioPage: what's on air now first (Apple's live stations as a
// now-and-next board, then the local broadcasters), then Apple's radio page in
// bands: stations to tune to (the user's own first), shows and episodes, and
// the ways to browse further.
func (r *radio) radioView(c *ui.Context, _ *ui.Route) {
	pg := &r.radioPg
	state := r.accountState()
	signedIn := account.HasAccount(state)
	live := r.live()
	grouping := r.grouping()
	var recent store.Result[[]services.Station]
	if signedIn {
		recent = r.recent()
	}
	if pg.menus == nil {
		pg.menus = kit.NewActionMenus(r.env.Actions, nil)
	}
	if pg.yoursMenus == nil {
		pg.yoursMenus = kit.NewActionMenus(r.env.Actions, &pg.yours.Row.Sel)
	}
	// The page's lists share one menu, whose popup is drawn once, last.
	ctx := r.env.ItemsCtx(pg.menus)
	ctx.NoPopup = true

	// Apple's page repeats the live stations (a featured row and a shelf);
	// the board already shows them, unless Sona couldn't list them.
	boardShown := live.HasData && len(live.Data) > 0
	var sections []services.Section
	for _, s := range grouping.Data.Sections {
		if !boardShown || !allLive(s) {
			sections = append(sections, s)
		}
	}
	liveSections, bands := placeSections(sections)
	retry := func() { r.env.Queries.Invalidate(keyLive, keyGrouping) }
	// "Your stations": the user's own, else the way to see them.
	hasYours := (signedIn && recent.HasData && len(recent.Data) > 0) || isSignedOut(state)
	yours := func() { r.yours(c, state, recent) }

	devstate.Scroll(c, &pg.pos) // dev builds: the offset survives a restart
	pg.scroll(c, func(gutter float32) {
		kit.PageHeader(c, i18n.T("Radio"), "", nil)
		if failed(live) && failed(grouping) {
			// Nothing on the page loaded: one block says so, not one per part.
			kit.ErrorBlock(c, i18n.T("Couldn't load Apple Music's radio"), grouping.Err, kit.ErrorBlockOpts{
				OnRetry: retry, Retrying: live.Fetching || grouping.Fetching,
			})
			return
		}
		r.liveNow(c, live, liveSections, ctx)
		switch {
		case pending(grouping):
			r.bandsSkeleton(c)
		case failed(grouping):
			ui.Column(c.Key("grouping-error")).Margin(bandGap, 0, 0, 0).Children(func() {
				if hasYours {
					ui.Column(c.Key("yours")).Margin(0, 0, 40, 0).Children(yours)
				}
				kit.ErrorBlock(c, i18n.T("Couldn't load Apple Music's radio page"), grouping.Err, kit.ErrorBlockOpts{
					OnRetry: func() { r.env.Queries.Invalidate(keyGrouping) }, Retrying: grouping.Fetching,
				})
			})
		default:
			for _, b := range bands {
				lead := b.id == bandStations && hasYours
				if !lead && len(b.sections) == 0 {
					continue
				}
				r.band(c, b, lead, yours, ctx)
			}
		}
		pg.menus.Popup(c)
	})
}

// liveNow is what's on air: Apple Music's live stations as the board, then the
// live shelves of Apple's page (the local broadcasters). The section's heading
// is at the bands' size, as the first of them.
func (r *radio) liveNow(c *ui.Context, live store.Result[[]services.Station], sections []services.Section, ctx kit.ItemsCtx) {
	pg := &r.radioPg
	title := i18n.T("Live now")
	ui.Column(c.Key("live-now")).Role(ui.RoleGroup).Label(title).Children(func() {
		text(c, title, "page").TextColor(theme.C("ink")).Margin(0, 0, headingGap, 0).Role(ui.RoleHeading).Level(2)
		switch {
		case pending(live):
			r.onAirSkeleton(c, &pg.loadingBoard)
		case failed(live):
			kit.ErrorBlock(c, i18n.T("Couldn't load the live stations"), live.Err, kit.ErrorBlockOpts{
				OnRetry: func() { r.env.Queries.Invalidate(keyLive) }, Retrying: live.Fetching,
			})
		case len(live.Data) == 0:
			kit.EmptyState(c, i18n.T("No live stations"), i18n.T("Apple Music lists no live radio for your storefront right now."), kit.EmptyOpts{})
		default:
			r.onAirBoard(c, &pg.board, live.Data, ctx, pg.menus)
		}
		if len(sections) > 0 {
			ui.Column(c.Key("live-sections")).Margin(40, 0, 0, 0).Children(func() {
				kit.EditorialSections(c, &pg.live, sections, ctx, kit.EditorialOpts{})
			})
		}
	})
}

// band is a band of the page: a line across the column, then its heading at
// the page-title size, then its sections. lead builds what comes first (the
// user's stations).
func (r *radio) band(c *ui.Context, b band, lead bool, yours func(), ctx kit.ItemsCtx) {
	title := bandTitle(b.id)
	ui.Column(c.Key("band-"+string(b.id))).Margin(bandGap, 0, 0, 0).Padding(bandPad, 0, 0, 0).
		Role(ui.RoleGroup).Label(title).Children(func() {
		ui.Box(c.Key("rule")).Absolute().Top(0).Left(0).Right(0).Height(1).Background(theme.C("line"))
		text(c, title, "page").TextColor(theme.C("ink")).Margin(0, 0, headingGap, 0).Role(ui.RoleHeading).Level(2)
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
// broadcasters' heading and rows, then the first band's line and heading and
// its shelves.
func (r *radio) bandsSkeleton(c *ui.Context) {
	pg := &r.radioPg
	hover := theme.C("hover")
	sm := theme.Current().Radius["sm"]
	bar := func(w, h float32) ui.Element {
		return ui.Box(c).Width(w).Height(h).Radius(sm).Background(hover)
	}
	ui.Column(c.Key("bands-skeleton")).Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		ui.Column(c.Key("broadcasters")).Margin(40, 0, 0, 0).Children(func() {
			ui.Row(c).Height(28).Margin(0, 0, 12, 0).AlignItems(ui.Center).Children(func() { bar(160, 16) })
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
		ui.Column(c.Key("first-band")).Margin(bandGap, 0, 0, 0).Padding(bandPad, 0, 0, 0).Children(func() {
			ui.Box(c.Key("rule")).Absolute().Top(0).Left(0).Right(0).Height(1).Background(theme.C("line"))
			ui.Row(c).Height(30).Margin(0, 0, headingGap, 0).AlignItems(ui.Center).Children(func() { bar(112, 20) })
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
