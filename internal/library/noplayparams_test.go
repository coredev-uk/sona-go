package library

import (
	"context"
	"testing"
)

// A song with no playParams is stored as such and read back as such: the
// release rule keeps a dated one, or one on an album that is out, listed as
// out, and Song.NoPlayParams lets a caller tell it from one Apple plays.
func TestSongReadsBackNoPlayParams(t *testing.T) {
	e := newEnv(t)
	f := e.fake
	f.artists = []map[string]any{artist("r.a", "Various")}
	f.albums = []map[string]any{album("l.old", "Old", "Various", "r.a", "2024-01-02T00:00:00Z")}
	plays := song("i.plays", "Plays", "Various", "Old", "l.old", "r.a")
	withheld := song("i.dated", "Dated", "Various", "Old", "l.old", "r.a")
	delete(withheld["attributes"].(map[string]any), "playParams")
	withheld["attributes"].(map[string]any)["releaseDate"] = "2020-07-24"
	undated := unreleasedSong("i.undated", "Undated", "Various", "l.old", "r.a")
	f.songs = []map[string]any{plays, withheld, undated}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, tc := range []struct {
		id         string
		noParams   bool
		catalogID  string
		unreleased bool
	}{
		{"i.plays", false, "cplays", false},
		{"i.dated", true, "", false},
		{"i.undated", true, "", false}, // its album is out, so it is out
	} {
		s, err := e.lib.Song(ctx, tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if s.NoPlayParams != tc.noParams || s.CatalogID != tc.catalogID || s.Unreleased != tc.unreleased {
			t.Errorf("%s: NoPlayParams %v, CatalogID %q, Unreleased %v; want %v, %q, %v",
				tc.id, s.NoPlayParams, s.CatalogID, s.Unreleased, tc.noParams, tc.catalogID, tc.unreleased)
		}
	}
}
