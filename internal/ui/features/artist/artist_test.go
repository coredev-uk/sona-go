package artist

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

func plain(id string) services.Album { return services.Album{ID: id, Name: id} }

func albums(ids ...string) []services.Album {
	out := make([]services.Album, len(ids))
	for i, id := range ids {
		out[i] = plain(id)
	}
	return out
}

func shelf(view string, more bool, ids ...string) services.ArtistShelf {
	return services.ArtistShelf{View: view, Kind: services.ShelfAlbums, More: more, Albums: albums(ids...)}
}

func page(latest string, shelves ...services.ArtistShelf) *services.CatalogArtist {
	p := &services.CatalogArtist{Shelves: shelves}
	if latest != "" {
		a := plain(latest)
		p.LatestRelease = &a
	}
	return p
}

func TestReleaseTitleMovesTheSuffixOut(t *testing.T) {
	for _, tc := range []struct{ in, title, kind string }{
		{"Daydreaming - Single", "Daydreaming", "single"},
		{"Com Lag: 2+2=5 - EP", "Com Lag: 2+2=5", "ep"},
		// Other titles stay whole, dashes included.
		{"The King of Limbs - Live from the Basement", "The King of Limbs - Live from the Basement", ""},
		{"Single", "Single", ""},
		// Only at the end.
		{"Single - Remixes", "Single - Remixes", ""},
	} {
		if title, kind := releaseTitle(tc.in); title != tc.title || kind != tc.kind {
			t.Errorf("releaseTitle(%q) = %q, %q; want %q, %q", tc.in, title, kind, tc.title, tc.kind)
		}
	}
}

func TestReleaseCountCountsEachReleaseOnce(t *testing.T) {
	p := page("s",
		shelf("full-albums", false, "a", "b"),
		shelf("singles", false, "s"),
		shelf("featured-albums", false, "a"),
		shelf("appears-on-albums", false, "x", "y"),
	)
	if n, ok := releaseCount(p); !ok || n != 3 {
		t.Errorf("releaseCount = %d, %v; want 3, true", n, ok)
	}
}

func TestReleaseCountIsUnknownWhileOwnListsHaveMore(t *testing.T) {
	if _, ok := releaseCount(page("", shelf("full-albums", false, "a"), shelf("singles", true, "s"))); ok {
		t.Error("count known while singles has more to fetch")
	}
	// Others' albums with more don't make the artist's own count unknown.
	if n, ok := releaseCount(page("", shelf("singles", false, "s"), shelf("appears-on-albums", true, "x"))); !ok || n != 1 {
		t.Errorf("releaseCount = %d, %v; want 1, true", n, ok)
	}
}

func TestShelvesForFollowTheViewsAskedFor(t *testing.T) {
	p := page("", shelf("compilation-albums", false, "c"), shelf("singles", false, "s"))
	var got []string
	for _, sh := range shelvesFor(p, []string{"singles", "live-albums", "compilation-albums"}) {
		got = append(got, sh.View)
	}
	if want := []string{"singles", "compilation-albums"}; !slices.Equal(got, want) {
		t.Errorf("shelves %q, want %q", got, want)
	}
}

func TestVideoShelfTakesTheFirstOfTopAndFull(t *testing.T) {
	// Apple sends the full list first; the top pick still wins as it is first in the views.
	p := page("", services.ArtistShelf{View: "music-videos", Kind: services.ShelfVideos}, services.ArtistShelf{View: "top-music-videos", Kind: services.ShelfVideos})
	if sh, ok := videoShelf(p); !ok || sh.View != "top-music-videos" {
		t.Errorf("video shelf %q, %v", sh.View, ok)
	}
	if _, ok := videoShelf(page("")); ok {
		t.Error("a page with no videos has a video shelf")
	}
}

func TestEssentialsAreTheFeaturedAlbums(t *testing.T) {
	p := page("", shelf("full-albums", false, "a"), shelf("featured-albums", false, "e"))
	if sh, ok := essentials(p); !ok || sh.Albums[0].ID != "e" {
		t.Errorf("essentials = %+v, %v", sh, ok)
	}
	if _, ok := essentials(page("", shelf("full-albums", false, "a"))); ok {
		t.Error("essentials without featured-albums")
	}
}

func TestArtistLineIsKindGenreOriginCount(t *testing.T) {
	for _, tc := range []struct {
		got, want string
	}{
		{artistLine("Alternative", "Abingdon, Oxfordshire, England", 0, ""), "Artist · Alternative · Abingdon, Oxfordshire, England"},
		{artistLine("", "", 1, ""), "Artist · 1 release"},
		{artistLine("", "", 5, ""), "Artist · 5 releases"},
		{artistLine("Pop", "", 0, "2 albums, 9 songs in your library"), "Artist · Pop · 2 albums, 9 songs in your library"},
	} {
		if tc.got != tc.want {
			t.Errorf("line %q, want %q", tc.got, tc.want)
		}
	}
}

func TestBioLeadIsTheFirstParagraph(t *testing.T) {
	if got := bioLead("First line.\nStill first.\n\nSecond."); got != "First line.\nStill first." {
		t.Errorf("lead %q", got)
	}
	if got := bioLead(""); got != "" {
		t.Errorf("lead of nothing %q", got)
	}
	if got := bioLead("  One.  \n\n\n\nTwo."); got != "One." {
		t.Errorf("lead %q", got)
	}
}

func TestLayoutPutsTheSlipBesideFrom1040TheLeadBesideFrom800AndShrinksThePortraitUnder720(t *testing.T) {
	for _, tc := range []struct {
		width    float32
		wide     bool
		beside   bool
		portrait float32
		slip     float32
	}{
		{1144, true, true, 200, 152},
		{1040, true, true, 200, 152},
		{900, true, false, 176, 88},
		{800, true, false, 176, 88},
		{660, false, false, 112, 64},
		// Before the first layout the page is measured as 0: the full-size portrait, no jump.
		{0, false, false, 200, 88},
	} {
		if l := layoutFor(tc.width); l.wide != tc.wide || l.slipBeside != tc.beside || l.portrait != tc.portrait || l.slip != tc.slip {
			t.Errorf("layoutFor(%v) = %+v", tc.width, l)
		}
	}
}

func TestTimelineGroupsReleasesByYearNewestFirstOnceEach(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	al := func(id, name, date string) services.Album {
		return services.Album{ID: id, Name: name, ReleaseDate: date}
	}
	shelves := []services.ArtistShelf{
		{View: services.ViewFullAlbums, Albums: []services.Album{al("a", "A", "2007-10-10"), al("b", "B", "2001-01-01")}},
		{View: services.ViewSingles, More: true, Albums: []services.Album{al("s", "S - Single", "2007-02-01"), al("a", "A", "2007-10-10"), al("e", "E - EP", "")}},
		{View: services.ViewLiveAlbums, Albums: []services.Album{al("l", "L", "2001-06-01")}},
	}
	years, more := buildTimeline(shelves, now)
	var got []string
	for _, y := range years {
		for i, a := range y.albums {
			got = append(got, y.label+":"+a.ID+":"+y.lines[i])
		}
	}
	want := []string{"2007:a:Album", "2007:s:Single", "2001:l:Live", "2001:b:Album", ":e:EP"}
	if !slices.Equal(got, want) {
		t.Errorf("timeline %q, want %q", got, want)
	}
	if len(more) != 1 || more[0].view != services.ViewSingles {
		t.Errorf("more %v, want the singles view only", more)
	}
}

func TestTimelineKeepsTheNewestAndOffersTheShelfItCutShort(t *testing.T) {
	var albums []services.Album
	for i := range timelineMax + 3 {
		albums = append(albums, services.Album{ID: fmt.Sprint(i), Name: "R", ReleaseDate: fmt.Sprintf("%d-01-01", 2000+i)})
	}
	years, more := buildTimeline([]services.ArtistShelf{{View: services.ViewFullAlbums, Albums: albums}}, time.Now())
	n := 0
	for _, y := range years {
		n += len(y.albums)
	}
	if n != timelineMax || years[0].label != fmt.Sprint(2000+timelineMax+2) {
		t.Errorf("%d releases, first year %q", n, years[0].label)
	}
	if len(more) != 1 {
		t.Errorf("more %v, want the albums view", more)
	}
}

func TestCatalogueListsSitSideBySideAsFarAsTheyFit(t *testing.T) {
	for _, tc := range []struct {
		width float32
		lists int
		want  int
	}{{1144, 3, 3}, {660, 3, 2}, {200, 3, 1}, {1144, 1, 1}} {
		if got := catalogueColumns(tc.width, tc.lists); got != tc.want {
			t.Errorf("catalogueColumns(%v, %d) = %d, want %d", tc.width, tc.lists, got, tc.want)
		}
	}
}

func TestViewTitlesNameTheArtist(t *testing.T) {
	for view, want := range map[string]string{
		"top-songs":          "Top songs by Radiohead",
		"featured-albums":    "Essential albums by Radiohead",
		"full-albums":        "Albums by Radiohead",
		"singles":            "Singles and EPs featuring Radiohead",
		"live-albums":        "Live albums by Radiohead",
		"compilation-albums": "Compilations featuring Radiohead",
		"appears-on-albums":  "Albums featuring Radiohead",
		"playlists":          "Playlists featuring Radiohead",
		"music-videos":       "Music videos by Radiohead",
		"top-music-videos":   "Music videos by Radiohead",
		"similar-artists":    "Similar artists to Radiohead",
	} {
		if got, ok := viewTitle(view, "Radiohead"); !ok || got != want {
			t.Errorf("viewTitle(%q) = %q, %v; want %q", view, got, ok, want)
		}
	}
	if isArtistView("nonsense") || !isArtistView("singles") {
		t.Error("isArtistView")
	}
}
