package services

import (
	"context"
	"testing"

	"github.com/coredev-uk/sona-go/internal/artwork"
)

func TestRecentSearchesShowLibraryPlaylistsTheirLibraryCover(t *testing.T) {
	r := newCoverRig(t, nil)
	ctx := context.Background()
	s := &SearchService{e: r.e, m: r.e.m}
	mosaic := artwork.Mosaic([]string{albumArt("a"), albumArt("b"), albumArt("c"), albumArt("d")})

	// Remembered with no artwork, as a mosaic playlist shows in the result.
	for _, id := range []string{"p.other", "p.gen"} {
		if err := s.RememberItem(ctx, RecentItem{Kind: ItemPlaylist, ID: id, Source: SourceLibrary, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	// A catalogue playlist keeps what it was remembered with.
	if err := s.RememberItem(ctx, RecentItem{Kind: ItemPlaylist, ID: "pl.x", Source: SourceCatalog, Name: "x", Art: "/art/k/96.jpg"}); err != nil {
		t.Fatal(err)
	}
	art := func() map[string]string {
		rows, err := s.Recent(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, row := range rows {
			out[row.ItemID] = row.Art
		}
		return out
	}

	got := art()
	if want := r.e.m.artOf(mosaic).Small; got["p.other"] != want || want == "" {
		t.Errorf("the mosaic playlist shows %q, want %q", got["p.other"], want)
	}
	if want := r.e.m.artOf(genTemplate("p.gen")).Small; got["p.gen"] != want {
		t.Errorf("Apple's cover: %q, want %q", got["p.gen"], want)
	}
	if got["pl.x"] != "/art/k/96.jpg" {
		t.Errorf("a catalogue playlist shows %q", got["pl.x"])
	}

	// After the user changes the cover, the list shows the new one.
	if _, err := r.pl.SetCover(ctx, "p.gen", "PDCXS18"); err != nil {
		t.Fatal(err)
	}
	if want := r.e.m.artOf(design(t, "p.gen", "PDCXS18")).Small; art()["p.gen"] != want {
		t.Errorf("a chosen design: %q, want %q", art()["p.gen"], want)
	}
	if _, err := r.pl.SetCover(ctx, "p.other", CoverNone); err != nil {
		t.Fatal(err)
	}
	if got := art()["p.other"]; got != "" {
		t.Errorf("none: %q", got)
	}
}
