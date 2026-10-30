package library

import (
	"context"
	"testing"
)

func TestSearchAllResolvesRowsInMatchOrder(t *testing.T) {
	e := newEnv(t)
	f := e.fake
	f.artists = []map[string]any{artist("r.a", "Blue Band"), artist("r.b", "Other")}
	f.albums = []map[string]any{
		album("l.1", "Blue", "Blue Band", "r.a", "2024-01-02T00:00:00Z"),
		album("l.2", "Red", "Other", "r.b", "2024-03-01T00:00:00Z"),
	}
	f.songs = []map[string]any{
		song("i.1", "Blue Moon", "Other", "Red", "l.2", "r.b"),
		song("i.2", "Blues", "Blue Band", "Blue", "l.1", "r.a"),
		song("i.3", "Green", "Other", "Red", "l.2", "r.b"),
		song("i.4", "Blue", "Blue Band", "Blue", "l.1", "r.a"),
	}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	r, err := e.lib.SearchAll(ctx, "blue", 2)
	if err != nil {
		t.Fatal(err)
	}
	// Three songs match "blue" (title or artist); two fit, so More is set.
	if len(r.Songs) != 2 || !r.SongsMore {
		t.Fatalf("songs = %+v more=%v", r.Songs, r.SongsMore)
	}
	hits, _ := e.lib.Search(ctx, "blue", 2, "song")
	for i, s := range r.Songs {
		if s.LibraryID != hits[i].LibraryID || s.Name == "" || s.AlbumLibraryID == "" {
			t.Fatalf("song %d = %+v, want hit %+v", i, s, hits[i])
		}
	}
	if len(r.Albums) != 1 || r.Albums[0].LibraryID != "l.1" || r.Albums[0].Name != "Blue" || r.AlbumsMore {
		t.Fatalf("albums = %+v", r.Albums)
	}
	if len(r.Artists) != 1 || r.Artists[0].LibraryID != "r.a" || r.ArtistsMore {
		t.Fatalf("artists = %+v", r.Artists)
	}
	if len(r.Playlists) != 0 {
		t.Fatalf("playlists = %+v", r.Playlists)
	}

	empty, err := e.lib.SearchAll(ctx, "  ", 5)
	if err != nil || len(empty.Songs)+len(empty.Albums)+len(empty.Artists)+len(empty.Playlists) != 0 ||
		empty.Songs == nil || empty.Playlists == nil {
		t.Fatalf("blank term = %+v, %v", empty, err)
	}
}
