package search

import (
	"net/url"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
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

// scrollPage builds a page's frame: one scroll container holding the page's
// column (the header scrolls with the content), inside the shell's gutters.
// body builds the page into it and is given the Pane virtual lists are built
// in. identity names what the page lists: a change starts from the top.
func (s *search) scrollPage(c *ui.Context, pv *pageView, identity string, body func(pane *kit.Pane)) {
	if pv.shown != identity {
		if pv.shown != "" {
			pv.pos = ui.ScrollState{}
		}
		pv.shown = identity
	}
	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	if pv.width > 0 && pv.width < narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	scrollbar := tk.Metric["scrollbar"]
	pv.inner = pv.width - 2*gutter - scrollbar
	devstate.Scroll(c, &pv.pos) // dev builds: the offset survives a restart
	sc := ui.Scroll(c).TrackScroll(&pv.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		ui.Column(c).Padding(padTop, gutter+scrollbar, padBottom, gutter).Children(func() {
			body(&kit.Pane{Pos: &pv.pos, Bounds: sc.Bounds})
		})
	})
	if b := sc.Bounds(); b.W != pv.width || b.H != pv.height {
		pv.width, pv.height = b.W, b.H
		c.Invalidate() // the columns follow the width
	}
}

// grid lays n cells out in cols equal columns (auto-fill: the last row's
// empty cells keep the others' width).
func grid(c *ui.Context, key string, cols int, gapX, gapY float32, n int, cell func(i int)) {
	ui.Column(c.Key(key)).FillWidth().Gap(gapY).Role(ui.RoleList).Children(func() {
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
	ctx kit.ItemsCtx, songs []services.Song, pending int, label string, onPlay func(i int),
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
		Columns:    kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: true},
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

// loadingRows is v0.2.0's LoadingRows: six rows while the results load.
func loadingRows(c *ui.Context) {
	ui.Column(c.Key("loading")).FillWidth().Gap(12).Role(ui.RoleStatus).Label(i18n.T("Searching")).Children(func() {
		for i := range 6 {
			ui.Row(c.Key(i)).Height(44).AlignItems(ui.Center).Gap(12).PaddingX(8).Children(func() {
				kit.Skeleton(c, 32, 32)
				kit.Skeleton(c, 200, 14)
			})
		}
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

// searchPage is v0.2.0's SearchPage: the term as the title with the scope
// switch, then the recent searches and browse categories, or the results.
func (s *search) searchPage(c *ui.Context, r *ui.Route) {
	q, scope := routeParams(r)
	v := &s.pages.search
	v.wantFocus, v.claimed = s.field.takeResultsFocus(), false
	s.scrollPage(c, &v.pageView, scope+"\x00"+q, func(pane *kit.Pane) {
		title := i18n.T("Search")
		if q != "" {
			title = "“" + q + "”"
		}
		kit.PageHeader(c, title, "", func() { s.scopeSwitch(c, q, scope) })
		switch {
		case q == "":
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
			s.libraryResults(c, pane, q)
		default:
			s.catalogResults(c, pane, q)
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
		}
	})
}

// catalogResults is v0.2.0's CatalogResults.
func (s *search) catalogResults(c *ui.Context, pane *kit.Pane, q string) {
	v := &s.pages.search
	res := s.catalog(q)
	if res.HasData {
		v.prev, v.hasPrev = res.Data, true
	}
	// Results of the last term stay on screen while this one loads.
	data, show := res.Data, res.HasData
	if !show && res.Err == nil && v.hasPrev {
		data, show = v.prev, true
	}
	switch {
	case !show && res.Err == nil:
		loadingRows(c)
		return
	case res.Err != nil && !res.HasData:
		code := services.CodeOf(res.Err)
		state, _ := s.authStatus()
		if !hasAccount(state) && (code == services.CodeNotFound || code == services.CodeUnauthorized) {
			var action *kit.EmptyAction
			if s.env.Surfaces != nil {
				action = &kit.EmptyAction{Label: i18n.T("Sign in"), Run: s.env.Surfaces.OpenAccount}
			}
			kit.EmptyState(c, i18n.T("Sign in to search Apple Music"),
				i18n.T("Searching the catalogue needs your account's storefront. Your library on this computer can be searched either way."),
				kit.EmptyOpts{Action: action})
			return
		}
		kit.ErrorBlock(c, i18n.T("Couldn't search Apple Music"), res.Err, kit.ErrorBlockOpts{
			OnRetry:  func() { s.env.Queries.Invalidate(keyCatalogSearch(q)) },
			Retrying: res.Fetching,
		})
		return
	case len(data.Sections) == 0:
		kit.EmptyState(c, i18n.T("Nothing on Apple Music for “{q}”", "q", q),
			i18n.T("Check the spelling, try fewer words, or search your library instead."), kit.EmptyOpts{})
		return
	}

	ctx := s.env.ItemsCtx(s.itemMenus(&v.menus, nil))
	ctx.OnOpen = s.opened(q, scopeCatalog)
	for i, sec := range data.Sections {
		s.catalogSection(c, pane, ctx, q, sec, i == 0)
	}
}

// catalogSection is v0.2.0's CatalogSection: the Top results as rows, Songs
// as a track list, every other group as an item shelf.
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
		kit.Shelf(c, kit.ShelfOpts{Title: title, First: first}, func() {
			cols := v.columns(280, 24)
			ui.Column(c.Key("top")).FillWidth().Role(ui.RoleList).Label(i18n.T("{title} for {q}", "title", title, "q", q)).Children(func() {
				grid(c, "top-grid", cols, 24, 4, len(items), func(i int) {
					it := items[i]
					s.itemRow(c, it, rowOpts{
						key: strconv.Itoa(i) + ":" + it.Kind + ":" + it.ID, size: 48, onOpen: ctx.OnOpen,
						menus: s.itemMenus(&v.rowMenus, nil), focus: i == 0 && v.claim(),
					})
				})
			})
			if v.rowMenus != nil {
				v.rowMenus.Popup(c)
			}
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
		kit.Shelf(c, kit.ShelfOpts{Title: title, First: first, More: s.seeAll(more)}, func() {
			s.songList(c, &v.songs, &v.songMenus, pane, ctx, songs, 0, i18n.T("Songs matching {q}", "q", q), func(i int) {
				s.opened(q, scopeCatalog)(from[i])
				s.playSongs(songs, i, searchFrom)
			})
			if v.claim() {
				v.songs.Nav().Want(0)
			}
		})
	default:
		kit.ItemShelf(c, v.shelf(sec.Group), title, sec.Items, ctx, kit.ItemShelfOpts{First: first, More: more})
	}
}

// seeAll is a shelf's "See all" action to a page, nil for none.
func (s *search) seeAll(path string) func() {
	if path == "" {
		return nil
	}
	return func() { s.env.Nav.Push(path) }
}

// libraryResults is v0.2.0's LibraryResults: songs, artists, albums and
// playlists synced to this computer.
func (s *search) libraryResults(c *ui.Context, pane *kit.Pane, q string) {
	v := &s.pages.search
	res := s.librarySearch(q, libraryPerKind)
	if res.HasData {
		v.prevLib, v.hasLib = res.Data, true
	}
	if res.Err != nil && !res.HasData {
		kit.ErrorBlock(c, i18n.T("Couldn't search your library"), res.Err, kit.ErrorBlockOpts{
			OnRetry: func() { s.env.Queries.Invalidate(keyLibrarySearch(q, libraryPerKind)) },
		})
		return
	}
	r, ok := res.Data, res.HasData
	if !ok && v.hasLib {
		r, ok = v.prevLib, true
	}
	if !ok {
		loadingRows(c)
		return
	}
	if len(r.Songs) == 0 && len(r.Albums) == 0 && len(r.Artists) == 0 && len(r.Playlists) == 0 {
		kit.EmptyState(c, i18n.T("Nothing in your library for “{q}”", "q", q),
			i18n.T("Only what is synced to this computer is searched. Try Apple Music for everything else."), kit.EmptyOpts{})
		return
	}
	artists, albums, playlists, _ := libraryCards(r)
	ctx := s.env.ItemsCtx(s.itemMenus(&v.libMenus, nil))
	ctx.OnOpen = s.opened(q, scopeLibrary)
	first := true
	if len(r.Songs) > 0 {
		shown := r.Songs
		if !v.allSongs {
			shown = shown[:min(len(shown), songRows)]
		}
		count := len(r.Songs)
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
			s.songList(c, &v.libSongs, &v.libSongMenus, pane, ctx, shown, 0, i18n.T("Library songs matching {q}", "q", q), func(i int) {
				s.rememberTerm(q, scopeLibrary)
				s.playSongs(shown, i, searchFrom)
			})
			if v.claim() {
				v.libSongs.Nav().Want(0)
			}
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
		kit.ItemShelf(c, v.shelf("lib:"+sec.key), groupTitle(sec.key), sec.items, ctx, kit.ItemShelfOpts{First: first})
		first = false
	}
}
