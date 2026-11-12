package library

import (
	"context"
	"time"

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
		// translators: {0} is the number of singles and EPs listed.
		subline = i18n.T("{0, plural, one {# single or EP} other {# singles & EPs}}", "0", list.Total)
	default:
		// translators: {0} is the number of albums listed.
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

	var rail func()
	if railShown(q.sort, q.desc, list.Total) && !filteredNone {
		jumps, ready := l.albumLetters(q, list)
		rail = func() {
			l.letterRail(c, jumps, ready, func(slot int) {
				pg.jumpTo(jumps[slot])
				c.Invalidate()
			})
		}
	}
	l.scrollPageRail(c, &pg.pageView, q.key(), true, rail, func(pane *kit.Pane) {
		l.pageHead(c, headOpts{title: i18n.T("Albums"), count: subline, actions: func() {
			// The type and the sort are the page's filter bar.
			ui.Row(c.Key("filters")).Gap(8).AlignItems(ui.Center).Role(ui.RoleSearch).Label(i18n.T("Filter albums")).Children(func() {
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
		}})
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
			if rail != nil {
				pg.gridAnchor(c, pane)
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

// albumTile is CoverTile of a library album: its page when one exists and the
// album as the tile's menu target; a pre-added album not out yet says when it
// is ("Out 19 Nov"), as a catalogue tile does. v0.2.0's tiles have no play
// control.
func (l *library) albumTile() func(a *services.Album) kit.CoverItem {
	return func(a *services.Album) kit.CoverItem {
		it := *a
		tile := kit.CoverItem{
			Key:      a.ID,
			Title:    a.Name,
			Subtitle: a.Artist,
			Art:      a.Art.Medium,
			Explicit: a.Explicit,
			Badge:    kit.ReleaseBadge(it, time.Now()),
			Item:     &kit.Item{Kind: kit.KindAlbum, Target: it},
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

// jumpTo scrolls the grid so the row of item i is the first in view.
func (pg *albumsPage) jumpTo(i int) {
	cols, tile := kit.CoverColumns(pg.gridW, 0)
	pg.pos.Y = max(0, pg.gridTop+float32(i/cols)*kit.CoverRowHeight(tile, false)-16)
}

// albumLetters is the rail of albums ordered by title or artist.
func (l *library) albumLetters(q listQuery, list *listView[services.Album]) (letterJumps, bool) {
	name := func(a *services.Album) string {
		if q.sort == "artist" {
			return a.Artist
		}
		return a.Name
	}
	held := func(i int) string {
		if a := list.At(i); a != nil {
			return name(a)
		}
		return ""
	}
	key := store.Key{"library", "albums", "letters", q.sort, q.kind, list.Total}
	return jumpsOf(l, list.Total, list.first.HasData, held, key, func(ctx context.Context, i int) (string, error) {
		p, err := l.lib.Albums(ctx, q.sort, false, q.kind, i, 1)
		if err != nil || len(p.Items) == 0 {
			return "", err
		}
		return name(&p.Items[0]), nil
	})
}
