package kit

import (
	"image/color"
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
