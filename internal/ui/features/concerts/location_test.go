package concerts

import (
	"errors"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// refusingSave is the service with a city that cannot be saved.
type refusingSave struct{ concertsAPI }

func (refusingSave) SetLocation(string) (services.ConcertLocation, error) {
	return services.ConcertLocation{}, errors.New("disk full")
}

func TestLocationDialogListsSuggestedCities(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand)
	r.shows("Set location", "Example Band with Opener Duo")
	r.click("Set location")
	r.shows("Location",
		"Concerts are listed near the city you choose. Sona keeps your choice on this computer and tells Apple Music only the area it covers.",
		"Search cities", "London", ", United Kingdom", "Manchester")
	// The storefront's own city is the chosen one; none is a custom city yet.
	r.lacks("Use my storefront’s main city")
	if !r.tt.Focused("Search cities") {
		t.Errorf("the field has no focus; texts %q", r.tt.Texts())
	}
	// Escape closes it, with the field's text forgotten.
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Drain()
	r.lacks("Search cities")
}

func TestLocationDialogSearchesAndPicksTheFirstMatchOnEnter(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand)
	r.shows("Example Band with Opener Duo")
	before := r.amp.last().Get("geoHashLocation")
	r.click("Set location")
	r.shows("Search cities")
	r.tt.Type("manch")
	r.shows("Manchester")
	// The list shows the last answer while a new search loads, so Enter picks
	// from the answer to the whole term only once it is in and drawn: the
	// typed letters are searched one by one, and an earlier answer starts
	// with another city.
	r.tt.Until("the matches of the whole term", func() bool {
		_, ok := store.Peek[[]services.ConcertPlace](r.h.Env.Queries, store.Key{"concerts", "places", "manch"})
		return ok
	})
	r.tt.Drain()
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the dialog to close", func() bool { return !r.tt.HasText("Search cities") })
	if got := r.h.Settings.Get().Concerts.City; got == "" {
		t.Error("the city was not saved")
	}
	// The listing is asked again, near the new place, and the button names it.
	r.tt.Until("the listing near Manchester", func() bool { return r.amp.last().Get("geoHashLocation") != before })
	r.shows("Near Manchester", "Manchester")
	r.lacks("Set location")
}

// holdingPlaces is the service with the answer for one search term held back
// until release is closed.
type holdingPlaces struct {
	concertsAPI
	term    string
	release chan struct{}
}

func (h holdingPlaces) Places(query string) []services.ConcertPlace {
	if query == h.term {
		<-h.release
	}
	return h.concertsAPI.Places(query)
}

// Fast typing leaves the list showing an earlier term's cities while the
// whole term's answer loads: Enter in that time picks nothing.
func TestLocationDialogEnterWaitsForTheWholeTermsAnswer(t *testing.T) {
	release := make(chan struct{})
	r := newRigAmp(t, func() *fakeApple { a := &fakeApple{}; a.set(exampleBand); return a }(), "/concerts", 1132, 800, nil, func(api concertsAPI) concertsAPI {
		return holdingPlaces{concertsAPI: api, term: "manch", release: release}
	})
	r.shows("Example Band with Opener Duo")
	r.click("Set location")
	r.shows("Search cities")
	r.tt.Type("manch")
	r.shows("Manchester") // the suggestions, shown while "manch" loads
	r.tt.Drain()
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	if !r.tt.HasText("Search cities") {
		t.Fatal("Enter picked a city from an earlier term's answer")
	}
	if got := r.h.Settings.Get().Concerts.City; got != "" {
		t.Fatalf("a city %q was saved from an earlier term's answer", got)
	}
	close(release)
	r.tt.Until("the matches of the whole term", func() bool {
		_, ok := store.Peek[[]services.ConcertPlace](r.h.Env.Queries, store.Key{"concerts", "places", "manch"})
		return ok
	})
	r.tt.Drain()
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the dialog to close", func() bool { return !r.tt.HasText("Search cities") })
	if got := r.h.Settings.Get().Concerts.City; got == "" {
		t.Error("the city was not saved once the answer was in")
	}
}

func TestLocationDialogSaysWhenNoCityMatches(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand)
	r.shows("Set location")
	r.click("Set location")
	r.shows("Search cities")
	r.tt.Type("zzqqx")
	r.shows("No cities found. Try the name of a bigger city nearby.")
	r.tt.Until("the announcement", func() bool {
		return slices.Contains(r.tt.Announcements(), "No cities found")
	})
	r.lacks("Cities that match")
	// Enter with nothing to pick changes nothing.
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	if r.h.Settings.Get().Concerts.City != "" {
		t.Error("Enter saved a city with no match")
	}
}

func TestLocationDialogKeysMoveAmongTheCities(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand)
	r.shows("Set location")
	r.click("Set location")
	r.shows("Suggestions", "Manchester")
	cities := r.cityLabels()
	if len(cities) < 3 {
		t.Fatalf("only %d cities listed: %q", len(cities), r.tt.Texts())
	}
	key := func(k ui.Key) { r.tt.Key(0, k); r.tt.Drain(); r.tt.Drain() }
	focused := func(label string) {
		t.Helper()
		if !r.tt.Focused(label) {
			t.Errorf("focus is not on %q; texts %q", label, r.tt.Texts())
		}
	}
	key(ui.KeyDown) // field -> first city
	focused(cities[0])
	key(ui.KeyDown)
	focused(cities[1])
	key(ui.KeyEnd)
	focused(cities[len(cities)-1])
	key(ui.KeyDown) // stays on the last
	focused(cities[len(cities)-1])
	key(ui.KeyHome)
	focused(cities[0])
	key(ui.KeyUp) // from the first, back to the field
	focused("Search cities")
	if r.h.Settings.Get().Concerts.City != "" {
		t.Error("moving saved a city")
	}
}

// cityLabels are the listed cities' names for assistive technology, in order.
func (r *rig) cityLabels() []string {
	var out []string
	for _, w := range r.tt.Texts() {
		if len(w) > len(", United Kingdom") && w[len(w)-len(", United Kingdom"):] == ", United Kingdom" {
			out = append(out, w)
		}
	}
	return out
}

func TestLocationDialogPickingSavesAndResetGoesBack(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 800, exampleBand)
	r.shows("Example Band with Opener Duo")
	r.click("Set location")
	r.shows("Manchester")
	r.click("Manchester, United Kingdom")
	r.tt.Until("the dialog to close", func() bool { return !r.tt.HasText("Search cities") })
	if got := r.h.Settings.Get().Concerts.City; got == "" {
		t.Fatal("the city was not saved")
	}
	r.shows("Near Manchester")
	r.lacks("the main city of your Apple Music storefront")

	// With a city of its own the dialog offers the way back, and marks the chosen one.
	r.click("Manchester")
	r.shows("Search cities", "Use my storefront’s main city")
	r.click("Use my storefront’s main city")
	r.tt.Until("the dialog to close", func() bool { return !r.tt.HasText("Search cities") })
	if got := r.h.Settings.Get().Concerts.City; got != "" {
		t.Errorf("the city is %q after the reset", got)
	}
	r.shows("Near London, the main city of your Apple Music storefront", "Set location")
}

func TestLocationDialogToastsWhenTheCityCannotBeSaved(t *testing.T) {
	amp := &fakeApple{}
	amp.set(exampleBand)
	r := newRigAmp(t, amp, "/concerts", 1132, 800, nil, func(a concertsAPI) concertsAPI { return refusingSave{a} })
	r.shows("Example Band with Opener Duo")
	r.click("Set location")
	r.shows("Manchester")
	r.click("Manchester, United Kingdom")
	r.tt.Until("the toast", r.tt.Shows("Couldn't save your city"))
	// The dialog stays, with the cities still pickable.
	r.shows("Search cities", "Manchester")
	if r.h.Settings.Get().Concerts.City != "" {
		t.Error("a refused city was kept")
	}
}
