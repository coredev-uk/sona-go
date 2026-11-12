package library

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The ages Recently added is filed under, newest first.
const (
	bucketToday = iota
	bucketWeek
	bucketMonth
	bucketEarlier
	recentBuckets
)

// recentGroup is a run of the list added in one age: items start to
// start+count.
type recentGroup struct{ bucket, start, count int }

// recentGrid is the state of one group's cover grid.
type recentGrid struct {
	st    kit.CoverState
	menus *kit.ActionMenus
}

// recentPage is the Recently added page's view state.
type recentPage struct {
	pageView
	// cover holds the pager, and the grid of the loading placeholders.
	cover coverList
	grids [recentBuckets]recentGrid
	// built is the tiles each group's grid built last (as start+1, end+1; 0
	// is none), which the pages fetched follow.
	built [recentBuckets][2]int
	// groups are the list's runs; they are read again when the first page,
	// the size of the list or the day changes.
	groups   []recentGroup
	src      *services.LibraryEntry
	srcLen   int
	srcTotal int
	srcDay   int
}

// recentBucket is the age an item added at ms (Unix milliseconds) is filed
// under: calendar days, as History counts them. An unknown date is Earlier.
func recentBucket(ms int64, now time.Time) int {
	if ms <= 0 {
		return bucketEarlier
	}
	switch ago := i18n.CalendarDays(time.UnixMilli(ms), now, time.Local); {
	case ago <= 0:
		return bucketToday
	case ago < 7:
		return bucketWeek
	case ago < 30:
		return bucketMonth
	}
	return bucketEarlier
}

func recentBucketLabel(b int) string {
	switch b {
	case bucketToday:
		return i18n.T("Today")
	case bucketWeek:
		return i18n.T("Past week")
	case bucketMonth:
		return i18n.T("Past month")
	}
	return i18n.T("Earlier")
}

func entryAdded(e *services.LibraryEntry) int64 {
	switch {
	case e.Album != nil:
		return e.Album.DateAdded
	case e.Playlist != nil:
		return e.Playlist.DateAdded
	}
	return 0
}

// groupsOf files the first page's entries (newest first) under their ages.
// What lies past the first page is older than all of it, so it joins the
// last group when that is Earlier, and is Earlier otherwise.
func groupsOf(items []services.LibraryEntry, total int, now time.Time) []recentGroup {
	var out []recentGroup
	for i := range items {
		b := recentBucket(entryAdded(&items[i]), now)
		if n := len(out); n > 0 {
			b = max(b, out[n-1].bucket) // newest first: an age never goes back
			if out[n-1].bucket == b {
				out[n-1].count++
				continue
			}
		}
		out = append(out, recentGroup{bucket: b, start: i, count: 1})
	}
	if rest := total - len(items); rest > 0 {
		if n := len(out); n > 0 && out[n-1].bucket == bucketEarlier {
			out[n-1].count += rest
		} else {
			out = append(out, recentGroup{bucket: bucketEarlier, start: len(items), count: rest})
		}
	}
	return out
}

// groupsNow is the list's groups, worked out again only when what they
// follow changed.
func (pg *recentPage) groupsNow(list *listView[services.LibraryEntry], now time.Time) []recentGroup {
	items := list.first.Data.Items
	var first *services.LibraryEntry
	if len(items) > 0 {
		first = &items[0]
	}
	day := now.Year()*1000 + now.YearDay()
	if pg.groups != nil && pg.src == first && pg.srcLen == len(items) && pg.srcTotal == list.Total && pg.srcDay == day {
		return pg.groups
	}
	pg.src, pg.srcLen, pg.srcTotal, pg.srcDay = first, len(items), list.Total, day
	pg.groups = groupsOf(items, list.Total, now)
	pg.built = [recentBuckets][2]int{}
	return pg.groups
}

// report records the tiles a group's grid built, and moves the pages
// fetched to cover every group's.
func (pg *recentPage) report(c *ui.Context, bucket, start, end int) {
	pg.built[bucket] = [2]int{start + 1, end + 1}
	lo, hi := math.MaxInt, 0
	for _, r := range pg.built {
		if r[0] > 0 {
			lo, hi = min(lo, r[0]-1), max(hi, r[1]-1)
		}
	}
	if lo == math.MaxInt {
		return
	}
	next := [2]int{lo / pageSize, hi / pageSize}
	if next != [2]int{pg.cover.first, pg.cover.last} {
		pg.cover.first, pg.cover.last = next[0], next[1]
		c.Invalidate()
	}
}

// recentPage is RecentlyAddedPage: albums and playlists in the order they
// joined the library, newest first (no sort control), filed under the age
// they were added: Today, Past week, Past month, Earlier.
func (l *library) recentPage(c *ui.Context, r *ui.Route) {
	pg := &l.recent
	list := l.recentList()

	subline := ""
	if list.Total > 0 && !list.Pending() {
		// translators: {0} is the number of albums and playlists listed.
		subline = i18n.T("{0, plural, one {# album or playlist} other {# albums and playlists}}", "0", list.Total)
	}
	l.scrollPage(c, &pg.pageView, "recent", func(pane *kit.Pane) {
		l.pageHead(c, headOpts{title: i18n.T("Recently added"), count: subline})
		if l.gate(c, gateOpts{
			total: list.Total, pending: list.Pending(), err: list.Err(), retry: list.Retry, retrying: list.Retrying(),
			kind: kindRecent, frame: kit.FrameCovers,
			empty: i18n.T("Albums and playlists you add in Apple Music appear here, newest first."),
		}) {
			return
		}
		if list.Pending() {
			ui.Row(c.Key("loading-head")).Height(22).AlignItems(ui.Center).Margin(0, 0, 16, 0).Children(func() {
				kit.Skeleton(c, 96, 16)
			})
			coverGrid(l, c, &pg.cover, pane, list, i18n.T("Recently added"), l.entryTile())
			return
		}
		tile := l.entryTile()
		now := time.Now()
		for gi, g := range pg.groupsNow(list, now) {
			grid := &pg.grids[g.bucket]
			label := recentBucketLabel(g.bucket)
			last := gi == len(pg.groups)-1
			ui.Column(c.Key(g.bucket)).FillWidth().Role(ui.RoleGroup).Label(label).Children(func() {
				ui.Row(c.Key("heading")).FillWidth().AlignItems(ui.Center).Gap(12).Margin(0, 0, 16, 0).Children(func() {
					theme.Text(c, label, "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
					theme.Text(c, i18n.FormatNumber(float64(g.count)), "sub").TextColor(theme.C("ink-muted")).FontFeatures("tnum").Role(ui.RoleNone)
					ui.Box(c.Key("rule")).Grow(1).Height(1).Background(theme.C("line"))
				})
				start := g.start
				kit.CoverGrid(c, &grid.st, g.count, func(i int) *kit.CoverItem {
					e := list.At(start + i)
					if e == nil {
						return nil
					}
					t := tile(e)
					return &t
				}, kit.CoverGridOpts{
					CoverOpts: kit.CoverOpts{
						Label: i18n.T("Recently added, {when}", "when", label), Art: l.env.Art, Motion: l.env.Motion, Restricted: l.restricted(),
						Menus: l.itemMenus(&grid.menus, &grid.st.Sel),
					},
					Pane:    pane,
					OnRange: func(s, e int) { pg.report(c, g.bucket, start+s, start+e) },
				})
				if !last {
					ui.Box(c.Key("space")).Height(40)
				}
			})
		}
	})
}

// keyRecentlyAdded is the prefix of the Recently added pages' keys: a sync
// of albums or playlists and a change to the playlist tree invalidate it.
var keyRecentlyAdded = store.Key{"library", "recently-added"}

// recentList is the recently added entries in pages of 200 (key `["library",
// "recently-added", page]`).
func (l *library) recentList() *listView[services.LibraryEntry] {
	return usePaged(l, &l.recent.cover.pager, func(n int) store.Key {
		return append(slices.Clip(keyRecentlyAdded), n)
	}, func(ctx context.Context, off, lim int) (listPage[services.LibraryEntry], error) {
		p, err := l.lib.RecentlyAdded(ctx, off, lim)
		return listPage[services.LibraryEntry]{p.Items, p.Total}, err
	})
}

// entryTile is the tile of an entry: an album as in Albums, a playlist as in
// Playlists with "Playlist" for a subtitle when it has no curator. An entry
// of neither is an empty tile.
func (l *library) entryTile() func(e *services.LibraryEntry) kit.CoverItem {
	album, playlist := l.albumTile(), l.playlistTile()
	return func(e *services.LibraryEntry) kit.CoverItem {
		switch {
		case e.Album != nil:
			return album(e.Album)
		case e.Playlist != nil:
			tile := playlist(e.Playlist)
			if e.Playlist.Curator == "" {
				tile.Subtitle = i18n.T("Playlist")
			}
			return tile
		}
		return kit.CoverItem{}
	}
}
