package library

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// actLib is a LibraryService for the item actions: it answers states from a
// table (an item not in it is as its source says), records what it was
// asked, and fails on request.
type actLib struct {
	libraryAPI

	mu      sync.Mutex
	states  map[string]services.ItemState // by kind/id
	asked   [][]services.ItemRef
	favs    []actRating
	less    []actRating
	removed []services.ItemRef
	added   []actAdd
	copied  []services.ItemRef

	// gate, when set, holds Favorite until it is closed.
	gate chan struct{}
	// failRate fails Favorite and SuggestLess of the IDs named ("*": all);
	// failRemove, failAdd, failCopy and failStates fail those calls.
	failRate   map[string]error
	failRemove error
	failAdd    error
	failCopy   error
	failStates error
}

type actRating struct {
	ref services.ItemRef
	on  bool
}

type actAdd struct {
	typ string
	ids []string
}

func (f *actLib) key(kind, id string) string { return kind + "/" + id }

func (f *actLib) state(r services.ItemRef) services.ItemState {
	if st, ok := f.states[f.key(r.Kind, r.ID)]; ok {
		return st
	}
	return services.ItemState{Kind: r.Kind, ID: r.ID, InLibrary: r.Source == services.SourceLibrary, LibraryID: r.LibraryID}
}

func (f *actLib) States(_ context.Context, refs []services.ItemRef) ([]services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, slices.Clone(refs))
	if f.failStates != nil {
		return nil, f.failStates
	}
	out := make([]services.ItemState, len(refs))
	for i, r := range refs {
		out[i] = f.state(r)
	}
	return out, nil
}

func (f *actLib) rate(r services.ItemRef, on bool, value int, into *[]actRating) (services.ItemState, error) {
	f.mu.Lock()
	*into = append(*into, actRating{r, on})
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failRate[r.ID]; err != nil {
		return services.ItemState{}, err
	}
	if err := f.failRate["*"]; err != nil {
		return services.ItemState{}, err
	}
	st := f.state(r)
	st.Rating = 0
	if on {
		st.Rating = value
	}
	if f.states == nil {
		f.states = map[string]services.ItemState{}
	}
	f.states[f.key(r.Kind, r.ID)] = st
	return st, nil
}

func (f *actLib) Favorite(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	return f.rate(r, on, 1, &f.favs)
}

func (f *actLib) SuggestLess(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	return f.rate(r, on, -1, &f.less)
}

func (f *actLib) Remove(_ context.Context, r services.ItemRef) (services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, r)
	if f.failRemove != nil {
		return services.ItemState{}, f.failRemove
	}
	st := f.state(r)
	st.InLibrary, st.LibraryID = false, ""
	return st, nil
}

func (f *actLib) Add(_ context.Context, typ string, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, actAdd{typ, slices.Clone(ids)})
	return f.failAdd
}

func (f *actLib) CopyLink(r services.ItemRef) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.copied = append(f.copied, r)
	return "", f.failCopy
}

func (f *actLib) setState(kind string, st services.ItemState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.states == nil {
		f.states = map[string]services.ItemState{}
	}
	f.states[f.key(kind, st.ID)] = st
}

func (f *actLib) stateCalls() [][]services.ItemRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

func (f *actLib) favourited() []actRating {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.favs)
}

func (f *actLib) removals() []services.ItemRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.removed)
}

func (f *actLib) additions() []actAdd {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.added)
}

func (f *actLib) copies() []services.ItemRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.copied)
}

// The items the tests act on.
var (
	actCatalogSong = services.Song{ID: "1001", Source: services.SourceCatalog, CatalogID: "1001", Name: "Airbag"}
	actLibrarySong = services.Song{ID: "i.abc", Source: services.SourceLibrary, CatalogID: "1002", LibraryID: "i.abc", Name: "Paranoid Android"}
	actUploadSong  = services.Song{ID: "i.upl", Source: services.SourceLibrary, LibraryID: "i.upl", Name: "Demo"}
)

// newActRig is the library over a recording LibraryService.
func newActRig(t *testing.T) (*pageRig, *actLib) {
	t.Helper()
	f := &actLib{}
	r := newPageRig(t, albumsPath, 900, 700, func(l *library) {
		f.libraryAPI = l.lib
		l.lib = f
	})
	// A confirmation left asking would leak into the next test.
	t.Cleanup(kit.CancelConfirm)
	return r, f
}

// actDef is the action id of a kind of target.
func actDef(t *testing.T, r *pageRig, kind kit.ItemKind, id string) *kit.ActionDef {
	t.Helper()
	for _, d := range r.h.Env.Actions.For(kind) {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no action %s for %s: %v", id, kind, actIDs(r.h.Env.Actions.For(kind)))
	return nil
}

func actBulk(t *testing.T, r *pageRig, items []kit.Item, id string) *kit.ActionDef {
	t.Helper()
	for _, d := range r.h.Env.Actions.BulkFor(items) {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no bulk action %s for %v", id, items)
	return nil
}

func actIDs(defs []*kit.ActionDef) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.ID
	}
	return out
}

// actContext is what a menu on items would read once its data loaded.
func actContext(r *pageRig, items ...kit.Item) *kit.ActionContext {
	var defs []*kit.ActionDef
	if len(items) > 1 {
		defs = r.h.Env.Actions.BulkFor(items)
	} else {
		defs = r.h.Env.Actions.For(items[0].Kind)
	}
	loaded := map[string]any{}
	for _, d := range defs {
		for _, md := range d.Data {
			if _, done := loaded[md.ID]; !done {
				loaded[md.ID] = md.Load(context.Background(), items)
			}
		}
	}
	return r.h.Env.Actions.Context(items, nil, loaded)
}

// actCached is the state in the store, ok when there is one.
func actCached(r *pageRig, kind, id string) (services.ItemState, bool) {
	res := store.Use(r.h.Env.Queries, kit.StateKey(kind, id), func(context.Context) (services.ItemState, error) {
		return services.ItemState{}, errors.New("not cached")
	}, store.Options{Stale: store.Forever})
	return res.Data, res.HasData
}

// actWatcher counts the fetches of a query the way a mounted page does: the
// count moves when something invalidates the key.
type actWatcher struct {
	r     *pageRig
	key   store.Key
	n     atomic.Int32
	fetch func(context.Context) (int, error)
}

func (w *actWatcher) use() bool {
	return store.Use(w.r.h.Env.Queries, w.key, w.fetch, store.Options{Stale: store.Forever}).HasData
}

func actWatch(r *pageRig, key store.Key) *actWatcher {
	w := &actWatcher{r: r, key: append(slices.Clone(key), "watch")}
	w.fetch = func(context.Context) (int, error) { w.n.Add(1); return 0, nil }
	// Its own key under the watched one, so no page's type clashes with it.
	r.tt.Until("the first fetch of a watched query", w.use)
	return w
}

// reaches waits until the query has been fetched want times; it keeps the
// query in use, as a page does every frame.
func (w *actWatcher) reaches(what string, want int32) {
	w.r.t.Helper()
	w.r.tt.Until(what, func() bool { w.use(); return w.n.Load() >= want })
}

func TestLibraryActionsRegisterEveryIDOnceAndListPerKind(t *testing.T) {
	r, _ := newActRig(t)
	reg := r.h.Env.Actions
	// Menu order: group (item, then rating), then order.
	want := map[kit.ItemKind][]string{
		kit.KindSong: {
			"library.add.song", "library.remove.song", "library.copy-link.song",
			"library.favorite.song", "library.suggest-less.song",
		},
		kit.KindAlbum: {
			"library.add.album", "library.remove.album", "library.copy-link.album",
			"library.favorite.album", "library.suggest-less.album",
		},
		kit.KindArtist: {
			"library.remove.artist", "library.copy-link.artist",
			"library.favorite.artist", "library.suggest-less.artist",
		},
		kit.KindPlaylist: {
			"library.add.playlist", "library.copy-link.playlist",
			"library.favorite.playlist", "library.suggest-less.playlist",
		},
		kit.KindStation:    {"library.copy-link.station"},
		kit.KindMusicVideo: nil,
	}
	for kind, ids := range want {
		if got := actIDs(reg.For(kind)); !slices.Equal(got, ids) && (len(got) != 0 || len(ids) != 0) {
			t.Errorf("%s: %v, want %v", kind, got, ids)
		}
	}
	songs := []kit.Item{kit.SongItem(actCatalogSong), kit.SongItem(actLibrarySong)}
	if got := actIDs(reg.BulkFor(songs)); !slices.Equal(got, []string{"library.add.many", "library.remove.many", "library.favorite.many"}) {
		t.Errorf("bulk songs: %v", got)
	}
	mixed := []kit.Item{kit.SongItem(actCatalogSong), kit.ArtistItem(services.Artist{ID: "r1", Name: "Radiohead"})}
	if got := actIDs(reg.BulkFor(mixed)); !slices.Equal(got, []string{"library.remove.many", "library.favorite.many"}) {
		t.Errorf("bulk song+artist: %v", got)
	}
	lists := []kit.Item{kit.PlaylistItem(services.Playlist{ID: "p1"}), kit.PlaylistItem(services.Playlist{ID: "p2"})}
	if got := actIDs(reg.BulkFor(lists)); !slices.Equal(got, []string{"library.add.many", "library.favorite.many"}) {
		t.Errorf("bulk playlists: %v", got)
	}
	// Icons and groups are v0.2.0's.
	for id, icon := range map[string]string{
		"library.favorite.song": "star", "library.suggest-less.song": "thumbs-down", "library.copy-link.song": "link-2",
		"library.add.song": "plus", "library.remove.song": "trash-2",
	} {
		if d := actDef(t, r, kit.KindSong, id); d.Icon != icon {
			t.Errorf("%s icon %q, want %q", id, d.Icon, icon)
		}
	}
	if actDef(t, r, kit.KindSong, "library.favorite.song").Group != kit.GroupRating ||
		actDef(t, r, kit.KindSong, "library.add.song").Group != kit.GroupItem {
		t.Error("groups")
	}
}

func TestFavouriteAndSuggestLessFollowTheLoadedState(t *testing.T) {
	r, _ := newActRig(t)
	item := kit.SongItem(actCatalogSong)
	fav, less := actDef(t, r, kit.KindSong, "library.favorite.song"), actDef(t, r, kit.KindSong, "library.suggest-less.song")

	// Before the state loads: named, but not enabled.
	empty := r.h.Env.Actions.Context([]kit.Item{item}, nil, nil)
	if fav.Label(empty) != "Favourite" || fav.Enabled(empty) || less.Label(empty) != "Suggest less" || less.Enabled(empty) {
		t.Errorf("unloaded: %q enabled %v, %q enabled %v", fav.Label(empty), fav.Enabled(empty), less.Label(empty), less.Enabled(empty))
	}

	ctx := actContext(r, item)
	if fav.Label(ctx) != "Favourite" || fav.Filled(ctx) || !fav.Enabled(ctx) || less.Label(ctx) != "Suggest less" || less.Filled(ctx) {
		t.Errorf("rating 0: %q filled %v enabled %v", fav.Label(ctx), fav.Filled(ctx), fav.Enabled(ctx))
	}

	r2, f2 := newActRig(t)
	f2.setState("songs", services.ItemState{Kind: "songs", ID: "1001", Rating: 1})
	ctx = actContext(r2, item)
	if fav.Label(ctx) != "Undo favourite" || !fav.Filled(ctx) || less.Filled(ctx) {
		t.Errorf("rating 1: %q filled %v", fav.Label(ctx), fav.Filled(ctx))
	}

	r3, f3 := newActRig(t)
	f3.setState("songs", services.ItemState{Kind: "songs", ID: "1001", Rating: -1})
	ctx = actContext(r3, item)
	if less.Label(ctx) != "Undo suggest less" || !less.Filled(ctx) || fav.Filled(ctx) {
		t.Errorf("rating -1: %q filled %v", less.Label(ctx), less.Filled(ctx))
	}

	// A state that cannot be had leaves the rating rows disabled.
	r4, f4 := newActRig(t)
	f4.failStates = services.ErrNotReleased
	ctx = actContext(r4, item)
	if fav.Enabled(ctx) || less.Enabled(ctx) {
		t.Error("rating rows enabled without a state")
	}
}

func TestAddAndDeleteAvailabilityFollowTheLibraryState(t *testing.T) {
	r, f := newActRig(t)
	add, del := actDef(t, r, kit.KindSong, "library.add.song"), actDef(t, r, kit.KindSong, "library.remove.song")
	avail := func(d *kit.ActionDef, a *kit.ActionContext) bool { return d.Available == nil || d.Available(a) }

	// A catalogue song not in the library: Add, no Delete. Not enabled until
	// its state loads.
	item := kit.SongItem(actCatalogSong)
	unloaded := r.h.Env.Actions.Context([]kit.Item{item}, nil, nil)
	if !avail(add, unloaded) || add.Enabled(unloaded) || avail(del, unloaded) {
		t.Errorf("unloaded catalogue song: add %v enabled %v delete %v", avail(add, unloaded), add.Enabled(unloaded), avail(del, unloaded))
	}
	loaded := actContext(r, item)
	if !avail(add, loaded) || !add.Enabled(loaded) || avail(del, loaded) || add.Label(loaded) != "Add to library" {
		t.Errorf("catalogue song: add %v delete %v label %q", avail(add, loaded), avail(del, loaded), add.Label(loaded))
	}

	// The same song once the state says it is in the library.
	f.setState("songs", services.ItemState{Kind: "songs", ID: "1001", InLibrary: true, LibraryID: "i.1"})
	r2, f2 := newActRig(t)
	f2.setState("songs", services.ItemState{Kind: "songs", ID: "1001", InLibrary: true, LibraryID: "i.1"})
	inLib := actContext(r2, item)
	if avail(add, inLib) || !avail(del, inLib) {
		t.Errorf("song in library: add %v delete %v", avail(add, inLib), avail(del, inLib))
	}

	// A library song: Delete, no Add (even before its state loads); an
	// upload (no catalogue ID) has no link.
	lib := kit.SongItem(actLibrarySong)
	unloaded = r.h.Env.Actions.Context([]kit.Item{lib}, nil, nil)
	if avail(add, unloaded) || !avail(del, unloaded) {
		t.Errorf("library song: add %v delete %v", avail(add, unloaded), avail(del, unloaded))
	}
	link := actDef(t, r, kit.KindSong, "library.copy-link.song")
	if !avail(link, unloaded) || avail(link, r.h.Env.Actions.Context([]kit.Item{kit.SongItem(actUploadSong)}, nil, nil)) ||
		!avail(link, r.h.Env.Actions.Context([]kit.Item{item}, nil, nil)) {
		t.Error("Copy link is for items Apple Music links to")
	}

	// Albums: an album not out yet is pre-added; artists have no Add and
	// playlists no Delete.
	addAlbum := actDef(t, r, kit.KindAlbum, "library.add.album")
	future := services.Album{ID: "al1", Source: services.SourceCatalog, Name: "Soon", Prerelease: true, ReleaseAt: time.Now().Add(48 * time.Hour).UnixMilli()}
	undated := services.Album{ID: "al2", Source: services.SourceCatalog, Name: "Undated", Prerelease: true}
	out := services.Album{ID: "al3", Source: services.SourceCatalog, Name: "Out", Prerelease: true, ReleaseAt: time.Now().Add(-48 * time.Hour).UnixMilli()}
	for _, c := range []struct {
		a    services.Album
		want string
	}{{future, "Pre-add to library"}, {undated, "Pre-add to library"}, {out, "Add to library"}, {services.Album{ID: "al4", Source: services.SourceCatalog}, "Add to library"}} {
		if got := addAlbum.Label(r.h.Env.Actions.Context([]kit.Item{kit.AlbumItem(c.a)}, nil, nil)); got != c.want {
			t.Errorf("album %q: %q, want %q", c.a.Name, got, c.want)
		}
	}
	for _, id := range []string{"library.add.artist", "library.remove.playlist"} {
		for _, d := range r.h.Env.Actions.For(kit.KindArtist) {
			if d.ID == id {
				t.Errorf("%s registered", id)
			}
		}
		for _, d := range r.h.Env.Actions.For(kit.KindPlaylist) {
			if d.ID == id {
				t.Errorf("%s registered", id)
			}
		}
	}
}

func TestFavouriteCallsTheServiceAndShowsAtOnce(t *testing.T) {
	r, f := newActRig(t)
	f.gate = make(chan struct{})
	watch := actWatch(r, store.KeyLibraryFavourites)
	counts := actWatch(r, store.KeyLibraryCounts)
	item := kit.SongItem(actCatalogSong)
	ctx := actContext(r, item)

	actDef(t, r, kit.KindSong, "library.favorite.song").Run(ctx)
	// The call is in flight (held), and the cache already says Favourite.
	r.tt.Until("the Favorite call and the optimistic state", func() bool {
		st, ok := actCached(r, "songs", "1001")
		return len(f.favourited()) == 1 && ok && st.Rating == 1
	})
	call := f.favourited()[0]
	if !call.on || call.ref.Kind != "songs" || call.ref.ID != "1001" || call.ref.CatalogID != "1001" || call.ref.Source != services.SourceCatalog {
		t.Errorf("Favorite got %+v", call)
	}
	close(f.gate)
	// A plain rating of a song refreshes the favourites, not the counts.
	watch.reaches("the favourites refreshed", 2)
	if n := counts.n.Load(); n != 1 {
		t.Errorf("a rating refreshed the counts: %d fetches", n)
	}
	if st, _ := actCached(r, "songs", "1001"); st.Rating != 1 {
		t.Errorf("cached %+v", st)
	}

	// The state it learnt is Favourite now: the same action undoes it.
	f.gate = nil
	ctx = actContext(r, item)
	fav := actDef(t, r, kit.KindSong, "library.favorite.song")
	if fav.Label(ctx) != "Undo favourite" || !fav.Filled(ctx) {
		t.Errorf("after favouriting: %q filled %v", fav.Label(ctx), fav.Filled(ctx))
	}
}

func TestUndoFavouriteAndSuggestLessCallTheirVerbs(t *testing.T) {
	r, f := newActRig(t)
	f.setState("songs", services.ItemState{Kind: "songs", ID: "1001", Rating: 1})
	f.setState("albums", services.ItemState{Kind: "albums", ID: "al1", Rating: -1})
	ctx := actContext(r, kit.SongItem(actCatalogSong))
	actDef(t, r, kit.KindSong, "library.favorite.song").Run(ctx)
	r.tt.Until("Favorite off", func() bool { return len(f.favourited()) == 1 })
	if f.favourited()[0].on {
		t.Error("an item already Favourite was favourited again")
	}

	album := kit.AlbumItem(services.Album{ID: "al1", Source: services.SourceCatalog, Name: "Kid A"})
	actDef(t, r, kit.KindAlbum, "library.suggest-less.album").Run(actContext(r, album))
	r.tt.Until("SuggestLess off", func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.less) == 1 })
	if f.less[0].on || f.less[0].ref.Kind != "albums" {
		t.Errorf("got %+v", f.less[0])
	}
	// An album's rating does not refresh the favourite songs.
}

func TestFavouriteFailureRestoresTheStateAndToasts(t *testing.T) {
	r, f := newActRig(t)
	f.failRate = map[string]error{"*": services.ErrNotReleased}
	item := kit.SongItem(actCatalogSong)
	ctx := actContext(r, item)
	if st, ok := actCached(r, "songs", "1001"); !ok || st.Rating != 0 {
		t.Fatalf("loaded %+v %v", st, ok)
	}
	actDef(t, r, kit.KindSong, "library.favorite.song").Run(ctx)
	r.tt.Until("the failure's toast", r.tt.Shows("Couldn't favourite “Airbag”", i18n.ErrorText(services.CodeNotReleased)))
	if st, _ := actCached(r, "songs", "1001"); st.Rating != 0 {
		t.Errorf("the failed rating stayed: %+v", st)
	}
}

func TestCopyLinkCopiesAndToastsEitherWay(t *testing.T) {
	r, f := newActRig(t)
	actDef(t, r, kit.KindSong, "library.copy-link.song").Run(actContext(r, kit.SongItem(actCatalogSong)))
	r.tt.Until("the toast", r.tt.Shows("Link copied"))
	got := f.copies()
	if len(got) != 1 || got[0].Kind != "songs" || got[0].ID != "1001" {
		t.Errorf("copied %+v", got)
	}

	f.failCopy = services.ErrClipboard
	actDef(t, r, kit.KindAlbum, "library.copy-link.album").Run(actContext(r, kit.AlbumItem(services.Album{ID: "al1", Source: services.SourceCatalog, Name: "Kid A"})))
	r.tt.Until("the failure's toast", r.tt.Shows("Couldn't copy the link", i18n.ErrorText(services.CodeOf(services.ErrClipboard))))

	// A station's link is asked as a station of the catalogue.
	st := kit.StationItem(services.Station{ID: "ra.1", Name: "Radio"})
	f.failCopy = nil
	actDef(t, r, kit.KindStation, "library.copy-link.station").Run(r.h.Env.Actions.Context([]kit.Item{st}, nil, nil))
	r.tt.Until("the station link", func() bool { return len(f.copies()) == 3 })
	if c := f.copies()[2]; c.Kind != "stations" || c.ID != "ra.1" || c.Source != services.SourceCatalog {
		t.Errorf("station link asked as %+v", c)
	}
}

func TestAddToLibraryCallsAddWithTheCatalogueIDAndRefreshesTheLists(t *testing.T) {
	r, f := newActRig(t)
	counts := actWatch(r, store.KeyLibraryCounts)
	songs := actWatch(r, store.Key{"library", "songs"})
	item := kit.SongItem(services.Song{ID: "x-lib", Source: services.SourceCatalog, CatalogID: "cat-9", Name: "Karma Police"})
	ctx := actContext(r, item)
	actDef(t, r, kit.KindSong, "library.add.song").Run(ctx)
	r.tt.Until("the toast", r.tt.Shows("Added “Karma Police” to your library"))
	adds := f.additions()
	if len(adds) != 1 || adds[0].typ != "songs" || !slices.Equal(adds[0].ids, []string{"cat-9"}) {
		t.Errorf("added %+v", adds)
	}
	counts.reaches("the counts refreshed", 2)
	songs.reaches("the songs refreshed", 2)

	f.failAdd = services.ErrNotReleased
	actDef(t, r, kit.KindPlaylist, "library.add.playlist").Run(
		actContext(r, kit.PlaylistItem(services.Playlist{ID: "pl.1", Source: services.SourceCatalog, Name: "Mix"})))
	r.tt.Until("the failure's toast", r.tt.Shows("Couldn't add “Mix” to your library", i18n.ErrorText(services.CodeNotReleased)))
	if got := f.additions(); got[len(got)-1].typ != "playlists" {
		t.Errorf("added %+v", got)
	}
}

func TestDeleteAsksFirstAndCancelDoesNothing(t *testing.T) {
	inBubble(t, deleteAsksFirstAndCancelDoesNothing)
}

func deleteAsksFirstAndCancelDoesNothing(t *testing.T) {
	r, f := newActRig(t)
	counts := actWatch(r, store.KeyLibraryCounts)
	item := kit.SongItem(actLibrarySong)
	ctx := actContext(r, item)
	del := actDef(t, r, kit.KindSong, "library.remove.song")

	del.Run(ctx)
	r.tt.Drain()
	if !kit.ConfirmPending() || !r.tt.Shows("Delete “Paranoid Android” from your library?", "It also leaves any playlists it's in.")() {
		t.Fatalf("no confirmation: texts %q", r.tt.Texts())
	}
	if len(f.removals()) != 0 {
		t.Fatal("deleted before the answer")
	}
	if err := r.tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	r.tt.Quiesce()
	if kit.ConfirmPending() || len(f.removals()) != 0 {
		t.Errorf("cancel: pending %v, removals %v", kit.ConfirmPending(), f.removals())
	}
	if st, _ := actCached(r, "songs", "i.abc"); !st.InLibrary {
		t.Errorf("cancel changed the state: %+v", st)
	}

	del.Run(ctx)
	r.tt.Drain()
	if err := r.tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the toast", r.tt.Shows("Deleted “Paranoid Android” from your library"))
	got := f.removals()
	if len(got) != 1 || got[0].Kind != "songs" || got[0].ID != "i.abc" || got[0].LibraryID != "i.abc" {
		t.Errorf("removed %+v", got)
	}
	if st, ok := actCached(r, "songs", "i.abc"); !ok || st.InLibrary {
		t.Errorf("cached %+v", st)
	}
	counts.reaches("the counts refreshed", 2)
}

func TestDeleteNotesPerKindAndFailureToast(t *testing.T) {
	r, f := newActRig(t)
	f.failRemove = services.ErrNotReleased
	album := kit.AlbumItem(services.Album{ID: "l.1", Source: services.SourceLibrary, LibraryID: "l.1", Name: "OK Computer"})
	actDef(t, r, kit.KindAlbum, "library.remove.album").Run(actContext(r, album))
	r.tt.Drain()
	if !r.tt.Shows("Delete “OK Computer” from your library?", "Its songs leave your library and any playlists they're in.")() {
		t.Fatalf("album confirmation: %q", r.tt.Texts())
	}
	_ = r.tt.Click("Delete")
	r.tt.Until("the failure's toast", r.tt.Shows("Couldn't delete “OK Computer”", i18n.ErrorText(services.CodeNotReleased)))
	// The failed removal put the library state back.
	if st, _ := actCached(r, "albums", "l.1"); !st.InLibrary {
		t.Errorf("state after a failed delete: %+v", st)
	}

	artist := kit.ArtistItem(services.Artist{ID: "r.1", Source: services.SourceLibrary, LibraryID: "r.1", Name: "Radiohead"})
	actDef(t, r, kit.KindArtist, "library.remove.artist").Run(actContext(r, artist))
	r.tt.Drain()
	if !r.tt.Shows("Delete “Radiohead” from your library?", "Their albums and songs leave your library too.")() {
		t.Fatalf("artist confirmation: %q", r.tt.Texts())
	}
}

func actSongs(n int, source services.Source) []kit.Item {
	var out []kit.Item
	for i := range n {
		id := string(rune('a' + i))
		s := services.Song{ID: "c-" + id, Source: source, CatalogID: "c-" + id, Name: "Song " + id}
		if source == services.SourceLibrary {
			s.ID, s.LibraryID = "i."+id, "i."+id
		}
		out = append(out, kit.SongItem(s))
	}
	return out
}

func TestBulkFavouriteSkipsWhatAlreadyIs(t *testing.T) {
	inBubble(t, bulkFavouriteSkipsWhatAlreadyIs)
}

func bulkFavouriteSkipsWhatAlreadyIs(t *testing.T) {
	r, f := newActRig(t)
	items := actSongs(3, services.SourceCatalog)
	f.setState("songs", services.ItemState{Kind: "songs", ID: "c-b", Rating: 1})
	fav := actBulk(t, r, items, "library.favorite.many")
	ctx := actContext(r, items...)
	if fav.Label(ctx) != "Favourite" || fav.Filled(ctx) || !fav.Enabled(ctx) {
		t.Errorf("mixed: %q filled %v enabled %v", fav.Label(ctx), fav.Filled(ctx), fav.Enabled(ctx))
	}
	fav.Run(ctx)
	r.tt.Until("two Favorite calls", func() bool { return len(f.favourited()) == 2 })
	var ids []string
	for _, c := range f.favourited() {
		if !c.on {
			t.Errorf("%+v is not favouriting", c)
		}
		ids = append(ids, c.ref.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"c-a", "c-c"}) {
		t.Errorf("favourited %v, want the two not yet favourite", ids)
	}
	r.tt.Quiesce()
	if n := len(f.favourited()); n != 2 {
		t.Errorf("%d calls", n)
	}

	// Everything Favourite: the action undoes all of them.
	r.tt.Until("all favourite in the cache", func() bool {
		for _, id := range []string{"c-a", "c-b", "c-c"} {
			if st, ok := actCached(r, "songs", id); !ok || st.Rating != 1 {
				return false
			}
		}
		return true
	})
	ctx = actContext(r, items...)
	if fav.Label(ctx) != "Undo favourite" || !fav.Filled(ctx) {
		t.Errorf("all favourite: %q filled %v", fav.Label(ctx), fav.Filled(ctx))
	}
	fav.Run(ctx)
	r.tt.Until("three more calls", func() bool { return len(f.favourited()) == 5 })
	for _, c := range f.favourited()[2:] {
		if c.on {
			t.Errorf("%+v favourited while undoing", c)
		}
	}
}

func TestBulkFavouriteFailureNamesHowManyFailed(t *testing.T) {
	inBubble(t, bulkFavouriteFailureNamesHowManyFailed)
}

func bulkFavouriteFailureNamesHowManyFailed(t *testing.T) {
	r, f := newActRig(t)
	items := actSongs(3, services.SourceCatalog)
	f.failRate = map[string]error{"c-b": services.ErrNotReleased}
	ctx := actContext(r, items...)
	actBulk(t, r, items, "library.favorite.many").Run(ctx)
	r.tt.Until("the toast", r.tt.Shows("Couldn't favourite 1 of 3 songs", i18n.ErrorText(services.CodeNotReleased)))
	if st, _ := actCached(r, "songs", "c-b"); st.Rating != 0 {
		t.Errorf("the failed one stayed favourite: %+v", st)
	}
	if st, _ := actCached(r, "songs", "c-a"); st.Rating != 1 {
		t.Errorf("the others did not: %+v", st)
	}

	// The failed one is not retried.
	r.tt.Quiesce()
	if n := len(f.favourited()); n != 3 {
		t.Errorf("%d Favorite calls, want 3", n)
	}
}

func TestBulkAddOnlyAddsWhatIsNotInTheLibrary(t *testing.T) {
	r, f := newActRig(t)
	cat := actSongs(3, services.SourceCatalog)
	f.setState("songs", services.ItemState{Kind: "songs", ID: "c-b", InLibrary: true, LibraryID: "i.b"})
	albumCat := kit.AlbumItem(services.Album{ID: "al-1", Source: services.SourceCatalog, CatalogID: "cat-al-1", Name: "Kid A"})
	items := append(slices.Clone(cat), albumCat)
	add := actBulk(t, r, items, "library.add.many")
	ctx := actContext(r, items...)
	if !add.Available(ctx) || !add.Enabled(ctx) {
		t.Fatal("Add to library is not offered")
	}
	add.Run(ctx)
	r.tt.Until("the toast", r.tt.Shows("Added 4 items to your library"))
	adds := f.additions()
	slices.SortFunc(adds, func(a, b actAdd) int { return len(b.typ) - len(a.typ) })
	got := map[string][]string{}
	for _, a := range adds {
		got[a.typ] = a.ids
	}
	if len(adds) != 2 || !slices.Equal(got["songs"], []string{"c-a", "c-c"}) || !slices.Equal(got["albums"], []string{"cat-al-1"}) {
		t.Errorf("added %+v", adds)
	}

	// Nothing to add: the action is not listed.
	r2, f2 := newActRig(t)
	lib := actSongs(2, services.SourceLibrary)
	ctx = actContext(r2, lib...)
	if actBulk(t, r2, lib, "library.add.many").Available(ctx) {
		t.Error("Add to library offered for songs already in the library")
	}
	_ = f2
}

func TestBulkDeleteAsksOnceThenRemovesEachOne(t *testing.T) {
	r, f := newActRig(t)
	items := actSongs(3, services.SourceLibrary)
	del := actBulk(t, r, items, "library.remove.many")
	ctx := actContext(r, items...)
	if !del.Available(ctx) {
		t.Fatal("Delete from library is not offered for library songs")
	}
	if actBulk(t, r, items, "library.remove.many").Available(r.h.Env.Actions.Context(actSongs(2, services.SourceCatalog), nil, nil)) {
		t.Error("Delete offered for catalogue songs")
	}
	del.Run(ctx)
	r.tt.Drain()
	if !r.tt.Shows("Delete 3 songs from your library?", "They also leave any playlists they're in.")() {
		t.Fatalf("confirmation: %q", r.tt.Texts())
	}
	if len(f.removals()) != 0 {
		t.Fatal("removed before the answer")
	}
	_ = r.tt.Click("Delete")
	r.tt.Until("the toast", r.tt.Shows("Deleted 3 songs from your library"))
	var ids []string
	for _, c := range f.removals() {
		ids = append(ids, c.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"i.a", "i.b", "i.c"}) {
		t.Errorf("removed %v", ids)
	}
}

func TestMenuDataOfAMenuIsOneStatesCall(t *testing.T) {
	r, f := newActRig(t)
	items := actSongs(5, services.SourceCatalog)

	// The bulk menu loads the whole selection at once.
	bulk := actBulk(t, r, items, "library.favorite.many")
	_ = bulk.Data[0].Load(context.Background(), items)
	calls := f.stateCalls()
	if len(calls) != 1 || len(calls[0]) != 5 {
		t.Fatalf("bulk menu: %d States calls %v", len(calls), calls)
	}

	// Single menus opened together share a call too.
	more := actSongs(8, services.SourceCatalog)[5:]
	more = append(more, kit.SongItem(services.Song{ID: "z1", Source: services.SourceCatalog, Name: "Z1"}), kit.SongItem(services.Song{ID: "z2", Source: services.SourceCatalog, Name: "Z2"}))
	fav := actDef(t, r, kit.KindSong, "library.favorite.song")
	var wg sync.WaitGroup
	for _, it := range more {
		wg.Add(1)
		go func() { defer wg.Done(); fav.Data[0].Load(context.Background(), []kit.Item{it}) }()
	}
	wg.Wait()
	calls = f.stateCalls()
	if len(calls) != 2 || len(calls[1]) != len(more) {
		t.Fatalf("%d items in a window: %d States calls %v", len(more), len(calls), calls)
	}

	// What was learnt is not asked again within the minute.
	_ = bulk.Data[0].Load(context.Background(), items)
	if n := len(f.stateCalls()); n != 2 {
		t.Errorf("a second menu on the same items asked Go again (%d calls)", n)
	}
}

func TestItemStateEventWritesTheStateAndRefreshesFavourites(t *testing.T) {
	r, _ := newActRig(t)
	watch := actWatch(r, store.KeyLibraryFavourites)
	if _, ok := actCached(r, "songs", "1001"); ok {
		t.Fatal("cached before the event")
	}
	r.h.Bus.Emit(services.EventItemState, services.ItemState{Kind: "songs", ID: "1001", InLibrary: true, Rating: 1})
	r.tt.Until("the state written", func() bool {
		st, ok := actCached(r, "songs", "1001")
		return ok && st.Rating == 1 && st.InLibrary
	})
	watch.reaches("the favourites refreshed", 2)
}

func TestRowMenuOfASongOffersFavouriteAndRunsIt(t *testing.T) {
	inBubble(t, rowMenuOfASongOffersFavouriteAndRunsIt)
}

func rowMenuOfASongOffersFavouriteAndRunsIt(t *testing.T) {
	f := &actLib{}
	r := newSongsRig(t, songsPath, func(l *library) {
		f.libraryAPI = l.lib
		l.lib = f
	})
	r.tt.Until("the songs", r.tt.Shows("Apple Orchard song 1"))
	if err := r.tt.RightClick("Apple Orchard song 1"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the row menu", r.tt.Shows("Favourite", "Suggest less"))
	// The menu's data is one States call for the row, then the rating rows
	// are enabled.
	r.tt.Until("the state asked", func() bool { return len(f.stateCalls()) > 0 })
	r.tt.Quiesce() // the answer comes back through the Poster
	if err := r.tt.Click("Favourite"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the Favorite call", func() bool { return len(f.favourited()) == 1 })
	if c := f.favourited()[0]; !c.on || c.ref.Kind != "songs" {
		t.Errorf("Favorite got %+v", c)
	}
}
