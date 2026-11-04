package kit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/media"
	"github.com/coredev-uk/sona-go/internal/ui/media/mediatest"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The GIF's two frames, in the Plan9 palette's nearest colours.
var (
	mediaGifRed  = color.RGBA{204, 0, 0, 255}
	mediaGifBlue = color.RGBA{0, 0, 204, 255}
)

// mediaGifFrames are an animated GIF of two 32×32 frames, red then blue, 100 ms each.
func mediaGifFrames(t testing.TB) []byte {
	t.Helper()
	g := &gif.GIF{LoopCount: 0}
	for _, c := range []color.RGBA{{200, 30, 30, 255}, {30, 30, 200, 255}} {
		img := image.NewPaletted(image.Rect(0, 0, 32, 32), palette.Plan9)
		for i := range img.Pix {
			img.Pix[i] = uint8(img.Palette.Index(c))
		}
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// closing is a decoder that counts the closes it gets.
type mediaClosing struct {
	ui.VideoDecoder
	closed *atomic.Int32
}

func (d mediaClosing) Close() error {
	d.closed.Add(1)
	if c, ok := d.VideoDecoder.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// mediaFakeOpen stands in for the motion cache and the decoder: it makes players
// of an animated GIF.
type mediaFakeOpen struct {
	t     testing.TB
	mu    sync.Mutex
	calls []mediaOpenCall
	fail  error
	gate  chan struct{}
	// ignoreCancel makes a gated open finish even if it was cancelled, as a
	// decoder that was already opening when the view went away does.
	ignoreCancel bool
	closed       atomic.Int32
}

type mediaOpenCall struct {
	url string
	px  int
}

func (f *mediaFakeOpen) open(ctx context.Context, url string, px int) (*ui.VideoPlayer, error) {
	if f.gate != nil {
		if f.ignoreCancel {
			<-f.gate
		} else {
			select {
			case <-f.gate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, mediaOpenCall{url, px})
	fail := f.fail
	f.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	d, err := ui.NewGIFDecoder(mediaGifFrames(f.t))
	if err != nil {
		return nil, err
	}
	p, err := ui.NewVideoPlayer(mediaClosing{d, &f.closed})
	if err != nil {
		return nil, err
	}
	return p.SetLoop(true), nil
}

func (f *mediaFakeOpen) opened() []mediaOpenCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]mediaOpenCall(nil), f.calls...)
}

// waitClosed waits for n decoders to have been closed (the player closes its
// decoder on a goroutine of its own).
func (f *mediaFakeOpen) waitClosed(t *testing.T, n int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for f.closed.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d decoders closed, want %d", f.closed.Load(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

type mediaMotionFiles struct{}

func (mediaMotionFiles) Local(context.Context, string, int, string) (string, error) { return "", nil }

// mediaMotionEnv is a policy over a fake decoder, in a light theme, with the
// settings the policy reads under the test's control.
type mediaMotionEnv struct {
	p       *MotionPolicy
	f       *mediaFakeOpen
	bus     *store.Bus
	mt      *mainThread
	setting *atomic.Bool
}

func newMediaMotionEnv(t *testing.T) *mediaMotionEnv {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() {
		theme.SetMode(theme.ModeSystem)
		theme.SetReduceEffects(false)
		theme.SetReducedMotion(false)
	})
	e := &mediaMotionEnv{f: &mediaFakeOpen{t: t}, bus: store.NewBus(nil), mt: newMainThread(), setting: &atomic.Bool{}}
	e.bus.SetPoster(nil)
	e.setting.Store(true)
	e.p = NewMotionPolicy(e.bus, e.mt.post, MotionOptions{Files: mediaMotionFiles{}, Setting: e.setting.Load})
	e.p.open = e.f.open
	e.p.setCodec("avc")
	t.Cleanup(e.p.Close)
	return e
}

func TestMotionPolicyAllowsMotionOnlyWhenNothingArguesAgainstIt(t *testing.T) {
	e := newMediaMotionEnv(t)
	if !e.p.Allowed() {
		t.Fatal("not allowed with the setting on, nothing reduced, on mains power")
	}
	e.setting.Store(false)
	if e.p.Allowed() {
		t.Error("allowed with the setting off")
	}
	e.setting.Store(true)

	theme.SetReduceEffects(true)
	if e.p.Allowed() {
		t.Error("allowed under reduce effects")
	}
	theme.SetReduceEffects(false)
	theme.SetReducedMotion(true)
	if e.p.Allowed() {
		t.Error("allowed under reduced motion")
	}
	theme.SetReducedMotion(false)

	// Battery, from Go's event; mains again clears it.
	e.bus.Emit(services.EventPower, services.PowerState{Followed: true, OnBattery: true, Source: "battery"})
	if e.p.Allowed() {
		t.Error("allowed on battery")
	}
	e.bus.Emit(services.EventPower, services.PowerState{Followed: true, Source: "ac"})
	if !e.p.Allowed() {
		t.Error("not allowed again on mains power")
	}

	e.p.setCodec("")
	if e.p.Allowed() {
		t.Error("allowed with no decoder")
	}
}

func TestMotionPolicyStartsFromThePowerSnapshotAndNeedsFiles(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	p := NewMotionPolicy(nil, nil, MotionOptions{Files: mediaMotionFiles{}, Power: func() services.PowerState { return services.PowerState{OnBattery: true} }})
	p.setCodec("avc")
	defer p.Close()
	if p.Allowed() {
		t.Error("allowed on battery at the start")
	}
	// No motion cache, no motion.
	q := NewMotionPolicy(nil, nil, MotionOptions{})
	q.setCodec("avc")
	if q.Allowed() {
		t.Error("allowed with no files to play")
	}
	// A nil policy allows nothing and tolerates everything.
	var n *MotionPolicy
	n.SetVisible(false)
	n.Cover()()
	n.Close()
	if n.Allowed() || n.Covered() || n.Visible() {
		t.Error("a nil policy allowed something")
	}
	if r := n.Report(); r.Allowed || r.Playing != 0 {
		t.Errorf("nil report %+v", r)
	}
}

func TestMotionPolicyCoverNestsAndReleasesOnce(t *testing.T) {
	e := newMediaMotionEnv(t)
	a := e.p.Cover()
	b := e.p.Cover()
	if !e.p.Covered() {
		t.Fatal("not covered")
	}
	a()
	a() // a second release is nothing
	if !e.p.Covered() {
		t.Error("one release uncovered a doubly covered page")
	}
	b()
	if e.p.Covered() {
		t.Error("still covered")
	}
	e.p.SetVisible(false)
	if e.p.Visible() {
		t.Error("visible after SetVisible(false)")
	}
}

func TestMotionPx(t *testing.T) {
	for _, tc := range []struct {
		size, scale float32
		want        int
	}{
		{180, 1, 180}, {180, 2, 360}, {180, 3, 360}, // density at most 2
		{420, 1.5, 630}, {200, 0.5, 200}, // and at least 1
		{4, 1, 16}, {5000, 1, 2048}, // the cache serves 16–2048
	} {
		if got := motionPx(tc.size, tc.scale); got != tc.want {
			t.Errorf("motionPx(%v, %v) = %d, want %d", tc.size, tc.scale, got, tc.want)
		}
	}
}

type mediaCoverApp struct {
	e      *mediaMotionEnv
	l      *ArtLoader
	motion string
	size   float32
	top    float32 // padding above the cover: far enough, it is out of the window
	opts   MotionOpts
}

func (a *mediaCoverApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(10).PaddingY(10 + a.top).Children(func() {
		MotionCover(c.Key("m"), a.e.p, a.l, "/art/k1/300x300.jpg", a.motion, a.size, a.opts)
	})
}

func newMediaCoverApp(t *testing.T, e *mediaMotionEnv, motion string) (*mediaCoverApp, *ui.Tester) {
	t.Helper()
	a := &mediaCoverApp{e: e, motion: motion, size: 100}
	a.l = newArtLoader(&fakeArt{}, e.mt.post, 2)
	t.Cleanup(a.l.Close)
	return a, ui.NewTester(a.view, 200, 200)
}

// frames builds the view twice (the second sees what the first laid out) and
// runs what posted.
// mediaSetClock moves the policy's clock.
func mediaSetClock(p *MotionPolicy, f func() time.Time) {
	p.mu.Lock()
	p.now = f
	p.mu.Unlock()
}

func mediaFrames(t *testing.T, tt *ui.Tester, mt *mainThread) {
	t.Helper()
	tt.Frame()
	mt.run(t, 0)
	tt.Frame()
}

// mediaSettle runs frames and posted funcs until the n-th decoder has opened and
// its outcome has come in (no slot is loading).
func mediaSettle(t *testing.T, tt *ui.Tester, e *mediaMotionEnv, opens int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		tt.Frame()
		e.mt.run(t, 0)
		tt.Frame()
		if len(e.f.opened()) >= opens && !e.loading() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d opens, want %d", len(e.f.opened()), opens)
		}
		time.Sleep(time.Millisecond)
	}
}

// loading is whether any slot waits for its decoder.
func (e *mediaMotionEnv) loading() bool {
	e.p.mu.Lock()
	defer e.p.mu.Unlock()
	for s := range e.p.slots {
		if s.state == slotLoading {
			return true
		}
	}
	return false
}

// mediaIsMoving reports whether the pixel at the centre of the cover is one of the
// GIF's two frames (not the still's --hover or the art's red).
func mediaIsMoving(tt *ui.Tester) bool {
	got := at(tt, 60, 60)
	return near(got, mediaGifRed) || near(got, mediaGifBlue)
}

func TestMotionCoverPlaysOverTheStillAtTheRequestedRendition(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	a.opts.VideoSize = 0
	a.l.SetScale(1.5) // the window's scale, as the app tells it before the first frame
	tt.SetScale(1.5)
	mediaSettle(t, tt, e, 1)
	tt.Frame()
	tt.Advance(motionFade + 50*time.Millisecond)
	tt.Frame()
	calls := e.f.opened()
	// 100 DIPs at 1.5 is 150 device pixels, which the cache buckets itself.
	if len(calls) != 1 || calls[0].url != "/motion/k1/720.mp4" || calls[0].px != 150 {
		t.Fatalf("opened %v, want the cover's URL at 150 px", calls)
	}
	if !mediaIsMoving(tt) {
		t.Errorf("the video is not drawn over the still: centre %v", at(tt, 60, 60))
	}
	if r := e.p.Report(); r.Playing != 1 || r.Last != "playing" || !r.Allowed {
		t.Errorf("report %+v", r)
	}
}

func TestMotionCoverAsksForTheVideoSizeNotTheCoversWhenGiven(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	a.size, a.opts.VideoSize = 100, 420
	mediaSettle(t, tt, e, 1)
	if calls := e.f.opened(); calls[0].px != 420 {
		t.Errorf("asked for %d px, want the 420 VideoSize", calls[0].px)
	}
}

func TestMotionCoverShowsOnlyTheStillWhereMotionIsNotAllowed(t *testing.T) {
	for name, off := range map[string]func(*mediaMotionEnv){
		"setting off":    func(e *mediaMotionEnv) { e.setting.Store(false) },
		"reduce effects": func(*mediaMotionEnv) { theme.SetReduceEffects(true) },
		"reduced motion": func(*mediaMotionEnv) { theme.SetReducedMotion(true) },
		"battery": func(e *mediaMotionEnv) {
			e.bus.Emit(services.EventPower, services.PowerState{OnBattery: true})
		},
		"no decoder": func(e *mediaMotionEnv) { e.p.setCodec("") },
	} {
		t.Run(name, func(t *testing.T) {
			e := newMediaMotionEnv(t)
			off(e)
			_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
			mediaFrames(t, tt, e.mt)
			time.Sleep(20 * time.Millisecond)
			mediaFrames(t, tt, e.mt)
			if n := len(e.f.opened()); n != 0 {
				t.Errorf("%d decoders opened where motion is off", n)
			}
		})
	}
	// And a cover with no motion URL opens nothing.
	e := newMediaMotionEnv(t)
	_, tt := newMediaCoverApp(t, e, "")
	mediaFrames(t, tt, e.mt)
	time.Sleep(20 * time.Millisecond)
	mediaFrames(t, tt, e.mt)
	if n := len(e.f.opened()); n != 0 {
		t.Errorf("%d decoders opened with no motion URL", n)
	}
}

func TestMotionCoverKeepsTheStillWhenTheVideoFails(t *testing.T) {
	e := newMediaMotionEnv(t)
	e.f.fail = errors.New("download failed")
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	mediaSettle(t, tt, e, 1)
	tt.Advance(time.Second)
	tt.Frame()
	if mediaIsMoving(tt) {
		t.Error("a video is drawn after the download failed")
	}
	if r := e.p.Report(); r.Last != "download-failed: download failed" || r.Playing != 0 {
		t.Errorf("report %+v", r)
	}
	// It is not asked for again every frame.
	for range 3 {
		mediaFrames(t, tt, e.mt)
	}
	if n := len(e.f.opened()); n != 1 {
		t.Errorf("%d attempts after one failure", n)
	}
}

// mediaSlotOf is the one registered slot.
func mediaSlotOf(t *testing.T, p *MotionPolicy) *motionSlot {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	for s := range p.slots {
		return s
	}
	t.Fatal("no slot registered")
	return nil
}

func TestMotionCoverPausesWhileCoveredOrHidden(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	mediaSettle(t, tt, e, 1)
	tt.Frame()
	s := mediaSlotOf(t, e.p)
	if !s.player.Playing() {
		t.Fatal("the video is not playing on screen")
	}

	release := e.p.Cover()
	tt.Frame()
	if s.player.Playing() {
		t.Error("the video plays under a covering overlay")
	}
	release()
	tt.Frame()
	if !s.player.Playing() {
		t.Error("the video did not resume when the page was uncovered")
	}

	e.p.SetVisible(false)
	tt.Frame()
	if s.player.Playing() {
		t.Error("the video plays in a hidden window")
	}
	e.p.SetVisible(true)
	tt.Frame()
	if !s.player.Playing() {
		t.Error("the video did not resume when the window showed")
	}

	// A cover that is part of the overlay keeps playing while the page is covered.
	a.opts.Overlay = true
	e.p.Cover()
	tt.Frame()
	if !s.player.Playing() {
		t.Error("an overlay's own cover paused under itself")
	}
}

func TestMotionCoverDropsItsDecoderWhenMotionStopsOrTheViewIsGone(t *testing.T) {
	e := newMediaMotionEnv(t)
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	mediaSettle(t, tt, e, 1)
	tt.Frame()
	if e.f.closed.Load() != 0 {
		t.Fatal("closed early")
	}
	// Motion no longer allowed: the decoder goes at once.
	e.setting.Store(false)
	tt.Frame()
	e.f.waitClosed(t, 1)
	if r := e.p.Report(); r.Playing != 0 {
		t.Errorf("%d videos playing after motion stopped", r.Playing)
	}

	// Allowed again: it opens anew.
	e.setting.Store(true)
	mediaSettle(t, tt, e, 2)
	s := mediaSlotOf(t, e.p)

	// A view that is gone builds nothing, so the sweep closes the decoder.
	later := time.Now().Add(idleAfter + time.Second)
	mediaSetClock(e.p, func() time.Time { return later })
	stale, _ := e.p.stale()
	e.p.release(stale)
	e.f.waitClosed(t, 2)
	if s.state != slotIdle || s.player != nil {
		t.Errorf("the swept slot is %v with a player %v", s.state, s.player)
	}
	// While the window is hidden nothing is swept.
	mediaSettle(t, tt, e, 3)
	e.p.SetVisible(false)
	mediaSetClock(e.p, func() time.Time { return later.Add(time.Hour) })
	stale, _ = e.p.stale()
	e.p.release(stale)
	if e.f.closed.Load() != 2 {
		t.Errorf("a hidden window's decoder was swept")
	}
}

func TestMotionCoverClosesALateDecoderNobodyWantsAnyMore(t *testing.T) {
	e := newMediaMotionEnv(t)
	e.f.gate, e.f.ignoreCancel = make(chan struct{}), true
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	tt.Frame() // the open starts and waits at the gate
	e.setting.Store(false)
	tt.Frame() // nothing wants it now: the slot is released, the load cancelled
	close(e.f.gate)
	// The decoder opens anyway, and is closed when it arrives: nobody plays it.
	deadline := time.Now().Add(5 * time.Second)
	for len(e.f.opened()) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("the late open never finished")
		}
		time.Sleep(time.Millisecond)
	}
	for deadline := time.Now().Add(5 * time.Second); e.f.closed.Load() < 1; time.Sleep(time.Millisecond) {
		e.mt.run(t, 0) // the late answer is among what posted
		if time.Now().After(deadline) {
			break
		}
	}
	e.f.waitClosed(t, 1)
	tt.Frame()
	if r := e.p.Report(); r.Playing != 0 {
		t.Errorf("a cancelled load is playing: %+v", r)
	}
}

func TestMotionCoverThatLoadsWhileCoveredOrOffScreenDoesNotPlay(t *testing.T) {
	// Covered by Now Playing while the file is still opening: the player
	// starts playing when made, and must be paused as soon as the view sees it.
	e := newMediaMotionEnv(t)
	e.f.gate = make(chan struct{})
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	tt.Frame()
	release := e.p.Cover()
	close(e.f.gate)
	mediaSettle(t, tt, e, 1)
	tt.Frame()
	s := mediaSlotOf(t, e.p)
	if s.state != slotReady || s.player.Playing() {
		t.Errorf("a video that loaded under an overlay plays: state %v", s.state)
	}
	if r := e.p.Report(); r.Playing != 0 {
		t.Errorf("report counts %d playing", r.Playing)
	}
	release()
	tt.Frame()
	if !s.player.Playing() {
		t.Error("it did not play once uncovered")
	}

	// Out of the window: the same.
	e = newMediaMotionEnv(t)
	a, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	a.top = 600
	mediaSettle(t, tt, e, 1)
	tt.Frame()
	s = mediaSlotOf(t, e.p)
	if s.state != slotReady || s.player.Playing() {
		t.Errorf("a cover out of view plays: state %v", s.state)
	}
	// Scrolled into view, it plays.
	a.top = 0
	tt.Frame()
	tt.Frame()
	if !s.player.Playing() {
		t.Error("it did not play once in view")
	}
}

func TestMotionPolicyReportsADecoderFailureAsAMediaError(t *testing.T) {
	e := newMediaMotionEnv(t)
	e.f.fail = fmt.Errorf("%w: no plugin", media.ErrDecode)
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	mediaSettle(t, tt, e, 1)
	if r := e.p.Report(); !strings.HasPrefix(r.Last, "media-error: ") {
		t.Errorf("last = %q, want a media-error", r.Last)
	}
}

func TestMotionPolicyProbesTheDecoderOffTheMainThread(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	mt := newMainThread()
	gate := make(chan struct{})
	var probes atomic.Int32
	p := NewMotionPolicy(nil, mt.post, MotionOptions{Files: mediaMotionFiles{}})
	p.probe = func() string { probes.Add(1); <-gate; return "avc" }
	defer p.Close()
	// The first ask returns at once with "no", however long GStreamer takes.
	done := make(chan bool)
	go func() { done <- p.Allowed() }()
	select {
	case ok := <-done:
		if ok {
			t.Error("allowed before the probe answered")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Allowed waited for the probe")
	}
	if p.Allowed() {
		t.Error("allowed while probing")
	}
	close(gate)
	mt.run(t, 1) // the answer is posted to the main thread, which redraws
	if !p.Allowed() {
		t.Error("not allowed once the probe found a decoder")
	}
	if probes.Load() != 1 {
		t.Errorf("%d probes", probes.Load())
	}
	if r := p.Report(); r.H264 != "probably" {
		t.Errorf("report %+v", r)
	}
}

func TestMotionPolicyProbedIsFalseUntilTheProbeAnswers(t *testing.T) {
	mt := newMainThread()
	gate := make(chan struct{})
	p := NewMotionPolicy(nil, mt.post, MotionOptions{Files: mediaMotionFiles{}})
	p.probe = func() string { <-gate; return "" }
	defer p.Close()
	if p.Probed() {
		t.Error("probed before the probe answered")
	}
	close(gate)
	mt.run(t, 1)
	if !p.Probed() {
		t.Error("not probed after the answer, even a negative one")
	}
	var n *MotionPolicy
	if n.Probed() {
		t.Error("a nil policy is probed")
	}
}

func TestMotionPolicySweepEndsWithTheLastSlotAndSleepsWhileHidden(t *testing.T) {
	sweeping := func(p *MotionPolicy) bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.sweeping
	}
	e := newMediaMotionEnv(t)
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	mediaSettle(t, tt, e, 1)
	if !sweeping(e.p) {
		t.Fatal("no sweep while a slot is registered")
	}
	s := mediaSlotOf(t, e.p)

	// Hidden window: a slot nothing builds is not swept, and nothing is
	// posted, however many ticks pass.
	e.p.SetVisible(false)
	later := time.Now().Add(idleAfter + time.Second)
	mediaSetClock(e.p, func() time.Time { return later })
	e.mt.run(t, 0) // what was already posted
	time.Sleep(sweepEvery + 200*time.Millisecond)
	if n := len(e.mt.q); n != 0 {
		t.Errorf("%d funcs posted while the window was hidden", n)
	}
	if s.state != slotReady {
		t.Error("a hidden window's decoder was swept")
	}
	// Shown again: the next tick finds the stale slot and posts its release.
	e.p.SetVisible(true)
	deadline := time.Now().Add(5 * time.Second)
	for s.state == slotReady {
		e.mt.run(t, 0)
		if time.Now().After(deadline) {
			t.Fatal("the stale slot was never swept")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// With no slot left the sweep ends by itself on a tick; no call of ours.
	time.Sleep(sweepEvery + 200*time.Millisecond)
	if sweeping(e.p) {
		t.Error("the sweep goes on with no slots")
	}
}

type mediaTilesApp struct {
	e   *mediaMotionEnv
	l   *ArtLoader
	hot [2]bool
	// focus marks a hot tile as having only the keyboard focus.
	focus [2]bool
	// gone drops a tile's view altogether (it is not built).
	gone [2]bool
	url  [2]string
}

func (a *mediaTilesApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Row(c).Padding(10).Gap(10).Children(func() {
		for i := range a.hot {
			if a.gone[i] {
				continue
			}
			ui.Box(c.Key(i)).Size(80, 80).Background(theme.C("hover")).Children(func() {
				TileMotion(c.Key("tile"), a.e.p, a.url[i], 80, TileMotionOpts{Hot: a.hot[i], Focus: a.focus[i]})
			})
		}
	})
}

func newMediaTilesApp(t *testing.T, e *mediaMotionEnv) (*mediaTilesApp, *ui.Tester) {
	t.Helper()
	a := &mediaTilesApp{e: e, url: [2]string{"/motion/k1/720.mp4", "/motion/k2/720.mp4"}}
	a.l = newArtLoader(&fakeArt{}, e.mt.post, 1)
	t.Cleanup(a.l.Close)
	e.p.opts.Art = a.l // the window's scale, for the tiles' video size
	return a, ui.NewTester(a.view, 220, 100)
}

func TestTileMotionPlaysAfterTheDwellOnly(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaTilesApp(t, e)
	a.l.SetScale(2)
	tt.SetScale(2)
	mediaFrames(t, tt, e.mt)
	if n := len(e.f.opened()); n != 0 {
		t.Fatalf("a tile at rest opened %d decoders", n)
	}

	a.hot[0] = true
	tt.Frame()
	tt.Advance(300 * time.Millisecond) // short of the dwell
	time.Sleep(20 * time.Millisecond)
	e.mt.run(t, 0)
	if n := len(e.f.opened()); n != 0 {
		t.Fatalf("a tile hot for 300 ms opened %d decoders", n)
	}
	tt.Advance(120 * time.Millisecond) // 420 ms
	mediaSettle(t, tt, e, 1)
	calls := e.f.opened()
	// The 180 DIP rendition at 2x.
	if len(calls) != 1 || calls[0].url != "/motion/k1/720.mp4" || calls[0].px != 360 {
		t.Fatalf("opened %v, want the first tile's motion at 360 px", calls)
	}
	tt.Advance(motionFade + 50*time.Millisecond)
	tt.Frame()
	if got := at(tt, 50, 50); !near(got, mediaGifRed) && !near(got, mediaGifBlue) {
		t.Errorf("tile video not drawn: %v", got)
	}

	// Cold again: the decoder goes.
	a.hot[0] = false
	tt.Frame()
	e.f.waitClosed(t, 1)
}

func TestTileMotionPlaysOneTileAtATime(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaTilesApp(t, e)
	mediaFrames(t, tt, e.mt)
	a.hot[0] = true
	tt.Frame()
	tt.Advance(450 * time.Millisecond)
	mediaSettle(t, tt, e, 1)

	// The second tile is hot too (focus on one, hover on the other): it waits.
	a.hot[1] = true
	tt.Frame()
	tt.Advance(450 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	e.mt.run(t, 0)
	tt.Frame()
	if n := len(e.f.opened()); n != 1 {
		t.Fatalf("%d decoders open for two hot tiles", n)
	}
	// The first goes cold: the second plays.
	a.hot[0] = false
	tt.Frame()
	e.f.waitClosed(t, 1)
	tt.Advance(100 * time.Millisecond)
	mediaSettle(t, tt, e, 2)
	if calls := e.f.opened(); calls[1].url != "/motion/k2/720.mp4" {
		t.Errorf("the second tile's motion did not play: %v", calls)
	}
}

func TestTileMotionPlaysTheHoveredTileBeforeTheFocusedOne(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaTilesApp(t, e)
	mediaFrames(t, tt, e.mt)
	// The first tile keeps the keyboard focus (after a click on its play
	// button) and plays.
	a.hot[0], a.focus[0] = true, true
	tt.Frame()
	tt.Advance(450 * time.Millisecond)
	mediaSettle(t, tt, e, 1)

	// Hovering the second takes over once its dwell is done: the first's
	// decoder goes at once, though it is still focused.
	a.hot[1] = true
	tt.Frame()
	tt.Advance(450 * time.Millisecond)
	mediaSettle(t, tt, e, 2)
	e.f.waitClosed(t, 1)
	if calls := e.f.opened(); calls[1].url != "/motion/k2/720.mp4" {
		t.Errorf("the hovered tile's motion did not play: %v", calls)
	}
	// The focused one waits for the owner's release, it does not reopen.
	tt.Frame()
	e.mt.run(t, 0)
	tt.Frame()
	if len(e.f.opened()) != 2 {
		t.Errorf("%d decoders opened", len(e.f.opened()))
	}
}

func TestTileMotionOwnerThatFailedOrVanishedDoesNotBlockOthers(t *testing.T) {
	e := newMediaMotionEnv(t)
	e.f.fail = errors.New("offline")
	a, tt := newMediaTilesApp(t, e)
	mediaFrames(t, tt, e.mt)
	a.hot[0] = true
	tt.Frame()
	tt.Advance(450 * time.Millisecond)
	mediaSettle(t, tt, e, 1) // the first tile's load failed: it owns nothing worth keeping
	e.f.mu.Lock()
	e.f.fail = nil
	e.f.mu.Unlock()
	a.hot[1] = true
	tt.Frame()
	tt.Advance(450 * time.Millisecond)
	mediaSettle(t, tt, e, 2)
	if calls := e.f.opened(); len(calls) != 2 || calls[1].url != "/motion/k2/720.mp4" {
		t.Errorf("a failed owner blocked the next tile: %v", calls)
	}
}

func TestTileMotionOwnerThatVanishedIsReplacedAfterTheStaleTime(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaTilesApp(t, e)
	mediaSetClock(e.p, tt.Now) // the policy ages owners by the window's clock
	mediaFrames(t, tt, e.mt)
	a.hot[0] = true
	tt.Frame()
	tt.Advance(450 * time.Millisecond)
	mediaSettle(t, tt, e, 1)

	// The first tile's view is dropped (a page change, a scrolled-away row):
	// it is never built again and releases nothing. The second is hovered.
	a.gone[0] = true
	a.hot[1] = true
	tt.Frame()
	tt.Advance(450 * time.Millisecond) // its dwell: the owner is not stale yet
	tt.Frame()
	if n := len(e.f.opened()); n != 1 {
		t.Fatalf("%d decoders open: the second tile took a live owner's place", n)
	}
	// It asked to be built again after the stale time, with no other event.
	// (Advance builds the view only if something asked for it: no Frame here.)
	tt.Advance(ownerStale + 50*time.Millisecond)
	for deadline := time.Now().Add(5 * time.Second); len(e.f.opened()) < 2; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the second tile never took over: nothing woke it after the stale time")
		}
	}
	e.f.waitClosed(t, 1)
	if calls := e.f.opened(); calls[1].url != "/motion/k2/720.mp4" {
		t.Errorf("the second tile did not take over: %v", calls)
	}
}

func TestTileMotionNothingWithoutMotionOrWhereNotAllowed(t *testing.T) {
	e := newMediaMotionEnv(t)
	a, tt := newMediaTilesApp(t, e)
	a.url[0] = "" // Apple has no motion for this tile
	a.hot[0] = true
	mediaFrames(t, tt, e.mt)
	tt.Advance(time.Second)
	mediaFrames(t, tt, e.mt)
	if n := len(e.f.opened()); n != 0 {
		t.Errorf("a tile with no motion opened %d decoders", n)
	}

	e = newMediaMotionEnv(t)
	e.setting.Store(false)
	a, tt = newMediaTilesApp(t, e)
	a.hot[1] = true
	mediaFrames(t, tt, e.mt)
	tt.Advance(time.Second)
	mediaFrames(t, tt, e.mt)
	if n := len(e.f.opened()); n != 0 {
		t.Errorf("a tile opened %d decoders with motion off", n)
	}

	// A nil policy is nothing at all.
	var nilApp mediaTilesApp
	nilApp.url = a.url
	nilApp.l = a.l
	nilApp.e = &mediaMotionEnv{}
	nilApp.hot[1] = true
	tt = ui.NewTester(nilApp.view, 220, 100)
	mediaFrames(t, tt, e.mt)
	tt.Advance(time.Second)
	tt.Frame()
}

// TestMotionCoverPlaysARealFile decodes a generated H.264 clip through
// GStreamer into a MotionCover: the whole path but the download. It skips
// without GStreamer's H.264 decoder and encoder.
func TestMotionCoverPlaysARealFile(t *testing.T) {
	if media.Codec() != "avc" {
		t.Skip("no H.264 decoder in this GStreamer")
	}
	path := mediatest.Clip(t, 60)
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	mt := newMainThread()
	files := &mediaLocalFile{path: path}
	p := NewMotionPolicy(nil, mt.post, MotionOptions{Files: files})
	defer p.Close()
	e := &mediaMotionEnv{p: p, mt: mt, f: &mediaFakeOpen{t: t}}
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	deadline := time.Now().Add(10 * time.Second)
	for {
		tt.Frame()
		mt.run(t, 0)
		if s := mediaOnlySlot(p); s != nil && s.state == slotReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the clip did not open")
		}
		time.Sleep(5 * time.Millisecond)
	}
	tt.Frame()
	tt.Advance(motionFade + 100*time.Millisecond)
	tt.Frame()
	// The clip's picture is drawn: not the still's red and not the placeholder.
	hover := theme.C("hover")
	if got := at(tt, 60, 60); near(got, color.RGBA{220, 30, 30, 255}) || near(got, color.RGBA{hover.R, hover.G, hover.B, 255}) {
		t.Errorf("the clip is not drawn: %v", got)
	}
	if got := files.asked(); got != "/motion/k1/720.mp4" {
		t.Errorf("the cache was asked for %q", got)
	}
}

type mediaLocalFile struct {
	path string
	mu   sync.Mutex
	got  string
}

func (f *mediaLocalFile) asked() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got
}

func (f *mediaLocalFile) Local(_ context.Context, url string, _ int, codec string) (string, error) {
	f.mu.Lock()
	f.got = url
	f.mu.Unlock()
	if codec != "avc" {
		return "", errors.New("codec " + codec)
	}
	return f.path, nil
}

func mediaOnlySlot(p *MotionPolicy) *motionSlot {
	p.mu.Lock()
	defer p.mu.Unlock()
	for s := range p.slots {
		return s
	}
	return nil
}
