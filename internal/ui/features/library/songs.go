package library

import (
	"context"
	"strings"

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
	railFrame
	list songList
	// find is the find field's text. urlFind is the ?q= last read from the URL
	// and sent the text last written to it, which tells the URL's echo of
	// the field from a navigation that changed it.
	find, urlFind, sent string
	// announced is the find result last read out.
	announced string
}

// songsList is the songs of q in pages of 200 (key `["library", "songs",
// sort, desc, page, find?]`).
func (l *library) songsList(q listQuery) *listView[services.Song] {
	return usePaged(l, &l.songs.list.pager, func(n int) store.Key {
		key := store.Key{"library", "songs", q.sort, q.desc, n}
		if q.find != "" {
			key = append(key, q.find)
		}
		return key
	}, func(ctx context.Context, off, lim int) (listPage[services.Song], error) {
		p, err := l.lib.Songs(ctx, q.sort, q.desc, q.find, off, lim)
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
	counts := store.Use(l.env.Queries, store.KeyLibraryCounts, l.lib.Counts)
	l.syncFind(q)

	// While the words find nothing the library is not empty: the gate must
	// not say so.
	filtering := q.filtering()
	total := list.Total
	if filtering {
		total = max(total, counts.Data.Songs, 1)
	}
	filteredNone := filtering && !list.Pending() && list.Total == 0
	subline := ""
	switch {
	case list.Total == 0:
	case filtering && counts.HasData:
		subline = i18n.T("{found} of {total, plural, one {# song} other {# songs}}",
			"found", i18n.FormatNumber(float64(list.Total)), "total", counts.Data.Songs)
	default:
		subline = i18n.SongCount(list.Total)
	}
	l.announceFound(c, q, list)
	var rail func()
	if !filtering && railShown(q.sort, q.desc, list.Total) {
		jumps, ready := l.songLetters(q, list)
		rail = func() {
			l.letterRail(c, jumps, ready, func(slot int) {
				// The body starts under the table's header row.
				pg.Pos.Y = max(0, pg.gridTop+songsHeadHeight+float32(jumps[slot])*kit.TrackRowHeight)
				c.Invalidate()
			})
		}
	}
	l.scrollRail(c, &pg.railFrame, q.key(), rail, func(pane *kit.Pane) {
		l.pageHead(c, headOpts{title: i18n.T("Songs"), count: subline, actions: func() {
			l.songFinder(c, q)
			l.sortSelect(c, songsSpec, q)
			opts := kit.PlayButtonsOpts{PlayLabel: i18n.T("Play your songs"), Shuffle: true, Disabled: list.Total == 0}
			if filtering {
				// Name the controls for what they queue: the songs found.
				opts.PlayLabel = i18n.T("{found, plural, one {Play the # song found} other {Play the # songs found}}", "found", list.Total)
				opts.ShuffleLabel = i18n.T("{found, plural, one {Shuffle the # song found} other {Shuffle the # songs found}}", "found", list.Total)
			}
			play, shuffle := kit.PlayButtons(c, opts)
			if play {
				l.playSongsFrom(q, 0, false)
			}
			if shuffle {
				l.playSongsFrom(q, randomPlayable(list.Loaded()), true)
			}
		}})
		if l.gate(c, gateOpts{
			total: total, pending: list.Pending(), err: list.Err(), retry: list.Retry, retrying: list.Retrying(),
			kind: kindSongs, frame: kit.FrameBox,
			empty: i18n.T("Songs you add in Apple Music on any device appear here after the next sync."),
		}) {
			return
		}
		if filteredNone {
			kit.EmptyState(c, i18n.T("No songs match “{query}”", "query", strings.TrimSpace(q.find)),
				i18n.T("Check the spelling, or try fewer words. Find looks at a song's title, artist and album."),
				kit.EmptyOpts{Frame: kit.FrameBox, Action: &kit.EmptyAction{
					Label: i18n.T("Clear search"),
					Run: func() {
						next := q
						next.find = ""
						l.env.Nav.Replace(songsSpec.location(next))
					},
				}})
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

// syncFind brings the field's text in line with the URL when the URL is what
// changed (back, forward, a restore, a link), not when it only echoes what
// the field sent.
func (l *library) syncFind(q listQuery) {
	pg := &l.songs
	if q.find == pg.urlFind {
		return
	}
	pg.urlFind = q.find
	if q.find != pg.sent {
		pg.find = q.find
	}
}

// songFinder is the find field of the header, "Find in your songs", in a
// search landmark. Typing replaces the URL's ?q= (one history entry whatever
// the keystrokes); Escape in the field and its clear button empty it.
func (l *library) songFinder(c *ui.Context, q listQuery) {
	pg := &l.songs
	ui.Column(c.Key("find")).Shrink(0).Role(ui.RoleSearch).Label(i18n.T("Find songs in your library")).Children(func() {
		label := i18n.T("Find in your songs")
		kit.SearchField(c, &pg.find, kit.SearchFieldOpts{Placeholder: label, A11yLabel: label, Width: 220})
	})
	if pg.find != q.find {
		pg.sent = pg.find
		next := q
		next.find = pg.find
		l.env.Nav.Replace(songsSpec.location(next))
	}
}

// announceFound reads out the find's result when it changes (a list's result
// is not otherwise announced).
func (l *library) announceFound(c *ui.Context, q listQuery, list *listView[services.Song]) {
	pg := &l.songs
	text := ""
	if q.filtering() && !list.Pending() {
		if list.Total > 0 {
			text = i18n.T("{matches, plural, one {# song matches} other {# songs match}}", "matches", list.Total)
		} else {
			text = i18n.T("No song matches “{query}”", "query", strings.TrimSpace(q.find))
		}
	}
	if text != "" && pg.announced != text {
		c.Announce(text)
	}
	pg.announced = text
}

// playSongsFrom replaces the queue with the library in q's order (the songs
// found, while q finds), starting at index start; shuffle turns shuffle on
// first.
func (l *library) playSongsFrom(q listQuery, start int, shuffle bool) {
	l.env.Queue.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		if shuffle {
			l.play.SetShuffle(true)
		}
		return l.play.PlayLibrary(ctx, q.sort, q.desc, q.find, start)
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
	opts := l.env.TrackOpts(l.itemMenus(&g.menus, &g.st.Sel), l.restricted())
	opts.Label = o.label
	opts.Columns = kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: true}
	opts.OnPlay = o.onPlay
	opts.Sort = o.sort
	opts.OnRange = g.onRange(c)
	opts.Pane = pane
	kit.TrackList(c, &g.st, count, func(i int) *services.Song { return at(i) }, opts)
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
		p, err := l.lib.Songs(ctx, q.sort, false, "", i, 1)
		if err != nil || len(p.Items) == 0 {
			return "", err
		}
		return name(&p.Items[0]), nil
	})
}
