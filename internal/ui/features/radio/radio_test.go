package radio

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The Radio page's tests: v0.2.0's RadioPage.test.tsx and OnAir.test.tsx, and
// what its page and queries do besides.

var (
	appleMusic1 = liveStation("ra.1", "Apple Music 1")
	appleHits   = liveStation("ra.2", "Hits")
)

func episodeStation(id, name string) services.Station {
	return services.Station{ID: id, Name: name, Kind: services.StationEpisode, HasPage: true, Playable: true, ShowName: "Zane Lowe"}
}

func trackStation(id, name string, extra ...func(*services.Station)) services.Station {
	s := services.Station{ID: id, Name: name, Kind: services.StationTracks, HasPage: true, Playable: true, Provider: "Apple Music"}
	for _, f := range extra {
		f(&s)
	}
	return s
}

func broadcaster(id, name string) services.Station {
	return services.Station{ID: id, Name: name, Kind: services.StationBroadcast, Playable: true, Provider: "TuneIn"}
}

// signedIn is the setup of a signed-in account.
func signedIn(rg *rig) { rg.auth.state = "signed_in" }

// fake is the setup that changes the fake RadioService.
func fake(fn func(*fakeRadio)) func(*rig) { return func(rg *rig) { rg.api.set(fn) } }

// radioFixture is Apple's radio page as the RadioPage tests of v0.2.0 had it:
// a featured row and a shelf repeating the live stations, an episodes shelf,
// the local broadcasters and a shelf of genre stations.
func radioFixture(rg *rig) {
	live := []services.Station{appleMusic1, appleHits}
	rg.api.set(func(f *fakeRadio) {
		f.live = live
		f.grouping = services.EditorialPage{Sections: []services.Section{
			section("", itemsOf(live...), services.SectionHero),
			section("On Air Now", itemsOf(live...), services.SectionLive),
			section("Latest Episodes", itemsOf(episodeStation("ra.ep", "Episode One"))),
			section("Local Broadcasters", itemsOf(broadcaster("ra.kiss", "KISS")), services.SectionStations),
			section("Stations by Genre", itemsOf(trackStation("ra.pop", "Pop"))),
		}}
	})
}

// radioHeadings are the headings of the page, in page order.
var radioHeadings = []string{
	"Radio", "Live now", "Local Broadcasters", "Stations", "Your stations", "Stations by Genre",
	"Shows and episodes", "Latest Episodes", "Browse",
}

func TestRadioPageLeadsWithTheLiveBoardAndTheLocalBroadcastersThenTheBands(t *testing.T) {
	rg := newRig(t, radioPath, 1132, 1253, radioFixture)
	rg.tt.Until("the page", rg.tt.Shows("Local Broadcasters", "Latest Episodes", "Apple Music 1"))
	want := []string{"Radio", "Live now", "Local Broadcasters", "Stations", "Your stations", "Stations by Genre", "Shows and episodes", "Latest Episodes"}
	if got := rg.order(radioHeadings...); !slices.Equal(got, want) {
		t.Errorf("headings %q, want %q", got, want)
	}
	// Apple's own rows of the live stations are left out: the board lists them.
	if rg.tt.HasText("On Air Now") {
		t.Errorf("Apple's live shelf shows beside the board: %q", rg.tt.Texts())
	}
	// Nothing was placed in Browse: the band is left out.
	if rg.tt.HasText("Browse") {
		t.Error("an empty Browse band shows")
	}
	// Signed out: "Your stations" says how to see them.
	if !rg.tt.HasText("Sign in to see the stations you play in Apple Music on any of your devices.") {
		t.Errorf("no sign-in text: %q", rg.tt.Texts())
	}
	rg.click("Sign in")
	if !rg.h.Env.Surfaces.AccountOpen() {
		t.Error("Sign in did not open the account dialog")
	}
	if n := rg.api.count("recent"); n != 0 {
		t.Errorf("the user's stations were asked for while signed out (%d)", n)
	}
}

func TestRadioPagePutsTheUsersStationsFirstInTheStationsBandWhenSignedIn(t *testing.T) {
	rg := newRig(t, radioPath, 1132, 1253, radioFixture, signedIn,
		fake(func(f *fakeRadio) { f.recent = []services.Station{trackStation("ra.mine", "My Station")} }))
	rg.tt.Until("the shelf", rg.tt.Shows("My Station", "Stations by Genre"))
	got := rg.order("Stations", "Your stations", "Stations by Genre")
	if want := []string{"Stations", "Your stations", "Stations by Genre"}; !slices.Equal(got, want) {
		t.Errorf("order %q, want %q", got, want)
	}
	if rg.tt.HasText("Sign in") {
		t.Errorf("a signed-in page offers to sign in: %q", rg.tt.Texts())
	}
}

func TestRadioPageKeepsApplesLiveShelvesWhenSonaCouldNotListTheLiveStations(t *testing.T) {
	rg := newRig(t, radioPath, 1132, 1253, radioFixture, fake(func(f *fakeRadio) { f.liveErr = errors.New("offline") }))
	rg.tt.Until("the page", rg.tt.Shows("Couldn't load the live stations", "On Air Now", "Latest Episodes"))
	if rg.tt.HasText("Couldn't load Apple Music's radio") {
		t.Error("one failure is worded as the whole page failing")
	}
}

func TestRadioPageSaysOnceThatNothingLoaded(t *testing.T) {
	rg := newRig(t, radioPath, 1132, 1253, fake(func(f *fakeRadio) { f.liveErr, f.groupingErr = errors.New("offline"), errors.New("offline") }))
	rg.tt.Until("the block", rg.tt.Shows("Couldn't load Apple Music's radio"))
	if rg.tt.HasText("Couldn't load the live stations") || rg.tt.HasText("Live now") {
		t.Errorf("a part of the page shows beside the whole-page block: %q", rg.tt.Texts())
	}
	if n := rg.count("Try again"); n != 1 {
		t.Errorf("%d Try again buttons, want one", n)
	}
	// One Retry asks for both again.
	liveBefore, groupingBefore := rg.api.count("live"), rg.api.count("grouping")
	radioFixture(rg)
	rg.api.set(func(f *fakeRadio) { f.liveErr, f.groupingErr = nil, nil })
	rg.click("Try again")
	rg.tt.Until("the page", rg.tt.Shows("Apple Music 1", "Latest Episodes"))
	if rg.api.count("live") <= liveBefore || rg.api.count("grouping") <= groupingBefore {
		t.Errorf("retry asked live %d→%d, grouping %d→%d", liveBefore, rg.api.count("live"), groupingBefore, rg.api.count("grouping"))
	}
}

func TestRadioPageShowsTheUsersStationsAboveAGroupingThatFailed(t *testing.T) {
	rg := newRig(t, radioPath, 1132, 1253, radioFixture, signedIn, fake(func(f *fakeRadio) {
		f.groupingErr = errors.New("offline")
		f.recent = []services.Station{trackStation("ra.mine", "My Station")}
	}))
	rg.tt.Until("the page", rg.tt.Shows("Couldn't load Apple Music's radio page", "My Station", "Apple Music 1"))
	if got := rg.order("Your stations", "Couldn't load Apple Music's radio page"); !slices.Equal(got, []string{"Your stations", "Couldn't load Apple Music's radio page"}) {
		t.Errorf("order %q", got)
	}
	if rg.tt.HasText("Couldn't load Apple Music's radio") && !rg.tt.HasText("Couldn't load Apple Music's radio page") {
		t.Error("the whole-page block shows")
	}
	// Retry asks for the page alone and it fills in.
	before := rg.api.count("live")
	rg.api.set(func(f *fakeRadio) { f.groupingErr = nil })
	rg.click("Try again")
	rg.tt.Until("the bands", rg.tt.Shows("Latest Episodes"))
	if rg.api.count("live") != before {
		t.Errorf("retry asked the live stations again (%d→%d)", before, rg.api.count("live"))
	}
}

func TestRadioPageSaysWhenNoStationIsLive(t *testing.T) {
	rg := newRig(t, radioPath, 1132, 1253, radioFixture, fake(func(f *fakeRadio) { f.live = nil }))
	rg.tt.Until("the empty state", rg.tt.Shows("No live stations", "Apple Music lists no live radio for your storefront right now."))
	// With no board, Apple's live shelves stay.
	if !rg.tt.HasText("On Air Now") {
		t.Error("Apple's live shelf was hidden though the board is empty")
	}
}

func TestRadioPageShowsFramesWhileItLoads(t *testing.T) {
	hold := make(chan struct{})
	rg := newRig(t, radioPath, 1132, 1253, radioFixture, fake(func(f *fakeRadio) { f.hold = hold }))
	rg.tt.Frame()
	if rg.count("Loading") < 2 {
		t.Errorf("loading frames: %d, want one for the board and one for the bands (%q)", rg.count("Loading"), rg.tt.Texts())
	}
	if !rg.tt.HasText("Live now") || !rg.tt.HasText("Radio") {
		t.Errorf("the headings wait for the data: %q", rg.tt.Texts())
	}
	close(hold)
	rg.tt.Until("the data", rg.tt.Shows("Apple Music 1 tagline", "Latest Episodes"))
	if rg.count("Loading") != 0 {
		t.Errorf("a loading frame stayed: %q", rg.tt.Texts())
	}
}

func TestRadioPageTilesPlayAndOpen(t *testing.T) {
	mine := trackStation("ra.mine", "My Station", func(s *services.Station) { s.ShowName = "A Show" })
	quiet := broadcaster("ra.quiet", "Quiet FM")
	rg := newRig(t, radioPath, 1132, 1253, radioFixture, signedIn, fake(func(f *fakeRadio) {
		f.recent = []services.Station{mine, quiet}
		f.details["ra.mine"] = services.StationDetail{Station: mine, Schedule: []services.StationEvent{}}
	}))
	rg.tt.Until("the shelf", rg.tt.Shows("My Station", "Quiet FM"))
	// The tile's subtitle: the tagline, else the show, else the provider.
	if !rg.tt.HasText("A Show") || !rg.tt.HasText("TuneIn") {
		t.Errorf("subtitles missing: %q", rg.tt.Texts())
	}
	// A station with a page has a play control named for it, apart from the
	// tile that opens the page.
	rg.click("Play My Station")
	rg.tt.Until("the station playing", func() bool { return slices.Equal(rg.api.plays(), []string{"ra.mine"}) })
	rg.click("My Station")
	if rg.router.Path() != "/station/ra.mine" {
		t.Errorf("opening the tile went to %q", rg.router.Path())
	}
	// A station without a page plays when its tile is opened.
	rg = newRig(t, radioPath, 1132, 1253, radioFixture, signedIn, fake(func(f *fakeRadio) { f.recent = []services.Station{quiet} }))
	rg.tt.Until("the shelf", rg.tt.Shows("Quiet FM"))
	rg.click("Quiet FM Play")
	rg.tt.Until("the broadcaster playing", func() bool { return slices.Equal(rg.api.plays(), []string{"ra.quiet"}) })
	if rg.router.Path() != radioPath {
		t.Errorf("a tile with no page navigated to %q", rg.router.Path())
	}
}

func TestPlaybackOfANewStationMakesTheUsersStationsStale(t *testing.T) {
	rg := newRig(t, radioPath, 1132, 1253, radioFixture, signedIn,
		fake(func(f *fakeRadio) { f.recent = []services.Station{trackStation("ra.mine", "My Station")} }))
	rg.tt.Until("the shelf", rg.tt.Shows("My Station"))
	emit := func(s services.PlaybackState) {
		rg.h.Bus.Emit(services.EventPlaybackState, s)
		rg.tt.Drain()
	}
	asked := func(want int) {
		t.Helper()
		rg.tt.Until("recent asked", func() bool { return rg.api.count("recent") == want })
	}
	asked(1)
	// A station of songs starts: Apple's history changes.
	emit(services.PlaybackState{State: "playing", Station: &services.QueueStation{ID: "ra.1", Name: "One"}})
	asked(2)
	// The same one again, a new state of it: once per change.
	emit(services.PlaybackState{State: "paused", Station: &services.QueueStation{ID: "ra.1", Name: "One"}})
	rg.tt.Drain()
	asked(2)
	// A live stream is a station too.
	emit(services.PlaybackState{State: "playing", Current: &services.QueueItem{ID: "ra.2", Live: true}})
	asked(3)
	// A song with no station changes nothing, and forgets the last station.
	emit(services.PlaybackState{State: "playing", Current: &services.QueueItem{ID: "123"}})
	asked(3)
	emit(services.PlaybackState{State: "playing", Current: &services.QueueItem{ID: "ra.2", Live: true}})
	asked(4)
	// An episode (not live, nor a station of songs) is not recorded.
	emit(services.PlaybackState{State: "playing", Current: &services.QueueItem{ID: "ra.ep"}})
	asked(4)
}

func TestRadioQueriesUseV020Keys(t *testing.T) {
	for _, c := range []struct{ got, want []any }{
		{keyGrouping, []any{"radio", "grouping"}},
		{keyLive, []any{"radio", "live"}},
		{keyRecent, []any{"radio", "recent"}},
		{kit.StationKey("ra.1"), []any{"radio", "station", "ra.1"}},
	} {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("key %v, want %v", c.got, c.want)
		}
	}
	// Each lives as long as v0.2.0 kept it.
	if editorialStale != 10*time.Minute || recentStale != time.Minute || kit.StationStale != 5*time.Minute {
		t.Errorf("stale times %v %v %v", editorialStale, recentStale, kit.StationStale)
	}
}
