package search

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// seedRecent remembers a mix of entries, oldest first: two terms (one per
// scope), a catalog artist, a library album, a library song and a catalog
// song.
func seedRecent(t *testing.T, r *pageRig) {
	t.Helper()
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(r.api.RememberTerm(ctx, "alpha", scopeCatalog))
	must(r.api.RememberTerm(ctx, "beta", scopeLibrary))
	must(r.api.RememberItem(ctx, services.RecentItem{Kind: services.ItemArtist, ID: "9", Source: services.SourceCatalog, Name: "Blue Band", Subtitle: "Rock"}))
	must(r.api.RememberItem(ctx, services.RecentItem{Kind: services.ItemAlbum, ID: "l.5", Source: services.SourceLibrary, Name: "Library Album", Subtitle: "Al"}))
	must(r.api.RememberItem(ctx, services.RecentItem{Kind: services.ItemSong, ID: "i.7", Source: services.SourceLibrary, Name: "Library Song", Subtitle: "Al"}))
	must(r.api.RememberItem(ctx, services.RecentItem{Kind: services.ItemSong, ID: "3", Source: services.SourceCatalog, Name: "Catalog Song", Subtitle: "Blue Band"}))
}

func TestRecentShelfListsTheTermsAndItemsOfTheScopeOnScreen(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{})
	seedRecent(t, r)
	r.s.refreshRecent()
	r.tt.Until("the catalog entries", r.tt.Shows("Recently searched", "Clear", "alpha", "Blue Band", "Catalog Song"))
	for _, w := range []string{"beta", "Library Album", "Library Song"} {
		if r.tt.HasText(w) {
			t.Errorf("the catalog scope lists %q", w)
		}
	}
	if !r.tt.HasText("Artist · Rock") || !r.tt.HasText("Song · Blue Band") {
		t.Errorf("entries lack their kind and subtitle; texts %q", r.tt.Texts())
	}

	r.router.Replace("/search?scope=library")
	r.tt.Until("the library entries", r.tt.Shows("beta", "Library Album"))
	for _, w := range []string{"alpha", "Blue Band", "Catalog Song"} {
		if r.tt.HasText(w) {
			t.Errorf("the library scope lists %q", w)
		}
	}
	// A library song has no page to return to: not listed at all.
	if r.tt.HasText("Library Song") {
		t.Error("a library song is listed")
	}
}

func TestRecentShelfIsHiddenWhenThereIsNothing(t *testing.T) {
	r := newPageRig(t, "/search?scope=library", rigOpts{})
	r.tt.Until("the page", r.tt.Shows("Search your library"))
	if r.tt.HasText("Recently searched") || r.tt.HasText("Clear") {
		t.Error("the shelf shows with nothing remembered")
	}
}

func TestRecentTermEntrySearchesAgainKeepingTheScope(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{})
	seedRecent(t, r)
	r.s.refreshRecent()
	r.tt.Until("the entries", r.tt.Shows("alpha"))
	r.click("alpha")
	if got := r.loc(); got != "/search?q=alpha" {
		t.Errorf("opened %q", got)
	}
	r2 := newPageRig(t, "/search?scope=library", rigOpts{})
	seedRecent(t, r2)
	r2.s.refreshRecent()
	r2.tt.Until("the entries", r2.tt.Shows("beta"))
	r2.click("beta")
	if got := r2.loc(); got != "/search?q=beta&scope=library" {
		t.Errorf("library term opened %q", got)
	}
}

func TestRecentItemEntryOpensItsPageAndIsRememberedAgain(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{})
	seedRecent(t, r)
	r.s.refreshRecent()
	r.tt.Until("the entries", r.tt.Shows("Blue Band", "Catalog Song"))
	r.click("Blue Band")
	if got := r.loc(); got != "/artist/9" {
		t.Errorf("opened %q", got)
	}
	// A recent song opens its song page.
	r.router.Replace("/search")
	r.tt.Until("the entries", r.tt.Shows("Catalog Song"))
	r.click("Catalog Song")
	if got := r.loc(); got != "/song/3" {
		t.Errorf("a recent song opened %q", got)
	}
	// Remembering again moves it to the top.
	r.tt.Until("the artist on top", func() bool {
		got, _ := r.api.Recent(context.Background())
		return len(got) > 0 && got[0].Name == "Catalog Song"
	})
}

func TestRecentEntryXForgetsIt(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{})
	seedRecent(t, r)
	r.s.refreshRecent()
	r.tt.Until("the entries", r.tt.Shows("alpha", "Blue Band"))
	rect, _ := r.tt.Find("alpha")
	r.tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	r.tt.Frame()
	r.click("Remove alpha from recent searches")
	r.tt.Until("alpha gone", func() bool { return !r.tt.HasText("alpha") })
	if slices.Contains(r.recentKinds(), "term:catalog:alpha") {
		t.Error("the service still remembers alpha")
	}
	if !r.tt.HasText("Blue Band") {
		t.Error("another entry went with it")
	}
	if _, ok := r.tt.Find("Remove Blue Band from recent searches"); !ok {
		t.Error("an item entry has no remove button")
	}
}

func TestRecentClearForgetsEverything(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{})
	seedRecent(t, r)
	r.s.refreshRecent()
	r.tt.Until("the entries", r.tt.Shows("Clear", "alpha"))
	r.click("Clear")
	r.tt.Until("the shelf gone", func() bool { return !r.tt.HasText("Recently searched") })
	if got := r.recentKinds(); len(got) != 0 {
		t.Errorf("after Clear the service remembers %v", got)
	}
}

func TestRecentShelfShowsAtMostTwelve(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{})
	for i := range 15 {
		if err := r.api.RememberTerm(context.Background(), fmt.Sprintf("term %02d", i), scopeCatalog); err != nil {
			t.Fatal(err)
		}
	}
	r.s.refreshRecent()
	r.tt.Until("the entries", r.tt.Shows("term 14"))
	shown := 0
	for _, w := range r.tt.Texts() {
		var n int
		if _, err := fmt.Sscanf(w, "term %d", &n); err == nil {
			shown++
		}
	}
	if shown != maxShelf {
		t.Errorf("%d entries shown, want %d", shown, maxShelf)
	}
}

func TestRememberingIsBestEffortAndNeverKeepsLibrarySongs(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{})
	song := services.Item{Kind: services.ItemSong, ID: "i.1", Name: "Lib", Song: &services.Song{ID: "i.1", Source: services.SourceLibrary}}
	r.s.rememberItem(song, services.SourceLibrary)
	r.s.rememberTerm("   ", scopeCatalog) // blank: nothing
	r.s.rememberTerm("gamma", scopeCatalog)
	r.untilRecent("term:catalog:gamma")
	if got := r.recentKinds(); !slices.Equal(got, []string{"term:catalog:gamma"}) {
		t.Errorf("remembered %v, want only the term", got)
	}
}

func TestRecentEntryEnumerationHelpers(t *testing.T) {
	all := []services.RecentSearch{
		{Kind: services.RecentTerm, Scope: scopeCatalog, Term: "a"},
		{Kind: services.RecentTerm, Scope: scopeLibrary, Term: "b"},
		{Kind: services.ItemAlbum, Source: services.SourceLibrary, Name: "L"},
		{Kind: services.ItemAlbum, Source: services.SourceCatalog, Name: "C"},
	}
	if got := recentEntries(all, scopeCatalog); len(got) != 2 || got[0].Term != "a" || got[1].Name != "C" {
		t.Errorf("catalog entries %+v", got)
	}
	if got := recentEntries(all, scopeLibrary); len(got) != 2 || got[0].Term != "b" || got[1].Name != "L" {
		t.Errorf("library entries %+v", got)
	}
	if p := recentPath(services.RecentSearch{Kind: services.ItemSong, ItemID: "3", Source: services.SourceCatalog}); p != "/song/3" {
		t.Errorf("catalog song path %q", p)
	}
	if p := recentPath(services.RecentSearch{Kind: services.ItemSong, ItemID: "i.3", Source: services.SourceLibrary}); p != "" {
		t.Errorf("library song path %q", p)
	}
	if p := recentPath(services.RecentSearch{Kind: services.ItemAlbum, ItemID: "l.1", Source: services.SourceLibrary}); p != "/album/l.1" {
		t.Errorf("library album path %q", p)
	}
}
