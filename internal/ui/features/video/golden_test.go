package video

import (
	"sync"
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

var fontOnce sync.Once

// fonts registers the interface font before a golden renders: text built
// earlier would use the system's sans, and the golden would differ in CI.
func fonts(t *testing.T) {
	t.Helper()
	fontOnce.Do(func() {
		if err := theme.RegisterFonts(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestGoldenPage(t *testing.T) {
	for _, mode := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(mode.name, func(t *testing.T) {
			fonts(t)
			p := newPageRig(t, mode.dark, 1132, 1253)
			p.loaded()
			p.tt.Settle()
			uitest.Golden(t, p.tt.Image(), "video-page-"+mode.name)
		})
	}
}

func TestGoldenPageNarrow(t *testing.T) {
	fonts(t)
	p := newPageRig(t, false, 940, 600)
	p.loaded()
	p.tt.Settle()
	uitest.Golden(t, p.tt.Image(), "video-page-940-light")
}

func TestGoldenView(t *testing.T) {
	for _, mode := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(mode.name, func(t *testing.T) {
			fonts(t)
			r := newRig(t, mode.dark, viewMount, 1132, 1253)
			// Paused: the controls stay and nothing ticks. A 4:3 picture
			// shows the letterbox.
			r.play(store.StatePaused)
			r.env.Video.Open()
			r.tt.Frame()
			r.tt.Advance(fadeIn)
			r.surface.Show(320, 240, 0, gradient(320, 240))
			r.tt.Until("the picture", r.v.feed.HasPicture)
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), "video-view-"+mode.name)
		})
	}
}

// gradient is a w×h I420 picture of a luma ramp with a colour split, so the
// golden shows the conversion and the shape.
func gradient(w, h int) []byte {
	cw, ch := (w+1)/2, (h+1)/2
	b := make([]byte, w*h+2*cw*ch)
	for y := range h {
		for x := range w {
			b[y*w+x] = byte(16 + x*219/w)
		}
	}
	for y := range ch {
		for x := range cw {
			b[w*h+y*cw+x] = byte(64 + y*128/ch)
			b[w*h+cw*ch+y*cw+x] = byte(192 - x*128/cw)
		}
	}
	return b
}
