package library

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

func artistsTestEntry(name string) services.ArtistEntry {
	return services.ArtistEntry{ID: "r." + name, Name: name, Albums: 1, Songs: 10}
}

func artistsTestEntries(names ...string) []services.ArtistEntry {
	out := make([]services.ArtistEntry, len(names))
	for i, n := range names {
		out[i] = artistsTestEntry(n)
	}
	return out
}

func artistsTestNames(list []indexedArtist) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.entry.Name
	}
	return out
}

func artistsTestLibrary() []indexedArtist {
	return indexArtists(artistsTestEntries(
		"Zara Larsson", "björk", "Beyoncé", "The Beatles", "Édith Piaf", "Eric Clapton", "AC/DC", "2Pac",
		"坂本龍一", "Sigur Rós", "Mötley Crüe", "Röyksopp", "Radiohead", "Band 10", "Band 9",
	))
}

func TestIndexArtistsOrdersAToZThenOthers(t *testing.T) {
	want := []string{
		"AC/DC", "Band 9", "Band 10", "Beyoncé", "björk", "Édith Piaf", "Eric Clapton", "Mötley Crüe",
		"Radiohead", "Röyksopp", "Sigur Rós", "The Beatles", "Zara Larsson", "2Pac", "坂本龍一",
	}
	if got := artistsTestNames(artistsTestLibrary()); !slices.Equal(got, want) {
		t.Errorf("order %q, want %q", got, want)
	}
}

func TestIndexArtistsFilesAccentedNamesUnderTheirBaseLetter(t *testing.T) {
	letters := map[string]string{}
	for _, a := range artistsTestLibrary() {
		letters[a.entry.Name] = a.letter
	}
	for name, want := range map[string]string{"Édith Piaf": "E", "Röyksopp": "R", "björk": "B", "2Pac": "#", "坂本龍一": "#"} {
		if letters[name] != want {
			t.Errorf("%s is filed under %q, want %q", name, letters[name], want)
		}
	}
}

func TestFilterArtists(t *testing.T) {
	lib := artistsTestLibrary()
	names := func(q string) []string { return artistsTestNames(filterArtists(lib, q)) }

	if len(filterArtists(lib, "")) != len(lib) || len(filterArtists(lib, "   ")) != len(lib) {
		t.Error("an empty or blank query drops artists")
	}
	for q, want := range map[string][]string{
		"RADIO":       {"Radiohead"},
		"bjork":       {"björk"},
		"edith":       {"Édith Piaf"},
		"Édith":       {"Édith Piaf"},
		"sigur ros":   {"Sigur Rós"},
		"motley crue": {"Mötley Crüe"},
		// A name without its spaces and punctuation.
		"acdc":     {"AC/DC"},
		"sigurros": {"Sigur Rós"},
		"坂本":       {"坂本龍一"},
		"zzzz":     {},
	} {
		if got := names(q); !slices.Equal(got, want) {
			t.Errorf("filter %q = %q, want %q", q, got, want)
		}
	}
	if !slices.Contains(names("rós"), "Sigur Rós") {
		t.Error("a typed accent finds the name")
	}
}

func TestFilterArtistsRanksStartsThenWordsThenTheRest(t *testing.T) {
	list := indexArtists(artistsTestEntries("Sunset Rubdown", "Brian Eno", "Enobarbus", "Eno", "Beno"))
	want := []string{"Eno", "Enobarbus", "Brian Eno", "Beno"}
	if got := artistsTestNames(filterArtists(list, "eno")); !slices.Equal(got, want) {
		t.Errorf("ranked %q, want %q", got, want)
	}
}

func TestListRows(t *testing.T) {
	lib := artistsTestLibrary()
	rows := listRows(lib, true)
	var heads []string
	artists := 0
	for _, r := range rows {
		if r.artist == nil {
			heads = append(heads, r.letter)
		} else {
			artists++
		}
	}
	if want := []string{"A", "B", "E", "M", "R", "S", "T", "Z", "#"}; !slices.Equal(heads, want) {
		t.Errorf("headings %q, want %q", heads, want)
	}
	if artists != len(lib) {
		t.Errorf("%d artist rows of %d", artists, len(lib))
	}

	// Filtering is flat: each artist with its place in the results.
	flat := listRows(filterArtists(lib, "r"), false)
	for i, r := range flat {
		if r.artist == nil || r.position != i {
			t.Fatalf("row %d is %+v, want an artist at position %d", i, r, i)
		}
	}
	tops := rowTops(rows)
	if want := float32(len(heads)*letterRowHeight + len(lib)*artistRowHeight); tops[len(rows)] != want || tops[1] != letterRowHeight {
		t.Errorf("tops end at %v (want %v), the first artist at %v", tops[len(rows)], want, tops[1])
	}
}

func TestLibraryLine(t *testing.T) {
	for _, c := range []struct {
		albums, songs int
		want          string
	}{
		{12, 148, "12 albums, 148 songs"},
		{1, 1, "1 album, 1 song"},
		{3, 0, "3 albums"},
		{0, 3, "3 songs"},
		{0, 0, ""},
	} {
		if got := libraryLine(c.albums, c.songs); got != c.want {
			t.Errorf("libraryLine(%d, %d) = %q, want %q", c.albums, c.songs, got, c.want)
		}
	}
}

// A release tile has the album's motion artwork and, for an album not out
// yet, its release badge (v0.2.0's albumTile).
func TestArtistsReleaseTileHasMotionAndTheReleaseBadge(t *testing.T) {
	soon := time.Now().Add(30 * 24 * time.Hour)
	a := services.Album{Name: "Later", Motion: "motion.mp4", Prerelease: true, ReleaseAt: soon.UnixMilli(), ReleaseDate: soon.Format("2006-01-02")}
	tile := artistsReleaseTile(kit.CoverItem{}, a)
	if tile.Motion != "motion.mp4" || !strings.HasPrefix(tile.Badge, "Out ") {
		t.Errorf("motion %q badge %q", tile.Motion, tile.Badge)
	}
	out := artistsReleaseTile(kit.CoverItem{}, services.Album{Name: "Now", ReleaseDate: "2020-01-01"})
	if out.Badge != "" {
		t.Errorf("an album that is out has the badge %q", out.Badge)
	}
}
