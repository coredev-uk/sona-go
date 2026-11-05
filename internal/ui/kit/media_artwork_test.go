package kit

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func TestArtAspectReadsTheShapeOfTheURL(t *testing.T) {
	for url, want := range map[string]float32{
		"/art/k1/800x200.jpg":          4,
		"/art/k1/300x300.jpg":          1,
		"/art/k1/300.jpg":              1, // n×n, as earlier versions saved
		"/art/k1/1920x800.jpg?retry=1": 2.4,
		"/art/k1/backdrop.jpg":         1,
		"https://example.com/art.png":  1,
		"":                             1,
		"/art/k1/0x0.jpg":              1,
		"/art/k1/640x360.jpg":          640.0 / 360,
	} {
		if got := artAspect(url); got < want-0.01 || got > want+0.01 {
			t.Errorf("artAspect(%q) = %v, want %v", url, got, want)
		}
	}
	for _, tc := range []struct {
		w, h, aspect, gw, gh float32
	}{
		{100, 100, 4, 100, 25}, // a wide banner stays wide
		{100, 100, 0.5, 50, 100},
		{100, 100, 1, 100, 100},
		{160, 90, 1, 90, 90},
	} {
		if gw, gh := containBox(tc.w, tc.h, tc.aspect); gw != tc.gw || gh != tc.gh {
			t.Errorf("containBox(%v, %v, %v) = %v×%v, want %v×%v", tc.w, tc.h, tc.aspect, gw, gh, tc.gw, tc.gh)
		}
	}
}

func TestArtworkContainRequestsTheArtworksOwnShapeInsideTheBox(t *testing.T) {
	src := &fakeArt{}
	a := &artApp{src: "/art/k1/800x200.jpg", size: 100, opts: ArtOpts{Contain: true}}
	tt, mt := newArtApp(t, src, a)
	tt.Frame()
	mt.run(t, 1)
	tt.Frame()
	if got := src.requests(); len(got) != 1 || got[0] != [2]int{100, 25} {
		t.Errorf("requests %v, want one of 100×25", got)
	}
	// Letterboxed: the picture is in the middle band, the box's top is bare.
	red := color.RGBA{220, 30, 30, 255}
	if got := at(tt, 60, 60); !near(got, red) {
		t.Errorf("the picture's middle: %v", got)
	}
	hover := theme.C("hover")
	if got := at(tt, 60, 14); !near(got, color.RGBA{hover.R, hover.G, hover.B, 255}) && near(got, red) {
		t.Errorf("the picture was stretched over the box: %v at the top", got)
	}
}

func TestArtworkFallbackReplacesTheGlyphAndTheSquare(t *testing.T) {
	built := 0
	a := &artApp{src: "", size: 60, opts: ArtOpts{Edge: true, Fallback: func(c *ui.Context) {
		built++
		ui.Row(c).Size(60, 60).Background(ui.RGB(0, 128, 0))
	}}}
	tt, _ := newArtApp(t, &fakeArt{}, a)
	tt.Frame()
	if built == 0 {
		t.Fatal("the fallback was not built")
	}
	if got := at(tt, 40, 40); !near(got, color.RGBA{0, 128, 0, 255}) {
		t.Errorf("fallback not drawn: %v", got)
	}
}

type mediaFluidApp struct {
	l      *ArtLoader
	width  float32
	size   float32
	aspect float32
}

func (a *mediaFluidApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(10).Children(func() {
		ui.Box(c.Key("cell")).Width(a.width).Children(func() {
			FluidArtwork(c.Key("f"), a.l, "/art/k1/300x300.jpg", a.size, ArtOpts{Aspect: a.aspect})
		})
	})
}

func TestFluidArtworkFillsItsContainerAtItsAspectAndRequestsAtSize(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	src := &fakeArt{}
	mt := newMainThread()
	a := &mediaFluidApp{width: 80, size: 160, aspect: 16.0 / 9}
	a.l = newArtLoader(src, mt.post, 2)
	defer a.l.Close()
	tt := ui.NewTester(a.view, 200, 200)
	tt.Frame()
	mt.run(t, 1)
	tt.Frame()
	// Requested at the largest width (160 × 90), shown at the width it got (80 × 45).
	if got := src.requests(); len(got) != 1 || got[0] != [2]int{160, 90} {
		t.Errorf("requests %v, want one of 160×90", got)
	}
	red := color.RGBA{220, 30, 30, 255}
	if got := at(tt, 50, 30); !near(got, red) {
		t.Errorf("the cover's middle: %v", got)
	}
	if got := at(tt, 50, 10+45+10); near(got, red) { // below 16:9 of 80 wide
		t.Errorf("the cover is taller than 16:9: %v below it", got)
	}
	if got := at(tt, 10+80+10, 30); near(got, red) { // right of the container
		t.Errorf("the cover is wider than its container: %v beside it", got)
	}
}

func TestFluidArtworkPlaceholderIsFortyPercentOfSize(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	a := &mediaFluidApp{width: 100, size: 100, aspect: 0}
	a.l = newArtLoader(&fakeArt{gate: make(chan struct{})}, newMainThread().post, 1)
	defer a.l.Close()
	tt := ui.NewTester(a.view, 200, 200)
	tt.Frame()
	hover := theme.C("hover")
	if got := at(tt, 14, 14); !near(got, color.RGBA{hover.R, hover.G, hover.B, 255}) {
		t.Errorf("fluid placeholder corner %v, want --hover", got)
	}
	glyph := 0
	for y := 40; y < 90; y++ {
		for x := 40; x < 90; x++ {
			if !near(at(tt, x, y), color.RGBA{hover.R, hover.G, hover.B, 255}) {
				glyph++
			}
		}
	}
	if glyph < 20 {
		t.Errorf("no glyph in the middle of the fluid placeholder: %d pixels", glyph)
	}
}

func TestArtLoaderBackdropDecodesTheTinyImageOnce(t *testing.T) {
	src := &fakeArt{}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 2)
	defer l.Close()
	if l.backdrop(nil, "") != nil {
		t.Error("a backdrop of nothing")
	}
	cv := l.backdrop(nil, "/art/k1/backdrop.jpg")
	mt.run(t, 1)
	if cv.bitmap() == nil {
		t.Fatal("the backdrop did not decode")
	}
	if w, h := cv.bitmap().Size(); w != 16 || h != 16 {
		t.Errorf("backdrop is %d×%d, its own size is 16×16", w, h)
	}
	if again := l.backdrop(nil, "/art/k1/backdrop.jpg"); again != cv {
		t.Error("a second ask made a new cover")
	}
	// A cover of the same artwork is another picture.
	l.cover(nil, rankBuilt, "/art/k1/300x300.jpg", 50, 50)
	mt.run(t, 1)
	if got := src.requests(); len(got) != 2 || got[0] != [2]int{0, 0} || got[1] != [2]int{50, 50} {
		t.Errorf("requests %v: one backdrop, one cover", got)
	}
	// Not one of ours: failed for good, nothing asked.
	if bad := l.backdrop(nil, "/elsewhere/backdrop.jpg"); bad.state != coverFailed {
		t.Errorf("a foreign backdrop is %v", bad.state)
	}
}

func TestArtLoaderWaitsForTheScaleWhenAskedTo(t *testing.T) {
	mt := newMainThread()
	l := newArtLoader(&fakeArt{}, mt.post, 1)
	defer l.Close()
	if l.scaleKnown() {
		t.Fatal("the scale is known before anything painted")
	}
	// The painter learns it, even the same 1, and asks for the frame that was waiting.
	l.noteScale(1)
	mt.run(t, 1)
	if !l.scaleKnown() {
		t.Error("the scale is not known after painting")
	}
	// A second paint at the same scale asks for nothing.
	l.noteScale(1)
	mt.run(t, 0)

	l2 := newArtLoader(&fakeArt{}, mt.post, 1)
	defer l2.Close()
	l2.SetScale(2)
	if !l2.scaleKnown() || l2.scale != 2 {
		t.Error("the scale the app told is not known")
	}
}
