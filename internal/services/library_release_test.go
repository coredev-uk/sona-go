package services

import (
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// A pre-added library album as Apple lists it carries no releaseDate of its
// own and no isPrerelease: only its catalog album has them, and the library
// stores what the walk fetched (library.Album.CatalogReleaseDate and
// Prerelease). The album is out at midnight on the catalog's date, local time.
func TestLibAlbumRelease(t *testing.T) {
	release := localDay(t, "2026-11-20")
	before, after := release.Add(-18*24*time.Hour), release.Add(time.Second)
	tests := []struct {
		name         string
		a            library.Album
		now          time.Time
		prerelease   bool
		date         string
		releaseAt    int64
		wantReleased bool
	}{
		{"pre-added: no library date, the catalog's ahead", library.Album{CatalogReleaseDate: "2026-11-20", Prerelease: true}, before, true, "2026-11-20", release.UnixMilli(), false},
		{"the catalog's date alone, without the flag", library.Album{CatalogReleaseDate: "2026-11-20"}, before, true, "2026-11-20", release.UnixMilli(), false},
		{"the date has passed: out", library.Album{CatalogReleaseDate: "2026-11-20", Prerelease: true}, after, false, "2026-11-20", release.UnixMilli(), true},
		{"a future library date", library.Album{ReleaseDate: "2026-11-20"}, before, true, "2026-11-20", release.UnixMilli(), false},
		{"a library date that has passed, a stale flag", library.Album{ReleaseDate: "2020-01-01", Prerelease: true}, before, false, "2020-01-01", localDay(t, "2020-01-01").UnixMilli(), true},
		{"the catalog's date is ahead of the library's", library.Album{ReleaseDate: "2026-10-01", CatalogReleaseDate: "2026-11-20", Prerelease: true}, before, true, "2026-11-20", release.UnixMilli(), false},
		{"a year only, flagged", library.Album{CatalogReleaseDate: "2027", Prerelease: true}, before, true, "2027", 0, false},
		{"a year only, not flagged", library.Album{CatalogReleaseDate: "2027"}, before, false, "2027", 0, true},
		{"no date at all, not flagged", library.Album{}, before, false, "", 0, true},
		{"no date at all, flagged", library.Album{Prerelease: true}, before, true, "", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := mapper{now: func() time.Time { return tc.now }}
			got := m.libAlbum(tc.a)
			if got.Prerelease != tc.prerelease || got.ReleaseDate != tc.date || got.ReleaseAt != tc.releaseAt {
				t.Fatalf("libAlbum = prerelease %v, date %q, at %d; want %v, %q, %d", got.Prerelease, got.ReleaseDate, got.ReleaseAt, tc.prerelease, tc.date, tc.releaseAt)
			}
		})
	}
}

// A library song is playable unless the library says it is not out
// (library.Song.Unreleased: tested in internal/library), and an upload plays
// by its library ID.
func TestLibSongPlayableFollowsTheLibrary(t *testing.T) {
	m := mapper{now: time.Now}
	tests := []struct {
		name string
		s    library.Song
		want bool
	}{
		{"out", library.Song{LibraryID: "i.1"}, true},
		{"not out", library.Song{LibraryID: "i.1", Unreleased: true}, false},
		{"an upload", library.Song{LibraryID: "i.1", CatalogID: ""}, true},
		{"no ID", library.Song{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.libSong(tc.s).Playable; got != tc.want {
				t.Fatalf("Playable = %v, want %v", got, tc.want)
			}
		})
	}
}

// preAddedStore holds a pre-added library album as the sync stores it: no
// library release date, the catalog's date and flag, one song out (dated, with
// playParams), one out with playParams but no date of its own, and two not
// (neither).
func preAddedStore(t *testing.T) *db.DB {
	t.Helper()
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	id, err := d.Q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
		LibraryID: db.NullString("l.soon"), CatalogID: db.NullString("7"), Name: "Soon",
		IsPrerelease: 1, CatalogReleaseDate: "2026-11-20", UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "album", Ref: id, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	for i, tr := range []struct {
		id, date     string
		noPlayParams int64
	}{{"i.out", "2026-09-17", 0}, {"i.later", "", 1}, {"i.last", "", 1}, {"i.single", "", 0}} {
		ref, err := d.Q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
			LibraryID: db.NullString(tr.id), Name: tr.id, AlbumLibraryID: db.NullString("l.soon"), ArtistName: map[int64]string{0: "Mia Vale", 1: "unknown"}[tr.noPlayParams],
			TrackNumber: int64(i + 1), DiscNumber: 1, ReleaseDate: tr.date, NoPlayParams: tr.noPlayParams, UpdatedAt: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: ref, Generation: 1, AddedRank: sql.NullInt64{Int64: int64(i), Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestLibraryAlbumDetailOfAPreAddedAlbum(t *testing.T) {
	ctx := t.Context()
	d := preAddedStore(t)
	release := localDay(t, "2026-11-20")
	now := release.Add(-5 * time.Hour)
	lib := library.New(d, nil)
	lib.SetClock(func() time.Time { return now })
	e := &Env{Deps: Deps{DB: d, Library: lib, Logger: discard(), Now: func() time.Time { return now }}}
	s := &LibraryService{o: e, m: mapper{now: e.Now}}

	got, err := s.Album(ctx, "l.soon")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Album.Prerelease || got.Album.ReleaseAt != release.UnixMilli() || got.Album.ReleaseDate != "2026-11-20" {
		t.Fatalf("album = %+v, want a pre-release out on 2026-11-20", got.Album)
	}
	if p := playables(got.Tracks); !slices.Equal(p, []bool{true, false, false, true}) {
		t.Fatalf("playable = %v, want the songs already out (dated, or with playParams)", p)
	}
	// Apple's "unknown" is the credit of the songs not out, and no artist.
	var artists []string
	for _, tr := range got.Tracks {
		artists = append(artists, tr.Artist)
	}
	if !slices.Equal(artists, []string{"Mia Vale", "", "", "Mia Vale"}) {
		t.Fatalf("artists = %q, want none for the songs not out", artists)
	}
	// The song list holds only the songs out, with their count: the album
	// page keeps the others, as songs to come.
	songs, err := s.Songs(ctx, "title", false, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if p := playables(songs.Items); !slices.Equal(p, []bool{true, true}) || songs.Total != 2 {
		t.Fatalf("songs = %v of %d, want i.out and i.single only", p, songs.Total)
	}

	// At midnight on the day it is out, and its songs play.
	now = release
	got, err = s.Album(ctx, "l.soon")
	if err != nil {
		t.Fatal(err)
	}
	if got.Album.Prerelease || got.Album.ReleaseAt != release.UnixMilli() || !slices.Equal(playables(got.Tracks), []bool{true, true, true, true}) {
		t.Fatalf("released = %+v, playable %v", got.Album, playables(got.Tracks))
	}
	if songs, err = s.Songs(ctx, "title", false, 0, 10); err != nil || len(songs.Items) != 4 || songs.Total != 4 {
		t.Fatalf("songs on release day = %+v, %v, want all four", songs, err)
	}
}

// Playback guards a pre-added library song the way it guards a catalog one:
// the song itself is refused (ErrNotReleased), a play list drops it, and from
// the album's day on it plays. The library keeps no date for the album, and no
// date for the song.
func TestPlaybackRefusesPreAddedLibrarySongs(t *testing.T) {
	ctx := t.Context()
	d := preAddedStore(t)
	now := localDay(t, "2026-11-01")
	lib := library.New(d, nil)
	lib.SetClock(func() time.Time { return now })
	e := &Env{Deps: Deps{DB: d, Library: lib, Logger: discard(), Now: func() time.Time { return now }}}
	s := &PlaybackService{o: e, m: mapper{now: e.Now}, meta: map[string]trackMeta{}}
	refs := []TrackRef{{ID: "i.out", Source: SourceLibrary}, {ID: "i.later", Source: SourceLibrary}, {ID: "i.last", Source: SourceLibrary}, {ID: "i.single", Source: SourceLibrary}}

	for _, start := range []int{1, 2} {
		if _, _, err := s.resolve(ctx, refs, start); !errors.Is(err, ErrNotReleased) || CodeOf(err) != CodeNotReleased {
			t.Fatalf("start %d: err = %v, want not_released", start, err)
		}
	}
	items, at, err := s.resolve(ctx, refs, 0)
	if err != nil || len(items) != 2 || items[0].ID != "i.out" || items[1].ID != "i.single" || at != 0 {
		t.Fatalf("resolve from the song out = %+v, %d, %v", items, at, err)
	}

	now = localDay(t, "2026-11-20")
	items, _, err = s.resolve(ctx, refs, 1)
	if err != nil || len(items) != 4 {
		t.Fatalf("resolve on release day = %+v, %v", items, err)
	}
}

func playables(songs []Song) []bool {
	out := make([]bool, len(songs))
	for i, s := range songs {
		out[i] = s.Playable
	}
	return out
}

// Library → Songs plays the song list (PlayLibrary), which is the list of
// the songs out: a song not out is neither in the queue nor an index of it,
// and from the album's day on it is.
func TestPlayLibraryQueuesOnlyTheSongsOut(t *testing.T) {
	ctx := t.Context()
	d := preAddedStore(t)
	eng := &playEngine{events: make(chan playback.Event)}
	coord, err := coordinator.New(coordinator.Options{Engine: eng})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	now := localDay(t, "2026-11-01")
	lib := library.New(d, nil)
	lib.SetClock(func() time.Time { return now })
	e := &Env{Deps: Deps{DB: d, Library: lib, Coord: coord, Logger: discard(), Now: func() time.Time { return now }}}
	s := &PlaybackService{o: e, m: mapper{now: e.Now}, meta: map[string]trackMeta{}}

	// By title the songs out are i.out and i.single (i.last and i.later are
	// not listed).
	if err := s.PlayLibrary(ctx, "title", false, 2, ""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("start 2 of 2 songs: %v, want invalid_argument", err)
	}
	// The song the caller meant is found in the list as it is now, wherever
	// the caller's page had it.
	if err := s.PlayLibrary(ctx, "title", false, 0, "i.single"); err != nil {
		t.Fatal(err)
	}
	if got := s.Queue().Index; got != 1 {
		t.Fatalf("started at %d, want i.single (1)", got)
	}
	if err := s.PlayLibrary(ctx, "title", false, 0, "i.later"); err != nil {
		t.Fatalf("a song not in the list falls back to the index: %v", err)
	}
	if got := s.Queue().Index; got != 0 {
		t.Fatalf("started at %d, want the index 0", got)
	}
	if err := s.PlayLibrary(ctx, "title", false, 0, ""); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range s.Queue().Items {
		ids = append(ids, it.ID)
	}
	if !slices.Equal(ids, []string{"i.out", "i.single"}) {
		t.Fatalf("queue = %v, want only the songs out", ids)
	}

	now = localDay(t, "2026-11-20")
	if err := s.PlayLibrary(ctx, "title", false, 0, ""); err != nil {
		t.Fatalf("on release day: %v", err)
	}
	if n := len(s.Queue().Items); n != 4 {
		t.Fatalf("queue on release day has %d songs, want 4", n)
	}
}
