package playlist

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

func fixtureArt(n int) services.Art {
	u := strings.NewReplacer("{w}", "96", "{h}", "96").Replace(fixture.Template(n))
	return services.Art{Small: u, Medium: u, Large: u}
}

// coverBounds is the box, in DIPs, of the cover's saturated pixels near r:
// where the artwork really is, found in the picture and not in the layout.
func coverBounds(im *image.RGBA, sc float32, r ui.Rect) ui.Rect {
	x0, y0 := int((r.X-6)*sc), int((r.Y-6)*sc)
	x1, y1 := int((r.X+r.W+6)*sc), int((r.Y+r.H+6)*sc)
	minx, miny, maxx, maxy := math.MaxInt, math.MaxInt, -1, -1
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c := im.RGBAAt(x, y)
			if max(c.R, c.G, c.B)-min(c.R, c.G, c.B) > 60 {
				minx, miny, maxx, maxy = min(minx, x), min(miny, y), max(maxx, x), max(maxy, y)
			}
		}
	}
	return ui.Rect{X: float32(minx) / sc, Y: float32(miny) / sc, W: float32(maxx+1-minx) / sc, H: float32(maxy+1-miny) / sc}
}

// glyphCentres are the middle of the dark triangle's box and its centre of
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
// and one inside a folder, at device scales 1 and 2), and its triangle is
// optically centred: the middle of its box within half a pixel to the right
// of the button's, and its mass within half a pixel of it either way.
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
			art := coverBounds(rest, sc, btn)
			if art.W == 0 || math.Abs(float64(art.X-btn.X)) > 0.01 || math.Abs(float64(art.Y-btn.Y)) > 0.01 ||
				art.W != btn.W || art.H != btn.H {
				t.Errorf("scale %v, %s: button %v, artwork %v", sc, name, btn, art)
			}

			e.hover(name)
			e.tt.Settle()
			box, mass := glyphCentres(e.tt.Image(), sc, btn)
			cx, cy := float64(btn.X+btn.W/2), float64(btn.Y+btn.H/2)
			if d := box[0] - cx; d < -0.01 || d > 0.51 || math.Abs(box[1]-cy) > 0.01 {
				t.Errorf("scale %v, %s: the triangle's box is off the button's middle by (%.2f, %.2f)", sc, name, d, box[1]-cy)
			}
			if dx, dy := mass[0]-cx, mass[1]-cy; math.Abs(dx) > 0.6 || math.Abs(dy) > 0.1 {
				t.Errorf("scale %v, %s: the triangle's mass is off the button's middle by (%.2f, %.2f)", sc, name, dx, dy)
			}
		}
	}
}
