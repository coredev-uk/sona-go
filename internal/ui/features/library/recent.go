package library

import (
	"context"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// recentPage is the Recently added page's view state.
type recentPage struct {
	kit.PageFrame
	cover coverList
}

// recentPage is RecentlyAddedPage: albums and playlists in the order they
// joined the library, newest first (no sort control).
func (l *library) recentPage(c *ui.Context, r *ui.Route) {
	pg := &l.recent
	list := l.recentList()

	subline := ""
	if list.Total > 0 && !list.Pending() {
		// translators: {0} is the number of albums and playlists listed.
		subline = i18n.T("{0, plural, one {# album or playlist} other {# albums and playlists}}", "0", list.Total)
	}
	pg.Scroll(c, "recent", func(pane *kit.Pane, _ float32) {
		kit.PageHeader(c, i18n.T("Recently added"), subline, nil)
		if l.gate(c, gateOpts{
			total: list.Total, pending: list.Pending(), err: list.Err(), retry: list.Retry, retrying: list.Retrying(),
			kind: kindRecent, frame: kit.FrameCovers,
			empty: i18n.T("Albums and playlists you add in Apple Music appear here, newest first."),
		}) {
			return
		}
		coverGrid(l, c, &pg.cover, pane, list, i18n.T("Recently added"), l.entryTile())
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
