package kit

import (
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// updateMediaGolden rewrites the media goldens instead of comparing:
// go test ./internal/ui/kit -run Golden -update-media. (A tiny helper of this
// area until uitest's shared Golden lands.)
var updateMediaGolden = flag.Bool("update-media", false, "rewrite the media golden images")

var mediaFonts sync.Once

// mediaGolden compares img with testdata/golden/media-<name>.png: the same
// size, and at most 0.5% of the pixels more than 12 per channel off (glyph
// edges differ a little between CPUs), or writes it with -update-media.
func mediaGolden(t *testing.T, name string, img *image.RGBA) {
	t.Helper()
	path := filepath.Join("testdata", "golden", "media-"+name+".png")
	if *updateMediaGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("no golden image %s (run with -update-media): %v", path, err)
	}
	defer func() { _ = f.Close() }()
	want, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if want.Bounds() != img.Bounds() {
		t.Fatalf("%s: image is %v, golden %v", name, img.Bounds(), want.Bounds())
	}
	diff := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			r, g, b, _ := want.At(x, y).RGBA()
			c := img.RGBAAt(x, y)
			d := func(a uint8, w uint32) int { return max(int(a)-int(w>>8), int(w>>8)-int(a)) }
			if d(c.R, r) > 12 || d(c.G, g) > 12 || d(c.B, b) > 12 {
				diff++
			}
		}
	}
	if limit := img.Bounds().Dx() * img.Bounds().Dy() / 200; diff > limit {
		out := filepath.Join(os.TempDir(), "media-"+name+"-got.png")
		if f, err := os.Create(out); err == nil {
			_ = png.Encode(f, img)
			_ = f.Close()
		}
		t.Errorf("%s: %d pixels differ from the golden (limit %d); the frame is at %s", name, diff, limit, out)
	}
}

// mediaGalleryView is every media component in one frame.
func mediaGalleryView(l *ArtLoader) func(*ui.Context) {
	return func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Fill().Background(theme.C("ground")).Padding(16).Gap(14).Children(func() {
			ui.Row(c.Key("avatars")).Gap(10).AlignItems(ui.Center).Children(func() {
				for i, n := range []struct {
					name string
					size float32
				}{{"Thom Yorke", 24}, {"Nigel Godrich", 32}, {"The Bad Plus", 40}, {"米津玄師", 56}, {"j-hope", 56}} {
					PersonAvatar(c.Key(i), l, n.name, "", n.size)
				}
			})
			ui.Row(c.Key("shadcn")).Gap(24).AlignItems(ui.Center).Children(func() {
				Avatar(c.Key("a"), l, AvatarOpts{Fallback: "AB", Badge: true, Size: AvatarLarge})
				AvatarGroup(c.Key("g"), l, []AvatarOpts{{Fallback: "A"}, {Fallback: "B"}, {Fallback: "C"}}, 3)
				SonaMark(c.Key("m"), 0)
				EqualiserBars(c.Key("eq"), false, nil)
			})
			ui.Row(c.Key("art")).Gap(10).Children(func() {
				Artwork(c.Key("sq"), l, "", 56, ArtOpts{Edge: true})
				Artwork(c.Key("rd"), l, "", 56, ArtOpts{Round: true})
				Backdrop(c.Key("bd"), l, BackdropOpts{Color: "#c2553a", Tone: ToneGround}, func() {
					ui.Text(c, "Backdrop").FontSize(16).FontWeight(600).TextColor(theme.C("ink"))
				}).Size(150, 56).Padding(8)
			})
			GradientText(c.Key("grad"), "A word being sung", GradientOpts{
				Size: 28, Weight: 700, Base: theme.C("ink-muted"), Fill: theme.C("amber-ink"), Progress: 0.55,
			})
		})
	}
}

func TestMediaGoldens(t *testing.T) {
	mediaFonts.Do(func() {
		if err := theme.RegisterFonts(); err != nil {
			t.Fatal(err)
		}
	})
	defer theme.SetMode(theme.ModeSystem)
	for _, mode := range []struct {
		name string
		m    theme.Mode
	}{{"light", theme.ModeLight}, {"dark", theme.ModeDark}} {
		t.Run(mode.name, func(t *testing.T) {
			theme.SetMode(mode.m)
			l := newArtLoader(&fakeArt{}, newMainThread().post, 1)
			defer l.Close()
			tt := ui.NewTester(mediaGalleryView(l), 360, 300)
			tt.SetDark(mode.m == theme.ModeDark)
			tt.Frame()
			tt.Frame()
			mediaGolden(t, "components-"+mode.name, tt.Image())
		})
	}
}
