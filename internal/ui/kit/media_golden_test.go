package kit

import (
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/playback/video"
	"github.com/coredev-uk/sona-go/internal/ui/media"
	"github.com/coredev-uk/sona-go/internal/ui/media/mediatest"
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
	mediaGoldenWithin(t, name, img, 200)
}

// mediaGoldenWithin is mediaGolden allowing 1/frac of the pixels to differ
// (the colour bars' corner patch is noise, different on each decode).
func mediaGoldenWithin(t *testing.T, name string, img *image.RGBA, frac int) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("golden images are compared on Linux only: other platforms draw text with their own rasteriser")
	}
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
	diff, worst := 0, 0
	var box image.Rectangle
	var worstAt image.Point
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			r, g, b, _ := want.At(x, y).RGBA()
			c := img.RGBAAt(x, y)
			d := func(a uint8, w uint32) int { return max(int(a)-int(w>>8), int(w>>8)-int(a)) }
			m := max(d(c.R, r), d(c.G, g), d(c.B, b))
			if m > 12 {
				diff++
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
			if m > worst {
				worst, worstAt = m, image.Pt(x, y)
			}
		}
	}
	if limit := img.Bounds().Dx() * img.Bounds().Dy() / frac; diff > limit {
		out := filepath.Join(t.ArtifactDir(), "media-"+name+"-got.png")
		if f, err := os.Create(out); err == nil {
			_ = png.Encode(f, img)
			_ = f.Close()
		}
		t.Errorf("%s: %d pixels differ from the golden (limit %d), within %v; the worst, %d off, is at %v (got %v, want %v); the frame is at %s",
			name, diff, limit, box, worst, worstAt, img.RGBAAt(worstAt.X, worstAt.Y), want.At(worstAt.X, worstAt.Y), out)
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

// mediaPicture is a w×h I420 picture: a luma ramp across, blue-ish chroma on
// the left and red-ish on the right.
func mediaPicture(w, h int) []byte {
	b := make([]byte, media.I420Len(w, h))
	cw, ch := (w+1)/2, (h+1)/2
	for y := range h {
		for x := range w {
			b[y*w+x] = byte(40 + 180*x/w)
		}
	}
	for y := range ch {
		for x := range cw {
			b[w*h+y*cw+x] = byte(190 - 100*x/cw)
			b[w*h+cw*ch+y*cw+x] = byte(70 + 120*x/cw)
		}
	}
	return b
}

// mediaMotionView is the components that play: a motion cover (square and
// round), a hovered tile's motion, the music video surface and fluid artwork.
func mediaMotionView(p *MotionPolicy, l *ArtLoader, feed *VideoFeed) func(*ui.Context) {
	return func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Fill().Background(theme.C("ground")).Padding(16).Gap(14).Children(func() {
			ui.Row(c.Key("covers")).Gap(14).Children(func() {
				MotionCover(c.Key("cover"), p, l, "/art/k1/300x300.jpg", "/motion/k1/720.mp4", 96, MotionOpts{})
				MotionCover(c.Key("round"), p, l, "/art/k1/300x300.jpg", "/motion/k1/720.mp4", 96, MotionOpts{Round: true})
				ui.Box(c.Key("tile")).Size(96, 96).Children(func() {
					Artwork(c.Key("art"), l, "/art/k1/300x300.jpg", 96, ArtOpts{Edge: true})
					TileMotion(c.Key("motion"), p, "/motion/k1/720.mp4", 96, TileMotionOpts{Hot: true})
				})
			})
			ui.Row(c.Key("video")).Gap(14).Children(func() {
				ui.Box(c.Key("surface")).Size(160, 90).Background(theme.C("surface")).Children(func() {
					VideoSurface(c.Key("v"), feed, VideoOpts{A11yLabel: "Music video"})
				})
				ui.Box(c.Key("fluid")).Width(96).Children(func() {
					FluidArtwork(c.Key("f"), l, "/art/k1/300x300.jpg", 96, ArtOpts{Aspect: 16.0 / 9, Edge: true})
				})
			})
		})
	}
}

// TestMediaMotionGoldens draws the components that play with a real decoder
// and a generated static H.264 clip (colour bars: any frame is the same
// picture); it skips where GStreamer cannot decode or encode one.
func TestMediaMotionGoldens(t *testing.T) {
	if media.Codec() != "avc" {
		t.Skip("no H.264 decoder in this GStreamer: motion goldens need one")
	}
	clip := mediatest.ClipPattern(t, 90, "smpte")
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
			mt := newMainThread()
			l := newArtLoader(&fakeArt{}, mt.post, 2)
			defer l.Close()
			l.SetScale(1)
			p := NewMotionPolicy(nil, mt.post, MotionOptions{Files: &mediaLocalFile{path: clip}, Art: l})
			p.setCodec("avc")
			defer p.Close()
			surface := video.NewSurface()
			feed := NewVideoFeed(surface, mt.post)
			feed.Start()
			defer feed.Stop()

			tt := ui.NewTester(mediaMotionView(p, l, feed), 420, 260)
			tt.SetDark(mode.m == theme.ModeDark)
			surface.Show(160, 90, 0, mediaPicture(160, 90))
			tt.Frame()
			deadline := time.Now().Add(10 * time.Second)
			for {
				tt.Frame()
				mt.run(t, 0)
				tt.Advance(450 * time.Millisecond) // the tile's dwell
				ready := 0
				p.mu.Lock()
				for s := range p.slots {
					if s.state == slotReady {
						ready++
					}
				}
				p.mu.Unlock()
				if ready == 3 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("%d of 3 videos opened", ready)
				}
				time.Sleep(5 * time.Millisecond)
			}
			tt.Frame()
			tt.Advance(motionFade + 100*time.Millisecond)
			for until := time.Now().Add(5 * time.Second); !feed.picture; time.Sleep(2 * time.Millisecond) {
				tt.Frame()
				mt.run(t, 0)
				if time.Now().After(until) {
					t.Fatal("the music video picture did not show")
				}
			}
			tt.Frame()
			mediaGoldenWithin(t, "motion-"+mode.name, tt.Image(), 50)
		})
	}
}
