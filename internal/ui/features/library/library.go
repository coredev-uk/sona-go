// Package library is the library feature: the Library pages (Recently
// added, Artists, Albums, Songs, Music videos, Favourite songs, History,
// Playlists), the sync mirror and its sidebar indicator, the item-state
// layer and the item actions every list and header uses (add to and remove
// from the library, Favourite, Suggest less, Copy link).
package library

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// The library's routes.
const (
	recentPath    = "/library/recently-added"
	artistsPath   = "/library/artists"
	albumsPath    = "/library/albums"
	songsPath     = "/library/songs"
	videosPath    = "/library/music-videos"
	favouritePath = "/library/favourites"
	historyPath   = "/library/history"
	playlistsPath = "/library/playlists"
	madeForYou    = "/library/made-for-you"
)

// libraryAPI is what the pages and actions ask of LibraryService.
type libraryAPI interface {
	Songs(ctx context.Context, sort string, desc bool, offset, limit int) (services.SongPage, error)
	Albums(ctx context.Context, sort string, desc bool, albumType string, offset, limit int) (services.AlbumPage, error)
	Playlists(ctx context.Context, sort string, desc bool, offset, limit int) (services.PlaylistPage, error)
	ArtistIndex(ctx context.Context) ([]services.ArtistEntry, error)
	ArtistAlbums(ctx context.Context, id string) (services.ArtistAlbums, error)
	Artist(ctx context.Context, id string) (services.ArtistDetail, error)
	Album(ctx context.Context, id string) (services.AlbumDetail, error)
	Counts(ctx context.Context) (services.Counts, error)
	RecentlyAdded(ctx context.Context, offset, limit int) (services.LibraryEntryPage, error)
	FavoriteSongs(ctx context.Context, offset, limit int) (services.SongPage, error)
	MusicVideos(ctx context.Context, offset, limit int) (services.MusicVideoPage, error)
	History(ctx context.Context, days int) (services.ListeningHistory, error)
	Add(ctx context.Context, typ string, ids []string) error
	Sync() error
	SyncStatus(ctx context.Context) (services.SyncStatus, error)
	States(ctx context.Context, refs []services.ItemRef) ([]services.ItemState, error)
	Favorite(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
	SuggestLess(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
	Remove(ctx context.Context, r services.ItemRef) (services.ItemState, error)
	CopyLink(r services.ItemRef) (string, error)
}

// playbackAPI is what the pages ask of PlaybackService.
type playbackAPI interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
	PlayLibrary(ctx context.Context, sort string, desc bool, start int) error
	UndoReplace(ctx context.Context) (bool, error)
	SetShuffle(on bool)
}

// library is the feature's state.
type library struct {
	env  *module.Env
	lib  libraryAPI
	play playbackAPI

	// openAccount opens the account dialog (the Sign in action of an empty
	// library); nil without Env.Surfaces, which hides the action.
	openAccount func()

	sync syncStore

	albums    albumsPage
	songs     songsPage
	playlists playlistsPage
	recent    recentPage
	favourite favouritePage
	history   historyPage
	videos    videosPage
	artists   artistsPage
}

// New is the library's module.
func New(env *module.Env) module.Module {
	l := newLibrary(env,
		services.Service[services.LibraryService](env.Core),
		services.Service[services.PlaybackService](env.Core))
	return l.module()
}

func newLibrary(env *module.Env, lib libraryAPI, play playbackAPI) *library {
	l := &library{env: env, lib: lib, play: play}
	if env.Surfaces != nil {
		l.openAccount = env.Surfaces.OpenAccount
	}
	return l
}

// module is the Module of this library state.
func (l *library) module() module.Module {
	title := func(msg func() string) func(*ui.Route) string {
		return func(*ui.Route) string { return msg() }
	}
	route := func(pattern string, msg func() string, build func(*ui.Context, *ui.Route)) module.Route {
		return module.Route{Pattern: pattern, Title: title(msg), Build: build}
	}
	nav := func(msg func() string, icon, to string, order int) module.NavItem {
		return module.NavItem{Section: module.NavLibrary, Label: msg, Icon: icon, To: to, Order: order}
	}
	recently := func() string { return i18n.T("Recently added") }
	artists := func() string { return i18n.T("Artists") }
	albums := func() string { return i18n.T("Albums") }
	songs := func() string { return i18n.T("Songs") }
	videos := func() string { return i18n.T("Music videos") }
	favourite := func() string { return i18n.T("Favourite songs") }
	history := func() string { return i18n.T("History") }
	playlists := func() string { return i18n.T("Playlists") }
	return module.Module{
		ID: "library",
		Routes: []module.Route{
			route(recentPath, recently, l.recentPage),
			route(artistsPath, artists, l.artistsPage),
			route(albumsPath, albums, l.albumsPage),
			route(songsPath, songs, l.songsPage),
			route(videosPath, videos, l.videosPage),
			// Made for You is a sidebar section now; an old link (or a saved
			// last page) opens Home.
			{Pattern: madeForYou, Title: title(func() string { return "" }), Build: func(*ui.Context, *ui.Route) {
				l.env.Nav.Replace("/")
			}},
			route(favouritePath, favourite, l.favouritePage),
			route(historyPath, history, l.historyPage),
			route(playlistsPath, playlists, l.playlistsPage),
		},
		Nav: []module.NavItem{
			nav(recently, "clock-3", recentPath, 5),
			nav(artists, "mic-vocal", artistsPath, 10),
			nav(albums, "disc-3", albumsPath, 20),
			nav(songs, "music", songsPath, 30),
			nav(videos, "clapperboard", videosPath, 40),
			nav(favourite, "star", favouritePath, 60),
			nav(history, "history", historyPath, 65),
			nav(playlists, "list-music", playlistsPath, 70),
		},
		Actions: l.actions(),
		Slots:   module.Slots{SidebarFooter: l.syncIndicator},
		Init:    l.init,
	}
}

// Query keys, as v0.2.0's shared/lib/keys.ts: ["library", …].
var (
	keyLibrary    = store.Key{"library"}
	keyCounts     = store.Key{"library", "counts"}
	keySync       = store.Key{"library", "sync-status"}
	keyFavourites = store.Key{"library", "favorite-songs"}
	keyRecent     = store.Key{"library", "recent"}
)

// init follows what changes the library: a sync finishing a domain, a
// whole sync, (a rating changed outside the UI is Env.ItemStates').
func (l *library) init() {
	q := l.env.Queries
	store.On(l.env.Bus, services.EventSyncProgress, func(p services.SyncProgress) {
		l.sync.progress(p)
		if keys := syncInvalidations(p); len(keys) > 0 {
			q.Invalidate(keys...)
		}
	})
	store.On(l.env.Bus, services.EventSyncDone, func(d services.SyncDone) {
		l.sync.done(d)
		q.Invalidate(keyLibrary)
	})
}

// domainLists are the lists a sync domain's rows feed (v0.2.0's
// features/library/invalidations.ts).
var domainLists = map[string][]store.Key{
	"artists":         {{"library", "artists"}, {"library", "artist"}},
	"albums":          {{"library", "albums"}, {"library", "album"}, {"library", "recently-added"}},
	"songs":           {{"library", "songs"}, {"library", "album"}, {"library", "artist"}, {"library", "favorite-songs"}},
	"playlists":       {{"library", "playlists"}, {"library", "recently-added"}},
	"playlist-tracks": {{"library", "playlist"}},
	"ratings":         {{"library", "favorite-songs"}, {"library", "state"}},
}

// syncInvalidations are the queries a sync's progress makes stale: when a
// domain finishes, its lists and the counts, so the library fills in
// domain by domain instead of waiting for the whole sync. Ticks inside a
// domain only move the indicator.
func syncInvalidations(p services.SyncProgress) []store.Key {
	if !p.Finished {
		return nil
	}
	lists, ok := domainLists[p.Domain]
	if !ok {
		lists = []store.Key{keyLibrary}
	}
	return append(append([]store.Key(nil), lists...), keyCounts, keySync)
}

// failToast toasts what failed and why (shared/lib/run.ts).
func failToast(what string, err error) {
	kit.ToastErrorf(what, i18n.ErrorText(services.CodeOf(err)))
}
