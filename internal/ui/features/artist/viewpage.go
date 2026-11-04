package artist

import (
	"context"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// loadingFirst placeholder rows or tiles show while the first page loads,
	// loadingNext while a further one does.
	loadingFirst = 12
	loadingNext  = 6
	// prefetch: the next page is fetched when the last 10 loaded rows come
	// into view.
	prefetch = 10
)

// viewState is the see-all page's view state.
type viewState struct {
	id, view string
	pos      ui.ScrollState
	width    float32
	// depth is how many pages are asked for: it grows as the list scrolls
	// (v0.2.0's infinite query).
	depth int
	// rangeEnd is the last row the list built, as it reported it.
	rangeEnd int

	tracks tracksState
	grid   kit.ItemGridState
	menus  *kit.ActionMenus
	// songs or items are the loaded pages flattened, rebuilt when a page
	// arrives (loaded counts the pages they hold).
	songs  []services.Song
	items  []services.Item
	loaded int
}

// viewRouteTitle names the page for assistive technology and history.
func (a *artist) viewRouteTitle(r *ui.Route) string {
	view := r.Param("view")
	if !isArtistView(view) {
		return i18n.T("Not found")
	}
	name := i18n.T("this artist")
	if res := a.catalog(r.Param("id")); res.HasData {
		name = res.Data.Artist.Name
	}
	title, _ := viewTitle(view, name)
	return title
}

// viewPages is every loaded page of an artist view, in order, asked for as
// deep as depth. err is the first page's or a further one's failure;
// fetching: a request is in flight; more: Apple has a page after the last
// loaded.
func (a *artist) viewPages(id, view string, depth int) (pages []services.ArtistViewPage, err error, fetching, more bool) {
	offset := 0
	for range max(1, depth) {
		key := append(slices.Clone(keyArtistView(id, view)), offset)
		off := offset
		res := store.Use(a.env.Queries, key, func(ctx context.Context) (services.ArtistViewPage, error) {
			return a.cat.ArtistView(ctx, id, view, off, viewPage)
		}, store.Options{Stale: store.Forever})
		if !res.HasData {
			return pages, res.Err, res.Fetching || res.Loading, false
		}
		pages = append(pages, res.Data)
		if res.Data.Next < 0 {
			return pages, nil, false, false
		}
		offset, more = res.Data.Next, true
	}
	return pages, nil, false, more
}

// pageItems are a fetched page's tiles as items.
func pageItems(p services.ArtistViewPage) []services.Item {
	var items []services.Item
	switch p.Kind {
	case services.ShelfAlbums:
		by := "year"
		if p.View == services.ViewAppearsOn {
			by = "artist"
		}
		for _, al := range p.Albums {
			items = append(items, albumItem(al, by))
		}
	case services.ShelfPlaylists:
		for _, pl := range p.Playlists {
			items = append(items, playlistItem(pl))
		}
	case services.ShelfVideos:
		for _, v := range p.Videos {
			items = append(items, videoItem(v, "year"))
		}
	case services.ShelfArtists:
		for _, ar := range p.Artists {
			items = append(items, artistItem(ar))
		}
	}
	return items
}

// viewPage is ArtistViewPage: an artist shelf in full ("See all"): songs as a
// track list, the rest as a cover grid, fetched page by page as it scrolls.
func (a *artist) viewPage(c *ui.Context, r *ui.Route) {
	id, view := r.Param("id"), r.Param("view")
	vs := &a.view
	if vs.id != id || vs.view != view {
		*vs = viewState{id: id, view: view}
	}
	known := isArtistView(view)
	name := ""
	if known {
		if res := a.catalog(id); res.HasData {
			name = res.Data.Artist.Name
		}
	}
	title := i18n.T("Not found")
	if known {
		n := name
		if n == "" {
			n = i18n.T("this artist")
		}
		title, _ = viewTitle(view, n)
	}
	a.scaffold(c, &vs.pos, &vs.width, func(pane *kit.Pane) {
		a.viewHeader(c, id, name, title)
		if !known {
			kit.EmptyState(c, i18n.T("There's no such list"),
				i18n.T("This artist page doesn't have a list called “{view}”.", "view", view),
				kit.EmptyOpts{Action: new(a.backToArtists())})
			return
		}
		a.viewBody(c, vs, id, view, name, title, pane)
	})
}

// viewHeader is the page's header: the view's title with a subline linking
// to the artist (the name, or "Artist" until it loads).
func (a *artist) viewHeader(c *ui.Context, id, name, title string) {
	tk := theme.Current()
	subline := name
	if subline == "" {
		subline = i18n.T("Artist")
	}
	ui.Row(c.Key("header")).FillWidth().AlignItems(ui.End).Margin(0, 0, 24, 0).Shrink(0).Children(func() {
		ui.Column(c).MinWidth(0).Shrink(1).Children(func() {
			t := tk.Type["title"]
			ui.Text(c, title).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking).
				TextColor(theme.C("ink")).SingleLine().Role(ui.RoleHeading).Level(1)
			s := tk.Type["secondary"]
			if a.has(artistPath(id)) {
				ui.Link(c, subline, artistPath(id)).FontSize(s.Size).TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
			} else {
				ui.Text(c, subline).FontSize(s.Size).FixedLineHeight(s.LineHeight).TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
			}
		})
	})
}

// viewBody is the list under the header: loading, failed, the songs or the
// tiles, and the empty state.
func (a *artist) viewBody(c *ui.Context, vs *viewState, id, view, name, title string, pane *kit.Pane) {
	pages, err, fetching, more := a.viewPages(id, view, max(1, vs.depth))
	vs.depth = max(vs.depth, 1)
	if err != nil {
		kit.ErrorBlock(c, i18n.T("Couldn't load this list"), err, kit.ErrorBlockOpts{
			OnRetry:  func() { a.env.Queries.Invalidate(keyArtistView(id, view)) },
			Retrying: fetching,
		})
		return
	}
	first := len(pages) == 0
	loading := 0
	switch {
	case first:
		loading = loadingFirst
	case fetching:
		loading = loadingNext
	}
	// Fetch the next page when the last loaded rows come into view (the
	// last range the list reported, which a page arriving does not change).
	total := 0
	for _, p := range pages {
		total += len(p.Songs) + len(p.Albums) + len(p.Playlists) + len(p.Videos) + len(p.Artists)
	}
	if vs.rangeEnd >= total-prefetch && more && !fetching && vs.depth <= len(pages) {
		vs.depth = len(pages) + 1
		c.Invalidate()
	}
	onRange := func(_, end int) { vs.rangeEnd = end }

	if view == services.ViewTopSongs {
		if vs.loaded != len(pages) {
			vs.songs = vs.songs[:0]
			for _, p := range pages {
				vs.songs = append(vs.songs, p.Songs...)
			}
			vs.loaded = len(pages)
		}
		songs := vs.songs
		if vs.tracks.menus == nil {
			vs.tracks.menus = kit.NewActionMenus(a.env.Actions, &vs.tracks.Sel)
		}
		cur := a.env.Play.State()
		o := kit.TrackListOpts{
			Label:      title,
			Columns:    kit.TrackColumns{RowNumbers: true, Artwork: true, Album: true},
			OnPlay:     func(i int) { a.queue.PlaySongs(songs, i, from(id)) },
			Playing:    cur.State == "playing",
			Restricted: a.restricted(),
			Art:        a.env.Art, Menus: vs.tracks.menus, Motion: a.env.Motion, Pane: pane, OnRange: onRange,
		}
		if cur.Current != nil {
			cid, cat := cur.Current.ID, cur.Current.CatalogID
			o.Current = func(s *services.Song) bool { return kit.SongIsCurrent(s, cid, cat) }
		}
		if a.has(albumPath("_")) {
			o.OpenAlbum = func(id string) { a.env.Nav.Push(albumPath(id)) }
		}
		kit.TrackList(c, &vs.tracks.TrackListState, total+loading, func(i int) *services.Song {
			if i < total {
				return &songs[i]
			}
			return nil
		}, o)
	} else {
		if vs.loaded != len(pages) {
			vs.items = vs.items[:0]
			for _, p := range pages {
				vs.items = append(vs.items, pageItems(p)...)
			}
			vs.loaded = len(pages)
		}
		if vs.menus == nil {
			vs.menus = kit.NewActionMenus(a.env.Actions, &vs.grid.CoverState.Sel)
		}
		tileMin := float32(160)
		if view == services.ViewMusicVideos || view == services.ViewTopMusicVideos {
			tileMin = 240
		}
		ctx := kit.ItemsCtx{Nav: a.open, Art: a.env.Art, Motion: a.env.Motion, Menus: vs.menus, Restricted: a.restricted()}
		kit.ItemGrid(c, &vs.grid, total+loading, func(i int) *services.Item {
			if i < total {
				return &vs.items[i]
			}
			return nil
		}, ctx, kit.ItemGridOpts{Label: title, Min: tileMin, Pane: pane, OnRange: onRange})
	}

	if !first && total == 0 {
		text := i18n.T("Apple Music has nothing in this list for this artist in your storefront.")
		if name != "" {
			text = i18n.T("Apple Music has nothing in this list for {name} in your storefront.", "name", name)
		}
		kit.EmptyState(c, i18n.T("Nothing here yet"), text, kit.EmptyOpts{}).Margin(8, 0, 0, 0)
	}
}
