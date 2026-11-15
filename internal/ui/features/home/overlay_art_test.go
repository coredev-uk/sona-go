package home

import (
	"strconv"
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/overlay"
)

// A row's play wash covers exactly its cover; a tile's play disc is centred
// on its cover, on whole pixels: at 1x and 2x, over the fixture's generated
// covers (saturated gradients, so the picture shows where the cover is).
func TestRowOverlaysLieOnTheirCover(t *testing.T) {
	for _, tile := range []bool{false, true} {
		for _, sc := range []float32{1, 2} {
			name := "row"
			if tile {
				name = "tile"
			}
			t.Run(name+" "+strconv.Itoa(int(sc))+"x", func(t *testing.T) {
				r := newRowsRig(t, moodStations(), rowsOpts{Label: "Find Your Mood", Rows: 3, Min: 280, Tile: tile}, 640, 1000)
				r.tt.SetScale(sc)
				r.tt.Settle()
				im := r.tt.Image()
				for _, n := range []string{"Energy", "Focus", "Relax"} {
					over, ok := r.tt.Find("Play " + n)
					if !ok {
						t.Fatalf("scale %v: no play control on %s: %q", sc, n, r.tt.Texts())
					}
					entry, _ := r.tt.Find(n)
					art, ok := overlay.ArtBounds(im, sc, entry)
					if !ok {
						t.Fatalf("scale %v: no cover drawn in %s's entry %v", sc, n, entry)
					}
					if tile {
						overlay.Inside(t, n+"'s play disc", over, art)
						overlay.Centred(t, n+"'s play disc", over, art, 0)
						overlay.WholePixels(t, n+"'s play disc", over, sc)
					} else {
						overlay.Same(t, n+"'s play wash", over, art)
						overlay.WholePixels(t, n+"'s play wash", over, sc)
					}
				}
			})
		}
	}
}
