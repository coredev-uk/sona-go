package artist

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// fakeMotion stands in for the motion cache and the decoder: it notes which
// file each cover asked for and at what size, and fails, so the still stays.
type fakeMotion struct {
	mu    sync.Mutex
	calls []motionAsk
}

type motionAsk struct {
	url string
	px  int
}

func (f *fakeMotion) open(_ context.Context, url string, px int) (*ui.VideoPlayer, error) {
	f.mu.Lock()
	f.calls = append(f.calls, motionAsk{url, px})
	f.mu.Unlock()
	return nil, errors.New("no decoder in this test")
}

func (f *fakeMotion) asked() []motionAsk {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]motionAsk(nil), f.calls...)
}

const headMotion = "/motion/657515/1.mp4"

// motionRig is the artist page whose header has motion artwork, over a motion
// policy that allows motion and opens every file through the fake.
func motionRig(t *testing.T, w, h int) (*rig, *fakeMotion) {
	t.Helper()
	f := &fakeMotion{}
	r := newRig(t, "/artist/657515", w, h, func(r *rig) {
		r.cat.page = radiohead()
		r.cat.page.Header.Motion = headMotion
		p := kit.NewTestMotionPolicy(r.h.Main.Post, f.open)
		t.Cleanup(p.Close)
		r.h.Env.Motion = p
	})
	return r, f
}

// idle runs the bubble's clock past what the page waits on (the item states
// gather their loads for 16 ms), lets every goroutine that started finish,
// and runs what they posted: what a cover was going to ask for has been
// asked by now.
func (r *rig) idle() {
	time.Sleep(time.Second)
	r.tt.Quiesce()
}

// The window of a test inside a synctest bubble. MyGo rasterises a frame of
// 2 × 128k pixels or more on helper goroutines it shares between windows,
// which a bubble must not meet (sync.WaitGroup would be used from both
// sides), so it stays below that.
const bubbleW, bubbleH = 640, 400

func TestHeadMotionCoverAsksForTheArtistsMotionWhenThePolicyAllows(t *testing.T) {
	r, f := motionRig(t, 1132, 1500)
	r.shows("Radiohead", "Discography")
	r.tt.Until("the portrait to ask for its motion", func() bool { return len(f.asked()) > 0 })
	r.tt.Settle()
	// The portrait asks for the 200 DIP rendition (kit.MotionOpts.VideoSize),
	// not for its own size, at the window's scale (1 here).
	if got, want := f.asked(), []motionAsk{{headMotion, 200}}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("the portrait asked for %+v, want %+v", got, want)
	}
	// The video failed to open: the page is the same, with the still.
	if !r.tt.HasText("Radiohead") || !r.tt.HasText("Discography") {
		t.Errorf("page %q", r.tt.Texts())
	}
}

func TestAnArtistWithoutMotionAsksForNone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeMotion{}
		r := newRig(t, "/artist/657515", bubbleW, bubbleH, func(r *rig) {
			r.cat.page = radiohead()
			p := kit.NewTestMotionPolicy(r.h.Main.Post, f.open)
			t.Cleanup(p.Close)
			r.h.Env.Motion = p
		})
		r.shows("Radiohead")
		r.idle()
		if got := f.asked(); len(got) != 0 {
			t.Errorf("a portrait with no motion file asked for %+v", got)
		}
	})
}

func TestHeadMotionCoverStaysStillWhileMotionIsReduced(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(bool)
	}{
		{"reduce effects", theme.SetReduceEffects},
		{"reduced motion", theme.SetReducedMotion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tc.set(true)
				t.Cleanup(func() { tc.set(false) })
				r, f := motionRig(t, bubbleW, bubbleH)
				r.shows("Radiohead")
				r.idle() // a request that got through has run by now
				if got := f.asked(); len(got) != 0 {
					t.Fatalf("the portrait asked for %+v while %s is on", got, tc.name)
				}
				// The same page asks as soon as nothing argues against motion.
				tc.set(false)
				r.tt.Until("the portrait to ask once motion is allowed", func() bool { return len(f.asked()) > 0 })
				if got := f.asked(); got[0].url != headMotion {
					t.Errorf("the portrait asked for %+v", got)
				}
			})
		})
	}
}
