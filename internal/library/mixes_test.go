package library

import (
	"context"
	"slices"
	"testing"
)

// withCatalog gives library playlist pl a catalog copy catID whose
// lastModifiedDate is modified, as include=catalog does.
func withCatalog(pl map[string]any, catID, modified string) map[string]any {
	pl["attributes"].(map[string]any)["playParams"].(map[string]any)["globalId"] = catID
	pl["relationships"] = map[string]any{"catalog": map[string]any{"data": []any{map[string]any{
		"id": catID, "type": "playlists", "attributes": map[string]any{"lastModifiedDate": modified},
	}}}}
	return pl
}

func trackIDs(songs []Song) []string {
	out := make([]string, len(songs))
	for i, s := range songs {
		out[i] = s.LibraryID
	}
	return out
}

// Apple regenerates a personal mix in its catalog copy and leaves the
// library copy's lastModifiedDate where it was: the mix's freshness is its
// catalog stamp, for the stored stamp and for refetching its tracks. Any
// other playlist keeps the library copy's stamp.
func TestSyncPersonalMixTakesTheCatalogStamp(t *testing.T) {
	const (
		libStamp  = "2019-04-12T22:52:43Z"
		week1     = "2026-11-06T02:00:00Z"
		week2     = "2026-11-13T02:00:00Z"
		otherCat1 = "2026-01-01T00:00:00Z"
		otherCat2 = "2026-11-01T00:00:00Z"
	)
	e := newEnv(t)
	f := e.fake
	f.songs = []map[string]any{
		song("i.1", "One", "Zed", "Beta", "l.1", "r.a"),
		song("i.2", "Two", "Abba", "Alpha", "l.2", "r.b"),
		song("i.3", "Three", "Zed", "Beta", "l.1", "r.a"),
	}
	f.playlists = []map[string]any{
		withCatalog(playlist("p.mix", "New Music Mix", libStamp), "pl.pm-1", week1),
		// Not a mix: its catalog copy moves, its library copy does not.
		withCatalog(playlist("p.apple", "Apple's Picks", libStamp), "pl.apple", otherCat1),
	}
	f.plTracks["p.mix"] = []map[string]any{f.songs[0], f.songs[1]}
	f.plTracks["p.apple"] = []map[string]any{f.songs[0]}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p, songs, err := e.lib.AllPlaylist(ctx, "p.mix")
	if err != nil || p.LastModified != week1 || !slices.Equal(trackIDs(songs), []string{"i.1", "i.2"}) {
		t.Fatalf("first sync: mix = %q, %v, %v", p.LastModified, trackIDs(songs), err)
	}

	// The weekly refresh: new songs and a new catalog stamp, the library stamp unchanged.
	f.mu.Lock()
	f.playlists = []map[string]any{
		withCatalog(playlist("p.mix", "New Music Mix", libStamp), "pl.pm-1", week2),
		withCatalog(playlist("p.apple", "Apple's Picks", libStamp), "pl.apple", otherCat2),
	}
	f.plTracks["p.mix"] = []map[string]any{f.songs[2], f.songs[1], f.songs[0]}
	f.plTracks["p.apple"] = []map[string]any{f.songs[2]}
	f.mu.Unlock()
	f.reset()
	if err := e.run(t, DomainPlaylists, DomainPlaylistTracks); err != nil {
		t.Fatal(err)
	}
	p, songs, err = e.lib.AllPlaylist(ctx, "p.mix")
	if err != nil || p.LastModified != week2 || !slices.Equal(trackIDs(songs), []string{"i.3", "i.2", "i.1"}) {
		t.Fatalf("mix after its refresh: Updated %q, tracks %v, %v", p.LastModified, trackIDs(songs), err)
	}
	if n := len(f.reqs("/v1/me/library/playlists/p.mix/tracks")); n == 0 {
		t.Fatal("a mix whose catalog stamp moved was not refetched")
	}
	if n := len(f.reqs("/v1/me/library/playlists/p.apple/tracks")); n != 0 {
		t.Fatalf("a playlist whose library stamp is unchanged was refetched %d times", n)
	}
	p, songs, err = e.lib.AllPlaylist(ctx, "p.apple")
	if err != nil || p.LastModified != libStamp || !slices.Equal(trackIDs(songs), []string{"i.1"}) {
		t.Fatalf("non-mix playlist: stamp %q, tracks %v, %v", p.LastModified, trackIDs(songs), err)
	}

	// Nothing moved: a mix is not refetched.
	f.reset()
	if err := e.run(t, DomainPlaylists, DomainPlaylistTracks); err != nil {
		t.Fatal(err)
	}
	if n := len(f.reqs("/v1/me/library/playlists/p.mix/tracks")); n != 0 {
		t.Fatalf("an unchanged mix was refetched %d times", n)
	}
}
