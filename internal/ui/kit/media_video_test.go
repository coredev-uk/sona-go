package kit

import (
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/playback/video"
	"github.com/coredev-uk/sona-go/internal/ui/media"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// grey is a w×h I420 picture of one grey (no colour).
func mediaGrey(w, h int, luma byte) []byte {
	b := make([]byte, media.I420Len(w, h))
	for i := range b {
		b[i] = 128
	}
	for i := range w * h {
		b[i] = luma
	}
	return b
}

type mediaVideoApp struct{ v *VideoFeed }

func (a *mediaVideoApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Box(c).Size(200, 100).Children(func() {
		VideoSurface(c.Key("v"), a.v, VideoOpts{A11yLabel: "Music video"})
	})
}

// until runs frames and posted funcs until ok holds.
func mediaUntil(t *testing.T, tt *ui.Tester, mt *mainThread, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		tt.Frame()
		mt.run(t, 0)
		tt.Frame()
		if ok() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestVideoSurfaceShowsTheLatestPictureLetterboxed(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	surface := video.NewSurface()
	mt := newMainThread()
	v := NewVideoFeed(surface, mt.post)
	v.Start()
	defer v.Stop()
	a := &mediaVideoApp{v: v}
	tt := ui.NewTester(a.view, 200, 100)

	// A 4:3 picture shown before the view draws: nothing is converted until
	// it is drawn, and then the picture on show is.
	surface.Show(64, 48, 0, mediaGrey(64, 48, 235))
	tt.Frame()
	mediaUntil(t, tt, mt, "the picture to show", func() bool { return near(at(tt, 100, 50), color.RGBA{255, 255, 255, 255}) })
	// Letterboxed: 133×100 in the middle of 200×100, the sides bare.
	ground := theme.C("ground")
	if got := at(tt, 10, 50); !near(got, color.RGBA{ground.R, ground.G, ground.B, 255}) {
		t.Errorf("the side of the letterbox: %v, want the ground %v", got, ground)
	}
	if got := at(tt, 60, 50); !near(got, color.RGBA{255, 255, 255, 255}) {
		t.Errorf("the picture's left part: %v", got)
	}

	// The next picture replaces it in place.
	surface.Show(64, 48, time.Second, mediaGrey(64, 48, 16))
	mediaUntil(t, tt, mt, "the next picture", func() bool { return near(at(tt, 100, 50), color.RGBA{0, 0, 0, 255}) })

	// Cleared: the picture comes down.
	surface.Clear()
	mediaUntil(t, tt, mt, "the picture to come down", func() bool {
		return near(at(tt, 100, 50), color.RGBA{ground.R, ground.G, ground.B, 255})
	})
}

// refRGB is a limited-range BT.709 Y′CbCr sample as the React player's shader
// converted it: what the video's colours were before the GPU did it.
func refRGB(y, cb, cr byte) color.RGBA {
	clamp := func(f float64) uint8 { return uint8(math.Min(255, math.Max(0, math.Round(f)))) }
	l := 1.1643836 * (float64(y) - 16)
	d, e := float64(cb)-128, float64(cr)-128
	return color.RGBA{clamp(l + 1.7927411*e), clamp(l - 0.2132486*d - 0.5329093*e), clamp(l + 2.1124018*d), 255}
}

func abs(n int) int { return max(n, -n) }

// flatPicture is a w×h I420 picture of one colour.
func flatPicture(w, h int, y, cb, cr byte) []byte {
	b := make([]byte, media.I420Len(w, h))
	cw, ch := (w+1)/2, (h+1)/2
	for i := range b {
		switch {
		case i < w*h:
			b[i] = y
		case i < w*h+cw*ch:
			b[i] = cb
		default:
			b[i] = cr
		}
	}
	return b
}

// The GPU's conversion (the CPU renderer's here) is BT.709 in video's limited
// range: the colours of the CPU conversion this replaced.
func TestVideoSurfaceColoursAreBT709Limited(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	surface := video.NewSurface()
	mt := newMainThread()
	v := NewVideoFeed(surface, mt.post)
	v.Start()
	defer v.Stop()
	tt := ui.NewTester((&mediaVideoApp{v: v}).view, 200, 100)
	tt.Frame()

	for _, c := range []struct {
		name      string
		y, cb, cr byte
	}{
		{"red", 63, 102, 240},
		{"green", 173, 42, 26},
		{"blue", 32, 240, 118},
		{"skin", 150, 110, 150},
		{"white", 235, 128, 128},
		{"black", 16, 128, 128},
	} {
		want := refRGB(c.y, c.cb, c.cr)
		surface.Show(64, 48, 0, flatPicture(64, 48, c.y, c.cb, c.cr))
		mediaUntil(t, tt, mt, c.name+" to show", func() bool {
			got := at(tt, 100, 50)
			return abs(int(got.R)-int(want.R)) <= 3 && abs(int(got.G)-int(want.G)) <= 3 && abs(int(got.B)-int(want.B)) <= 3
		})
	}
}

func TestVideoSurfaceIsNamed(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	v := NewVideoFeed(video.NewSurface(), nil)
	tt := ui.NewTester((&mediaVideoApp{v: v}).view, 200, 100)
	tt.Frame()
	if _, ok := tt.Find("Music video"); !ok {
		t.Error("the video has no label")
	}
}
