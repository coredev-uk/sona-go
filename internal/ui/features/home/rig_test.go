package home

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// fakeDiscover answers Home, RefreshHome and HomeShelf from what a test
// sets, counting the calls.
type fakeDiscover struct {
	mu          sync.Mutex
	page        services.HomePage
	err         error
	refresh     services.HomePage
	refreshErr  error
	shelves     map[string]services.Section
	shelfErr    error
	homeCalls   int
	refreshes   int
	shelfCalls  int
	refreshGate chan struct{} // non-nil: RefreshHome waits for it to close
	shelfGate   chan struct{} // non-nil: HomeShelf waits for it to close
	homeGate    chan struct{} // non-nil: Home waits for it to close
}

func (f *fakeDiscover) Home(context.Context) (services.HomePage, error) {
	f.mu.Lock()
	gate := f.homeGate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.homeCalls++
	return f.page, f.err
}

func (f *fakeDiscover) RefreshHome(context.Context) (services.HomePage, error) {
	f.mu.Lock()
	gate := f.refreshGate
	f.refreshes++
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refresh, f.refreshErr
}

func (f *fakeDiscover) HomeShelf(_ context.Context, id string) (services.Section, error) {
	f.mu.Lock()
	gate := f.shelfGate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shelfCalls++
	if f.shelfErr != nil {
		return services.Section{}, f.shelfErr
	}
	return f.shelves[id], nil
}

func (f *fakeDiscover) set(fn func(f *fakeDiscover)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeDiscover) calls() (home, refresh, shelf int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.homeCalls, f.refreshes, f.shelfCalls
}

// fakeLib is the library the local shelves read: every answer is what a test
// sets.
type fakeLib struct {
	mu          sync.Mutex
	recent      []services.Song
	recentErr   error
	days        []services.HistoryDay
	historyErr  error
	albums      []services.Album
	albumsErr   error
	playlists   []services.Playlist
	playlistErr error
	asked       []string
	// recentGate, when set, holds RecentlyPlayed until it is closed.
	recentGate chan struct{}
}

func (l *fakeLib) note(s string) {
	l.mu.Lock()
	l.asked = append(l.asked, s)
	l.mu.Unlock()
}

func (l *fakeLib) count(prefix string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, a := range l.asked {
		if strings.HasPrefix(a, prefix) {
			n++
		}
	}
	return n
}

func (l *fakeLib) RecentlyPlayed(_ context.Context, limit, offset int) ([]services.Song, error) {
	l.note(fmt.Sprintf("recent %d %d", limit, offset))
	l.mu.Lock()
	gate := l.recentGate
	l.mu.Unlock()
	if gate != nil {
		<-gate
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.recent, l.recentErr
}

func (l *fakeLib) History(_ context.Context, days int) (services.ListeningHistory, error) {
	l.note(fmt.Sprintf("history %d", days))
	l.mu.Lock()
	defer l.mu.Unlock()
	return services.ListeningHistory{Days: l.days}, l.historyErr
}

func (l *fakeLib) Albums(_ context.Context, sort string, desc bool, typ string, off, limit int) (services.AlbumPage, error) {
	l.note(fmt.Sprintf("albums %s %v %q %d %d", sort, desc, typ, off, limit))
	l.mu.Lock()
	defer l.mu.Unlock()
	return services.AlbumPage{Items: l.albums, Total: len(l.albums)}, l.albumsErr
}

func (l *fakeLib) Playlists(_ context.Context, sort string, desc bool, off, limit int) (services.PlaylistPage, error) {
	l.note(fmt.Sprintf("playlists %s %v %d %d", sort, desc, off, limit))
	l.mu.Lock()
	defer l.mu.Unlock()
	return services.PlaylistPage{Items: l.playlists, Total: len(l.playlists)}, l.playlistErr
}

// recorder notes what the registered players were asked to play.
type recorder struct {
	mu    sync.Mutex
	plays []string
}

func (r *recorder) add(s string) {
	r.mu.Lock()
	r.plays = append(r.plays, s)
	r.mu.Unlock()
}

func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.plays)
}

// rig is Home over a full Env: the fake services behind the page, a router,
// and a tester.
type rig struct {
	t      *testing.T
	h      *uitest.Harness
	home   *home
	router *ui.Router
	tt     *uitest.Session
	disc   *fakeDiscover
	// auth is the sign-in state in the cache; noAuth leaves it unknown (the
	// reply of Go has not come).
	auth   services.AuthStatus
	noAuth bool
	lib    *fakeLib
	rec    *recorder
	bus    *store.Bus
	ui     []uitest.Option
	// routes says which pages a module serves (default: the ones Home links to).
	routes func(string) bool
	// concerts is the concerts module's shelf (Env.ConcertsShelf); nil: none.
	concerts func(c *ui.Context, first bool)
}

// signedIn is the status of an account that can read Home.
var signedIn = services.AuthStatus{State: "signed_in", StorefrontID: "gb"}

// rigOpt sets what the fake services answer before the first frame: the
// tester draws one as it is made, which starts the queries.
type rigOpt func(*rig)

func withHome(p services.HomePage) rigOpt    { return func(r *rig) { r.disc.page = p } }
func withDisc(f func(*fakeDiscover)) rigOpt  { return func(r *rig) { f(r.disc) } }
func withLib(f func(*fakeLib)) rigOpt        { return func(r *rig) { f(r.lib) } }
func withAuth(st services.AuthStatus) rigOpt { return func(r *rig) { r.auth = st } }

// withUI gives the harness these options (the real Core, for the covers).
func withUI(o ...uitest.Option) rigOpt { return func(r *rig) { r.ui = o } }

// withRoutes sets which pages a module serves.
func withRoutes(f func(string) bool) rigOpt { return func(r *rig) { r.routes = f } }

func withConcerts(f func(c *ui.Context, first bool)) rigOpt {
	return func(r *rig) { r.concerts = f }
}

// withUnknownAuth leaves the sign-in state unknown: Go has not answered.
func withUnknownAuth() rigOpt { return func(r *rig) { r.noAuth = true } }

// newRig builds Home over the harness (its fixture library is not used: the
// local shelves read the rig's fake). The page is the route's at path, in a
// window of w by h.
func newRig(t *testing.T, path string, w, hgt int, opts ...rigOpt) *rig {
	t.Helper()
	r := &rig{t: t, router: ui.NewRouter(path), disc: &fakeDiscover{}, auth: signedIn, lib: &fakeLib{}, rec: &recorder{}}
	for _, o := range opts {
		o(r)
	}
	if len(r.ui) == 0 {
		r.ui = []uitest.Option{uitest.Empty()} // the artwork cache is the Core's: Bare has none
	}
	h := uitest.New(t, r.ui...)
	r.h = h
	// Home hears the rig's own bus: the Core's start-up events (its real,
	// signed-out account) must not answer for the fake sign-in state.
	env := new(module.Env)
	*env = *h.Env
	env.Bus = store.NewBus(nil)
	env.Bus.SetPoster(h.Main.Post)
	r.bus = env.Bus
	env.Nav = r.router
	env.HasRoute = func(p string) bool {
		if r.routes != nil {
			return r.routes(p)
		}
		for _, prefix := range []string{"/album/", "/playlist/", "/station/", "/artist/", "/song/", "/for-you/", "/library/albums", "/library/playlists", "/room/"} {
			if strings.HasPrefix(p, prefix) {
				return true
			}
		}
		return false
	}
	if env.Surfaces == nil {
		env.Surfaces = store.NewSurfaces()
	}
	for _, kind := range []string{"album", "playlist", "station", "song"} {
		env.Players.Register(kind, func(it services.Item) { r.rec.add(it.Kind + ":" + it.ID) })
	}
	env.Players.RegisterSongs(func(songs []services.Song, start int, label string) {
		r.rec.add(fmt.Sprintf("songs:%s@%d/%d", label, start, len(songs)))
	})
	for _, o := range opts {
		o(r)
	}
	env.ConcertsShelf = r.concerts
	// What the auth module does: the cache follows auth:changed.
	store.On(env.Bus, services.EventAuthChanged, func(s services.AuthStatus) { store.Set(env.Queries, store.KeyAuth, s) })
	if r.noAuth {
		env.Core = nil // no answer from Go: the status stays unknown
	} else {
		store.Set(env.Queries, store.KeyAuth, r.auth)
	}
	r.home = newHome(env, r.disc, r.lib)
	r.home.init()
	r.tt = h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // frames settle at once; motion is not what these tests check
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				switch {
				case rt.Match(homePath):
					r.home.homePage(c, rt)
				case rt.Match(shelfPattern):
					r.home.shelfPage(c, rt)
				}
			})
		})
	}, w, hgt)
	return r
}

// until draws frames until every text is on screen.
func (r *rig) until(what string, texts ...string) {
	r.t.Helper()
	r.tt.Until(what, r.tt.Shows(texts...))
}

// gone is the condition that none of the texts is on screen.
func (r *rig) gone(texts ...string) func() bool {
	return func() bool {
		for _, s := range texts {
			if r.tt.HasText(s) {
				return false
			}
		}
		return true
	}
}

func (r *rig) click(what string) {
	r.t.Helper()
	if err := r.tt.Click(what); err != nil {
		r.t.Fatal(err)
	}
	r.tt.Drain()
}

// emit sends a Go event as the services do.
func (r *rig) emit(name string, payload any) {
	r.bus.Emit(name, payload)
	r.tt.Drain()
}

// path is the router's current path.
func (r *rig) path() string { return r.router.Path() }

// The data of the tests: invented albums, playlists and stations.

func coverArt(i int) services.Art {
	t := fixture.Template(i)
	return services.Art{Small: t, Medium: t, Large: t}
}

func albumItem(id, name, artist string, i int) services.Item {
	return services.Item{
		Kind: services.ItemAlbum, ID: id, Name: name, Subtitle: artist, Art: coverArt(i),
		Album: &services.Album{ID: id, Name: name, Artist: artist},
	}
}

func playlistItem(id, name, curator string, i int) services.Item {
	return services.Item{
		Kind: services.ItemPlaylist, ID: id, Name: name, Subtitle: curator, Art: coverArt(i),
		Playlist: &services.Playlist{ID: id, Name: name, Curator: curator},
	}
}

func stationItem(id, name string, i int, page bool) services.Item {
	return services.Item{
		Kind: services.ItemStation, ID: id, Name: name, Art: coverArt(i),
		Station: &services.Station{ID: id, Name: name, Playable: true, HasPage: page, Kind: services.StationTracks},
	}
}

func section(id, kind, title string, items ...services.Item) services.Section {
	return services.Section{ID: id, Kind: kind, Title: title, Items: items}
}

// albums is n invented albums "<prefix> 1"…, by one artist.
func albums(prefix string, n, cover int) []services.Item {
	var out []services.Item
	for i := 1; i <= n; i++ {
		out = append(out, albumItem(fmt.Sprintf("%s%d", prefix, i), fmt.Sprintf("%s %d", prefix, i), "Artist "+prefix, cover+i))
	}
	return out
}

// samplePage is a Home with every kind of shelf: a lead of twelve picks (the
// first a new release), Recently Played, the featured station, Made for You
// (a shared caption, a page of its own), a station shelf, two more plain
// shelves, and two for the index.
func samplePage() services.HomePage {
	picks := albums("Pick", 12, 0)
	picks[0].Name, picks[0].Subtitle, picks[0].Caption = "Lead Album", "Lead Artist", "New Release"
	picks[0].Album.Name = "Lead Album"
	recent := section("recent", services.SectionShelf, "Recently Played",
		albumItem("r1", "Recent One", "Rae", 20), stationItem("r2", "Recent Station", 21, false), albumItem("r3", "Recent Three", "Raf", 22))
	recent.Source = recentlyPlayed
	hero := section("hero", services.SectionHero, "", stationItem("hero1", "Featured Station", 30, true))
	hero.Items[0].Subtitle, hero.Items[0].Caption = "Apple Music Hits", "Radio"
	var made []services.Item
	for i := 1; i <= 4; i++ {
		p := playlistItem(fmt.Sprintf("m%d", i), fmt.Sprintf("Mix %d", i), "Apple Music", 40+i)
		p.Caption = "Apple Music for You"
		made = append(made, p)
	}
	madeFor := section("made", services.SectionShelf, "Made for You", made...)
	madeFor.SeeAll = true
	stations := section("moods", services.SectionShelf, "Find Your Mood",
		stationItem("st1", "Energy", 50, true), stationItem("st2", "Focus", 51, true), stationItem("st3", "Relax", 52, false))
	g1 := section("g1", services.SectionShelf, "Indie Picks", albums("Indie", 4, 60)...)
	g2 := section("g2", services.SectionShelf, "Jazz Picks", albums("Jazz", 4, 70)...)
	fans := section("fans", services.SectionShelf, "Fans Also Like",
		append(albums("Fan", 3, 80), playlistItem("fp1", "Fan Mix", "Curator", 90))...)
	fans.SeeAll = true
	because := section("because", services.SectionShelf, "Because You Like Rock", albums("Rock", 2, 95)...)
	return services.HomePage{
		Sections:  []services.Section{section("top", services.SectionFeature, "Top Picks for You", picks...), recent, hero, madeFor, stations, g1, g2, fans, because},
		FetchedAt: uitest.Epoch.Add(-5 * time.Minute).UnixMilli(),
	}
}
