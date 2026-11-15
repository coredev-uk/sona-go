package library

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// placeholderLibrary is a library with an upcoming album pre-added the way
// Apple lists it (testdata/albums/gta6-prerelease.json in services): out on
// 2026-11-20, one song out, the others "Track N" by "unknown" with no
// playParams and no date; beside an old album, all by one artist.
func placeholderLibrary(e *env) {
	f := e.fake
	f.artists = []map[string]any{artist("r.a", "Various")}
	f.albums = []map[string]any{
		preAdded("l.soon", "Soon", "Various", "r.a", "2026-10-23T17:56:10Z", "2026-11-20", true),
		album("l.old", "Old", "Various", "r.a", "2024-01-02T00:00:00Z"),
	}
	out := song("i.out", "Out Already", "Various", "Soon", "l.soon", "r.a")
	out["attributes"].(map[string]any)["releaseDate"] = "2026-09-17"
	f.songs = []map[string]any{
		out,
		unreleasedSong("i.t3", "Track 3", "unknown", "l.soon", "r.a"),
		unreleasedSong("i.t7", "Track 7", "unknown", "l.soon", "r.a"),
		song("i.old", "Older", "Various", "Old", "l.old", "r.a"),
	}
}

func songIDs(songs []Song) []string {
	out := make([]string, len(songs))
	for i, s := range songs {
		out[i] = s.LibraryID
	}
	slices.Sort(out)
	return out
}

func hitIDs(t *testing.T, l *Library, term string) []string {
	t.Helper()
	hits, err := l.Search(context.Background(), term, 20, "song")
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.LibraryID
	}
	slices.Sort(out)
	return out
}

// Song lists, their count, an artist's songs, favourites and the search leave
// out the songs of a pre-added album that are not out, and hold them once the
// album's day has come (the clock decides; Library.SetClock).
func TestSongListsLeaveOutUnreleasedPlaceholders(t *testing.T) {
	e := newEnv(t)
	placeholderLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	e.lib.SetClock(func() time.Time { return now })
	for _, id := range []string{"i.out", "i.t3", "i.old"} {
		if _, err := e.db.W.ExecContext(ctx, `INSERT INTO favorites (kind, apple_id, value, updated_at) VALUES ('library-songs', ?, 1, 1)`, id); err != nil {
			t.Fatal(err)
		}
	}

	check := func(when string, want []string) {
		t.Helper()
		songs, err := e.lib.AllSongs(ctx, ListOptions{})
		if err != nil || !slices.Equal(songIDs(songs), want) {
			t.Errorf("%s: Songs = %v, %v; want %v", when, songIDs(songs), err, want)
		}
		for _, sortBy := range []Sort{SortArtist, SortAlbum, SortDateAdded} {
			if again, err := e.lib.Songs(ctx, ListOptions{Sort: sortBy, Limit: 100}); err != nil || !slices.Equal(songIDs(again), want) {
				t.Errorf("%s: Songs by %s = %v, %v; want %v", when, sortBy, songIDs(again), err, want)
			}
		}
		// A page counts the songs it can reach: offset and limit apply after
		// the placeholders are gone.
		if page, err := e.lib.Songs(ctx, ListOptions{Offset: len(want) - 1, Limit: 5}); err != nil || len(page) != 1 {
			t.Errorf("%s: last page = %v, %v; want one song", when, songIDs(page), err)
		}
		if c, err := e.lib.Counts(ctx); err != nil || c.Songs != len(want) || c.Albums != 2 {
			t.Errorf("%s: Counts = %+v, %v; want %d songs, 2 albums", when, c, err, len(want))
		}
		_, _, artistSongs, err := e.lib.Artist(ctx, "r.a")
		if err != nil || !slices.Equal(songIDs(artistSongs), want) {
			t.Errorf("%s: the artist's songs = %v, %v; want %v", when, songIDs(artistSongs), err, want)
		}
		if idx, err := e.lib.ArtistIndex(ctx); err != nil || len(idx) != 1 || idx[0].Songs != len(want) {
			t.Errorf("%s: artist index = %+v, %v; want %d songs", when, idx, err, len(want))
		}
		favs, total, err := e.lib.FavoriteSongs(ctx, ListOptions{})
		wantFavs := slices.DeleteFunc(slices.Clone(want), func(id string) bool { return id == "i.t7" })
		if err != nil || !slices.Equal(songIDs(favs), wantFavs) || total != len(wantFavs) {
			t.Errorf("%s: favourites = %v of %d, %v; want %v", when, songIDs(favs), total, err, wantFavs)
		}
		// The placeholders' names are what the index holds; the search
		// leaves them out all the same.
		wantTrack := []string(nil)
		if slices.Contains(want, "i.t3") {
			wantTrack = []string{"i.t3", "i.t7"}
		}
		if got := hitIDs(t, e.lib, "track"); !slices.Equal(got, wantTrack) {
			t.Errorf("%s: search \"track\" = %v, want %v", when, got, wantTrack)
		}
		if got := hitIDs(t, e.lib, "unknown"); !slices.Equal(got, wantTrack) {
			t.Errorf("%s: search \"unknown\" (the placeholder artist) = %v, want %v", when, got, wantTrack)
		}
		if got := hitIDs(t, e.lib, "soon"); !slices.Equal(got, slices.DeleteFunc(slices.Clone(want), func(id string) bool { return id == "i.old" })) {
			t.Errorf("%s: search by the album's name = %v, want its songs out", when, got)
		}
		res, err := e.lib.SearchAll(ctx, "track", 10)
		if err != nil || !slices.Equal(songIDs(res.Songs), wantTrack) || res.SongsMore {
			t.Errorf("%s: SearchAll songs = %v, %v", when, songIDs(res.Songs), err)
		}
	}
	check("before the release", []string{"i.old", "i.out"})

	// The album page keeps them: it is the album's own list.
	if _, tracks, err := e.lib.Album(ctx, "l.soon"); err != nil || len(tracks) != 3 {
		t.Fatalf("Album = %d tracks, %v; want all three", len(tracks), err)
	}

	// The last minute before, and the first of the day.
	now = time.Date(2026, 11, 19, 23, 59, 59, 0, time.Local)
	check("one second before", []string{"i.old", "i.out"})
	now = time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local)
	check("at the release", []string{"i.old", "i.out", "i.t3", "i.t7"})
}

// After the release the sync replaces the placeholders with Apple's real
// metadata, and the songs are in the lists and the search with no manual
// sync: the incremental run asks again for the albums and songs still ahead.
func TestSyncAfterReleaseReplacesPlaceholderSongs(t *testing.T) {
	e := newEnv(t)
	placeholderLibrary(e)
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	e.sync.now = func() time.Time { return now }
	e.lib.SetClock(func() time.Time { return now })
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if got := hitIDs(t, e.lib, "glass"); len(got) != 0 {
		t.Fatalf("a song nobody has seen is found: %v", got)
	}

	// Apple publishes the album: the songs get their names, artists, play
	// parameters and dates, and the album its own date and no flag.
	e.fake.mu.Lock()
	for i, id := range []string{"i.t3", "i.t7"} {
		name := []string{"Glass Houses", "Evening Bus"}[i]
		s := song(id, name, "Mia Vale", "Soon", "l.soon", "r.a")
		s["attributes"].(map[string]any)["releaseDate"] = "2026-11-20"
		e.fake.songs[1+i] = s
	}
	out := album("l.soon", "Soon", "Various", "r.a", "2026-10-23T17:56:10Z")
	out["attributes"].(map[string]any)["releaseDate"] = "2026-11-20"
	e.fake.albums[0] = out
	e.fake.mu.Unlock()
	now = time.Date(2026, 11, 20, 9, 0, 0, 0, time.Local)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}

	songs, err := e.lib.AllSongs(ctx, ListOptions{Sort: SortTitle})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range songs {
		names = append(names, s.Name+" · "+s.ArtistName)
		if s.NoPlayParams || s.ArtistName == "unknown" || s.Name == "Track 3" || s.Name == "Track 7" {
			t.Errorf("placeholder metadata kept: %+v", s)
		}
	}
	want := []string{"Evening Bus · Mia Vale", "Glass Houses · Mia Vale", "Older · Various", "Out Already · Various"}
	if !slices.Equal(names, want) {
		t.Fatalf("songs = %q, want %q", names, want)
	}
	if got := hitIDs(t, e.lib, "glass"); !slices.Equal(got, []string{"i.t3"}) {
		t.Errorf("search \"glass\" = %v, want the song that came out", got)
	}
	if got := hitIDs(t, e.lib, "track"); len(got) != 0 {
		t.Errorf("search \"track\" = %v, the placeholder's name is gone", got)
	}
	if c, err := e.lib.Counts(ctx); err != nil || c.Songs != 4 {
		t.Errorf("Counts = %+v, %v; want 4 songs", c, err)
	}
}

// The songs still without playParams are asked for again whatever their album
// says: an album that has come out and lost its flag (or whose date has just
// passed) does not keep them from being refreshed.
func TestSyncRefreshesPlaceholderSongsOfAnAlbumThatIsOut(t *testing.T) {
	e := newEnv(t)
	placeholderLibrary(e)
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	e.sync.now = func() time.Time { return now }
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	// Apple has cleared the album's flag and dated it; the songs are not yet
	// published.
	e.fake.mu.Lock()
	out := album("l.soon", "Soon", "Various", "r.a", "2026-10-23T17:56:10Z")
	out["attributes"].(map[string]any)["releaseDate"] = "2026-11-20"
	e.fake.albums[0] = out
	e.fake.mu.Unlock()
	now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.Local)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	e.fake.reset()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(e.fake.seen(), func(r string) bool {
		return strings.Contains(r, "/songs?") && strings.Contains(r, "ids=i.t3%2Ci.t7&")
	}) {
		t.Fatalf("the placeholder songs were not asked for: %v", e.fake.seen())
	}
}
