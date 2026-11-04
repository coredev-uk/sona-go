package library

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// albumTypes are the Album type control's choices: "" is all.
var albumTypes = []string{"", "albums", "singles"}

// albumsPage is the Albums page's view state.
type albumsPage struct {
	pageView
	cover coverList
	// announced is the filter result last read out.
	announced string
}

// albumsPage is LibraryAlbumsPage: the header (title, count, the type and
// sort controls), then the cover grid inside one scroll container that
// scrolls the header too.
func (l *library) albumsPage(c *ui.Context, r *ui.Route) {
	q := albumsSpec.parse(r)
	pg := &l.albums
	list := l.albumList(q)
	counts := store.Use(l.env.Queries, keyCounts, l.lib.Counts)

	// A type with no albums must not read as an empty library.
	total := list.Total
	if q.kind != "" {
		total = max(total, counts.Data.Albums)
	}
	filteredNone := q.kind != "" && !list.Pending() && list.Total == 0
	noneTitle := i18n.T("No albums besides singles and EPs")
	if q.kind == "singles" {
		noneTitle = i18n.T("No singles or EPs in your library")
	}
	subline := ""
	switch {
	case list.Pending() || (list.Total == 0 && q.kind == ""):
	case q.kind == "singles":
		subline = i18n.T("{0, plural, one {# single or EP} other {# singles & EPs}}", "0", list.Total)
	default:
		subline = i18n.T("{0, plural, one {# album} other {# albums}}", "0", list.Total)
	}
	// What a chosen type shows, read out when it changes (a filter's result
	// is not otherwise announced).
	announcement := ""
	if q.kind != "" && !list.Pending() {
		announcement = subline
		if filteredNone {
			announcement = noneTitle
		}
	}
	if announcement != "" && announcement != pg.announced {
		c.Announce(announcement)
	}
	pg.announced = announcement

	l.scrollPage(c, &pg.pageView, q.key(), func(pane *kit.Pane) {
		kit.PageHeader(c, i18n.T("Albums"), subline, func() {
			kind := q.kind
			opts := []kit.SegmentOption[string]{
				{Value: "", Label: i18n.T("All")},
				{Value: "albums", Label: i18n.T("Albums")},
				{Value: "singles", Label: i18n.T("Singles & EPs")},
			}
			if kit.Segmented(c, &kind, opts, i18n.T("Album type")).Changed() && kind != q.kind {
				next := q
				next.kind = kind
				l.env.Nav.Replace(albumsSpec.location(next))
			}
			l.sortSelect(c, albumsSpec, q)
		})
		switch {
		case l.gate(c, gateOpts{
			total: total, pending: list.Pending(), err: list.Err(), retry: list.Retry, retrying: list.Retrying(),
			kind: kindAlbums, frame: kit.FrameCovers,
			empty: i18n.T("Albums you add in Apple Music appear here after the next sync, newest first."),
		}):
		case filteredNone:
			body := i18n.T("Everything in your library is a single or an EP.")
			if q.kind == "singles" {
				body = i18n.T("Singles and EPs you add in Apple Music appear here after the next sync.")
			}
			kit.EmptyState(c, noneTitle, body, kit.EmptyOpts{Frame: kit.FrameCovers, Action: &kit.EmptyAction{
				Label: i18n.T("Show all albums"),
				Run: func() {
					next := q
					next.kind = ""
					l.env.Nav.Replace(albumsSpec.location(next))
				},
			}})
		default:
			label := i18n.T("Albums in your library")
			if q.kind == "singles" {
				label = i18n.T("Singles and EPs in your library")
			}
			coverGrid(l, c, &pg.cover, pane, list, label, l.albumTile())
		}
	})
}

// albumList is the albums of q in pages of 200 (key `["library", "albums",
// sort, desc, page, type?]`).
func (l *library) albumList(q listQuery) *listView[services.Album] {
	return usePaged(l, &l.albums.cover.pager, func(n int) store.Key {
		key := store.Key{"library", "albums", q.sort, q.desc, n}
		if q.kind != "" {
			key = append(key, q.kind)
		}
		return key
	}, func(ctx context.Context, off, lim int) (listPage[services.Album], error) {
		p, err := l.lib.Albums(ctx, q.sort, q.desc, q.kind, off, lim)
		return listPage[services.Album]{p.Items, p.Total}, err
	})
}

// albumTile is CoverTile of a library album: its page when one exists, the
// play control, and the album as the tile's menu target.
func (l *library) albumTile() func(a *services.Album) kit.CoverItem {
	return func(a *services.Album) kit.CoverItem {
		it := *a
		tile := kit.CoverItem{
			Key:      a.ID,
			Title:    a.Name,
			Subtitle: a.Artist,
			Art:      a.Art.Medium,
			Explicit: a.Explicit,
			Item:     &kit.Item{Kind: kit.KindAlbum, Target: it},
			Play:     &kit.CoverPlay{Label: a.Name, Run: func() { l.playAlbum(it) }},
		}
		l.openTo(&tile, "/album/"+a.ID)
		return tile
	}
}

// openTo makes a tile open the page at path, when a module registered it:
// nothing links to a page that is not ported yet.
func (l *library) openTo(t *kit.CoverItem, path string) {
	if l.env.HasRoute != nil && l.env.HasRoute(path) {
		t.Open = func() { l.env.Nav.Push(path) }
	}
}

// playAlbum replaces the queue with the album's songs and plays it from the
// first one (v0.2.0's playSongs over the album's tracks): the songs are
// read from the library, then PlaybackService starts the queue, both off
// the main thread.
func (l *library) playAlbum(a services.Album) {
	go func() {
		detail, err := l.lib.Album(context.Background(), a.ID)
		if err != nil {
			failToast(i18n.T("Couldn't load the album"), err)
			return
		}
		l.env.Post(func() { l.env.Queue.PlaySongs(detail.Tracks, 0, services.PlayFrom{Kind: "album", ID: a.ID}) })
	}()
}
