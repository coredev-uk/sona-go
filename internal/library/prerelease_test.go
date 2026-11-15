package library

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// localDay is midnight starting the local day "YYYY-MM-DD".
func localDay(t testing.TB, day string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation(time.DateOnly, day, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// prereleaseLibrary is a library with one album pre-added: out on 2026-11-20,
// one of its songs already out (with playParams and a date), one not. The
// library lists the album without a releaseDate; only its catalog album says
// when it is out.
func prereleaseLibrary(e *env) {
	f := e.fake
	f.artists = []map[string]any{artist("r.a", "Various")}
	f.albums = []map[string]any{
		preAdded("l.soon", "Soon", "Various", "r.a", "2026-10-23T17:56:10Z", "2026-11-20", true),
		album("l.old", "Old", "Various", "r.a", "2024-01-02T00:00:00Z"),
	}
	out := song("i.out", "Out", "Various", "Soon", "l.soon", "r.a")
	out["attributes"].(map[string]any)["releaseDate"] = "2026-09-17"
	f.songs = []map[string]any{
		out,
		unreleasedSong("i.later", "Later", "Various", "l.soon", "r.a"),
		song("i.old", "Older", "Various", "Old", "l.old", "r.a"),
	}
}

func TestSyncStoresAPreAddedAlbumsRelease(t *testing.T) {
	e := newEnv(t)
	prereleaseLibrary(e)
	e.lib.SetClock(func() time.Time { return localDay(t, "2026-11-18") })
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a, songs, err := e.lib.Album(ctx, "l.soon")
	if err != nil {
		t.Fatal(err)
	}
	if !a.Prerelease || a.ReleaseDate != "" || a.CatalogReleaseDate != "2026-11-20" {
		t.Fatalf("pre-added album = %+v, want flagged, no library date, the catalog's 2026-11-20", a)
	}
	if len(songs) != 2 {
		t.Fatalf("songs = %+v", songs)
	}
	bySong := map[string]Song{}
	for _, s := range songs {
		bySong[s.LibraryID] = s
	}
	if s := bySong["i.out"]; s.Unreleased || s.ReleaseDate != "2026-09-17" {
		t.Fatalf("a song out has a date that has passed: %+v", s)
	}
	s := bySong["i.later"]
	if !s.Unreleased || s.ReleaseDate != "" {
		t.Fatalf("a song not out = %+v, want unreleased with no date of its own", s)
	}
	one, err := e.lib.Song(ctx, "i.later")
	if err != nil || !one.Unreleased {
		t.Fatalf("Song = %+v, %v", one, err)
	}
	// The songs list leaves the placeholder out.
	list, err := e.lib.Songs(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, l := range list {
		listed = append(listed, l.LibraryID)
		if l.Unreleased {
			t.Fatalf("listed song %+v is unreleased", l)
		}
	}
	slices.Sort(listed)
	if !slices.Equal(listed, []string{"i.old", "i.out"}) {
		t.Fatalf("Songs = %v, want the songs out only", listed)
	}
	old, _, err := e.lib.Album(ctx, "l.old")
	if err != nil || old.Prerelease || old.CatalogReleaseDate != "" || old.ReleaseDate != "2020-01-01" {
		t.Fatalf("released album = %+v, %v", old, err)
	}
}

// The newest-first walk stops at the first album it has, so a pre-added
// album would keep its release state for good: the incremental sync asks
// again for the albums still ahead (and for their songs without playParams),
// which clears the flag once the album is out.
func TestIncrementalSyncRefreshesUpcomingAlbums(t *testing.T) {
	e := newEnv(t)
	prereleaseLibrary(e)
	// Every step is within FullSyncEvery of the first run, so each is an
	// incremental one: only that run can clear the flag.
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	e.sync.now = func() time.Time { return now }
	e.lib.SetClock(func() time.Time { return now })
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Nothing changed: one lookup each for the upcoming album and its
	// unplayable song, and not for anything already out.
	e.fake.reset()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	var lookups []string
	for _, r := range e.fake.seen() {
		if strings.Contains(r, "ids=") {
			lookups = append(lookups, r)
		}
	}
	// The domains run in parallel: either may ask first.
	slices.Sort(lookups)
	if len(lookups) != 2 || !strings.Contains(lookups[0], "/albums?") || !strings.Contains(lookups[0], "ids=l.soon&") ||
		!strings.Contains(lookups[1], "/songs?") || !strings.Contains(lookups[1], "ids=i.later&") {
		t.Fatalf("lookups = %v, want l.soon and i.later only", lookups)
	}

	// The song comes out, and then the album: Apple now dates the album
	// itself and drops the flag.
	e.fake.mu.Lock()
	later := song("i.later", "Later", "Various", "Soon", "l.soon", "r.a")
	later["attributes"].(map[string]any)["releaseDate"] = "2026-10-08"
	e.fake.songs[1] = later
	e.fake.mu.Unlock()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if s, _ := e.lib.Song(ctx, "i.later"); s.Unreleased || s.ReleaseDate != "2026-10-08" {
		t.Fatalf("song after it came out = %+v", s)
	}
	if a, _, _ := e.lib.Album(ctx, "l.soon"); !a.Prerelease {
		t.Fatalf("album still ahead lost its flag: %+v", a)
	}
	e.fake.mu.Lock()
	out := album("l.soon", "Soon", "Various", "r.a", "2026-10-23T17:56:10Z")
	out["attributes"].(map[string]any)["releaseDate"] = "2026-11-20"
	e.fake.albums[0] = out
	e.fake.mu.Unlock()
	now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	fullBefore, err := e.db.RQ.GetSyncState(ctx, string(DomainAlbums))
	if err != nil {
		t.Fatal(err)
	}
	e.fake.reset()
	e.steps = nil
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(e.fake.seen(), func(r string) bool { return strings.Contains(r, "/albums?") && strings.Contains(r, "ids=l.soon&") }) {
		t.Fatalf("the run did not ask for the upcoming album: %v", e.fake.seen())
	}
	if st, _ := e.db.RQ.GetSyncState(ctx, string(DomainAlbums)); st.LastFull != fullBefore.LastFull {
		t.Fatalf("last_full moved from %v to %v: not an incremental run", fullBefore.LastFull, st.LastFull)
	}
	a, _, err := e.lib.Album(ctx, "l.soon")
	if err != nil || a.Prerelease || a.ReleaseDate != "2026-11-20" {
		t.Fatalf("album after its release = %+v, %v", a, err)
	}
	if !slices.ContainsFunc(e.steps, func(p Progress) bool { return p.Domain == DomainAlbums }) {
		t.Fatalf("the change was not reported: %+v", e.steps)
	}

	// Settled: no more lookups.
	e.fake.reset()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	for _, r := range e.fake.seen() {
		if strings.Contains(r, "ids=") {
			t.Fatalf("a released album is asked for again: %s", r)
		}
	}
}

// Rows stored before the migration are brought right by the full run it
// forces (00012 clears the albums and songs domains' last full run).
func TestFullSyncBackfillsAPreAddedAlbumStoredAsReleased(t *testing.T) {
	e := newEnv(t)
	prereleaseLibrary(e)
	e.lib.SetClock(func() time.Time { return localDay(t, "2026-11-18") })
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := e.db.W.ExecContext(ctx, `UPDATE albums SET is_prerelease = 0, catalog_release_date = '' WHERE library_id = 'l.soon';
		UPDATE tracks SET no_play_params = 0;
		UPDATE sync_state SET last_full = NULL WHERE domain IN ('albums', 'songs')`); err != nil {
		t.Fatal(err)
	}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	a, _, err := e.lib.Album(ctx, "l.soon")
	if err != nil || !a.Prerelease || a.CatalogReleaseDate != "2026-11-20" {
		t.Fatalf("album after the backfill = %+v, %v", a, err)
	}
	if s, _ := e.lib.Song(ctx, "i.later"); !s.Unreleased {
		t.Fatalf("song after the backfill = %+v", s)
	}
}

// A pre-added album's placeholder songs ("Track 3", artist "unknown") are
// stored like any song but are not songs anyone can find yet: only their album
// lists them, until the day it is out (a fake clock), and then Songs, its
// count, the artist's songs, the favourites and the search have them. The
// next sync gives them their real names.
func TestUnreleasedSongsAreListedOnlyOnTheirAlbumUntilTheyAreOut(t *testing.T) {
	e := newEnv(t)
	prereleaseLibrary(e)
	// Two more not-out songs, as Apple names them, and a favourite of each kind.
	e.fake.songs = append(e.fake.songs,
		unreleasedSong("i.t3", "Track 3", "unknown", "l.soon", "r.a"),
		unreleasedSong("i.t7", "Track 7", "unknown", "l.soon", "r.a"))
	release := localDay(t, "2026-11-20")
	now := release.Add(-5 * time.Hour)
	e.lib.SetClock(func() time.Time { return now })
	e.sync.now = func() time.Time { return now }
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, id := range []string{"i.out", "i.t3"} {
		if _, err := e.db.W.ExecContext(ctx, `INSERT INTO favorites (kind, apple_id, value, updated_at) VALUES ('library-songs', ?, 1, 1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	// state is what every song-level view of the library lists as of the clock.
	type state struct {
		songs, artist, favourites, search, searchAll []string
		count, artistSongs                           int
	}
	ids := func(songs []Song) []string {
		out := []string{}
		for _, s := range songs {
			out = append(out, s.LibraryID)
		}
		slices.Sort(out)
		return out
	}
	read := func(term string) state {
		t.Helper()
		var st state
		all, err := e.lib.AllSongs(ctx, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		st.songs = ids(all)
		c, err := e.lib.Counts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		st.count = c.Songs
		_, _, artistSongs, err := e.lib.Artist(ctx, "r.a")
		if err != nil {
			t.Fatal(err)
		}
		st.artist = ids(artistSongs)
		idx, err := e.lib.ArtistIndex(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range idx {
			if a.LibraryID == "r.a" {
				st.artistSongs = a.Songs
			}
		}
		fav, total, err := e.lib.FavoriteSongs(ctx, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		st.favourites = ids(fav)
		if total != len(fav) {
			t.Fatalf("favourites total %d for %d listed", total, len(fav))
		}
		hits, err := e.lib.Search(ctx, term, 50, "song")
		if err != nil {
			t.Fatal(err)
		}
		st.search = []string{}
		for _, h := range hits {
			st.search = append(st.search, h.LibraryID)
		}
		slices.Sort(st.search)
		r, err := e.lib.SearchAll(ctx, term, 25)
		if err != nil {
			t.Fatal(err)
		}
		st.searchAll = ids(r.Songs)
		return st
	}
	want := func(what string, got state, songs, artist, fav, search []string) {
		t.Helper()
		if !slices.Equal(got.songs, songs) || got.count != len(songs) || !slices.Equal(got.artist, artist) || got.artistSongs != len(artist) ||
			!slices.Equal(got.favourites, fav) || !slices.Equal(got.search, search) || !slices.Equal(got.searchAll, search) {
			t.Fatalf("%s: %+v, want songs %v, artist %v, favourites %v, search %v", what, got, songs, artist, fav, search)
		}
	}

	// Five hours before: the placeholders are on their album and nowhere else.
	want("before", read("track"), []string{"i.old", "i.out"}, []string{"i.old", "i.out"}, []string{"i.out"}, []string{})
	if _, songs, err := e.lib.Album(ctx, "l.soon"); err != nil || len(songs) != 4 {
		t.Fatalf("the album lists %d songs, %v, want its four, the placeholders too", len(songs), err)
	}
	for _, s := range []string{"i.later", "i.t3"} {
		if un, err := e.lib.Unreleased(ctx, s); err != nil || !un {
			t.Fatalf("Unreleased(%s) = %v, %v", s, un, err)
		}
	}
	if _, err := e.lib.Unreleased(ctx, "i.none"); err == nil {
		t.Fatal("a song the library doesn't have is not unreleased")
	}
	// The search never offered them, whichever word it was asked for.
	if got := read("later").search; len(got) != 0 {
		t.Fatalf("search found %v before the release", got)
	}

	// The day comes: the songs are out, with no sync in between.
	now = release.Add(time.Minute)
	all := []string{"i.later", "i.old", "i.out", "i.t3", "i.t7"}
	want("after", read("track"), all, all, []string{"i.out", "i.t3"}, []string{"i.t3", "i.t7"})

	// Back before it (a clock set back): hidden again; nothing was stored.
	now = release.Add(-time.Hour)
	want("again before", read("track"), []string{"i.old", "i.out"}, []string{"i.old", "i.out"}, []string{"i.out"}, []string{})

	// Apple publishes the songs: the next sync gives them their names, and
	// they're found by them.
	now = release.Add(time.Hour)
	e.fake.mu.Lock()
	real3 := song("i.t3", "Jealous Guy", "Yung Lean", "Soon", "l.soon", "r.a")
	e.fake.songs[3] = real3
	e.fake.mu.Unlock()
	e.fake.mu.Lock()
	out := album("l.soon", "Soon", "Various", "r.a", "2026-10-23T17:56:10Z")
	out["attributes"].(map[string]any)["releaseDate"] = "2026-11-20"
	e.fake.albums[0] = out
	e.fake.mu.Unlock()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if got := read("jealous"); !slices.Equal(got.search, []string{"i.t3"}) || !slices.Contains(got.songs, "i.t3") {
		t.Fatalf("after the sync: %+v, want the song found by its real name", got)
	}
}

// The release rule, case by case, as of a fake clock: a dated song goes by
// its date, one with playParams is out, and only one with neither goes by its
// library album (its own date, replaced by the catalogue's when that is
// ahead or the library has none; a year alone goes by Apple's flag).
func TestReleaseRule(t *testing.T) {
	release := localDay(t, "2026-11-20")
	before, after := release.Add(-time.Hour), release
	albums := map[string]struct {
		date, catalogDate string
		flagged           int64
	}{
		"l.ahead":    {"", "2026-11-20", 1},
		"l.unflag":   {"", "2026-11-20", 0},
		"l.libdate":  {"2026-11-20", "", 0},
		"l.stale":    {"2020-01-01", "", 1},
		"l.catahead": {"2026-10-01", "2026-11-20", 1},
		"l.yearflag": {"", "2027", 1},
		"l.year":     {"", "2027", 0},
		"l.nodate":   {"", "", 0},
		"l.nodateup": {"", "", 1},
		"l.out":      {"2020-01-01", "", 0},
	}
	tests := []struct {
		name         string
		album        string // "" none stored
		date         string
		noPlayParams int64
		now          time.Time
		want         bool // not out
	}{
		{"no playParams on an album ahead", "l.ahead", "", 1, before, true},
		{"no playParams, the album's day has come", "l.ahead", "", 1, after, false},
		{"the catalogue's day alone, no flag", "l.unflag", "", 1, before, true},
		{"the library's own day ahead", "l.libdate", "", 1, before, true},
		{"a library day passed, a stale flag", "l.stale", "", 1, before, false},
		{"the catalogue's day is ahead of the library's", "l.catahead", "", 1, before, true},
		{"a year only, flagged", "l.yearflag", "", 1, after, true},
		{"a year only, not flagged", "l.year", "", 1, after, false},
		{"no day at all, not flagged", "l.nodate", "", 1, before, false},
		{"no day at all, flagged", "l.nodateup", "", 1, before, true},
		// Apple gives some singles playParams and no releaseDate (Flower In A
		// Hurricane, GTA VI): the song is out whatever its album says.
		{"playParams and no date on an album ahead", "l.ahead", "", 0, before, false},
		{"a song out, on an album ahead", "l.ahead", "2026-09-17", 1, before, false},
		{"a song dated ahead", "l.out", "2026-12-01", 0, release, true},
		{"a song dated today", "l.out", "2026-11-20", 0, after, false},
		{"a song dated a year ahead", "l.out", "2027", 0, after, false},
		{"no playParams on an album out", "l.out", "", 1, after, false},
		{"no playParams, no stored album", "", "", 1, after, false},
	}
	e := newEnv(t)
	ctx := context.Background()
	for id, a := range albums {
		if _, err := e.db.Q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
			LibraryID: db.NullString(id), Name: id, ReleaseDate: a.date, CatalogReleaseDate: a.catalogDate, IsPrerelease: a.flagged, UpdatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i, tc := range tests {
		id := fmt.Sprintf("i.%d", i)
		if _, err := e.db.Q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
			LibraryID: db.NullString(id), Name: tc.name, AlbumLibraryID: db.NullString(tc.album), ReleaseDate: tc.date,
			NoPlayParams: tc.noPlayParams, UpdatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e.lib.SetClock(func() time.Time { return tc.now })
			got, err := e.lib.Unreleased(ctx, fmt.Sprintf("i.%d", i))
			if err != nil || got != tc.want {
				t.Fatalf("Unreleased = %v, %v; want %v", got, err, tc.want)
			}
			song, err := e.lib.Song(ctx, fmt.Sprintf("i.%d", i))
			if err != nil || song.Unreleased != tc.want {
				t.Fatalf("Song.Unreleased = %v, %v; want %v", song.Unreleased, err, tc.want)
			}
		})
	}
}
