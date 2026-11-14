package album

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/theme/palette"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// maxStep is the biggest change in any channel between two pixels one under
// the other that still is a gradient: a smootherstep over the shortest
// dissolve has a slope under 5 of 255 a pixel, an edge is a jump of tens.
const maxStep = 8

// column is the page's left edge column, which nothing is drawn on but the
// band, as pixels from the top.
func column(img *image.RGBA, to int) []color.RGBA {
	out := make([]color.RGBA, 0, to)
	for y := range to {
		out = append(out, img.RGBAAt(4, y))
	}
	return out
}

// step is the biggest difference in a channel of two pixels.
func step(a, b color.RGBA) int {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return max(d(a.R, b.R), d(a.G, b.G), d(a.B, b.B))
}

// bandEnd is a row under the band's dissolve and above the songs' header.
func (r *rig) bandEnd() int {
	r.t.Helper()
	song, ok := r.tt.Find("Low Tide")
	if !ok {
		r.t.Fatalf("no song on screen: %q", r.tt.Texts())
	}
	return int(song.Y) - 60
}

func TestBandDissolvesIntoTheGroundWithoutAnEdge(t *testing.T) {
	for _, c := range []struct {
		name string
		w, h int
		dark bool
	}{
		{"wide", 1132, 900, false},
		{"wide dark", 1132, 900, true},
		{"short pane", 1132, 480, false},
		{"narrow", 700, 900, false},
		{"narrow dark", 700, 900, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var opts []uitest.Option
			th := palette.Light
			if c.dark {
				opts, th = []uitest.Option{uitest.Dark()}, palette.Dark
			}
			r := newRig(t, "/album/100", c.w, c.h, opts...)
			r.addCatalog(saltFlats(coloured("#c4793a")))
			r.shows("Quiet Records")
			r.tt.Settle()
			end := r.bandEnd()
			px := column(r.tt.Image(), end+1)
			if want := hexRGBA(t, palette.SleeveSurface("#c4793a", th)); !closeTo(px[4], want) {
				t.Errorf("the band starts as %v, want the cover's colour %v", px[4], want)
			}
			if want := hexRGBA(t, palette.Of(th).Ground); !closeTo(px[end], want) {
				t.Errorf("the band ends as %v, want the ground %v", px[end], want)
			}
			for y := 1; y < len(px); y++ {
				if s := step(px[y-1], px[y]); s > maxStep {
					t.Fatalf("an edge at y=%d: %v then %v", y, px[y-1], px[y])
				}
			}
		})
	}
}

// Reduce effects prints the band solid and ends it square: no gradient.
func TestReduceEffectsEndsTheBandSquare(t *testing.T) {
	theme.SetReduceEffects(true)
	t.Cleanup(func() { theme.SetReduceEffects(false) })
	r := newRig(t, "/album/100", 1132, 900)
	r.addCatalog(saltFlats(coloured("#c4793a")))
	r.shows("Quiet Records")
	r.tt.Settle()
	px := column(r.tt.Image(), r.bandEnd()+1)
	want := hexRGBA(t, palette.SleeveSurface("#c4793a", palette.Light))
	edge := -1
	for y := 1; y < len(px); y++ {
		if step(px[y-1], px[y]) > maxStep {
			if edge >= 0 {
				t.Fatalf("two edges down a solid band, at y=%d and y=%d", edge, y)
			}
			edge = y
		}
	}
	if edge < 0 {
		t.Fatal("no end to the band")
	}
	for y := range edge {
		if !closeTo(px[y], want) {
			t.Fatalf("the solid band is %v at y=%d, want its colour %v", px[y], y, want)
		}
	}
}

// The band's far end only ever moves towards the colour the text on it is not:
// the text's contrast is chosen for the band's colour, and can only improve.
func TestSideOfTheBandNeverCostsTheTextItsContrast(t *testing.T) {
	covers := []string{"#1b2a5c", "#f4efe2", "#808080", "#f2c94c", "#c0392b", "#0d3b2e", "#c4793a", "#ffffff", "#000000"}
	for _, dark := range []bool{false, true} {
		for _, bg := range covers {
			var opts []uitest.Option
			th := palette.Light
			if dark {
				opts, th = []uitest.Option{uitest.Dark()}, palette.Dark
			}
			t.Run(fmt.Sprintf("%s dark=%v", bg, dark), func(t *testing.T) {
				r := newRig(t, "/album/100", 1132, 900, opts...)
				r.addCatalog(saltFlats(coloured(bg)))
				r.shows("Quiet Records")
				k := r.a.pal.colours
				if k == nil {
					t.Fatal("no colours")
				}
				pal, _ := palette.ForAlbum(&palette.Cover{Background: bg}, nil, th, true)
				far := k.side[len(k.side)-1].Color
				mixed := palette.Over(pal.Sleeve.Surface, fmt.Sprintf("#%02x%02x%02x", far.R, far.G, far.B), float64(far.A)/255)
				for _, ink := range []string{pal.Sleeve.Ink, pal.Sleeve.InkMuted} {
					if now, was := palette.Contrast(ink, mixed), palette.Contrast(ink, pal.Sleeve.Surface); now < was-0.01 {
						t.Errorf("text %s on the band's far end reads %.2f:1, on its colour %.2f:1", ink, now, was)
					}
				}
			})
		}
	}
}

// A short pane takes the small cover and the compact head, so Play and the
// first songs are on the first screen.
func TestShortPaneShowsPlayAndTheFirstSongs(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 480)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) { coloured("#c4793a")(p) }))
	r.shows("Quiet Records")
	r.tt.Settle()
	if play, ok := r.tt.Find("Play Salt Flats"); !ok || play.Y+play.H > 480 {
		t.Errorf("Play at %+v, off a pane 480 high", play)
	}
	if song, ok := r.tt.Find("Glass Houses"); !ok || song.Y+song.H > 480 {
		t.Errorf("the third song at %+v, off a pane 480 high", song)
	}
	r.wantNoTexts("Mia Vale's second album.")
}
