package search

import (
	"strings"
	"testing"
)

func TestGroupPageHeaderLinksBackToTheSearch(t *testing.T) {
	r := newPageRig(t, "/search/song?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the page", r.tt.Shows("Songs", "Apple Music results for ", "“blue”", "Blue Song 1"))
	r.click("“blue”")
	if got := r.loc(); got != "/search?q=blue" {
		t.Errorf("the term opened %q", got)
	}
}

func TestGroupPageTitlesAnUnknownGroupInSentenceCase(t *testing.T) {
	r := newPageRig(t, "/search/podcast?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the page", r.tt.Shows("Podcasts"))
}

func TestGroupPageLoadsPagesAsTheEndComesNear(t *testing.T) {
	// 20 songs in pages of 12: the window shows the end of the first page,
	// so the second is asked for without scrolling.
	r := newPageRig(t, "/search/song?q=blue", rigOpts{signedIn: true})
	r.tt.Until("both pages", r.tt.Shows("Blue Song 1", "Blue Song 20"))
	if n := r.apple.requests("offset=12"); n != 1 {
		t.Errorf("the second page was asked for %d times, want once", n)
	}
	if r.apple.requests("offset=20") != 0 {
		t.Error("a page past the end was asked for")
	}
}

func TestGroupPageWaitsForTheEndToComeWithinAScreen(t *testing.T) {
	// 300 songs, 40 to a page, in a short window: the end is far away.
	r := newPageRig(t, "/search/song?q=blue", rigOpts{
		signedIn: true, width: 1132, height: 360,
		setup: func(a *pageApple) { a.songsTotal, a.songsPage = 300, 40 },
	})
	r.tt.Until("the first page", r.tt.Shows("Blue Song 1"))
	for range 6 {
		r.tt.Drain()
	}
	if n := r.apple.requests("offset=40"); n != 0 {
		t.Fatalf("the second page was asked for before the end came near (%d)", n)
	}
	// Scrolling down brings the end within a screen: the next page loads.
	for i := 0; i < 10 && r.apple.requests("offset=40") == 0; i++ {
		r.tt.Scroll(600, 200, 0, 600)
		r.tt.Drain()
	}
	r.tt.Until("the second page", func() bool { return r.apple.requests("offset=40") > 0 })
}

func TestGroupPageSongsPlayFromTheRowAndRemember(t *testing.T) {
	r := newPageRig(t, "/search/song?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the songs", r.tt.Shows("Blue Song 2", "Blue Song 20"))
	rect, _ := r.tt.Find("Blue Song 2")
	r.tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	r.tt.Frame()
	r.click("Play Blue Song 2")
	r.tt.Until("the song to play", func() bool { return r.play.plays() >= 1 })
	if r.play.plays() != 1 {
		t.Fatalf("played %d times", r.play.plays())
	}
	r.play.mu.Lock()
	start, n, from := r.play.start[0], len(r.play.refs[0]), r.play.from[0]
	r.play.mu.Unlock()
	if start != 1 || n != 20 || from.ID != "search" {
		t.Errorf("played %d songs from %d (%+v), want all 20 from index 1", n, start, from)
	}
	r.untilRecent("term:catalog:blue", "song:Blue Song 2")
}

func TestGroupPageItemsAreATileGrid(t *testing.T) {
	r := newPageRig(t, "/search/album?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the albums", r.tt.Shows("Albums", "Blue Hour", "Blue Monday"))
	r.click("Blue Monday")
	if got := r.loc(); got != "/album/6" {
		t.Errorf("opened %q", got)
	}
	r.untilRecent("term:catalog:blue", "album:Blue Monday")
}

func TestGroupPageFailureWithNothingLoadedHasItsOwnTitleAndRetry(t *testing.T) {
	down := func(a *pageApple) { a.fail["/v1/catalog/gb/search"] = 400 }
	r := newPageRig(t, "/search/album?q=blue", rigOpts{signedIn: true, setup: down})
	r.tt.Until("the error", r.tt.Shows("Couldn't load albums"))
	r.apple.setFail("/v1/catalog/gb/search", 0)
	r.click("Try again")
	r.tt.Until("the albums", r.tt.Shows("Blue Hour"))

	r2 := newPageRig(t, "/search/mystery?q=blue", rigOpts{signedIn: true, setup: down})
	r2.tt.Until("the default error", r2.tt.Shows("Couldn't load these results"))
}

func TestGroupPageLaterPageFailureKeepsTheListAndOffersRetry(t *testing.T) {
	r := newPageRig(t, "/search/song?q=blue", rigOpts{signedIn: true, setup: func(a *pageApple) { a.failFrom = 12 }})
	r.tt.Until("the failure under the list", r.tt.Shows("Blue Song 1", "Blue Song 12", "Couldn't load more"))
	if r.tt.HasText("Couldn't load songs") {
		t.Error("the first page's error shows over a list")
	}
	r.apple.mu.Lock()
	r.apple.failFrom = 0
	r.apple.mu.Unlock()
	r.click("Try again")
	r.tt.Until("the rest", r.tt.Shows("Blue Song 20"))
	if r.tt.HasText("Couldn't load more") {
		t.Error("the error stays after the retry")
	}
}

func TestGroupPageEmptyHasItsOwnTitle(t *testing.T) {
	r := newPageRig(t, "/search/song?q=nothing", rigOpts{signedIn: true})
	r.tt.Until("the empty state", r.tt.Shows("No songs for “nothing”", "Apple Music has nothing more of this kind for the search."))
	if !strings.Contains(strings.Join(r.tt.Texts(), "|"), "Songs") {
		t.Error("the page lost its title")
	}
}
