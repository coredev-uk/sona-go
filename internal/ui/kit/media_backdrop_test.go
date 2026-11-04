package kit

import (
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

type backdropApp struct {
	l    *ArtLoader
	opts BackdropOpts
}

func (a *backdropApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Fill().Children(func() {
		Backdrop(c.Key("b"), a.l, a.opts, func() {
			ui.Text(c, "Title")
		}).Size(200, 200)
	})
}

func newBackdropApp(t *testing.T, src *fakeArt, o BackdropOpts) (*backdropApp, *ui.Tester, *mainThread) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem); theme.SetReduceEffects(false) })
	mt := newMainThread()
	a := &backdropApp{l: newArtLoader(src, mt.post, 2), opts: o}
	t.Cleanup(a.l.Close)
	return a, ui.NewTester(a.view, 200, 200), mt
}

// within is whether each channel of a is at most tol from b's.
func within(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

// scrimmed is what a picture colour looks like under the backdrop's scrim: the
// surface or ground token at the theme's opacity over it.
func scrimmed(under color.RGBA, tone string) color.RGBA {
	s := theme.C(tone)
	k := float64(theme.Current().Scalar["backdrop-scrim"])
	mix := func(over, under uint8) uint8 { return uint8(k*float64(over) + (1-k)*float64(under) + 0.5) }
	return color.RGBA{mix(s.R, under.R), mix(s.G, under.G), mix(s.B, under.B), 255}
}

func TestBackdropDrawsTheColourAtOnceThenTheImageFadesIn(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{})}
	a, tt, mt := newBackdropApp(t, src, BackdropOpts{Src: "/art/k1/backdrop.jpg", Color: "#c81e1e", Tone: ToneGround})
	tt.Frame()
	red := scrimmed(color.RGBA{200, 30, 30, 255}, "ground")
	if got := at(tt, 150, 150); !near(got, red) {
		t.Errorf("before the image decodes: %v, want the colour under the scrim %v", got, red)
	}

	close(src.gate)
	mt.run(t, 1)
	tt.Frame()
	// Early in the fade the image is part of the way there.
	tt.Advance(20 * time.Millisecond)
	tt.Frame()
	blue := scrimmed(color.RGBA{30, 60, 200, 255}, "ground")
	mid := at(tt, 150, 150)
	if within(mid, red, 2) || within(mid, blue, 2) {
		t.Errorf("mid-fade %v should be neither the colour %v nor the image %v", mid, red, blue)
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if got := at(tt, 150, 150); !near(got, blue) {
		t.Errorf("after the fade: %v, want the image under the scrim %v", got, blue)
	}
	if got := len(src.requests()); got != 1 {
		t.Errorf("%d requests for one backdrop: %v", got, src.requests())
	}
	_ = a
}

func TestBackdropTonesTheScrim(t *testing.T) {
	for _, tone := range []struct {
		t    BackdropTone
		name string
	}{{ToneSurface, "surface"}, {ToneGround, "ground"}} {
		_, tt, _ := newBackdropApp(t, &fakeArt{}, BackdropOpts{Color: "#000000", Tone: tone.t})
		tt.Frame()
		want := scrimmed(color.RGBA{0, 0, 0, 255}, tone.name)
		if got := at(tt, 150, 150); !near(got, want) {
			t.Errorf("tone %s: %v, want %v", tone.name, got, want)
		}
	}
	// Dark mode's scrim is the lighter 86%.
	_, tt, _ := newBackdropApp(t, &fakeArt{}, BackdropOpts{Color: "#ffffff"})
	theme.SetMode(theme.ModeDark)
	tt.SetDark(true)
	tt.Frame()
	want := scrimmed(color.RGBA{255, 255, 255, 255}, "surface")
	if got := at(tt, 150, 150); !near(got, want) {
		t.Errorf("dark: %v, want %v", got, want)
	}
}

func TestBackdropNeverShowsAnEmptyFrameWhenTheSourceChanges(t *testing.T) {
	src := &fakeArt{}
	a, tt, mt := newBackdropApp(t, src, BackdropOpts{Src: "/art/k1/backdrop.jpg"})
	tt.Frame()
	mt.run(t, 1)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	blue := scrimmed(color.RGBA{30, 60, 200, 255}, "surface")
	if got := at(tt, 150, 150); !near(got, blue) {
		t.Fatalf("first backdrop: %v, want %v", got, blue)
	}

	// The next source is gated: until it decodes, the last image stays.
	src.gate = make(chan struct{})
	a.opts.Src = "/art/k2/backdrop.jpg"
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if got := at(tt, 150, 150); !near(got, blue) {
		t.Errorf("while the next backdrop decodes: %v, want the last image %v", got, blue)
	}
	close(src.gate)
	mt.run(t, 1)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if got := at(tt, 150, 150); !near(got, blue) {
		t.Errorf("after: %v", got)
	}
	if n := len(src.requests()); n != 2 {
		t.Errorf("%d requests, want one per source: %v", n, src.requests())
	}
}

func TestBackdropDrawsNothingWithoutImageOrColourAndUnderReduceEffects(t *testing.T) {
	_, plain, _ := newBackdropApp(t, &fakeArt{}, BackdropOpts{})
	plain.Frame()
	ground := at(plain, 150, 150)

	_, tt, _ := newBackdropApp(t, &fakeArt{}, BackdropOpts{Color: "#ff0000"})
	tt.Frame()
	if got := at(tt, 150, 150); near(got, ground) {
		t.Fatalf("a colour draws nothing: %v", got)
	}
	theme.SetReduceEffects(true)
	tt.Frame()
	if got := at(tt, 150, 150); !near(got, ground) {
		t.Errorf("under reduce effects: %v, want the bare %v", got, ground)
	}
	// A bad colour is nothing, not a panic.
	if _, ok := hexColour("nope"); ok {
		t.Error("a bad colour parsed")
	}
	if c, ok := hexColour("#00ff00"); !ok || c != ui.RGB(0, 255, 0) {
		t.Errorf("hexColour = %v, %v", c, ok)
	}
}

func TestBackdropHoldsItsChildren(t *testing.T) {
	_, tt, _ := newBackdropApp(t, &fakeArt{}, BackdropOpts{Color: "#ff0000"})
	tt.Frame()
	if !tt.HasText("Title") {
		t.Errorf("the content is missing: %v", tt.Texts())
	}
}
