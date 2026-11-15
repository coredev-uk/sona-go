package services

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

// gtaAlbum is the album page of "Grand Theft Auto VI: The Album" (gb, album
// 6812476961) as Apple answered it on 2026-11-14, six days before its
// release, trimmed to what Sona reads (testdata/albums/gta6-prerelease.json):
// 34 tracks, ten of them out. Eight carry playParams and a releaseDate; two
// (6 "Flower In A Hurricane" and 22 "Blood In Your Eyes") carry playParams
// and no releaseDate; the other 24 are "Track N" with neither.
func gtaAlbum(t *testing.T) applemusic.Album {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "albums", "gta6-prerelease.json"))
	if err != nil {
		t.Fatal(err)
	}
	return decode[applemusic.Page[applemusic.Album]](t, string(b)).Data[0]
}

// gtaOut are the track numbers that are out: the songs with playParams.
var gtaOut = []int{2, 4, 5, 6, 8, 10, 11, 19, 22, 32}

func TestGTAAlbumFixtureSongsFollowThemselves(t *testing.T) {
	now := localDay(t, "2026-11-14").Add(12 * time.Hour)
	s := detailsService(t)
	s.m = mapper{now: func() time.Time { return now }}
	p := s.albumPage(t.Context(), gtaAlbum(t))

	if !p.Album.Prerelease || p.Album.ReleaseDate != "2026-11-20" || p.Album.ReleaseAt != localDay(t, "2026-11-20").UnixMilli() {
		t.Fatalf("album = %+v, want a pre-release out on 2026-11-20", p.Album)
	}
	if len(p.Tracks) != 34 {
		t.Fatalf("%d tracks, want 34", len(p.Tracks))
	}
	var out []int
	for _, tr := range p.Tracks {
		if tr.Playable {
			out = append(out, tr.TrackNumber)
		}
	}
	if !slices.Equal(out, gtaOut) {
		t.Fatalf("playable tracks = %v, want %v", out, gtaOut)
	}
	flower := p.Tracks[5]
	if flower.Name != "Flower In A Hurricane" || flower.ReleaseDate != "" || !flower.Playable {
		t.Fatalf("Flower In A Hurricane = %+v, want playable (playParams, no releaseDate)", flower)
	}
	if p.Tracks[0].Playable {
		t.Fatalf("Track 1 = %+v, want not out", p.Tracks[0])
	}

	// From midnight on the release day the album is out; the songs Apple
	// hasn't published yet stay unplayable (no playParams).
	now = localDay(t, "2026-11-20")
	p = s.albumPage(t.Context(), gtaAlbum(t))
	if p.Album.Prerelease {
		t.Fatalf("album on release day = %+v, want out", p.Album)
	}
	out = out[:0]
	for _, tr := range p.Tracks {
		if tr.Playable {
			out = append(out, tr.TrackNumber)
		}
	}
	if !slices.Equal(out, gtaOut) {
		t.Fatalf("playable tracks on release day = %v, want %v", out, gtaOut)
	}
}

// Playing the album queues the songs that are out and refuses starting on one
// that is not, for the catalog copies the album page stores.
func TestGTAAlbumFixturePlaysTheSongsOut(t *testing.T) {
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	a := gtaAlbum(t)
	if _, err := d.Q.UpsertCatalogAlbum(ctx, dbq.UpsertCatalogAlbumParams{
		CatalogID: db.NullString(a.ID), Name: a.Attributes.Name, ReleaseDate: a.Attributes.ReleaseDate, UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	var refs []TrackRef
	for _, tr := range a.Relationships.Tracks.Data {
		tr.Relationships.Albums.Data = []applemusic.Album{{ID: a.ID}}
		if _, err := catalog.StoreSong(ctx, d.Q, tr, 1); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, TrackRef{ID: tr.ID, Source: SourceCatalog})
	}
	now := localDay(t, "2026-11-14")
	e := &Env{Deps: Deps{DB: d, Logger: discard(), Now: func() time.Time { return now }}}
	s := &PlaybackService{o: e, m: mapper{now: e.Now}, meta: map[string]trackMeta{}}

	items, at, err := s.resolve(ctx, refs, 5) // Flower In A Hurricane
	if err != nil || len(items) != len(gtaOut) || items[at].ID != "6812477273" {
		t.Fatalf("resolve from Flower In A Hurricane = %d items at %d, %v; want the %d songs out, starting on it", len(items), at, err, len(gtaOut))
	}
	if _, _, err := s.resolve(ctx, refs, 0); CodeOf(err) != CodeNotReleased {
		t.Fatalf("resolve from Track 1: %v, want not_released", err)
	}
}

// The same album pre-added to the library: Apple lists its library songs with
// the catalog's playParams and dates, so the single without a date plays, and
// the page the library shows agrees with the song list.
func TestGTAAlbumPreAddedSongsFollowThemselves(t *testing.T) {
	d := preAddedStore(t) // the pre-added album, out on 2026-11-20
	now := localDay(t, "2026-11-14")
	lib := library.New(d, nil)
	lib.SetClock(func() time.Time { return now })
	m := mapper{now: func() time.Time { return now }}
	flower, err := lib.Song(t.Context(), "i.single") // playParams, no date
	if err != nil {
		t.Fatal(err)
	}
	track1, err := lib.Song(t.Context(), "i.later") // no playParams, no date
	if err != nil {
		t.Fatal(err)
	}
	if !m.libSong(flower).Playable {
		t.Fatal("Flower In A Hurricane (playParams, no date) is not playable in the library")
	}
	if m.libSong(track1).Playable {
		t.Fatal("Track 1 (no playParams, album ahead) is playable in the library")
	}
}
