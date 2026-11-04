// Package artist is the artist feature: an artist's page (library or
// catalogue) with its see-all lists, and "Go to artist" on song rows. It is
// v0.2.0's features/artist.
package artist

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

const (
	artistPattern = "/artist/{id}"
	viewPattern   = "/artist/{id}/view/{view}"
	// artistsPath is the library's artists page, the error pages' way back.
	artistsPath = "/library/artists"
	// viewPage is the page size of a see-all list.
	viewPage = 50
)

// libraryAPI is what the page asks of LibraryService.
type libraryAPI interface {
	Artist(ctx context.Context, id string) (services.ArtistDetail, error)
}

// catalogAPI is what the page asks of CatalogService.
type catalogAPI interface {
	Artist(ctx context.Context, id string) (services.CatalogArtist, error)
	ArtistView(ctx context.Context, id, view string, offset, limit int) (services.ArtistViewPage, error)
}

// authAPI is the account state the explicit-content rule depends on.
type authAPI interface {
	Status() services.AuthStatus
}

// commands is what plays collections: kit.QueueCommands, Env.Queue once it
// lands (queue.go is its stand-in).
type commands interface {
	PlaySongs(songs []services.Song, start int, from services.PlayFrom)
	ShuffleSongs(songs []services.Song, from services.PlayFrom)
}

// ratings draws an item's Favourite and Suggest Less buttons over its state
// (*kit.ItemStates).
type ratings interface {
	RatingButtons(c *ui.Context, kind string, it kit.StateItem, name string)
}

// artist is the feature's state.
type artist struct {
	env    *module.Env
	lib    libraryAPI
	cat    catalogAPI
	auth   authAPI
	queue  commands
	states ratings
	// now is the clock an upcoming release is compared with.
	now func() time.Time

	page pageState
	view viewState
}

// New is the artist module.
func New(env *module.Env) module.Module {
	system := services.Service[services.SystemService](env.Core)
	svc := services.Service[services.PlaybackService](env.Core)
	states := kit.NewItemStates(env.Queries, env.Post, services.Service[services.LibraryService](env.Core))
	a := newArtist(env,
		services.Service[services.LibraryService](env.Core),
		services.Service[services.CatalogService](env.Core),
		services.Service[services.AuthService](env.Core),
		&queueCommands{svc: svc, play: env.Play, post: env.Post, mode: func() string { return system.Settings().Queue.ReplaceConfirm }},
		states)
	return module.Module{
		ID: "artist",
		Routes: []module.Route{
			{Pattern: artistPattern, Title: func(*ui.Route) string { return i18n.T("Artist") }, Build: a.artistPage},
			{Pattern: viewPattern, Title: a.viewRouteTitle, Build: a.viewPage},
		},
		Actions: []kit.ActionDef{goToArtist(env)},
		Init:    func() { a.init(); states.Follow(env.Bus) },
	}
}

func newArtist(env *module.Env, lib libraryAPI, cat catalogAPI, auth authAPI, queue commands, states ratings) *artist {
	return &artist{env: env, lib: lib, cat: cat, auth: auth, queue: queue, states: states, now: time.Now}
}

// goToArtist is the song row's "Go to artist" (id artist.go-to, group item,
// order 20), listed only for a song that names its artist.
func goToArtist(env *module.Env) kit.ActionDef {
	return kit.ActionDef{
		ID:     "artist.go-to",
		Target: kit.KindSong,
		Label:  func(*kit.ActionContext) string { return i18n.T("Go to artist") },
		Icon:   "mic-vocal",
		Group:  kit.GroupItem,
		Order:  20,
		Available: func(a *kit.ActionContext) bool {
			return a.Item().Song().ArtistID != ""
		},
		Run: func(a *kit.ActionContext) {
			a.Navigate(artistPath(a.Item().Song().ArtistID))
		},
	}
}

// artistPath is an artist's page.
func artistPath(id string) string { return "/artist/" + url.PathEscape(id) }

// viewPath is an artist view's see-all page.
func viewPath(id, view string) string { return artistPath(id) + "/view/" + url.PathEscape(view) }

// Query keys, v0.2.0's shared/lib/keys.ts.
func keyLibraryArtist(id string) store.Key { return store.Key{"library", "artist", id} }
func keyCatalogArtist(id string) store.Key { return store.Key{"catalog", "artist", id} }
func keyArtistView(id, view string) store.Key {
	return store.Key{"catalog", "artist", id, "view", view}
}

var keyAuth = store.Key{"auth"}

// isLibraryID: library artist IDs start "r."; anything else is catalogue.
func isLibraryID(id string) bool { return strings.HasPrefix(id, "r.") }

// init follows the account: a restricted one cannot play explicit releases.
// (v0.2.0's artist module has no event handlers of its own: the library's
// sync invalidations reach ["library","artist"].)
func (a *artist) init() {
	store.On(a.env.Bus, services.EventAuthChanged, func(s services.AuthStatus) {
		store.Set(a.env.Queries, keyAuth, s)
	})
}

// restricted reports whether the account may not play explicit items.
func (a *artist) restricted() bool {
	r := store.Use(a.env.Queries, keyAuth, func(context.Context) (services.AuthStatus, error) {
		return a.auth.Status(), nil
	}, store.Options{Stale: store.Forever})
	return r.HasData && r.Data.Restricted
}

// has reports whether a page of the app exists: links go only to those.
func (a *artist) has(path string) bool { return a.env.HasRoute != nil && a.env.HasRoute(path) }

// open opens a page of the app, when it exists.
func (a *artist) open(path string) {
	if a.has(path) {
		a.env.Nav.Push(path)
	}
}
