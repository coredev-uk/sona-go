package library

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// seed fills the fake with n songs over n/10 albums and 5 artists, three
// playlists and a few ratings. Song i is "Song <i>"; songs[0] is the newest.
func seed(f *fakeApple, n int) {
	for i := range 5 {
		f.artists = append(f.artists, artist(fmt.Sprintf("r.%d", i), fmt.Sprintf("Artist %d", i)))
	}
	for i := range max(1, n/10) {
		f.albums = append(f.albums, album(fmt.Sprintf("l.%d", i), fmt.Sprintf("Album %d", i), fmt.Sprintf("Artist %d", i%5), fmt.Sprintf("r.%d", i%5),
			time.Date(2024, 1, 1+i%28, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)))
	}
	for i := range n {
		a := i / 10
		f.songs = append(f.songs, song(fmt.Sprintf("i.%d", i), fmt.Sprintf("Song %d", i), fmt.Sprintf("Artist %d", a%5),
			fmt.Sprintf("Album %d", a), fmt.Sprintf("l.%d", a), fmt.Sprintf("r.%d", a%5)))
	}
	f.playlists = []map[string]any{
		playlist("p.big", "Big Mix", "2024-05-01T00:00:00Z"),
		playlist("p.empty", "Empty", "2024-05-01T00:00:00Z"),
		playlist("p.small", "Café Songs", "2024-05-01T00:00:00Z"),
	}
	var big []map[string]any
	for i := range 120 {
		big = append(big, f.songs[i%len(f.songs)])
	}
	// A playlist track that is not in the library songs.
	big = append(big, song("i.extra", "Extra Track", "Guest", "Elsewhere", "l.x", "r.x"))
	f.plTracks["p.big"] = big
	f.plTracks["p.small"] = []map[string]any{f.songs[1], f.songs[0]}
	f.ratings["library-songs"] = map[string]int{"i.5": 1, "i.7": -1}
	f.ratings["library-albums"] = map[string]int{"l.2": 1}
}

func TestSyncFull(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 250)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, err := e.lib.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c != (Counts{Songs: 250, Albums: 25, Artists: 5, Playlists: 3}) {
		t.Fatalf("counts = %+v", c)
	}

	// Every domain finished, in dependency order, with page progress before.
	var finished []Domain
	for _, p := range e.steps {
		if p.Finished {
			if p.Err != nil {
				t.Fatalf("%s: %v", p.Domain, p.Err)
			}
			finished = append(finished, p.Domain)
		}
	}
	if !slices.Equal(finished, AllDomains) {
		t.Fatalf("finished = %v", finished)
	}
	if !slices.ContainsFunc(e.steps, func(p Progress) bool { return p.Domain == DomainSongs && p.Done == 100 && p.Total == 250 }) {
		t.Fatalf("no per-page songs progress: %+v", e.steps)
	}

	// Date added: newest first is Apple's order.
	songs, err := e.lib.Songs(ctx, ListOptions{Sort: SortDateAdded, Desc: true, Limit: 3})
	if err != nil || len(songs) != 3 || songs[0].LibraryID != "i.0" || songs[2].LibraryID != "i.2" {
		t.Fatalf("newest songs = %+v, %v", songs, err)
	}
	s := songs[0]
	if s.CatalogID != "c0" || s.AlbumLibraryID != "l.0" || s.ArtistLibraryID != "r.0" || s.ArtworkTemplate != "https://art/i.0/{w}x{h}bb.jpg" || s.DurationMS != 200000 {
		t.Fatalf("song fields = %+v", s)
	}

	alb, tracks, err := e.lib.Album(ctx, "l.3")
	if err != nil || alb.Name != "Album 3" || alb.CatalogID != "cl.3" || alb.ArtistLibraryID != "r.3" || len(tracks) != 10 || alb.DateAdded.IsZero() {
		t.Fatalf("album = %+v, %d tracks, %v", alb, len(tracks), err)
	}
	art, albums, artSongs, err := e.lib.Artist(ctx, "r.1")
	if err != nil || art.ArtworkTemplate == "" || art.CatalogID != "cr.1" || len(albums) != 5 || len(artSongs) != 50 {
		t.Fatalf("artist = %+v, %d albums, %d songs, %v", art, len(albums), len(artSongs), err)
	}

	pl, plSongs, err := e.lib.Playlist(ctx, "p.big", ListOptions{Limit: 500})
	if err != nil || pl.CatalogID != "pl.u-p.big" || pl.TrackCount != 121 || len(plSongs) != 121 || plSongs[120].LibraryID != "i.extra" {
		t.Fatalf("playlist = %+v, %d songs, %v", pl, len(plSongs), err)
	}
	if _, empty, err := e.lib.Playlist(ctx, "p.empty", ListOptions{}); err != nil || len(empty) != 0 {
		t.Fatalf("empty playlist: %d, %v", len(empty), err)
	}
	// The playlist-only track is stored but not a library song.
	if c, _ := e.lib.Counts(ctx); c.Songs != 250 {
		t.Fatalf("songs = %d after playlist tracks", c.Songs)
	}

	if v, _ := e.lib.Rating(ctx, applemusic.TypeLibrarySongs, "i.5"); v != applemusic.RatingFavorite {
		t.Fatalf("rating i.5 = %d", v)
	}
	if v, _ := e.lib.Rating(ctx, applemusic.TypeLibrarySongs, "i.7"); v != applemusic.RatingSuggestLess {
		t.Fatalf("rating i.7 = %d", v)
	}
	if v, _ := e.lib.Rating(ctx, applemusic.TypeLibraryAlbums, "l.2"); v != applemusic.RatingFavorite {
		t.Fatalf("album rating l.2 = %d", v)
	}

	hits, err := e.lib.Search(ctx, "cafe", 10)
	if err != nil || len(hits) != 1 || hits[0].Kind != "playlist" || hits[0].LibraryID != "p.small" {
		t.Fatalf("search cafe = %+v, %v", hits, err)
	}
	hits, err = e.lib.Search(ctx, "song 24", 20, "song")
	if err != nil || len(hits) == 0 {
		t.Fatalf("search song 24 = %+v, %v", hits, err)
	}
	for _, h := range hits {
		if h.Kind != "song" {
			t.Fatalf("kind filter leaked %+v", h)
		}
	}
	if !slices.ContainsFunc(hits, func(h Hit) bool { return h.Name == "Song 24" }) ||
		!slices.ContainsFunc(hits, func(h Hit) bool { return h.Name == "Song 240" }) {
		t.Fatalf("prefix search missed: %+v", hits)
	}

	st, err := e.lib.SyncStatus(ctx)
	if err != nil || len(st) != len(AllDomains) {
		t.Fatalf("status = %+v, %v", st, err)
	}
	for _, s := range st {
		if s.InProgress || s.LastSuccess.IsZero() || s.LastError != "" {
			t.Fatalf("status %+v", s)
		}
	}
}

func TestSyncRemovesDeletedItems(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 100)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	e.fake.mu.Lock()
	e.fake.songs = e.fake.songs[:60]
	e.fake.albums = e.fake.albums[:6]
	e.fake.playlists = e.fake.playlists[1:] // drop p.big
	delete(e.fake.ratings["library-songs"], "i.5")
	e.fake.mu.Unlock()
	if err := e.runFull(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, _ := e.lib.Counts(ctx)
	if c != (Counts{Songs: 60, Albums: 6, Artists: 5, Playlists: 2}) {
		t.Fatalf("counts = %+v", c)
	}
	if _, err := e.lib.Song(ctx, "i.99"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed song still stored: %v", err)
	}
	if _, err := e.lib.Song(ctx, "i.extra"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("track only referenced by a removed playlist survived: %v", err)
	}
	if _, err := e.lib.Song(ctx, "i.1"); err != nil {
		t.Fatalf("song in a remaining playlist lost: %v", err)
	}
	if v, _ := e.lib.Rating(ctx, applemusic.TypeLibrarySongs, "i.5"); v != 0 {
		t.Fatalf("cleared rating kept: %d", v)
	}
	if hits, _ := e.lib.Search(ctx, "Song 99", 5); slices.ContainsFunc(hits, func(h Hit) bool { return h.LibraryID == "i.99" }) {
		t.Fatalf("search index kept removed song: %+v", hits)
	}
	var orphans int
	e.db.R.QueryRow(`SELECT count(*) FROM tracks WHERE library_id IS NOT NULL`).Scan(&orphans)
	if orphans != 60 {
		t.Fatalf("library track rows = %d, want 60", orphans)
	}
}

func TestSyncResumesFromCheckpoint(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 1000)
	e.fake.failPath, e.fake.failOffset = "/v1/me/library/songs", 500
	err := e.run(t, DomainSongs)
	if err == nil {
		t.Fatal("want injected failure")
	}
	ctx := context.Background()
	st, _ := e.db.RQ.GetSyncState(ctx, string(DomainSongs))
	// Pages before the failing one may have been cancelled in flight, so the
	// checkpoint is any page boundary up to it; everything before it is stored.
	cp := int(st.NextOffset)
	if st.InProgress != 1 || cp == 0 || cp > 500 || cp%100 != 0 || st.Total != 1000 || st.LastError == "" {
		t.Fatalf("checkpoint = %+v", st)
	}
	if !slices.ContainsFunc(e.steps, func(p Progress) bool { return p.Finished && p.Err != nil }) {
		t.Fatal("failure not reported through progress")
	}
	if c, _ := e.lib.Counts(ctx); c.Songs != cp {
		t.Fatalf("stored before failure = %d, checkpoint %d", c.Songs, cp)
	}

	e.fake.mu.Lock()
	e.fake.failPath = ""
	e.fake.pageReqs = map[string][]int{}
	e.fake.mu.Unlock()
	if err := e.run(t, DomainSongs); err != nil {
		t.Fatal(err)
	}
	if reqs := e.fake.reqs("/v1/me/library/songs"); slices.Min(reqs) != cp || len(reqs) != (1000-cp)/100 {
		t.Fatalf("resumed page requests = %v, want offsets %d..900", reqs, cp)
	}
	if c, _ := e.lib.Counts(ctx); c.Songs != 1000 {
		t.Fatalf("songs after resume = %d", c.Songs)
	}
	st, _ = e.db.RQ.GetSyncState(ctx, string(DomainSongs))
	if st.InProgress != 0 || !st.LastSuccess.Valid || st.LastError != "" {
		t.Fatalf("state after resume = %+v", st)
	}
	// The next full run starts fresh from offset 0.
	e.fake.reset()
	if err := e.runFull(t, DomainSongs); err != nil {
		t.Fatal(err)
	}
	if reqs := e.fake.reqs("/v1/me/library/songs"); len(reqs) != 10 || slices.Min(reqs) != 0 {
		t.Fatalf("fresh run requests = %v", reqs)
	}
}

func TestSyncSkipsUnchangedPlaylists(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 50)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	e.fake.mu.Lock()
	e.fake.pageReqs = map[string][]int{}
	e.fake.playlists[2] = playlist("p.small", "Café Songs", "2024-06-01T00:00:00Z")
	e.fake.plTracks["p.small"] = []map[string]any{e.fake.songs[3]}
	e.fake.mu.Unlock()
	if err := e.run(t, DomainPlaylists, DomainPlaylistTracks); err != nil {
		t.Fatal(err)
	}
	if n := len(e.fake.reqs("/v1/me/library/playlists/p.big/tracks")); n != 0 {
		t.Fatalf("unchanged playlist refetched %d times", n)
	}
	_, songs, err := e.lib.Playlist(context.Background(), "p.small", ListOptions{})
	if err != nil || len(songs) != 1 || songs[0].LibraryID != "i.3" {
		t.Fatalf("changed playlist = %+v, %v", songs, err)
	}
}

func TestSyncRejectsConcurrentRun(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 10)
	e.fake.block = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- e.sync.Sync(context.Background(), Full, nil, DomainSongs) }()
	for !e.sync.Running() {
		time.Sleep(time.Millisecond)
	}
	if err := e.sync.Sync(context.Background(), Incremental, nil); !errors.Is(err, ErrSyncRunning) {
		t.Fatalf("second Sync = %v", err)
	}
	close(e.fake.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSyncStopsOnUnauthorized(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 10)
	e.fake.forbid = true
	err := e.run(t)
	if !errors.Is(err, applemusic.ErrUnauthorized) {
		t.Fatalf("err = %v", err)
	}
	var finished []Domain
	for _, p := range e.steps {
		if p.Finished {
			finished = append(finished, p.Domain)
		}
	}
	if !slices.Equal(finished, []Domain{DomainArtists}) {
		t.Fatalf("domains attempted after an auth failure: %v", finished)
	}
}
