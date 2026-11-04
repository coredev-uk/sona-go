package radio

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// The cases of v0.2.0's bands.test.ts.

func stationItem(kind string) services.Item {
	return services.Item{Kind: services.ItemStation, ID: kind, Station: &services.Station{Kind: kind}}
}

func plainItem(kind string) services.Item { return services.Item{Kind: kind, ID: kind} }

func section(id string, items []services.Item, kind ...string) services.Section {
	k := services.SectionShelf
	if len(kind) > 0 {
		k = kind[0]
	}
	return services.Section{ID: id, Kind: k, Title: id, Items: items}
}

func sectionIDs(ss []services.Section) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.ID
	}
	return out
}

func TestSectionPlaceByKindElseByMostItems(t *testing.T) {
	for _, c := range []struct {
		name string
		sec  services.Section
		want place
	}{
		{"local broadcasters", section("Local", []services.Item{stationItem("broadcast")}, services.SectionStations), placeLive},
		{"live", section("On air", []services.Item{stationItem("live")}, services.SectionLive), placeLive},
		{"links", section("More", []services.Item{plainItem(services.ItemRoom)}, services.SectionLinks), placeBrowse},
		{"bricks", section("Bricks", []services.Item{plainItem(services.ItemRoom)}, services.SectionBricks), placeBrowse},
		{"genre stations", section("Genres", []services.Item{stationItem("tracks"), stationItem("tracks")}), placeStations},
		{"episodes beat an album 1-1 by first item", section("Episodes", []services.Item{stationItem("episode"), plainItem(services.ItemAlbum)}), placeShows},
		{"curators", section("Hosts", []services.Item{plainItem(services.ItemCurator)}), placeShows},
		{"videos", section("Interviews", []services.Item{plainItem(services.ItemMusicVideo)}), placeShows},
		// Apple's featured row of live stations stays with the live stations.
		{"hero of live stations", section("", []services.Item{stationItem("live"), stationItem("live")}, services.SectionHero), placeLive},
		// A tie goes to the first item's place.
		{"tie", section("Mixed", []services.Item{plainItem(services.ItemGrouping), stationItem("tracks")}), placeBrowse},
		{"nothing", section("Empty", nil), placeShows},
		{"station with no DTO", section("Odd", []services.Item{{Kind: services.ItemStation, ID: "x"}}), placeStations},
		{"charts and rooms browse", section("Charts", []services.Item{plainItem(services.ItemCharts), plainItem(services.ItemMultiroom)}), placeBrowse},
	} {
		if got := sectionPlace(c.sec); got != c.want {
			t.Errorf("%s: placed %q, want %q", c.name, got, c.want)
		}
	}
}

func TestItemPlace(t *testing.T) {
	for kind, want := range map[string]place{
		"live": placeLive, "broadcast": placeLive, "episode": placeShows, "tracks": placeStations,
	} {
		if got := itemPlace(stationItem(kind)); got != want {
			t.Errorf("station %s: %q, want %q", kind, got, want)
		}
	}
	for _, kind := range []string{services.ItemRoom, services.ItemMultiroom, services.ItemGrouping, services.ItemCharts, services.ItemLink} {
		if got := itemPlace(plainItem(kind)); got != placeBrowse {
			t.Errorf("%s: %q, want browse", kind, got)
		}
	}
	for _, kind := range []string{services.ItemCurator, services.ItemMusicVideo, services.ItemAlbum, services.ItemPlaylist} {
		if got := itemPlace(plainItem(kind)); got != placeShows {
			t.Errorf("%s: %q, want shows", kind, got)
		}
	}
}

func TestPlaceSectionsOrdersTheBandsAndKeepsApplesOrderInsideEach(t *testing.T) {
	sections := []services.Section{
		section("Latest Episodes", []services.Item{stationItem("episode")}),
		section("Local Broadcasters", []services.Item{stationItem("broadcast")}, services.SectionStations),
		section("Top Stations", []services.Item{stationItem("tracks")}),
		section("All Shows", []services.Item{plainItem(services.ItemCurator)}),
		section("More to Explore", []services.Item{plainItem(services.ItemRoom)}, services.SectionLinks),
		section("Stations by Genre", []services.Item{stationItem("tracks")}),
	}
	live, bands := placeSections(sections)
	if got := sectionIDs(live); !slices.Equal(got, []string{"Local Broadcasters"}) {
		t.Errorf("live = %q", got)
	}
	want := map[bandID][]string{
		bandStations: {"Top Stations", "Stations by Genre"},
		bandShows:    {"Latest Episodes", "All Shows"},
		bandBrowse:   {"More to Explore"},
	}
	if len(bands) != 3 {
		t.Fatalf("%d bands", len(bands))
	}
	for i, id := range []bandID{bandStations, bandShows, bandBrowse} {
		if bands[i].id != id || !slices.Equal(sectionIDs(bands[i].sections), want[id]) {
			t.Errorf("band %d = %s %q, want %s %q", i, bands[i].id, sectionIDs(bands[i].sections), id, want[id])
		}
	}
}

func TestPlaceSectionsKeepsEveryBandEmptyOrNot(t *testing.T) {
	live, bands := placeSections(nil)
	if len(live) != 0 || len(bands) != 3 {
		t.Fatalf("live %v, bands %v", live, bands)
	}
	for i, id := range []bandID{bandStations, bandShows, bandBrowse} {
		if bands[i].id != id || len(bands[i].sections) != 0 {
			t.Errorf("band %d = %+v", i, bands[i])
		}
	}
}

func TestPlaceSectionsKeepsAParagraphWithTheSectionItIntroduces(t *testing.T) {
	intro := section("intro", nil, services.SectionText)
	outro := section("outro", nil, services.SectionText)
	_, bands := placeSections([]services.Section{
		section("Episodes", []services.Item{stationItem("episode")}),
		intro,
		section("Genres", []services.Item{stationItem("tracks")}),
		outro,
	})
	if got := sectionIDs(bands[0].sections); !slices.Equal(got, []string{"intro", "Genres", "outro"}) {
		t.Errorf("stations band = %q", got)
	}
	if got := sectionIDs(bands[1].sections); !slices.Equal(got, []string{"Episodes"}) {
		t.Errorf("shows band = %q", got)
	}
}

func TestPlaceSectionsPutsParagraphsOnlyAmongShows(t *testing.T) {
	// Nothing but a paragraph: it has no section to go with.
	_, bands := placeSections([]services.Section{section("alone", nil, services.SectionText)})
	if got := sectionIDs(bands[1].sections); !slices.Equal(got, []string{"alone"}) {
		t.Errorf("shows band = %q", got)
	}
}

func TestBandTitles(t *testing.T) {
	for id, want := range map[bandID]string{bandStations: "Stations", bandShows: "Shows and episodes", bandBrowse: "Browse"} {
		if got := bandTitle(id); got != want {
			t.Errorf("bandTitle(%s) = %q, want %q", id, got, want)
		}
	}
}
