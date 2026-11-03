package services

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/library"
)

// Library state, ratings (Favourite, Suggest Less), removal and links for
// any item the UI shows: LibraryService.States, Favorite, SuggestLess,
// Remove, CopyLink.

// stateKinds are the ItemRef kinds that have a library state and rating.
var stateKinds = map[string]bool{
	applemusic.TypeSongs: true, applemusic.TypeAlbums: true, applemusic.TypeArtists: true,
	applemusic.TypePlaylists: true, applemusic.TypeMusicVideos: true,
}

// removeKinds are the kinds Remove deletes from the library (library
// playlists are deleted through the playlist editing service).
var removeKinds = map[string]bool{
	applemusic.TypeSongs: true, applemusic.TypeAlbums: true, applemusic.TypeArtists: true,
	applemusic.TypeMusicVideos: true,
}

// linkPaths are the music.apple.com path segments of ItemRef kinds.
var linkPaths = map[string]string{
	applemusic.TypeSongs: "song", applemusic.TypeAlbums: "album", applemusic.TypeArtists: "artist",
	applemusic.TypePlaylists: "playlist", applemusic.TypeMusicVideos: "music-video", "stations": "station",
}

// refIDs returns r's catalog and library IDs (either may be "").
func refIDs(r ItemRef) (catalogID, libraryID string) {
	catalogID, libraryID = r.CatalogID, r.LibraryID
	switch r.Source {
	case SourceCatalog:
		if catalogID == "" {
			catalogID = r.ID
		}
	case SourceLibrary:
		if libraryID == "" {
			libraryID = r.ID
		}
	}
	return catalogID, libraryID
}

// stateKey is the resource whose library state answers for r: its catalog
// resource when there is one (catalog and library copies share a rating),
// else its library copy.
func stateKey(r ItemRef) (applemusic.ItemKey, error) {
	if !stateKinds[r.Kind] {
		return applemusic.ItemKey{}, fmt.Errorf("%w: item kind %q", ErrInvalidArgument, r.Kind)
	}
	cat, lib := refIDs(r)
	switch {
	case cat != "":
		return applemusic.ItemKey{Type: r.Kind, ID: cat}, nil
	case lib != "":
		return applemusic.ItemKey{Type: "library-" + r.Kind, ID: lib}, nil
	}
	return applemusic.ItemKey{}, fmt.Errorf("%w: %s without an ID", ErrInvalidArgument, r.Kind)
}

func itemState(r ItemRef, st applemusic.LibraryState) ItemState {
	return ItemState{Kind: r.Kind, ID: r.ID, InLibrary: st.InLibrary, LibraryID: st.LibraryID, Rating: st.Rating}
}

// States returns the library state (in the library, its library ID) and
// rating of each item, in order, the way the web player hydrates a page:
// one batched Apple lookup for the items not seen in the last two minutes
// (catalog items by catalog ID, library-only items by library ID, artists'
// ratings by ID). After the user's own changes their outcome is reported
// for five minutes, since Apple's answers lag. Signed out or offline, the
// answers come from the synced library and local ratings.
func (s *LibraryService) States(ctx context.Context, refs []ItemRef) ([]ItemState, error) {
	return s.states(ctx, refs, s.o.Auth.Status().State == auth.StateSignedIn)
}

// states is States; online says whether Apple may be asked.
func (s *LibraryService) states(ctx context.Context, refs []ItemRef, online bool) ([]ItemState, error) {
	keys := make([]applemusic.ItemKey, len(refs))
	missing := map[string][]string{}
	for i, r := range refs {
		k, err := stateKey(r)
		if err != nil {
			return nil, err
		}
		keys[i] = k
		if _, ok := s.o.members.state(k); !ok {
			missing[k.Type] = append(missing[k.Type], k.ID)
		}
	}
	if len(missing) > 0 && online {
		if err := s.fetchStates(ctx, missing); err != nil {
			s.o.Report(err)
			s.o.Logger.Warn("library: item states", "err", err)
		}
	}
	out := make([]ItemState, len(refs))
	for i, r := range refs {
		st, ok := s.o.members.state(keys[i])
		if !ok {
			st = s.localState(ctx, r)
		}
		out[i] = itemState(r, st)
	}
	return out, nil
}

// fetchStates asks Apple for the states of the keys in missing (IDs by
// type) and caches every answer; IDs Apple does not report are not in the
// library and unrated.
func (s *LibraryService) fetchStates(ctx context.Context, missing map[string][]string) error {
	catalogIDs, libraryIDs := map[string][]string{}, map[string][]string{}
	var artists []string
	for typ, ids := range missing {
		switch {
		case typ == applemusic.TypeArtists:
			artists = ids
		case isLibraryType(typ):
			libraryIDs[typ] = ids
		default:
			catalogIDs[typ] = ids
		}
	}
	remember := func(ids map[string][]string, got map[applemusic.ItemKey]applemusic.LibraryState) {
		for typ, list := range ids {
			for _, id := range list {
				k := applemusic.ItemKey{Type: typ, ID: id}
				s.o.members.remember(k, got[k])
			}
		}
	}
	g, gctx := errgroup.WithContext(ctx)
	if len(catalogIDs) > 0 {
		g.Go(func() error {
			got, err := s.o.API.CatalogLibraryStates(gctx, s.o.Storefront(), catalogIDs)
			if err == nil {
				remember(catalogIDs, got)
			}
			return err
		})
	}
	if len(libraryIDs) > 0 {
		g.Go(func() error {
			got, err := s.o.API.LibraryItemStates(gctx, libraryIDs)
			if err == nil {
				remember(libraryIDs, got)
			}
			return err
		})
	}
	if len(artists) > 0 {
		// Catalog artists carry no inLibrary: membership comes from the
		// synced library, the rating from /v1/me/ratings.
		g.Go(func() error {
			ratings, err := s.o.API.Ratings(gctx, applemusic.TypeArtists, artists)
			if err != nil {
				return err
			}
			for _, id := range artists {
				lid, in := s.o.members.lookup(gctx, kindArtist, id)
				s.o.members.remember(applemusic.ItemKey{Type: applemusic.TypeArtists, ID: id},
					applemusic.LibraryState{InLibrary: in, LibraryID: lid, Rating: ratings[id]})
			}
			return nil
		})
	}
	return g.Wait()
}

// localState answers for r from the synced library and local ratings.
func (s *LibraryService) localState(ctx context.Context, r ItemRef) applemusic.LibraryState {
	cat, lib := refIDs(r)
	var st applemusic.LibraryState
	if lib != "" {
		st.InLibrary, st.LibraryID = true, lib
	} else {
		st.LibraryID, st.InLibrary = s.o.members.lookup(ctx, addKinds[r.Kind], cat)
	}
	if st.LibraryID != "" {
		st.Rating, _ = s.o.Library.Rating(ctx, "library-"+r.Kind, st.LibraryID)
	}
	if st.Rating == 0 && cat != "" {
		st.Rating, _ = s.o.Library.Rating(ctx, r.Kind, cat)
	}
	return st
}

// current returns r's state as last known: cached, else local.
func (s *LibraryService) current(ctx context.Context, r ItemRef, k applemusic.ItemKey) applemusic.LibraryState {
	if st, ok := s.o.members.state(k); ok {
		return st
	}
	return s.localState(ctx, r)
}

// Favorite favourites (on) or un-favourites an item and returns its new
// state. As on the web, favouriting a catalog song also adds it to the
// library: it is reported in the library at once and the library syncs.
func (s *LibraryService) Favorite(ctx context.Context, r ItemRef, on bool) (ItemState, error) {
	return s.rate(ctx, r, on, applemusic.RatingFavorite)
}

// SuggestLess sets (on) or clears an item's Suggest Less rating and
// returns its new state.
func (s *LibraryService) SuggestLess(ctx context.Context, r ItemRef, on bool) (ItemState, error) {
	return s.rate(ctx, r, on, applemusic.RatingSuggestLess)
}

func (s *LibraryService) rate(ctx context.Context, r ItemRef, on bool, value int) (ItemState, error) {
	k, err := stateKey(r)
	if err != nil {
		return ItemState{}, err
	}
	st := s.current(ctx, r, k)
	cat, lib := refIDs(r)
	if lib == "" {
		lib = st.LibraryID
	}
	it := library.Rated{Kind: r.Kind, CatalogID: cat, LibraryID: lib}
	if value == applemusic.RatingFavorite {
		err = s.o.Library.SetFavorite(ctx, it, on)
	} else {
		err = s.o.Library.SetSuggestLess(ctx, it, on)
	}
	if err != nil {
		return ItemState{}, s.o.Fail(err)
	}
	st.Rating = 0
	if on {
		st.Rating = value
	}
	if on && value == applemusic.RatingFavorite && r.Kind == applemusic.TypeSongs && cat != "" && !st.InLibrary {
		st.InLibrary = true
		s.o.members.markAdded(applemusic.TypeSongs, []string{cat})
		s.syncSoon(addDomains[applemusic.TypeSongs]...)
	}
	s.o.members.hold(k, st)
	return itemState(r, st), nil
}

// Remove deletes an item (kind "songs", "albums", "artists" or
// "music-videos") from the library, as the web player's Delete from
// Library: an album takes its songs with it, an artist their albums and
// songs, and removed songs leave every playlist. The local library
// updates at once. It returns the item's new state.
func (s *LibraryService) Remove(ctx context.Context, r ItemRef) (ItemState, error) {
	if !removeKinds[r.Kind] {
		return ItemState{}, fmt.Errorf("%w: remove %q from the library", ErrInvalidArgument, r.Kind)
	}
	k, err := stateKey(r)
	if err != nil {
		return ItemState{}, err
	}
	st := s.current(ctx, r, k)
	cat, lib := refIDs(r)
	if lib == "" {
		lib = st.LibraryID
	}
	if lib == "" && cat != "" {
		// Added this session and not synced yet: ask Apple for the copy.
		if err := s.fetchStates(ctx, map[string][]string{r.Kind: {cat}}); err != nil {
			return ItemState{}, s.o.Fail(err)
		}
		st, _ = s.o.members.state(k)
		lib = st.LibraryID
	}
	if lib == "" {
		return ItemState{}, fmt.Errorf("%w: %s %s is not in the library", library.ErrNotFound, r.Kind, r.ID)
	}
	if err := s.o.Library.Remove(ctx, r.Kind, lib); err != nil {
		return ItemState{}, s.o.Fail(err)
	}
	st.InLibrary, st.LibraryID = false, ""
	s.o.members.hold(k, st)
	s.o.members.hold(applemusic.ItemKey{Type: "library-" + r.Kind, ID: lib}, st)
	if cat != "" {
		s.o.members.unmarkAdded(r.Kind, []string{cat})
	}
	if s.removeRecheck > 0 {
		go s.confirmRemoved(r.Kind, lib)
	}
	return itemState(r, st), nil
}

// confirmRemoved checks, removeRecheck after a removal, that Apple really
// dropped library item lib: a DELETE that arrives while Apple is still
// processing the item's add is acknowledged but ignored (observed with an
// album removed seconds after adding it). If the item is still there, the
// DELETE is sent once more. The local library already reflects the
// removal either way.
func (s *LibraryService) confirmRemoved(kind, lib string) {
	time.Sleep(s.removeRecheck)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	libType := "library-" + kind
	got, err := s.o.API.LibraryItemStates(ctx, map[string][]string{libType: {lib}})
	if err != nil {
		s.o.Logger.Warn("library: recheck removal", "id", lib, "err", err)
		return
	}
	if !got[applemusic.ItemKey{Type: libType, ID: lib}].InLibrary {
		return
	}
	s.o.Logger.Info("library: removal did not take, deleting again", "id", lib)
	if err := s.o.API.DeleteFromLibrary(ctx, kind, lib); err != nil {
		s.o.Report(err)
		s.o.Logger.Warn("library: repeat removal", "id", lib, "err", err)
	}
}

// CopyLink copies an item's music.apple.com link (kind "songs", "albums",
// "artists", "playlists", "music-videos" or "stations") to the clipboard
// and returns it. Items with no catalog equivalent (uploads, private
// playlists) have no link.
func (s *LibraryService) CopyLink(r ItemRef) (string, error) {
	seg, ok := linkPaths[r.Kind]
	cat, _ := refIDs(r)
	if !ok || cat == "" {
		return "", fmt.Errorf("%w: %s %q has no Apple Music link", ErrInvalidArgument, r.Kind, r.ID)
	}
	u := "https://music.apple.com/" + url.PathEscape(s.o.Storefront()) + "/" + seg + "/" + url.PathEscape(cat)
	if !s.setClipboard(u) {
		return "", fmt.Errorf("copy link: %w", ErrClipboard)
	}
	return u, nil
}
