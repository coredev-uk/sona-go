package album

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/theme/palette"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func coloured(bg string, text ...string) func(*services.CatalogAlbum) {
	return func(p *services.CatalogAlbum) {
		p.ArtworkColors = services.ArtworkColors{Background: bg}
		for i, dst := range []*string{&p.ArtworkColors.Text1, &p.ArtworkColors.Text2, &p.ArtworkColors.Text3, &p.ArtworkColors.Text4} {
			if i < len(text) {
				*dst = text[i]
			}
		}
	}
}

func hexRGBA(t *testing.T, s string) color.RGBA {
	t.Helper()
	c, ok := palette.ParseHex(s)
	if !ok {
		t.Fatalf("not a colour: %q", s)
	}
	return color.RGBA{uint8(c[0]*255 + 0.5), uint8(c[1]*255 + 0.5), uint8(c[2]*255 + 0.5), 255}
}

func closeTo(a, b color.RGBA) bool {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return d(a.R, b.R) <= 2 && d(a.G, b.G) <= 2 && d(a.B, b.B) <= 2
}

// sleevePixel is the sleeve's colour: the pixel left of the label in the fine
// print, inside the sleeve's padding.
func (r *rig) sleevePixel() color.RGBA {
	r.t.Helper()
	b, ok := r.tt.Find("Quiet Records")
	if !ok {
		r.t.Fatalf("no fine print on screen: %q", r.tt.Texts())
	}
	return r.tt.Image().RGBAAt(int(b.X)-6, int(b.Y+b.H/2))
}

// playPixel is the play control's fill: just inside its left edge.
func (r *rig) playPixel() color.RGBA {
	r.t.Helper()
	b, ok := r.tt.Find("Play Salt Flats")
	if !ok {
		r.t.Fatalf("no Play button on screen: %q", r.tt.Texts())
	}
	return r.tt.Image().RGBAAt(int(b.X)+3, int(b.Y+b.H/2))
}

func TestSleeveIsPrintedInTheCoversBackgroundColour(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 900)
	r.addCatalog(saltFlats(coloured("#8acf00", "#050800")))
	r.shows("Quiet Records")
	r.tt.Settle()
	if got, want := r.sleevePixel(), hexRGBA(t, "#8acf00"); !closeTo(got, want) {
		t.Errorf("the sleeve is %v, want the cover's colour %v", got, want)
	}
}

func TestSleeveKeepsOffTheGroundAndIsNoBrightSlabInTheDarkTheme(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 900, uitest.Dark())
	r.addCatalog(saltFlats(coloured("#fffce9", "#050608")))
	r.shows("Quiet Records")
	r.tt.Settle()
	want := hexRGBA(t, palette.SleeveSurface("#fffce9", palette.Dark))
	if got := r.sleevePixel(); !closeTo(got, want) {
		t.Errorf("the dark sleeve is %v, want %v", got, want)
	}
	if l := palette.Lightness(palette.SleeveSurface("#fffce9", palette.Dark)); l > 0.37 {
		t.Errorf("the dark sleeve is %.2f light", l)
	}
}

func TestCoverWithNoColourKeepsTheSleevesWash(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 900)
	r.addCatalog(saltFlats())
	r.shows("Quiet Records")
	r.tt.Settle()
	// The ground under 4% ink.
	want := hexRGBA(t, palette.Over(palette.Themes[palette.Light].Ground, palette.Themes[palette.Light].Ink, 0.04))
	if got := r.sleevePixel(); !closeTo(got, want) {
		t.Errorf("a cover with no colour: sleeve %v, want the wash %v", got, want)
	}
}

func TestDominantColourPrintsTheSleeveWhenAppleGivesNone(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 900)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Header = services.Header{Colors: []string{"#3366cc"}}
	}))
	r.shows("Quiet Records")
	r.tt.Settle()
	want := hexRGBA(t, palette.SleeveSurface("#3366cc", palette.Light))
	if got := r.sleevePixel(); !closeTo(got, want) {
		t.Errorf("sleeve %v, want %v", got, want)
	}
}

func TestPlayControlTakesTheCoversAccent(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%v", dark), func(t *testing.T) {
			var opts []uitest.Option
			th := palette.Light
			if dark {
				opts, th = []uitest.Option{uitest.Dark()}, palette.Dark
			}
			r := newRig(t, "/album/100", 1132, 900, opts...)
			r.addCatalog(saltFlats(coloured("#3f546a", "#fdc14c", "#fcb92d", "#d7ab52", "#d6a539")))
			r.shows("Quiet Records")
			r.tt.Settle()
			pal, _ := palette.ForAlbum(&palette.Cover{Background: "#3f546a", Text1: "#fdc14c", Text2: "#fcb92d", Text3: "#d7ab52", Text4: "#d6a539"}, nil, th, false)
			if pal.Accent == nil {
				t.Fatal("the fixture has no accent")
			}
			if got, want := r.playPixel(), hexRGBA(t, pal.Accent.Fill); !closeTo(got, want) {
				t.Errorf("play control %v, want the accent %v", got, want)
			}
		})
	}
}

func TestGreyscaleCoverKeepsAmber(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 900)
	r.addCatalog(saltFlats(coloured("#ffffff", "#0c0c0c", "#191919")))
	r.shows("Quiet Records")
	r.tt.Settle()
	if got, want := r.playPixel(), hexRGBA(t, palette.Themes[palette.Light].Amber); !closeTo(got, want) {
		t.Errorf("play control %v, want amber %v", got, want)
	}
}

func TestNarrowLayoutPrintsTheHeadOnTheSleeve(t *testing.T) {
	r := newRig(t, "/album/100", 760, 1200)
	r.addCatalog(saltFlats(coloured("#8acf00", "#050800", "#070a00", "#203000")))
	r.shows("Salt Flats", "Quiet Records")
	r.tt.Settle()
	// The title is on the sleeve: the pixel left of it is the sleeve's colour.
	b, ok := r.tt.Find("Salt Flats")
	if !ok {
		t.Fatal("no title")
	}
	got := r.tt.Image().RGBAAt(int(b.X)-4, int(b.Y+b.H/2))
	if want := hexRGBA(t, "#8acf00"); !closeTo(got, want) {
		t.Errorf("narrow: beside the title %v, want the sleeve %v", got, want)
	}
}

func TestColoursAreBuiltOncePerCoverThemeAndLayout(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 900)
	r.addCatalog(saltFlats(coloured("#8acf00", "#050800")))
	r.shows("Quiet Records")
	r.tt.Settle()
	first := r.a.pal.colours
	if first == nil {
		t.Fatal("no colours built")
	}
	for range 3 {
		r.tt.Frame()
	}
	if r.a.pal.colours != first {
		t.Error("colours were rebuilt with nothing changed")
	}
	t.Cleanup(func() { theme.SetMode(theme.ModeLight) })
	theme.SetMode(theme.ModeDark)
	r.tt.Frame()
	r.tt.Settle()
	if r.a.pal.colours == first {
		t.Error("colours kept after the theme changed")
	}
	if got, want := r.sleevePixel(), hexRGBA(t, palette.SleeveSurface("#8acf00", palette.Dark)); !closeTo(got, want) {
		t.Errorf("dark sleeve %v, want %v", got, want)
	}
}

func TestTextOnTheSleeveFollowsItsInk(t *testing.T) {
	// A cover whose Apple text colour differs from the theme's ink.
	cover := palette.Cover{Background: "#3f546a", Text1: "#ffffff", Text2: "#dbe4ee"}
	pal, _ := palette.ForAlbum(&cover, nil, palette.Light, true)
	if pal.Sleeve.Ink == palette.Themes[palette.Light].Ink {
		t.Fatal("the fixture's ink is the theme's")
	}
	r := newRig(t, "/album/100", 760, 1200)
	r.addCatalog(saltFlats(coloured(cover.Background, cover.Text1, cover.Text2)))
	r.shows("Salt Flats", "Quiet Records")
	r.tt.Settle()
	b, ok := r.tt.Find("Salt Flats")
	if !ok {
		t.Fatal("no title")
	}
	// The title's most inked pixel: the one furthest from the sleeve.
	img, bg := r.tt.Image(), hexRGBA(t, pal.Sleeve.Surface)
	var best color.RGBA
	far := -1
	for y := int(b.Y); y < int(b.Y+b.H); y++ {
		for x := int(b.X); x < int(b.X+b.W); x++ {
			p := img.RGBAAt(x, y)
			d := max(int(p.R)-int(bg.R), int(bg.R)-int(p.R)) + max(int(p.G)-int(bg.G), int(bg.G)-int(p.G)) + max(int(p.B)-int(bg.B), int(bg.B)-int(p.B))
			if d > far {
				best, far = p, d
			}
		}
	}
	if want := hexRGBA(t, pal.Sleeve.Ink); !closeTo(best, want) {
		t.Errorf("the title is drawn in %v, want the sleeve's ink %v", best, want)
	}
}

func TestArtistLinkTakesTheCoversAccent(t *testing.T) {
	cover := palette.Cover{Background: "#3f546a", Text1: "#fdc14c", Text2: "#fcb92d", Text3: "#d7ab52", Text4: "#d6a539"}
	pal, _ := palette.ForAlbum(&cover, nil, palette.Light, true)
	if pal.Accent == nil {
		t.Fatal("the fixture has no accent")
	}
	r := newRig(t, "/album/100", 1132, 900)
	r.addCatalog(saltFlats(coloured(cover.Background, cover.Text1, cover.Text2, cover.Text3, cover.Text4)))
	r.shows("Quiet Records")
	r.tt.Settle()
	b, ok := r.tt.Find("Mia Vale")
	if !ok {
		t.Fatal("no byline")
	}
	// The name's most inked pixel: the one furthest from the sleeve it is
	// printed on.
	img, bg := r.tt.Image(), hexRGBA(t, pal.Sleeve.Surface)
	var best color.RGBA
	far := -1
	for y := int(b.Y); y < int(b.Y+b.H); y++ {
		for x := int(b.X); x < int(b.X+b.W); x++ {
			p := img.RGBAAt(x, y)
			d := max(int(p.R)-int(bg.R), int(bg.R)-int(p.R)) + max(int(p.G)-int(bg.G), int(bg.G)-int(p.G)) + max(int(p.B)-int(bg.B), int(bg.B)-int(p.B))
			if d > far {
				best, far = p, d
			}
		}
	}
	if want := hexRGBA(t, pal.Sleeve.AccentInk); !closeTo(best, want) {
		t.Errorf("the artist is drawn in %v, want the accent as ink on the sleeve %v", best, want)
	}
}
