package song

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// creditRig is the Airbag page with its credits showing. The lookup knows
// Nigel Godrich as artist "ng1" and nobody else (the song's own artist,
// Radiohead, is on the page and needs no lookup).
func creditRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t, 1132, 1400, airbag())
	r.catalog.credits = map[string]string{"Nigel Godrich": "ng1"}
	r.open()
	r.tt.Until("the credits", r.tt.Shows("Nigel Godrich", "Third Person", "Chris Blair"))
	return r
}

// creditLabel is the credited name's link as assistive technology names it.
func creditLabel(name string) string { return name + ", artist page" }

// tabTo presses Tab until the control called label has the focus.
func (r *rig) tabTo(label string) {
	r.t.Helper()
	for range 100 {
		if tr := access.Read(r.t, r.tt.Tester); tr.Focus != nil && tr.Focus.Label == label {
			return
		}
		r.tt.Key(0, ui.KeyTab)
	}
	r.t.Fatalf("Tab never reached %q", label)
}

// A click on a credited person whom exactly one artist of the catalogue is
// named after opens that artist's page.
func TestCreditClickOpensTheArtistPage(t *testing.T) {
	r := creditRig(t)
	r.click(creditLabel("Nigel Godrich"))
	r.tt.Until("the artist page", r.tt.Shows("ARTIST PAGE ng1"))
	if got := r.route(); got != "/artist/ng1" {
		t.Errorf("route %q, want /artist/ng1", got)
	}
	if got := r.catalog.creditLookups(); !slices.Equal(got, []string{"Nigel Godrich"}) {
		t.Errorf("lookups %q, want one for the name clicked", got)
	}
}

// A credit named like one of the song's own artists (Radiohead, on this page
// with its ID) opens that artist at once, with no lookup, and the credit
// link is not the byline's (it has its own name).
func TestCreditOfASongArtistOpensAtOnceWithoutALookup(t *testing.T) {
	r := creditRig(t)
	r.click(creditLabel("Radiohead"))
	r.tt.Until("the artist page", r.tt.Shows("ARTIST PAGE a1"))
	if got := r.catalog.creditLookups(); len(got) != 0 {
		t.Errorf("lookups %q for the song's own artist, want none", got)
	}
}

// A person no one artist is known to be opens the search of artists for the
// name, where the user picks: the lookup answered "", and the name is
// written into the search's query.
func TestCreditWithoutOneArtistOpensTheArtistSearch(t *testing.T) {
	r := creditRig(t)
	r.click(creditLabel("Third Person"))
	r.tt.Until("the search", r.tt.Shows("SEARCH artist q=Third Person"))
	if got := r.route(); got != "/search/artist?q=Third+Person" {
		t.Errorf("route %q, want the artists search for the name", got)
	}
}

// A failed lookup falls back to the search too, with no error shown; the
// failure is not kept, so the next click asks again.
func TestCreditLookupFailureOpensTheSearchAndAsksAgain(t *testing.T) {
	r := creditRig(t)
	r.catalog.creditErr = errors.New("offline")
	r.click(creditLabel("Chris Blair"))
	r.tt.Until("the search", r.tt.Shows("SEARCH artist q=Chris Blair"))
	r.router.Back()
	r.open()
	r.catalog.mu.Lock()
	r.catalog.creditErr = nil
	r.catalog.credits["Chris Blair"] = "cb1"
	r.catalog.mu.Unlock()
	r.tabTo(creditLabel("Chris Blair"))
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the artist page", r.tt.Shows("ARTIST PAGE cb1"))
	if got := r.catalog.creditLookups(); !slices.Equal(got, []string{"Chris Blair", "Chris Blair"}) {
		t.Errorf("lookups %q, want the failed name asked again", got)
	}
}

// A name looked up opens where it did the first time without asking again,
// for as long as the song's page shows, whether it found an artist or not.
func TestCreditResolutionsAreKeptPerSongPage(t *testing.T) {
	r := creditRig(t)
	for range 2 {
		r.click(creditLabel("Nigel Godrich"))
		r.tt.Until("the artist page", r.tt.Shows("ARTIST PAGE ng1"))
		r.router.Back()
		r.open()
		r.tt.Until("the credits", r.tt.Shows("Third Person"))
		r.click(creditLabel("Third Person"))
		r.tt.Until("the search", r.tt.Shows("SEARCH artist q=Third Person"))
		r.router.Back()
		r.open()
		r.tt.Until("the credits", r.tt.Shows("Nigel Godrich"))
	}
	if got := r.catalog.creditLookups(); !slices.Equal(got, []string{"Nigel Godrich", "Third Person"}) {
		t.Errorf("lookups %q, want each name asked once", got)
	}
}

// Enter on the focused name does what a click does: it is a Tab stop, the
// people in reading order, and Space does nothing.
func TestCreditEnterOpensTheArtistPage(t *testing.T) {
	r := creditRig(t)
	r.tabTo(creditLabel("Nigel Godrich"))
	r.tt.Key(0, ui.KeySpace)
	r.tt.Drain()
	if got := r.route(); got != "/song/1440" {
		t.Fatalf("Space opened %q", got)
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the artist page", r.tt.Shows("ARTIST PAGE ng1"))

	// And a name that goes to the search, from the keyboard.
	r.router.Back()
	r.open()
	r.tt.Until("the credits", r.tt.Shows("Third Person"))
	r.tabTo(creditLabel("Third Person"))
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the search", r.tt.Shows("SEARCH artist q=Third Person"))

	// A song's own artist, from the keyboard, with no lookup.
	r.router.Back()
	r.open()
	r.tt.Until("the credits", r.tt.Shows("Third Person"))
	r.tabTo(creditLabel("Radiohead"))
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the artist page", r.tt.Shows("ARTIST PAGE a1"))
	if got := r.catalog.creditLookups(); !slices.Equal(got, []string{"Nigel Godrich", "Third Person"}) {
		t.Errorf("lookups %q", got)
	}
}

// While a lookup runs the name shows nothing until it has run for the busy
// time, then says so to assistive technology (the spinner beside the name has
// no role of its own); a lookup that answers at once never does.
func TestCreditBusyShowsOnlyAfterAWhile(t *testing.T) {
	const busy = "Finding Nigel Godrich…"
	t.Run("slow", func(t *testing.T) {
		r := creditRig(t)
		r.s.busyAfter = 400 * time.Millisecond
		gate := make(chan struct{})
		r.catalog.creditGate = gate
		var heard []string
		hear := func() []string {
			heard = append(heard, r.tt.Announcements()...)
			return heard
		}
		r.click(creditLabel("Nigel Godrich"))
		r.tt.Drain()
		if slices.Contains(hear(), busy) {
			t.Fatal("the lookup is busy before the busy time")
		}
		r.tt.Until("the busy state", func() bool { return slices.Contains(hear(), busy) })
		if got := r.route(); got != "/song/1440" {
			t.Errorf("the page moved to %q while the lookup runs", got)
		}
		// Enter on the same name while it runs asks nothing more.
		r.tt.Key(0, ui.KeyEnter)
		close(gate)
		r.tt.Until("the artist page", r.tt.Shows("ARTIST PAGE ng1"))
		if got := r.catalog.creditLookups(); len(got) != 1 {
			t.Errorf("lookups %q, want one", got)
		}
		if n := countOf(hear(), busy); n != 1 {
			t.Errorf("announced %d times, want once", n)
		}
	})
	t.Run("quick", func(t *testing.T) {
		r := creditRig(t)
		r.click(creditLabel("Nigel Godrich"))
		r.tt.Until("the artist page", r.tt.Shows("ARTIST PAGE ng1"))
		if slices.Contains(r.tt.Announcements(), busy) {
			t.Error("a quick lookup showed its busy state")
		}
	})
}

func countOf(all []string, one string) int {
	n := 0
	for _, a := range all {
		if a == one {
			n++
		}
	}
	return n
}

// An answer that arrives after the user has gone elsewhere opens nothing, and
// of two names clicked in a row only the last opens.
func TestCreditAnswerOpensOnlyWhereAndWhenWanted(t *testing.T) {
	t.Run("left the page", func(t *testing.T) {
		r := creditRig(t)
		gate := make(chan struct{})
		r.catalog.creditGate = gate
		r.click(creditLabel("Nigel Godrich"))
		r.router.Push("/library/songs")
		r.tt.Until("the songs", r.tt.Shows("SONGS PAGE"))
		close(gate)
		r.tt.Drain()
		time.Sleep(50 * time.Millisecond)
		r.tt.Drain()
		if got := r.route(); got != "/library/songs" {
			t.Errorf("the answer moved the page to %q", got)
		}
	})
	t.Run("last click wins", func(t *testing.T) {
		r := creditRig(t)
		gate := make(chan struct{})
		r.catalog.creditGate = gate
		r.click(creditLabel("Nigel Godrich"))
		r.click(creditLabel("Third Person"))
		close(gate)
		r.tt.Until("the search", r.tt.Shows("SEARCH artist q=Third Person"))
		r.tt.Drain()
		time.Sleep(50 * time.Millisecond)
		r.tt.Drain()
		if got := r.route(); got != "/search/artist?q=Third+Person" {
			t.Errorf("route %q, want the last name's search", got)
		}
	})
}

// With no artist page or no artists' search to open, a credited name is plain
// text, not a link and not a Tab stop.
func TestCreditNamesAreTextWithoutThePagesTheyOpen(t *testing.T) {
	for _, routes := range []map[string]bool{
		{},
		{"/artist/": true},
		{"/search/": true},
	} {
		r := newRig(t, 1132, 1400, airbag())
		r.routes = routes
		r.open()
		r.tt.Until("the credits", r.tt.Shows("Nigel Godrich"))
		tr := access.Read(t, r.tt.Tester)
		if _, ok := tr.Find(ui.RoleLink, creditLabel("Nigel Godrich")); ok {
			t.Errorf("routes %v: a credited name is a link", routes)
		}
		_ = r.tt.Click("Nigel Godrich")
		r.tt.Drain()
		if got := r.route(); got != "/song/1440" {
			t.Errorf("routes %v: a click moved the page to %q", routes, got)
		}
		if got := r.catalog.creditLookups(); len(got) != 0 {
			t.Errorf("routes %v: lookups %q", routes, got)
		}
	}
}
