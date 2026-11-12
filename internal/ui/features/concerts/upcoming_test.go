package concerts

import (
	"net/http"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// shelfRig shows draw (another page's shelf) over the fake Apple.
func shelfRig(t *testing.T, w int, fail bool, draw func(c *ui.Context, r *rig), specs ...spec) *rig {
	t.Helper()
	amp := &fakeApple{}
	amp.set(specs...)
	amp.fail.Store(fail)
	hn := uitest.New(t, uitest.Apple(http.NotFoundHandler()))
	hn.Network.ServeHandler(ampHost, amp)
	r := &rig{t: t, h: hn, amp: amp, router: ui.NewRouter("/home")}
	computerIn(t, "Europe/London")
	hn.Env.Nav = r.router
	hn.Env.HasRoute = func(p string) bool { return p == concertsPath }
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Padding(24).Children(func() { draw(c, r) })
		})
		kit.Toaster(c)
	}, w, 900)
	return r
}

// An artist's shows: two near the chosen city, two elsewhere.
var artistShows = func() []spec {
	mk := func(id, venue, city, start string, near bool) spec {
		return spec{
			id: id, lineup: "Example Band", venue: venue, city: city, zone: "Europe/London", start: start,
			artist: "900001", near: near, lat: 51.6, lon: -0.1, tickets: []string{"Seller", "https://tickets.example.com/" + id},
		}
	}
	return []spec{
		mk("a1", "Example Hall", "London", "2027-04-14T17:00:00Z", true),
		mk("a2", "Placeholder Arena", "Watford", "2027-05-02T18:30:00Z", true),
		mk("a3", "Faraway Stadium", "Leeds", "2027-09-01T18:00:00Z", false),
		mk("a4", "Last Dome", "Bristol", "2027-10-01T18:00:00Z", false),
	}
}()

func TestArtistConcertsShelfShowsThreeThenAll(t *testing.T) {
	r := shelfRig(t, 1132, false, func(c *ui.Context, r *rig) { ArtistConcerts(c, r.h.Env, "900001", true) }, artistShows...)
	r.shows("Upcoming concerts", "Show all 4", "Example Hall", "Placeholder Arena", "Faraway Stadium", "More concerts near London")
	// The venue leads on an artist's own page, with the city under it; the
	// name is left out, and so is a portrait and a link.
	r.shows("London", "Watford", "Wed", "Apr")
	r.tt.Lacks("Last Dome")
	r.lacksExactly("Example Band")
	r.tt.MustClick("Show all 4")
	r.shows("Last Dome", "Show fewer")
	r.tt.MustClick("Show fewer")
	r.tt.Until("three rows again", func() bool { return !r.tt.HasText("Last Dome") })
	r.tt.MustClick("More concerts near London")
	if got := r.router.Path(); got != "/concerts" {
		t.Errorf("the link went to %q", got)
	}
}

func TestArtistConcertsNamesTheLineupOnlyWhenOthersPlay(t *testing.T) {
	with := artistShows[0]
	with.lineup = "Example Band with Opener Duo"
	r := shelfRig(t, 1132, false, func(c *ui.Context, r *rig) { ArtistConcerts(c, r.h.Env, "900001", true) }, with, artistShows[1])
	r.shows("Upcoming concerts", "Example Hall", "Example Band with Opener Duo")
	r.tt.Lacks("Show all")
	r.lacksExactly("Example Band")
}

func TestArtistConcertsDrawsNothingWithNoShowsOrOnFailure(t *testing.T) {
	r := shelfRig(t, 1132, false, func(c *ui.Context, r *rig) { ArtistConcerts(c, r.h.Env, "900099", true) }, artistShows...)
	answered(r, func() store.Result[services.ArtistConcerts] { return dataOf(r.h.Env).artist("900099") })
	r.tt.Lacks("Upcoming concerts")

	r = shelfRig(t, 1132, true, func(c *ui.Context, r *rig) { ArtistConcerts(c, r.h.Env, "900001", true) }, artistShows...)
	answered(r, func() store.Result[services.ArtistConcerts] { return dataOf(r.h.Env).artist("900001") })
	r.tt.Lacks("Upcoming concerts")
}

func TestConcertsShelfShowsTheNextSixInTwoColumns(t *testing.T) {
	r := shelfRig(t, 1132, false, func(c *ui.Context, r *rig) { ConcertsShelf(c, r.h.Env, true) }, bulk(8)...)
	r.shows("Concerts in London", "See all", "Band 0", "Band 5", "Jan")
	r.tt.Lacks("Band 6")
	r.tt.MustClick("See all")
	if got := r.router.Path(); got != "/concerts" {
		t.Errorf("See all went to %q", got)
	}
	// Both columns are there: the fourth show starts the second, level with the first.
	a, _ := r.tt.Find("Band 0")
	b, _ := r.tt.Find("Band 3")
	if a.X >= b.X || a.Y != b.Y {
		t.Errorf("Band 0 at %v, Band 3 at %v: not two columns", a, b)
	}
}

func TestConcertsShelfIsOneColumnWhenNarrow(t *testing.T) {
	r := shelfRig(t, 800, false, func(c *ui.Context, r *rig) { ConcertsShelf(c, r.h.Env, true) }, bulk(8)...)
	r.shows("Concerts in London", "Band 0", "Band 5")
	a, _ := r.tt.Find("Band 0")
	b, _ := r.tt.Find("Band 3")
	if a.X != b.X || a.Y >= b.Y {
		t.Errorf("Band 0 at %v, Band 3 at %v: not one column", a, b)
	}
}

func TestConcertsShelfSaysWhenNothingIsListedAndLeavesOutOnFailure(t *testing.T) {
	r := shelfRig(t, 1132, false, func(c *ui.Context, r *rig) { ConcertsShelf(c, r.h.Env, true) })
	r.shows("Concerts in London", "No concerts", "No upcoming concerts. Please check back later.", "Set location")
	r.tt.MustClick("Set location")
	if got := r.router.Path(); got != "/concerts" {
		t.Errorf("the action went to %q", got)
	}

	r = shelfRig(t, 1132, true, func(c *ui.Context, r *rig) { ConcertsShelf(c, r.h.Env, true) }, bulk(2)...)
	answered(r, func() store.Result[services.ConcertsPage] {
		return dataOf(r.h.Env).concerts(requestOf(dayRange{}, false, "", pageSize))
	})
	r.tt.Lacks("Concerts", "Band 0")
}
