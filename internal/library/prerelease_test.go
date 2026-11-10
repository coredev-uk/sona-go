package library

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

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
	if s := bySong["i.out"]; s.NoPlayParams || s.ReleaseDate != "2026-09-17" {
		t.Fatalf("a song out has playParams and a date: %+v", s)
	}
	s := bySong["i.later"]
	if !s.NoPlayParams || s.ReleaseDate != "" || !s.Album.Prerelease || s.Album.CatalogDate != "2026-11-20" {
		t.Fatalf("a song not out = %+v, want no playParams and its album's release state", s)
	}
	one, err := e.lib.Song(ctx, "i.later")
	if err != nil || one.NoPlayParams != true || one.Album != s.Album {
		t.Fatalf("Song = %+v, %v", one, err)
	}
	// Lists carry the same state.
	list, err := e.lib.Songs(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range list {
		if l.LibraryID == "i.later" && (!l.NoPlayParams || !l.Album.Prerelease) {
			t.Fatalf("listed song = %+v", l)
		}
		if l.LibraryID == "i.old" && (l.NoPlayParams || l.Album.Prerelease) {
			t.Fatalf("a released album's song = %+v", l)
		}
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
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	e.sync.now = func() time.Time { return now }
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
	if s, _ := e.lib.Song(ctx, "i.later"); s.NoPlayParams || s.ReleaseDate != "2026-10-08" {
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
	now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	e.steps = nil
	if err := e.run(t); err != nil {
		t.Fatal(err)
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
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := e.db.W.ExecContext(ctx, `UPDATE albums SET is_prerelease = 0, catalog_release_date = '' WHERE library_id = 'l.soon';
		UPDATE tracks SET has_play_params = 1;
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
	if s, _ := e.lib.Song(ctx, "i.later"); !s.NoPlayParams {
		t.Fatalf("song after the backfill = %+v", s)
	}
}
