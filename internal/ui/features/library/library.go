// Package library is the library feature. Phase 0 ports its Albums page:
// the grid of the library's albums, sorted and filtered by type, whose
// tiles play the album. The other library pages, the item actions and the
// sync indicator are ported with their features.
package library

import (
	"context"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// albumsPath is the Albums page's route.
const albumsPath = "/library/albums"

// libraryAPI is what the page asks of LibraryService.
type libraryAPI interface {
	Albums(ctx context.Context, sort string, desc bool, albumType string, offset, limit int) (services.AlbumPage, error)
	Album(ctx context.Context, id string) (services.AlbumDetail, error)
	Counts(ctx context.Context) (services.Counts, error)
	SyncStatus(ctx context.Context) (services.SyncStatus, error)
}

// playbackAPI is what a tile's play button asks of PlaybackService.
type playbackAPI interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
}

// authAPI is the sign-in state the empty library's text depends on.
type authAPI interface {
	Status() services.AuthStatus
}

// library is the feature's state.
type library struct {
	env  *module.Env
	lib  libraryAPI
	play playbackAPI
	auth authAPI

	// syncing follows sync:progress and sync:done; events counts them, so
	// a SyncStatus answer older than an event does not override it
	// (v0.2.0's useSync).
	syncing bool
	events  int

	albums albumsPage

	mu      sync.Mutex
	failed  []failure // play failures waiting to be shown
	nothing int       // "nothing here can be played" notices waiting
}

// failure is a command that failed.
type failure struct {
	what string
	code services.ErrorCode
}

// New is the library's module.
func New(env *module.Env) module.Module {
	l := newLibrary(env,
		services.Service[services.LibraryService](env.Core),
		services.Service[services.PlaybackService](env.Core),
		services.Service[services.AuthService](env.Core))
	return module.Module{
		ID: "library",
		Routes: []module.Route{{
			Pattern: albumsPath,
			Title:   func(*ui.Route) string { return i18n.T("Albums") },
			Build:   l.albumsPage,
		}},
		Nav: []module.NavItem{{
			Section: module.NavLibrary,
			Label:   func() string { return i18n.T("Albums") },
			Icon:    "disc-3",
			To:      albumsPath,
			Order:   20,
		}},
		Init: l.init,
	}
}

func newLibrary(env *module.Env, lib libraryAPI, play playbackAPI, auth authAPI) *library {
	return &library{env: env, lib: lib, play: play, auth: auth}
}

// Query keys, as v0.2.0's shared/lib/keys.ts: ["library", …].
var (
	keyLibrary = store.Key{"library"}
	keyCounts  = store.Key{"library", "counts"}
	keySync    = store.Key{"library", "sync-status"}
	keyAuth    = store.Key{"auth"}
)

// init follows what changes the library: a sync finishing a domain, a
// whole sync, a rating changed outside the UI (`sona like`), and the
// account.
func (l *library) init() {
	q := l.env.Queries
	store.On(l.env.Bus, services.EventSyncProgress, func(p services.SyncProgress) {
		l.syncing = true
		l.events++
		if keys := syncInvalidations(p); len(keys) > 0 {
			q.Invalidate(keys...)
		}
	})
	store.On(l.env.Bus, services.EventSyncDone, func(services.SyncDone) {
		l.syncing = false
		l.events++
		q.Invalidate(keyLibrary)
	})
	store.On(l.env.Bus, services.EventItemState, func(s services.ItemState) {
		store.Set(q, store.Key{"library", "state", s.Kind, s.ID}, s)
		q.Invalidate(store.Key{"library", "favorite-songs"})
	})
	store.On(l.env.Bus, services.EventAuthChanged, func(s services.AuthStatus) {
		store.Set(q, keyAuth, s)
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

// fail queues a failure for the view to toast, and redraws.
func (l *library) fail(what string, err error) {
	f := failure{what: what, code: services.CodeOf(err)}
	l.env.Post(func() {
		l.mu.Lock()
		l.failed = append(l.failed, f)
		l.mu.Unlock()
	})
}

// toasts shows what failed since the last frame.
func (l *library) toasts(c *ui.Context) {
	l.mu.Lock()
	failed, nothing := l.failed, l.nothing
	l.failed, l.nothing = nil, 0
	l.mu.Unlock()
	for _, f := range failed {
		c.AddToast(ui.Toast{ID: f.what, Title: f.what, Description: i18n.ErrorText(f.code), Type: "error"})
	}
	if nothing > 0 {
		c.AddToast(ui.Toast{
			ID:          "nothing-to-play",
			Title:       i18n.T("Nothing here can be played"),
			Description: i18n.T("None of these songs is available to stream."),
			Type:        "error",
		})
	}
}

// playAlbum replaces the queue with the album's songs and plays it from
// the first one (v0.2.0's playSongs over the album's tracks): the songs
// are read from the library, then PlaybackService starts the queue, both
// off the main thread.
func (l *library) playAlbum(a services.Album) {
	go func() {
		ctx := context.Background()
		detail, err := l.lib.Album(ctx, a.ID)
		if err != nil {
			l.fail(i18n.T("Couldn't load the album"), err)
			return
		}
		var refs []services.TrackRef
		for _, s := range detail.Tracks {
			if s.Playable {
				refs = append(refs, services.TrackRef{ID: s.ID, Source: s.Source})
			}
		}
		if len(refs) == 0 {
			l.env.Post(func() {
				l.mu.Lock()
				l.nothing++
				l.mu.Unlock()
			})
			return
		}
		if err := l.play.PlayItems(ctx, refs, 0, services.PlayFrom{Kind: "album", ID: a.ID}); err != nil {
			l.fail(i18n.T("Couldn't start playback"), err)
		}
	}()
}
