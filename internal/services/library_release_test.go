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

// A library song is playable unless the library says it is not out or Apple
// withheld it (library-only, no playParams, a date of its own); a song
// with no IDs can't play. Apple's stand-in artist is no artist on a song not
// out, and a real artist named so stays.
func TestLibSongMapping(t *testing.T) {
	m := mapper{}
	tests := []struct {
		name       string
		s          library.Song
		playable   bool
		artistName string
	}{
		{"a song out", library.Song{LibraryID: "i.1", ArtistName: "Yung Lean"}, true, "Yung Lean"},
		{"a placeholder", library.Song{LibraryID: "i.1", ArtistName: "unknown", Unreleased: true}, false, ""},
		{"a placeholder in other case", library.Song{LibraryID: "i.1", ArtistName: "Unknown", Unreleased: true}, false, ""},
		{"a credited song not out", library.Song{LibraryID: "i.1", ArtistName: "Yung Lean", Unreleased: true}, false, "Yung Lean"},
		{"an artist named unknown on a song out", library.Song{LibraryID: "i.1", ArtistName: "unknown"}, true, "unknown"},
		{"no ID", library.Song{}, false, ""},
		// Apple answers "no longer available" for a library-only song with
		// no playParams and a date of its own (docs/apple-music-reference.md §3.1).
		{"library-only, no playParams, dated", library.Song{LibraryID: "i.1", ArtistName: "Yung Lean", NoPlayParams: true, ReleaseDate: "2020-07-24"}, false, "Yung Lean"},
		{"library-only, no playParams, no date: goes by its album", library.Song{LibraryID: "i.1", ArtistName: "Yung Lean", NoPlayParams: true}, true, "Yung Lean"},
		{"with a catalogue copy, no playParams, dated", library.Song{LibraryID: "i.1", CatalogID: "1440", ArtistName: "Yung Lean", NoPlayParams: true, ReleaseDate: "2020-07-24"}, true, "Yung Lean"},
		{"library-only with playParams, dated", library.Song{LibraryID: "i.1", ArtistName: "Yung Lean", ReleaseDate: "2020-07-24"}, true, "Yung Lean"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.libSong(tc.s)
			if got.Playable != tc.playable || got.Artist != tc.artistName {
				t.Fatalf("Playable %v, artist %q; want %v, %q", got.Playable, got.Artist, tc.playable, tc.artistName)
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
	e := &Env{Deps: Deps{DB: d, Library: libraryAt(d, &now), Logger: discard(), Now: func() time.Time { return now }}}
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
	// The song list has only the songs out, and counts only them: the others
	// are listed on their album alone.
	songs, err := s.Songs(ctx, "title", false, "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if p := playables(songs.Items); !slices.Equal(p, []bool{true, true}) || songs.Total != 2 {
		t.Fatalf("songs playable = %v of %d (title order: i.out, i.single)", p, songs.Total)
	}
	if c, err := s.Counts(ctx); err != nil || c.Songs != 2 {
		t.Fatalf("counts = %+v, %v, want the 2 songs out", c, err)
	}
	// Apple's stand-in artist is no one: the placeholders on the album name none.
	for _, tr := range got.Tracks {
		if !tr.Playable && tr.Artist != "" {
			t.Fatalf("a song not out is credited to %q", tr.Artist)
		}
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
	// And they are songs of the library: listed, counted.
	if songs, err = s.Songs(ctx, "title", false, "", 0, 10); err != nil || len(songs.Items) != 4 || songs.Total != 4 {
		t.Fatalf("songs on release day = %+v, %v", songs, err)
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
	e := &Env{Deps: Deps{DB: d, Library: libraryAt(d, &now), Logger: discard(), Now: func() time.Time { return now }}}
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

// Library → Songs plays the song list (PlayLibrary), which goes by the same
// rule as the list: the songs not out are not in it, so the queue holds only
// the songs out.

// libraryAt is the library over d reading the clock at *now.
func libraryAt(d *db.DB, now *time.Time) *library.Library {
	l := library.New(d, nil)
	l.SetClock(func() time.Time { return *now })
	return l
}

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
	e := &Env{Deps: Deps{DB: d, Library: libraryAt(d, &now), Coord: coord, Logger: discard(), Now: func() time.Time { return now }}}
	s := &PlaybackService{o: e, m: mapper{now: e.Now}, meta: map[string]trackMeta{}}

	// By title: i.last, i.later (neither out), i.out, i.single (out). The
	// list has only the two out, so the rows are 0 and 1.
	if err := s.PlayLibrary(ctx, "title", false, "", 2); CodeOf(err) != CodeInvalidArgument {
		t.Fatalf("start 2 of the 2 songs out: %v, want invalid_argument", err)
	}
	if err := s.PlayLibrary(ctx, "title", false, "", 0); err != nil {
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
	if err := s.PlayLibrary(ctx, "title", false, "", 0); err != nil {
		t.Fatalf("on release day: %v", err)
	}
	if n := len(s.Queue().Items); n != 4 {
		t.Fatalf("queue on release day has %d songs, want 4", n)
	}
}

// The library search offers the songs that are out only: a pre-added album's
// placeholders are in the search index, but nobody can find them by name until
// their day. The album's page (LibraryService.Album) lists them all.
func TestLibrarySearchLeavesOutPreAddedSongsUntilTheyAreOut(t *testing.T) {
	ctx := t.Context()
	d := preAddedStore(t)
	if err := library.RebuildSearch(ctx, d); err != nil {
		t.Fatal(err)
	}
	now := localDay(t, "2026-11-01")
	e := &Env{Deps: Deps{DB: d, Library: libraryAt(d, &now), Logger: discard(), Now: func() time.Time { return now }}}
	s := &SearchService{e: e, m: mapper{now: e.Now}}
	names := func() []string {
		t.Helper()
		got, err := s.Library(ctx, "i", 25)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, so := range got.Songs {
			out = append(out, so.Name)
		}
		slices.Sort(out)
		return out
	}
	if got := names(); !slices.Equal(got, []string{"i.out", "i.single"}) {
		t.Fatalf("before the release: found %v, want the two songs out", got)
	}
	now = localDay(t, "2026-11-20")
	if got := names(); !slices.Equal(got, []string{"i.last", "i.later", "i.out", "i.single"}) {
		t.Fatalf("on release day: found %v, want all four", got)
	}
}
