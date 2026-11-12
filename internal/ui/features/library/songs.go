package library

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// songList is the state of a paged list shown as a track table.
type songList struct {
	st    kit.TrackListState
	menus *kit.ActionMenus
	pager
}

// songsPage is the Songs page's view state.
type songsPage struct {
	pageView
	list songList
}

// songsList is the songs of q in pages of 200 (key `["library", "songs",
// sort, desc, page]`).
func (l *library) songsList(q listQuery) *listView[services.Song] {
	return usePaged(l, &l.songs.list.pager, func(n int) store.Key {
		return store.Key{"library", "songs", q.sort, q.desc, n}
	}, func(ctx context.Context, off, lim int) (listPage[services.Song], error) {
		p, err := l.lib.Songs(ctx, q.sort, q.desc, off, lim)
		return listPage[services.Song]{p.Items, p.Total}, err
	})
}

// songsHeadHeight is the table's header row and the space under it, which
// the rows start after (kit's trackHeaderHeight and the body's 4px).
const songsHeadHeight = 36

// songsPage is LibrarySongsPage: the header (title, count, sort and play
// controls) and the song table inside one scroll container.
func (l *library) songsPage(c *ui.Context, r *ui.Route) {
	q := songsSpec.parse(r)
	pg := &l.songs
	list := l.songsList(q)

	subline := ""
	if list.Total > 0 {
		subline = i18n.T("{0, plural, one {# song} other {# songs}}", "0", list.Total)
	}
	var rail func()
	if railShown(q.sort, q.desc, list.Total) {
		jumps, ready := l.songLetters(q, list)
		rail = func() {
			l.letterRail(c, jumps, ready, func(slot int) {
				// The body starts under the table's header row.
				pg.pos.Y = max(0, pg.gridTop+songsHeadHeight+float32(jumps[slot])*kit.TrackRowHeight)
				c.Invalidate()
			})
		}
	}
	l.scrollPageRail(c, &pg.pageView, q.key(), true, rail, func(pane *kit.Pane) {
		l.pageHead(c, headOpts{title: i18n.T("Songs"), count: subline, actions: func() {
			l.sortSelect(c, songsSpec, q)
			play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{
				PlayLabel: i18n.T("Play your songs"), Shuffle: true, Disabled: list.Total == 0,
			})
			if play {
				l.playSongsFrom(q, 0, false)
			}
			if shuffle {
				l.playSongsFrom(q, randomPlayable(list.Loaded()), true)
			}
		}})
		if l.gate(c, gateOpts{
			total: list.Total, pending: list.Pending(), err: list.Err(), retry: list.Retry, retrying: list.Retrying(),
			kind: kindSongs, frame: kit.FrameBox,
			empty: i18n.T("Songs you add in Apple Music on any device appear here after the next sync."),
		}) {
			return
		}
		sortState := &kit.TrackSortState{Desc: q.desc, OnSort: func(key string) {
			next := q
			if key == q.sort {
				next.desc = !q.desc
			} else {
				next.sort, next.desc = key, startsDesc(key)
			}
			l.env.Nav.Replace(songsSpec.location(next))
		}}
		switch q.sort {
		case kit.SortTitle, kit.SortArtist, kit.SortAlbum:
			sortState.Key = q.sort
		}
		if rail != nil {
			pg.gridAnchor(c, pane)
		}
		l.songTable(c, &pg.list, pane, list, songTableOpts{
			label:  i18n.T("Songs in your library"),
			sort:   sortState,
			onPlay: func(i int) { l.playSongsFrom(q, i, false) },
		})
	})
}

// playSongsFrom replaces the queue with the library in q's order, starting
// at index start; shuffle turns shuffle on first.
func (l *library) playSongsFrom(q listQuery, start int, shuffle bool) {
	l.env.Queue.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		if shuffle {
			l.play.SetShuffle(true)
		}
		return l.play.PlayLibrary(ctx, q.sort, q.desc, start)
	}, nil)
}

// songTableOpts are what a page's song table adds to the shared setup.
type songTableOpts struct {
	label  string
	sort   *kit.TrackSortState
	onPlay func(i int)
}

// songTable builds a paged list of songs as a kit.TrackList in pane; a list
// still reading its first page shows loading rows. The rows built decide
// the pages fetched.
func (l *library) songTable(c *ui.Context, g *songList, pane *kit.Pane, list *listView[services.Song], o songTableOpts) {
	count := list.Total
	at := list.At
	if list.Pending() {
		count, at = skeletonRows, func(int) *services.Song { return nil }
	}
	var cur *services.QueueItem
	playing := false
	if l.env.Play != nil {
		s := l.env.Play.State()
		cur = s.Current
		playing = s.State == store.StatePlaying
	}
	opts := kit.TrackListOpts{
		Label:      o.label,
		Columns:    kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: true},
		OnPlay:     o.onPlay,
		Sort:       o.sort,
		OnRange:    g.onRange(c),
		Pane:       pane,
		Current:    cur,
		Playing:    playing,
		Restricted: l.restricted(),
		Art:        l.env.Art,
		Motion:     l.env.Motion,
		Menus:      l.itemMenus(&g.menus, &g.st.Sel),
	}
	if l.hasPage("/artist/x") {
		opts.OpenArtist = func(id string) { l.env.Nav.Push("/artist/" + id) }
	}
	if l.hasPage("/album/x") {
		opts.OpenAlbum = func(id string) { l.env.Nav.Push("/album/" + id) }
	}
	kit.TrackList(c, &g.st, count, func(i int) *services.Song { return at(i) }, opts)
}

// hasPage: a module registered a page for path (the song table links only
// to pages that exist).
func (l *library) hasPage(path string) bool {
	return l.env.HasRoute != nil && l.env.HasRoute(path)
}

// songLetters is the rail of songs ordered by title, artist or album.
func (l *library) songLetters(q listQuery, list *listView[services.Song]) (letterJumps, bool) {
	name := func(s *services.Song) string {
		switch q.sort {
		case "artist":
			return s.Artist
		case "album":
			return s.Album
		}
		return s.Name
	}
	held := func(i int) string {
		if s := list.At(i); s != nil {
			return name(s)
		}
		return ""
	}
	key := store.Key{"library", "songs", "letters", q.sort, list.Total}
	return jumpsOf(l, list.Total, list.first.HasData, held, key, func(ctx context.Context, i int) (string, error) {
		p, err := l.lib.Songs(ctx, q.sort, false, i, 1)
		if err != nil || len(p.Items) == 0 {
			return "", err
		}
		return name(&p.Items[0]), nil
	})
}
