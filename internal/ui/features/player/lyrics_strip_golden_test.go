package player

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// TestFullScreenLyricsStripGoldens is the foot strip at the widths it has to
// hold (1280 is in TestFullScreenLyricsGoldens): a full-screen 1920 and a
// narrow window, light and dark. The transport and the scrubber are one
// cluster at the middle, the song at the left, the controls at the right.
func TestFullScreenLyricsStripGoldens(t *testing.T) {
	// A picture is all each case checks, and only Linux compares it: see
	// TestFullScreenLyricsGoldens.
	if runtime.GOOS != "linux" {
		t.Skip("golden images are compared on Linux only: macOS and Windows draw text differently")
	}
	for _, size := range []struct {
		name string
		w, h int
	}{{"1920", 1920, 1080}, {"narrow", 1024, 576}} {
		for _, mode := range []struct {
			name string
			opts []uitest.Option
		}{{"light", nil}, {"dark", []uitest.Option{uitest.Dark()}}} {
			t.Run(fmt.Sprintf("%s/%s", size.name, mode.name), func(t *testing.T) {
				r := newNPRigWith(t, mode.opts, func(r *npRig) { r.width, r.height = size.w, size.h })
				loader := kit.NewArtLoaderFrom(backdropArt{}, r.h.Main.Post)
				t.Cleanup(loader.Close)
				r.h.Env.Art = loader
				r.lyr.set(services.Lyrics{Timing: "Word", Lines: sketchLines()}, nil)
				r.play(12_900, func(s *services.PlaybackState) {
					s.Current.Backdrop = "/art/k1/backdrop.jpg"
					s.Current.Art.Small = "/art/k1/art.jpg"
					s.Current.Art.Large = "/art/k1/art.jpg"
				})
				r.p.ui.openRoom()
				r.tt.Until("the view", r.tt.Shows("Full-Screen Lyrics"))
				r.tt.Until("the wash to load", func() bool { return loader.Pending() == 0 })
				r.settle()
				r.advance(300 * time.Millisecond)
				r.advance(300 * time.Millisecond)
				r.wake()
				r.tt.Until("the strip", r.tt.Shows("Exit full screen"))
				r.advance(300 * time.Millisecond)
				uitest.Golden(t, r.tt.Image(), "full-screen-lyrics-strip-"+size.name+"-"+mode.name)
			})
		}
	}
}
