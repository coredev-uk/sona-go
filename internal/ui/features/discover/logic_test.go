package discover

import (
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

// item is an Item of a kind with the art the bands read: a web link to a video
// has a picture and a URL.
func item(kind, url string) services.Item {
	it := services.Item{Kind: kind, ID: kind, Name: kind, URL: url}
	if url != "" {
		it.Art.Medium = "/art/0123456789abcdef0123456789abcdef/320x180.jpg"
	}
	return it
}

func section(id, kind string, kinds ...string) services.Section {
	s := services.Section{ID: id, Kind: kind, Title: id}
	for _, k := range kinds {
		s.Items = append(s.Items, item(k, ""))
	}
	return s
}

func ids(sections []services.Section) []string {
	out := make([]string, len(sections))
	for i, s := range sections {
		out[i] = s.ID
	}
	return out
}

type bandShape struct {
	id       bandID
	sections []string
}

func sameShapes(a, b []bandShape) bool {
	return slices.EqualFunc(a, b, func(x, y bandShape) bool { return x.id == y.id && slices.Equal(x.sections, y.sections) })
}

func describe(bands []band) []bandShape {
	var out []bandShape
	for _, b := range bands {
		out = append(out, bandShape{b.id, ids(b.sections)})
	}
	return out
}

// bands.test.ts
func TestBandsGroupShelvesByWhatTheyHoldInTheBandsOrder(t *testing.T) {
	got := describe(toBands([]services.Section{
		section("tracks", services.SectionTracks, "song"),
		section("playlists", services.SectionShelf, "playlist", "playlist"),
		section("albums", services.SectionShelf, "album", "album"),
		section("bricks", services.SectionBricks, "curator"),
		section("live", services.SectionLive, "station"),
		section("episodes", services.SectionShelf, "station"),
		section("videos", services.SectionShelf, "music-video"),
		section("links", services.SectionLinks, "grouping"),
	}))
	want := []bandShape{
		{bandMusic, []string{"tracks", "albums"}},
		{bandPlaylists, []string{"playlists"}},
		{bandRadio, []string{"live", "episodes"}},
		{bandVideo, []string{"videos"}},
		{bandBrowse, []string{"bricks", "links"}},
	}
	if !sameShapes(got, want) {
		t.Errorf("bands %v, want %v", got, want)
	}
}

func TestBandsFileAMixedShelfByMostOfItsItemsAndATieByItsFirst(t *testing.T) {
	if got := toBands([]services.Section{section("mixed", services.SectionShelf, "album", "playlist", "playlist")}); got[0].id != bandPlaylists {
		t.Errorf("mostly playlists filed under %s", got[0].id)
	}
	if got := toBands([]services.Section{section("tie", services.SectionShelf, "album", "playlist")}); got[0].id != bandMusic {
		t.Errorf("a tie filed under %s, want the first item's band", got[0].id)
	}
}

func TestBandsCountAWebLinkToAVideoAsAVideoAndAnyOtherAsBrowsing(t *testing.T) {
	interviews := services.Section{ID: "interviews", Kind: services.SectionShelf, Items: []services.Item{item("link", "https://music.apple.com/gb/post/123")}}
	concerts := services.Section{ID: "concerts", Kind: services.SectionShelf, Items: []services.Item{item("link", "https://music.apple.com/gb/concerts")}}
	got := describe(toBands([]services.Section{interviews, concerts}))
	want := []bandShape{{bandVideo, []string{"interviews"}}, {bandBrowse, []string{"concerts"}}}
	if !sameShapes(got, want) {
		t.Errorf("bands %v, want %v", got, want)
	}
}

func TestBandsKeepAParagraphWithTheSectionItIntroduces(t *testing.T) {
	got := toBands([]services.Section{
		section("albums", services.SectionShelf, "album"),
		section("intro", services.SectionText),
		section("playlists", services.SectionShelf, "playlist"),
		section("outro", services.SectionText),
	})
	var shape [][]string
	for _, b := range got {
		shape = append(shape, ids(b.sections))
	}
	want := [][]string{{"albums"}, {"intro", "playlists", "outro"}}
	if !slices.EqualFunc(shape, want, slices.Equal) {
		t.Errorf("bands %v, want %v", shape, want)
	}
}

func TestBandsFileASectionOfOnlyParagraphsUnderBrowse(t *testing.T) {
	got := describe(toBands([]services.Section{section("only", services.SectionText)}))
	if want := []bandShape{{bandBrowse, []string{"only"}}}; !sameShapes(got, want) {
		t.Errorf("bands %v, want %v", got, want)
	}
}

// What the front page shows isn't repeated in the shelves below it
// (FrontPage.test.tsx / NewPage.test.tsx).
func TestIssueLeavesOutWhatTheFrontPageShowsUnlessThatWouldEmptyTheShelf(t *testing.T) {
	cards := func(kind string, ids ...string) []services.Item {
		var out []services.Item
		for _, id := range ids {
			out = append(out, services.Item{Kind: kind, ID: id, Name: "Card " + id})
		}
		return out
	}
	hero := services.Section{ID: "hero", Kind: services.SectionHero, Items: cards("playlist", "1", "2", "3", "4")}
	thisWeek := services.Section{ID: "New This Week", Kind: services.SectionShelf, Title: "New This Week", Items: append(cards("playlist", "3"), cards("album", "30")...)}
	spotlight := services.Section{ID: "Spotlight", Kind: services.SectionFeature, Title: "Spotlight", Items: cards("playlist", "4")}
	tracks := services.Section{ID: "tracks", Kind: services.SectionTracks, Title: "Songs", Items: cards("playlist", "3")}
	is := makeIssue([]services.Section{hero, thisWeek, spotlight, tracks})
	if len(is.hero) != 4 {
		t.Fatalf("the front page has %d cards, want 4", len(is.hero))
	}
	byID := map[string][]string{}
	for _, b := range is.bands {
		for _, s := range b.sections {
			for _, it := range s.Items {
				byID[s.ID] = append(byID[s.ID], it.Kind+":"+it.ID)
			}
		}
	}
	if want := []string{"album:30"}; !slices.Equal(byID["New This Week"], want) {
		t.Errorf("New This Week = %v, want %v", byID["New This Week"], want)
	}
	if want := []string{"playlist:4"}; !slices.Equal(byID["Spotlight"], want) {
		t.Errorf("a shelf the front page would empty = %v, want its own items %v", byID["Spotlight"], want)
	}
	if want := []string{"playlist:3"}; !slices.Equal(byID["tracks"], want) {
		t.Errorf("a track block is left whole: %v", byID["tracks"])
	}
}

func TestIssueHasNoFrontPageWithoutAHeroAndAnotherHeroIsASection(t *testing.T) {
	a := services.Section{ID: "a", Kind: services.SectionHero, Items: []services.Item{{Kind: "playlist", ID: "1"}}}
	b := services.Section{ID: "b", Kind: services.SectionHero, Items: []services.Item{{Kind: "playlist", ID: "2"}}}
	is := makeIssue([]services.Section{a, b})
	if len(is.hero) != 1 || is.hero[0].ID != "1" {
		t.Errorf("front page %v, want the first hero's cards", is.hero)
	}
	if len(is.bands) != 1 || is.bands[0].sections[0].ID != "b" {
		t.Errorf("bands %v, want the second hero as a section", is.bands)
	}
	if is := makeIssue([]services.Section{section("albums", services.SectionShelf, "album")}); len(is.hero) != 0 {
		t.Errorf("a page without a hero has a front page: %v", is.hero)
	}
}

func TestChartTitlesAreSonasWordsForKnownKeysAndAppleTitlesForTheRest(t *testing.T) {
	for key, want := range map[string]string{
		"songs":                "Top songs",
		"albums":               "Top albums",
		"playlists":            "Top playlists",
		"music-videos":         "Top music videos",
		"cityCharts":           "Top 25 songs by city",
		"dailyGlobalTopCharts": "Top 100 songs by country",
		"somethingNew":         "Apple's title",
	} {
		if got := chartTitle(services.Chart{Key: key, Title: "Apple's title"}); got != want {
			t.Errorf("chartTitle(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestFreshnessIsTheHonestLine(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) int64 { return now.Add(-d).UnixMilli() }
	for _, tc := range []struct {
		at    int64
		stale bool
		want  string
	}{
		{0, false, ""},
		{0, true, ""},
		{ago(10 * time.Second), false, "Updated just now"},
		{ago(12 * time.Minute), false, "Updated 12 min. ago"},
		{ago(3 * time.Hour), false, "Updated 3 hr. ago"},
		{ago(26 * time.Hour), false, "Updated yesterday"},
		{ago(5 * 24 * time.Hour), false, "Updated 5 days ago"},
		{ago(12 * time.Minute), true, "Couldn't reach Apple Music · showing the copy from 12 min. ago"},
	} {
		if got := freshness(tc.at, tc.stale, now); got != tc.want {
			t.Errorf("freshness(%v, %v) = %q, want %q", tc.at, tc.stale, got, tc.want)
		}
	}
}

// search-params.ts: a value is kept or it is dropped, and a URL carries only
// what is set.
func TestRouteURLsCarryOnlyWhatIsSet(t *testing.T) {
	for got, want := range map[string]string{
		roomURL("6828645964", ""):                   "/room/6828645964",
		roomURL("6828645964", "alphabet"):           "/room/6828645964?sort=alphabet",
		roomURL("a/b", ""):                          "/room/a%2Fb",
		chartsURL(""):                               "/charts",
		chartsURL("14"):                             "/charts?genre=14",
		chartURL("songs", "", ""):                   "/charts/songs",
		chartURL("cityCharts", "14", "135"):         "/charts/cityCharts?chartId=135&genre=14",
		chartURL("dailyGlobalTopCharts", "", "119"): "/charts/dailyGlobalTopCharts?chartId=119",
	} {
		if got != want {
			t.Errorf("url %q, want %q", got, want)
		}
	}
}

// The front page's layout: the lead takes the column's whole width, in the
// hero shape until that reaches leadMaxHeight and wider from there (never
// wider than the banner's 4:1), and the strip under it shows four cards a
// page, in a row of four or two rows of two.
func TestFrontLayoutAtTheWidthsOfTheReferencePage(t *testing.T) {
	l := layoutFor(1200, 0)
	if l.lead != 1200 || l.perPage != 4 || l.columns != 4 || l.rows != 1 {
		t.Errorf("at 1200: %+v", l)
	}
	for _, tc := range []struct{ width, view, height float32 }{
		{800, 0, 458},   // the hero shape: 800 / (1060/608)
		{941, 0, 539},   // still the hero shape, just under the cap
		{1200, 0, 540},  // capped: the box widens
		{1600, 0, 540},  // capped
		{2400, 0, 600},  // the banner's own 4:1, never wider
		{920, 700, 315}, // a short window: 45% of the page, so the strip shows
		{920, 400, 280}, // never under 280
		{1920, 1100, 495},
		{1920, 1400, 540}, // a tall window keeps the cap
	} {
		if h := layoutFor(tc.width, tc.view).artHeight(); h != tc.height {
			t.Errorf("at %v x %v: lead art height %v, want %v", tc.width, tc.view, h, tc.height)
		}
	}
	// The strip reaches 8 past the lead's edges on each side.
	if got := float32(l.columns)*l.card + float32(l.columns-1)*stripGap; got > l.lead+16 || got < l.lead+16-float32(l.columns) {
		t.Errorf("the strip is %v wide, want the lead's width and 16", got)
	}
	for _, tc := range []struct {
		width, lead   float32
		columns, rows int
	}{
		{1056, 1056, 4, 1},
		{880, 880, 4, 1},
		{879, 879, 2, 2},
		{648, 648, 2, 2},
	} {
		l := layoutFor(tc.width, 0)
		if l.lead != tc.lead || l.columns != tc.columns || l.rows != tc.rows || l.perPage != 4 || l.rowHeight != listRow {
			t.Errorf("at %v: %+v", tc.width, l)
		}
	}
}

// useLeadRotation.ts: one deadline; a hold starts the interval over.
func TestRotationAdvancesAtTheDeadlineAndRestartsAfterEachChange(t *testing.T) {
	var r rotation
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	free := func() bool { return false }
	step := func(at time.Time, running bool, lead int) (bool, time.Duration) {
		return r.step(at, running, lead, free)
	}

	if adv, wait := step(t0, true, 0); adv || wait != rotateEvery {
		t.Errorf("armed: advance %v wait %v, want a whole interval", adv, wait)
	}
	if adv, wait := step(t0.Add(rotateEvery-time.Millisecond), true, 0); adv || wait != time.Millisecond {
		t.Errorf("just before: advance %v wait %v", adv, wait)
	}
	if adv, _ := step(t0.Add(rotateEvery), true, 0); !adv {
		t.Error("at the deadline the lead moves on")
	}
	// The lead changed to 1: a whole interval from there.
	t1 := t0.Add(rotateEvery)
	if adv, wait := step(t1, true, 1); adv || wait != rotateEvery {
		t.Errorf("after a change: advance %v wait %v, want a whole interval", adv, wait)
	}
	// The user chose another card half way: the interval starts over.
	t2 := t1.Add(3 * time.Second)
	if adv, wait := step(t2, true, 4); adv || wait != rotateEvery {
		t.Errorf("a chosen card: advance %v wait %v, want a whole interval", adv, wait)
	}
	// A hold stops it; its end starts a whole interval.
	t3 := t2.Add(2 * time.Second)
	step(t3, false, 4)
	t4 := t3.Add(5 * time.Second)
	if adv, wait := step(t4, true, 4); adv || wait != rotateEvery {
		t.Errorf("after a hold: advance %v wait %v, want a whole interval", adv, wait)
	}
}

func TestRotationWaitsAnotherIntervalAtATickThatFindsThePageOccupied(t *testing.T) {
	var r rotation
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	busy := true
	occupied := func() bool { return busy }
	r.step(t0, true, 0, occupied)
	if adv, wait := r.step(t0.Add(rotateEvery), true, 0, occupied); adv || wait != rotateEvery {
		t.Errorf("occupied: advance %v wait %v, want another interval", adv, wait)
	}
	if adv, _ := r.step(t0.Add(2*rotateEvery-time.Millisecond), true, 0, occupied); adv {
		t.Error("moved on before the second interval ended")
	}
	busy = false
	if adv, _ := r.step(t0.Add(2*rotateEvery), true, 0, occupied); !adv {
		t.Error("a free page moves on at the deadline")
	}
}

func TestRotationStartsOverAfterAGapNoFrameRan(t *testing.T) {
	var r rotation
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	free := func() bool { return false }
	r.step(t0, true, 0, free)
	// The window was hidden: nothing drew for a minute. It does not move on
	// the moment it is back.
	back := t0.Add(time.Minute)
	if adv, wait := r.step(back, true, 0, free); adv || wait != rotateEvery {
		t.Errorf("after a gap: advance %v wait %v, want a whole interval", adv, wait)
	}
	// A timer that fires a little late is not a gap.
	if adv, _ := r.step(back.Add(rotateEvery+200*time.Millisecond), true, 0, free); !adv {
		t.Error("a late timer must still move on")
	}
}
