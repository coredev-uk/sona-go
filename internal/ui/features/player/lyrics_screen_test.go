package player

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// backdropArt stands in for the artwork cache: every "/art/…" path has a
// template, every cover and backdrop is a solid JPEG.
type backdropArt struct{}

func (backdropArt) Template(p string) (string, bool) {
	return "https://is1-ssl.mzstatic.com/image/thumb/Music/x/{w}x{h}bb.jpg", strings.HasPrefix(p, "/art/")
}

func solidJPEG(w, h int, c color.RGBA) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var b bytes.Buffer
	err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95})
	return b.Bytes(), err
}

func (backdropArt) Get(_ context.Context, _ string, w, h int) ([]byte, error) {
	return solidJPEG(w, h, color.RGBA{220, 30, 30, 255})
}

// Backdrop is a picture that changes across: blue at the left, going to
// a dusky red at the right, so a fade that is one colour all along shows.
func (backdropArt) Backdrop(context.Context, string) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			k := float64(x) / 63
			img.SetRGBA(x, y, color.RGBA{uint8(30 + 100*k), 60, uint8(200 - 120*k), 255})
		}
	}
	var b bytes.Buffer
	err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95})
	return b.Bytes(), err
}

// within is whether each channel of a is at most tol from b's.
func within(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

// textContrast is how far apart the lightest and darkest pixels inside text's
// box are (0-255): the words stand out of what is behind them when it is high,
// and a scrim drawn over them flattens it.
func textContrast(t *testing.T, r *npRig, text string) int {
	t.Helper()
	box, ok := r.tt.Find(text)
	if !ok {
		t.Fatalf("%q is not on screen: %q", text, r.tt.Texts())
	}
	img := r.tt.Image()
	lo, hi := 255, 0
	for y := int(box.Y); y < int(box.Y+box.H); y++ {
		for x := int(box.X); x < int(box.X+box.W); x++ {
			c := img.RGBAAt(x, y)
			l := (int(c.R)*299 + int(c.G)*587 + int(c.B)*114) / 1000
			lo, hi = min(lo, l), max(hi, l)
		}
	}
	return hi - lo
}

// The cover's backdrop is a wash behind the lyrics, never a veil over them:
// once it has loaded (a song with a cover, as every real one has), Now
// Playing's lyric pane and the full-screen lyrics still show their lines.
func TestLyricsShowOverTheCoversBackdrop(t *testing.T) {
	for _, mode := range []struct {
		name string
		opts []uitest.Option
	}{{"light", nil}, {"dark", []uitest.Option{uitest.Dark()}}} {
		for _, screen := range []string{"pane", "room"} {
			t.Run(mode.name+"/"+screen, func(t *testing.T) {
				r := newNPRigWith(t, mode.opts)
				loader := kit.NewArtLoaderFrom(backdropArt{}, r.h.Main.Post)
				t.Cleanup(loader.Close)
				r.h.Env.Art = loader
				r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(30, false)}, nil)
				r.play(5000, func(s *services.PlaybackState) {
					s.Current.Backdrop = "/art/k1/backdrop.jpg"
					s.Current.Art.Small = "/art/k1/art.jpg"
				})
				if screen == "room" {
					r.p.ui.openRoom()
				} else {
					r.open("")
				}
				r.tt.Until("the lyrics", r.tt.Shows("Line 1 sung words"))
				// The backdrop decodes off the main thread and fades in.
				r.tt.Until("the backdrop to load", func() bool { return loader.Pending() == 0 })
				r.settle()
				r.advance(300 * time.Millisecond)
				r.advance(300 * time.Millisecond)

				if got := textContrast(t, r, "Line 1 sung words"); got < 60 {
					t.Errorf("%s: the lines' contrast with what is behind them is %d/255; something is drawn over them", screen, got)
				}
			})
		}
	}
}

// A sheet fades out into the wash, not into a flat colour: where the fade
// is solid (the sheet's edge) it is the colour of the wash beside it, all
// along the edge, though the wash changes across.
func TestLyricsFadesMeetTheWash(t *testing.T) {
	for _, mode := range []struct {
		name string
		opts []uitest.Option
	}{{"light", nil}, {"dark", []uitest.Option{uitest.Dark()}}} {
		for _, screen := range []string{"pane", "room"} {
			t.Run(mode.name+"/"+screen, func(t *testing.T) {
				r := newNPRigWith(t, mode.opts)
				loader := kit.NewArtLoaderFrom(backdropArt{}, r.h.Main.Post)
				t.Cleanup(loader.Close)
				r.h.Env.Art = loader
				var sheet ui.Rect
				r.inspect = func(c *ui.Context) {
					if screen == "room" {
						if sh := r.p.np.room; sh != nil {
							sheet = sh.box.Bounds(c)
						}
					} else if sh := r.p.np.sheet; sh != nil {
						sheet = sh.box.Bounds(c)
					}
				}
				r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(30, false)}, nil)
				r.play(5000, func(s *services.PlaybackState) { s.Current.Backdrop = "/art/k1/backdrop.jpg" })
				if screen == "room" {
					r.p.ui.openRoom()
				} else {
					r.open("")
				}
				r.tt.Until("the lyrics", r.tt.Shows("Line 1 sung words"))
				r.tt.Until("the backdrop to load", func() bool { return loader.Pending() == 0 })
				r.settle()
				r.advance(300 * time.Millisecond)
				r.advance(300 * time.Millisecond)

				if sheet.W == 0 {
					t.Fatal("the sheet has no bounds")
				}
				img := r.tt.Image()
				bottom := int(sheet.Y + sheet.H)
				for _, f := range []float32{0.1, 0.5, 0.9} {
					x := int(sheet.X + f*sheet.W)
					in, out := img.RGBAAt(x, bottom-2), img.RGBAAt(x, bottom+3)
					if !within(in, out, 6) {
						t.Errorf("at %d%% across, the fade's edge is %v and the wash beside it %v", int(f*100), in, out)
					}
				}
			})
		}
	}
}
