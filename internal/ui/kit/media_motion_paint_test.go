package kit

import (
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// mediaRefresh is a 60 Hz display's frame, the step of the clock below.
const mediaRefresh = 16700 * time.Microsecond

// mediaPlay runs d of the window's time in steps of one refresh, giving the
// decoder goroutine a moment of real time in each (the Tester's clock does not
// wait for it), and returns the colours the cover's centre showed and how many
// times the view was built.
func mediaPlay(tt *ui.Tester, a *mediaCoverApp, at func() color.RGBA, d time.Duration) (seen map[color.RGBA]int, builds int) {
	seen = map[color.RGBA]int{}
	before := a.builds
	for range int(d / mediaRefresh) {
		tt.Advance(mediaRefresh)
		time.Sleep(time.Millisecond)
		seen[at()]++
	}
	return seen, a.builds - before
}

// TestMotionCoverPaintsItsFramesWithoutBuildingTheView: the GIF's frames last
// 100 ms, which is ten a second, and each of them paints the cover again;
// none builds the view (a ui.Video did, ten builds a second) or lays it out.
func TestMotionCoverPaintsItsFramesWithoutBuildingTheView(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	mediaSettle(t, tt, e, 1)
	tt.Advance(motionFade + 50*time.Millisecond) // the fade-in, which animates the view
	centre := func() color.RGBA { return at(tt, 60, 60) }

	seen, builds := mediaPlay(tt, a, centre, time.Second)
	t.Logf("%d builds in a second of a 10 fps video", builds)
	if builds > 2 {
		t.Errorf("%d builds in a second of a 10 fps video, want at most 2", builds)
	}
	var red, blue int
	for c, n := range seen {
		switch {
		case near(c, mediaGifRed):
			red += n
		case near(c, mediaGifBlue):
			blue += n
		}
	}
	// Each picture lasts 100 ms of the second's 59 refreshes: about 6 of each.
	if red < 3 || blue < 3 {
		t.Errorf("the picture showed red in %d refreshes and blue in %d: %v", red, blue, seen)
	}
}

// TestMotionCoverStandsStillWhilePausedAndPlaysOnAgain: a cover that is
// covered, or whose window is hidden, asks for no frames, and goes on when it
// shows again.
func TestMotionCoverStandsStillWhilePausedAndPlaysOnAgain(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	mediaSettle(t, tt, e, 1)
	tt.Advance(motionFade + 50*time.Millisecond)
	centre := func() color.RGBA { return at(tt, 60, 60) }
	if seen, _ := mediaPlay(tt, a, centre, 400*time.Millisecond); len(seen) < 2 {
		t.Fatalf("the video does not move: %v", seen)
	}

	release := e.p.Cover()
	tt.Frame() // the view says the page is covered
	seen, builds := mediaPlay(tt, a, centre, time.Second)
	if len(seen) != 1 || builds != 0 {
		t.Errorf("covered: %d pictures and %d builds in a second, want one picture and no builds: %v", len(seen), builds, seen)
	}

	release()
	tt.Frame()
	if seen, builds := mediaPlay(tt, a, centre, time.Second); len(seen) < 2 || builds > 2 {
		t.Errorf("uncovered: %d pictures and %d builds in a second, want the video to move again with at most 2 builds: %v", len(seen), builds, seen)
	}

	e.p.SetVisible(false)
	tt.Frame()
	if seen, builds := mediaPlay(tt, a, centre, time.Second); len(seen) != 1 || builds != 0 {
		t.Errorf("hidden: %d pictures and %d builds in a second, want one picture and no builds: %v", len(seen), builds, seen)
	}
}

// TestMotionCoverStopsAtReducedMotion: the desktop asks for less motion while
// the video plays: the still replaces it and nothing asks for a frame.
func TestMotionCoverStopsAtReducedMotion(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	mediaSettle(t, tt, e, 1)
	tt.Advance(motionFade + 50*time.Millisecond)
	centre := func() color.RGBA { return at(tt, 60, 60) }
	if seen, _ := mediaPlay(tt, a, centre, 400*time.Millisecond); len(seen) < 2 {
		t.Fatalf("the video does not move: %v", seen)
	}

	theme.SetReducedMotion(true)
	tt.Frame()
	tt.Frame()
	seen, builds := mediaPlay(tt, a, centre, time.Second)
	if builds != 0 {
		t.Errorf("%d builds in a second of reduced motion", builds)
	}
	for c := range seen {
		if near(c, mediaGifRed) || near(c, mediaGifBlue) {
			t.Errorf("the video still shows under reduced motion: %v", c)
		}
	}
}
