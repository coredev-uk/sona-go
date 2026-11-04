package kit

import (
	"fmt"
	"image/color"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func TestInitials(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		// The first letter of the first and last word.
		{"Thom Yorke", "TY"},
		{"Nigel Godrich", "NG"},
		{"Ludwig van Beethoven", "LB"},
		{"Dan Grech-Marguerat", "DG"},
		{"Simon & Garfunkel", "SG"},
		// Never more than two, one for a single word.
		{"Madonna", "M"},
		{"Juan Carlos de la Cruz Pérez", "JP"},
		{"A", "A"},
		// Upper-cased, punctuation before a letter skipped.
		{"j-hope", "J"},
		{"lil' kim", "LK"},
		{"  (Strings)  ", "S"},
		{"Edward O'Brien", "EO"},
		{"'Cause I Said", "CS"},
		// Numbers count, a leading The goes, bare symbols are ignored.
		{"50 Cent", "5C"},
		{"The Bad Plus", "BP"},
		{"The Weeknd", "W"},
		{"The", "T"},
		{"Mike & The Mechanics", "MM"},
		// Other scripts, as whole characters.
		{"米津玄師", "米"},
		{"堀 正輝", "堀正"},
		{"봉준호", "봉"},
		{"فيروز", "ف"},
		{"Владимир Высоцкий", "ВВ"},
		{"ヨーヨー・マ", "ヨ"},
		{"Émile Zola", "ÉZ"},
		{"e\u0301mile", "\u00c9"},
		{"𠮷野 家", "𠮷家"},
		// A letter that would upper-case to several stays.
		{"ßo", "ß"},
		// No letter or number: the first character; nothing for nothing.
		{"$$$", "$"},
		{"🎻 !!!", "🎻"},
		{"   ", ""},
		{"", ""},
		// Sequences are one character: a family, a flag, a skin tone, a Hangul jamo syllable.
		{"👨‍👩‍👧 !!!", "👨‍👩‍👧"},
		{"🇬🇧 !!!", "🇬🇧"},
		{"👍🏽 !!!", "👍🏽"},
		{"\u1112\u1161\u11ab", "\ud55c"},
	} {
		if got := Initials(tc.name); got != tc.want {
			t.Errorf("Initials(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAvatarTone(t *testing.T) {
	// Deterministic, and blind to case, spacing and Unicode form.
	if AvatarTone("Thom Yorke") != AvatarTone("  thom yorke ") {
		t.Error("case and spacing change the tone")
	}
	if AvatarTone("Émile") != AvatarTone("E\u0301mile") {
		t.Error("the Unicode form changes the tone")
	}
	// FNV-1a over UTF-16 units: fixed values, so tints never move between
	// sessions or versions (these are v0.2.0's).
	for name, want := range map[string]int{"": 5, "a": 4, "thom yorke": AvatarTone("Thom Yorke")} {
		if got := AvatarTone(name); got != want {
			t.Errorf("AvatarTone(%q) = %d, want %d", name, got, want)
		}
	}
	// Every tint gets used across a crowd, and none out of range.
	seen := map[int]bool{}
	for i := range 400 {
		tone := AvatarTone(fmt.Sprintf("Person %d", i))
		if tone < 0 || tone >= AvatarTones {
			t.Fatalf("tone %d out of range", tone)
		}
		seen[tone] = true
	}
	if len(seen) != AvatarTones {
		t.Errorf("%d of %d tints used", len(seen), AvatarTones)
	}
	// The members of one record keep apart.
	record := map[int]bool{}
	for _, n := range []string{"Thom Yorke", "Jonny Greenwood", "Colin Greenwood", "Philip Selway", "Nigel Godrich"} {
		record[AvatarTone(n)] = true
	}
	if len(record) < 3 {
		t.Errorf("one record's names use %d tints", len(record))
	}
}

// lum is the WCAG relative luminance of a colour.
func mediaLum(c ui.Color) float64 {
	f := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

func mediaContrast(a, b ui.Color) float64 {
	hi, lo := mediaLum(a), mediaLum(b)
	if lo > hi {
		hi, lo = lo, hi
	}
	return (hi + 0.05) / (lo + 0.05)
}

func TestAvatarTintsAreLegibleAndDistinct(t *testing.T) {
	defer theme.SetMode(theme.ModeSystem)
	for _, mode := range []theme.Mode{theme.ModeLight, theme.ModeDark} {
		theme.SetMode(mode)
		bgs := map[ui.Color]bool{}
		for n := range AvatarTones {
			bg, fg := theme.C(fmt.Sprintf("avatar-%d-bg", n)), theme.C(fmt.Sprintf("avatar-%d-fg", n))
			if r := mediaContrast(bg, fg); r < 4.5 {
				t.Errorf("mode %v tint %d: text contrast %.2f:1, want at least 4.5", mode, n, r)
			}
			bgs[bg] = true
		}
		if len(bgs) != AvatarTones {
			t.Errorf("mode %v: %d distinct tints of %d", mode, len(bgs), AvatarTones)
		}
	}
}

type mediaAvatarApp struct {
	l    *ArtLoader
	name string
	src  string
	size float32
}

func (a *mediaAvatarApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(10).Children(func() {
		PersonAvatar(c.Key("p"), a.l, a.name, a.src, a.size)
	})
}

func newMediaAvatarApp(t *testing.T, src *fakeArt, a *mediaAvatarApp) (*ui.Tester, *mainThread) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	mt := newMainThread()
	a.l = newArtLoader(src, mt.post, 2)
	t.Cleanup(a.l.Close)
	return ui.NewTester(a.view, 200, 200), mt
}

func TestPersonAvatarDrawsInitialsOnItsTint(t *testing.T) {
	a := &mediaAvatarApp{name: "Thom Yorke", size: 80}
	tt, _ := newMediaAvatarApp(t, &fakeArt{}, a)
	tt.Frame()
	tone := AvatarTone("Thom Yorke")
	bg := theme.C(fmt.Sprintf("avatar-%d-bg", tone))
	// The tint fills the circle (just inside its left edge at mid-height)...
	if got := at(tt, 14, 50); !near(got, color.RGBA{bg.R, bg.G, bg.B, 255}) {
		t.Errorf("tint %v at the edge of the circle, got %v", bg, got)
	}
	// ...but not the corner of its box: it is round.
	if got := at(tt, 11, 11); near(got, color.RGBA{bg.R, bg.G, bg.B, 255}) {
		t.Errorf("the corner is tinted: %v", got)
	}
	// The initials are drawn in the foreground tint in the middle.
	fg := theme.C(fmt.Sprintf("avatar-%d-fg", tone))
	ink := 0
	for y := 30; y < 70; y++ {
		for x := 30; x < 70; x++ {
			if g := at(tt, x, y); near(g, color.RGBA{fg.R, fg.G, fg.B, 255}) {
				ink++
			}
		}
	}
	if ink < 30 {
		t.Errorf("no initials in %v: %d pixels", fg, ink)
	}
}

func TestPersonAvatarShowsThePictureAndFallsBackWhenItFails(t *testing.T) {
	src := &fakeArt{}
	a := &mediaAvatarApp{name: "Thom Yorke", src: "/art/k1/300x300.jpg", size: 80}
	tt, mt := newMediaAvatarApp(t, src, a)
	tt.Frame()
	mt.run(t, 1)
	tt.Frame()
	if got := at(tt, 50, 50); !near(got, color.RGBA{220, 30, 30, 255}) {
		t.Errorf("the picture is not shown: %v", got)
	}
	if got := src.requests(); len(got) != 1 || got[0] != [2]int{80, 80} {
		t.Errorf("requests %v, want one of 80×80", got)
	}

	// A picture that fails keeps the initials.
	failing := &fakeArt{fail: 100}
	b := &mediaAvatarApp{name: "Thom Yorke", src: "/art/k1/300x300.jpg", size: 80}
	tt, mt = newMediaAvatarApp(t, failing, b)
	tt.Frame()
	mt.run(t, 1)
	tt.Frame()
	bg := theme.C(fmt.Sprintf("avatar-%d-bg", AvatarTone("Thom Yorke")))
	if got := at(tt, 14, 50); !near(got, color.RGBA{bg.R, bg.G, bg.B, 255}) {
		t.Errorf("a failed picture does not fall back to the tint: %v", got)
	}
}

func TestAvatarSizesAndGroup(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	l := newArtLoader(&fakeArt{}, newMainThread().post, 1)
	defer l.Close()
	for size, want := range map[AvatarSize]float32{AvatarSmall: 24, AvatarDefault: 32, AvatarLarge: 40} {
		if got := size.Diameter(); got != want {
			t.Errorf("size %d is %v, want %v", size, got, want)
		}
	}
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Padding(10).Gap(8).Children(func() {
			Avatar(c.Key("a"), l, AvatarOpts{Fallback: "AB", Badge: true, A11yLabel: "Ada"})
			AvatarGroup(c.Key("g"), l, []AvatarOpts{{Fallback: "A"}, {Fallback: "B"}, {Fallback: "C"}}, 4)
			SonaMark(c.Key("m"), 0)
		})
	}, 200, 200)
	tt.Frame()
	if !tt.HasText("AB") || !tt.HasText("+4") {
		t.Errorf("fallbacks missing: %v", tt.Texts())
	}
	if _, ok := tt.Find("Ada"); !ok {
		t.Error("the avatar is not named")
	}
	// The group overlaps by 8 DIPs: the second avatar starts 24 DIPs after the first.
	a, _ := tt.Find("A")
	b, _ := tt.Find("B")
	if d := b.X - a.X; math.Abs(float64(d)-24) > 8 { // the texts are centred in their circles
		t.Errorf("avatars are %v DIPs apart, want about 24", d)
	}
}

// A group avatar's ring is drawn outside its edge, like the box-shadow ring
// of v0.2.0, so the picture keeps its full diameter.
func TestAvatarRingSitsOutsideTheEdge(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	l := newArtLoader(&fakeArt{}, newMainThread().post, 1)
	defer l.Close()
	red := ui.RGB(220, 30, 30)
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).FillWidth().FillHeight().Padding(20).Background(red).AlignItems(ui.Start).Children(func() {
			Avatar(c.Key("a"), l, AvatarOpts{Fallback: "AB", Ring: true, A11yLabel: "Ada"})
		})
	}, 100, 100)
	tt.Frame()
	r := controlsFind(t, tt, "Ada")
	img := tt.Image()
	s := float32(img.Bounds().Dx()) / 100
	px := func(x, y float32) color.RGBA { return img.RGBAAt(int(x*s), int(y*s)) }
	mid := r.Y + r.H/2
	bgc := theme.C("background")
	bg := color.RGBA{bgc.R, bgc.G, bgc.B, 255}
	if got := px(r.X-1, mid); !near(got, bg) {
		t.Errorf("1 DIP outside the edge is %v, want the background ring", got)
	}
	if got := px(r.X-3, mid); !near(got, color.RGBA{220, 30, 30, 255}) {
		t.Errorf("3 DIPs outside the edge is %v, want the page behind", got)
	}
	if got := px(r.X+r.W/2, mid-r.H/2+1); near(got, bg) {
		t.Errorf("the ring is drawn inside the avatar's edge: %v", got)
	}
}
