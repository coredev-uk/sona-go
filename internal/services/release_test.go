package services

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// A pre-release album as Apple lists it (trimmed from a live answer): no
// isPrerelease on the album page, the release date ahead, the singles
// already out dated and playable, the rest without play parameters.
const prereleaseAlbum = `{"id":"7","type":"albums","attributes":{"name":"Soon","artistName":"Band","releaseDate":"2026-11-19","playParams":{"id":"7","kind":"album"}},
	"relationships":{"tracks":{"data":[
		{"id":"1","type":"songs","attributes":{"name":"Track 1"}},
		{"id":"2","type":"songs","attributes":{"name":"Single","releaseDate":"2026-09-17","playParams":{"id":"2","kind":"song"}}},
		{"id":"3","type":"songs","attributes":{"name":"Undated","playParams":{"id":"3","kind":"song"}}}]}}}`

func localDay(t *testing.T, date string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation(time.DateOnly, date, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPrereleaseAlbumMapping(t *testing.T) {
	release := localDay(t, "2026-11-19")
	now := release.Add(-18 * 24 * time.Hour)
	s := detailsService(t)
	s.m = mapper{now: func() time.Time { return now }}
	p := s.albumPage(t.Context(), decode[applemusic.Album](t, prereleaseAlbum))
	if !p.Album.Prerelease || p.Album.ReleaseAt != release.UnixMilli() {
		t.Fatalf("album = %+v, want a pre-release out at %d", p.Album, release.UnixMilli())
	}
	if got := []bool{p.Tracks[0].Playable, p.Tracks[1].Playable, p.Tracks[2].Playable}; got[0] || !got[1] || got[2] {
		t.Fatalf("playable = %v, want only the single already out", got)
	}

	// From midnight on the release day it is out.
	now = release
	p = s.albumPage(t.Context(), decode[applemusic.Album](t, prereleaseAlbum))
	if p.Album.Prerelease || p.Album.ReleaseAt != release.UnixMilli() || !p.Tracks[2].Playable {
		t.Fatalf("released album = %+v, tracks %+v", p.Album, p.Tracks)
	}

	// Without a full date Apple's flag decides; a song dated ahead is not
	// playable anywhere.
	yearOnly := s.m.amAlbum(decode[applemusic.Album](t, `{"id":"8","type":"albums","attributes":{"name":"Later","releaseDate":"2027","isPrerelease":true}}`))
	if !yearOnly.Prerelease || yearOnly.ReleaseAt != 0 {
		t.Fatalf("year-only pre-release = %+v", yearOnly)
	}
	future := s.m.amSong(decode[applemusic.Song](t, `{"id":"9","type":"songs","attributes":{"name":"Next","releaseDate":"2026-12-01","playParams":{"id":"9","kind":"song"}}}`))
	if future.Playable {
		t.Fatal("a song dated ahead is playable")
	}
}

func TestPlaybackRefusesUnreleasedSongs(t *testing.T) {
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := d.Q.UpsertCatalogAlbum(ctx, dbq.UpsertCatalogAlbumParams{CatalogID: db.NullString("7"), Name: "Soon", ReleaseDate: "2026-11-19", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	for _, tr := range []struct{ id, date string }{{"1", ""}, {"2", "2026-09-17"}, {"4", "2026-12-01"}} {
		if _, err := d.Q.UpsertCatalogTrack(ctx, dbq.UpsertCatalogTrackParams{
			CatalogID: db.NullString(tr.id), AlbumCatalogID: db.NullString("7"), Name: "Track " + tr.id, ReleaseDate: tr.date, UpdatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	now := localDay(t, "2026-11-01")
	e := &Env{Deps: Deps{DB: d, Logger: discard(), Now: func() time.Time { return now }}}
	s := &PlaybackService{o: e, m: mapper{now: e.Now}, meta: map[string]trackMeta{}}
	refs := []TrackRef{{ID: "1", Source: SourceCatalog}, {ID: "2", Source: SourceCatalog}, {ID: "4", Source: SourceCatalog}}

	for _, start := range []int{0, 2} {
		if _, _, err := s.resolve(ctx, refs, start); !errors.Is(err, ErrNotReleased) || ErrorCode(err) != CodeNotReleased {
			t.Fatalf("start %d: err = %v, want not_released", start, err)
		}
	}
	items, i, err := s.resolve(ctx, refs, 1)
	if err != nil || len(items) != 1 || items[0].ID != "2" || i != 0 {
		t.Fatalf("resolve from the single = %+v, %d, %v", items, i, err)
	}

	// Release day: the album's songs play; the one dated later still waits.
	now = localDay(t, "2026-11-19")
	items, _, err = s.resolve(ctx, refs, 0)
	if err != nil || len(items) != 2 || items[0].ID != "1" || items[1].ID != "2" {
		t.Fatalf("resolve on release day = %+v, %v", items, err)
	}
}

// Playing the library song list applies the same rule: a song without a
// date of its own goes by its album's.
func TestPlayLibrarySkipsUnreleasedSongs(t *testing.T) {
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	for _, a := range []struct{ id, date string }{{"l.soon", "2026-11-19"}, {"l.old", "2020-01-01"}} {
		if _, err := d.Q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{LibraryID: db.NullString(a.id), Name: a.id, ReleaseDate: a.date, UpdatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	// In title order: a single already out, a song of the pre-release
	// album without a date, a song of an old album without one, one of
	// no stored album, and one dated ahead itself.
	for _, tr := range []struct{ id, name, album, date string }{
		{"i.1", "A single", "l.soon", "2026-09-17"},
		{"i.2", "B undated", "l.soon", ""},
		{"i.3", "C old", "l.old", ""},
		{"i.4", "D loose", "", ""},
		{"i.5", "E later", "", "2026-12-01"},
	} {
		ref, err := d.Q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
			LibraryID: db.NullString(tr.id), Name: tr.name, AlbumLibraryID: db.NullString(tr.album), ReleaseDate: tr.date, UpdatedAt: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: ref, Generation: 1}); err != nil {
			t.Fatal(err)
		}
	}
	eng := &playEngine{events: make(chan playback.Event)}
	coord, err := coordinator.New(coordinator.Options{Engine: eng})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	now := localDay(t, "2026-11-01")
	e := &Env{Deps: Deps{DB: d, Library: library.New(d, nil), Coord: coord, Logger: discard(), Now: func() time.Time { return now }}}
	s := &PlaybackService{o: e, m: mapper{now: e.Now}, meta: map[string]trackMeta{}}

	for _, start := range []int{1, 4} {
		if err := s.PlayLibrary(ctx, "title", false, start); ErrorCode(err) != CodeNotReleased {
			t.Fatalf("start %d: %v, want not_released", start, err)
		}
	}
	if err := s.PlayLibrary(ctx, "title", false, 2); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range s.Queue().Items {
		ids = append(ids, it.ID)
	}
	if !slices.Equal(ids, []string{"i.1", "i.3", "i.4"}) {
		t.Fatalf("queue = %v, want the songs already out", ids)
	}
	if got := eng.played(); !slices.Equal(got, []string{"i.3"}) {
		t.Fatalf("played %v, want the start song", got)
	}
}
