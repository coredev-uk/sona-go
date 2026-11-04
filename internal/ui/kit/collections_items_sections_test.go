package kit

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

func sectionOf(id, kind, title string, items ...services.Item) services.Section {
	return services.Section{ID: id, Kind: kind, Title: title, Items: items}
}

func wideCards(n int) []services.Item {
	var items []services.Item
	for i := 1; i <= n; i++ {
		items = append(items, wideItem(strconv.Itoa(i), artPath(strconv.Itoa(i), 1000, 250), ""))
	}
	return items
}

// sectionsApp builds EditorialSections on a page of the given width.
func sectionsApp(t *testing.T, width int, sections []services.Section, o EditorialOpts) (*itemsApp, *ui.Tester, *EditorialState) {
	t.Helper()
	st := &EditorialState{}
	a, tt := newItemsApp(t, width, 1400, nil, func(c *ui.Context, ctx ItemsCtx) {
		EditorialSections(c, st, sections, ctx, o)
	})
	a.load(t, tt, 0)
	return a, tt, st
}

func TestEditorialSectionsRenderEachKind(t *testing.T) {
	stationSec := sectionOf("st", services.SectionStations, "Local Broadcasters", stationItem(false, true))
	sections := []services.Section{
		sectionOf("h", services.SectionHero, "", wideCards(3)...),
		sectionOf("b", services.SectionBricks, "Browse", wideCards(2)...),
		sectionOf("t", services.SectionTracks, "Top Songs", trackItems(4)...),
		sectionOf("l", services.SectionLinks, "More to Explore", exploreItems()...),
		stationSec,
		{ID: "x", Kind: services.SectionText, Text: "A paragraph about the page."},
		sectionOf("p", services.SectionPlaylists, "ignored", albumItems(2)...),
		sectionOf("f", services.SectionFeature, "Made for You", albumItems(2)...),
		sectionOf("s", services.SectionShelf, "Fresh Picks", albumItems(2)...),
	}
	_, tt, _ := sectionsApp(t, 1000, sections, EditorialOpts{First: true})
	for _, want := range []string{
		"Browse", "Top Songs", "More to Explore", "Local Broadcasters", "A paragraph about the page.",
		"Playlists", "Made for You", "Fresh Picks", "Song 1", "Concerts", "Local FM",
	} {
		if !tt.HasText(want) {
			t.Errorf("no %q: %q", want, tt.Texts())
		}
	}
	// A playlists section is worded by the shell, not by its (empty) title.
	if tt.HasText("ignored") {
		t.Error("a playlists section shows its own title")
	}
	// Sections stack in order.
	var ys []float32
	for _, name := range []string{"Browse", "Top Songs", "More to Explore", "Local Broadcasters", "Playlists", "Made for You", "Fresh Picks"} {
		r, _ := tt.Find(name)
		ys = append(ys, r.Y)
	}
	if !slices.IsSorted(ys) {
		t.Errorf("sections are out of order: %v", ys)
	}
}

func TestEditorialSectionsHeroesAreCroppedWideRowsOfMin420AndBricksOfMin240(t *testing.T) {
	// 900 wide: two columns of 420, three of 240.
	heroes := sectionOf("h", services.SectionHero, "Heroes", wideCards(3)...)
	bricks := sectionOf("b", services.SectionBricks, "Bricks", wideCards(4)...)
	_, tt, _ := sectionsApp(t, 900+48, []services.Section{heroes, bricks}, EditorialOpts{First: true})
	// Heroes: two a page (so "1 of 2"), bricks: three a page ("1 of 2").
	if n := countText(tt, "1 of 2"); n != 2 {
		t.Errorf("%d paged rows, want both: %q", n, tt.Texts())
	}
	// Each card is named twice (its label and its title). Item 3 is only in
	// the bricks' first page; Item 4 in neither's.
	if n3, n4 := countText(tt, "Item 3"), countText(tt, "Item 4"); n3 != 2 || n4 != 0 {
		t.Errorf("Item 3 named %d times (want the bricks' card), Item 4 %d (want none): %q", n3, n4, tt.Texts())
	}
	if n2 := countText(tt, "Item 2"); n2 != 4 {
		t.Errorf("Item 2 named %d times, want a hero's and a brick's card: %q", n2, tt.Texts())
	}
	h1, _ := tt.Find("Previous in Heroes")
	b1, _ := tt.Find("Previous in Bricks")
	if h1.Y >= b1.Y {
		t.Errorf("Heroes %v above Bricks %v", h1, b1)
	}
}

func TestEditorialSectionsHeroArtIsRequestedAtTheHeroShape(t *testing.T) {
	// One 420 column: the 4:1 banner is cropped to 1060:608, asked for at it.
	a, tt, _ := sectionsApp(t, 420+48, []services.Section{sectionOf("h", services.SectionHero, "", wideCards(1)...)}, EditorialOpts{First: true})
	a.load(t, tt, 1)
	if got := a.fake.requests(); !slices.Equal(got, [][2]int{{420, 241}}) {
		t.Errorf("requests %v, want [420 241]", got)
	}
	// Bricks are bound by their 2:1 frame: the 4:1 banner letterboxed whole.
	a, tt, _ = sectionsApp(t, 240+48, []services.Section{sectionOf("b", services.SectionBricks, "", wideCards(1)...)}, EditorialOpts{First: true})
	a.load(t, tt, 1)
	if got := a.fake.requests(); !slices.Equal(got, [][2]int{{240, 60}}) {
		t.Errorf("requests %v, want [240 60]", got)
	}
}

func TestEditorialSectionsSeeAllDefaultsToTheRoomAndMoreForOverridesIt(t *testing.T) {
	shelf := sectionOf("s", services.SectionShelf, "Fresh Picks", albumItems(5)...)
	shelf.Room = "77"
	a, tt, _ := sectionsApp(t, 3*160+2*24+48, []services.Section{shelf}, EditorialOpts{First: true})
	if err := tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.nav, []string{"/room/77"}) {
		t.Errorf("nav %v", a.rec.nav)
	}
	if _, ok := tt.Find("Show all 5"); ok {
		t.Error("a section with a room has the Show all toggle")
	}

	a, tt, _ = sectionsApp(t, 3*160+2*24+48, []services.Section{shelf}, EditorialOpts{First: true, MoreFor: func(s services.Section) string {
		return "/seeall/" + s.ID
	}})
	if err := tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.nav, []string{"/seeall/s"}) {
		t.Errorf("nav %v", a.rec.nav)
	}

	// Without a room, "Show all" opens the rest in place.
	plain := sectionOf("s", services.SectionShelf, "Fresh Picks", albumItems(5)...)
	_, tt, _ = sectionsApp(t, 3*160+2*24+48, []services.Section{plain}, EditorialOpts{First: true})
	if _, ok := tt.Find("Show all 5"); !ok {
		t.Errorf("no toggle: %q", tt.Texts())
	}
	if _, ok := tt.Find("See all"); ok {
		t.Error("a section without a room links See all")
	}
}

func TestEditorialSectionsFeatureSectionsUseTilesOfMin208(t *testing.T) {
	// 904 wide fits four tiles of 208, five of 160.
	width := 4*208 + 3*24 + 48
	feature := sectionOf("f", services.SectionFeature, "Made for You", albumItems(6)...)
	shelf := sectionOf("s", services.SectionShelf, "Fresh Picks", albumItems(6)...)
	_, tt, _ := sectionsApp(t, width, []services.Section{feature, shelf}, EditorialOpts{First: true})
	count := func(prefix string) (n int) {
		for _, s := range tt.Texts() {
			if strings.HasPrefix(s, prefix) {
				n++
			}
		}
		return n
	}
	// Both shelves show "Album 1".. as titles and "Artist 1".. as subtitles:
	// four of the feature's, five of the cover shelf's.
	if got := count("Artist "); got != 4+5 {
		t.Errorf("%d tiles show, want 4 feature + 5 shelf: %q", got, tt.Texts())
	}
}

func TestEditorialSectionsATextSectionIsAMutedParagraphCappedAt72chAndFirstHasNoGap(t *testing.T) {
	long := strings.Repeat("Words that run on and on across the page until the line has to break. ", 12)
	sections := []services.Section{
		{ID: "x", Kind: services.SectionText, Text: long},
		{ID: "y", Kind: services.SectionText, Text: "Second paragraph."},
	}
	_, tt, _ := sectionsApp(t, 1200, sections, EditorialOpts{First: true})
	first, ok := tt.Find(long)
	if !ok {
		t.Fatalf("no paragraph: %q", tt.Texts())
	}
	if first.W > textMax+0.5 || first.W < 400 {
		t.Errorf("the paragraph is %v wide, want a line capped at %v", first.W, float32(textMax))
	}
	if first.Y != 24 {
		t.Errorf("the first paragraph starts at %v, want the page's 24 padding", first.Y)
	}
	second, _ := tt.Find("Second paragraph.")
	// 40 above the second, whatever the first's height.
	if gap := second.Y - (first.Y + first.H); gap != 40 {
		t.Errorf("the gap between paragraphs is %v, want 40", gap)
	}
	// Not the first on its page: 40 above it.
	_, tt, _ = sectionsApp(t, 1200, sections[:1], EditorialOpts{})
	if got, _ := tt.Find(long); got.Y != 24+40 {
		t.Errorf("a paragraph not first starts at %v, want 64", got.Y)
	}
}

func TestEditorialSectionsPlayingAStationPlaysTheSectionsMatchingItem(t *testing.T) {
	other := stationItem(false, true)
	other.ID, other.Name = "ra.2", "Other FM"
	other.Station = &services.Station{ID: "ra.2", Name: "Other FM", HasPage: false, Playable: true}
	sec := sectionOf("st", services.SectionStations, "Local Broadcasters", stationItem(false, true), other,
		albumItem(1)) // an item without a station is left out of the list
	a, tt, _ := sectionsApp(t, 700, []services.Section{sec}, EditorialOpts{First: true})
	if err := tt.Click("Other FM Play"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.played, []string{"station:ra.2"}) {
		t.Errorf("played %v", a.rec.played)
	}
	if tt.HasText("Album 1") {
		t.Error("an item without a station is in the station list")
	}
}

func TestEditorialSectionsShareTheMenusAndSurviveSectionsWithTheSameTitle(t *testing.T) {
	sections := []services.Section{
		sectionOf("a", services.SectionHero, "", wideCards(1)...),
		sectionOf("b", services.SectionHero, "", wideCards(1)...),
		sectionOf("c", services.SectionLinks, "", exploreItems()...),
		sectionOf("d", services.SectionLinks, "", exploreItems()...),
		sectionOf("e", services.SectionShelf, "Same", albumItems(2)...),
		sectionOf("f", services.SectionShelf, "Same", albumItems(2)...),
	}
	_, tt, _ := sectionsApp(t, 900, sections, EditorialOpts{First: true})
	if _, ok := tt.Find("Same"); !ok {
		t.Errorf("sections with equal titles: %q", tt.Texts())
	}
}
