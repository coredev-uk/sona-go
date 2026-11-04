package home

import (
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// errDown is a request that never reached Apple.
var errDown = &url.Error{Op: "Get", URL: "https://amp-api.music.apple.com/v1/me/recommendations", Err: errors.New("connection refused")}

func (r *rig) homeWith(page services.HomePage) {
	r.disc.set(func(f *fakeDiscover) { f.page = page })
}

func requireTexts(t *testing.T, r *rig, want ...string) {
	t.Helper()
	for _, w := range want {
		if !slices.Contains(r.tt.Texts(), w) {
			t.Errorf("the page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
}

// requireNoTexts fails when one of the texts is on screen on its own (a label
// that contains it is not it).
func requireNoTexts(t *testing.T, r *rig, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if slices.Contains(r.tt.Texts(), w) {
			t.Errorf("the page shows %q", w)
		}
	}
}

func TestHomeShowsTheLeadTheShelvesAndTheIndex(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Top Picks for You", "Because You Like Rock")

	// The header: the title and when Apple picked, with Refresh.
	requireTexts(t, r, "For you", "Picked for you 5 min. ago", "Refresh")
	// The lead: its first pick large with what it is, the rest as rows.
	requireTexts(t, r, "Lead Album", "Lead Artist", "Album · New Release", "Pick 2", "Pick 9", "Show all 12")
	requireNoTexts(t, r, "Pick 10", "Pick 12")
	// Shelves by kind: rows for Recently Played and stations, tiles for the rest.
	requireTexts(t, r, "Recently Played", "Recent One", "Recent Station", "Find Your Mood", "Energy", "Relax",
		"Made for You", "Mix 1", "Mix 4", "Indie Picks", "Indie 1", "Jazz Picks", "Jazz 4")
	// The hero shelf with one card is the hero card.
	requireTexts(t, r, "Featured Station", "Apple Music Hits", "Station · Radio")
	// A caption every item shares is left off.
	requireNoTexts(t, r, "Apple Music for You")
	// The shelves past the third plain one are listed, not shown.
	requireTexts(t, r, "More for you", "Fans Also Like", "3 albums · 1 playlist", "Because You Like Rock", "2 albums")
	requireNoTexts(t, r, "Fan 1", "Rock 1")
	// Loaded Home has no local shelves, no sign-in notice.
	requireNoTexts(t, r, "Sign in", "Recommendations need Apple Music", "Recently added", "Your playlists")
	if n, _, _ := r.disc.calls(); n != 1 {
		t.Errorf("Home asked %d times, want once", n)
	}
}

func TestHomeIsSignedOutWithoutRecommendations(t *testing.T) {
	r := newRig(t, "/", 1132, 1253,
		withAuth(services.AuthStatus{State: "signed_out"}),
		withLib(func(l *fakeLib) {
			l.albums = []services.Album{{ID: "a1", Name: "Shelf Album", Artist: "Shelf Artist"}}
			l.playlists = []services.Playlist{{ID: "p1", Name: "Shelf Mix", TrackCount: 12}, {ID: "p2", Name: "Curated Mix", Curator: "A. Curator"}}
		}))
	r.until("the notice", "Recommendations need Apple Music", "Shelf Album", "Curated Mix")
	requireTexts(t, r, "For you", "Sign in to see picks, mixes and stations chosen for you.", "Sign in",
		"Shelf Album", "Shelf Artist", "Shelf Mix", "12 songs", "Curated Mix", "A. Curator")
	// No Refresh, no Recently played (it is Apple's), and Home was not asked.
	requireNoTexts(t, r, "Refresh", "Recently played", "Played on this computer")
	if n, _, _ := r.disc.calls(); n != 0 {
		t.Errorf("signed out, Home asked %d times", n)
	}
	// Sign in opens the account dialog.
	if r.home.env.Surfaces.AccountOpen() {
		t.Fatal("the account dialog was open")
	}
	r.click("Sign in")
	if !r.home.env.Surfaces.AccountOpen() {
		t.Error("Sign in did not open the account dialog")
	}
	// The shelves link to the library's pages.
	r.click("See all")
	if got := r.path(); got != "/library/albums" {
		t.Errorf("the first See all went to %q, want /library/albums", got)
	}
}

func TestHomeWhileRestoringIsALoadingLeadNotTheSignedOutState(t *testing.T) {
	r := newRig(t, "/", 1132, 1000, withAuth(services.AuthStatus{State: "restoring"}))
	r.until("the header", "For you")
	for range 3 {
		r.tt.Drain()
	}
	requireNoTexts(t, r, "Recommendations need Apple Music", "Refresh", "Recently added")
	if _, ok := r.tt.Find("Loading"); !ok {
		t.Errorf("no loading lead; texts %q", r.tt.Texts())
	}
	// Once the sign-in is read, Home loads.
	r.disc.set(func(f *fakeDiscover) { f.page = samplePage() })
	r.auth.set(signedIn)
	r.emit(services.EventAuthChanged, signedIn)
	r.until("the page", "Top Picks for You")
	requireNoTexts(t, r, "Loading")
}

func TestHomeShowsOnlyItsHeaderUntilTheSignInStateIsKnown(t *testing.T) {
	r := newRig(t, "/", 1132, 1000, withGatedAuth())
	t.Cleanup(func() { close(r.auth.gate) })
	for range 4 {
		r.tt.Drain()
	}
	requireTexts(t, r, "For you")
	requireNoTexts(t, r, "Recommendations need Apple Music", "Sign in", "Refresh", "Loading")
}

func TestHomeShowsALoadingLeadWhileTheAccountsHomeIsRead(t *testing.T) {
	gate := make(chan struct{})
	r := newRig(t, "/", 1132, 1000, withHome(samplePage()), withDisc(func(f *fakeDiscover) { f.homeGate = gate }))
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})
	r.until("the header", "For you")
	for range 3 {
		r.tt.Drain()
	}
	if _, ok := r.tt.Find("Loading"); !ok {
		t.Errorf("no loading lead; texts %q", r.tt.Texts())
	}
	// The header is there, with Refresh; not the signed-out state.
	requireTexts(t, r, "Refresh")
	requireNoTexts(t, r, "Recommendations need Apple Music")
	close(gate)
	r.until("the page", "Top Picks for You")
}

func TestHomeOfflineWithoutACopyHasNoRetry(t *testing.T) {
	r := newRig(t, "/", 1132, 1253,
		withAuth(services.AuthStatus{State: "signed_in", Offline: true}),
		withDisc(func(f *fakeDiscover) { f.err = errDown }),
		withLib(func(l *fakeLib) { l.albums = []services.Album{{ID: "a1", Name: "Shelf Album", Artist: "Shelf Artist"}} }))
	r.until("the offline notice", "No recommendations while offline", "Sona fetches them as soon as it reconnects.", "Shelf Album")
	requireNoTexts(t, r, "Refresh", "Try again", "Couldn't load your recommendations")
	// Signed in, so Apple's recently played list is read too.
	requireTexts(t, r, "Recently played")
}

func TestHomeErrorOnlineRetries(t *testing.T) {
	r := newRig(t, "/", 1132, 1253,
		withDisc(func(f *fakeDiscover) { f.err = errDown }),
		withLib(func(l *fakeLib) { l.albums = []services.Album{{ID: "a1", Name: "Shelf Album", Artist: "Shelf Artist"}} }))
	r.until("the error", "Couldn't load your recommendations", "Try again", "Shelf Album")
	requireTexts(t, r, "Refresh")
	// Retry asks again and shows the page it gets.
	r.disc.set(func(f *fakeDiscover) { f.err, f.page = nil, samplePage() })
	r.click("Try again")
	r.until("the page", "Top Picks for You")
	requireNoTexts(t, r, "Couldn't load your recommendations")
	if n, _, _ := r.disc.calls(); n < 2 {
		t.Errorf("Home asked %d times, want a retry", n)
	}
}

func TestHomeWithNothingIsHeaderAndLocalShelves(t *testing.T) {
	r := newRig(t, "/", 1132, 1253,
		withHome(services.HomePage{Sections: []services.Section{}, FetchedAt: uitest.Epoch.UnixMilli()}),
		withLib(func(l *fakeLib) {
			l.albums = []services.Album{{ID: "a1", Name: "Shelf Album", Artist: "Shelf Artist"}}
			l.recent = []services.Song{{ID: "s1", Name: "Heard Song", Artist: "Heard Artist", Album: "Heard Album", Playable: true}}
		}))
	r.until("the local shelves", "Shelf Album", "Heard Song")
	requireTexts(t, r, "For you", "Refresh", "Recently played", "Recently added")
	requireNoTexts(t, r, "Recommendations need Apple Music", "More for you")
}

func TestHomeSublineSaysWhenTheCopyIsStale(t *testing.T) {
	page := samplePage()
	page.Stale = true
	r := newRig(t, "/", 1132, 1253, withHome(page))
	r.until("the stale line", "Couldn't refresh · from 5 min. ago")
	requireNoTexts(t, r, "Picked for you 5 min. ago")
}

func TestRefreshReplacesTheCachedPageAndSpinsWhileItRuns(t *testing.T) {
	fresh := samplePage()
	fresh.Sections[0].Title = "Fresh Picks"
	fresh.FetchedAt = uitest.Epoch.UnixMilli()
	gate := make(chan struct{})
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()), withDisc(func(f *fakeDiscover) { f.refresh, f.refreshGate = fresh, gate }))
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})
	r.until("the page", "Top Picks for You", "Refresh")

	r.click("Refresh")
	r.tt.Until("refreshing", r.tt.Shows("Refreshing…"))
	requireNoTexts(t, r, "Refresh")
	// A second click while it runs asks once only.
	_ = r.tt.Click("Refreshing…")
	r.tt.Drain()
	// The icon spins: the window keeps asking for frames.
	r.tt.Advance(300 * time.Millisecond)
	if !r.tt.FramePending() {
		t.Error("the spinner stopped asking for frames")
	}
	close(gate)
	r.tt.Until("the fresh page", r.tt.Shows("Fresh Picks", "Picked for you just now", "Refresh"))
	requireNoTexts(t, r, "Top Picks for You", "Refreshing…")
	if _, n, _ := r.disc.calls(); n != 1 {
		t.Errorf("RefreshHome ran %d times, want 1", n)
	}
}

func TestRefreshFailureShowsAnErrorBlockWithRetry(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()), withDisc(func(f *fakeDiscover) { f.refreshErr = errDown }))
	r.until("the page", "Top Picks for You", "Refresh")
	r.click("Refresh")
	r.tt.Until("the error", r.tt.Shows("Couldn't refresh your recommendations", "Try again"))
	// The page under it stays.
	requireTexts(t, r, "Top Picks for You")
	// Retry runs the refresh again; success clears the error.
	fresh := samplePage()
	fresh.Sections[0].Title = "Fresh Picks"
	r.disc.set(func(f *fakeDiscover) { f.refreshErr, f.refresh = nil, fresh })
	r.click("Try again")
	r.tt.Until("recovered", r.tt.Shows("Fresh Picks"))
	requireNoTexts(t, r, "Couldn't refresh your recommendations")
	if _, n, _ := r.disc.calls(); n != 2 {
		t.Errorf("RefreshHome ran %d times, want 2", n)
	}
}

func TestRefreshIsHiddenOfflineAndSignedOut(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Top Picks for You", "Refresh")
	// Going offline hides it (Go renews Home when it reconnects).
	st := signedIn
	st.Offline = true
	r.auth.set(st)
	r.emit(services.EventAuthChanged, st)
	r.tt.Until("no refresh", r.gone("Refresh"))
	requireTexts(t, r, "Top Picks for You")
}

func TestARenewedHomeEventReplacesThePageWithoutAFetch(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Top Picks for You")
	before, _, _ := r.disc.calls()
	fresh := samplePage()
	fresh.Sections[0].Title = "Renewed Picks"
	fresh.FetchedAt = uitest.Epoch.UnixMilli()
	r.emit(services.EventHome, fresh)
	r.tt.Until("the renewed page", r.tt.Shows("Renewed Picks", "Picked for you just now"))
	if after, _, _ := r.disc.calls(); after != before {
		t.Errorf("the event made %d fetches", after-before)
	}
}

func TestAChangeOfAccountDropsTheDiscoverQueries(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Top Picks for You")
	before, _, _ := r.disc.calls()
	fresh := samplePage()
	fresh.Sections[0].Title = "Other Account Picks"
	r.homeWith(fresh)
	r.emit(services.EventAuthChanged, signedIn)
	r.tt.Until("the other account's page", r.tt.Shows("Other Account Picks"))
	if after, _, _ := r.disc.calls(); after <= before {
		t.Errorf("auth:changed did not make Home reload (%d, %d)", before, after)
	}
}

func TestSigningOutShowsTheSignedOutHome(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Top Picks for You")
	out := services.AuthStatus{State: "signed_out"}
	r.auth.set(out)
	r.emit(services.EventAuthChanged, out)
	r.tt.Until("signed out", r.tt.Shows("Recommendations need Apple Music"))
	requireNoTexts(t, r, "Top Picks for You", "Refresh")
}

func TestLeadPicksOpenAndPlay(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Top Picks for You", "Lead Album")
	// The play control is named with the pick.
	r.click("Play Lead Album")
	if got := r.rec.all(); !slices.Equal(got, []string{"album:Pick1"}) {
		t.Errorf("played %q, want the lead album", got)
	}
	// The name opens the pick's page.
	r.click("Lead Album")
	if got := r.path(); got != "/album/Pick1" {
		t.Errorf("the lead opened %q", got)
	}
}

func TestLeadShowsAllPicksOnDemand(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Show all 12")
	requireNoTexts(t, r, "Pick 12", "Show fewer")
	r.click("Show all 12")
	r.tt.Until("every pick", r.tt.Shows("Pick 12", "Show fewer"))
	r.click("Show fewer")
	r.tt.Until("fewer", r.gone("Pick 12", "Show fewer"))
	requireTexts(t, r, "Show all 12")
}

func TestLeadWithFewPicksHasNoToggle(t *testing.T) {
	page := samplePage()
	page.Sections[0].Items = page.Sections[0].Items[:5]
	r := newRig(t, "/", 1132, 1253, withHome(page))
	r.until("the page", "Pick 5")
	requireNoTexts(t, r, "Show all 5", "Show fewer")
}

func TestLeadStacksUnderNineHundredDIPsAndTitlesItselfTopPicksWithoutATitle(t *testing.T) {
	page := samplePage()
	page.Sections[0].Title = ""
	r := newRig(t, "/", 760, 1253, withHome(page))
	r.until("the page", "Top picks", "Lead Album", "Pick 2")
	// Stacked, the picks sit under the lead pick.
	lead, _ := r.tt.Find("Lead Album")
	pick, _ := r.tt.Find("Pick 2")
	if pick.Y <= lead.Y+lead.H {
		t.Errorf("the picks (y %.0f) are not under the lead (y %.0f)", pick.Y, lead.Y)
	}
}

func TestLeadSitsBesideThePicksFromNineHundredDIPs(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Lead Album", "Pick 2")
	lead, _ := r.tt.Find("Lead Album")
	pick, _ := r.tt.Find("Pick 2")
	if pick.X <= lead.X+lead.W {
		t.Errorf("the picks (x %.0f) are not beside the lead (x %.0f)", pick.X, lead.X)
	}
}

func TestHeroCardPlaysAndOpens(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Featured Station")
	r.click("Play Featured Station")
	if got := r.rec.all(); !slices.Equal(got, []string{"station:hero1"}) {
		t.Errorf("played %q", got)
	}
	r.click("Featured Station")
	if got := r.path(); got != "/station/hero1" {
		t.Errorf("the hero opened %q", got)
	}
}

func TestAHeroShelfWithSeveralItemsIsAnEditorialSection(t *testing.T) {
	page := samplePage()
	page.Sections[2].Items = append(page.Sections[2].Items, stationItem("hero2", "Second Feature", 31, true))
	r := newRig(t, "/", 1132, 1253, withHome(page))
	r.until("the page", "Featured Station", "Second Feature")
	// The wide cards keep their play controls.
	r.click("Play Second Feature")
	if got := r.rec.all(); !slices.Equal(got, []string{"station:hero2"}) {
		t.Errorf("played %q", got)
	}
}

func TestShelvesWithAPageLinkToIt(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Made for You", "See all")
	r.click("See all")
	if got := r.path(); got != "/for-you/made" {
		t.Errorf("See all went to %q, want /for-you/made", got)
	}
}

func TestRecentlyPlayedRowsPlayAndOpen(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	r.until("the page", "Recent One", "Recent Station")
	// A station with no page plays when it is opened.
	r.click("Recent Station")
	if got := r.rec.all(); !slices.Equal(got, []string{"station:r2"}) {
		t.Errorf("played %q, want the station", got)
	}
	if got := r.path(); got != "/" {
		t.Errorf("a page-less station navigated to %q", got)
	}
	// An album opens its page.
	r.click("Recent One")
	if got := r.path(); got != "/album/r1" {
		t.Errorf("the album opened %q", got)
	}
}

func TestIndexLinesOpenTheirShelfInPlace(t *testing.T) {
	r := newRig(t, "/", 1132, 1600, withHome(indexPage()))
	r.until("the page", "Fans Also Like", "3 albums · 1 playlist")
	requireNoTexts(t, r, "Fan 1")
	r.click("Fans Also Like")
	r.tt.Until("the shelf", r.tt.Shows("Fan 1", "Fan 3", "Fan Mix", "See all"))
	// The other line stays closed.
	requireNoTexts(t, r, "Rock 1")
	r.click("See all")
	if got := r.path(); got != "/for-you/fans" {
		t.Errorf("the index's See all went to %q", got)
	}
}

func TestIndexLineClosesAgain(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(indexPage()))
	r.until("the page", "Because You Like Rock")
	r.click("Because You Like Rock")
	r.tt.Until("open", r.tt.Shows("Rock 1", "Rock 2"))
	r.click("Because You Like Rock")
	r.tt.Until("closed", r.gone("Rock 1", "Rock 2"))
}

func TestHomeBuildsTheConcertsShelfTheConcertsModuleHandsOver(t *testing.T) {
	page := samplePage()
	page.Sections = append(page.Sections, section("concerts", services.SectionConcerts, "Concerts"))
	var firsts []bool
	r := newRig(t, "/", 1132, 1253, withHome(page), withConcerts(func(c *ui.Context, first bool) {
		firsts = append(firsts, first)
		ui.Text(c, "Shows near you")
	}))
	r.until("the concerts shelf", "Shows near you")
	for _, first := range firsts {
		if first {
			t.Error("Home's concerts shelf is not the first of the page")
		}
	}

	// Without the module nothing stands in for it.
	r2 := newRig(t, "/", 1132, 1253, withHome(page))
	r2.until("the page", "Top Picks for You")
	requireNoTexts(t, r2, "Shows near you")
}

func TestItemsWhosePageIsNotServedArePlainText(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()),
		withRoutes(func(p string) bool { return strings.HasPrefix(p, "/for-you/") }))
	r.until("the page", "Lead Album", "Pick 2", "Recent One", "Featured Station")
	// No page to open: a click on a name, a row or the hero goes nowhere.
	for _, name := range []string{"Lead Album", "Pick 2", "Recent One", "Featured Station"} {
		_ = r.tt.Click(name)
		r.tt.Drain()
		if got := r.path(); got != "/" {
			t.Fatalf("a click on %q went to %q", name, got)
		}
	}
	// Their play controls still work.
	r.click("Play Lead Album")
	if got := r.rec.all(); !slices.Equal(got, []string{"album:Pick1"}) {
		t.Errorf("played %q", got)
	}
	// A shelf of Home's own (See all) still opens.
	r.click("See all")
	if got := r.path(); got != "/for-you/made" {
		t.Errorf("See all went to %q", got)
	}
}

// indexPage is Home with three plain shelves and two for the index, and no
// lead: the index is near the top of the window. Only the index's "Fans Also
// Like" has a page.
func indexPage() services.HomePage {
	full := samplePage()
	byID := map[string]services.Section{}
	for _, s := range full.Sections {
		byID[s.ID] = s
	}
	made := byID["made"]
	made.SeeAll = false
	return services.HomePage{
		Sections:  []services.Section{made, byID["g1"], byID["g2"], byID["fans"], byID["because"]},
		FetchedAt: full.FetchedAt,
	}
}
