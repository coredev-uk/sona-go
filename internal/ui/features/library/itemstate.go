package library

import (
	"context"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// Stand-in for kit.ItemStates (album-artist-song PR); drop at rebase. The
// names and semantics are that type's (kit/song_itemstate.go): v0.2.0's
// shared/lib/item-state.ts. States load in batches (every load within
// ~16 ms is one LibraryService.States call), changes show at once and are
// put back when they fail. The commands with their toasts are in
// libactions.go.
//
// Differences from the kit type, for the rebase: api is read at call time
// (a test wraps l.lib after the module is built), and there is no Use (the
// pages of this feature read no state in a view).
//
// Load answers from what the helper last learnt (a load or a change, fresh
// for a minute) or asks Go; store.Client has no read outside a view, so the
// cache is this mirror, and every answer is also written to the store with
// store.Set so the views of the item show it. A rating changed outside the
// UI (EventItemState) reaches the store only: its menu shows the old one
// until the minute passes.

// itemStateService is what the item state helpers ask of LibraryService.
type itemStateService interface {
	States(ctx context.Context, refs []services.ItemRef) ([]services.ItemState, error)
	Favorite(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
	SuggestLess(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
	Remove(ctx context.Context, r services.ItemRef) (services.ItemState, error)
	Add(ctx context.Context, typ string, ids []string) error
}

// State kinds, Apple's catalogue types (v0.2.0's StateKind).
const (
	stateSongs       = "songs"
	stateAlbums      = "albums"
	stateArtists     = "artists"
	statePlaylists   = "playlists"
	stateMusicVideos = "music-videos"
)

// stateItem is what the item DTOs (Song, Album, Artist, Playlist,
// MusicVideo) share.
type stateItem struct {
	ID        string
	Source    services.Source
	CatalogID string
	LibraryID string
}

func stateOfSong(s services.Song) stateItem {
	return stateItem{s.ID, s.Source, s.CatalogID, s.LibraryID}
}

func stateOfAlbum(a services.Album) stateItem {
	return stateItem{a.ID, a.Source, a.CatalogID, a.LibraryID}
}

func stateOfVideo(v services.MusicVideo) stateItem {
	return stateItem{v.ID, v.Source, v.CatalogID, v.LibraryID}
}

func stateOfArtist(a services.Artist) stateItem {
	return stateItem{a.ID, a.Source, a.CatalogID, a.LibraryID}
}

func stateOfPlaylist(p services.Playlist) stateItem {
	return stateItem{p.ID, p.Source, p.CatalogID, p.LibraryID}
}

// stateOfItem is the state kind and item of a menu's item; ok is false for
// kinds with no library state here (stations).
func stateOfItem(it kit.Item) (kind string, si stateItem, ok bool) {
	switch it.Kind {
	case kit.KindSong:
		return stateSongs, stateOfSong(it.Song()), true
	case kit.KindAlbum:
		return stateAlbums, stateOfAlbum(it.Album()), true
	case kit.KindArtist:
		return stateArtists, stateOfArtist(it.Artist()), true
	case kit.KindPlaylist:
		return statePlaylists, stateOfPlaylist(it.Playlist()), true
	case kit.KindMusicVideo:
		return stateMusicVideos, stateOfVideo(it.MusicVideo()), true
	}
	return "", stateItem{}, false
}

// itemRefOf is the ItemRef LibraryService takes for an item.
func itemRefOf(kind string, it stateItem) services.ItemRef {
	return services.ItemRef{Kind: kind, ID: it.ID, Source: it.Source, CatalogID: it.CatalogID, LibraryID: it.LibraryID}
}

// membershipLists are the library lists to refresh when an item joins or
// leaves the library (v0.2.0's item-state.ts).
var membershipLists = []store.Key{
	keyCounts,
	{"library", "songs"},
	{"library", "albums"},
	{"library", "artists"},
	{"library", "album"},
	{"library", "artist"},
	{"library", "recently-added"},
	keyFavourites,
}

// stateStale is how long a state is fresh: Go caches Apple's answers for
// two minutes, the views refresh after one.
const stateStale = time.Minute

// stateBatchWait is how long loads gather into one States call.
const stateBatchWait = 16 * time.Millisecond

type stateAnswer struct {
	st  services.ItemState
	err error
}

type statePending struct {
	ref services.ItemRef
	ch  chan stateAnswer
}

// knownState is a state and when it was learnt.
type knownState struct {
	st services.ItemState
	at time.Time
}

// itemStates is the library state and rating of the items a menu acts on,
// and the commands that change them.
type itemStates struct {
	q   *store.Client
	api func() itemStateService

	mu        sync.Mutex
	queue     []statePending
	scheduled bool
	// last is the state each item was last known in (loaded or changed),
	// under mu: the base of a change and what Load answers from.
	last map[string]knownState
}

func newItemStates(q *store.Client, api func() itemStateService) *itemStates {
	return &itemStates{q: q, api: api, last: map[string]knownState{}}
}

// load asks for one state, batched with every other made soon.
func (s *itemStates) load(ctx context.Context, ref services.ItemRef) (services.ItemState, error) {
	ch := make(chan stateAnswer, 1)
	s.mu.Lock()
	s.queue = append(s.queue, statePending{ref, ch})
	if !s.scheduled {
		s.scheduled = true
		time.AfterFunc(stateBatchWait, s.flush)
	}
	s.mu.Unlock()
	select {
	case a := <-ch:
		return a.st, a.err
	case <-ctx.Done():
		return services.ItemState{}, ctx.Err()
	}
}

func (s *itemStates) flush() {
	s.mu.Lock()
	batch := s.queue
	s.queue, s.scheduled = nil, false
	s.mu.Unlock()
	refs := make([]services.ItemRef, len(batch))
	for i, p := range batch {
		refs[i] = p.ref
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	states, err := s.api().States(ctx, refs)
	for i, p := range batch {
		switch {
		case err != nil:
			p.ch <- stateAnswer{err: err}
		case i >= len(states):
			p.ch <- stateAnswer{err: context.DeadlineExceeded}
		default:
			p.ch <- stateAnswer{st: states[i]}
		}
	}
}

func stateID(kind, id string) string { return kind + "/" + id }

func (s *itemStates) remember(kind string, st services.ItemState) {
	s.mu.Lock()
	s.last[stateID(kind, st.ID)] = knownState{st, time.Now()}
	s.mu.Unlock()
}

func (s *itemStates) known(kind, id string) (services.ItemState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.last[stateID(kind, id)]
	return k.st, ok && time.Since(k.at) < stateStale
}

// base is the state a change starts from: the last one learnt, however old,
// else what the item itself says.
func (s *itemStates) base(kind string, it stateItem) (st services.ItemState, learnt bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k, ok := s.last[stateID(kind, it.ID)]; ok {
		return k.st, true
	}
	return services.ItemState{Kind: kind, ID: it.ID, InLibrary: it.Source == services.SourceLibrary, LibraryID: it.LibraryID}, false
}

func (s *itemStates) forget(kind, id string) {
	s.mu.Lock()
	delete(s.last, stateID(kind, id))
	s.mu.Unlock()
}

// Load is an item's state for a command (a menu): the one last known, else
// asked of Go, batched with every other load of the frame. ok is false when
// it cannot be had. Safe from any goroutine.
func (s *itemStates) Load(ctx context.Context, kind string, it stateItem) (services.ItemState, bool) {
	if it.ID == "" {
		return services.ItemState{}, false
	}
	if st, ok := s.known(kind, it.ID); ok {
		return st, true
	}
	st, err := s.load(ctx, itemRefOf(kind, it))
	if err != nil {
		return services.ItemState{}, false
	}
	s.remember(kind, st)
	store.Set(s.q, itemStateKey(kind, it.ID), st)
	return st, true
}

// stateChange is a command that changes an item's state: a rating (the
// kit.RatingAction values) or its removal from the library.
type stateChange string

// changeRemove is the change besides ratings.
const changeRemove stateChange = "remove"

func ratingChange(a kit.RatingAction) stateChange { return stateChange(a) }

// Change changes an item's rating or removes it from the library: the new
// state shows at once, Go's answer replaces it, and a failure restores the
// old state and returns the error (no toast: Rate and the bulk commands say
// what failed). The lists a membership change touches are refreshed. Safe
// from any goroutine; it blocks for the call.
func (s *itemStates) Change(ctx context.Context, kind string, it stateItem, ch stateChange) (services.ItemState, error) {
	key := itemStateKey(kind, it.ID)
	prev, learnt := s.base(kind, it)
	next := prev
	switch kit.RatingAction(ch) {
	case kit.RateFavorite:
		next.Rating = 1
	case kit.RateSuggestLess:
		next.Rating = -1
	case kit.RateUnfavorite, kit.RateUndoSuggestLess:
		next.Rating = 0
	default:
		next.InLibrary, next.LibraryID = false, ""
	}
	s.remember(kind, next)
	store.Set(s.q, key, next)
	ref := itemRefOf(kind, it)
	api := s.api()
	var got services.ItemState
	var err error
	switch kit.RatingAction(ch) {
	case kit.RateFavorite, kit.RateUnfavorite:
		got, err = api.Favorite(ctx, ref, ch == stateChange(kit.RateFavorite))
	case kit.RateSuggestLess, kit.RateUndoSuggestLess:
		got, err = api.SuggestLess(ctx, ref, ch == stateChange(kit.RateSuggestLess))
	default:
		got, err = api.Remove(ctx, ref)
	}
	if err != nil {
		if learnt {
			s.remember(kind, prev)
			store.Set(s.q, key, prev)
		} else {
			s.forget(kind, it.ID)
			s.q.Invalidate(key)
		}
		return services.ItemState{}, err
	}
	s.remember(kind, got)
	store.Set(s.q, key, got)
	switch {
	case got.InLibrary != prev.InLibrary || ch == changeRemove:
		s.q.Invalidate(membershipLists...)
	case kind == stateSongs:
		s.q.Invalidate(keyFavourites)
	}
	return got, nil
}

// Add adds catalogue items of one kind to the library (LibraryService.Add
// with each item's catalogue ID, else its ID), marks them in the library and
// refreshes the lists. Safe from any goroutine; it blocks for the call.
func (s *itemStates) Add(ctx context.Context, kind string, items []stateItem) error {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = firstID(it.CatalogID, it.ID)
	}
	if err := s.api().Add(ctx, kind, ids); err != nil {
		return err
	}
	for _, it := range items {
		if k, ok := s.known(kind, it.ID); ok {
			k.InLibrary = true
			s.remember(kind, k)
			store.Set(s.q, itemStateKey(kind, it.ID), k)
		}
		s.q.Invalidate(itemStateKey(kind, it.ID))
	}
	s.q.Invalidate(membershipLists...)
	return nil
}

func firstID(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
