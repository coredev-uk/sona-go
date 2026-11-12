package search

import (
	"net/url"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry, from the reference page (AppShell's main, PageHeader)
// in DIPs.
const (
	padTop    = 28 // pt-7
	padBottom = 48 // pb-12
	narrow    = 960
	// libraryPerKind is how many library matches per kind are asked for.
	libraryPerKind = 25
)

// pages is the state of the feature's pages.
type pages struct {
	search  searchView
	group   groupView
	profile profileView
}

// pageView is the view state every search page keeps: the scroll place, the
// size of its scroll container as laid out in the last frame, and the
// field's request to focus the first result.
type pageView struct {
	pos ui.ScrollState
	// width, height are the scroll container's size, zero until a frame
	// laid it out; inner is the width the content has.
	width, height, inner float32
	// outer is the width the page has with its rail, top the scroll
	// container's place in the window; laidY is the scroll offset the last
	// layout was built at and builtY the one this frame is built at: the
	// places the last layout gave are in content terms by laidY.
	outer, top, laidY, builtY float32
	// anchors are the places in the content the rail jumps to, by id;
	// focusID the one that holds the focus after a jump.
	anchors  map[string]float32
	focusID  string
	focusNow bool
	// shown is the identity of what the page lists: another one starts from
	// its top.
	shown string

	wantFocus, claimed bool
}

// columns is how many columns of at least min DIPs, gap apart, fit the
// page's content (auto-fill, minmax(min, 1fr)).
func (pv *pageView) columns(min, gap float32) int {
	if pv.inner <= 0 {
		return 1
	}
	return max(1, int((pv.inner+gap)/(min+gap)))
}

// claim is true once per frame for the first focusable result built, when
// the field asked to move into the results.
func (pv *pageView) claim() bool {
	if pv.wantFocus && !pv.claimed {
		pv.claimed = true
		return true
	}
	return false
}

// railWidth is the result groups' rail, beside the results column from
// narrow up.
const railWidth = 200

// scrollPage builds a page's frame: one scroll container holding the page's
// column (the header scrolls with the content), inside the shell's gutters.
// body builds the page into it and is given the Pane virtual lists are built
// in. identity names what the page lists: a change starts from the top.
func (s *search) scrollPage(c *ui.Context, pv *pageView, identity string, body func(pane *kit.Pane)) {
	s.framePage(c, pv, identity, nil, body)
}

// framePage is scrollPage with a rail: when rail is not nil and the page is
// wide, it builds into a column left of the scroll container, which does
// not scroll with the content.
func (s *search) framePage(c *ui.Context, pv *pageView, identity string, rail func(), body func(pane *kit.Pane)) {
	if pv.shown != identity {
		if pv.shown != "" {
			pv.pos = ui.ScrollState{}
		}
		pv.shown = identity
		clear(pv.anchors)
		pv.focusID = ""
	}
	pv.laidY = pv.builtY
	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	small := pv.outer > 0 && pv.outer < narrow
	if small {
		gutter = tk.Metric["gutter-narrow"]
	}
	showRail := rail != nil && !small
	scrollbar := tk.Metric["scrollbar"]
	left := gutter
	pv.inner = pv.width - left - gutter - scrollbar
	devstate.Scroll(c, &pv.pos) // dev builds: the offset survives a restart
	var sc ui.Element
	outer := ui.Row(c).Grow(1).Basis(0).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch)
	outer.Children(func() {
		if showRail {
			ui.Column(c.Key("rail-slot")).Width(railWidth+gutter).Shrink(0).Padding(padTop, 0, padBottom, gutter).Children(rail)
		}
		sc = ui.Scroll(c).TrackScroll(&pv.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
		sc.Children(func() {
			ui.Column(c).Padding(padTop, gutter+scrollbar, padBottom, left).Children(func() {
				body(&kit.Pane{Pos: &pv.pos, Bounds: sc.Bounds})
			})
		})
	})
	pv.builtY = pv.pos.Y
	b, ob := sc.Bounds(), outer.Bounds()
	if b.W != pv.width || b.H != pv.height || b.Y != pv.top || ob.W != pv.outer {
		pv.width, pv.height, pv.top, pv.outer = b.W, b.H, b.Y, ob.W
		c.Invalidate() // the columns follow the width
	}
}

// anchored marks e as a place the rail jumps to: its place in the page's
// content is kept for the jump, and it takes the keyboard focus after one
// (a TabStop(false), so Tab never stops at it).
func (pv *pageView) anchored(e ui.Element, id, label string) ui.Element {
	if b := e.Bounds(); b.H > 0 {
		// The last layout's place, whether or not it is on screen.
		if pv.anchors == nil {
			pv.anchors = map[string]float32{}
		}
		pv.anchors[id] = b.Y - pv.top + pv.laidY
	}
	if pv.focusID == id {
		// While it has the focus it is a group named label, so that what the
		// focus lands on is read.
		e.TabStop(false).FocusRing(false).Role(ui.RoleGroup).Label(label)
		switch {
		case pv.focusNow:
			e.Focus()
			pv.focusNow = false
		case !e.Focused():
			pv.focusID = ""
		}
	}
	return e
}

// jump scrolls the anchor to the top of the page (the room of the title's
// line above it) and moves the focus to it.
func (pv *pageView) jump(id string) {
	if y, ok := pv.anchors[id]; ok {
		pv.pos.Y = max(0, y-padTop)
	}
	pv.focusID, pv.focusNow = id, true
}

// current is the anchor the view is at: the last whose place is within a
// header's height of the top, of ids in page order.
func (pv *pageView) current(ids []string) string {
	cur := ""
	for _, id := range ids {
		if y, ok := pv.anchors[id]; ok && y <= pv.pos.Y+padTop+24 {
			cur = id
		}
	}
	return cur
}

// grid lays n cells out in cols equal columns (auto-fill: the last row's
// empty cells keep the others' width). It is one list named label, whose
// items are the cells.
func grid(c *ui.Context, key, label string, cols int, gapX, gapY float32, n int, cell func(i int)) {
	ui.Column(c.Key(key)).FillWidth().Gap(gapY).Role(ui.RoleList).Label(label).Children(func() {
		for r := 0; r*cols < n; r++ {
			ui.Row(c.Key(r)).FillWidth().Gap(gapX).AlignItems(ui.Start).Children(func() {
				for i := r * cols; i < (r+1)*cols; i++ {
					col := ui.Column(c.Key(i)).Grow(1).Basis(0).MinWidth(0)
					if i < n {
						col.Role(ui.RoleListItem).Children(func() { cell(i) })
					}
				}
			})
		}
	})
}

// routed reports whether a page the feature links to is ported.
func (s *search) routed(path string) bool { return s.env.HasRoute != nil && s.env.HasRoute(path) }

// itemMenus is the item menu of a list over its selection, made once; nil
// when the shell has no action registry.
func (s *search) itemMenus(m **kit.ActionMenus, sel kit.SelectionScope) *kit.ActionMenus {
	if *m == nil && s.env.Actions != nil {
		*m = kit.NewActionMenus(s.env.Actions, sel)
	}
	return *m
}

// openLink is the path of a page for the track list's artist and album
// cells; nil when the page is not ported.
func (s *search) openLink(prefix string) func(id string) {
	if !s.routed(prefix + "x") {
		return nil
	}
	return func(id string) { s.env.Nav.Push(prefix + url.PathEscape(id)) }
}

// songList is the songs of a result as v0.2.0's TrackList: number, artwork,
// artist and album columns. pending adds loading rows after the songs.
func (s *search) songList(c *ui.Context, st *kit.TrackListState, menus **kit.ActionMenus, pane *kit.Pane,
	ctx kit.ItemsCtx, songs []services.Song, pending int, album bool, label string, onPlay func(i int),
) {
	var current func(*services.Song) bool
	if ctx.Current != nil {
		current = func(s *services.Song) bool { return s != nil && ctx.Current(*s) }
	}
	at := func(i int) *services.Song {
		if i < len(songs) {
			return &songs[i]
		}
		return nil
	}
	kit.TrackList(c, st, len(songs)+pending, at, kit.TrackListOpts{
		Label:      label,
		Columns:    kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: album},
		OnPlay:     onPlay,
		Current:    current,
		Playing:    ctx.Playing,
		Restricted: ctx.Restricted,
		Art:        s.env.Art,
		Menus:      s.itemMenus(menus, &st.Sel),
		Motion:     s.env.Motion,
		Pane:       pane,
		OpenArtist: s.openLink("/artist/"),
		OpenAlbum:  s.openLink("/album/"),
	})
}

// searchView is the search page's state.
type searchView struct {
	pageView
	// rowMenus is the Top results rows' menu; menus the shelves' base menu.
	rowMenus, menus *kit.ActionMenus
	songMenus       *kit.ActionMenus
	libMenus        *kit.ActionMenus
	libSongMenus    *kit.ActionMenus
	songs, libSongs kit.TrackListState
	shelves         map[string]*kit.CoverShelfState
	browse          kit.ItemGridState
	allSongs        bool

	// The last catalog and library results shown, kept on screen while the
	// next ones load (keepPreviousData).
	prev    services.SearchResults
	hasPrev bool
	prevLib services.LibrarySearch
	hasLib  bool
	// announced is the outcome of the search last read out.
	announced outcome
}

func (v *searchView) shelf(key string) *kit.CoverShelfState {
	if v.shelves == nil {
		v.shelves = map[string]*kit.CoverShelfState{}
	}
	if v.shelves[key] == nil {
		v.shelves[key] = &kit.CoverShelfState{}
	}
	return v.shelves[key]
}

// outcome names what a search found: the term and scope searched, and
// whether anything came back.
type outcome struct {
	scope, q string
	none     bool
}

// announce reads out what a search found, once, when its results arrive
// (the results are Busy until then). The text is made only when it is said.
func (v *searchView) announce(c *ui.Context, o outcome, text func() string) {
	if v.announced == o {
		return
	}
	v.announced = o
	c.Announce(text())
}

// resultsFor is what is said when a search found something.
func resultsFor(q string) func() string {
	return func() string { return i18n.T("Results for “{q}”", "q", q) }
}

// searching says the page's next results are loading while the last ones stay
// on screen: a status of no size, read as "Searching" (the results are Busy).
func searching(c *ui.Context) {
	ui.Box(c.Key("searching")).Size(0, 0).Role(ui.RoleStatus).Label(i18n.T("Searching"))
}

// searchPage is v0.2.0's SearchPage: the term as the title with the scope
// switch, then the recent searches and browse categories, or the results
// with the rail of their groups beside them.
func (s *search) searchPage(c *ui.Context, r *ui.Route) {
	q, scope := routeParams(r)
	v := &s.pages.search
	v.wantFocus, v.claimed = s.field.takeResultsFocus(), false
	// What the term found is read before the frame is built: the rail beside
	// the results lists its groups.
	var cat catalogState
	var lib libraryState
	var rail func()
	var entries []railEntry
	loading := false
	switch {
	case q == "":
	case scope == scopeLibrary:
		lib = s.libraryState(q)
		entries, loading = lib.entries(), lib.loading()
	default:
		cat = s.catalogState(q)
		entries, loading = cat.entries(), cat.loading()
	}
	switch {
	case loading:
		rail = func() { railSkeleton(c) }
	case len(entries) > 1:
		rail = func() { s.railList(c, &v.pageView, entries) }
	}
	s.framePage(c, &v.pageView, scope+"\x00"+q, rail, func(pane *kit.Pane) {
		s.queryHeader(c, q, scope)
		if len(entries) > 1 && v.outer > 0 && v.outer < narrow {
			s.chipRow(c, &v.pageView, entries)
		}
		switch {
		case q == "":
			v.announced = outcome{}
			shown := s.recentShelf(c, &v.pageView, scope)
			if scope == scopeLibrary {
				if shown {
					ui.Box(c).Height(40)
				}
				kit.EmptyState(c, i18n.T("Search your library"),
					i18n.T("Songs, albums, artists and playlists synced to this computer, matched as you type each word."), kit.EmptyOpts{})
			} else if _, known := s.authStatus(); known {
				s.browseShelf(c, pane, !shown)
			}
		case scope == scopeLibrary:
			s.libraryResults(c, pane, q, lib)
		default:
			s.catalogResults(c, pane, q, cat)
		}
	})
}

// opened is what opening a result remembers: the search that found it and
// the result.
func (s *search) opened(q, scope string) func(services.Item) {
	return func(it services.Item) {
		s.rememberTerm(q, scope)
		s.rememberItem(it, itemSource(it))
	}
}

// browseShelf is v0.2.0's Browse: the categories of the empty search page.
func (s *search) browseShelf(c *ui.Context, pane *kit.Pane, first bool) {
	v := &s.pages.search
	title := i18n.T("Browse categories")
	res := s.browse()
	kit.Shelf(c, kit.ShelfOpts{Title: title, First: first}, func() {
		switch {
		case res.Err != nil && !res.HasData:
			kit.ErrorBlock(c, i18n.T("Couldn't load the categories"), res.Err, kit.ErrorBlockOpts{
				OnRetry:  func() { s.env.Queries.Invalidate(keyBrowse) },
				Retrying: res.Fetching,
			})
		case !res.HasData:
			cols := v.columns(160, 24)
			tile := (v.inner - 24*float32(cols-1)) / float32(cols)
			if v.inner <= 0 {
				tile = 160
			}
			ui.Column(c.Key("browse-loading")).FillWidth().Gap(24).Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
				for r := range 12 / cols {
					ui.Row(c.Key(r)).Gap(24).Children(func() {
						for i := range cols {
							kit.Skeleton(c.Key(i), tile, tile)
						}
					})
				}
			})
		default:
			items := res.Data
			ctx := s.env.ItemsCtx(nil)
			ctx.OnOpen = func(it services.Item) { s.rememberItem(it, itemSource(it)) }
			kit.ItemGrid(c, &v.browse, len(items), func(i int) *services.Item { return &items[i] }, ctx,
				kit.ItemGridOpts{Label: title, Pane: pane})
			if len(items) > 0 && v.claim() {
				v.browse.Nav().Want(0)
			}
		}
	})
}

// catalogState is what the Apple Music search of a term has so far: the
// results of the last term stay while this one loads.
type catalogState struct {
	res  store.Result[services.SearchResults]
	data services.SearchResults
	show bool
}

func (s *search) catalogState(q string) catalogState {
	v := &s.pages.search
	res := s.catalog(q)
	if res.HasData {
		v.prev, v.hasPrev = res.Data, true
	}
	cs := catalogState{res: res, data: res.Data, show: res.HasData}
	if !cs.show && res.Err == nil && v.hasPrev {
		cs.data, cs.show = v.prev, true
	}
	return cs
}

// loading: nothing to show yet.
func (cs catalogState) loading() bool { return !cs.show && cs.res.Err == nil }

// entries are the groups found, for the rail; none before there are results.
func (cs catalogState) entries() []railEntry {
	if !cs.show || (cs.res.Err != nil && !cs.res.HasData) {
		return nil
	}
	out := make([]railEntry, len(cs.data.Sections))
	for i, sec := range cs.data.Sections {
		out[i] = railEntry{id: groupID(sec.Group), title: groupTitle(sec.Group), count: len(sec.Items)}
	}
	return out
}

// catalogResults is v0.2.0's CatalogResults.
func (s *search) catalogResults(c *ui.Context, pane *kit.Pane, q string, cs catalogState) {
	v := &s.pages.search
	res, data := cs.res, cs.data
	switch {
	case cs.loading():
		s.resultsSkeleton(c)
		return
	case res.Err != nil && !res.HasData:
		code := services.CodeOf(res.Err)
		state, _ := s.authStatus()
		if !account.HasAccount(state) && (code == services.CodeNotFound || code == services.CodeUnauthorized) {
			var action *kit.EmptyAction
			if s.env.Surfaces != nil {
				action = &kit.EmptyAction{Label: i18n.T("Sign in"), Run: s.env.Surfaces.OpenAccount}
			}
			s.noResults(c, i18n.T("Sign in to search Apple Music"),
				i18n.T("Searching the catalogue needs your account's storefront. Your library on this computer can be searched either way."),
				action)
			return
		}
		kit.ErrorBlock(c, i18n.T("Couldn't search Apple Music"), res.Err, kit.ErrorBlockOpts{
			OnRetry:  func() { s.env.Queries.Invalidate(keyCatalogSearch(q)) },
			Retrying: res.Fetching,
		})
		return
	case len(data.Sections) == 0:
		empty := i18n.T("Nothing on Apple Music for “{q}”", "q", q)
		if res.HasData {
			v.announce(c, outcome{scopeCatalog, q, true}, func() string { return empty })
		} else {
			searching(c)
		}
		s.noResults(c, empty,
			i18n.T("Check the spelling, try fewer words, or search your library instead."),
			&kit.EmptyAction{Label: i18n.T("Search your library"), Run: func() { s.env.Nav.Replace(searchLocation(q, scopeLibrary)) }})
		return
	}

	if res.HasData {
		v.announce(c, outcome{scopeCatalog, q, false}, resultsFor(q))
	} else {
		searching(c) // the last term's results stay on screen meanwhile
	}
	ctx := s.env.ItemsCtx(s.itemMenus(&v.menus, nil))
	ctx.OnOpen = s.opened(q, scopeCatalog)
	ui.Column(c.Key("results")).FillWidth().Busy(!res.HasData).Children(func() {
		for i, sec := range data.Sections {
			s.catalogSection(c, pane, ctx, q, sec, i == 0)
		}
	})
}

// catalogSection is v0.2.0's CatalogSection: the Top results as the best
// match and rows, Songs as a track list, every other group as an item shelf.
func (s *search) catalogSection(c *ui.Context, pane *kit.Pane, ctx kit.ItemsCtx, q string, sec services.SearchSection, first bool) {
	v := &s.pages.search
	title := groupTitle(sec.Group)
	more := ""
	if seeAllWanted(sec) {
		more = groupLocation(sec.Group, q)
	}
	switch sec.Group {
	case "top":
		items := sec.Items[:min(len(sec.Items), topRows)]
		s.groupBlock(c, &v.pageView, sec.Group, title, first, func() {
			kit.Shelf(c, kit.ShelfOpts{Title: title, First: true}, func() {
				if len(items) > 0 {
					s.topResults(c, ctx, q, items)
				}
			})
		})
	case "song":
		var songs []services.Song
		var from []services.Item
		for _, it := range sec.Items {
			if it.Song != nil && len(songs) < songRows {
				songs = append(songs, *it.Song)
				from = append(from, it)
			}
		}
		s.groupBlock(c, &v.pageView, sec.Group, title, first, func() {
			kit.Shelf(c, kit.ShelfOpts{Title: title, First: true, More: s.seeAll(more)}, func() {
				s.songList(c, &v.songs, &v.songMenus, pane, ctx, songs, 0, v.inner >= songAlbumFrom, i18n.T("Songs matching {q}", "q", q), func(i int) {
					s.opened(q, scopeCatalog)(from[i])
					s.env.Queue.PlaySongs(songs, i, searchFrom)
				})
				if v.claim() {
					v.songs.Nav().Want(0)
				}
			})
		})
	default:
		st := v.shelf(sec.Group)
		s.itemBlock(c, &v.pageView, st, sec.Group, title, sec.Items, ctx, more, first)
		if len(sec.Items) > 0 && v.claim() {
			st.Row.Nav().Want(0)
		}
	}
}

// seeAll is a shelf's "See all" action to a page, nil for none.
func (s *search) seeAll(path string) func() {
	if path == "" {
		return nil
	}
	return func() { s.env.Nav.Push(path) }
}

// libraryState is what the library search of a term has so far.
type libraryState struct {
	res services.LibrarySearch
	err error
	ok  bool
	// fetched: the answer is this term's, not the last one's.
	fetched, fetching bool
}

func (s *search) libraryState(q string) libraryState {
	v := &s.pages.search
	res := s.librarySearch(q, libraryPerKind)
	if res.HasData {
		v.prevLib, v.hasLib = res.Data, true
	}
	ls := libraryState{res: res.Data, err: res.Err, ok: res.HasData, fetched: res.HasData, fetching: res.Fetching}
	if res.Err != nil && !res.HasData {
		return ls
	}
	if !ls.ok && v.hasLib {
		ls.res, ls.ok = v.prevLib, true
	}
	return ls
}

func (ls libraryState) failed() bool  { return ls.err != nil && !ls.fetched }
func (ls libraryState) loading() bool { return !ls.failed() && !ls.ok }
func (ls libraryState) empty() bool {
	r := ls.res
	return len(r.Songs)+len(r.Albums)+len(r.Artists)+len(r.Playlists) == 0
}

// entries are the groups found, for the rail.
func (ls libraryState) entries() []railEntry {
	if ls.failed() || !ls.ok {
		return nil
	}
	var out []railEntry
	for _, g := range []struct {
		key string
		n   int
	}{{"song", len(ls.res.Songs)}, {"artist", len(ls.res.Artists)}, {"album", len(ls.res.Albums)}, {"playlist", len(ls.res.Playlists)}} {
		if g.n > 0 {
			out = append(out, railEntry{id: groupID("lib:" + g.key), title: groupTitle(g.key), count: g.n})
		}
	}
	return out
}

// libraryResults is v0.2.0's LibraryResults: songs, artists, albums and
// playlists synced to this computer.
func (s *search) libraryResults(c *ui.Context, pane *kit.Pane, q string, ls libraryState) {
	v := &s.pages.search
	switch {
	case ls.failed():
		kit.ErrorBlock(c, i18n.T("Couldn't search your library"), ls.err, kit.ErrorBlockOpts{
			OnRetry: func() { s.env.Queries.Invalidate(keyLibrarySearch(q, libraryPerKind)) },
		})
		return
	case ls.loading():
		s.resultsSkeleton(c)
		return
	case ls.empty():
		empty := i18n.T("Nothing in your library for “{q}”", "q", q)
		if ls.fetched {
			v.announce(c, outcome{scopeLibrary, q, true}, func() string { return empty })
		} else {
			searching(c)
		}
		s.noResults(c, empty,
			i18n.T("Only what is synced to this computer is searched. Try Apple Music for everything else."),
			&kit.EmptyAction{Label: i18n.T("Search Apple Music"), Run: func() { s.env.Nav.Replace(searchLocation(q, scopeCatalog)) }})
		return
	}
	if ls.fetched {
		v.announce(c, outcome{scopeLibrary, q, false}, resultsFor(q))
	} else {
		searching(c) // the last term's results stay on screen meanwhile
	}
	artists, albums, playlists, _ := libraryCards(ls.res)
	ctx := s.env.ItemsCtx(s.itemMenus(&v.libMenus, nil))
	ctx.OnOpen = s.opened(q, scopeLibrary)
	ui.Column(c.Key("results")).FillWidth().Busy(!ls.fetched).Children(func() {
		s.libraryShelves(c, pane, ctx, q, ls.res, artists, albums, playlists)
	})
}

// libraryShelves are the library results' shelves: songs, then the artists,
// albums and playlists that were found.
func (s *search) libraryShelves(c *ui.Context, pane *kit.Pane, ctx kit.ItemsCtx, q string, r services.LibrarySearch, artists, albums, playlists []services.Item) {
	v := &s.pages.search
	first := true
	if len(r.Songs) > 0 {
		shown := r.Songs
		if !v.allSongs {
			shown = shown[:min(len(shown), songRows)]
		}
		count := len(r.Songs)
		s.groupBlock(c, &v.pageView, "lib:song", groupTitle("song"), true, func() {
			kit.Shelf(c, kit.ShelfOpts{Title: groupTitle("song"), First: true, Actions: func() {
				if count <= songRows {
					return
				}
				label := i18n.T("Show all {songCount}", "songCount", count)
				if v.allSongs {
					label = i18n.T("Show fewer")
				}
				if kit.ShelfToggle(c, label, v.allSongs).Clicked() {
					v.allSongs = !v.allSongs
				}
			}}, func() {
				s.songList(c, &v.libSongs, &v.libSongMenus, pane, ctx, shown, 0, v.inner >= songAlbumFrom, i18n.T("Library songs matching {q}", "q", q), func(i int) {
					s.rememberTerm(q, scopeLibrary)
					s.env.Queue.PlaySongs(r.Songs, i, searchFrom)
				})
				if v.claim() {
					v.libSongs.Nav().Want(0)
				}
			})
		})
		first = false
	}
	for _, sec := range []struct {
		key   string
		items []services.Item
	}{{"artist", artists}, {"album", albums}, {"playlist", playlists}} {
		if len(sec.items) == 0 {
			continue
		}
		st := v.shelf("lib:" + sec.key)
		s.itemBlock(c, &v.pageView, st, "lib:"+sec.key, groupTitle(sec.key), sec.items, ctx, "", first)
		if v.claim() {
			st.Row.Nav().Want(0)
		}
		first = false
	}
}
