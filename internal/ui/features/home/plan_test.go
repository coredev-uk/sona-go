package home

import (
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

// The cases of v0.2.0's plan.test.ts.

func planItem(kind, caption string) services.Item {
	return services.Item{Kind: kind, ID: kind, Caption: caption}
}

func planShelf(id, kind string, items []services.Item, source string) services.Section {
	return services.Section{ID: id, Kind: kind, Title: id, Items: items, Source: source}
}

func plainShelf(id string) services.Section {
	return planShelf(id, services.SectionShelf, []services.Item{planItem("album", "")}, "")
}

func ids(sections []services.Section) []string {
	out := make([]string, len(sections))
	for i, s := range sections {
		out[i] = s.ID
	}
	return out
}

func TestPlanLeadsWithTheFirstFeatureShelfAndListsPlainShelvesPastTheFirstFew(t *testing.T) {
	feature := func(id string) services.Section {
		return planShelf(id, services.SectionFeature, []services.Item{planItem("album", "")}, "")
	}
	sections := []services.Section{
		feature("top"),
		planShelf("recent", services.SectionShelf, []services.Item{planItem("album", "")}, recentlyPlayed),
		plainShelf("oasis"),
		feature("made"),
		plainShelf("zara"),
		planShelf("stations", services.SectionShelf, []services.Item{planItem("station", ""), planItem("station", "")}, ""),
		plainShelf("new"),
		planShelf("hero", services.SectionHero, []services.Item{planItem("album", "")}, ""),
		plainShelf("country"),
		plainShelf("pop"),
		feature("replay"),
	}
	plan := planHome(sections)
	if plan.lead == nil || plan.lead.ID != "top" {
		t.Fatalf("lead %v, want top", plan.lead)
	}
	// Recently Played, stations, features and heroes never move to the index.
	want := []string{"recent", "oasis", "made", "zara", "stations", "new", "hero", "replay"}
	if got := ids(plan.open); !slices.Equal(got, want) {
		t.Errorf("open %q, want %q", got, want)
	}
	if got := ids(plan.index); !slices.Equal(got, []string{"country", "pop"}) {
		t.Errorf("index %q, want country, pop", got)
	}
}

func TestPlanKeepsTheConcertsShelfWhichHasNoItemsOfItsOwnInTheOpen(t *testing.T) {
	sections := []services.Section{
		planShelf("top", services.SectionFeature, []services.Item{planItem("album", "")}, ""),
		plainShelf("a"), plainShelf("b"), plainShelf("c"), plainShelf("d"),
		planShelf("concerts", services.SectionConcerts, []services.Item{}, ""),
	}
	plan := planHome(sections)
	if got := ids(plan.open); !slices.Equal(got, []string{"a", "b", "c", "concerts"}) {
		t.Errorf("open %q", got)
	}
	if got := ids(plan.index); !slices.Equal(got, []string{"d"}) {
		t.Errorf("index %q", got)
	}
}

func TestPlanHasNoLeadWithoutAFeatureShelfWithItems(t *testing.T) {
	plan := planHome([]services.Section{planShelf("top", services.SectionFeature, []services.Item{}, ""), plainShelf("oasis")})
	if plan.lead != nil {
		t.Errorf("lead %v, want none", plan.lead.ID)
	}
	if got := ids(plan.open); !slices.Equal(got, []string{"top", "oasis"}) {
		t.Errorf("open %q", got)
	}
}

func TestPlanOfNothingIsEmpty(t *testing.T) {
	plan := planHome(nil)
	if plan.lead != nil || len(plan.open) != 0 || len(plan.index) != 0 {
		t.Errorf("plan %+v", plan)
	}
}

func TestIsStationsNeedsItemsAndEveryOneAStation(t *testing.T) {
	cases := []struct {
		name  string
		items []services.Item
		want  bool
	}{
		{"none", nil, false},
		{"all stations", []services.Item{planItem("station", ""), planItem("station", "")}, true},
		{"one other", []services.Item{planItem("station", ""), planItem("album", "")}, false},
	}
	for _, c := range cases {
		if got := isStations(services.Section{Items: c.items}); got != c.want {
			t.Errorf("%s: isStations %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWithoutSharedCaptionDropsACaptionEveryItemSharesAndKeepsVariedOnes(t *testing.T) {
	same := []services.Item{planItem("playlist", "Apple Music for You"), planItem("playlist", "Apple Music for You")}
	got := withoutSharedCaption(same)
	if got[0].Caption != "" || got[1].Caption != "" {
		t.Errorf("captions %q %q, want both dropped", got[0].Caption, got[1].Caption)
	}
	if same[0].Caption == "" {
		t.Error("the shelf's own items were changed")
	}
	varied := []services.Item{planItem("album", "New Release"), planItem("station", "Made for You")}
	if got := withoutSharedCaption(varied); &got[0] != &varied[0] {
		t.Error("varied captions were copied: want the same slice")
	}
	one := []services.Item{planItem("album", "New Release")}
	if got := withoutSharedCaption(one); got[0].Caption != "New Release" {
		t.Error("a single item's caption was dropped: it is not shared")
	}
	none := []services.Item{planItem("album", ""), planItem("album", "")}
	if got := withoutSharedCaption(none); &got[0] != &none[0] {
		t.Error("items without captions were copied")
	}
}

func TestCatalogueLineIsWhatItIsThenApplesReason(t *testing.T) {
	cases := []struct {
		kind, caption, want string
	}{
		{services.ItemAlbum, "New Release", "Album · New Release"},
		{services.ItemPlaylist, "", "Playlist"},
		{services.ItemStation, "Made for You", "Station · Made for You"},
		{services.ItemArtist, "", "Artist"},
		{services.ItemSong, "", "Song"},
		{services.ItemMusicVideo, "", "Music video"},
		{services.ItemCurator, "", ""},
		{services.ItemCurator, "Hosted by Zane", "Hosted by Zane"},
	}
	for _, c := range cases {
		if got := catalogueLine(services.Item{Kind: c.kind, Caption: c.caption}); got != c.want {
			t.Errorf("%s/%q: %q, want %q", c.kind, c.caption, got, c.want)
		}
	}
}

func TestBackdropURLReplacesTheSizeOfAGoArtPath(t *testing.T) {
	cases := map[string]string{
		"/art/k1/300x300.jpg":          "/art/k1/backdrop.jpg",
		"/art/k1/300.jpg":              "/art/k1/backdrop.jpg",
		"/art/k1/300x300.jpg?v=2":      "/art/k1/backdrop.jpg?v=2",
		"":                             "",
		"https://x.test/a/{w}x{h}.jpg": "https://x.test/a/{w}x{h}.jpg",
	}
	for in, want := range cases {
		if got := backdropURL(in); got != want {
			t.Errorf("backdropURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHoldsSaysWhatAShelfHoldsInTheOrderItMeetsTheKinds(t *testing.T) {
	var items []services.Item
	for range 8 {
		items = append(items, planItem("album", ""))
	}
	for range 4 {
		items = append(items, planItem("playlist", ""))
	}
	items = append(items, planItem("station", ""), planItem("link", ""))
	if got, want := holds(items), "8 albums · 4 playlists · 1 station"; got != want {
		t.Errorf("holds %q, want %q", got, want)
	}
	if got := holds([]services.Item{planItem("artist", ""), planItem("song", "")}); got != "1 artist · 1 song" {
		t.Errorf("holds %q", got)
	}
}

func TestHasAccountIsSignedInOrValidating(t *testing.T) {
	for state, want := range map[string]bool{"signed_in": true, "validating": true, "signed_out": false, "restoring": false, "needs_reauth": false, "": false} {
		if got := hasAccount(state); got != want {
			t.Errorf("hasAccount(%q) = %v, want %v", state, got, want)
		}
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) int64 { return now.Add(-d).UnixMilli() }
	cases := []struct {
		stamp int64
		want  string
	}{
		{0, "never"},
		{at(10 * time.Second), "just now"},
		{at(5 * time.Minute), "5 min. ago"},
		{at(59 * time.Minute), "59 min. ago"},
		{at(3 * time.Hour), "3 hr. ago"},
		{at(26 * time.Hour), "yesterday"},
		{at(3 * 24 * time.Hour), "3 days ago"},
	}
	for _, c := range cases {
		if got := relativeTime(c.stamp, now); got != c.want {
			t.Errorf("relativeTime(-%v) = %q, want %q", now.UnixMilli()-c.stamp, got, c.want)
		}
	}
}
