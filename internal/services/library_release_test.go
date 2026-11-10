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

// A song of a pre-added album that is not out is in the library with no
// playParams and no releaseDate; one already out has both. Without an album
// not out, a song with neither is as playable as it was.
func TestLibSongRelease(t *testing.T) {
	release := localDay(t, "2026-11-20")
	ahead := library.ReleaseState{CatalogDate: "2026-11-20", Prerelease: true}
	tests := []struct {
		name string
		s    library.Song
		now  time.Time
		want bool
	}{
		{"no playParams on an album ahead", library.Song{LibraryID: "i.1", NoPlayParams: true, Album: ahead}, release.Add(-time.Hour), false},
		{"no playParams, no date, the album's date has passed", library.Song{LibraryID: "i.1", NoPlayParams: true, Album: ahead}, release, true},
		{"no playParams, a year-only album still flagged", library.Song{LibraryID: "i.1", NoPlayParams: true, Album: library.ReleaseState{CatalogDate: "2027", Prerelease: true}}, release, false},
		{"playParams but no date of its own on an album ahead", library.Song{LibraryID: "i.1", Album: ahead}, release.Add(-time.Hour), false},
		{"a song out, on an album ahead", library.Song{LibraryID: "i.1", ReleaseDate: "2026-09-17", Album: ahead}, release.Add(-time.Hour), true},
		{"a song dated ahead", library.Song{LibraryID: "i.1", ReleaseDate: "2026-12-01"}, release, false},
		{"no playParams on an album that is out", library.Song{LibraryID: "i.1", NoPlayParams: true, Album: library.ReleaseState{Date: "2020-01-01"}}, release, true},
		{"no playParams, no stored album", library.Song{LibraryID: "i.1", NoPlayParams: true}, release, true},
		{"no ID", library.Song{NoPlayParams: false}, release, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := mapper{now: func() time.Time { return tc.now }}
			if got := m.libSong(tc.s).Playable; got != tc.want {
				t.Fatalf("Playable = %v, want %v", got, tc.want)
			}
		})
	}
}

// preAddedStore holds a pre-added library album as the sync stores it: no
// library release date, the catalog's date and flag, one song out (dated, with
// playParams) and two not (neither).
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
	}{{"i.out", "2026-09-17", 0}, {"i.later", "", 1}, {"i.last", "", 1}} {
		ref, err := d.Q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
			LibraryID: db.NullString(tr.id), Name: tr.id, AlbumLibraryID: db.NullString("l.soon"),
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
	e := &Env{Deps: Deps{DB: d, Library: library.New(d, nil), Logger: discard(), Now: func() time.Time { return now }}}
	s := &LibraryService{o: e, m: mapper{now: e.Now}}

	got, err := s.Album(ctx, "l.soon")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Album.Prerelease || got.Album.ReleaseAt != release.UnixMilli() || got.Album.ReleaseDate != "2026-11-20" {
		t.Fatalf("album = %+v, want a pre-release out on 2026-11-20", got.Album)
	}
	if p := playables(got.Tracks); !slices.Equal(p, []bool{true, false, false}) {
		t.Fatalf("playable = %v, want only the song already out", p)
	}
	// The song list shows the same, without the album page's help.
	songs, err := s.Songs(ctx, "title", false, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if p := playables(songs.Items); !slices.Equal(p, []bool{false, false, true}) {
		t.Fatalf("songs playable = %v (title order: i.last, i.later, i.out)", p)
	}

	// At midnight on the day it is out, and its songs play.
	now = release
	got, err = s.Album(ctx, "l.soon")
	if err != nil {
		t.Fatal(err)
	}
	if got.Album.Prerelease || got.Album.ReleaseAt != release.UnixMilli() || !slices.Equal(playables(got.Tracks), []bool{true, true, true}) {
		t.Fatalf("released = %+v, playable %v", got.Album, playables(got.Tracks))
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
	e := &Env{Deps: Deps{DB: d, Logger: discard(), Now: func() time.Time { return now }}}
	s := &PlaybackService{o: e, m: mapper{now: e.Now}, meta: map[string]trackMeta{}}
	refs := []TrackRef{{ID: "i.out", Source: SourceLibrary}, {ID: "i.later", Source: SourceLibrary}, {ID: "i.last", Source: SourceLibrary}}

	for _, start := range []int{1, 2} {
		if _, _, err := s.resolve(ctx, refs, start); !errors.Is(err, ErrNotReleased) || CodeOf(err) != CodeNotReleased {
			t.Fatalf("start %d: err = %v, want not_released", start, err)
		}
	}
	items, at, err := s.resolve(ctx, refs, 0)
	if err != nil || len(items) != 1 || items[0].ID != "i.out" || at != 0 {
		t.Fatalf("resolve from the song out = %+v, %d, %v", items, at, err)
	}

	now = localDay(t, "2026-11-20")
	items, _, err = s.resolve(ctx, refs, 1)
	if err != nil || len(items) != 3 {
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

// Library → Songs plays the song list (PlayLibrary), which goes by the same
// rule: starting on a song not out is refused, and starting on one that is
// out queues only the songs out.
func TestPlayLibraryRefusesPreAddedSongs(t *testing.T) {
	ctx := t.Context()
	d := preAddedStore(t)
	eng := &playEngine{events: make(chan playback.Event)}
	coord, err := coordinator.New(coordinator.Options{Engine: eng})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	now := localDay(t, "2026-11-01")
	e := &Env{Deps: Deps{DB: d, Library: library.New(d, nil), Coord: coord, Logger: discard(), Now: func() time.Time { return now }}}
	s := &PlaybackService{o: e, m: mapper{now: e.Now}, meta: map[string]trackMeta{}}

	// By title: i.last, i.later (neither out), i.out.
	for _, start := range []int{0, 1} {
		if err := s.PlayLibrary(ctx, "title", false, start); CodeOf(err) != CodeNotReleased {
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
	if !slices.Equal(ids, []string{"i.out"}) {
		t.Fatalf("queue = %v, want only the song out", ids)
	}

	now = localDay(t, "2026-11-20")
	if err := s.PlayLibrary(ctx, "title", false, 0); err != nil {
		t.Fatalf("on release day: %v", err)
	}
	if n := len(s.Queue().Items); n != 3 {
		t.Fatalf("queue on release day has %d songs, want 3", n)
	}
}
