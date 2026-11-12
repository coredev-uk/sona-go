package home

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// recentSongs is how many songs the Recently played shelf shows.
	recentSongs = 10
	// historyDays is how many days of this computer's history the fallback
	// reads (v0.2.0: History(30)).
	historyDays = 30
	// localShelfItems is how many albums and playlists the local shelves show.
	localShelfItems = 12
)

// localState is the state of the local shelves: the track list, and the cover
// rows of the albums and playlists.
type localState struct {
	played    kit.TrackListState
	menus     *kit.ActionMenus
	added     kit.CoverState
	playlists kit.CoverState
}

// localShelves is what Sona knows without Apple's recommendations: recent
// plays (Apple's, or this computer's history offline), recent additions and
// playlists. Shown signed out, offline without a cached Home, or when Home is
// empty.
func (h *home) localShelves(c *ui.Context, signedIn bool, ctx kit.ItemsCtx) {
	if signedIn {
		h.recentlyPlayed(c, ctx)
	}
	h.recentlyAdded(c, ctx)
	h.recentPlaylists(c, ctx)
}

// recentlyPlayed is Apple's recently played songs, or this computer's play
// history when Apple can't be reached.
func (h *home) recentlyPlayed(c *ui.Context, ctx kit.ItemsCtx) {
	q := h.env.Queries
	recent := store.Use(q, store.KeyLibraryRecent, func(cx context.Context) ([]services.Song, error) {
		return h.lib.RecentlyPlayed(cx, recentSongs, 0)
	}, store.Options{Stale: store.Forever})
	// Apple's list failed: what this computer played is a different list,
	// and says so. A refresh that failed behind data still shows the data.
	appleFailed := !recent.HasData && recent.Err != nil
	var history store.Result[services.ListeningHistory]
	if appleFailed {
		history = store.Use(q, keyRecentLocal, func(cx context.Context) (services.ListeningHistory, error) {
			return h.lib.History(cx, historyDays)
		}, store.Options{Stale: store.Forever})
	}

	title := i18n.T("Recently played")
	if appleFailed {
		title = i18n.T("Played on this computer")
	}
	var songs []services.Song
	have := false
	switch {
	case recent.HasData:
		songs, have = recent.Data, true
	case appleFailed && history.HasData:
		for _, d := range history.Data.Days {
			for _, e := range d.Entries {
				songs = append(songs, e.Song)
			}
		}
		songs, have = songs[:min(len(songs), recentSongs)], true
	}

	switch {
	case !have && (!appleFailed || history.Loading || !history.HasData && history.Err == nil):
		kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Recently played")}, func() { h.songsSkeleton(c) })
	case !have:
		kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Recently played")}, func() {
			kit.ErrorBlock(c, i18n.T("Couldn't load what you played recently"), recent.Err, kit.ErrorBlockOpts{
				OnRetry:  func() { q.Invalidate(store.KeyLibraryRecent) },
				Retrying: recent.Fetching || history.Fetching,
			})
		})
	case len(songs) == 0:
		kit.Shelf(c, kit.ShelfOpts{Title: title}, func() {
			body := i18n.T("Songs you play in Sona or Apple Music on any device appear here.")
			if appleFailed {
				body = i18n.T("Songs you play in Sona appear here.")
			}
			kit.EmptyState(c, i18n.T("Nothing played yet"), body, kit.EmptyOpts{})
		})
	default:
		kit.Shelf(c, kit.ShelfOpts{Title: title}, func() { h.songsList(c, songs, ctx) })
	}
}

// songsList is the songs as a track list: row numbers, artwork, artist and
// album; a click plays the list from that row, as the queue labelled
// "recently-played".
func (h *home) songsList(c *ui.Context, songs []services.Song, ctx kit.ItemsCtx) {
	st := &h.local
	o := h.env.TrackOpts(kit.MenusIn(&st.menus, h.env.Actions, &st.played.Sel), ctx.Restricted)
	o.Label = i18n.T("Recently played songs")
	o.Columns = kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: true}
	o.OnPlay = func(i int) { ctx.Players.PlaySongList(songs, i, recentlyPlayed) }
	kit.TrackList(c, &st.played, len(songs), func(i int) *services.Song { return &songs[i] }, o)
}

// songsSkeleton is five placeholder rows where the songs will be.
func (h *home) songsSkeleton(c *ui.Context) {
	hover, r := theme.C("hover"), theme.Current().Radius["sm"]
	ui.Column(c.Key("songs-loading")).Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		for i := range 5 {
			ui.Row(c.Key(i)).Height(44).PaddingX(8).Gap(12).AlignItems(ui.Center).Children(func() {
				kit.Bar(c, 32, 32).Margin(0, 0, 0, 56)
				ui.Box(c).WidthPercent(float32(24 + i*5)).Height(14).Radius(r).Background(hover)
			})
		}
	})
}

// recentlyAdded is the first 12 albums by date added, newest first, as a row of
// covers; nothing when the library has none.
func (h *home) recentlyAdded(c *ui.Context, ctx kit.ItemsCtx) {
	q := h.env.Queries
	res := store.Use(q, keyAdded, func(cx context.Context) (services.AlbumPage, error) {
		return h.lib.Albums(cx, "dateAdded", true, "", 0, localShelfItems)
	}, store.Options{Stale: store.Forever})
	title := i18n.T("Recently added")
	if res.Err != nil {
		kit.Shelf(c, kit.ShelfOpts{Title: title}, func() {
			kit.QueryFailed(c, q, i18n.T("Couldn't read your albums"), res.Err, keyAdded)
		})
		return
	}
	if res.HasData && len(res.Data.Items) == 0 {
		return
	}
	var tiles []kit.CoverItem
	open := h.env.Routed("/album/x")
	for _, a := range res.Data.Items {
		t := kit.CoverItem{Key: a.ID, Title: a.Name, Subtitle: a.Artist, Art: a.Art.Medium}
		if open {
			t.Open = func() { ctx.Nav("/album/" + a.ID) }
		}
		tiles = append(tiles, t)
	}
	h.coverShelf(c, &h.local.added, title, i18n.T("Albums recently added to your library"), "/library/albums", tiles, ctx)
}

// recentPlaylists is the 12 playlists last modified, as a row of covers: the
// song count, or the curator; nothing on a failure or when there are none.
func (h *home) recentPlaylists(c *ui.Context, ctx kit.ItemsCtx) {
	res := store.Use(h.env.Queries, keyPlaylists, func(cx context.Context) (services.PlaylistPage, error) {
		return h.lib.Playlists(cx, "lastModified", true, 0, localShelfItems)
	}, store.Options{Stale: store.Forever})
	if res.Err != nil || (res.HasData && len(res.Data.Items) == 0) {
		return
	}
	var tiles []kit.CoverItem
	open := h.env.Routed("/playlist/x")
	for _, p := range res.Data.Items {
		sub := p.Curator
		if p.TrackCount > 0 {
			sub = i18n.SongCount(p.TrackCount)
		}
		t := kit.CoverItem{Key: p.ID, Title: p.Name, Subtitle: sub, Art: p.Art.Medium}
		if open {
			t.Open = func() { ctx.Nav("/playlist/" + p.ID) }
		}
		tiles = append(tiles, t)
	}
	h.coverShelf(c, &h.local.playlists, i18n.T("Your playlists"), i18n.T("Recently updated playlists"), "/library/playlists", tiles, ctx)
}

// coverShelf is a titled row of tiles with "See all" to a library page, where
// a module serves it.
func (h *home) coverShelf(c *ui.Context, st *kit.CoverState, title, label, all string, tiles []kit.CoverItem, ctx kit.ItemsCtx) {
	so := kit.ShelfOpts{Title: title}
	if h.env.Routed(all) {
		so.More = func() { ctx.Nav(all) }
	}
	kit.Shelf(c, so, func() {
		kit.CoverRow(c, st, tiles, kit.CoverRowOpts{
			CoverOpts: kit.CoverOpts{Label: label, Art: h.env.Art, Motion: h.env.Motion, Restricted: ctx.Restricted},
		})
	})
}
