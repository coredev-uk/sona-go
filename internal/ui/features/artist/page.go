package artist

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// narrow is the window width under which the page's gutters shrink.
const narrow = 960

// shelfState is a cover shelf's state with its own item menu: several shelves
// share a page, and a menu over one selection draws its popup once.
type shelfState struct {
	kit.CoverShelfState
	menus *kit.ActionMenus
}

// tracksState is a track list's state with its item menu.
type tracksState struct {
	kit.TrackListState
	menus *kit.ActionMenus
}

// pageState is the artist page's view state: the scroll place, the content
// width of the last frame, and the state of the lists on it.
type pageState struct {
	id    string
	pos   ui.ScrollState
	width float32 // the content's width as the last frame laid it out

	// readMore: "Read more" was pressed; the About band (aboutTop is its
	// place in the scrolled content) scrolls to the top and focusAbout moves
	// the focus onto it.
	readMore, focusAbout bool
	aboutTop             float32

	top, songs tracksState
	head       *kit.ActionMenus
	shelves    map[string]*shelfState
	lists      map[string]*releaseList
}

func (p *pageState) shelf(key string) *shelfState {
	if p.shelves == nil {
		p.shelves = map[string]*shelfState{}
	}
	if p.shelves[key] == nil {
		p.shelves[key] = &shelfState{}
	}
	return p.shelves[key]
}

func (p *pageState) list(key string) *releaseList {
	if p.lists == nil {
		p.lists = map[string]*releaseList{}
	}
	if p.lists[key] == nil {
		p.lists[key] = &releaseList{}
	}
	return p.lists[key]
}

// gutter is the page's side padding, which the header bleeds through.
func gutter(c *ui.Context) float32 {
	tk := theme.Current()
	if w, _ := c.Size(); w < narrow {
		return tk.Metric["gutter-narrow"]
	}
	return tk.Metric["gutter"]
}

// scaffold is the page every artist route builds into: one scroll container
// with the page's padding, whose content width the page follows (v0.2.0's
// useElementWidth on the page root; 0 before the first layout).
func (a *artist) scaffold(c *ui.Context, pos *ui.ScrollState, width *float32, body func(pane *kit.Pane)) {
	g, scrollbar := gutter(c), theme.Current().Metric["scrollbar"]
	devstate.Scroll(c, pos) // dev builds: the offset survives a restart
	sc := ui.Scroll(c).TrackScroll(pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	pane := &kit.Pane{Pos: pos, Bounds: sc.Bounds}
	sc.Children(func() {
		ui.Column(c).Padding(28, g+scrollbar, 48, g).Children(func() {
			page := ui.Column(c.Key("page")).FillWidth()
			page.Draw(func(_ *ui.Painter, r ui.Rect) {
				if r.W != *width {
					*width = r.W
					c.Invalidate() // the layout follows the width
				}
			})
			if w := page.Bounds().W; w > 0 && *width == 0 {
				*width = w
			}
			page.Children(func() { body(pane) })
		})
	})
}

// artistPage is ArtistPage: a library artist (id "r.…") loads
// LibraryService.Artist, a catalogue one CatalogService.Artist.
func (a *artist) artistPage(c *ui.Context, r *ui.Route) {
	id := r.Param("id")
	pg := &a.page
	if pg.id != id {
		*pg = pageState{id: id}
	}
	lib := isLibraryID(id)
	var (
		libQ store.Result[services.ArtistDetail]
		catQ store.Result[services.CatalogArtist]
		key  store.Key
	)
	if lib {
		key = keyLibraryArtist(id)
		libQ = store.Use(a.env.Queries, key, func(ctx context.Context) (services.ArtistDetail, error) {
			return a.lib.Artist(ctx, id)
		}, store.Options{Stale: store.Forever})
	} else {
		key = keyCatalogArtist(id)
		catQ = a.catalog(id)
	}
	hasData, err, fetching := catQ.HasData, catQ.Err, catQ.Fetching
	if lib {
		hasData, err, fetching = libQ.HasData, libQ.Err, libQ.Fetching
	}
	a.scaffold(c, &pg.pos, &pg.width, func(pane *kit.Pane) {
		switch {
		case err != nil && !hasData:
			kit.DetailError(c, kit.NounArtist, err, kit.DetailErrorOpts{
				Retry:    func() { a.env.Queries.Invalidate(key) },
				Retrying: fetching,
				Back:     a.backToArtists(),
				SignIn:   a.signIn(),
			})
		case !hasData:
			a.skeleton(c, pg.width)
		case lib:
			a.libraryArtist(c, pg, &libQ.Data, pane)
		default:
			a.catalogArtist(c, pg, &catQ.Data, pane)
		}
	})
}

// catalog is a catalogue artist page, loaded.
func (a *artist) catalog(id string) store.Result[services.CatalogArtist] {
	return store.Use(a.env.Queries, keyCatalogArtist(id), func(ctx context.Context) (services.CatalogArtist, error) {
		return a.cat.Artist(ctx, id)
	}, store.Options{Stale: store.Forever})
}

// backToArtists is the way back from a missing artist: the library's Artists
// page, when the app has one.
func (a *artist) backToArtists() kit.EmptyAction {
	back := kit.EmptyAction{Label: i18n.T("Go to Artists")}
	if a.has(artistsPath) {
		back.Run = func() { a.env.Nav.Push(artistsPath) }
	}
	return back
}

// from is the collection an artist's songs play from.
func from(id string) services.PlayFrom { return services.PlayFrom{Kind: "artist", ID: id} }

func anyPlayable(songs []services.Song) bool {
	for i := range songs {
		if songs[i].Playable {
			return true
		}
	}
	return false
}

// catalogArtist is CatalogArtistPage.
func (a *artist) catalogArtist(c *ui.Context, pg *pageState, page *services.CatalogArtist, pane *kit.Pane) {
	art := page.Artist
	lead := bioLead(page.Bio)
	releases, known := releaseCount(page)
	if !known {
		releases = 0
	}
	genre := art.Genre
	if len(page.Genres) > 0 {
		genre = page.Genres[0]
	}
	h := headProps{
		artist: art, header: page.Header,
		line: artistLine(genre, page.Origin, releases, ""),
		lead: lead,
		// An artist with one release has it in the discography; the header
		// doesn't repeat it.
		latest: page.LatestRelease,
		width:  pg.width,
	}
	if releases == 1 {
		h.latest = nil
	}
	h.actions = func() {
		if len(page.TopSongs) > 0 {
			play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{
				PlayLabel: i18n.T("Play {artistName}'s top songs", "artistName", art.Name),
				Shuffle:   true,
				Disabled:  !anyPlayable(page.TopSongs),
			})
			if play {
				a.env.Queue.PlaySongs(page.TopSongs, 0, from(art.ID))
			}
			if shuffle {
				a.env.Queue.ShuffleSongs(page.TopSongs, from(art.ID))
			}
		}
		a.headerActions(c, pg, page.Artist)
	}
	a.head(c, pg, h)
	a.catalogBody(c, pg, page, pane, false)
}

// headerActions are the rating buttons and the header's item menu of an
// artist the catalogue knows.
func (a *artist) headerActions(c *ui.Context, pg *pageState, art services.Artist) {
	a.env.ItemStates.RatingButtons(c, kit.StateArtists, kit.StateItem{ID: art.ID, Source: art.Source, CatalogID: art.CatalogID, LibraryID: art.LibraryID}, art.Name)
	if pg.head == nil {
		pg.head = kit.NewActionMenus(a.env.Actions, nil)
	}
	pg.head.Trigger(c.Key("more"), kit.ActionTarget{Item: kit.ArtistItem(art), Name: art.Name}, kit.TriggerOpts{Header: true})
	pg.head.Popup(c)
}

// libraryArtist is LibraryArtistPage: the albums and songs in the library
// first, then (for artists Apple Music knows) the same lead and bands as the
// catalogue page.
func (a *artist) libraryArtist(c *ui.Context, pg *pageState, d *services.ArtistDetail, pane *kit.Pane) {
	art := d.Artist
	var page *services.CatalogArtist
	if art.CatalogID != "" {
		if r := a.catalog(art.CatalogID); r.HasData {
			page = &r.Data
		}
	}
	var full, singles []services.Album
	for _, al := range d.Albums {
		if al.Single {
			singles = append(singles, al)
		} else {
			full = append(full, al)
		}
	}
	// translators: {0} is the number of the artist's albums and {1} the number of their songs.
	libraryLine := i18n.T("{0, plural, one {# album} other {# albums}}, {1, plural, one {# song} other {# songs}} in your library",
		"0", len(d.Albums), "1", len(d.Songs))
	genre, lead := "", ""
	h := headProps{artist: art, header: d.Header, width: pg.width}
	if page != nil {
		if len(page.Genres) > 0 {
			genre = page.Genres[0]
		}
		lead = bioLead(page.Bio)
		h.latest = page.LatestRelease
	}
	h.line, h.lead = artistLine(genre, "", 0, libraryLine), lead
	h.actions = func() {
		if len(d.Songs) > 0 {
			play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: art.Name, Shuffle: true, Disabled: !anyPlayable(d.Songs)})
			if play {
				a.env.Queue.PlaySongs(d.Songs, 0, from(art.ID))
			}
			if shuffle {
				a.env.Queue.ShuffleSongs(d.Songs, from(art.ID))
			}
		}
		if page != nil {
			a.headerActions(c, pg, page.Artist)
		}
	}
	a.head(c, pg, h)

	a.itemShelf(c, pg, "library:albums", i18n.T("Albums in your library"), a.albumTiles(full, "year"), false, nil, false)
	a.itemShelf(c, pg, "library:singles", i18n.T("Singles and EPs in your library"), a.albumTiles(singles, "year"), false, nil, false)
	if len(d.Songs) > 0 {
		kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Songs in your library")}, func() {
			a.tracks(c, &pg.songs, d.Songs, len(d.Songs), i18n.T("Songs by {artistName} in your library", "artistName", art.Name), from(art.ID), pane)
		})
	}
	if page != nil {
		ui.Column(c.Key("catalogue")).Margin(56, 0, 0, 0).Children(func() {
			a.catalogBody(c, pg, page, pane, true)
		})
	}
}

// tracks is a TrackList of the artist's first shown songs: row numbers,
// artwork and album columns; Enter or a double-click plays from that row
// with all the songs as the queue, as an artist collection.
func (a *artist) tracks(c *ui.Context, st *tracksState, songs []services.Song, shown int, label string, src services.PlayFrom, pane *kit.Pane) {
	if st.menus == nil {
		st.menus = kit.NewActionMenus(a.env.Actions, &st.Sel)
	}
	cur := a.env.Play.State()
	o := kit.TrackListOpts{
		Label:      label,
		Columns:    kit.TrackColumns{RowNumbers: true, Artwork: true, Album: true},
		OnPlay:     func(i int) { a.env.Queue.PlaySongs(songs, i, src) },
		Current:    cur.Current,
		Playing:    cur.State == "playing",
		Restricted: a.restricted(),
		Art:        a.env.Art, Menus: st.menus, Motion: a.env.Motion, Pane: pane,
	}
	if a.has(albumPath("_")) {
		o.OpenAlbum = func(id string) { a.env.Nav.Push(albumPath(id)) }
	}
	kit.TrackList(c, &st.TrackListState, min(shown, len(songs)), func(i int) *services.Song { return &songs[i] }, o)
}

// band is a band of the page: a 1 DIP line across the column and its heading
// at the page-title size, then its shelves (docs/design.md, Artist).
// It takes the focus for a jump (Read more) without being a Tab stop.
func (a *artist) band(c *ui.Context, id, title string, content func()) ui.Element {
	sec := ui.Column(c.Key(id)).Margin(56, 0, 0, 0).Role(ui.RoleGroup).Label(title).TabStop(false)
	sec.Children(func() {
		ui.Box(c.Key("line")).FillWidth().Height(1).Background(theme.C("line"))
		theme.StyleText(ui.Text(c, title), "page").
			TextColor(theme.C("ink")).Margin(24, 0, 20, 0).Role(ui.RoleHeading).Level(2)
		ui.Column(c.Key("content")).Children(content)
	})
	return sec
}

// catalogBody is everything Apple Music has on the artist below the header:
// the lead (top songs, with the essential albums beside them), then the
// bands: Discography (albums as covers, singles, live albums and
// compilations as the back catalogue's lists, music videos), Related
// (appears on, playlists, similar artists) and About.
func (a *artist) catalogBody(c *ui.Context, pg *pageState, page *services.CatalogArtist, pane *kit.Pane, embedded bool) {
	art := page.Artist
	wide := layoutFor(pg.width).wide
	top := len(page.TopSongs) > 0
	ess, hasEss := essentials(page)
	essAlbums := ess.Albums[:min(len(ess.Albums), essentialRows)]
	essential := hasEss && len(essAlbums) > 0

	topShelf := func() {
		title := i18n.T("Top songs")
		so := kit.ShelfOpts{Title: title, First: true}
		if page.TopSongsMore || len(page.TopSongs) > topRows {
			so.More = a.seeAll(art.ID, services.ViewTopSongs)
		}
		kit.Shelf(c, so, func() {
			a.tracks(c, &pg.top, page.TopSongs, topRows,
				i18n.T("Top songs by {artistName}", "artistName", art.Name), from(art.ID), pane)
		})
	}
	essentialShelf := func(first bool) {
		title, _ := viewTitle(services.ViewFeaturedAlbums, art.Name)
		so := kit.ShelfOpts{Title: title, First: first}
		if ess.More || len(ess.Albums) > essentialRows {
			so.More = a.seeAll(art.ID, services.ViewFeaturedAlbums)
		}
		cols := 1
		if !wide && top {
			cols = 2
		}
		kit.Shelf(c, so, func() {
			a.releaseList(c, pg.list("essential"), albumPtrs(essAlbums), releaseOpts{label: title, columns: cols})
		})
	}
	switch {
	case top && essential && wide:
		ui.Row(c.Key("lead")).FillWidth().Gap(40).AlignItems(ui.Start).Children(func() {
			ui.Column(c.Key("top")).Grow(1).Basis(0).MinWidth(0).Children(topShelf)
			ui.Column(c.Key("essential")).Width(300).Shrink(0).Children(func() { essentialShelf(true) })
		})
	case top || essential:
		ui.Column(c.Key("lead")).FillWidth().Children(func() {
			if top {
				topShelf()
			}
			if essential {
				essentialShelf(!top)
			}
		})
	}

	// The artist's concerts (features/concerts' ArtistConcerts) follow the
	// lead. They are never first on a catalogue artist's page (its header
	// precedes them); on a library artist's page the body is a block of its
	// own, so they are first when the lead is empty.
	if a.env.ArtistConcerts != nil {
		a.env.ArtistConcerts(c, art.ID, embedded && !top && !essential)
	}

	albums := shelvesFor(page, viewsAlbums)
	catalogue := shelvesFor(page, viewsCatalogue)
	videos, hasVideos := videoShelf(page)
	appears := shelvesFor(page, viewsAppears)
	related := shelvesFor(page, viewsRelated)
	// One column width for every catalogue grid on the page, so they line up.
	cols := catalogueColumns(pg.width, len(viewsCatalogue))

	if len(albums) > 0 || len(catalogue) > 0 || hasVideos {
		a.band(c, "artist-discography", i18n.T("Discography"), func() {
			first := true
			for _, sh := range albums {
				a.viewShelf(c, pg, sh, art, first)
				first = false
			}
			if a.catalogueLists(c, pg, catalogue, art, cols, false, first) {
				first = false
			}
			if hasVideos {
				a.viewShelf(c, pg, videos, art, first)
			}
		})
	}
	if len(appears) > 0 || len(related) > 0 {
		a.band(c, "artist-related", i18n.T("Related"), func() {
			first := !a.catalogueLists(c, pg, appears, art, cols, true, true)
			for _, sh := range related {
				a.viewShelf(c, pg, sh, art, first)
				first = false
			}
		})
	}
	a.about(c, pg, page, pane)
}

// seeAll is a shelf's "See all": its page, when the app has one.
func (a *artist) seeAll(id, view string) func() {
	path := viewPath(id, view)
	if !a.has(path) {
		return nil
	}
	return func() { a.env.Nav.Push(path) }
}

func albumPtrs(albums []services.Album) []*services.Album {
	out := make([]*services.Album, len(albums))
	for i := range albums {
		out[i] = &albums[i]
	}
	return out
}

// catalogueLists sets album shelves as the back catalogue's lists, side by
// side in cols columns, each with its title and "See all": the lists fill
// rows of cols slots, so a page's grids share one column width. It builds
// nothing, and says so, when every shelf is empty.
func (a *artist) catalogueLists(c *ui.Context, pg *pageState, shelves []services.ArtistShelf, art services.Artist, cols int, byArtist, first bool) bool {
	var lists []services.ArtistShelf
	for _, sh := range shelves {
		if len(sh.Albums) > 0 && isArtistView(sh.View) {
			lists = append(lists, sh)
		}
	}
	if len(lists) == 0 {
		return false
	}
	ui.Column(c.Key("lists:" + lists[0].View)).FillWidth().Children(func() {
		for row := 0; row*cols < len(lists); row++ {
			r := ui.Row(c.Key(row)).FillWidth().Gap(columnGap).AlignItems(ui.Start)
			if row > 0 || !first {
				r.Margin(40, 0, 0, 0)
			}
			r.Children(func() {
				for slot := range cols {
					i := row*cols + slot
					if i >= len(lists) {
						ui.Box(c.Key("empty-" + string(rune('a'+slot)))).Grow(1).Basis(0)
						continue
					}
					sh := lists[i]
					title, _ := viewTitle(sh.View, art.Name)
					so := kit.ShelfOpts{Title: title, First: true}
					if sh.More || len(sh.Albums) > catalogueRows {
						so.More = a.seeAll(art.ID, sh.View)
					}
					ui.Column(c.Key(sh.View)).Grow(1).Basis(0).MinWidth(0).Children(func() {
						kit.Shelf(c, so, func() {
							a.releaseList(c, pg.list(sh.View), albumPtrs(sh.Albums[:min(len(sh.Albums), catalogueRows)]), releaseOpts{label: title, byArtist: byArtist})
						})
					})
				}
			})
		}
	})
	return true
}

// viewShelf is a cover shelf for one of the artist's views; a shelf with a
// view Sona does not show is skipped.
func (a *artist) viewShelf(c *ui.Context, pg *pageState, sh services.ArtistShelf, art services.Artist, first bool) {
	title, ok := viewTitle(sh.View, art.Name)
	if !ok {
		return
	}
	var more func()
	if sh.More {
		more = a.seeAll(art.ID, sh.View)
	}
	a.itemShelf(c, pg, "shelf:"+sh.View, title, a.shelfTiles(sh), first, more, sh.Kind == services.ShelfVideos)
}

// itemShelf is a titled shelf of cover tiles with its own state and menu.
func (a *artist) itemShelf(c *ui.Context, pg *pageState, key, title string, tiles []kit.CoverItem, first bool, more func(), video bool) {
	if len(tiles) == 0 {
		return
	}
	st := pg.shelf(key)
	if st.menus == nil {
		st.menus = kit.NewActionMenus(a.env.Actions, &st.Row.Sel)
	}
	kit.CoverShelf(c, &st.CoverShelfState, title, tiles, kit.CoverShelfOpts{
		CoverOpts: kit.CoverOpts{
			Art: a.env.Art, Motion: a.env.Motion, Menus: st.menus, Restricted: a.restricted(), Video: video,
		},
		First: first, More: more,
	})
}

// about is the About band, shown only with a biography or facts: the
// biography split on blank lines in a 32rem column, the facts (From, Born or
// formed, Genre) beside it from 900 DIPs.
func (a *artist) about(c *ui.Context, pg *pageState, page *services.CatalogArtist, pane *kit.Pane) {
	type fact struct{ term, value string }
	var facts []fact
	if page.Origin != "" {
		facts = append(facts, fact{i18n.T("From"), page.Origin})
	}
	if page.BornOrFormed != "" {
		facts = append(facts, fact{i18n.T("Born or formed"), page.BornOrFormed})
	}
	if n := len(page.Genres); n > 0 {
		facts = append(facts, fact{i18n.T("{genreCount, plural, one {Genre} other {Genres}}", "genreCount", n), strings.Join(page.Genres, ", ")})
	}
	if page.Bio == "" && len(facts) == 0 {
		return
	}
	tk := theme.Current()
	sec := a.band(c, "artist-about", i18n.T("About {artistName}", "artistName", page.Artist.Name), func() {
		wide := layoutFor(pg.width).wide
		// The grid keeps its bio cell when there is no biography: 32 rem
		// wide beside the facts from 900 DIPs, an empty row above them
		// under that.
		bio := func() {
			cell := ui.Column(c.Key("bio"))
			if wide {
				cell.Width(512).Shrink(1).MinWidth(0)
			} else {
				cell.MaxWidth(512)
			}
			cell.Children(func() {
				if page.Bio == "" {
					return
				}
				for i, p := range paragraphs(page.Bio) {
					t := ui.Text(c.Key(i), p).FontSize(tk.Type["ui"].Size).FixedLineHeight(tk.Type["ui"].LineHeight).TextColor(theme.C("ink"))
					if i > 0 {
						t.Margin(12, 0, 0, 0)
					}
				}
			})
		}
		dl := func() {
			if len(facts) == 0 {
				return
			}
			ui.Column(c.Key("facts")).Gap(12).Grow(1).Basis(0).MinWidth(180).Children(func() {
				for _, f := range facts {
					ui.Column(c.Key(f.term)).Children(func() {
						ui.Text(c, f.term).FontSize(tk.Type["meta"].Size).FixedLineHeight(tk.Type["meta"].LineHeight).TextColor(theme.C("ink-muted"))
						ui.Text(c, f.value).FontSize(tk.Type["ui"].Size).FixedLineHeight(tk.Type["ui"].LineHeight).TextColor(theme.C("ink"))
					})
				}
			})
		}
		var layout ui.Element
		if wide {
			layout = ui.Row(c.Key("about")).FillWidth().Gap(48).AlignItems(ui.Start)
		} else {
			layout = ui.Column(c.Key("about")).FillWidth().Gap(24)
		}
		layout.Children(func() {
			bio()
			dl()
		})
	})
	// Where the band is in the scrolled content, as the last layout put it.
	if b := sec.Bounds(); b.H > 0 {
		pg.aboutTop = b.Y - pane.Bounds().Y + pane.Pos.Y
	}
	if pg.readMore {
		// "Read more": the band goes to the top (below its 24 DIP scroll
		// margin) and takes the focus, so the next Tab is inside it.
		pg.readMore, pg.focusAbout = false, true
		pg.pos.Y = max(0, pg.aboutTop-24)
		c.Invalidate()
	}
	if pg.focusAbout {
		pg.focusAbout = false
		sec.Focus()
	}
}

// signIn opens the account dialog from the "not signed in" error; nil (no
// button) without the shell's surfaces.
func (a *artist) signIn() func() {
	if a.env.Surfaces == nil {
		return nil
	}
	return a.env.Surfaces.OpenAccount
}
