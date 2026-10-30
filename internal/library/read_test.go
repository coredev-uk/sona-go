package library

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestListSorting(t *testing.T) {
	e := newEnv(t)
	f := e.fake
	f.artists = []map[string]any{artist("r.a", "Zed"), artist("r.b", "abba")}
	f.albums = []map[string]any{
		album("l.1", "beta", "Zed", "r.a", "2024-01-02T00:00:00Z"),
		album("l.2", "Alpha", "abba", "r.b", "2024-03-01T00:00:00Z"),
	}
	f.songs = []map[string]any{ // newest first
		song("i.1", "charlie", "Zed", "beta", "l.1", "r.a"),
		song("i.2", "Bravo", "abba", "Alpha", "l.2", "r.b"),
		song("i.3", "alpha", "Zed", "Alpha", "l.2", "r.a"),
	}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ids := func(s []Song) (out string) {
		for _, x := range s {
			out += x.LibraryID + " "
		}
		return out
	}
	for _, tc := range []struct {
		o    ListOptions
		want string
	}{
		{ListOptions{}, "i.3 i.2 i.1 "}, // title, case-insensitive
		{ListOptions{Desc: true}, "i.1 i.2 i.3 "},
		{ListOptions{Sort: SortArtist}, "i.2 i.3 i.1 "},
		{ListOptions{Sort: SortAlbum}, "i.2 i.3 i.1 "},
		{ListOptions{Sort: SortDateAdded, Desc: true}, "i.1 i.2 i.3 "},
		{ListOptions{Sort: SortDateAdded}, "i.3 i.2 i.1 "},
		{ListOptions{Limit: 1, Offset: 1}, "i.2 "},
	} {
		got, err := e.lib.Songs(ctx, tc.o)
		if err != nil || ids(got) != tc.want {
			t.Errorf("Songs(%+v) = %q, %v; want %q", tc.o, ids(got), err, tc.want)
		}
	}
	albums, _ := e.lib.Albums(ctx, ListOptions{Sort: SortDateAdded, Desc: true})
	if len(albums) != 2 || albums[0].LibraryID != "l.2" {
		t.Fatalf("albums by date = %+v", albums)
	}
	albums, _ = e.lib.Albums(ctx, ListOptions{})
	if albums[0].Name != "Alpha" {
		t.Fatalf("albums by title = %+v", albums)
	}
	artists, _ := e.lib.Artists(ctx, ListOptions{})
	if len(artists) != 2 || artists[0].Name != "abba" {
		t.Fatalf("artists = %+v", artists)
	}
	if hits, _ := e.lib.Search(ctx, `"*) OR`, 5); len(hits) != 0 {
		t.Fatalf("FTS syntax not neutralised: %+v", hits)
	}
}

// TestLargeLibraryListsFast syncs a 12k-song library through the real sync
// path and checks that the UI's list, count and search queries stay fast.
func TestLargeLibraryListsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("large library")
	}
	e := newEnv(t)
	seed(e.fake, 12000)
	start := time.Now()
	if err := e.run(t, DomainArtists, DomainAlbums, DomainSongs); err != nil {
		t.Fatal(err)
	}
	t.Logf("synced 12000 songs, 1200 albums in %v", time.Since(start).Round(time.Millisecond))
	ctx := context.Background()
	const budget = 50 * time.Millisecond
	check := func(name string, f func() (int, error), want int) {
		t.Helper()
		f() // warm the read connection
		start := time.Now()
		n, err := f()
		took := time.Since(start)
		if err != nil || n != want {
			t.Fatalf("%s: %d rows, %v", name, n, err)
		}
		t.Logf("%-28s %v", name, took)
		if took > budget {
			t.Errorf("%s took %v (budget %v)", name, took, budget)
		}
	}
	for _, s := range []Sort{SortTitle, SortArtist, SortAlbum, SortDateAdded} {
		for _, off := range []int{0, 11900} {
			check("songs "+string(s)+" @"+strconv.Itoa(off), func() (int, error) {
				r, err := e.lib.Songs(ctx, ListOptions{Sort: s, Offset: off, Limit: 100})
				return len(r), err
			}, 100)
		}
	}
	check("albums title", func() (int, error) {
		r, err := e.lib.Albums(ctx, ListOptions{Limit: 200})
		return len(r), err
	}, 200)
	check("counts", func() (int, error) {
		c, err := e.lib.Counts(ctx)
		return c.Songs, err
	}, 12000)
	check("search prefix", func() (int, error) {
		r, err := e.lib.Search(ctx, "son 1199", 50)
		return len(r), err
	}, 11)
	check("album detail", func() (int, error) {
		_, r, err := e.lib.Album(ctx, "l.500")
		return len(r), err
	}, 10)
}

func benchEnv(b *testing.B, n int) *env {
	b.Helper()
	e := newEnv(b)
	seed(e.fake, n)
	if err := e.run(b, DomainArtists, DomainAlbums, DomainSongs); err != nil {
		b.Fatal(err)
	}
	return e
}

func BenchmarkSongsPage(b *testing.B) {
	e := benchEnv(b, 10000)
	ctx := context.Background()
	for _, s := range []Sort{SortTitle, SortArtist, SortDateAdded} {
		b.Run(string(s), func(b *testing.B) {
			for i := 0; b.Loop(); i++ {
				if _, err := e.lib.Songs(ctx, ListOptions{Sort: s, Offset: (i * 100) % 10000, Limit: 100}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSearch(b *testing.B) {
	e := benchEnv(b, 10000)
	ctx := context.Background()
	for b.Loop() {
		if _, err := e.lib.Search(ctx, "son 12", 50); err != nil {
			b.Fatal(err)
		}
	}
}
