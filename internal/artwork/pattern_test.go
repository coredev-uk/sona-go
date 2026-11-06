package artwork

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// gen is the cover Apple generated for a playlist called "Sona", in the
// plain gradient design, as applemusic.Artwork.Template gives it.
const gen = "https://" + host + "/image/thumb/gen/{w}x{h}AM.PDCXS11.jpg?c1=F2DE9D&c2=FFE240&c3=B38400&c4=401F00&signature=ab12&t=U29uYQ%3D%3D&tc=FFFFFF&vkey=1"

// Only a playlist cover Apple generated has the designs, and a design
// swaps the design in its path alone, leaving the signed query intact.
func TestWithPatternOnlyForApplesPlaylistCovers(t *testing.T) {
	got, ok := WithPattern(gen, "PDCXS18")
	want := "https://" + host + "/image/thumb/gen/{w}x{h}AM.PDCXS18.jpg?c1=F2DE9D&c2=FFE240&c3=B38400&c4=401F00&signature=ab12&t=U29uYQ%3D%3D&tc=FFFFFF&vkey=1"
	if !ok || got != want {
		t.Fatalf("WithPattern = %q, %v", got, ok)
	}
	if p, ok := Pattern(got); !ok || p != "PDCXS18" {
		t.Errorf("Pattern = %q, %v", p, ok)
	}
	for _, tc := range []struct{ name, tmpl, p string }{
		{"album artwork", tmplFor("album"), "PDCXS12"},
		{"a person's initials", "https://" + host + "/image/thumb/gen/{w}x{h}AM.SCM01.jpg?name=QUI&signature=x", "PDCXS12"},
		{"a design Apple does not draw", gen, "PDCXS10"},
		{"none", "", "PDCXS12"},
		{"a mosaic", Mosaic([]string{tmplFor("a"), tmplFor("b"), tmplFor("c"), tmplFor("d")}), "PDCXS12"},
	} {
		if got, ok := WithPattern(tc.tmpl, tc.p); ok {
			t.Errorf("%s: WithPattern = %q", tc.name, got)
		}
	}
}

// Sign-out removes the cached generated playlist covers in every design:
// each carries a playlist's name, and the designs Sona asked for are in no
// library row.
func TestRemovePatternsDeletesTheirCacheAndNoOtherArtwork(t *testing.T) {
	c := newCache(t, newImageServer(t), "", 0)
	dots, _ := WithPattern(gen, "PDCXS18")
	for _, tm := range []string{gen, dots, red} {
		if _, err := c.Get(context.Background(), tm, 64, 64); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := c.RemovePatterns(); err != nil || n == 0 {
		t.Fatalf("RemovePatterns = %d, %v", n, err)
	}
	for _, tm := range []string{gen, dots} {
		if _, err := os.Stat(filepath.Join(c.dir, keyOf(tm))); err == nil {
			t.Errorf("%s is still cached", tm)
		}
	}
	if _, err := os.Stat(filepath.Join(c.dir, keyOf(red))); err != nil {
		t.Errorf("an album's artwork went too: %v", err)
	}
}
