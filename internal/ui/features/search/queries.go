package search

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// Query keys, as v0.2.0's shared/lib/keys.ts.
var (
	// keyRecent is the recent searches: user data, cleared with the library.
	keyRecent = store.Key{"library", "recent-searches"}
	// keyBrowse is the browse categories of the empty search page.
	keyBrowse = store.Key{"catalog", "search-browse"}
	keyAuth   = store.Key{"auth"}
)

func keyCatalogSearch(term string) store.Key { return store.Key{"catalog", "search", term} }

func keyLibrarySearch(term string, perKind int) store.Key {
	return store.Key{"library", "search", term, perKind}
}

// keyGroup is one "see all" page of a group: v0.2.0's infinite query held
// the pages under one key; here each page is its own query below it, by the
// offset it was asked at.
func keyGroup(term, group string, offset int) store.Key {
	return store.Key{"catalog", "search", term, "group", group, offset}
}

func keyProfile(id string) store.Key { return store.Key{"catalog", "profile", id} }

// browseStale is how long the browse categories stay fresh.
const browseStale = 30 * time.Minute

// catalog is the Apple Music results for a term (v0.2.0's useCatalogSearch).
// The caller keeps the previous results on screen while the next load.
func (s *search) catalog(term string) store.Result[services.SearchResults] {
	return store.Use(s.env.Queries, keyCatalogSearch(term), func(ctx context.Context) (services.SearchResults, error) {
		return s.api.Catalog(ctx, term)
	})
}

// librarySearch is the library matches per kind (useLibrarySearch).
func (s *search) librarySearch(term string, perKind int) store.Result[services.LibrarySearch] {
	return store.Use(s.env.Queries, keyLibrarySearch(term, perKind), func(ctx context.Context) (services.LibrarySearch, error) {
		return s.api.Library(ctx, term, perKind)
	})
}

// groupPageAt is the "see all" page of group at offset (v0.2.0's
// useGroupPages, one query per page).
func (s *search) groupPageAt(term, group string, offset int) store.Result[services.SearchGroupPage] {
	return store.Use(s.env.Queries, keyGroup(term, group, offset), func(ctx context.Context) (services.SearchGroupPage, error) {
		return s.api.Group(ctx, term, group, offset)
	})
}

// browse is the browse categories.
func (s *search) browse() store.Result[[]services.Item] {
	return store.Use(s.env.Queries, keyBrowse, s.api.Browse, store.Options{Stale: browseStale})
}

// profile is a listener's profile page.
func (s *search) profile(id string) store.Result[services.ProfileDetail] {
	return store.Use(s.env.Queries, keyProfile(id), func(ctx context.Context) (services.ProfileDetail, error) {
		return s.api.Profile(ctx, id)
	})
}

// recent is the remembered searches and opened results, newest first.
func (s *search) recent() store.Result[[]services.RecentSearch] {
	return store.Use(s.env.Queries, keyRecent, s.api.Recent, store.Options{Stale: store.Forever})
}

// authStatus is the account state (v0.2.0's useAuth) and whether it is
// known yet.
func (s *search) authStatus() (state string, known bool) {
	r := store.Use(s.env.Queries, keyAuth, func(context.Context) (services.AuthStatus, error) {
		return s.auth.Status(), nil
	})
	return r.Data.State, r.HasData
}

// hasAccount is the states in which the library and personal endpoints are
// usable (v0.2.0's hasAccount).
func hasAccount(state string) bool { return state == "signed_in" || state == "validating" }

// libraryCards is the library search results as items, per kind
// (v0.2.0's libraryCards).
func libraryCards(r services.LibrarySearch) (artists, albums, playlists, songs []services.Item) {
	for _, a := range r.Artists {
		artists = append(artists, services.Item{Kind: services.ItemArtist, ID: a.ID, Name: a.Name, Subtitle: a.Genre, Art: a.Art, Artist: &a})
	}
	for _, a := range r.Albums {
		albums = append(albums, services.Item{Kind: services.ItemAlbum, ID: a.ID, Name: a.Name, Subtitle: a.Artist, Art: a.Art, Album: &a})
	}
	for _, p := range r.Playlists {
		playlists = append(playlists, services.Item{Kind: services.ItemPlaylist, ID: p.ID, Name: p.Name, Subtitle: p.Curator, Art: p.Art, Playlist: &p})
	}
	for _, s := range r.Songs {
		songs = append(songs, services.Item{Kind: services.ItemSong, ID: s.ID, Name: s.Name, Subtitle: s.Artist, Art: s.Art, Song: &s})
	}
	return
}

// itemSource is the Source of an item's ID: library items come from the
// local index.
func itemSource(it services.Item) services.Source {
	var src services.Source
	switch {
	case it.Song != nil:
		src = it.Song.Source
	case it.Album != nil:
		src = it.Album.Source
	case it.Playlist != nil:
		src = it.Playlist.Source
	case it.Artist != nil:
		src = it.Artist.Source
	}
	if src == services.SourceLibrary {
		return services.SourceLibrary
	}
	return services.SourceCatalog
}

// refreshRecent makes the recent list ask again.
func (s *search) refreshRecent() { s.env.Queries.Invalidate(keyRecent) }

// rememberTerm remembers term as searched in scope. Best effort, off the
// main thread: a failed write never blocks opening a result.
func (s *search) rememberTerm(term, scope string) {
	if term == "" {
		return
	}
	go func() {
		if err := s.api.RememberTerm(context.Background(), term, scope); err == nil {
			s.refreshRecent()
		}
	}()
}

// rememberItem remembers an opened result. Best effort. A library song has
// no page to return to, so it is never remembered.
func (s *search) rememberItem(it services.Item, source services.Source) {
	if it.Kind == services.ItemSong && source == services.SourceLibrary {
		return
	}
	entry := services.RecentItem{
		Kind: it.Kind, ID: it.ID, Source: source,
		Name: it.Name, Subtitle: it.Subtitle, Art: it.Art.Small,
	}
	go func() {
		if err := s.api.RememberItem(context.Background(), entry); err == nil {
			s.refreshRecent()
		}
	}()
}

// forget removes a remembered entry; a failure toasts.
func (s *search) forget(key string) {
	s.runRecent(i18n.T("Couldn't remove the search"), func(ctx context.Context) error {
		return s.api.ForgetRecent(ctx, key)
	})
}

// clearRecent removes every remembered entry; a failure toasts.
func (s *search) clearRecent() {
	s.runRecent(i18n.T("Couldn't clear recent searches"), s.api.ClearRecent)
}

// runRecent runs a write off the main thread, refreshes the list after it,
// and toasts what failed (v0.2.0's run).
func (s *search) runRecent(what string, call func(context.Context) error) {
	go func() {
		err := call(context.Background())
		if err != nil {
			kit.ToastErrorf(what, i18n.ErrorText(services.CodeOf(err)))
		}
		s.refreshRecent()
	}()
}

// playSong plays a song result as a queue of one (the field's suggestions
// have no list to play from) named after the song, as v0.2.0's playItem
// does. Failures toast.
func (s *search) playSong(it services.Item) {
	if it.Song == nil {
		return
	}
	s.playSongs([]services.Song{*it.Song}, 0, services.PlayFrom{ID: it.Name})
}

// searchFrom is where songs played from search say they come from (v0.2.0's
// playSongs(songs, i, "search")).
var searchFrom = services.PlayFrom{ID: "search"}

// playSongs plays songs from start as the queue, from where they came: the
// playable ones are queued and start lands among them. Nothing playable
// toasts; a failed start toasts. It is the one seam to the queue commands:
// drop for env.Queue.PlaySongs when player-bar-queue merges.
func (s *search) playSongs(songs []services.Song, start int, from services.PlayFrom) {
	var refs []services.TrackRef
	index := 0
	for i, so := range songs {
		if !so.Playable {
			continue
		}
		if i == start {
			index = len(refs)
		}
		refs = append(refs, services.TrackRef{ID: so.ID, Source: so.Source})
	}
	if len(refs) == 0 {
		kit.ToastErrorf(i18n.T("Nothing here can be played"), i18n.T("None of these songs is available to stream."))
		return
	}
	go func() {
		if err := s.play.PlayItems(context.Background(), refs, index, from); err != nil {
			kit.ToastErrorf(i18n.T("Couldn't start playback"), i18n.ErrorText(services.CodeOf(err)))
		}
	}()
}
