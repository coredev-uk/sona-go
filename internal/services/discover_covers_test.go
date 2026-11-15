package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
)

// recentlyPlayedShelf is Apple's Recently Played shelf holding the library
// playlists ids, none with artwork (as Apple sends a mosaic playlist).
func recentlyPlayedShelf(t *testing.T, ids ...string) applemusic.MapResponse {
	t.Helper()
	refs, res := []map[string]string{}, map[string]any{}
	for _, id := range ids {
		refs = append(refs, map[string]string{"id": id, "type": "library-playlists"})
		res[id] = map[string]any{"id": id, "type": "library-playlists", "attributes": map[string]any{"name": id}}
	}
	body, err := json.Marshal(map[string]any{
		"data": []map[string]string{{"id": "rp", "type": "personal-recommendation"}},
		"resources": map[string]any{
			"personal-recommendation": map[string]any{"rp": map[string]any{
				"id": "rp", "type": "personal-recommendation",
				"attributes":    map[string]any{"title": map[string]any{"stringForDisplay": "Recently Played"}, "kind": "recently-played", "display": map[string]any{"kind": "MusicCoverShelf"}},
				"relationships": map[string]any{"contents": map[string]any{"data": refs}},
			}},
			"library-playlists": res,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var r applemusic.MapResponse
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRecentlyPlayedShowsLibraryPlaylistsTheirLibraryCover(t *testing.T) {
	r := newCoverRig(t, nil)
	ctx := context.Background()
	disc := &DiscoverService{e: r.e, m: r.e.m}
	mosaic := artwork.Mosaic([]string{albumArt("a"), albumArt("b"), albumArt("c"), albumArt("d")})

	shelfArt := func() map[string]Art {
		secs := disc.homeSections(ctx, recentlyPlayedShelf(t, "p.other", "p.apple", "p.gen", "p.nobody"))
		if len(secs) != 1 {
			t.Fatalf("sections = %+v", secs)
		}
		out := map[string]Art{}
		for _, it := range secs[0].Items {
			out[it.ID] = it.Art
			if it.Playlist.Art != it.Art {
				t.Errorf("%s: the playlist shows %+v, the card %+v", it.ID, it.Playlist.Art, it.Art)
			}
		}
		return out
	}

	got := shelfArt()
	for id, want := range map[string]string{"p.other": mosaic, "p.apple": albumArt("own"), "p.gen": genTemplate("p.gen")} {
		if got[id] != r.e.m.artOf(want) || got[id].Large == "" {
			t.Errorf("%s shows %+v, want %q", id, got[id], want)
		}
	}
	if got["p.nobody"] != (Art{}) {
		t.Errorf("a playlist outside the library shows %+v", got["p.nobody"])
	}

	// The user's choice wins: a design, then none.
	if _, err := r.pl.SetCover(ctx, "p.gen", "PDCXS18"); err != nil {
		t.Fatal(err)
	}
	if got := shelfArt()["p.gen"]; got != r.e.m.artOf(design(t, "p.gen", "PDCXS18")) {
		t.Errorf("a chosen design: %+v", got)
	}
	if _, err := r.pl.SetCover(ctx, "p.other", CoverNone); err != nil {
		t.Fatal(err)
	}
	if got := shelfArt()["p.other"]; got != (Art{}) {
		t.Errorf("a cover set to none: %+v", got)
	}
}
