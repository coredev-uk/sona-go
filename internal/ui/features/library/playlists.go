package library

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// playlistsPage is the Playlists page's view state.
type playlistsPage struct {
	kit.PageFrame
	cover coverList
}

// playlistsPage is LibraryPlaylistsPage: the header (title, count, the sort
// control), then the cover grid inside one scroll container that scrolls the
// header too.
func (l *library) playlistsPage(c *ui.Context, r *ui.Route) {
	q := playlistsSpec.parse(r)
	pg := &l.playlists
	list := l.playlistList(q)

	subline := ""
	if list.Total > 0 && !list.Pending() {
		// translators: {0} is the number of playlists listed.
		subline = i18n.T("{0, plural, one {# playlist} other {# playlists}}", "0", list.Total)
	}
	pg.Scroll(c, q.key(), func(pane *kit.Pane, _ float32) {
		kit.PageHeaderCounted(c, i18n.T("Playlists"), subline, func() {
			l.sortSelect(c, playlistsSpec, q)
		})
		if l.gate(c, gateOpts{
			total: list.Total, pending: list.Pending(), err: list.Err(), retry: list.Retry, retrying: list.Retrying(),
			kind: kindPlaylists, frame: kit.FrameCovers,
			empty: i18n.T("Playlists you make or add in Apple Music appear here after the next sync."),
		}) {
			return
		}
		coverGrid(l, c, &pg.cover, pane, list, i18n.T("Playlists in your library"), l.playlistTile())
	})
}

// playlistList is the playlists of q in pages of 200 (key `["library",
// "playlists", sort, desc, page]`).
func (l *library) playlistList(q listQuery) *listView[services.Playlist] {
	return usePaged(l, &l.playlists.cover.pager, func(n int) store.Key {
		return store.LibraryPlaylists(q.sort, q.desc, n)
	}, func(ctx context.Context, off, lim int) (listPage[services.Playlist], error) {
		p, err := l.lib.Playlists(ctx, q.sort, q.desc, off, lim)
		return listPage[services.Playlist]{p.Items, p.Total}, err
	})
}

// playlistSubtitle is the curator, else how many songs it holds.
func playlistSubtitle(p *services.Playlist) string {
	if p.Curator != "" {
		return p.Curator
	}
	if p.TrackCount > 0 {
		return i18n.SongCount(p.TrackCount)
	}
	return ""
}

// playlistTile is CoverTile of a library playlist: its page when one exists
// and the playlist as the tile's menu target. As in v0.2.0 it has no play
// control: a playlist plays from its page.
func (l *library) playlistTile() func(p *services.Playlist) kit.CoverItem {
	return func(p *services.Playlist) kit.CoverItem {
		item := kit.PlaylistItem(*p)
		tile := kit.CoverItem{
			Key:      p.ID,
			Title:    p.Name,
			Subtitle: playlistSubtitle(p),
			Art:      p.Art.Medium,
			Item:     &item,
		}
		l.openTo(&tile, "/playlist/"+p.ID)
		return tile
	}
}
