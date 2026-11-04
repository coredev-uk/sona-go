package discover

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

func playlistItem(id, name string) services.Item {
	return services.Item{Kind: services.ItemPlaylist, ID: id, Name: name, Playlist: &services.Playlist{ID: id, Name: name},
		Art: services.Art{Medium: fmt.Sprintf("/art/0123456789abcdef0123456789ab%04s/300x300.jpg", id)}}
}

func albumItem(id, name string) services.Item {
	return services.Item{Kind: services.ItemAlbum, ID: id, Name: name, Album: &services.Album{ID: id, Name: name},
		Art: services.Art{Medium: fmt.Sprintf("/art/0123456789abcdef0123456789ab%04s/300x300.jpg", id)}}
}

func stationItem(id, name string) services.Item {
	return services.Item{Kind: services.ItemStation, ID: id, Name: name, Station: &services.Station{ID: id, Name: name, Playable: true, HasPage: true}}
}

func shelfOf(id, title string, items ...services.Item) services.Section {
	return services.Section{ID: id, Kind: services.SectionShelf, Title: title, Items: items}
}

// issueSections is a front page of cards and one shelf in each of three bands.
func issueSections() []services.Section {
	return []services.Section{
		heroSection(heroCards(16)),
		shelfOf("Recent Releases", "Recent Releases", albumItem("20", "An Album")),
		shelfOf("Updated Playlists", "Updated Playlists", playlistItem("21", "A Playlist")),
		{ID: "Live Radio", Kind: services.SectionLive, Title: "Live Radio", Items: []services.Item{stationItem("ra.1", "A Station")}},
	}
}

// firstAt is where each text first shows in the last frame (-1: not at all).
func firstAt(r *rig, texts ...string) []int {
	all := r.tt.Texts()
	out := make([]int, len(texts))
	for i, s := range texts {
		out[i] = slices.Index(all, s)
	}
	return out
}

func TestNewPageShowsTheHeaderTheFrontPageTheContentsAndTheShelvesInBands(t *testing.T) {
	r := newRig(t, "/new", withApple(&fakeApple{t: t}), wideWindow(), withSize(1132, 1253))
	r.tt.Until("the page", r.tt.Shows("On this page", "Best New Songs", "New This Week"))
	r.d.front.paused = true
	r.tt.Drain()
	for _, w := range []string{"Discover", "Updated just now", "Featured", "Featured cards", "A-List Pop", "Songs and albums", "Radio", "Browse", "Listen to Live Radio Now"} {
		if !r.tt.HasText(w) {
			t.Errorf("the page lacks %q", w)
		}
	}
	// Bands are in order, each with its heading after the contents.
	at := firstAt(r, "On this page", "Best New Songs", "New This Week", "Listen to Live Radio Now")
	if !slices.IsSorted(at) || at[0] < 0 {
		t.Errorf("order of the page's parts %v", at)
	}
}

func TestNewPageContentsListsTheBandsInOrderWithTheirShelves(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api), withSize(1132, 1253))
	newPageWith(api, r.h.Clock.Now(), issueSections()...)
	r.tt.Until("the page", r.tt.Shows("On this page"))
	r.d.front.paused = true
	r.tt.Drain()
	// The contents' buttons, in order: a band, then its shelves.
	at := firstAt(r, "On this page", "Songs and albums", "Recent Releases", "Playlists", "Updated Playlists", "Radio", "Live Radio")
	if slices.Contains(at, -1) || !slices.IsSorted(at) {
		t.Errorf("contents order %v: %q", at, r.tt.Texts())
	}
	// Each band's heading follows the contents, with its shelf under it.
	texts := r.tt.Texts()
	rest := texts[slices.Index(texts, "Live Radio")+1:]
	h, shelf := slices.Index(rest, "Radio"), lastIndex(rest, "Live Radio")
	if h < 0 || shelf < h {
		t.Errorf("the Radio band's heading is not followed by its shelf: %q", rest)
	}
}

func TestNewPageHasNoContentsForASingleBand(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api))
	newPageWith(api, r.h.Clock.Now(), heroSection(heroCards(3)), shelfOf("Recent Releases", "Recent Releases", albumItem("20", "An Album")))
	r.tt.Until("the page", r.tt.Shows("Recent Releases"))
	if r.tt.HasText("On this page") {
		t.Errorf("a page of one band has a contents: %q", r.tt.Texts())
	}
}

func TestNewPageContentsJumpToAShelfAndMoveFocusToIt(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api), withSize(1132, 700))
	newPageWith(api, r.h.Clock.Now(), issueSections()...)
	r.tt.Until("the page", r.tt.Shows("On this page"))
	r.d.front.paused = true
	for range 5 {
		r.tt.Drain()
	}
	if len(r.d.newPg.anchors) == 0 {
		t.Fatal("the page has no places to jump to")
	}
	if r.d.newPg.pg.pos.Y != 0 {
		t.Fatalf("the page starts scrolled to %v", r.d.newPg.pg.pos.Y)
	}
	if err := r.tt.Click("Updated Playlists"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	r.tt.Drain()
	// The shelf is scrolled to the top (its scroll margin of 24 below it), and the
	// focus is on its section, which is not a stop of Tab.
	pos := r.d.newPg.pg.pos
	want := min(r.d.newPg.anchors["new:playlists-0"]-24, pos.MaxY)
	if got := pos.Y; got <= 0 || got < want-1 || got > want+1 {
		t.Errorf("scrolled to %v, want the shelf's place %v (the page's end is %v)", got, want, pos.MaxY)
	}
	if got := r.d.newPg.focus; got != "new:playlists-0" {
		t.Errorf("focus anchor %q", got)
	}
	if !r.tt.Focused("Updated Playlists") {
		t.Errorf("the focus is not on the shelf's section")
	}
	// A band's own button goes to its heading.
	if err := r.tt.Click("Songs and albums"); err == nil {
		r.tt.Drain()
	}
}

func TestNewPageShelvesLeaveOutWhatTheFrontPageShowsUnlessThatWouldEmptyTheShelf(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api), withSize(1132, 1253))
	hero := make([]services.Item, 4)
	for i := range hero {
		hero[i] = playlistItem(fmt.Sprint(i+1), fmt.Sprintf("Hero %d", i+1))
	}
	dup := func(id, name string) services.Item { it := playlistItem(id, name); return it }
	newPageWith(api, r.h.Clock.Now(), heroSection(hero),
		shelfOf("New This Week", "New This Week", dup("3", "Shelf three"), albumItem("30", "Shelf thirty")),
		shelfOf("Spotlight", "Spotlight", dup("4", "Shelf four")))
	r.tt.Until("the page", r.tt.Shows("New This Week", "Spotlight"))
	r.d.front.paused = true
	r.tt.Drain()
	if r.tt.HasText("Shelf three") {
		t.Error("the shelf repeats a card the front page shows")
	}
	if !r.tt.HasText("Shelf thirty") {
		t.Error("the shelf lost an item the front page does not show")
	}
	if !r.tt.HasText("Shelf four") {
		t.Error("a shelf the front page would empty was emptied")
	}
}

func TestNewPageShowsTheFreshnessLineAndTheStaleOne(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api))
	now := r.h.Clock.Now()
	newPageWith(api, now, shelfOf("S", "S", albumItem("1", "A")))
	p := api.pages["new"]
	p.FetchedAt = now.Add(-12 * time.Minute).UnixMilli()
	api.pages["new"] = p
	r.tt.Until("the freshness", r.tt.Shows("Updated 12 min. ago"))

	api2 := &fakeAPI{}
	r2 := newRig(t, "/new", withAPI(api2))
	newPageWith(api2, r2.h.Clock.Now(), shelfOf("S", "S", albumItem("1", "A")))
	p = api2.pages["new"]
	p.FetchedAt, p.Stale = r2.h.Clock.Now().Add(-12*time.Minute).UnixMilli(), true
	api2.pages["new"] = p
	r2.tt.Until("the stale line", r2.tt.Shows("Couldn't reach Apple Music · showing the copy from 12 min. ago"))

	api3 := &fakeAPI{}
	r3 := newRig(t, "/new", withAPI(api3))
	api3.pages = map[string]services.EditorialPage{"new": {Title: "New", Sections: []services.Section{shelfOf("S", "S", albumItem("1", "A"))}}}
	r3.tt.Until("the page", r3.tt.Shows("S"))
	if r3.tt.HasText("Updated") || r3.tt.HasText("Couldn't reach") {
		t.Errorf("a page never fetched has a freshness line: %q", r3.tt.Texts())
	}
}

func TestNewPageLoadingShowsTheSkeletonAndAPlaceForTheFreshnessLine(t *testing.T) {
	api := &fakeAPI{block: make(chan struct{})}
	r := newRig(t, "/new", withAPI(api), wideWindow())
	newPageWith(api, r.h.Clock.Now(), heroSection(heroCards(3)), shelfOf("S", "S", albumItem("1", "A")))
	r.tt.Until("the skeleton", r.tt.Shows("Discover", "Loading"))
	if r.tt.HasText("Featured") || r.tt.HasText("On this page") || r.tt.HasText("Updated") {
		t.Errorf("the loading page shows data: %q", r.tt.Texts())
	}
	close(api.block)
	r.tt.Until("the page", r.tt.Shows("Featured", "Updated just now"))
	if r.tt.HasText("Loading") {
		t.Errorf("the skeleton stays after the data arrives: %q", r.tt.Texts())
	}
}

func TestNewPageErrorShowsTheReasonAndRetries(t *testing.T) {
	api := &fakeAPI{err: applemusic.ErrNotFound}
	r := newRig(t, "/new", withAPI(api))
	newPageWith(api, r.h.Clock.Now(), shelfOf("S", "Recovered shelf", albumItem("1", "A")))
	r.tt.Until("the error", r.tt.Shows("Couldn't load what's new", i18n.ErrorText(services.CodeNotFound), "Try again"))
	api.mu.Lock()
	api.err = nil
	api.mu.Unlock()
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the page after retrying", r.tt.Shows("Recovered shelf"))
	if r.tt.HasText("Couldn't load what's new") {
		t.Error("the error stays after a retry that worked")
	}
}

func TestNewPageErrorShowsRetryingWhileItFetches(t *testing.T) {
	api := &fakeAPI{err: applemusic.ErrNotFound}
	r := newRig(t, "/new", withAPI(api))
	r.tt.Until("the error", r.tt.Shows("Couldn't load what's new"))
	api.mu.Lock()
	api.err, api.block = nil, make(chan struct{})
	api.mu.Unlock()
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the retrying state", r.tt.Shows("Retrying…"))
	close(api.block)
}

func TestNewPageWithNoSectionsSaysSo(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api))
	newPageWith(api, r.h.Clock.Now())
	r.tt.Until("the empty page", r.tt.Shows("Nothing here right now", "Apple Music has no music on this page at the moment."))
}

func TestNewPageSeeAllOpensTheSectionsRoom(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api), withSize(1132, 1200))
	s := shelfOf("Recent Releases", "Recent Releases", albumItem("20", "An Album"))
	s.Room = "6828645964"
	newPageWith(api, r.h.Clock.Now(), s)
	r.tt.Until("the page", r.tt.Shows("Recent Releases", "See all"))
	if err := r.tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.path(); got != "/room/6828645964" {
		t.Errorf("See all went to %q", got)
	}
}

func TestDiscoverReadsItsPagesFromTheKeysOfV020AndKeepsThemFreshForTenMinutes(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api))
	// A copy under v0.2.0's key is the page: it is not fetched again while it is fresh.
	store.Set(r.env.Queries, store.Key{"discover", "page", "new", ""}, services.EditorialPage{
		Title: "New", Sections: []services.Section{shelfOf("Cached", "From the cache", albumItem("1", "A"))},
		FetchedAt: r.h.Clock.Now().UnixMilli(),
	})
	r.tt.Until("the cached page", r.tt.Shows("From the cache"))
	for range 3 {
		r.tt.Drain()
	}
	if n := api.count("new"); n != 0 {
		t.Errorf("a fresh page was fetched %d times", n)
	}
	if editorialStale != 10*time.Minute {
		t.Errorf("editorial pages are fresh for %v, want 10 minutes", editorialStale)
	}
	for got, want := range map[string]string{
		keyRoom("6828645964", "alphabet").String():   "[discover room 6828645964 alphabet]",
		keyCharts("14").String():                     "[discover charts 14]",
		keyChart("cityCharts", "135", "14").String(): "[discover chart cityCharts 135 14]",
		keyPage("curator", "990050553").String():     "[discover page curator 990050553]",
	} {
		if got != want {
			t.Errorf("key %s, want %s", got, want)
		}
	}
}

func TestASignInChangeInvalidatesEveryDiscoverQuery(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api))
	newPageWith(api, r.h.Clock.Now(), shelfOf("S", "S", albumItem("1", "A")))
	r.tt.Until("the page", r.tt.Shows("S"))
	r.d.init() // follows the bus from here
	before := api.count("new")
	r.h.Bus.Emit(services.EventAuthChanged, services.AuthStatus{})
	r.tt.Until("the page fetched again", func() bool { return api.count("new") > before })
}

func TestDiscoverModuleHasTheRoutesAndLinksOfV020(t *testing.T) {
	r := newRig(t, "/new", withAPI(&fakeAPI{}))
	var patterns []string
	for _, rt := range r.mod.Routes {
		patterns = append(patterns, rt.Pattern)
	}
	want := []string{"/new", "/room/{id}", "/curator/{id}", "/multiroom/{id}", "/grouping/{id}", "/charts", "/charts/{chart}"}
	if !slices.Equal(patterns, want) {
		t.Errorf("routes %v, want %v", patterns, want)
	}
	type nav struct {
		label, icon, to string
		order           int
	}
	var got []nav
	for _, n := range r.mod.Nav {
		if n.Section != module.NavPrimary {
			t.Errorf("nav item %q is not in the primary section", n.Label())
		}
		got = append(got, nav{n.Label(), n.Icon, n.To, n.Order})
	}
	if want := []nav{{"Discover", "compass", "/new", 15}, {"Charts", "chart-no-axes-column", "/charts", 17}}; !slices.Equal(got, want) {
		t.Errorf("nav %+v, want %+v", got, want)
	}
	if r.mod.ID != "discover" {
		t.Errorf("module id %q", r.mod.ID)
	}
}

func lastIndex(s []string, v string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == v {
			return i
		}
	}
	return -1
}

// A shelf tile whose page is not ported builds without an opener: a click
// leads nowhere (kit.ItemsCtx.HasRoute).
func TestNewPageTileOfAnUnportedPageLeadsNowhere(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api), withoutRoutes("/album/{id}"))
	newPageWith(api, r.h.Clock.Now(), shelfOf("Recent Releases", "Recent Releases", albumItem("20", "An Album")))
	r.tt.Until("the page", r.tt.Shows("An Album"))
	_ = r.tt.Click("An Album")
	r.tt.Drain()
	if got := r.path(); got != "/new" {
		t.Errorf("a tile without a page navigated to %q", got)
	}
	r2 := newRig(t, "/new", withAPI(&fakeAPI{pages: map[string]services.EditorialPage{"new": editorial("", "New", shelfOf("Recent Releases", "Recent Releases", albumItem("20", "An Album")))}}))
	r2.tt.Until("the page", r2.tt.Shows("An Album"))
	if err := r2.tt.Click("An Album"); err != nil {
		t.Fatal(err)
	}
	r2.tt.Drain()
	if got := r2.path(); got != "/album/20" {
		t.Errorf("a tile with a page went to %q", got)
	}
}
