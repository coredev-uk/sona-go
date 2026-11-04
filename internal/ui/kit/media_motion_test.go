package kit

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"io"
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
	gifRed  = color.RGBA{204, 0, 0, 255}
	gifBlue = color.RGBA{0, 0, 204, 255}
)

// gifFrames are an animated GIF of two 32×32 frames, red then blue, 100 ms each.
func gifFrames(t testing.TB) []byte {
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
type closing struct {
	ui.VideoDecoder
	closed *atomic.Int32
}

func (d closing) Close() error {
	d.closed.Add(1)
	if c, ok := d.VideoDecoder.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// fakeOpen stands in for the motion cache and the decoder: it makes players
// of an animated GIF.
type fakeOpen struct {
	t      testing.TB
	mu     sync.Mutex
	calls  []openCall
	fail   error
	gate   chan struct{}
	closed atomic.Int32
}

type openCall struct {
	url string
	px  int
}

func (f *fakeOpen) open(ctx context.Context, url string, px int) (*ui.VideoPlayer, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, openCall{url, px})
	fail := f.fail
	f.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	d, err := ui.NewGIFDecoder(gifFrames(f.t))
	if err != nil {
		return nil, err
	}
	p, err := ui.NewVideoPlayer(closing{d, &f.closed})
	if err != nil {
		return nil, err
	}
	return p.SetLoop(true), nil
}

func (f *fakeOpen) opened() []openCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]openCall(nil), f.calls...)
}

// waitClosed waits for n decoders to have been closed (the player closes its
// decoder on a goroutine of its own).
func (f *fakeOpen) waitClosed(t *testing.T, n int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for f.closed.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d decoders closed, want %d", f.closed.Load(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

type motionFiles struct{}

func (motionFiles) Local(context.Context, string, int, string) (string, error) { return "", nil }

// motionEnv is a policy over a fake decoder, in a light theme, with the
// settings the policy reads under the test's control.
type motionEnv struct {
	p       *MotionPolicy
	f       *fakeOpen
	bus     *store.Bus
	mt      *mainThread
	setting *atomic.Bool
}

func newMotionEnv(t *testing.T) *motionEnv {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() {
		theme.SetMode(theme.ModeSystem)
		theme.SetReduceEffects(false)
		theme.SetReducedMotion(false)
	})
	e := &motionEnv{f: &fakeOpen{t: t}, bus: store.NewBus(nil), mt: newMainThread(), setting: &atomic.Bool{}}
	e.bus.SetPoster(nil)
	e.setting.Store(true)
	e.p = NewMotionPolicy(e.bus, e.mt.post, MotionOptions{Files: motionFiles{}, Setting: e.setting.Load})
	e.p.open = e.f.open
	e.p.codec = func() string { return "avc" }
	t.Cleanup(e.p.Close)
	return e
}

func TestMotionPolicyAllowsMotionOnlyWhenNothingArguesAgainstIt(t *testing.T) {
	e := newMotionEnv(t)
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

	e.p.codec = func() string { return "" }
	if e.p.Allowed() {
		t.Error("allowed with no decoder")
	}
}

func TestMotionPolicyStartsFromThePowerSnapshotAndNeedsFiles(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	p := NewMotionPolicy(nil, nil, MotionOptions{Files: motionFiles{}, Power: func() services.PowerState { return services.PowerState{OnBattery: true} }})
	p.codec = func() string { return "avc" }
	defer p.Close()
	if p.Allowed() {
		t.Error("allowed on battery at the start")
	}
	// No motion cache, no motion.
	q := NewMotionPolicy(nil, nil, MotionOptions{})
	q.codec = func() string { return "avc" }
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
	e := newMotionEnv(t)
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

type coverApp struct {
	e      *motionEnv
	l      *ArtLoader
	motion string
	size   float32
	opts   MotionOpts
}

func (a *coverApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(10).Children(func() {
		MotionCover(c.Key("m"), a.e.p, a.l, "/art/k1/300x300.jpg", a.motion, a.size, a.opts)
	})
}

func newCoverApp(t *testing.T, e *motionEnv, motion string) (*coverApp, *ui.Tester) {
	t.Helper()
	a := &coverApp{e: e, motion: motion, size: 100}
	a.l = newArtLoader(&fakeArt{}, e.mt.post, 2)
	t.Cleanup(a.l.Close)
	return a, ui.NewTester(a.view, 200, 200)
}

// frames builds the view twice (the second sees what the first laid out) and
// runs what posted.
func frames(t *testing.T, tt *ui.Tester, mt *mainThread) {
	t.Helper()
	tt.Frame()
	mt.run(t, 0)
	tt.Frame()
}

// settle runs frames and posted funcs until the n-th decoder has opened and
// its outcome has come in (no slot is loading).
func settle(t *testing.T, tt *ui.Tester, e *motionEnv, opens int) {
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
func (e *motionEnv) loading() bool {
	e.p.mu.Lock()
	defer e.p.mu.Unlock()
	for s := range e.p.slots {
		if s.state == slotLoading {
			return true
		}
	}
	return false
}

// isMoving reports whether the pixel at the centre of the cover is one of the
// GIF's two frames (not the still's --hover or the art's red).
func isMoving(tt *ui.Tester) bool {
	got := at(tt, 60, 60)
	return near(got, gifRed) || near(got, gifBlue)
}

func TestMotionCoverPlaysOverTheStillAtTheRequestedRendition(t *testing.T) {
	e := newMotionEnv(t)
	a, tt := newCoverApp(t, e, "/motion/k1/720.mp4")
	a.opts.VideoSize = 0
	a.l.SetScale(1.5) // the window's scale, as the app tells it before the first frame
	tt.SetScale(1.5)
	settle(t, tt, e, 1)
	tt.Frame()
	tt.Advance(motionFade + 50*time.Millisecond)
	tt.Frame()
	calls := e.f.opened()
	// 100 DIPs at 1.5 is 150 device pixels, which the cache buckets itself.
	if len(calls) != 1 || calls[0].url != "/motion/k1/720.mp4" || calls[0].px != 150 {
		t.Fatalf("opened %v, want the cover's URL at 150 px", calls)
	}
	if !isMoving(tt) {
		t.Errorf("the video is not drawn over the still: centre %v", at(tt, 60, 60))
	}
	if r := e.p.Report(); r.Playing != 1 || r.Last != "playing" || !r.Allowed {
		t.Errorf("report %+v", r)
	}
}

func TestMotionCoverAsksForTheVideoSizeNotTheCoversWhenGiven(t *testing.T) {
	e := newMotionEnv(t)
	a, tt := newCoverApp(t, e, "/motion/k1/720.mp4")
	a.size, a.opts.VideoSize = 100, 420
	settle(t, tt, e, 1)
	if calls := e.f.opened(); calls[0].px != 420 {
		t.Errorf("asked for %d px, want the 420 VideoSize", calls[0].px)
	}
}

func TestMotionCoverShowsOnlyTheStillWhereMotionIsNotAllowed(t *testing.T) {
	for name, off := range map[string]func(*motionEnv){
		"setting off":    func(e *motionEnv) { e.setting.Store(false) },
		"reduce effects": func(*motionEnv) { theme.SetReduceEffects(true) },
		"reduced motion": func(*motionEnv) { theme.SetReducedMotion(true) },
		"battery": func(e *motionEnv) {
			e.bus.Emit(services.EventPower, services.PowerState{OnBattery: true})
		},
		"no decoder": func(e *motionEnv) { e.p.codec = func() string { return "" } },
	} {
		t.Run(name, func(t *testing.T) {
			e := newMotionEnv(t)
			off(e)
			_, tt := newCoverApp(t, e, "/motion/k1/720.mp4")
			frames(t, tt, e.mt)
			time.Sleep(20 * time.Millisecond)
			frames(t, tt, e.mt)
			if n := len(e.f.opened()); n != 0 {
				t.Errorf("%d decoders opened where motion is off", n)
			}
		})
	}
	// And a cover with no motion URL opens nothing.
	e := newMotionEnv(t)
	_, tt := newCoverApp(t, e, "")
	frames(t, tt, e.mt)
	time.Sleep(20 * time.Millisecond)
	frames(t, tt, e.mt)
	if n := len(e.f.opened()); n != 0 {
		t.Errorf("%d decoders opened with no motion URL", n)
	}
}

func TestMotionCoverKeepsTheStillWhenTheVideoFails(t *testing.T) {
	e := newMotionEnv(t)
	e.f.fail = errors.New("download failed")
	_, tt := newCoverApp(t, e, "/motion/k1/720.mp4")
	settle(t, tt, e, 1)
	tt.Advance(time.Second)
	tt.Frame()
	if isMoving(tt) {
		t.Error("a video is drawn after the download failed")
	}
	if r := e.p.Report(); r.Last != "download-failed: download failed" || r.Playing != 0 {
		t.Errorf("report %+v", r)
	}
	// It is not asked for again every frame.
	for range 3 {
		frames(t, tt, e.mt)
	}
	if n := len(e.f.opened()); n != 1 {
		t.Errorf("%d attempts after one failure", n)
	}
}

// slotOf is the one registered slot.
func slotOf(t *testing.T, p *MotionPolicy) *motionSlot {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	for s := range p.slots {
		return s
	}
	t.Fatal("no slot registered")
	return nil
}

func TestMotionCoverPausesWhileCoveredHiddenOrOffScreen(t *testing.T) {
	e := newMotionEnv(t)
	a, tt := newCoverApp(t, e, "/motion/k1/720.mp4")
	settle(t, tt, e, 1)
	tt.Frame()
	s := slotOf(t, e.p)
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
	e := newMotionEnv(t)
	_, tt := newCoverApp(t, e, "/motion/k1/720.mp4")
	settle(t, tt, e, 1)
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
	settle(t, tt, e, 2)
	s := slotOf(t, e.p)

	// A view that is gone builds nothing, so the sweep closes the decoder.
	later := time.Now().Add(idleAfter + time.Second)
	e.p.now = func() time.Time { return later }
	e.p.release(e.p.stale())
	e.f.waitClosed(t, 2)
	if s.state != slotIdle || s.player != nil {
		t.Errorf("the swept slot is %v with a player %v", s.state, s.player)
	}
	// While the window is hidden nothing is swept.
	settle(t, tt, e, 3)
	e.p.SetVisible(false)
	e.p.now = func() time.Time { return later.Add(time.Hour) }
	e.p.release(e.p.stale())
	if e.f.closed.Load() != 2 {
		t.Errorf("a hidden window's decoder was swept")
	}
}

func TestMotionCoverClosesALateDecoderNobodyWantsAnyMore(t *testing.T) {
	e := newMotionEnv(t)
	e.f.gate = make(chan struct{})
	_, tt := newCoverApp(t, e, "/motion/k1/720.mp4")
	tt.Frame() // the open starts and waits at the gate
	e.setting.Store(false)
	tt.Frame() // nothing wants it now: the slot is released, the load cancelled
	close(e.f.gate)
	time.Sleep(50 * time.Millisecond)
	e.mt.run(t, 0)
	tt.Frame()
	if r := e.p.Report(); r.Playing != 0 {
		t.Errorf("a cancelled load is playing: %+v", r)
	}
}

type tilesApp struct {
	e   *motionEnv
	l   *ArtLoader
	hot [2]bool
	url [2]string
}

func (a *tilesApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Row(c).Padding(10).Gap(10).Children(func() {
		for i := range a.hot {
			ui.Box(c.Key(i)).Size(80, 80).Background(theme.C("hover")).Children(func() {
				TileMotion(c.Key("tile"), a.e.p, a.l, a.url[i], 80, TileMotionOpts{Hot: a.hot[i]})
			})
		}
	})
}

func newTilesApp(t *testing.T, e *motionEnv) (*tilesApp, *ui.Tester) {
	t.Helper()
	a := &tilesApp{e: e, url: [2]string{"/motion/k1/720.mp4", "/motion/k2/720.mp4"}}
	a.l = newArtLoader(&fakeArt{}, e.mt.post, 1)
	t.Cleanup(a.l.Close)
	return a, ui.NewTester(a.view, 220, 100)
}

func TestTileMotionPlaysAfterTheDwellOnly(t *testing.T) {
	e := newMotionEnv(t)
	a, tt := newTilesApp(t, e)
	a.l.SetScale(2)
	tt.SetScale(2)
	frames(t, tt, e.mt)
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
	settle(t, tt, e, 1)
	calls := e.f.opened()
	// The 180 DIP rendition at 2x.
	if len(calls) != 1 || calls[0].url != "/motion/k1/720.mp4" || calls[0].px != 360 {
		t.Fatalf("opened %v, want the first tile's motion at 360 px", calls)
	}
	tt.Advance(motionFade + 50*time.Millisecond)
	tt.Frame()
	if got := at(tt, 50, 50); !near(got, gifRed) && !near(got, gifBlue) {
		t.Errorf("tile video not drawn: %v", got)
	}

	// Cold again: the decoder goes.
	a.hot[0] = false
	tt.Frame()
	e.f.waitClosed(t, 1)
}

func TestTileMotionPlaysOneTileAtATime(t *testing.T) {
	e := newMotionEnv(t)
	a, tt := newTilesApp(t, e)
	frames(t, tt, e.mt)
	a.hot[0] = true
	tt.Frame()
	tt.Advance(450 * time.Millisecond)
	settle(t, tt, e, 1)

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
	settle(t, tt, e, 2)
	if calls := e.f.opened(); calls[1].url != "/motion/k2/720.mp4" {
		t.Errorf("the second tile's motion did not play: %v", calls)
	}
}

func TestTileMotionNothingWithoutMotionOrWhereNotAllowed(t *testing.T) {
	e := newMotionEnv(t)
	a, tt := newTilesApp(t, e)
	a.url[0] = "" // Apple has no motion for this tile
	a.hot[0] = true
	frames(t, tt, e.mt)
	tt.Advance(time.Second)
	frames(t, tt, e.mt)
	if n := len(e.f.opened()); n != 0 {
		t.Errorf("a tile with no motion opened %d decoders", n)
	}

	e = newMotionEnv(t)
	e.setting.Store(false)
	a, tt = newTilesApp(t, e)
	a.hot[1] = true
	frames(t, tt, e.mt)
	tt.Advance(time.Second)
	frames(t, tt, e.mt)
	if n := len(e.f.opened()); n != 0 {
		t.Errorf("a tile opened %d decoders with motion off", n)
	}

	// A nil policy is nothing at all.
	var nilApp tilesApp
	nilApp.url = a.url
	nilApp.l = a.l
	nilApp.e = &motionEnv{}
	nilApp.hot[1] = true
	tt = ui.NewTester(nilApp.view, 220, 100)
	frames(t, tt, e.mt)
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
	files := &localFile{path: path}
	p := NewMotionPolicy(nil, mt.post, MotionOptions{Files: files})
	defer p.Close()
	e := &motionEnv{p: p, mt: mt, f: &fakeOpen{t: t}}
	_, tt := newCoverApp(t, e, "/motion/k1/720.mp4")
	deadline := time.Now().Add(10 * time.Second)
	for {
		tt.Frame()
		mt.run(t, 0)
		if s := onlySlot(p); s != nil && s.state == slotReady {
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

type localFile struct {
	path string
	mu   sync.Mutex
	got  string
}

func (f *localFile) asked() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got
}

func (f *localFile) Local(_ context.Context, url string, _ int, codec string) (string, error) {
	f.mu.Lock()
	f.got = url
	f.mu.Unlock()
	if codec != "avc" {
		return "", errors.New("codec " + codec)
	}
	return f.path, nil
}

func onlySlot(p *MotionPolicy) *motionSlot {
	p.mu.Lock()
	defer p.mu.Unlock()
	for s := range p.slots {
		return s
	}
	return nil
}
