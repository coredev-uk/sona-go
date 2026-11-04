package album

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakes: the services the page asks of, answering from the test's data and
// noting what was asked. Hermetic: no network, no database.

type fakeCatalog struct {
	mu     sync.Mutex
	pages  map[string]services.CatalogAlbum
	err    error
	gate   chan struct{} // when set, Album waits for it to close
	called []string
}

func (f *fakeCatalog) Album(ctx context.Context, id string) (services.CatalogAlbum, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return services.CatalogAlbum{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = append(f.called, id)
	if f.err != nil {
		return services.CatalogAlbum{}, f.err
	}
	p, ok := f.pages[id]
	if !ok {
		return services.CatalogAlbum{}, fmt.Errorf("%w: album %s", library.ErrNotFound, id)
	}
	return p, nil
}

func (f *fakeCatalog) calls(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.called {
		if c == id {
			n++
		}
	}
	return n
}

func (f *fakeCatalog) fail(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

type fakeLibrary struct {
	mu     sync.Mutex
	albums map[string]services.AlbumDetail
	called []string
}

func (f *fakeLibrary) Album(_ context.Context, id string) (services.AlbumDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = append(f.called, id)
	d, ok := f.albums[id]
	if !ok {
		return services.AlbumDetail{}, fmt.Errorf("%w: album %s", library.ErrNotFound, id)
	}
	return d, nil
}

type played struct {
	ids   []string
	start int
	from  services.PlayFrom
}

type fakeQueue struct {
	mu      sync.Mutex
	plays   []played
	shuffle bool
	err     error
}

func (f *fakeQueue) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	p := played{start: start, from: from}
	for _, r := range refs {
		p.ids = append(p.ids, r.ID)
	}
	f.plays = append(f.plays, p)
	return nil
}

func (f *fakeQueue) SetShuffle(on bool) {
	f.mu.Lock()
	f.shuffle = on
	f.mu.Unlock()
}

func (f *fakeQueue) UndoReplace(context.Context) (bool, error) { return true, nil }

func (f *fakeQueue) last() (played, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.plays) == 0 {
		return played{}, false
	}
	return f.plays[len(f.plays)-1], true
}

type fakeRadio struct {
	mu     sync.Mutex
	played []string
}

func (f *fakeRadio) Play(_ context.Context, id string) error {
	f.mu.Lock()
	f.played = append(f.played, id)
	f.mu.Unlock()
	return nil
}

func (f *fakeRadio) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.played)
}

type fakeAuth struct{ status services.AuthStatus }

func (f fakeAuth) Status() services.AuthStatus { return f.status }

// fakeStates is LibraryService's item state side.
type fakeStates struct {
	mu      sync.Mutex
	inLib   map[string]bool
	rating  map[string]int
	added   []string
	favored []string
	loads   int
}

func (f *fakeStates) loaded() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loads > 0
}

func (f *fakeStates) States(_ context.Context, refs []services.ItemRef) ([]services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	out := make([]services.ItemState, len(refs))
	for i, r := range refs {
		out[i] = services.ItemState{Kind: r.Kind, ID: r.ID, InLibrary: f.inLib[r.ID], Rating: f.rating[r.ID]}
	}
	return out, nil
}

func (f *fakeStates) Favorite(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.favored = append(f.favored, r.ID)
	st := services.ItemState{Kind: r.Kind, ID: r.ID, InLibrary: f.inLib[r.ID]}
	if on {
		st.Rating = 1
	}
	f.rating[r.ID] = st.Rating
	return st, nil
}

func (f *fakeStates) SuggestLess(ctx context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	return services.ItemState{Kind: r.Kind, ID: r.ID, Rating: -1}, nil
}

func (f *fakeStates) Remove(_ context.Context, r services.ItemRef) (services.ItemState, error) {
	return services.ItemState{Kind: r.Kind, ID: r.ID}, nil
}

func (f *fakeStates) Add(_ context.Context, _ string, ids []string) error {
	f.mu.Lock()
	f.added = append(f.added, ids...)
	for _, id := range ids {
		f.inLib[id] = true
	}
	f.mu.Unlock()
	return nil
}

// rig is the page over the fakes, in a router, in a window.
type rig struct {
	t      *testing.T
	h      *uitest.Harness
	router *ui.Router
	a      *album
	tt     *uitest.Session

	cat    *fakeCatalog
	lib    *fakeLibrary
	queue  *fakeQueue
	radio  *fakeRadio
	states *fakeStates
	now    time.Time
	routes map[string]bool // what HasRoute says, by prefix
	ready  bool            // the page is built once the test has set its data
}

// newRig shows /album/<path> in a w×h window. Every route is ported unless
// the test says otherwise (unported(...)).
func newRig(t *testing.T, path string, w, h int, opts ...uitest.Option) *rig {
	t.Helper()
	r := &rig{
		t:     t,
		cat:   &fakeCatalog{pages: map[string]services.CatalogAlbum{}},
		lib:   &fakeLibrary{albums: map[string]services.AlbumDetail{}},
		queue: &fakeQueue{}, radio: &fakeRadio{}, states: &fakeStates{inLib: map[string]bool{}, rating: map[string]int{}},
		now: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
	}
	r.h = uitest.New(t, opts...)
	// The covers come from a generator: nothing fails and is asked for again.
	loader := kit.NewArtLoaderFrom(coverSource{}, r.h.Main.Post)
	t.Cleanup(func() { loader.Close(); loader.Wait() })
	r.h.Env.Art = loader
	return r.start(path, w, h)
}

func (r *rig) start(path string, w, h int) *rig {
	env := r.h.Env
	r.router = ui.NewRouter(path)
	env.Nav = r.router
	env.HasRoute = func(p string) bool {
		for prefix := range r.routes {
			if strings.HasPrefix(p, prefix) {
				return false
			}
		}
		return true
	}
	r.a = newAlbum(env)
	env.Actions = kit.NewActionRegistry(r.a.module().Actions...)
	r.a.menus = kit.NewActionMenus(env.Actions, nil)
	r.a.trackMenus = kit.NewActionMenus(env.Actions, &r.a.tracks.Sel)
	r.a.lib, r.a.cat, r.a.radio = r.lib, r.cat, r.radio
	r.a.auth = fakeAuth{services.AuthStatus{State: "signed_in"}}
	r.a.queue = newQueueCommands(r.queue, env.Play, nil)
	r.a.states = kit.NewItemStates(env.Queries, env.Post, r.states)
	r.a.video = func() bool { return true }
	r.a.now = func() time.Time { return r.now }
	r.tt = r.h.Tester(r.t, func(c *ui.Context) {
		if !r.ready {
			return // the test is still putting its data in place
		}
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(albumPath) {
					r.a.page(c, rt)
					kit.OverlayLayer(c)
					return
				}
				ui.Text(c, "page "+rt.Path())
			})
		})
	}, w, h)
	return r
}

// unported says these route prefixes have no page.
func (r *rig) unported(prefixes ...string) {
	r.routes = map[string]bool{}
	for _, p := range prefixes {
		r.routes[p] = true
	}
}

// open lets the page build: the data the test put in place is what it loads.
func (r *rig) open() {
	r.ready = true
	r.tt.Drain()
}

// shows draws frames until every text is on screen.
func (r *rig) shows(texts ...string) {
	r.t.Helper()
	if !r.ready {
		r.open()
	}
	r.tt.Until(fmt.Sprintf("%q on screen", texts), r.tt.Shows(texts...))
}

// click clicks a text and runs the frames it asks for.
func (r *rig) click(text string) {
	r.t.Helper()
	if err := r.tt.Click(text); err != nil {
		r.t.Fatalf("click %q: %v (texts %q)", text, err, r.tt.Texts())
	}
	r.tt.Drain()
}

// Fixture data: invented, as the uitest fixture's.

func track(n int, name string, mods ...func(*services.Song)) services.Song {
	s := services.Song{
		ID: fmt.Sprintf("t%d", n), CatalogID: fmt.Sprintf("c%d", n), Source: services.SourceCatalog,
		Name: name, Artist: "Mia Vale", Album: "Salt Flats", AlbumID: "100", ArtistID: "a1",
		DurationMs: 200000, TrackNumber: n, DiscNumber: 1, Playable: true,
	}
	for _, m := range mods {
		m(&s)
	}
	return s
}

func fiveTracks() []services.Song {
	return []services.Song{
		track(1, "Salt Flats"), track(2, "Low Tide"), track(3, "Glass Houses"), track(4, "Evening Bus"), track(5, "Paper Moon"),
	}
}

// saltFlats is a catalogue album page.
func saltFlats(mods ...func(*services.CatalogAlbum)) services.CatalogAlbum {
	tracks := fiveTracks()
	p := services.CatalogAlbum{
		AlbumDetail: services.AlbumDetail{
			Album: services.Album{
				ID: "100", Source: services.SourceCatalog, CatalogID: "100", Name: "Salt Flats", Artist: "Mia Vale", ArtistID: "a1",
				ReleaseDate: "2019-03-08", Genre: "Indie Pop", TrackCount: 5,
				Art: services.Art{Small: "/art/100/s.jpg", Medium: "/art/100/m.jpg", Large: "/art/100/l.jpg"},
			},
			Tracks: tracks, DurationMs: 1000000, Label: "Quiet Records", Copyright: "℗ 2019 Mia Vale",
			Notes: "A record about driving across the flats at dusk.\n\nRecorded in a week.",
		},
		NotesShort: "Mia Vale's second album.",
		Artists:    []services.Artist{{ID: "a1", Name: "Mia Vale"}},
		PopularIDs: []string{"c2"},
		Genres:     []string{"Indie Pop", "Pop"},
	}
	for _, m := range mods {
		m(&p)
	}
	return p
}

func (r *rig) addCatalog(p services.CatalogAlbum) { r.cat.pages[p.Album.ID] = p }
