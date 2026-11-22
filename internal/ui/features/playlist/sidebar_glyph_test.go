package playlist

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/overlay"
)

func fixtureArt(n int) services.Art {
	u := strings.NewReplacer("{w}", "96", "{h}", "96").Replace(fixture.Template(n))
	return services.Art{Small: u, Medium: u, Large: u}
}

// glyphCentres are the middle of the dark glyph's box and its centre of
// mass inside r, in DIPs.
func glyphCentres(im *image.RGBA, sc float32, r ui.Rect) (box, mass [2]float64) {
	x0, y0 := int(r.X*sc), int(r.Y*sc)
	x1, y1 := int((r.X+r.W)*sc), int((r.Y+r.H)*sc)
	var sw, sx, sy float64
	minx, miny, maxx, maxy := math.MaxInt, math.MaxInt, -1, -1
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c := im.RGBAAt(x, y)
			l := (int(c.R) + int(c.G) + int(c.B)) / 3
			if l < 128 {
				w := float64(255-l) / 255
				sw, sx, sy = sw+w, sx+w*(float64(x)+.5), sy+w*(float64(y)+.5)
				minx, miny, maxx, maxy = min(minx, x), min(miny, y), max(maxx, x), max(maxy, y)
			}
		}
	}
	s := float64(sc)
	return [2]float64{float64(minx+maxx+1) / 2 / s, float64(miny+maxy+1) / 2 / s}, [2]float64{sx / sw / s, sy / sw / s}
}

// The shuffle button lies exactly on the artwork (a pinned row, a plain one
// and one inside a folder, at device scales 1 and 2), and its shuffle glyph
// is centred: the middle of its box within 0.6px of the button's, and its
// mass (which the arrowheads pull right) within 1.6px of it. The glyph is
// read in Lucide's shuffle, which Linux and Windows draw: macOS draws an SF
// Symbol, and there the test stops at the button.
func TestShuffleButtonLiesOnTheArtworkAndItsGlyphIsCentred(t *testing.T) {
	goldenFont(t)
	tree := sbLibrary()
	for i := range tree.Playlists {
		tree.Playlists[i].Playlist.Art = fixtureArt(i + 1)
	}
	pins := []services.Pin{{Kind: "playlist", ID: "p.a", Name: "Alpha", Action: "drillIn", Art: fixtureArt(7)}}
	e := sbSetupWith(t, tree, pins, nil)
	theme.SetMode(theme.ModeLight)
	e.shows("Alpha", "Road Trip", "Running")
	if err := e.tt.Click("Running"); err != nil {
		t.Fatal(err)
	}
	e.shows("Tempo")
	e.tt.Settle()

	for _, sc := range []float32{2, 1} {
		e.tt.SetScale(sc)
		e.tt.Settle()
		for _, name := range []string{"Alpha", "Road Trip", "Tempo"} {
			e.tt.Move(1000, 800)
			e.tt.Frame()
			e.tt.Settle()
			rest := e.tt.Image()
			btn, ok := e.tt.Find("Shuffle " + name)
			if !ok {
				t.Fatalf("no button for %s", name)
			}
			overlay.OnArtwork(t, fmt.Sprintf("scale %v, %s: the shuffle button", sc, name), rest, sc, btn)
			overlay.WholePixels(t, fmt.Sprintf("scale %v, %s: the shuffle button", sc, name), btn, sc)
			if runtime.GOOS != "linux" {
				continue // the glyph itself is read below, on Linux only
			}

			e.hover(name)
			e.tt.Settle()
			box, mass := glyphCentres(e.tt.Image(), sc, btn)
			cx, cy := float64(btn.X+btn.W/2), float64(btn.Y+btn.H/2)
			if dx, dy := box[0]-cx, box[1]-cy; math.Abs(dx) > 0.6 || math.Abs(dy) > 0.6 {
				t.Errorf("scale %v, %s: the glyph's box is off the button's middle by (%.2f, %.2f)", sc, name, dx, dy)
			}
			// The arrowheads sit at its right, so its mass lies to the right of its box's middle.
			if dx, dy := mass[0]-cx, mass[1]-cy; dx < 0 || dx > 1.6 || math.Abs(dy) > 0.6 {
				t.Errorf("scale %v, %s: the glyph's mass is off the button's middle by (%.2f, %.2f)", sc, name, dx, dy)
			}
		}
	}
	if runtime.GOOS != "linux" {
		t.Skip("pixel readings are asserted on Linux only: macOS draws the SF Symbol shuffle, whose shape the system sets (docs/design.md, \"Icons\")")
	}
}
