package concerts

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func TestPageListsTheAgendaByMonthAndDay(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 1000, exampleBand, sampleSinger, anotherAct)
	r.shows("Concerts", "April 2027", "June 2027", "Example Hall · London", "Another Act")
	for _, w := range []string{
		"Near London, the main city of your Apple Music storefront", "Updated just now", "Set location", "Filter concerts",
		"Placeholder Arena · Watford", "Nowhere Club · Reading", "6:00 PM · 6\u00a0mi away", "Your artist",
		"Wed, Apr 14", "Tue, Jun 1", "Wed", "14", "Tue", "1",
	} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
	// Two shows on the 14th share a day heading: one date column, then the rows.
	n := 0
	for _, w := range r.tt.Texts() {
		if w == "Wed" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the 14th has %d date columns, want 1", n)
	}
	if r.amp.last().Get("limit[popular-concerts]") != "100" {
		t.Errorf("asked %v, want a limit of 100", r.amp.last())
	}
	if r.amp.last().Get("filter[date]") != "" {
		t.Errorf("no filter asked for dates: %v", r.amp.last())
	}
}

func TestPageLinksTheLineupToTheArtistOnlyWhenThePageExists(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand)
	r.shows("Example Band with Opener Duo")
	r.click("Example Band with Opener Duo")
	if got := r.router.Path(); got != "/artist/900001" {
		t.Errorf("the line-up went to %q", got)
	}

	r = newRig(t, "/concerts", 1132, 800, exampleBand)
	r.h.Env.HasRoute = func(string) bool { return false } // the artist page is not ported
	r.shows("Example Band with Opener Duo")
	_ = r.tt.Click("Example Band with Opener Duo")
	if got := r.router.Path(); got != "/concerts" {
		t.Errorf("plain text navigated to %q", got)
	}
}

func TestDatesPresetGoesToTheURLAndToApple(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 1000, exampleBand, sampleSinger, anotherAct)
	r.shows("April 2027")
	r.click("Dates")
	r.click("This week")
	if got := r.router.Location(); got != "/concerts?when=week" {
		t.Errorf("location %q", got)
	}
	want, _ := presetRange(whenWeek, today)
	r.tt.Until("a listing for the week", func() bool {
		return r.amp.last().Get("filter[date]") == want.from+".."+want.to
	})
	// Nothing on the fixture's dates is this week.
	r.shows("No concerts", "Clear filters")

	// Any date clears it.
	r.click("Dates")
	r.click("Any date")
	if got := r.router.Location(); got != "/concerts" {
		t.Errorf("location %q", got)
	}
	r.shows("April 2027")
}

func TestChoosingDatesTakesTwoDaysInOrder(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 1000, exampleBand, sampleSinger, anotherAct)
	r.shows("April 2027")
	r.click("Dates")
	r.click("Choose dates…")
	if got := r.router.Location(); got != "/concerts?when=dates" {
		t.Errorf("location %q", got)
	}
	r.shows("From", "To", "Choose both dates")

	// A day before today is outside From's range: shown invalid, not applied.
	r.clickField("From")
	r.tt.Type("2001-01-01")
	r.tt.Drain()
	if got := r.router.Location(); got != "/concerts?when=dates" {
		t.Errorf("a past day was applied: %q", got)
	}
	// Half a day typed changes nothing either.
	r.tt.Type("-0")
	r.tt.Drain()
	if got := r.router.Location(); got != "/concerts?when=dates" {
		t.Errorf("half a day was applied: %q", got)
	}
	r.clickField("From")
	r.tt.Key(ui.Cmd, ui.KeyA)
	r.tt.Type("2027-04-01")
	r.tt.Drain()
	// Only a From: still no range.
	if got := r.router.Location(); got != "/concerts?from=2027-04-01&when=dates" {
		t.Errorf("location %q", got)
	}
	if !r.tt.HasText("Choose both dates") {
		t.Error("the hint went with one day")
	}
	// To may not come before From.
	r.clickField("To")
	r.tt.Type("2027-03-01")
	r.tt.Drain()
	if got := r.router.Location(); got != "/concerts?from=2027-04-01&when=dates" {
		t.Errorf("a To before From was applied: %q", got)
	}
	r.tt.Key(ui.Cmd, ui.KeyA)
	r.tt.Type("2027-04-30")
	r.tt.Drain()
	if got := r.router.Location(); got != "/concerts?from=2027-04-01&to=2027-04-30&when=dates" {
		t.Errorf("location %q", got)
	}
	r.tt.Until("the April listing", func() bool { return r.amp.last().Get("filter[date]") == "2027-04-01..2027-04-30" })
	r.tt.Until("only April", func() bool { return !r.tt.HasText("Another Act") && r.tt.HasText("Sample Singer") })
	r.lacks("Choose both dates")

	// A preset forgets the typed days.
	r.click("Dates")
	r.click("Next 30 days")
	if got := r.router.Location(); got != "/concerts?when=month" {
		t.Errorf("location %q", got)
	}
	if strings.Contains(r.router.Location(), "from") {
		t.Error("the preset kept the typed days")
	}
}

func TestGenreFilterNarrowsTheListing(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 1000, exampleBand, sampleSinger, anotherAct)
	r.shows("April 2027", "Genres", "All genres")
	r.click("Genres")
	r.click("Alternative")
	if got := r.router.Location(); got != "/concerts?genre=20" {
		t.Errorf("location %q", got)
	}
	r.tt.Until("only the alternative show", func() bool {
		return !r.tt.HasText("Sample Singer") && r.tt.HasText("Example Band with Opener Duo")
	})
	if got := r.amp.last().Get("filter[genre]"); got != "20" {
		t.Errorf("asked for genre %q", got)
	}
	r.click("Genres")
	r.click("All genres")
	if got := r.router.Location(); got != "/concerts" {
		t.Errorf("location %q", got)
	}
}

func TestYourArtistsSwitchShowsOnlyTheirShows(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 1000, exampleBand, sampleSinger, anotherAct)
	r.shows("Your artists (2)", "Example Band with Opener Duo")
	r.click("Your artists (2)")
	if got := r.router.Location(); got != "/concerts?yours=true" {
		t.Errorf("location %q", got)
	}
	r.tt.Until("only followed artists", func() bool {
		return !r.tt.HasText("Example Band with Opener Duo") && r.tt.HasText("Sample Singer") && r.tt.HasText("Another Act")
	})
	r.click("Your artists (2)")
	r.tt.Until("everyone again", r.tt.Shows("Example Band with Opener Duo"))
}

func TestShowMoreConcertsAsksForAnotherHundred(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 9000, bulk(120)...)
	r.shows("Band 0", "Show more concerts")
	r.lacks("Band 110")
	r.click("Show more concerts")
	r.tt.Until("120 concerts", r.tt.Shows("Band 119"))
	if got := r.amp.last().Get("limit[popular-concerts]"); got != "200" {
		t.Errorf("asked for %s concerts, want 200", got)
	}
	// 120 of 200: nothing more.
	r.lacks("Show more concerts")
}

func TestPageSaysWhenNothingIsListed(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800)
	r.shows("No concerts", "No upcoming concerts. Please check back later. A larger city nearby may have some.", "Set location")
	r.click("Set location")
	r.shows("Search cities")
}

func TestPageSaysWhenNothingMatchesTheFilters(t *testing.T) {
	r := newRig(t, "/concerts?genre=20", 1132, 800, anotherAct)
	r.shows("No concerts", "Apple Music lists nothing near London for these filters. Widen the dates or choose another genre.")
	r.click("Clear filters")
	if got := r.router.Location(); got != "/concerts" {
		t.Errorf("location %q", got)
	}
	r.shows("Another Act")
}

func TestPageShowsTheErrorAndRetries(t *testing.T) {
	amp := &fakeApple{}
	amp.set(exampleBand)
	amp.fail.Store(true) // before the first frame asks
	r := newRigAmp(t, amp, "/concerts", 1132, 800, nil, nil)
	r.shows("Couldn't load the concerts", "Try again")
	r.amp.fail.Store(false)
	r.click("Try again")
	r.shows("Example Band with Opener Duo")
	r.lacks("Couldn't load the concerts")
}

func TestPageShowsTheFrameWhileLoadingAndKeepsTheListWhileAFilterLoads(t *testing.T) {
	amp := &fakeApple{}
	amp.set(exampleBand, sampleSinger, anotherAct)
	release := amp.hold()
	hn := uitest.New(t, uitest.Apple(http.NotFoundHandler()))
	hn.Network.ServeHandler(ampHost, amp)
	r := newRigOver(t, hn, amp, "/concerts", 1132, 1000, nil)
	// Nothing yet: the agenda's frame, named for assistive technology; the
	// header already has its title and the location button.
	r.shows("Concerts", "Loading", "Set location")
	r.lacks("April 2027")
	release()
	r.shows("April 2027", "Genres")

	// A new filter keeps the list on screen until its listing arrives.
	release = amp.hold()
	r.click("Genres")
	r.click("Alternative")
	r.tt.Drain()
	r.tt.Drain()
	for _, w := range []string{"Sample Singer", "Another Act", "April 2027"} {
		if !r.tt.HasText(w) {
			t.Errorf("the list went while the filter loaded: no %q; texts %q", w, r.tt.Texts())
		}
	}
	// The list above stays rather than a frame of placeholders; it is marked
	// busy for assistive technology only (a11y_test.go).
	release()
	r.tt.Until("the filtered list", func() bool { return !r.tt.HasText("Sample Singer") && r.tt.HasText("Example Band with Opener Duo") })
}

func TestShowMoreSaysLoadingWhileItFetches(t *testing.T) {
	amp := &fakeApple{}
	amp.set(bulk(120)...)
	hn := uitest.New(t, uitest.Apple(http.NotFoundHandler()))
	hn.Network.ServeHandler(ampHost, amp)
	r := newRigOver(t, hn, amp, "/concerts", 1132, 9000, nil)
	r.shows("Band 0", "Show more concerts")
	release := amp.hold()
	r.click("Show more concerts")
	r.shows("Loading…", "Band 0") // the list stays, the button waits
	r.lacks("Show more concerts")
	release()
	r.tt.Until("every concert", r.tt.Shows("Band 119"))
}

// staleAPI answers with a listing Apple could not refresh.
type staleAPI struct{ concertsAPI }

func (s staleAPI) Concerts(ctx context.Context, req services.ConcertsRequest) (services.ConcertsPage, error) {
	p, err := s.concertsAPI.Concerts(ctx, req)
	p.Stale, p.FetchedAt = true, time.Now().Add(-3*time.Hour).UnixMilli()
	return p, err
}

func TestSublineSaysWhenTheCopyIsOld(t *testing.T) {
	amp := &fakeApple{}
	amp.set(exampleBand)
	hn := uitest.New(t, uitest.Apple(http.NotFoundHandler()))
	hn.Network.ServeHandler(ampHost, amp)
	r := newRigOver(t, hn, amp, "/concerts", 1132, 800, func(a concertsAPI) concertsAPI { return staleAPI{a} })
	r.shows("Couldn't reach Apple Music · showing the copy from 3 hr. ago")
	r.lacks("Updated")
}

func TestModuleDeclaresItsRouteAndNavigation(t *testing.T) {
	h := uitest.New(t, uitest.Apple(http.NotFoundHandler()))
	m := New(h.Env)
	if m.ID != "concerts" || len(m.Routes) != 1 || m.Routes[0].Pattern != "/concerts" {
		t.Errorf("module %q routes %+v", m.ID, m.Routes)
	}
	if len(m.Nav) != 1 {
		t.Fatalf("nav %+v", m.Nav)
	}
	nav := m.Nav[0]
	if nav.Label() != "Concerts" || nav.Icon != "ticket" || nav.To != "/concerts" || nav.Order != 25 || nav.Section != module.NavPrimary {
		t.Errorf("nav %+v", nav)
	}
	if m.Init == nil {
		t.Error("no Init: auth:changed is not followed")
	}
}
