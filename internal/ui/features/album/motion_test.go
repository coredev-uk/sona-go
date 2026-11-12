package album

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
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

// withMotion gives the page a motion policy that allows motion and opens
// every file through the fake.
func (r *rig) withMotion() *fakeMotion {
	f := &fakeMotion{}
	p := kit.NewTestMotionPolicy(r.h.Main.Post, f.open)
	r.t.Cleanup(p.Close)
	r.h.Env.Motion = p
	return f
}

// coverPx is the size the cover is drawn at, which the window at scale 1
// asks its motion file for.
func (r *rig) coverPx() int {
	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	if r.a.Width > 0 && r.a.Width < 960 {
		gutter = tk.Metric["gutter-narrow"]
	}
	return int(albumLayout(r.a.Width - 2*gutter - tk.Metric["scrollbar"]).cover)
}

// The window of a test inside a synctest bubble. MyGo rasterises a frame of
// 2 × 128k pixels or more on helper goroutines it shares between windows,
// which a bubble must not meet (sync.WaitGroup would be used from both
// sides), so it stays below that: the narrow layout.
const bubbleW, bubbleH = 640, 400

// idle runs the bubble's clock past what the page waits on (the item states
// gather their loads for 16 ms), lets every goroutine that started finish,
// and runs what they posted: what a cover was going to ask for has been
// asked by now.
func (r *rig) idle() {
	time.Sleep(time.Second)
	r.tt.Quiesce()
}

func withMotionURL(p *services.CatalogAlbum) { p.Header.Motion = "/motion/100/1.mp4" }

func TestMotionCoverAsksForTheAlbumsMotionWhenThePolicyAllows(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	f := r.withMotion()
	r.addCatalog(saltFlats(withMotionURL))
	r.shows("Salt Flats", "Songs on Salt Flats")
	r.tt.Until("the cover to ask for its motion", func() bool { return len(f.asked()) > 0 })
	r.tt.Settle()
	want := []motionAsk{{"/motion/100/1.mp4", r.coverPx()}}
	if got := f.asked(); len(got) != 1 || got[0] != want[0] {
		t.Errorf("the cover asked for %+v, want %+v", got, want)
	}
	// The video failed to open: the page is the same, with the still.
	r.wantTexts("Salt Flats", "Play Salt Flats")
}

func TestAnAlbumWithoutMotionAsksForNone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, "/album/100", bubbleW, bubbleH)
		f := r.withMotion()
		r.addCatalog(saltFlats())
		r.shows("Salt Flats")
		r.idle()
		if got := f.asked(); len(got) != 0 {
			t.Errorf("a cover with no motion file asked for %+v", got)
		}
	})
}

func TestMotionCoverStaysStillWhileMotionIsReduced(t *testing.T) {
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
				r := newRig(t, "/album/100", bubbleW, bubbleH)
				f := r.withMotion()
				r.addCatalog(saltFlats(withMotionURL))
				r.shows("Salt Flats", "Songs on Salt Flats")
				r.idle() // a request that got through has run by now
				if got := f.asked(); len(got) != 0 {
					t.Fatalf("the cover asked for %+v while %s is on", got, tc.name)
				}
				// The same page asks as soon as nothing argues against motion.
				tc.set(false)
				r.tt.Until("the cover to ask once motion is allowed", func() bool { return len(f.asked()) > 0 })
				if got := f.asked(); got[0].url != "/motion/100/1.mp4" {
					t.Errorf("the cover asked for %+v", got)
				}
			})
		})
	}
}
