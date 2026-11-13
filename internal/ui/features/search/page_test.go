package search

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// pagePlay records what the pages play.
type pagePlay struct {
	queueStub
	mu    sync.Mutex
	refs  [][]services.TrackRef
	start []int
	from  []services.PlayFrom
}

func (p *pagePlay) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refs, p.start, p.from = append(p.refs, refs), append(p.start, start), append(p.from, from)
	return nil
}

func (p *pagePlay) plays() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.refs)
}

// pageApple is a fake Apple Music API: the catalogue's search (the groups
// of "blue"), the "see all" pages of its songs, the browse categories and a
// listener's profile, in the JSON Apple sends.
type pageApple struct {
	mu    sync.Mutex
	fail  map[string]int // path -> status to answer
	asked []string       // request URIs
	gate  chan struct{}  // searches for "slow" wait for it to close
	// songsFirst is how many songs the first results page holds; songsTotal
	// how many the group has; songsPage the size of a "see all" page.
	songsFirst, songsTotal, songsPage int
	// failFrom: "see all" pages from this offset on fail (0: none).
	failFrom int
	// private and verified shape the profile; playlists how many it shares.
	private, verified bool
	playlists         int
}

func newPageApple() *pageApple {
	return &pageApple{fail: map[string]int{}, songsFirst: 10, songsTotal: 20, songsPage: 12, playlists: 2}
}

func (a *pageApple) setFail(path string, status int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if status == 0 {
		delete(a.fail, path)
	} else {
		a.fail[path] = status
	}
}

func (a *pageApple) requests(substr string) (n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, u := range a.asked {
		if strings.Contains(u, substr) {
			n++
		}
	}
	return n
}

func artwork(i int) string {
	return fmt.Sprintf(`"artwork":{"url":%q,"width":1000,"height":1000}`, fixture.Template(i))
}

func songJSON(n int) string {
	return fmt.Sprintf(`"%d":{"id":"%d","type":"songs","attributes":{"name":"Blue Song %d","artistName":"Blue Band","albumName":"Blue Hour",`+
		`"durationInMillis":180000,"playParams":{"id":"%d","kind":"song"},%s}}`, n, n, n, n, artwork(n))
}

func dataRefs(typ string, ids ...int) string {
	var s []string
	for _, id := range ids {
		s = append(s, fmt.Sprintf(`{"id":"%d","type":%q}`, id, typ))
	}
	return "[" + strings.Join(s, ",") + "]"
}

func seq(from, to int) []int {
	var out []int
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

func (a *pageApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.asked = append(a.asked, r.URL.RequestURI())
	status, gate := a.fail[r.URL.Path], a.gate
	a.mu.Unlock()
	if status != 0 {
		http.Error(w, "down", status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	switch r.URL.Path {
	case "/v1/catalog/gb/search":
		if strings.HasPrefix(q.Get("term"), "slow") && gate != nil {
			<-gate
		}
		if q.Get("term") == "nothing" {
			_, _ = w.Write([]byte(`{"results":{},"resources":{},"meta":{"results":{"order":[]}}}`))
			return
		}
		if g := q.Get("groups"); g != "" {
			a.groupPage(w, g, q.Get("offset"))
			return
		}
		songs := seq(1, a.songsFirst)
		var res []string
		for _, n := range songs {
			res = append(res, songJSON(n))
		}
		next := ""
		if a.songsTotal > a.songsFirst {
			next = fmt.Sprintf(`"next":"/v1/catalog/gb/search?groups=song&offset=%d&term=blue",`, a.songsFirst)
		}
		_, _ = fmt.Fprintf(w, `{"results":{
			"top":{"data":[{"id":"9","type":"artists"},{"id":"5","type":"albums"},{"id":"1","type":"songs"}]},
			"artist":{"data":[{"id":"9","type":"artists"}]},
			"album":{"data":[{"id":"5","type":"albums"}]},
			"song":{%s"data":%s},
			"playlist":{"data":[{"id":"pl.1","type":"playlists"}]}},
			"resources":{
			"songs":{%s},
			"artists":{"9":{"id":"9","type":"artists","attributes":{"name":"Blue Band","genreNames":["Rock"],%s}}},
			"albums":{"5":{"id":"5","type":"albums","attributes":{"name":"Blue Hour","artistName":"Blue Band","trackCount":10,%s}}},
			"playlists":{"pl.1":{"id":"pl.1","type":"playlists","attributes":{"name":"Blue Mix","curatorName":"Apple Music",%s}}}},
			"meta":{"results":{"order":["top","artist","album","song","playlist"]}}}`,
			next, dataRefs("songs", songs...), strings.Join(res, ","), artwork(9), artwork(5), artwork(7))
	case "/v1/recommendations/gb":
		_, _ = w.Write([]byte(`{"data":[{"id":"r1","type":"personal-recommendation"},{"id":"r2","type":"personal-recommendation"}],"resources":{
			"personal-recommendation":{
				"r1":{"id":"r1","type":"personal-recommendation","relationships":{"contents":{"data":[{"id":"988556214","type":"apple-curators"}]}}},
				"r2":{"id":"r2","type":"personal-recommendation","relationships":{"contents":{"data":[{"id":"6447688262","type":"editorial-items"}]}}}},
			"apple-curators":{"988556214":{"id":"988556214","type":"apple-curators","attributes":{"name":"Apple Music Alternative","shortName":"Alternative","kind":"Genre"}}},
			"editorial-items":{"6447688262":{"id":"6447688262","type":"editorial-items","attributes":{"plainEditorialNotes":{"name":"Ibiza"},
				"link":{"url":"https://music.apple.com/gb/multi-room/6447561826","target":"internal"}}}}}}`))
	case "/v1/social/gb/social-profiles":
		var pls, refs []string
		for i := 1; i <= a.playlists; i++ {
			refs = append(refs, fmt.Sprintf(`{"id":"pl.u-%d","type":"playlists"}`, i))
			pls = append(pls, fmt.Sprintf(`"pl.u-%d":{"id":"pl.u-%d","type":"playlists","attributes":{"name":"Shared Mix %d","curatorName":"Taylor",%s}}`, i, i, i, artwork(i)))
		}
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"sp.1","type":"social-profiles"}],"resources":{
			"social-profiles":{"sp.1":{"id":"sp.1","type":"social-profiles","attributes":{"name":"Taylor Listener","handle":"taylor","isVerified":%t,"isPrivate":%t},
				"relationships":{"shared-playlists":{"data":[%s]}}}},
			"playlists":{%s}}}`,
			a.verified, a.private, strings.Join(refs, ","), strings.Join(pls, ","))
	default:
		http.NotFound(w, r)
	}
}

// groupPage answers a "see all" page of the songs from offset.
func (a *pageApple) groupPage(w http.ResponseWriter, group, offset string) {
	var from int
	_, _ = fmt.Sscan(offset, &from)
	a.mu.Lock()
	failFrom := a.failFrom
	a.mu.Unlock()
	if failFrom > 0 && from >= failFrom {
		http.Error(w, "down", http.StatusInternalServerError)
		return
	}
	if group == "album" {
		_, _ = fmt.Fprintf(w, `{"results":{"album":{"data":%s}},"resources":{"albums":{`+
			`"5":{"id":"5","type":"albums","attributes":{"name":"Blue Hour","artistName":"Blue Band",%s}},`+
			`"6":{"id":"6","type":"albums","attributes":{"name":"Blue Monday","artistName":"New Order",%s}}}}}`,
			dataRefs("albums", 5, 6), artwork(5), artwork(6))
		return
	}
	to := min(from+a.songsPage, a.songsTotal)
	songs := seq(from+1, to)
	var res []string
	for _, n := range songs {
		res = append(res, songJSON(n))
	}
	next := ""
	if to < a.songsTotal {
		next = fmt.Sprintf(`"next":"/v1/catalog/gb/search?groups=song&offset=%d&term=blue",`, to)
	}
	_, _ = fmt.Fprintf(w, `{"results":{"song":{%s"data":%s}},"resources":{"songs":{%s}}}`, next, dataRefs("songs", songs...), strings.Join(res, ","))
}

// pageRig is the pages over a real SearchService, a fake Apple Music API on
// the harness network and the seeded library.
type pageRig struct {
	t      *testing.T
	h      *uitest.Harness
	router *ui.Router
	s      *search
	api    *services.SearchService
	play   *pagePlay
	apple  *pageApple
	tt     *uitest.Session
}

var pageAlbums = []fixture.Album{
	{Name: "Blue Hour", Artist: "Al"},
	{Name: "Blue Monday", Artist: "Bo"},
	{Name: "Paper Planes", Artist: "Di"},
}

type rigOpts struct {
	routes   func(string) bool // Env.HasRoute; every page by default
	signedIn bool
	width    int
	height   int
	// setup shapes the fake API before the first frame asks it.
	setup func(*pageApple)
	// wrap replaces the service the feature asks, before the first frame.
	wrap func(searchAPI) searchAPI
}

func newPageRig(t *testing.T, start string, ro rigOpts, opts ...uitest.Option) *pageRig {
	t.Helper()
	r := &pageRig{t: t, play: &pagePlay{}, apple: newPageApple()}
	if ro.setup != nil {
		ro.setup(r.apple)
	}
	opts = append([]uitest.Option{uitest.Albums(pageAlbums...)}, opts...)
	if ro.signedIn {
		opts = append(opts, uitest.Apple(r.apple))
	}
	r.h = uitest.New(t, opts...)
	r.h.Network.ServeHandler(ampHost, r.apple)
	r.router = ui.NewRouter(start)
	r.h.Env.Nav = r.router
	r.h.Env.Surfaces = store.NewSurfaces()
	r.h.Env.HasRoute = ro.routes
	if ro.routes == nil {
		r.h.Env.HasRoute = func(string) bool { return true }
	}
	r.api = services.Service[services.SearchService](r.h.Core)
	var api searchAPI = r.api
	if ro.wrap != nil {
		api = ro.wrap(api)
	}
	useQueue(r.h.Env, r.play)
	kit.SetToastPoster(r.h.Main.Post)
	t.Cleanup(func() { kit.CloseToast("") })
	r.s = newSearch(r.h.Env, api)
	w, h := ro.width, ro.height
	if w == 0 {
		w, h = 1132, 1300
	}
	r.tt = r.h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Background(theme.C("ground")).Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				switch {
				case rt.Match(searchPath):
					r.s.searchPage(c, rt)
				case rt.Match(groupPath):
					r.s.groupPage(c, rt)
				case rt.Match(profilePath):
					r.s.profilePage(c, rt)
				}
			})
		})
		kit.Toaster(c)
	}, w, h)
	return r
}

// loc is the router's location (path and query).
func (r *pageRig) loc() string { return r.router.Location() }

func (r *pageRig) recentKinds() []string {
	r.t.Helper()
	got, err := r.api.Recent(context.Background())
	if err != nil {
		r.t.Fatal(err)
	}
	var out []string
	for _, e := range got {
		if e.Kind == services.RecentTerm {
			out = append(out, "term:"+e.Scope+":"+e.Term)
		} else {
			out = append(out, e.Kind+":"+e.Name)
		}
	}
	return out
}

func (r *pageRig) untilRecent(want ...string) {
	r.t.Helper()
	r.tt.Until("the remembered entries", func() bool {
		got := r.recentKinds()
		for _, w := range want {
			if !slices.Contains(got, w) {
				return false
			}
		}
		return true
	})
}

// position is where text first shows, for order checks.
func (r *pageRig) position(text string) float32 {
	r.t.Helper()
	rect, ok := r.tt.Find(text)
	if !ok {
		r.t.Fatalf("%q is not on screen; texts %q", text, r.tt.Texts())
	}
	return rect.Y
}

func (r *pageRig) click(label string) {
	r.t.Helper()
	if err := r.tt.Click(label); err != nil {
		r.t.Fatal(err)
	}
}

func TestSearchPageWithoutAQueryShowsTheBrowseCategories(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{signedIn: true})
	r.tt.Until("the categories", r.tt.Shows("Search", "Browse categories", "Alternative", "Ibiza"))
	for _, w := range []string{"Search in", "Apple Music", "Library"} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q", w)
		}
	}
	if r.tt.HasText("Recently searched") {
		t.Error("no recent searches yet, but the shelf shows")
	}
}

func TestSearchPageBrowseOpensACategoryAndRemembersIt(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{signedIn: true})
	r.tt.Until("the categories", r.tt.Shows("Alternative"))
	r.click("Alternative")
	if got := r.loc(); got != "/curator/988556214" {
		t.Errorf("location %q, want the curator's page", got)
	}
	r.untilRecent("curator:Alternative")
}

func TestSearchPageBrowseFailureOffersRetry(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{signedIn: true, setup: func(a *pageApple) { a.fail["/v1/recommendations/gb"] = 400 }})
	r.tt.Until("the failure", r.tt.Shows("Browse categories", "Couldn't load the categories"))
	r.apple.setFail("/v1/recommendations/gb", 0)
	r.click("Try again")
	r.tt.Until("the categories", r.tt.Shows("Alternative"))
	if r.tt.HasText("Couldn't load the categories") {
		t.Error("the error stays after the retry")
	}
}

func TestSearchPageBrowseWaitsForTheAccountState(t *testing.T) {
	// A Bare-less signed-out Core still knows its state, so the categories
	// are asked for: Browse is shown and (without a storefront) fails.
	r := newPageRig(t, "/search", rigOpts{})
	r.tt.Until("the shelf", r.tt.Shows("Browse categories"))
}

func TestSearchPageTitleIsTheTerm(t *testing.T) {
	r := newPageRig(t, "/search?q=%20blue%20", rigOpts{signedIn: true})
	r.tt.Until("the results", r.tt.Shows("“blue”", "Top results"))
	r2 := newPageRig(t, "/search?q=%20%20", rigOpts{signedIn: true})
	r2.tt.Until("the page", r2.tt.Shows("Search", "Browse categories"))
	if r2.tt.HasText("“”") {
		t.Error("a blank term got a title")
	}
}

func TestScopeSwitchReplacesTheHistoryKeepingTheTerm(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the results", r.tt.Shows("Top results"))
	r.click("Library")
	if got := r.loc(); got != "/search?q=blue&scope=library" {
		t.Errorf("after Library: %q", got)
	}
	r.tt.Until("the library results", r.tt.Shows("Blue Hour"))
	if !r.tt.HasText("Albums") {
		t.Error("the library's albums shelf is missing")
	}
	r.click("Apple Music")
	if got := r.loc(); got != "/search?q=blue" {
		t.Errorf("after Apple Music: %q, want the default scope left out", got)
	}
	r.router.Back()
	r.tt.Drain()
	if got := r.loc(); got == "/search?q=blue&scope=library" {
		t.Error("the switch pushed history instead of replacing it")
	}
}

func TestCatalogResultsKeepApplesOrderAndTitles(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the results", r.tt.Shows("Top results", "Artists", "Albums", "Songs", "Playlists"))
	prev := float32(-1)
	for _, title := range []string{"Top results", "Artists", "Albums", "Songs", "Playlists"} {
		y := r.position(title)
		if y <= prev {
			t.Errorf("%q at y=%v is not below the group before (y=%v)", title, y, prev)
		}
		prev = y
	}
}

func TestCatalogTopResultsAreRowsWithKindAndSubtitle(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the results", r.tt.Shows("Top results"))
	for _, w := range []string{"Artist", "Album · Blue Band", "Song · Blue Band"} {
		if !r.tt.HasText(w) {
			t.Errorf("top results lack %q; texts %q", w, r.tt.Texts())
		}
	}
	if len(r.tt.Texts()) == 0 {
		t.Fatal("nothing shown")
	}
}

func TestCatalogSeeAllOnlyWhenThereIsMore(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the results", r.tt.Shows("Songs"))
	// Songs: ten came, eight show, and Apple has another page: See all.
	r.click("See all")
	if got := r.loc(); got != "/search/song?q=blue" {
		t.Errorf("See all went to %q", got)
	}
}

func TestCatalogSongsShowEightAndPlayFromTheRow(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the songs", r.tt.Shows("Songs"))
	if r.tt.HasText("Blue Song 9") || r.tt.HasText("Blue Song 10") {
		t.Error("more than eight songs show in the shelf")
	}
	if !r.tt.HasText("Blue Song 8") {
		t.Fatalf("the eighth song is missing; texts %q", r.tt.Texts())
	}
	rect, _ := r.tt.Find("Blue Song 3")
	r.tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	r.tt.Frame()
	r.click("Play Blue Song 3")
	r.tt.Until("the song to play", func() bool { return r.play.plays() >= 1 })
	if r.play.plays() != 1 {
		t.Fatalf("played %d times", r.play.plays())
	}
	r.play.mu.Lock()
	defer r.play.mu.Unlock()
	if r.play.start[0] != 2 || len(r.play.refs[0]) != 8 || r.play.refs[0][2].ID != "3" {
		t.Errorf("played %v from %d, want the eight songs from index 2", r.play.refs[0], r.play.start[0])
	}
	if r.play.from[0].ID != "search" {
		t.Errorf("played from %+v, want search", r.play.from[0])
	}
}

func TestCatalogPlayingASongRemembersTheTermAndTheSong(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the songs", r.tt.Shows("Blue Song 3"))
	rect, _ := r.tt.Find("Blue Song 3")
	r.tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	r.tt.Frame()
	r.click("Play Blue Song 3")
	r.untilRecent("term:catalog:blue", "song:Blue Song 3")
}

func TestTopResultRowsOpenPagesAndRememberTheTermAndItem(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the results", r.tt.Shows("Top results", "Album · Blue Band"))
	r.click("Blue Band")
	// The first top result is the artist.
	if got := r.loc(); got != "/artist/9" {
		t.Errorf("opened %q, want the artist", got)
	}
	r.untilRecent("term:catalog:blue", "artist:Blue Band")
}

func TestTopResultSongRowPlaysInPlace(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the results", r.tt.Shows("Song · Blue Band"))
	r.click("Blue Song 1, Play")
	if got := r.loc(); got != "/search?q=blue" {
		t.Errorf("a song row navigated to %q", got)
	}
	r.tt.Until("the song to play", func() bool { return r.play.plays() >= 1 })
	if r.play.plays() != 1 {
		t.Fatalf("played %d times, want 1", r.play.plays())
	}
	r.untilRecent("song:Blue Song 1")
}

func TestResultsLinkOnlyToPagesThatAreThere(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true, routes: func(p string) bool { return strings.HasPrefix(p, "/artist/") }})
	r.tt.Until("the results", r.tt.Shows("Album · Blue Band"))
	// The album row is not interactive: no page for it.
	before := r.loc()
	_ = r.tt.Click("Album · Blue Band")
	if r.loc() != before {
		t.Errorf("a row of a page that is not ported navigated to %q", r.loc())
	}
}

func TestCatalogLoadingRowsThenResultsAndPreviousResultsStay(t *testing.T) {
	gate := make(chan struct{})
	r := newPageRig(t, "/search?q=slow", rigOpts{signedIn: true, setup: func(a *pageApple) { a.gate = gate }})
	r.tt.Drain()
	if _, ok := r.tt.Find("Searching"); !ok {
		t.Errorf("no loading rows while the search is out; texts %q", r.tt.Texts())
	}
	if r.tt.HasText("Top results") {
		t.Error("results before the answer")
	}
	close(gate)
	r.tt.Until("the results", r.tt.Shows("Top results"))
	// Another term: the results of the last stay while it loads.
	next := make(chan struct{})
	r.apple.mu.Lock()
	r.apple.gate = next
	r.apple.mu.Unlock()
	r.router.Replace("/search?q=slow")
	r.tt.Drain()
	// (Same term is cached; a new one asks again.)
	r.router.Replace("/search?q=slow%20again")
	r.tt.Drain()
	if !r.tt.HasText("Top results") {
		t.Errorf("the last results went while the next load; texts %q", r.tt.Texts())
	}
	close(next)
}

// noStorefront is a SearchService whose catalogue has no storefront: what a
// signed-out account answers.
type noStorefront struct{ searchAPI }

func (noStorefront) Catalog(context.Context, string) (services.SearchResults, error) {
	return services.SearchResults{}, catalog.ErrNoStorefront
}

func TestCatalogSignedOutOffersSignIn(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{wrap: func(a searchAPI) searchAPI { return noStorefront{a} }})
	r.tt.Until("the sign-in state", r.tt.Shows("Sign in to search Apple Music", "Sign in"))
	if r.h.Env.Surfaces.AccountOpen() {
		t.Fatal("the account dialog is open before the click")
	}
	r.click("Sign in")
	if !r.h.Env.Surfaces.AccountOpen() {
		t.Error("Sign in did not open the account dialog")
	}
}

func TestCatalogFailureOffersRetry(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true, setup: func(a *pageApple) { a.fail["/v1/catalog/gb/search"] = 400 }})
	r.tt.Until("the error", r.tt.Shows("Couldn't search Apple Music"))
	r.apple.setFail("/v1/catalog/gb/search", 0)
	r.click("Try again")
	r.tt.Until("the results", r.tt.Shows("Top results"))
}

func TestCatalogWithNothingFoundSaysSo(t *testing.T) {
	r := newPageRig(t, "/search?q=nothing", rigOpts{signedIn: true})
	r.tt.Until("the empty state", r.tt.Shows("Nothing on Apple Music for “nothing”",
		"Check the spelling, try fewer words, or search your library instead."))
}

func TestLibraryScopeWithoutAQueryExplainsItself(t *testing.T) {
	r := newPageRig(t, "/search?scope=library", rigOpts{})
	r.tt.Until("the empty state", r.tt.Shows("Search your library",
		"Songs, albums, artists and playlists synced to this computer, matched as you type each word."))
	if r.tt.HasText("Browse categories") {
		t.Error("the library scope shows the catalogue's categories")
	}
}

func TestLibraryResultsListSongsThenAlbums(t *testing.T) {
	r := newPageRig(t, "/search?q=blue&scope=library", rigOpts{})
	r.tt.Until("the results", r.tt.Shows("Songs", "Albums", "Blue Hour", "Blue Monday"))
	if r.position("Songs") >= r.position("Albums") {
		t.Error("the library's songs are not above its albums")
	}
	for _, w := range []string{"Artists", "Playlists"} {
		if r.tt.HasText(w) && w == "Playlists" {
			t.Errorf("an empty %q shelf shows", w)
		}
	}
}

func TestLibraryOpeningAnAlbumRemembersTheLibraryTermAndAlbum(t *testing.T) {
	r := newPageRig(t, "/search?q=blue&scope=library", rigOpts{})
	r.tt.Until("the results", r.tt.Shows("Albums", "Blue Hour"))
	// The rail's Albums button jumps to the shelf and leaves the focus
	// there: the next stop is the shelf's first tile (the songs above it
	// name their album too).
	r.click("Albums, 2")
	r.tt.Drain()
	r.tt.Key(0, ui.KeyTab)
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	if !strings.HasPrefix(r.loc(), "/album/") {
		t.Fatalf("opened %q", r.loc())
	}
	r.untilRecent("term:library:blue", "album:Blue Hour")
}

func TestLibrarySongsPlayRememberOnlyTheTerm(t *testing.T) {
	r := newPageRig(t, "/search?q=blue&scope=library", rigOpts{})
	r.tt.Until("the songs", r.tt.Shows("Library songs matching blue", "Blue Hour song 1"))
	rect, _ := r.tt.Find("Blue Hour song 1")
	r.tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	r.tt.Frame()
	r.click("Play Blue Hour song 1")
	r.tt.Until("the song to play", func() bool { return r.play.plays() >= 1 })
	if r.play.plays() != 1 {
		t.Fatalf("played %d times", r.play.plays())
	}
	r.untilRecent("term:library:blue")
	for _, e := range r.recentKinds() {
		if strings.HasPrefix(e, "song:") {
			t.Errorf("a library song was remembered: %v", r.recentKinds())
		}
	}
}

func TestLibraryNothingFound(t *testing.T) {
	r := newPageRig(t, "/search?q=zzzz&scope=library", rigOpts{})
	r.tt.Until("the empty state", r.tt.Shows("Nothing in your library for “zzzz”",
		"Only what is synced to this computer is searched. Try Apple Music for everything else."))
}

func TestLibrarySongsToggleShowAllAndFewer(t *testing.T) {
	// Five albums of two songs each, all by "Tuner": ten songs match.
	var albums []fixture.Album
	for i := range 5 {
		albums = append(albums, fixture.Album{Name: fmt.Sprintf("Album %d", i), Artist: "Tuner"})
	}
	r := newPageRig(t, "/search?q=tuner&scope=library", rigOpts{}, uitest.Albums(albums...))
	r.tt.Until("the songs", r.tt.Shows("Songs", "Show all 10"))
	count := func() (n int) {
		for _, w := range r.tt.Texts() {
			if strings.HasPrefix(w, "Album ") && strings.Contains(w, " song ") {
				n++
			}
		}
		return n
	}
	// Each song's title shows once as the title and once as the label.
	few := count()
	r.click("Show all 10")
	r.tt.Until("fewer", r.tt.Shows("Show fewer"))
	if count() <= few {
		t.Errorf("Show all did not add songs (%d -> %d)", few, count())
	}
	r.click("Show fewer")
	r.tt.Until("all again", r.tt.Shows("Show all 10"))
	if count() != few {
		t.Errorf("Show fewer left %d song texts, want %d", count(), few)
	}
}

func TestFocusRequestMovesIntoTheFirstResult(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the results", r.tt.Shows("Top results", "Album · Blue Band"))
	r.s.field.resultsReq = true
	r.tt.Drain()
	r.tt.Drain()
	if !r.tt.Focused("Blue Band") {
		t.Errorf("the first result did not take the focus")
	}
}

func TestPageGoldens(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "page_light"
		var opts []uitest.Option
		if dark {
			name, opts = "page_dark", []uitest.Option{uitest.Dark()}
		}
		t.Run(name, func(t *testing.T) {
			r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true, width: 1132, height: 1500}, opts...)
			r.tt.Until("the results", r.tt.Shows("Top results", "Songs", "Playlists", "Blue Song 8"))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), name)
		})
	}
}

// TestPageStateGoldens: the results narrow (the rail a row of chips above
// them), nothing found, and the page while the results load.
func TestPageStateGoldens(t *testing.T) {
	cases := []struct {
		name, path string
		w, h       int
		gate       bool
		shows      []string
	}{
		{"page_narrow_light", "/search?q=blue", 760, 1000, false, []string{"Top results", "Songs", "Blue Song 5"}},
		{"page_empty_light", "/search?q=nothing", 1132, 420, false, []string{"Nothing on Apple Music for “nothing”"}},
		{"page_loading_light", "/search?q=slow", 1132, 640, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate := make(chan struct{})
			t.Cleanup(func() { close(gate) })
			r := newPageRig(t, tc.path, rigOpts{signedIn: true, width: tc.w, height: tc.h, setup: func(a *pageApple) {
				if tc.gate {
					a.gate = gate
				}
			}})
			if tc.gate {
				r.tt.Drain()
				r.tt.Frame()
			} else {
				r.tt.Until("the page", r.tt.Shows(tc.shows...))
				r.tt.Settle()
			}
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}

func TestResultsRailListsTheGroupsAndJumpsToOne(t *testing.T) {
	// A short window: the songs are below the fold.
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true, width: 1132, height: 500})
	r.tt.Until("the rail", r.tt.Shows("Top results", "Songs, 10"))
	for _, label := range []string{"Top results, 3", "Artists, 1", "Albums, 1", "Songs, 10", "Playlists, 1"} {
		if _, ok := r.tt.Find(label); !ok {
			t.Errorf("the rail lacks %q; texts %q", label, r.tt.Texts())
		}
	}
	v := &r.s.pages.search
	if v.Pos.Y != 0 {
		t.Fatalf("the page starts scrolled to %v", v.Pos.Y)
	}
	r.click("Songs, 10")
	r.tt.Drain()
	if v.Pos.Y <= 0 {
		t.Error("jumping to Songs did not scroll")
	}
	if !r.tt.Focused("Songs") {
		t.Errorf("the focus did not move to the Songs group")
	}
	// The rail stays in place while the results scroll.
	if _, ok := r.tt.Find("Playlists, 1"); !ok {
		t.Error("the rail went with the content")
	}
}

func TestResultsNarrowKeepTheGroupsAsARowAboveThem(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true, width: 760, height: 900})
	r.tt.Until("the chips", r.tt.Shows("Top results", "Songs, 10"))
	chip, _ := r.tt.Find("Songs, 10")
	if head := r.position("Top results"); chip.Y >= head {
		t.Errorf("the group chips (y=%v) are not above the first group (y=%v)", chip.Y, head)
	}
}

func TestNothingFoundOffersTheOtherScope(t *testing.T) {
	r := newPageRig(t, "/search?q=nothing", rigOpts{signedIn: true})
	r.tt.Until("the empty state", r.tt.Shows("Nothing on Apple Music for “nothing”"))
	r.click("Search your library")
	if got := r.loc(); got != "/search?q=nothing&scope=library" {
		t.Errorf("went to %q", got)
	}
	r.tt.Until("the library's empty state", r.tt.Shows("Nothing in your library for “nothing”"))
	r.click("Search Apple Music")
	if got := r.loc(); got != "/search?q=nothing" {
		t.Errorf("went to %q", got)
	}
}
