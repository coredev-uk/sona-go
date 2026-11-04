package kit

import (
	"context"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// ItemStateService is what the item state helpers ask of LibraryService
// (*services.LibraryService implements it).
type ItemStateService interface {
	States(ctx context.Context, refs []services.ItemRef) ([]services.ItemState, error)
	Favorite(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
	SuggestLess(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
	Remove(ctx context.Context, r services.ItemRef) (services.ItemState, error)
	Add(ctx context.Context, typ string, ids []string) error
}

// State kinds, Apple's catalogue types (v0.2.0's StateKind).
const (
	StateSongs       = "songs"
	StateAlbums      = "albums"
	StateArtists     = "artists"
	StatePlaylists   = "playlists"
	StateMusicVideos = "music-videos"
)

// StateItem is what the item DTOs (Song, Album, Artist, Playlist,
// MusicVideo) share.
type StateItem struct {
	ID        string
	Source    services.Source
	CatalogID string
	LibraryID string
}

// StateOfSong, StateOfAlbum and StateOfVideo name the item of a DTO.
func StateOfSong(s services.Song) StateItem {
	return StateItem{s.ID, s.Source, s.CatalogID, s.LibraryID}
}

// StateOfAlbum is the StateItem of an album.
func StateOfAlbum(a services.Album) StateItem {
	return StateItem{a.ID, a.Source, a.CatalogID, a.LibraryID}
}

// StateOfVideo is the StateItem of a music video.
func StateOfVideo(v services.MusicVideo) StateItem {
	return StateItem{v.ID, v.Source, v.CatalogID, v.LibraryID}
}

// ItemRefOf is the ItemRef LibraryService takes for an item.
func ItemRefOf(kind string, it StateItem) services.ItemRef {
	return services.ItemRef{Kind: kind, ID: it.ID, Source: it.Source, CatalogID: it.CatalogID, LibraryID: it.LibraryID}
}

// StateKey is the query key of an item's state (v0.2.0's keys.itemState):
// ["library","state",kind,id].
func StateKey(kind, id string) store.Key { return store.Key{"library", "state", kind, id} }

// membershipLists are the library lists to refresh when an item joins or
// leaves the library (v0.2.0's item-state.ts).
var membershipLists = []store.Key{
	{"library", "counts"},
	{"library", "songs"},
	{"library", "albums"},
	{"library", "artists"},
	{"library", "album"},
	{"library", "artist"},
	{"library", "recently-added"},
	{"library", "favorite-songs"},
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

// ItemStates is the library state and rating of the items a page shows, and
// the commands that change them (v0.2.0's shared/lib/item-state.ts and
// library-actions.ts): states load in batches (every load within a frame
// is one LibraryService.States call), changes show at once and are put
// back when they fail, and a toast says what failed. One per feature
// factory; Use, the buttons and the commands run on the main thread.
type ItemStates struct {
	q    *store.Client
	api  ItemStateService
	post store.Poster

	mu        sync.Mutex
	queue     []statePending
	scheduled bool

	// last is the state each item was last known in (loaded or changed),
	// under mu: the base of a change and what Load answers from.
	last map[string]knownState
}

// knownState is a state and when it was learnt.
type knownState struct {
	st services.ItemState
	at time.Time
}

// NewItemStates is the item states over a query client and LibraryService.
func NewItemStates(q *store.Client, post store.Poster, api ItemStateService) *ItemStates {
	return &ItemStates{q: q, api: api, post: post, last: map[string]knownState{}}
}

// load asks for one state, batched with every other made soon.
func (s *ItemStates) load(ctx context.Context, ref services.ItemRef) (services.ItemState, error) {
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

func (s *ItemStates) flush() {
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
	states, err := s.api.States(ctx, refs)
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

func (s *ItemStates) remember(kind string, st services.ItemState) {
	s.mu.Lock()
	s.last[stateID(kind, st.ID)] = knownState{st, time.Now()}
	s.mu.Unlock()
}

func (s *ItemStates) known(kind, id string) (services.ItemState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.last[stateID(kind, id)]
	return k.st, ok && time.Since(k.at) < stateStale
}

// base is the state a change starts from: the last one learnt, however old,
// else what the item itself says.
func (s *ItemStates) base(kind string, it StateItem) (st services.ItemState, learnt bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k, ok := s.last[stateID(kind, it.ID)]; ok {
		return k.st, true
	}
	return services.ItemState{Kind: kind, ID: it.ID, InLibrary: it.Source == services.SourceLibrary, LibraryID: it.LibraryID}, false
}

func (s *ItemStates) forget(kind, id string) {
	s.mu.Lock()
	delete(s.last, stateID(kind, id))
	s.mu.Unlock()
}

// StateOfArtist and StateOfPlaylist are the StateItem of those DTOs.
func StateOfArtist(a services.Artist) StateItem {
	return StateItem{a.ID, a.Source, a.CatalogID, a.LibraryID}
}

// StateOfPlaylist is the StateItem of a playlist.
func StateOfPlaylist(p services.Playlist) StateItem {
	return StateItem{p.ID, p.Source, p.CatalogID, p.LibraryID}
}

// Load is an item's state for a command (a menu): the one last known, else
// asked of Go, batched with every other load of the frame. ok is false when
// it cannot be had. Safe from any goroutine.
func (s *ItemStates) Load(ctx context.Context, kind string, it StateItem) (services.ItemState, bool) {
	if it.ID == "" {
		return services.ItemState{}, false
	}
	if st, ok := s.known(kind, it.ID); ok {
		return st, true
	}
	st, err := s.load(ctx, ItemRefOf(kind, it))
	if err != nil {
		return services.ItemState{}, false
	}
	s.remember(kind, st)
	store.Set(s.q, StateKey(kind, it.ID), st)
	return st, true
}

// Use is an item's library state and rating; no data while it loads, and
// never loaded for an item without an ID.
func (s *ItemStates) Use(kind string, it StateItem) store.Result[services.ItemState] {
	if it.ID == "" {
		return store.Result[services.ItemState]{}
	}
	ref := ItemRefOf(kind, it)
	r := store.Use(s.q, StateKey(kind, it.ID), func(ctx context.Context) (services.ItemState, error) {
		return s.load(ctx, ref)
	}, store.Options{Stale: stateStale})
	if r.HasData {
		s.remember(kind, r.Data)
	}
	return r
}

// failed toasts what failed and why.
func failedToast(what string, err error) {
	ToastErrorf(what, i18n.ErrorMessage(err))
}

// StateChange is a command that changes an item's state: a rating (the
// RatingAction values) or its removal from the library.
type StateChange string

// The changes besides ratings.
const ChangeRemove StateChange = "remove"

// ratingChange is the StateChange of a RatingAction.
func ratingChange(a RatingAction) StateChange { return StateChange(a) }

// ratingFailure is the toast of a failed change of subject.
func ratingFailure(ch StateChange, subject string) string {
	switch RatingAction(ch) {
	case RateFavorite:
		return i18n.T("Couldn't favourite {subject}", "subject", subject)
	case RateUnfavorite:
		return i18n.T("Couldn't undo favourite for {subject}", "subject", subject)
	case RateSuggestLess:
		return i18n.T("Couldn't suggest less of {subject}", "subject", subject)
	case RateUndoSuggestLess:
		return i18n.T("Couldn't undo suggest less for {subject}", "subject", subject)
	}
	return i18n.T("Couldn't remove {subject}", "subject", subject)
}

// Change changes an item's rating or removes it from the library: the new
// state shows at once, Go's answer replaces it, and a failure restores the
// old state and returns the error (no toast: Rate and the bulk commands
// say what failed). The lists a membership change touches are refreshed.
// Safe from any goroutine; it blocks for the call.
func (s *ItemStates) Change(ctx context.Context, kind string, it StateItem, ch StateChange) (services.ItemState, error) {
	key := StateKey(kind, it.ID)
	prev, learnt := s.base(kind, it)
	next := prev
	switch RatingAction(ch) {
	case RateFavorite:
		next.Rating = 1
	case RateSuggestLess:
		next.Rating = -1
	case RateUnfavorite, RateUndoSuggestLess:
		next.Rating = 0
	default:
		next.InLibrary, next.LibraryID = false, ""
	}
	s.remember(kind, next)
	store.Set(s.q, key, next)
	ref := ItemRefOf(kind, it)
	var got services.ItemState
	var err error
	switch RatingAction(ch) {
	case RateFavorite, RateUnfavorite:
		got, err = s.api.Favorite(ctx, ref, ch == StateChange(RateFavorite))
	case RateSuggestLess, RateUndoSuggestLess:
		got, err = s.api.SuggestLess(ctx, ref, ch == StateChange(RateSuggestLess))
	default:
		got, err = s.api.Remove(ctx, ref)
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
	case got.InLibrary != prev.InLibrary || ch == ChangeRemove:
		s.q.Invalidate(membershipLists...)
	case kind == StateSongs:
		s.q.Invalidate(store.Key{"library", "favorite-songs"})
	}
	return got, nil
}

// Rate changes an item's rating from the main thread, with a toast naming
// what failed.
func (s *ItemStates) Rate(kind string, it StateItem, name string, a RatingAction) {
	if a == RateNone || it.ID == "" {
		return
	}
	go func() {
		if _, err := s.Change(context.Background(), kind, it, ratingChange(a)); err != nil {
			failedToast(ratingFailure(ratingChange(a), i18n.T("“{name}”", "name", name)), err)
		}
	}()
}

// KindedItem is one item of a bulk command, with its state kind.
type KindedItem struct {
	Kind string
	Item StateItem
}

// RateMany changes the rating of several items at once (each shows at
// once); one toast names how many failed, what being the items in words
// ("12 songs").
func (s *ItemStates) RateMany(items []KindedItem, a RatingAction, what string) {
	if a == RateNone || len(items) == 0 {
		return
	}
	go func() {
		var wg sync.WaitGroup
		errs := make([]error, len(items))
		for i, it := range items {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = s.Change(context.Background(), it.Kind, it.Item, ratingChange(a))
			}()
		}
		wg.Wait()
		failed := 0
		var first error
		for _, err := range errs {
			if err != nil {
				failed++
				if first == nil {
					first = err
				}
			}
		}
		if failed == 0 {
			return
		}
		subject := what
		if failed != len(items) {
			subject = i18n.T("{count} of {what}", "count", failed, "what", what)
		}
		failedToast(ratingFailure(ratingChange(a), subject), first)
	}()
}

// Add adds a catalogue item to the library, with a toast either way.
func (s *ItemStates) Add(kind string, it StateItem, name string) {
	s.AddMany([]KindedItem{{kind, it}}, i18n.T("“{name}”", "name", name))
}

// AddMany adds catalogue items of several kinds to the library, one call
// per kind, with one toast naming them (what: "“Airbag”", "3 songs").
func (s *ItemStates) AddMany(items []KindedItem, what string) {
	if len(items) == 0 {
		return
	}
	go func() {
		byKind := map[string][]StateItem{}
		var kinds []string
		for _, it := range items {
			if _, ok := byKind[it.Kind]; !ok {
				kinds = append(kinds, it.Kind)
			}
			byKind[it.Kind] = append(byKind[it.Kind], it.Item)
		}
		for _, kind := range kinds {
			list := byKind[kind]
			ids := make([]string, len(list))
			for i, it := range list {
				ids[i] = firstID(it.CatalogID, it.ID)
			}
			if err := s.api.Add(context.Background(), kind, ids); err != nil {
				failedToast(i18n.T("Couldn't add {what} to your library", "what", what), err)
				return
			}
			for _, it := range list {
				if k, ok := s.known(kind, it.ID); ok {
					k.InLibrary = true
					s.remember(kind, k)
					store.Set(s.q, StateKey(kind, it.ID), k)
				}
				s.q.Invalidate(StateKey(kind, it.ID))
			}
		}
		s.q.Invalidate(membershipLists...)
		ToastSuccessf(i18n.T("Added {what} to your library", "what", what), "")
	}()
}

func firstID(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// removeNote is what else a removal takes with it.
func removeNote(kind string) string {
	switch kind {
	case StateAlbums:
		return i18n.T("Its songs leave your library and any playlists they're in.")
	case StateArtists:
		return i18n.T("Their albums and songs leave your library too.")
	default:
		return i18n.T("It also leaves any playlists it's in.")
	}
}

// Remove asks, then deletes an item from the library, with a toast either
// way. The question is Confirm's: the shell hosts it. Main thread only.
func (s *ItemStates) Remove(kind string, it StateItem, name string) {
	Confirm(ConfirmOptions{
		Title:        i18n.T("Delete “{name}” from your library?", "name", name),
		Body:         removeNote(kind),
		ConfirmLabel: i18n.T("Delete"),
		Destructive:  true,
	}, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			if _, err := s.Change(context.Background(), kind, it, ChangeRemove); err != nil {
				failedToast(i18n.T("Couldn't delete “{name}”", "name", name), err)
				return
			}
			ToastSuccessf(i18n.T("Deleted “{name}” from your library", "name", name), "")
		}()
	})
}

// RemoveMany asks once, then deletes several items from the library, with
// one toast (what: "3 songs").
func (s *ItemStates) RemoveMany(items []KindedItem, what string) {
	if len(items) == 0 {
		return
	}
	Confirm(ConfirmOptions{
		Title:        i18n.T("Delete {what} from your library?", "what", what),
		Body:         i18n.T("They also leave any playlists they're in."),
		ConfirmLabel: i18n.T("Delete"),
		Destructive:  true,
	}, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			var wg sync.WaitGroup
			errs := make([]error, len(items))
			for i, it := range items {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, errs[i] = s.Change(context.Background(), it.Kind, it.Item, ChangeRemove)
				}()
			}
			wg.Wait()
			for _, err := range errs {
				if err != nil {
					failedToast(i18n.T("Couldn't delete {what}", "what", what), err)
					return
				}
			}
			ToastSuccessf(i18n.T("Deleted {what} from your library", "what", what), "")
		}()
	})
}

// LibraryButton is the header's library control of an item: Add, or "In
// library" which asks to delete (v0.2.0's LibraryButton over useItemState).
// addLabel replaces "Add" ("Pre-add").
func (s *ItemStates) LibraryButton(c *ui.Context, kind string, it StateItem, name, addLabel string) {
	st := s.Use(kind, it)
	in := it.Source == services.SourceLibrary
	if st.HasData {
		in = st.Data.InLibrary
	}
	switch LibraryButton(c, LibraryOpts{
		Name: name, InLibrary: in, Pending: !st.HasData, AddLabel: addLabel, Playlist: kind == StatePlaylists,
	}) {
	case LibraryAdd:
		s.Add(kind, it, name)
	case LibraryRemove:
		s.Remove(kind, it, name)
	}
}

// RatingButtons is the header's Favourite and Suggest Less of an item
// (v0.2.0's RatingButtons over useItemState).
func (s *ItemStates) RatingButtons(c *ui.Context, kind string, it StateItem, name string) {
	st := s.Use(kind, it)
	s.Rate(kind, it, name, RatingButtons(c, RatingOpts{Name: name, Rating: st.Data.Rating, Pending: !st.HasData}))
}
