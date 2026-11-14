package services

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// Apple's artists relationship, as the live catalogue answers it (checked
// against "STAY" by The Kid LAROI & Justin Bieber): the artists in the order
// artistName joins them, named only when the request included them.
func TestAmSongKeepsEveryArtistInApplesOrder(t *testing.T) {
	m := mapper{}
	named := applemusic.Song{}
	named.ID, named.Type = "1574968888", "songs"
	named.Attributes.Name, named.Attributes.ArtistName = "STAY", "The Kid LAROI & Justin Bieber"
	laroi, bieber := applemusic.Artist{}, applemusic.Artist{}
	laroi.ID, laroi.Type, laroi.Attributes.Name = "1435848034", "artists", "The Kid LAROI"
	bieber.ID, bieber.Type, bieber.Attributes.Name = "320569549", "artists", "Justin Bieber"
	named.Relationships.Artists.Data = []applemusic.Artist{laroi, bieber}

	got := m.amSong(named)
	if got.ArtistID != "1435848034" || len(got.Artists) != 2 {
		t.Fatalf("song %+v, want the first artist's ID and both artists", got)
	}
	if a := got.Artists; a[0].ID != "1435848034" || a[0].Name != "The Kid LAROI" || a[1].ID != "320569549" || a[1].Name != "Justin Bieber" ||
		a[0].Source != SourceCatalog || a[1].CatalogID != "320569549" {
		t.Errorf("artists %+v", a)
	}

	// Without include=artists Apple sends the IDs alone: the first stays the
	// song's artist, and no artist is offered by an empty name.
	bare := named
	bare.Relationships.Artists.Data = []applemusic.Artist{{Resource: applemusic.Resource[applemusic.ArtistAttributes]{ID: "1435848034", Type: "artists"}}, {Resource: applemusic.Resource[applemusic.ArtistAttributes]{ID: "320569549", Type: "artists"}}}
	if got := m.amSong(bare); got.ArtistID != "1435848034" || got.Artists != nil {
		t.Errorf("unnamed artists: %+v", got)
	}
	// A library song's artists are library artists, which a catalogue artist
	// page can't open.
	lib := named
	lib.Type = "library-songs"
	l1 := laroi
	l1.ID, l1.Type = "r.1", "library-artists"
	lib.Relationships.Artists.Data = []applemusic.Artist{l1}
	if got := m.amSong(lib); got.ArtistID != "r.1" || got.Artists != nil {
		t.Errorf("library artists: %+v", got)
	}

	// An album credited to several artists keeps them; a compilation credited
	// to none ("Various Artists") has none.
	var split, various applemusic.Album
	split.ID, split.Type = "1440848092", "albums"
	split.Attributes.Name, split.Attributes.ArtistName = "Watch the Throne", "JAŸ-Z & Kanye West"
	split.Relationships.Artists.Data = []applemusic.Artist{laroi, bieber}
	various.ID, various.Type = "1138068544", "albums"
	various.Attributes.ArtistName = "Various Artists"
	if a := m.amAlbum(split); a.ArtistID != "1435848034" || len(a.Artists) != 2 {
		t.Errorf("split album %+v", a)
	}
	if a := m.amAlbum(various); a.ArtistID != "" || a.Artists != nil {
		t.Errorf("compilation %+v", a)
	}
}

// artistsSongs is Apple's song endpoint naming each song's artists when asked
// to (include=artists), "s1" and "s3" being collaborations.
type artistsSongs struct {
	mu       sync.Mutex
	requests []string // the query of each request
}

func (f *artistsSongs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.RawQuery)
	f.mu.Unlock()
	named := r.URL.Query().Get("include") == "artists"
	var data []string
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		artists := fmt.Sprintf(`{"id":"art.%[1]s.1","type":"artists"%[2]s}`, id, map[bool]string{true: `,"attributes":{"name":"First of ` + id + `"}`}[named])
		if id == "s1" || id == "s3" {
			artists += fmt.Sprintf(`,{"id":"art.%[1]s.2","type":"artists"%[2]s}`, id, map[bool]string{true: `,"attributes":{"name":"Second of ` + id + `"}`}[named])
		}
		data = append(data, fmt.Sprintf(`{"id":%[1]q,"type":"songs","attributes":{"name":"Song %[1]s","artistName":"Artist"},"relationships":{"artists":{"data":[%[2]s]}}}`, id, artists))
	}
	_, _ = fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(data, ","))
}

// A library song knows its library artist only. Its catalogue copy's artists
// come from one batched lookup of the catalogue IDs (the playback batching:
// names asked for with include=artists), nothing per song, and a song asked
// twice is read from the catalogue cache.
func TestSongArtistsLooksUpCatalogueCopiesInOneBatch(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	ce.signIn(t)
	f := &artistsSongs{}
	ce.songs = f.ServeHTTP
	cs := newCatalogService(ce.e)

	got, err := cs.SongArtists(t.Context(), []string{"s1", "s2", "s3", "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("answered %d songs: %+v", len(got), got)
	}
	var names []string
	for _, a := range got["s3"] {
		names = append(names, a.Name)
	}
	if !slices.Equal(names, []string{"First of s3", "Second of s3"}) || len(got["s2"]) != 1 || got["s1"][1].ID != "art.s1.2" {
		t.Errorf("artists %+v, want every one in Apple's order", got)
	}
	f.mu.Lock()
	reqs := slices.Clone(f.requests)
	f.mu.Unlock()
	if len(reqs) != 1 || !strings.Contains(reqs[0], "include=artists") {
		t.Fatalf("requests %q, want one batched request including the artists", reqs)
	}
	if _, err := cs.SongArtists(t.Context(), []string{"s1", "s3"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	n := len(f.requests)
	f.mu.Unlock()
	if n != 1 {
		t.Errorf("%d requests after asking again: the catalogue cache was not used", n)
	}
}

// The queue slot of a song fetched from the catalogue carries every artist, so
// the player can offer them; the first stays its ArtistID.
func TestPlayedCatalogueSongCarriesItsArtists(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	ce.signIn(t)
	f := &artistsSongs{}
	ce.songs = f.ServeHTTP
	refs := []TrackRef{{ID: "s1", Source: SourceCatalog}, {ID: "s2", Source: SourceCatalog}}
	if err := ce.play.PlayItems(t.Context(), refs, 0, PlayFrom{ID: "Top songs"}); err != nil {
		t.Fatal(err)
	}
	q := ce.play.Queue()
	if len(q.Items) != 2 {
		t.Fatalf("queue %+v", q.Items)
	}
	one, two := q.Items[0], q.Items[1]
	if one.ArtistID != "art.s1.1" || len(one.Artists) != 2 || one.Artists[1].Name != "Second of s1" {
		t.Errorf("the collaboration: %+v", one)
	}
	if two.ArtistID != "art.s2.1" || len(two.Artists) != 1 {
		t.Errorf("the solo song: %+v", two)
	}
}
