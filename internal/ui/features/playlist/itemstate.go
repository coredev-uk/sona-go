package playlist

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// This file is the playlist header's library state and rating (v0.2.0's
// shared/lib/item-state.ts and library-actions.ts, for one item).
// Replace with kit.ItemStates when album-artist-song merges.

// statePlaylists is the state kind of a playlist (Apple's catalogue type).
const statePlaylists = "playlists"

// stateAPI is what the header's state asks of LibraryService.
type stateAPI interface {
	States(ctx context.Context, refs []services.ItemRef) ([]services.ItemState, error)
	Favorite(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
	SuggestLess(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error)
	Add(ctx context.Context, typ string, ids []string) error
}

// membershipLists are the library lists to refresh when an item joins or
// leaves the library (item-state.ts).
var membershipLists = []store.Key{
	{"library", "counts"},
	{"library", "songs"},
	{"library", "albums"},
	{"library", "artists"},
	{"library", "album"},
	{"library", "artist"},
	{"library", "recently-added"},
	{"library", "favorite-songs"},
	keyPlaylists,
}

// stateStale is how long a state is fresh.
const stateStale = time.Minute

// headerState is the state and rating of the playlist a page shows, and the
// commands that change them: a change shows at once and is put back when it
// fails, with a toast saying what failed. Main thread only.
type headerState struct {
	q    *store.Client
	api  stateAPI
	post store.Poster
	// last is the state use saw, for a change to start from.
	last map[string]services.ItemState
}

func newHeaderState(q *store.Client, post store.Poster, api stateAPI) *headerState {
	return &headerState{q: q, api: api, post: post, last: map[string]services.ItemState{}}
}

// stateRef names a playlist for LibraryService.
func stateRef(pl services.Playlist) services.ItemRef {
	return services.ItemRef{Kind: statePlaylists, ID: pl.ID, Source: pl.Source, CatalogID: pl.CatalogID, LibraryID: pl.LibraryID}
}

func stateKey(id string) store.Key { return store.Key{"library", "state", statePlaylists, id} }

// use is the playlist's library state and rating; no data while it loads.
func (s *headerState) use(pl services.Playlist) store.Result[services.ItemState] {
	if pl.ID == "" {
		return store.Result[services.ItemState]{}
	}
	ref := stateRef(pl)
	r := store.Use(s.q, stateKey(pl.ID), func(ctx context.Context) (services.ItemState, error) {
		got, err := s.api.States(ctx, []services.ItemRef{ref})
		if err != nil {
			return services.ItemState{}, err
		}
		if len(got) == 0 {
			return services.ItemState{}, context.DeadlineExceeded
		}
		return got[0], nil
	}, store.Options{Stale: stateStale})
	if r.HasData {
		s.last[pl.ID] = r.Data
	}
	return r
}

// base is the state a change starts from: the last one seen, else what the
// playlist itself says.
func (s *headerState) base(pl services.Playlist) services.ItemState {
	if st, ok := s.last[pl.ID]; ok {
		return st
	}
	return services.ItemState{Kind: statePlaylists, ID: pl.ID, InLibrary: pl.Source == services.SourceLibrary, LibraryID: pl.LibraryID}
}

func ratingFailure(a kit.RatingAction, subject string) string {
	switch a {
	case kit.RateFavorite:
		return i18n.T("Couldn't favourite {subject}", "subject", subject)
	case kit.RateUnfavorite:
		return i18n.T("Couldn't undo favourite for {subject}", "subject", subject)
	case kit.RateSuggestLess:
		return i18n.T("Couldn't suggest less of {subject}", "subject", subject)
	default:
		return i18n.T("Couldn't undo suggest less for {subject}", "subject", subject)
	}
}

// rate changes the rating: the new state shows at once, Go's answer
// replaces it and a failure restores the old one and toasts.
func (s *headerState) rate(pl services.Playlist, a kit.RatingAction) {
	if a == kit.RateNone || pl.ID == "" {
		return
	}
	key := stateKey(pl.ID)
	prev, hadPrev := s.last[pl.ID]
	if !hadPrev {
		prev = s.base(pl)
	}
	next := prev
	switch a {
	case kit.RateFavorite:
		next.Rating = 1
	case kit.RateSuggestLess:
		next.Rating = -1
	default:
		next.Rating = 0
	}
	s.last[pl.ID] = next
	store.Set(s.q, key, next)
	ref := stateRef(pl)
	go func() {
		ctx := context.Background()
		var got services.ItemState
		var err error
		switch a {
		case kit.RateFavorite, kit.RateUnfavorite:
			got, err = s.api.Favorite(ctx, ref, a == kit.RateFavorite)
		default:
			got, err = s.api.SuggestLess(ctx, ref, a == kit.RateSuggestLess)
		}
		if err != nil {
			if hadPrev {
				store.Set(s.q, key, prev)
			} else {
				s.q.Invalidate(key)
			}
			s.post(func() { delete(s.last, pl.ID) })
			kit.ToastErrorf(ratingFailure(a, i18n.T("“{name}”", "name", pl.Name)), failText(err))
			return
		}
		store.Set(s.q, key, got)
		if got.InLibrary != prev.InLibrary {
			s.q.Invalidate(membershipLists...)
		}
	}()
}

// add adds a catalogue playlist to the library, with a toast either way.
func (s *headerState) add(pl services.Playlist) {
	id := pl.CatalogID
	if id == "" {
		id = pl.ID
	}
	go func() {
		if err := s.api.Add(context.Background(), statePlaylists, []string{id}); err != nil {
			kit.ToastErrorf(i18n.T("Couldn't add “{name}” to your library", "name", pl.Name), failText(err))
			return
		}
		s.post(func() {
			if st, ok := s.last[pl.ID]; ok {
				st.InLibrary = true
				s.last[pl.ID] = st
				store.Set(s.q, stateKey(pl.ID), st)
			}
			s.q.Invalidate(stateKey(pl.ID))
			s.q.Invalidate(membershipLists...)
		})
		kit.ShowToast(kit.ToastSpec{Title: i18n.T("Added “{name}” to your library", "name", pl.Name)})
	}()
}

// libraryButton is the header's library control: Add, or "In library".
// A library playlist is in the library; a catalogue one asks.
func (s *headerState) libraryButton(c *ui.Context, pl services.Playlist) {
	st := s.use(pl)
	in := pl.Source == services.SourceLibrary
	if st.HasData {
		in = st.Data.InLibrary
	}
	if kit.LibraryButton(c, kit.LibraryOpts{Name: pl.Name, InLibrary: in, Pending: !st.HasData, Playlist: true}) == kit.LibraryAdd {
		s.add(pl)
	}
}

// ratingButtons are the header's Favourite and Suggest Less.
func (s *headerState) ratingButtons(c *ui.Context, pl services.Playlist) {
	st := s.use(pl)
	s.rate(pl, kit.RatingButtons(c, kit.RatingOpts{Name: pl.Name, Rating: st.Data.Rating, Pending: !st.HasData}))
}
