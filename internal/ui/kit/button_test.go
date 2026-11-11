package kit

import (
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// buttonApp is a view with one button the tests drive.
type buttonApp struct {
	label   string
	opts    ButtonOpts
	clicked int
}

func (a *buttonApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(20).Gap(12).AlignItems(ui.Start).Children(func() {
		if Button(c.Key("b"), a.label, a.opts).Clicked() {
			a.clicked++
		}
		ui.Button(c, "Elsewhere") // takes the focus away
	})
}

func newButtonApp(t *testing.T, label string, o ButtonOpts) (*buttonApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	a := &buttonApp{label: label, opts: o}
	return a, ui.NewTester(a.view, 320, 160)
}

func TestButtonClicks(t *testing.T) {
	a, tt := newButtonApp(t, "Save", ButtonOpts{Icon: "plus"})
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if a.clicked != 1 {
		t.Errorf("clicked %d times", a.clicked)
	}
}

func TestDisabledButtonTakesNeitherClickNorFocus(t *testing.T) {
	a, tt := newButtonApp(t, "Save", ButtonOpts{Disabled: true})
	_ = tt.Click("Save")
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	if a.clicked != 0 {
		t.Errorf("a disabled button was clicked %d times", a.clicked)
	}
	if tt.Focused("Save") {
		t.Error("a disabled button took the focus")
	}
}

func TestButtonFocusEnterAndSpace(t *testing.T) {
	a, tt := newButtonApp(t, "Save", ButtonOpts{})
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Save") {
		t.Fatal("Tab did not focus the first button")
	}
	tt.Key(0, ui.KeyEnter)
	if a.clicked != 1 {
		t.Errorf("Enter: clicked %d times", a.clicked)
	}
	tt.Key(0, ui.KeySpace)
	if a.clicked != 2 {
		t.Errorf("Space: clicked %d times", a.clicked)
	}
	tt.Key(0, ui.KeyTab)
	if tt.Focused("Save") || !tt.Focused("Elsewhere") {
		t.Error("Tab did not move on")
	}
	tt.Key(0, ui.KeyEnter)
	if a.clicked != 2 {
		t.Errorf("Enter on another button clicked this one: %d", a.clicked)
	}
}

func TestIconOnlyButtonIsNamedByItsLabel(t *testing.T) {
	a, tt := newButtonApp(t, "", ButtonOpts{Icon: "ellipsis", Variant: Ghost, Size: SizeSM, A11yLabel: "More"})
	if _, ok := tt.Find("More"); !ok {
		t.Fatal("no element labelled More")
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if a.clicked != 1 {
		t.Errorf("clicked %d times", a.clicked)
	}
	// A square: as wide as it is high.
	r, _ := tt.Find("More")
	if r.W != 32 || r.H != 32 {
		t.Errorf("icon-only SM button is %v×%v, want 32×32", r.W, r.H)
	}
}

func TestButtonSizes(t *testing.T) {
	for _, tc := range []struct {
		size ButtonSize
		want float32
	}{{SizeMD, 36}, {SizeSM, 32}, {SizeXS, 24}} {
		for _, v := range []ButtonVariant{Default, Play} {
			_, tt := newButtonApp(t, "", ButtonOpts{Size: tc.size, Variant: v, Icon: "play", A11yLabel: "Go"})
			r, ok := tt.Find("Go")
			if !ok || r.W != tc.want || r.H != tc.want {
				t.Errorf("size %d, variant %d: icon-only box %v (found %v), want %v square", tc.size, v, r, ok, tc.want)
			}
		}
	}
	// With a label the height is the size's and the width follows the text.
	_, tt := newButtonApp(t, "A longer label", ButtonOpts{Size: SizeSM, Icon: "plus", A11yLabel: "Add"})
	text, _ := tt.Find("A longer label")
	if text.H > 32 || text.W < 60 {
		t.Errorf("label box %v", text)
	}
}

func TestToggleButtonShowsPressed(t *testing.T) {
	on := false
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Padding(20).Children(func() {
			if Button(c.Key("t"), "Shuffle", ButtonOpts{Variant: Ghost, Pressed: &on}).Clicked() {
				on = !on
			}
		})
	}, 200, 100)
	if err := tt.Click("Shuffle"); err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("not toggled")
	}
	tt.Advance(time.Second)
	tt.Move(150, 90) // off the button: the face is the pressed tone alone
	tt.Advance(time.Second)
	r, _ := tt.Find("Shuffle")
	img := tt.Image()
	got := img.RGBAAt(int(r.X)-6, int(r.Y+r.H/2))
	want := theme.C("active")
	if got.R != want.R || got.G != want.G || got.B != want.B {
		t.Errorf("pressed face %v, want %v", got, want)
	}
}

// face is the colour of the button's own background, a few DIPs inside its
// left edge and clear of its text.
func face(tt *ui.Tester, text string) color.RGBA {
	r, _ := tt.Find(text)
	tt.Frame()
	return tt.Image().RGBAAt(int(r.X)-6, int(r.Y+r.H/2))
}

func rgb(c ui.Color) color.RGBA { return color.RGBA{c.R, c.G, c.B, 255} }

func TestVariantFaces(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts ButtonOpts
		want string
	}{
		{"default is ink", ButtonOpts{}, "ink"},
		{"play is amber", ButtonOpts{Variant: Play}, "amber"},
		{"ghost has no face", ButtonOpts{Variant: Ghost}, "ground"},
		{"outline has no fill", ButtonOpts{Variant: Outline}, "ground"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, tt := newButtonApp(t, "Label", tc.opts)
			tt.Advance(time.Second)
			if got, want := face(tt, "Label"), rgb(theme.C(tc.want)); got != want {
				t.Errorf("face %v, want %s %v", got, tc.want, want)
			}
		})
	}
}

// The disabled Play disc keeps its full-opacity face, --hover, over whatever it
// sits on: the page's ground, and a cover's wash, where the colours that were
// chosen to come out right at 50% over the ground came out wrong.
func TestDisabledPlayDiscKeepsItsFace(t *testing.T) {
	for _, mode := range []theme.Mode{theme.ModeLight, theme.ModeDark} {
		theme.SetMode(mode)
		for name, behind := range map[string]ui.Color{
			"ground":     theme.C("ground"),
			"cover wash": ui.RGB(200, 40, 90),
		} {
			tt := ui.NewTester(func(c *ui.Context) {
				theme.Apply(c)
				ui.Column(c).Background(behind).Padding(20).AlignItems(ui.Start).Children(func() {
					Button(c.Key("b"), "", ButtonOpts{Variant: Play, Icon: "play", A11yLabel: "Play", Disabled: true, Size: SizeLG})
				})
			}, 200, 120)
			tt.SetDark(mode == theme.ModeDark)
			tt.Advance(time.Second)
			r, ok := tt.Find("Play")
			if !ok {
				t.Fatal("no Play disc")
			}
			// The disc's edge, off the glyph at its middle.
			if got, want := at(tt, int(r.X)+4, int(r.Y+r.H/2)), rgb(theme.C("hover")); got != want {
				t.Errorf("mode %v over the %s: disabled disc is %v, want --hover %v", mode, name, got, want)
			}
		}
	}
	theme.SetMode(theme.ModeSystem)
}

func TestHoverEasesOver120ms(t *testing.T) {
	_, tt := newButtonApp(t, "Label", ButtonOpts{Variant: Ghost})
	tt.Advance(time.Second)
	r, _ := tt.Find("Label")
	rest := face(tt, "Label")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(60 * time.Millisecond)
	mid := face(tt, "Label")
	tt.Advance(time.Second)
	end := face(tt, "Label")
	hover := rgb(theme.C("accent-soft").Over(theme.C("ground")))
	if rest == end || end != hover {
		t.Fatalf("rest %v, hovered %v, want %v", rest, end, hover)
	}
	if mid == rest || mid == end {
		t.Errorf("no ease: halfway through it is %v (rest %v, end %v)", mid, rest, end)
	}
	// Reduced motion: the face changes at once.
	theme.SetReducedMotion(true)
	defer theme.SetReducedMotion(false)
	tt.Move(300, 150)
	tt.Frame()
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	if got := face(tt, "Label"); got != hover {
		t.Errorf("reduced motion still eases: %v, want %v", got, hover)
	}
}

func TestSpinningButtonKeepsAskingForFramesUnlessMotionIsReduced(t *testing.T) {
	t.Cleanup(func() { theme.SetReducedMotion(false) })
	for _, reduced := range []bool{false, true} {
		theme.SetReducedMotion(reduced)
		_, tt := newButtonApp(t, "Signing in…", ButtonOpts{Icon: "loader-circle", Spin: true})
		tt.Frame()
		tt.Advance(300 * time.Millisecond)
		if got := tt.FramePending(); got == reduced {
			t.Errorf("reduced motion %v: frames pending %v", reduced, got)
		}
	}
	theme.SetReducedMotion(false)
	_, tt := newButtonApp(t, "Sign in", ButtonOpts{Icon: "loader-circle"})
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	if tt.FramePending() {
		t.Error("a button without Spin asks for frames")
	}
}
