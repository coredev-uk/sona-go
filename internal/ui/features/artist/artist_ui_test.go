package artist

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// fakeCatalog is CatalogService over a page and its views, recording what
// was asked.
type fakeCatalog struct {
	mu    sync.Mutex
	page  services.CatalogArtist
	err   error
	views map[string]func(offset int) services.ArtistViewPage
	calls []string
	// gate, when set, holds Artist until it is closed; viewGate does the
	// same for ArtistView.
	gate, viewGate chan struct{}
}

func (f *fakeCatalog) Artist(ctx context.Context, id string) (services.CatalogArtist, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "artist "+id)
	if f.err != nil {
		return services.CatalogArtist{}, f.err
	}
	return f.page, nil
}

func (f *fakeCatalog) ArtistView(ctx context.Context, id, view string, offset, limit int) (services.ArtistViewPage, error) {
	if f.viewGate != nil {
		select {
		case <-f.viewGate:
		case <-ctx.Done():
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("view %s %s %d/%d", id, view, offset, limit))
	if f.err != nil {
		return services.ArtistViewPage{}, f.err
	}
	return f.views[view](offset), nil
}

func (f *fakeCatalog) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeCatalog) fail(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

// fakeLibrary is LibraryService.Artist over one detail.
type fakeLibrary struct {
	detail services.ArtistDetail
	err    error
}

func (f fakeLibrary) Artist(context.Context, string) (services.ArtistDetail, error) {
	return f.detail, f.err
}

// fakePlayback is PlaybackService as far as the queue commands go.
type fakePlayback struct {
	mu      sync.Mutex
	plays   []playCall
	shuffle bool
}

type playCall struct {
	refs  []services.TrackRef
	start int
	from  services.PlayFrom
}

func (f *fakePlayback) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.plays = append(f.plays, playCall{refs, start, from})
	return nil
}
func (f *fakePlayback) UndoReplace(context.Context) (bool, error) { return true, nil }
func (f *fakePlayback) SetShuffle(on bool) {
	f.mu.Lock()
	f.shuffle = on
	f.mu.Unlock()
}
func (f *fakePlayback) played() []playCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.plays)
}

// fakeStates is LibraryService's item states and ratings.
type fakeStates struct {
	mu     sync.Mutex
	rating int
	asked  int
	faved  []bool
}

func (f *fakeStates) States(_ context.Context, refs []services.ItemRef) ([]services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked += len(refs)
	out := make([]services.ItemState, len(refs))
	for i, r := range refs {
		out[i] = services.ItemState{Kind: r.Kind, ID: r.ID, Rating: f.rating}
	}
	return out, nil
}
func (f *fakeStates) Favorite(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faved = append(f.faved, on)
	f.rating = map[bool]int{true: 1, false: 0}[on]
	return services.ItemState{Kind: r.Kind, ID: r.ID, Rating: f.rating}, nil
}
func (f *fakeStates) SuggestLess(context.Context, services.ItemRef, bool) (services.ItemState, error) {
	return services.ItemState{}, nil
}
func (f *fakeStates) Remove(context.Context, services.ItemRef) (services.ItemState, error) {
	return services.ItemState{}, nil
}
func (f *fakeStates) Add(context.Context, string, []string) error { return nil }

type fakeAuth struct{ restricted bool }

func (f fakeAuth) Status() services.AuthStatus { return services.AuthStatus{Restricted: f.restricted} }

// rig is the artist module over fakes, in a router, behind a tester.
type rig struct {
	t      *testing.T
	h      *uitest.Harness
	a      *artist
	router *ui.Router
	tt     *uitest.Session
	cat    *fakeCatalog
	lib    fakeLibrary
	play   *fakePlayback
	states *fakeStates
	auth   fakeAuth
	// pushed are the paths the router was sent to.
	routes []string
}

func hasPage(path string) bool {
	for _, p := range []string{"/artist/", "/album/", "/playlist/", "/music-video/", "/library/artists"} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// newRig opens path in a w×h window. real builds the Env over the seeded
// library (real LibraryService and PlaybackService); otherwise the services
// are fakes and the Env has none.
func newRig(t *testing.T, path string, w, h int, setup func(*rig)) *rig {
	t.Helper()
	r := &rig{t: t, cat: &fakeCatalog{views: map[string]func(int) services.ArtistViewPage{}}, play: &fakePlayback{}, states: &fakeStates{}}
	t.Cleanup(func() { kit.CloseToast("") })
	r.h = uitest.New(t)
	// Covers come through the harness's artwork cache, from its fake CDN.
	art = services.Art{
		Small:  r.h.Art.URL(fixture.Template(0), 40, 40),
		Medium: r.h.Art.URL(fixture.Template(1), 300, 300),
		Large:  r.h.Art.URL(fixture.Template(2), 1000, 1000),
	}
	if setup != nil {
		setup(r)
	}
	env := r.h.Env
	r.router = ui.NewRouter(path)
	env.Nav = r.router
	env.HasRoute = hasPage
	env.Actions = kit.NewActionRegistry(goToArtist(env), kit.ActionDef{
		ID: "test.album", Target: kit.KindAlbum, Group: kit.GroupItem, Order: 1,
		Label: func(*kit.ActionContext) string { return "Test album action" },
		Run:   func(*kit.ActionContext) {},
	}, kit.ActionDef{
		ID: "test.artist", Target: kit.KindArtist, Group: kit.GroupItem, Order: 1,
		Label: func(*kit.ActionContext) string { return "Test artist action" },
		Run:   func(*kit.ActionContext) {},
	})
	env.Actions.SetNavigator(r.router.Push)
	env.Actions.SetPoster(r.h.Main.Post)
	states := kit.NewItemStates(env.Queries, env.Post, r.states)
	queue := &queueCommands{svc: r.play, play: env.Play, post: env.Post}
	r.a = newArtist(env, r.lib, r.cat, r.auth, queue, states)
	r.a.init()
	r.tt = r.h.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				switch {
				case rt.Match(viewPattern):
					r.a.viewPage(c, rt)
				case rt.Match(artistPattern):
					r.a.artistPage(c, rt)
				}
			})
		})
	}, w, h)
	return r
}

// ---- fixtures: Radiohead as Apple's page, invented around the test ----

// art is the cover of every fixture item; newRig fills it in.
var art services.Art

func album(id, name, date string) services.Album {
	return services.Album{ID: id, Name: name, Artist: "Radiohead", ArtistID: "657515", ReleaseDate: date, Art: art, Source: services.SourceCatalog, CatalogID: id}
}

func shelfOf(view string, more bool, albums ...services.Album) services.ArtistShelf {
	return services.ArtistShelf{View: view, Kind: services.ShelfAlbums, More: more, Albums: albums}
}

func song(i int) services.Song {
	id := fmt.Sprint(900 + i)
	return services.Song{ID: id, CatalogID: id, Source: services.SourceCatalog, Name: fmt.Sprintf("Song %d", i), Artist: "Radiohead", ArtistID: "657515",
		Album: "Pablo Honey", AlbumID: "ph", DurationMs: 240000, Playable: true, Art: art}
}

func songs(n int) []services.Song {
	out := make([]services.Song, n)
	for i := range out {
		out[i] = song(i + 1)
	}
	return out
}

func radiohead() services.CatalogArtist {
	return services.CatalogArtist{
		Artist:       services.Artist{ID: "657515", Name: "Radiohead", Source: services.SourceCatalog, CatalogID: "657515", Art: art},
		Header:       services.Header{Colors: []string{"#334455"}},
		Bio:          "More than a band.\n\nFormed in 1985.",
		Origin:       "Abingdon, Oxfordshire, England",
		BornOrFormed: "1985",
		Genres:       []string{"Alternative"},
		LatestRelease: func() *services.Album {
			a := album("latest", "Hail to the Thief (Live Recordings 2003-2009)", "2016-05-03")
			return &a
		}(),
		TopSongs:     songs(8),
		TopSongsMore: true,
		Shelves: []services.ArtistShelf{
			shelfOf("featured-albums", false, album("ok", "OK Computer", "1997-05-21"), album("kida", "Kid A", "2000-10-02")),
			shelfOf("full-albums", false, album("ir", "In Rainbows", "2007-10-10")),
			shelfOf("singles", true, album("dd", "Daydreaming - Single", "2016-05-03"), album("ep1", "Com Lag: 2+2=5 - EP", "2004-04-24")),
			shelfOf("appears-on-albums", false, album("creep", "Creep (Very 2021 Rmx)", "2021-01-01")),
			shelfOf("compilation-albums", false, album("kam", "KID A MNESIA", "2021-11-05")),
			{View: "playlists", Kind: services.ShelfPlaylists, Playlists: []services.Playlist{{ID: "pl.1", Name: "Radiohead Essentials", Curator: "Apple Music Alternative", Art: art}}},
			{View: "similar-artists", Kind: services.ShelfArtists, Artists: []services.Artist{{ID: "999", Name: "Thom Yorke", Art: art}}},
		},
	}
}

func catalogRig(t *testing.T, mutate func(*services.CatalogArtist)) *rig {
	return newRig(t, "/artist/657515", 1132, 1500, func(r *rig) {
		r.cat.page = radiohead()
		if mutate != nil {
			mutate(&r.cat.page)
		}
	})
}

// y is where the text is on screen.
func (r *rig) y(text string) float32 {
	r.t.Helper()
	b, ok := r.tt.Find(text)
	if !ok {
		r.t.Fatalf("no %q on screen; texts %q", text, r.tt.Texts())
	}
	return b.Y
}

// clickWith clicks the middle of the first thing showing text, holding mods.
func (r *rig) clickWith(mods ui.Modifiers, text string) {
	r.t.Helper()
	b, ok := r.tt.Find(text)
	if !ok {
		r.t.Fatalf("no %q on screen", text)
	}
	r.tt.ClickAtWith(mods, b.X+b.W/2, b.Y+b.H/2)
}

func (r *rig) shows(texts ...string) {
	r.t.Helper()
	r.tt.Until("the page", r.tt.Shows(texts...))
}

// ---- the catalogue page ----

func TestCatalogueArtistPageHasTheHeaderTheLeadAndTheBandsInOrder(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Radiohead", "Discography", "Related", "About Radiohead")

	for _, w := range []string{
		"Artist · Alternative · Abingdon, Oxfordshire, England", // five own releases: the count is unknown (singles has more)
		"More than a band.", "Read more",
		// The latest release's slip.
		"Hail to the Thief (Live Recordings 2003-2009)", "Latest release · May 3, 2016", "Album",
	} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
	if r.tt.HasText("Formed in 1985.") && r.y("Formed in 1985.") < r.y("About Radiohead") {
		t.Error("the biography's second paragraph is in the header")
	}
	// Top songs beside the essential albums, ahead of the bands.
	for _, w := range []string{"Top songs", "Top songs by Radiohead", "Essential albums by Radiohead", "Song 1", "Song 6"} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
	if r.tt.HasText("Song 7") {
		t.Error("more than six top songs on the page")
	}
	d, rel, ab := r.y("Discography"), r.y("Related"), r.y("About Radiohead")
	if !(r.y("Essential albums by Radiohead") < d && d < rel && rel < ab) {
		t.Errorf("order: essentials %v, discography %v, related %v, about %v", r.y("Essential albums by Radiohead"), d, rel, ab)
	}
	// Discography: albums as covers, the back catalogue as lists (the title
	// without Apple's suffix, the kind in its line), then Related.
	for _, w := range []string{
		"Albums by Radiohead", "Singles and EPs featuring Radiohead", "Compilations featuring Radiohead",
		"Daydreaming", "2016 · Single", "Com Lag: 2+2=5", "2004 · EP", "KID A MNESIA",
		"Albums featuring Radiohead", "Radiohead · 2021", "Playlists featuring Radiohead", "Similar artists to Radiohead",
		"From", "Born or formed", "Genre", "Alternative",
	} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
	if slices.Contains(r.tt.Texts(), "Daydreaming - Single") {
		t.Error("the single's suffix is still in its title")
	}
	if got := r.y("Albums by Radiohead"); got < d {
		t.Errorf("Albums by Radiohead (%v) is above the Discography band (%v)", got, d)
	}
	if r.y("Albums featuring Radiohead") < rel {
		t.Error("Albums featuring is above the Related band")
	}
}

func TestPageAsksTheCatalogueOnceAndShowsSeeAllWhereThereIsMore(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Discography")
	if got := r.cat.asked(); !slices.Equal(got, []string{"artist 657515"}) {
		t.Errorf("asked %q", got)
	}
	// Top songs say there is more, singles has more, essentials has two of six.
	r.tt.Drain()
	n := 0
	for _, x := range r.tt.Texts() {
		if x == "See all" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("%d See all links, want 2 (top songs, singles); texts %q", n, r.tt.Texts())
	}
}

func TestSeeAllOpensTheViewPage(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Top songs")
	r.tt.Drain()
	if err := r.tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/artist/657515/view/top-songs" {
		t.Errorf("path %q", got)
	}
}

func TestOneReleaseGivesTheCountAndIsNotRepeatedInTheHeader(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 1200, func(r *rig) {
		p := radiohead()
		one := album("one", "Am Sorry - Single", "2016-05-03")
		p.Shelves = []services.ArtistShelf{shelfOf("singles", false, one)}
		p.LatestRelease, p.Genres, p.Origin, p.Bio, p.BornOrFormed = &one, nil, "", "", ""
		p.TopSongs = nil
		r.cat.page = p
	})
	r.shows("Artist · 1 release", "Discography")
	if r.tt.HasText("Latest release · May 3, 2016") || r.tt.HasText("Latest release") {
		t.Error("the one release is repeated in the header")
	}
	if !r.tt.HasText("Am Sorry") {
		t.Error("the release is not in the discography")
	}
	for _, w := range []string{"About Radiohead", "Read more", "Top songs"} {
		if r.tt.HasText(w) {
			t.Errorf("page has %q", w)
		}
	}
}

func TestReadMoveScrollsToAboutAndFocusesIt(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 600, func(r *rig) { r.cat.page = radiohead() })
	r.shows("Read more")
	r.tt.Drain()
	if r.a.page.pos.Y != 0 {
		t.Fatalf("scrolled to %v before the click", r.a.page.pos.Y)
	}
	if err := r.tt.Click("Read more"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the About band at the top", func() bool { return r.a.page.pos.Y > 100 && r.tt.Focused("About Radiohead") })
	if !r.tt.HasText("Formed in 1985.") {
		t.Errorf("the biography is not in the About band; texts %q", r.tt.Texts())
	}
}

func TestLatestReleaseOpensItsAlbum(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Latest release · May 3, 2016")
	if err := r.tt.Click("Hail to the Thief (Live Recordings 2003-2009)"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/album/latest" {
		t.Errorf("path %q", got)
	}
}

func TestLatestReleaseUpcomingShowsItsBadgeInPlaceOfTheDate(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 900, func(r *rig) {
		p := radiohead()
		a := album("soon", "Coming Home - EP", "2099-11-19")
		a.Prerelease, a.ReleaseAt, a.TrackCount = true, 4102444800000+18*24*3600*1000, 4
		p.LatestRelease = &a
		r.cat.page = p
	})
	r.shows("Coming Home")
	if !r.tt.HasText("Latest release · Out Jan 18, 2100") && !strings.Contains(strings.Join(r.tt.Texts(), "|"), "Latest release · Out") {
		t.Errorf("no release badge; texts %q", r.tt.Texts())
	}
	if !r.tt.HasText("EP · 4 songs") {
		t.Errorf("no kind line; texts %q", r.tt.Texts())
	}
}

func TestCoverShelfTilesLinkToTheirPages(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Albums by Radiohead", "In Rainbows")
	if err := r.tt.Click("In Rainbows"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/album/ir" {
		t.Errorf("path %q", got)
	}
}

// ---- playing ----

func TestPlayPlaysTheTopSongsAsAnArtistCollection(t *testing.T) {
	r := catalogRig(t, func(p *services.CatalogArtist) { p.TopSongs[1].Playable = false })
	r.shows("Radiohead", "Song 1")
	r.tt.Drain()
	if err := r.tt.Click("Play Radiohead's top songs"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the queue to start", func() bool { return len(r.play.played()) == 1 })
	got := r.play.played()[0]
	if len(got.refs) != 7 || got.start != 0 || got.from != (services.PlayFrom{Kind: "artist", ID: "657515"}) {
		t.Errorf("played %+v", got)
	}
	if slices.ContainsFunc(got.refs, func(ref services.TrackRef) bool { return ref.ID == "902" }) {
		t.Error("an unplayable song was queued")
	}
}

func TestShufflePlaysAllTopSongsWithShuffleOn(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Shuffle")
	if err := r.tt.Click("Shuffle"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the queue to start", func() bool { return len(r.play.played()) == 1 })
	got := r.play.played()[0]
	if !r.play.shuffle || len(got.refs) != 8 || got.from.Kind != "artist" {
		t.Errorf("shuffle %v, played %+v", r.play.shuffle, got)
	}
}

func TestPlayIsDisabledWhenNoTopSongCanPlay(t *testing.T) {
	r := catalogRig(t, func(p *services.CatalogArtist) {
		for i := range p.TopSongs {
			p.TopSongs[i].Playable = false
		}
	})
	r.shows("Radiohead", "Song 1")
	r.tt.Drain()
	_ = r.tt.Click("Play Radiohead's top songs")
	r.tt.Drain()
	if n := len(r.play.played()); n != 0 {
		t.Errorf("a disabled Play played %d times", n)
	}
}

func TestNoPlayButtonsWithoutTopSongs(t *testing.T) {
	r := catalogRig(t, func(p *services.CatalogArtist) { p.TopSongs = nil })
	r.shows("Discography")
	if r.tt.HasText("Shuffle") || r.tt.HasText("Top songs") {
		t.Errorf("Play buttons or top songs without songs; texts %q", r.tt.Texts())
	}
}

func TestTopSongDoubleClickPlaysFromThatRow(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Song 3")
	r.tt.Drain()
	b, _ := r.tt.Find("Song 3")
	x, y := b.X+b.W/2, b.Y+b.H/2
	r.tt.ClickAt(x, y)
	r.tt.ClickAt(x, y)
	r.tt.Until("the queue to start", func() bool { return len(r.play.played()) == 1 })
	got := r.play.played()[0]
	if got.start != 2 || len(got.refs) != 8 || got.from.ID != "657515" {
		t.Errorf("played %+v", got)
	}
}

func TestRatingButtonsFavouriteTheArtist(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Favourite Radiohead")
	// The buttons wait for the artist's state.
	r.tt.Until("the state to load", func() bool {
		r.states.mu.Lock()
		defer r.states.mu.Unlock()
		return r.states.asked > 0
	})
	r.tt.Drain()
	if err := r.tt.Click("Favourite Radiohead"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the rating to be saved", func() bool { return r.tt.HasText("Undo favourite for Radiohead") })
	if !slices.Equal(r.states.faved, []bool{true}) {
		t.Errorf("favourited %v", r.states.faved)
	}
}

func TestHeaderMenuListsTheArtistActions(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("More for Radiohead")
	if err := r.tt.Click("More for Radiohead"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if !r.tt.HasText("Test artist action") {
		t.Errorf("menu not open; texts %q", r.tt.Texts())
	}
}

func TestTopSongRowMenuHasGoToArtist(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Song 2")
	r.tt.Drain()
	b, _ := r.tt.Find("Song 2")
	r.tt.RightClickAt(b.X+b.W/2, b.Y+b.H/2)
	r.tt.Drain()
	if !r.tt.HasText("Go to artist") {
		t.Fatalf("the song's menu has no Go to artist; texts %q", r.tt.Texts())
	}
	if err := r.tt.Click("Go to artist"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.router.Path(); got != "/artist/657515" {
		t.Errorf("path %q", got)
	}
}

// ---- the release list ----

func TestReleaseRowClickOpensCmdClickSelects(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Essential albums by Radiohead", "Kid A")
	r.tt.Drain()
	r.clickWith(ui.Cmd, "Kid A")
	if got := r.router.Path(); got != "/artist/657515" {
		t.Fatalf("a Cmd+click opened %q", got)
	}
	list := r.a.page.list("essential")
	if !list.sel.Selected("kida") || list.sel.Len() != 1 {
		t.Errorf("selected %v", list.sel.State().Keys())
	}
	r.clickWith(ui.Cmd, "OK Computer")
	if list.sel.Len() != 2 {
		t.Errorf("selected %v", list.sel.State().Keys())
	}
	// A plain click opens the album.
	if err := r.tt.Click("Kid A"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/album/kida" {
		t.Errorf("path %q", got)
	}
}

func TestReleaseRowArrowsMoveBetweenRows(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("OK Computer", "Kid A")
	r.tt.Drain()
	b, _ := r.tt.Find("OK Computer")
	r.tt.ClickAtWith(ui.Cmd, b.X+b.W/2, b.Y+b.H/2) // focuses (and selects) the first row
	r.tt.Key(0, ui.KeyDown)
	r.tt.Drain()
	if !r.tt.Focused("Kid A") {
		t.Errorf("Down did not move the focus to Kid A")
	}
	if list := r.a.page.list("essential"); list.nav.Focus() != 1 {
		t.Errorf("focus at %d", list.nav.Focus())
	}
}

func TestReleaseRowContextMenuShowsAlbumActions(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Kid A")
	r.tt.Drain()
	if err := r.tt.RightClick("Kid A"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if !r.tt.HasText("Test album action") {
		t.Errorf("no menu; texts %q", r.tt.Texts())
	}
}

func TestExplicitReleaseIsWithheldFromARestrictedAccount(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 1200, func(r *rig) {
		r.cat.page = radiohead()
		r.cat.page.Shelves[0].Albums[1].Explicit = true
		r.auth.restricted = true
	})
	r.shows("Kid A")
	r.tt.Drain()
	if !r.tt.HasText("Explicit") {
		t.Errorf("no explicit mark; texts %q", r.tt.Texts())
	}
	if _, ok := r.tt.Find("Kid A"); !ok {
		t.Fatal("no Kid A")
	}
}

func TestReleaseListsFlowDownTheFirstColumn(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 1200, func(r *rig) {
		r.cat.page = radiohead()
		r.cat.page.Shelves[0].Albums = nil
		for i := range 4 {
			r.cat.page.Shelves[0].Albums = append(r.cat.page.Shelves[0].Albums, album(fmt.Sprint("e", i), fmt.Sprint("Essential ", i), "2001-01-01"))
		}
		r.cat.page.TopSongs, r.cat.page.TopSongsMore = nil, false
	})
	r.shows("Essential 0", "Essential 3")
	if a, b := r.y("Essential 0"), r.y("Essential 1"); !(a < b) {
		t.Errorf("not down the column: %v %v", a, b)
	}
	// Without top songs the list has the lead to itself, in one column.
	b0, _ := r.tt.Find("Essential 0")
	b3, _ := r.tt.Find("Essential 3")
	if b0.X != b3.X {
		t.Errorf("columns: %v vs %v", b0.X, b3.X)
	}
}

// ---- loading, errors ----

func TestLoadingShowsTheSkeleton(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 900, func(r *rig) {
		r.cat.page = radiohead()
		r.cat.gate = make(chan struct{})
	})
	r.tt.Drain()
	if !r.tt.HasText("Loading") {
		t.Errorf("no Loading status; texts %q", r.tt.Texts())
	}
	if r.tt.HasText("Discography") {
		t.Error("the page shows before its data")
	}
	close(r.cat.gate)
	r.shows("Discography")
	if r.tt.HasText("Loading") {
		t.Error("Loading is still shown")
	}
}

func TestNotFoundIsExplainedWithAWayBack(t *testing.T) {
	r := newRig(t, "/artist/1", 1132, 900, func(r *rig) { r.cat.fail(applemusic.ErrNotFound) })
	r.shows("This artist isn't available", "Go to Artists")
	if err := r.tt.Click("Go to Artists"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/library/artists" {
		t.Errorf("path %q", got)
	}
}

func TestLoadFailureOffersRetry(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 900, func(r *rig) {
		r.cat.page = radiohead()
		r.cat.fail(errors.New("boom"))
	})
	r.shows("Couldn't load this artist", "Try again")
	r.cat.fail(nil)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.shows("Discography")
	if got := r.cat.asked(); len(got) < 2 {
		t.Errorf("asked %q", got)
	}
}

func TestNoWayBackWhenTheArtistsPageDoesNotExist(t *testing.T) {
	r := newRig(t, "/artist/1", 1132, 900, func(r *rig) { r.cat.fail(applemusic.ErrNotFound) })
	r.shows("This artist isn't available")
	r.a.env.HasRoute = func(p string) bool { return p != "/library/artists" && hasPage(p) }
	r.tt.Drain()
	if r.tt.HasText("Go to Artists") {
		t.Error("a link to a page the app lacks")
	}
}

// ---- the library artist ----

func libraryDetail() services.ArtistDetail {
	lib := func(id, name string, single bool) services.Album {
		return services.Album{ID: id, Name: name, Artist: "Mia", ArtistID: "r.1", ReleaseDate: "2020-01-01", Single: single, Art: art, Source: services.SourceLibrary, LibraryID: id}
	}
	s := func(i int, playable bool) services.Song {
		return services.Song{ID: fmt.Sprint("i.", i), Source: services.SourceLibrary, LibraryID: fmt.Sprint("i.", i), Name: fmt.Sprint("Mine ", i), Artist: "Mia", Album: "Own Album", AlbumID: "l.1", DurationMs: 200000, Playable: playable, Art: art}
	}
	return services.ArtistDetail{
		Artist: services.Artist{ID: "r.1", Name: "Mia", Source: services.SourceLibrary, LibraryID: "r.1", CatalogID: "55", Art: art},
		Albums: []services.Album{lib("l.1", "Own Album", false), lib("l.2", "Short One", true), lib("l.3", "Second Album", false)},
		Songs:  []services.Song{s(1, true), s(2, false), s(3, true)},
	}
}

func libraryRig(t *testing.T, withCatalogue bool) *rig {
	return newRig(t, "/artist/r.1", 1132, 1700, func(r *rig) {
		r.lib = fakeLibrary{detail: libraryDetail()}
		if withCatalogue {
			p := radiohead()
			p.Artist = services.Artist{ID: "55", Name: "Mia", Source: services.SourceCatalog, CatalogID: "55", Art: art}
			p.Genres, p.Origin = []string{"Pop"}, ""
			r.cat.page = p
		} else {
			r.lib.detail.Artist.CatalogID = ""
		}
	})
}

func TestLibraryArtistShowsWhatIsInTheLibraryFirst(t *testing.T) {
	r := libraryRig(t, false)
	r.shows("Mia", "Albums in your library", "Songs in your library")
	for _, w := range []string{
		"Artist · 3 albums, 3 songs in your library", "Albums in your library", "Own Album", "Second Album",
		"Singles and EPs in your library", "Short One", "Songs in your library", "Songs by Mia in your library", "Mine 1", "Mine 3",
		"Play Mia",
	} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
	if got := r.cat.asked(); len(got) != 0 {
		t.Errorf("a library artist with no catalogue ID asked the catalogue: %q", got)
	}
	for _, w := range []string{"Discography", "Read more", "Favourite Mia", "More for Mia"} {
		if r.tt.HasText(w) {
			t.Errorf("page has %q without a catalogue page", w)
		}
	}
	if r.y("Albums in your library") > r.y("Singles and EPs in your library") || r.y("Singles and EPs in your library") > r.y("Songs in your library") {
		t.Error("shelves out of order")
	}
}

func TestLibraryArtistPlaysItsSongsAndSkipsUnplayableOnes(t *testing.T) {
	r := libraryRig(t, false)
	r.shows("Play Mia")
	if err := r.tt.Click("Play Mia"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the queue to start", func() bool { return len(r.play.played()) == 1 })
	got := r.play.played()[0]
	if len(got.refs) != 2 || got.start != 0 || got.from != (services.PlayFrom{Kind: "artist", ID: "r.1"}) {
		t.Errorf("played %+v", got)
	}
	// Enter on the row of Mine 3 plays from it (its place among the playable ones).
	b, _ := r.tt.Find("Mine 3")
	r.tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	r.tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	r.tt.Until("the second queue", func() bool { return len(r.play.played()) == 2 })
	if got := r.play.played()[1]; got.start != 1 || len(got.refs) != 2 {
		t.Errorf("played %+v", got)
	}
}

func TestLibraryArtistWithACataloguePageAddsItsLeadAndBands(t *testing.T) {
	r := libraryRig(t, true)
	r.shows("Mia", "Discography", "Related")
	for _, w := range []string{
		"Artist · Pop · 3 albums, 3 songs in your library", "More than a band.", "Read more",
		"Latest release", "Favourite Mia", "More for Mia", "Top songs", "Essential albums by Mia",
	} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
	if got := r.cat.asked(); !slices.Equal(got, []string{"artist 55"}) {
		t.Errorf("asked %q", got)
	}
	if r.y("Songs in your library") > r.y("Top songs") {
		t.Error("the catalogue body is above the library's songs")
	}
	// Library Play keeps the artist's name (the catalogue page's is "top songs").
	if r.tt.HasText("Play Mia's top songs") {
		t.Error("the library artist offers the top songs' Play")
	}
}

func TestLibraryArtistFailsLikeTheCatalogueOne(t *testing.T) {
	r := newRig(t, "/artist/r.1", 1132, 900, func(r *rig) { r.lib = fakeLibrary{err: errors.New("boom")} })
	r.shows("Couldn't load this artist", "Try again")
}

// ---- the see-all page ----

func viewRig(t *testing.T, view string, mutate func(*rig)) *rig {
	return newRig(t, "/artist/657515/view/"+view, 1132, 900, func(r *rig) {
		r.cat.page = radiohead()
		if mutate != nil {
			mutate(r)
		}
	})
}

func songPages(total int) func(int) services.ArtistViewPage {
	return func(offset int) services.ArtistViewPage {
		end := min(offset+viewPage, total)
		p := services.ArtistViewPage{View: "top-songs", Kind: services.ShelfSongs, Next: end}
		if end >= total {
			p.Next = -1
		}
		for i := offset; i < end; i++ {
			p.Songs = append(p.Songs, song(i+1))
		}
		return p
	}
}

func albumPages(view string, total int) func(int) services.ArtistViewPage {
	return func(offset int) services.ArtistViewPage {
		end := min(offset+viewPage, total)
		p := services.ArtistViewPage{View: view, Kind: services.ShelfAlbums, Next: end}
		if end >= total {
			p.Next = -1
		}
		for i := offset; i < end; i++ {
			p.Albums = append(p.Albums, album(fmt.Sprint("al", i), fmt.Sprint("Record ", i), "2001-01-01"))
		}
		return p
	}
}

func TestViewPageTopSongsAreATrackListThatPlays(t *testing.T) {
	r := viewRig(t, "top-songs", func(r *rig) { r.cat.views["top-songs"] = songPages(30) })
	r.shows("Top songs by Radiohead", "Song 1", "Song 30")
	if !r.tt.HasText("Radiohead") {
		t.Error("no subline")
	}
	if got := r.cat.asked(); !slices.Contains(got, "view 657515 top-songs 0/50") {
		t.Errorf("asked %q", got)
	}
	r.tt.Drain()
	b, _ := r.tt.Find("Song 5")
	r.tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	r.tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	r.tt.Until("the queue", func() bool { return len(r.play.played()) == 1 })
	if got := r.play.played()[0]; got.start != 4 || len(got.refs) != 30 || got.from.Kind != "artist" {
		t.Errorf("played %+v", got)
	}
}

func TestViewPageSublineLinksBackToTheArtist(t *testing.T) {
	r := viewRig(t, "full-albums", func(r *rig) { r.cat.views["full-albums"] = albumPages("full-albums", 3) })
	r.shows("Albums by Radiohead", "Record 2")
	if err := r.tt.Click("Radiohead"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/artist/657515" {
		t.Errorf("path %q", got)
	}
}

func TestViewPageShowsAGridOfTilesAndLinksThem(t *testing.T) {
	r := viewRig(t, "full-albums", func(r *rig) { r.cat.views["full-albums"] = albumPages("full-albums", 5) })
	r.shows("Record 0", "Record 4", "2001")
	if err := r.tt.Click("Record 3"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/album/al3" {
		t.Errorf("path %q", got)
	}
}

func TestViewPageFetchesFiftyAtATimeAsItScrolls(t *testing.T) {
	r := viewRig(t, "top-songs", func(r *rig) { r.cat.views["top-songs"] = songPages(420) })
	r.shows("Top songs by Radiohead", "Song 1")
	r.tt.Until("the lists to settle", func() bool {
		n := len(r.cat.asked())
		r.tt.Drain()
		return len(r.cat.asked()) == n && n > 1
	})
	got := r.cat.asked()
	for _, c := range got {
		if strings.HasPrefix(c, "view") && !strings.HasSuffix(c, "/50") {
			t.Errorf("page size in %q", c)
		}
	}
	if slices.Contains(got, "view 657515 top-songs 400/50") {
		t.Fatalf("everything was fetched before the list was scrolled: %q", got)
	}
	// The end of the list comes into view: the rest follows.
	r.tt.Until("the last page", func() bool {
		r.a.view.pos.Y = 1e9 // the user keeps scrolling down
		return slices.Contains(r.cat.asked(), "view 657515 top-songs 400/50")
	})
}

func TestViewPageShowsPlaceholdersWhileLoading(t *testing.T) {
	r := viewRig(t, "top-songs", func(r *rig) {
		r.cat.views["top-songs"] = songPages(30)
		r.cat.viewGate = make(chan struct{})
	})
	r.tt.Drain()
	loading := func() int {
		n := 0
		for _, x := range r.tt.Texts() {
			if x == "Loading" {
				n++
			}
		}
		return n
	}
	if n := loading(); n < loadingFirst {
		t.Errorf("%d placeholder rows while loading, want %d; texts %q", n, loadingFirst, r.tt.Texts())
	}
	if r.tt.HasText("Nothing here yet") {
		t.Error("the empty state shows while the first page loads")
	}
	close(r.cat.viewGate)
	r.shows("Song 1", "Song 30")
	if n := loading(); n != 0 {
		t.Errorf("%d placeholder rows after loading", n)
	}
}

func TestViewPageEmptyList(t *testing.T) {
	r := viewRig(t, "singles", func(r *rig) {
		r.cat.views["singles"] = func(int) services.ArtistViewPage {
			return services.ArtistViewPage{View: "singles", Kind: services.ShelfAlbums, Next: -1}
		}
	})
	r.shows("Nothing here yet", "Apple Music has nothing in this list for Radiohead in your storefront.")
}

func TestViewPageUnknownView(t *testing.T) {
	r := viewRig(t, "nonsense", nil)
	r.shows("Not found", "There's no such list", "This artist page doesn't have a list called “nonsense”.", "Go to Artists")
	for _, c := range r.cat.asked() {
		if strings.HasPrefix(c, "view") {
			t.Errorf("an unknown view was fetched: %q", c)
		}
	}
	if err := r.tt.Click("Go to Artists"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/library/artists" {
		t.Errorf("path %q", got)
	}
}

func TestViewPageFailureOffersRetry(t *testing.T) {
	r := viewRig(t, "singles", func(r *rig) {
		r.cat.views["singles"] = albumPages("singles", 2)
		r.cat.err = errors.New("boom")
	})
	r.shows("Couldn't load this list", "Try again")
	r.cat.fail(nil)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.shows("Record 0", "Record 1")
}

func TestViewPageTitlesNameTheArtistOrSayThisArtist(t *testing.T) {
	r := viewRig(t, "playlists", func(r *rig) {
		r.cat.views["playlists"] = func(int) services.ArtistViewPage {
			return services.ArtistViewPage{View: "playlists", Kind: services.ShelfPlaylists, Next: -1,
				Playlists: []services.Playlist{{ID: "pl.1", Name: "Mix One", Curator: "Apple", Art: art}}}
		}
	})
	r.shows("Playlists featuring Radiohead", "Mix One", "Apple")
}

// ---- the song row's "Go to artist" ----

func TestGoToArtistIsListedForSongsThatNameAnArtist(t *testing.T) {
	r := catalogRig(t, nil)
	reg := r.a.env.Actions
	var got *kit.ActionDef
	for _, d := range reg.For(kit.KindSong) {
		if d.ID == "artist.go-to" {
			got = d
		}
	}
	if got == nil {
		t.Fatal("no artist.go-to on songs")
	}
	if got.Group != kit.GroupItem || got.Order != 20 {
		t.Errorf("group %v order %d", got.Group, got.Order)
	}
	named, anon := song(1), song(2)
	anon.ArtistID = ""
	ctx := reg.Context([]kit.Item{kit.SongItem(named)}, nil, nil)
	if got.Label(ctx) != "Go to artist" || !got.Available(ctx) {
		t.Errorf("label %q, available %v", got.Label(ctx), got.Available(ctx))
	}
	if got.Available(reg.Context([]kit.Item{kit.SongItem(anon)}, nil, nil)) {
		t.Error("offered for a song with no artist")
	}
	got.Run(ctx)
	if p := r.router.Path(); p != "/artist/657515" {
		t.Errorf("went to %q", p)
	}
}

// ---- layout ----

func TestLatestReleaseSitsBesideTheTextWhenWideAndBelowWhenNarrow(t *testing.T) {
	wide := catalogRig(t, nil)
	wide.shows("Latest release · May 3, 2016")
	slip, _ := wide.tt.Find("Latest release · May 3, 2016")
	name, _ := wide.tt.Find("Radiohead")
	if !(slip.X > name.X+300) || slip.Y > wide.y("Shuffle") {
		t.Errorf("wide: slip at %v, name at %v", slip, name)
	}

	narrow := newRig(t, "/artist/657515", 760, 1400, func(r *rig) { r.cat.page = radiohead() })
	narrow.shows("Latest release · May 3, 2016")
	if !(narrow.y("Latest release · May 3, 2016") > narrow.y("Shuffle")) {
		t.Error("narrow: the latest release is not under the actions")
	}
	// Essential albums stack under the top songs.
	if !(narrow.y("Essential albums by Radiohead") > narrow.y("Song 6")) {
		t.Error("narrow: essential albums beside the top songs")
	}
}

func TestCatalogueListsShareARowWhenTheyFitAndStackWhenTheyDoNot(t *testing.T) {
	wide := catalogRig(t, nil)
	wide.shows("Singles and EPs featuring Radiohead", "Compilations featuring Radiohead")
	if a, b := wide.y("Singles and EPs featuring Radiohead"), wide.y("Compilations featuring Radiohead"); a != b {
		t.Errorf("wide: lists at %v and %v", a, b)
	}
	narrow := newRig(t, "/artist/657515", 500, 2400, func(r *rig) { r.cat.page = radiohead() })
	narrow.shows("Singles and EPs featuring Radiohead", "Compilations featuring Radiohead")
	if a, b := narrow.y("Singles and EPs featuring Radiohead"), narrow.y("Compilations featuring Radiohead"); !(a < b) {
		t.Errorf("narrow: lists at %v and %v", a, b)
	}
}

func TestRestrictedAccountFollowsTheAuthEvent(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Discography")
	if r.a.restricted() {
		t.Fatal("restricted before the account said so")
	}
	r.h.Bus.Emit(services.EventAuthChanged, services.AuthStatus{Restricted: true})
	r.tt.Drain()
	if !r.a.restricted() {
		t.Error("the event did not restrict the page")
	}
}

// ---- goldens ----

func goldenRig(t *testing.T, dark bool) *rig {
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	r := newRig(t, "/artist/657515", 1132, 1253, func(r *rig) { r.cat.page = radiohead() })
	theme.SetMode(map[bool]theme.Mode{true: theme.ModeDark, false: theme.ModeLight}[dark])
	return r
}

func TestGoldenArtistLight(t *testing.T) {
	r := goldenRig(t, false)
	r.shows("Discography")
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "artist_light")
}

func TestGoldenArtistDark(t *testing.T) {
	r := goldenRig(t, true)
	r.tt.SetDark(true)
	r.shows("Discography")
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "artist_dark")
}
