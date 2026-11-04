package home

import (
	"fmt"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

func shelfOf(id, title string, n int) services.Section {
	s := section(id, services.SectionShelf, title, albums("Shelf", n, 10)...)
	s.SeeAll = true
	return s
}

func TestShelfPageShowsTheShelfInFullUnderItsTitle(t *testing.T) {
	r := newRig(t, "/for-you/made", 1132, 1253, withDisc(func(f *fakeDiscover) {
		f.shelves = map[string]services.Section{"made": shelfOf("made", "Made for You", 14)}
	}))
	r.until("the shelf", "Made for You", "Shelf 1", "Shelf 14")
	if _, ok := r.tt.Find("Made for You"); !ok {
		t.Error("the grid and the heading are not named for the shelf")
	}
	if _, _, n := r.disc.calls(); n != 1 {
		t.Errorf("HomeShelf asked %d times, want once", n)
	}
	// A tile opens its item.
	r.click("Shelf 3")
	if got := r.path(); got != "/album/Shelf3" {
		t.Errorf("the tile opened %q", got)
	}
	if got := r.home.titles["made"]; got != "Made for You" {
		t.Errorf("the window title is %q once the shelf was read", got)
	}
}

func TestShelfPageLoadsWithPlaceholderTiles(t *testing.T) {
	gate := make(chan struct{})
	r := newRig(t, "/for-you/made", 1132, 1253, withDisc(func(f *fakeDiscover) {
		f.shelves = map[string]services.Section{"made": shelfOf("made", "Made for You", 3)}
		f.shelfGate = gate
	}))
	r.tt.Drain()
	r.tt.Drain()
	if _, ok := r.tt.Find("Loading"); !ok {
		t.Errorf("no loading grid; texts %q", r.tt.Texts())
	}
	requireNoTexts(t, r, "Made for You", "Couldn't load this shelf")
	close(gate)
	r.until("the shelf", "Made for You", "Shelf 3")
	requireNoTexts(t, r, "Loading")
}

func TestShelfPageErrorOffersARetry(t *testing.T) {
	r := newRig(t, "/for-you/made", 1132, 1253, withDisc(func(f *fakeDiscover) { f.shelfErr = errDown }))
	r.until("the error", "Couldn't load this shelf", "Try again")
	r.disc.set(func(f *fakeDiscover) {
		f.shelfErr = nil
		f.shelves = map[string]services.Section{"made": shelfOf("made", "Made for You", 2)}
	})
	r.click("Try again")
	r.until("the shelf", "Made for You", "Shelf 2")
	requireNoTexts(t, r, "Couldn't load this shelf")
}

func TestShelfPageFollowsTheShelfTheRouteNames(t *testing.T) {
	r := newRig(t, "/for-you/a", 1132, 1253, withDisc(func(f *fakeDiscover) {
		f.shelves = map[string]services.Section{"a": shelfOf("a", "First Shelf", 2), "b": shelfOf("b", "Second Shelf", 2)}
	}))
	r.until("the first", "First Shelf")
	r.router.Push("/for-you/b")
	r.until("the second", "Second Shelf")
	requireNoTexts(t, r, "First Shelf")
	for i := range 2 {
		if !r.tt.HasText(fmt.Sprintf("Shelf %d", i+1)) {
			t.Errorf("the second shelf lacks Shelf %d", i+1)
		}
	}
}

func TestSeeAllOnHomeLeadsToTheShelfPage(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withDisc(func(f *fakeDiscover) {
		f.page = samplePage()
		f.shelves = map[string]services.Section{"made": shelfOf("made", "Made for You", 5)}
	}))
	r.until("home", "Made for You", "See all")
	r.click("See all")
	r.until("the shelf page", "Shelf 5")
	if got := r.path(); got != "/for-you/made" {
		t.Errorf("path %q", got)
	}
}
