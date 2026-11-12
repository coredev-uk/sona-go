package radio

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// radioPage is the Radio page's state: its scroll container, the one item
// menu its lists share, and what each list keeps (selection, "show all").
type radioPage struct {
	frame
	menus *kit.ActionMenus
	board board
	// loading is the width the loading frames' columns follow.
	loadingBoard, loadingStations, loadingTiles widthOf

	live kit.EditorialState
	// stations is the state of each local broadcasters list, by section.
	stations map[string]*kit.StationListState
	bands    map[bandID]*kit.EditorialState
	yours    kit.CoverShelfState
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
