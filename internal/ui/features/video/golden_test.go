package video

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func TestGoldenPage(t *testing.T) {
	for _, mode := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(mode.name, func(t *testing.T) {
			p := newPageRig(t, mode.dark, 1132, 1500)
			p.loaded()
			p.tt.Settle()
			uitest.Golden(t, p.tt.Image(), "video-page-"+mode.name)
		})
	}
}

func TestGoldenPageNarrow(t *testing.T) {
	p := newPageRig(t, false, 940, 600)
	p.loaded()
	p.tt.Settle()
	uitest.Golden(t, p.tt.Image(), "video-page-940-light")
}

// 707 is the page's narrowest real width (a 940 window less the sidebar): the
// actions stack under the title block.
func TestGoldenPageNarrowest(t *testing.T) {
	p := newPageRig(t, false, 707, 900)
	p.loaded()
	p.tt.Settle()
	uitest.Golden(t, p.tt.Image(), "video-page-707-light")
}

// TestGoldenViewNarrow is the view at the window's minimum, 940×600.
func TestGoldenViewNarrow(t *testing.T) {
	r := newRig(t, false, viewMount, 940, 600)
	r.play(store.StatePaused)
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Advance(fadeIn)
	r.surface.Show(320, 180, 0, gradient(320, 180))
	r.tt.Until("the picture", r.v.feed.HasPicture)
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "video-view-940-light")
}

func TestGoldenView(t *testing.T) {
	for _, mode := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(mode.name, func(t *testing.T) {
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

// TestGoldenPageWide is the page at 1920×1200: the stage spans the column, and
// the 16:9 picture is centred in it at the height cap.
func TestGoldenPageWide(t *testing.T) {
	p := newPageRig(t, false, 1920, 1200)
	p.loaded()
	p.tt.Settle()
	uitest.Golden(t, p.tt.Image(), "video-page-1920-light")
}
