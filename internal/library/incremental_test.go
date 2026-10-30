package library

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// libraryRequests counts the requests to the library collections and
// ratings (the playlist list is always walked).
func libraryRequests(reqs []string) (n int) {
	for _, r := range reqs {
		if !strings.HasPrefix(r, "/v1/me/library/playlists") {
			n++
		}
	}
	return n
}

func TestIncrementalResyncTakesOnlyNewItems(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 250)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Nothing changed: a handful of small requests, no rewrites.
	e.fake.reset()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if n := libraryRequests(e.fake.seen()); n != 3 {
		t.Fatalf("no-change resync made %d library requests: %v", n, e.fake.seen())
	}
	if reqs := e.fake.reqs("/v1/me/library/playlists/p.big/tracks"); len(reqs) != 0 {
		t.Fatalf("unchanged playlist tracks refetched: %v", reqs)
	}

	// Three songs and an album added on another device, one of them rated.
	e.fake.mu.Lock()
	var added []map[string]any
	for i := range 3 {
		added = append(added, song(fmt.Sprintf("i.new%d", i), fmt.Sprintf("New %d", i), "Artist 0", "Fresh", "l.new", "r.0"))
	}
	e.fake.songs = append(added, e.fake.songs...)
	e.fake.albums = append([]map[string]any{album("l.new", "Fresh", "Artist 0", "r.0", "2026-10-30T10:00:00Z")}, e.fake.albums...)
	e.fake.ratings["library-songs"]["i.new1"] = 1
	e.fake.mu.Unlock()
	e.fake.reset()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if reqs := e.fake.reqs("/v1/me/library/songs"); !slices.Equal(reqs, []int{0}) {
		t.Fatalf("song pages = %v, want only the first", reqs)
	}
	var rated []string
	for _, r := range e.fake.seen() {
		if strings.HasPrefix(r, "/v1/me/ratings/") {
			rated = append(rated, r)
		}
	}
	if len(rated) != 2 || !strings.Contains(rated[0], "i.new0%2Ci.new1%2Ci.new2") || !strings.Contains(rated[1], "l.new") {
		t.Fatalf("ratings requests = %v, want the new songs and album only", rated)
	}
	if c, _ := e.lib.Counts(ctx); c.Songs != 253 || c.Albums != 26 {
		t.Fatalf("counts = %+v", c)
	}
	songs, err := e.lib.Songs(ctx, ListOptions{Sort: SortDateAdded, Desc: true, Limit: 4})
	if err != nil || len(songs) != 4 || songs[0].LibraryID != "i.new0" || songs[2].LibraryID != "i.new2" || songs[3].LibraryID != "i.0" {
		t.Fatalf("newest songs = %+v, %v", songs, err)
	}
	if v, _ := e.lib.Rating(ctx, applemusic.TypeLibrarySongs, "i.new1"); v != applemusic.RatingFavorite {
		t.Fatalf("new song's rating = %d", v)
	}
	if hits, _ := e.lib.Search(ctx, "fresh", 5, "album"); len(hits) != 1 {
		t.Fatalf("search index missed the new album: %+v", hits)
	}
	st, _ := e.db.RQ.GetSyncState(ctx, string(DomainSongs))
	if !st.LastFull.Valid || st.LastSuccess.Int64 < st.LastFull.Int64 {
		t.Fatalf("songs state = %+v", st)
	}
}

func TestIncrementalResyncRunsFullyWhenCountsDiffer(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 100)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	e.fake.mu.Lock()
	e.fake.songs = e.fake.songs[:70]
	e.fake.artists = e.fake.artists[:4]
	e.fake.mu.Unlock()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if c, _ := e.lib.Counts(ctx); c.Songs != 70 || c.Artists != 4 {
		t.Fatalf("counts after removal = %+v", c)
	}
}

// Songs and albums count every removal (the new items are taken first), but
// artists are counted only: a removal offset by an add keeps the count, so
// only a full run sees it, on demand or once FullSyncEvery has passed.
func TestFullSyncDetectsRemovalHiddenFromIncremental(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 100)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	e.fake.mu.Lock()
	e.fake.artists = append(e.fake.artists[:4], artist("r.new", "Newcomer"))
	e.fake.mu.Unlock()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, _, err := e.lib.Artist(ctx, "r.4"); err != nil {
		t.Fatalf("incremental run removed an artist it cannot see: %v", err)
	}

	later := time.Now().Add(FullSyncEvery + time.Hour)
	e.sync.now = func() time.Time { return later }
	e.fake.reset()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if reqs := e.fake.reqs("/v1/me/library/songs"); !slices.Equal(reqs, []int{0}) {
		t.Fatalf("due resync was not full: song pages %v", reqs)
	}
	if _, _, _, err := e.lib.Artist(ctx, "r.4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("full sync kept a removed artist: %v", err)
	}
	if _, _, _, err := e.lib.Artist(ctx, "r.new"); err != nil {
		t.Fatalf("full sync missed the new artist: %v", err)
	}
}

func TestIncrementalPlaylistsRefreshOnlyChanges(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 20)
	e.fake.folders[applemusic.PlaylistsRoot] = []map[string]any{folder("p.f1", "Mixes"), e.fake.playlists[0], e.fake.playlists[1], e.fake.playlists[2]}
	if err := e.run(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	folderReqs := func() (n int) {
		for _, r := range e.fake.seen() {
			if strings.HasPrefix(r, "/v1/me/library/playlist-folders/") {
				n++
			}
		}
		return n
	}
	e.fake.reset()
	if err := e.run(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	if n := folderReqs(); n != 0 {
		t.Fatalf("unchanged playlists refreshed the folder tree (%d requests)", n)
	}

	// A playlist removed elsewhere: dropped, and the tree refreshed.
	e.fake.mu.Lock()
	e.fake.playlists = e.fake.playlists[:2]
	e.fake.folders[applemusic.PlaylistsRoot] = e.fake.folders[applemusic.PlaylistsRoot][:3]
	e.fake.mu.Unlock()
	e.fake.reset()
	if err := e.run(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.lib.Playlist(ctx, "p.small", ListOptions{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed playlist kept: %v", err)
	}
	if folderReqs() == 0 {
		t.Fatal("folder tree not refreshed after a removal")
	}
}

// TestNoChangeResyncLargeLibrary bounds a no-change resync of a 10k-song
// library: well under 5 s.
func TestNoChangeResyncLargeLibrary(t *testing.T) {
	if testing.Short() {
		t.Skip("large library")
	}
	e := newEnv(t)
	seed(e.fake, 10_000)
	start := time.Now()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	full := time.Since(start)
	e.fake.reset()
	start = time.Now()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	t.Logf("10k songs: full sync %v, no-change resync %v (%d requests)", full.Round(time.Millisecond), took.Round(time.Millisecond), len(e.fake.seen()))
	if took > 5*time.Second {
		t.Fatalf("no-change resync took %v", took)
	}
}

func BenchmarkNoChangeResync(b *testing.B) {
	e := newEnv(b)
	seed(e.fake, 10_000)
	if err := e.run(b); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if err := e.run(b); err != nil {
			b.Fatal(err)
		}
	}
}
