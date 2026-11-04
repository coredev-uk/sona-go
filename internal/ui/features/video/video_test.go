package video

import (
	"context"
	"image/color"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	pvideo "github.com/coredev-uk/sona-go/internal/playback/video"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeSvc records what the view asked of PlaybackService.
type fakeSvc struct {
	mu    sync.Mutex
	calls []string
	seeks []int64
	vols  []float64
	fail  error
	done  chan struct{}
}

func newFakeSvc() *fakeSvc { return &fakeSvc{done: make(chan struct{}, 32)} }

func (f *fakeSvc) record(name string) error {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	err := f.fail
	f.mu.Unlock()
	f.done <- struct{}{}
	return err
}

func (f *fakeSvc) Toggle(context.Context) error { return f.record("toggle") }
func (f *fakeSvc) Next(context.Context) error   { return f.record("next") }
func (f *fakeSvc) Prev(context.Context) error   { return f.record("prev") }
func (f *fakeSvc) Seek(_ context.Context, ms int64) error {
	f.mu.Lock()
	f.seeks = append(f.seeks, ms)
	f.mu.Unlock()
	return f.record("seek")
}

func (f *fakeSvc) SetVolume(_ context.Context, v float64) error {
	f.mu.Lock()
	f.vols = append(f.vols, v)
	f.mu.Unlock()
	return f.record("volume")
}

func (f *fakeSvc) PlayItems(context.Context, []services.TrackRef, int, services.PlayFrom) error {
	return f.record("play items")
}

// wait blocks until n calls have been recorded (they run on goroutines).
func (f *fakeSvc) wait(t *testing.T, n int) []string {
	t.Helper()
	for range n {
		select {
		case <-f.done:
		case <-time.After(5 * time.Second):
			t.Fatalf("expected %d service calls, got %v", n, f.snapshot())
		}
	}
	return f.snapshot()
}

func (f *fakeSvc) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// quiet reports that nothing was called (after the frames that would have).
func (f *fakeSvc) quiet(t *testing.T, what string) {
	t.Helper()
	select {
	case <-f.done:
		t.Errorf("%s: the service was called: %v", what, f.snapshot())
	case <-time.After(30 * time.Millisecond):
	}
}

// fakeCat answers CatalogService.
type fakeCat struct {
	mu     sync.Mutex
	vid    services.CatalogVideo
	err    error
	asked  int
	opened []string
	failed error
	done   chan struct{}
}

func (f *fakeCat) MusicVideo(_ context.Context, id string) (services.CatalogVideo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked++
	return f.vid, f.err
}

func (f *fakeCat) OpenInAppleMusic(url string) error {
	f.mu.Lock()
	f.opened = append(f.opened, url)
	err := f.failed
	f.mu.Unlock()
	if f.done != nil {
		f.done <- struct{}{}
	}
	return err
}

// wait blocks until a link was opened (OpenInAppleMusic runs on a goroutine).
func (f *fakeCat) wait(t *testing.T) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the link was never opened")
	}
}

func (f *fakeCat) set(fn func(*fakeCat)) { f.mu.Lock(); fn(f); f.mu.Unlock() }

// fakeQueue records what the page played.
type fakeQueue struct {
	videos [][]services.MusicVideo
	songs  [][]services.Song
	from   []services.PlayFrom
	opened func()
}

func (q *fakeQueue) PlayVideos(v []services.MusicVideo, _ int, opened func()) {
	q.videos = append(q.videos, v)
	q.opened = opened
}

func (q *fakeQueue) PlaySongs(s []services.Song, _ int, from services.PlayFrom) {
	q.songs = append(q.songs, s)
	q.from = append(q.from, from)
}

// fakeNav records the pages pushed.
type fakeNav struct{ pushed []string }

func (n *fakeNav) Push(p string)      { n.pushed = append(n.pushed, p) }
func (n *fakeNav) Replace(p string)   { n.pushed = append(n.pushed, p) }
func (n *fakeNav) Back()              {}
func (n *fakeNav) Forward()           {}
func (n *fakeNav) Path() string       { return "/" }
func (n *fakeNav) Location() string   { return "/" }
func (n *fakeNav) last() string       { return n.pushed[len(n.pushed)-1] }
func (n *fakeNav) went(p string) bool { return slices.Contains(n.pushed, p) }

// fakeWindow is the window the view goes full screen in.
type fakeWindow struct{ full bool }

type rig struct {
	t       *testing.T
	h       *uitest.Harness
	env     *module.Env
	v       *video
	svc     *fakeSvc
	cat     *fakeCat
	queue   *fakeQueue
	nav     *fakeNav
	win     *fakeWindow
	surface *pvideo.Surface
	caps    services.PlaybackCapabilities
	unknown bool // Go has not answered what the platform supports
	tt      *uitest.Session
}

// newRig builds the feature over the fakes. mount builds what the window
// shows: the page's route, or the layer.
func newRig(t *testing.T, dark bool, mount func(r *rig) func(c *ui.Context), w, h int) *rig {
	t.Helper()
	opts := []uitest.Option{uitest.Bare()}
	if dark {
		opts = append(opts, uitest.Dark())
	}
	return newRigWith(t, opts, mount, w, h)
}

// newRigWith is newRig over the options of uitest.New (a full Env has the
// artwork cache that serves covers).
func newRigWith(t *testing.T, opts []uitest.Option, mount func(r *rig) func(c *ui.Context), w, h int) *rig {
	t.Helper()
	hn := uitest.New(t, opts...)
	r := &rig{t: t, h: hn, env: hn.Env, svc: newFakeSvc(), cat: &fakeCat{}, queue: &fakeQueue{}, nav: &fakeNav{}, win: &fakeWindow{}, surface: pvideo.NewSurface()}
	r.caps = services.PlaybackCapabilities{Available: true, Video: true, Volume: true}
	r.env.Actions = kit.NewActionRegistry()
	r.env.Actions.Register(kit.ActionDef{
		ID: "test.play-next.video", Target: kit.KindMusicVideo, Group: kit.GroupQueue, Order: 10,
		Label: func(*kit.ActionContext) string { return "Play next" }, Icon: "list-start",
		Run: func(*kit.ActionContext) {},
	})
	r.env.Video = store.NewVideoView(r.surface)
	r.env.Video.BindWindow(func() bool { return r.win.full }, func(on bool) { r.win.full = on })
	r.env.Nav = r.nav
	r.env.HasRoute = func(path string) bool {
		for _, p := range []string{"/music-video/", "/artist/", "/song/", "/search"} {
			if len(path) >= len(p) && path[:len(p)] == p {
				return true
			}
		}
		return false
	}
	r.v = newVideo(r.env, r.cat, r.svc, func() (services.PlaybackCapabilities, bool) { return r.caps, !r.unknown })
	r.v.q = r.queue
	t.Cleanup(func() {
		r.env.Video.Close()
		r.v.closed()
	})
	r.tt = hn.Tester(t, mount(r), w, h)
	return r
}

// viewMount is the layer over a page with a button, as the shell builds it:
// the page takes no input while the layer is open, and a closed layer's
// wrapper takes none.
func viewMount(r *rig) func(c *ui.Context) {
	return func(c *ui.Context) {
		open := r.v.layerOpen()
		ui.Column(c).Fill().IgnoreInput(open).Children(func() {
			kit.Button(c.Key("behind"), "Page behind", kit.ButtonOpts{})
			ui.Box(c).Grow(1)
		})
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).IgnoreInput(!open).Children(func() { r.v.layer(c) })
		kit.OverlayLayer(c)
	}
}

func viewRig(t *testing.T) *rig { return newRig(t, false, viewMount, 1132, 800) }

// the queue entry that plays in most tests.
func videoItem(uid uint64) *services.QueueItem {
	return &services.QueueItem{
		UID: uid, ID: "mv.1", CatalogID: "mv.1", Name: "Sky Video", Artist: "Mia Rivers", ArtistID: "ar.1", Video: true,
		DurationMs: 200_000,
	}
}

// play seeds the mirror with a music video in the given state.
func (r *rig) play(state string, mutate ...func(*services.PlaybackState)) {
	s := store.EmptyState()
	s.State = state
	s.Current = videoItem(1)
	s.Index = 0
	s.DurationMs, s.PositionMs, s.At = 200_000, 30_000, r.h.Clock.Now().UnixMilli()
	s.Volume = 0.6
	s.Controls = services.PlaybackControls{Previous: true, Next: true, Seek: true}
	for _, m := range mutate {
		m(&s)
	}
	r.env.Play.Seed(s, services.QueueState{Index: -1})
	r.tt.Frame()
}

// open opens the view over a playing video and draws it.
func (r *rig) open() {
	r.t.Helper()
	r.play(store.StatePlaying)
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Until("the view", r.tt.Shows("Sky Video", "Mia Rivers"))
	r.tt.Advance(fadeIn) // it has faded in
}

// grey is a w×h I420 picture of one grey.
func grey(w, h int, luma byte) []byte {
	b := make([]byte, w*h+2*((w+1)/2)*((h+1)/2))
	for i := range b {
		b[i] = 128
	}
	for i := range w * h {
		b[i] = luma
	}
	return b
}

// onLinux: pixels are read on Linux only (Pango and the CPU renderer shape
// the text and edges there; other systems' output differs).
var onLinux = runtime.GOOS == "linux"

// shows reports whether the middle of the picture area has that colour; off
// Linux, where pixels are not read, whether a picture is on show.
// quiet drains what goroutines post until nothing has come for a while: the
// Core's start-up events and the like have reached the mirror.
func (r *rig) quiet() {
	r.t.Helper()
	for still, deadline := 0, time.Now().Add(5*time.Second); still < 10 && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		pending := r.h.Main.Pending()
		r.tt.Drain()
		if pending {
			still = 0
		} else {
			still++
		}
	}
}

func (r *rig) shows(want color.RGBA) bool {
	if !onLinux {
		return r.v.feed.HasPicture()
	}
	return near(pixel(r.tt, centreX, centreY), want)
}

func pixel(tt *uitest.Session, x, y int) color.RGBA {
	r, g, b, _ := tt.Image().At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
}

func near(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R)+d(a.G, b.G)+d(a.B, b.B) < 24
}

var (
	white = color.RGBA{255, 255, 255, 255}
	black = color.RGBA{0, 0, 0, 255}
)

// centre is a point inside the picture area, between the strip and the
// transport.
const centreX, centreY = 566, 380

func TestSurfaceShowsTheLatestPictureAndDropsItOnStop(t *testing.T) {
	r := viewRig(t)
	r.open()

	// Before the first picture the stage is a black 16:9 box (no cover here).
	if r.v.feed.HasPicture() {
		t.Fatal("a picture before any came")
	}
	if onLinux {
		if got := pixel(r.tt, centreX, centreY); got == white {
			t.Fatalf("a picture before any came: %v", got)
		}
	}
	r.surface.Show(64, 36, 0, grey(64, 36, 235))
	r.surface.Show(64, 36, time.Second, grey(64, 36, 16))
	r.surface.Show(64, 36, 2*time.Second, grey(64, 36, 235)) // the latest
	r.tt.Until("the latest picture", func() bool { return r.shows(white) })
	if !r.v.feed.HasPicture() {
		t.Error("the feed has no picture")
	}
	if w, h := r.v.feed.PictureSize(); w != 64 || h != 36 {
		t.Errorf("picture size %d×%d", w, h)
	}

	// The next picture replaces it in place.
	r.surface.Show(64, 36, 3*time.Second, grey(64, 36, 16))
	r.tt.Until("the next picture", func() bool { return r.shows(black) })

	// The engine takes the picture off (the item has no video): it goes.
	r.surface.Clear()
	r.tt.Until("the picture to come down", func() bool { return !r.v.feed.HasPicture() })

	// Closing the view stops the feed and forgets the picture: what the
	// surface shows afterwards is not converted or kept.
	r.surface.Show(64, 36, 4*time.Second, grey(64, 36, 235))
	r.tt.Until("a picture again", r.v.feed.HasPicture)
	r.env.Video.Close()
	r.tt.Drain()
	if r.v.feed.HasPicture() || r.v.view.open {
		t.Errorf("a closed view kept its picture: %v %v", r.v.feed.HasPicture(), r.v.view.open)
	}
	before := r.v.feed.Pictures()
	r.surface.Show(64, 36, 5*time.Second, grey(64, 36, 235))
	time.Sleep(20 * time.Millisecond)
	r.tt.Drain()
	if r.v.feed.HasPicture() || r.v.feed.Pictures() != before {
		t.Error("a closed view took a picture")
	}
}

func TestPictureIsLetterboxedAtItsOwnShape(t *testing.T) {
	r := viewRig(t)
	r.open()
	r.surface.Show(64, 48, 0, grey(64, 48, 235)) // 4:3 in a wide stage
	r.tt.Until("the picture", func() bool { return r.shows(white) })
	if w, h := r.v.feed.PictureSize(); w != 64 || h != 48 {
		t.Errorf("picture size %d×%d", w, h)
	}
	if fw, fh := fitPicture(64, 48, r.v.view.stage.W, r.v.view.stage.H); fw >= r.v.view.stage.W-2 || fh < r.v.view.stage.H-2 {
		t.Errorf("the 4:3 box %v×%v in a stage %v×%v: not pillarboxed", fw, fh, r.v.view.stage.W, r.v.view.stage.H)
	}
	if !onLinux {
		return
	}
	ground := theme.C("ground")
	if got := pixel(r.tt, 60, centreY); !near(got, color.RGBA{ground.R, ground.G, ground.B, 255}) {
		t.Errorf("beside the picture: %v, want the ground", got)
	}
	if got := pixel(r.tt, centreX-100, centreY); !near(got, white) {
		t.Errorf("inside the picture: %v", got)
	}
}

func TestPictureOfAnEarlierEntryIsNeverShown(t *testing.T) {
	r := viewRig(t)
	r.open()
	r.surface.Show(64, 36, 0, grey(64, 36, 235))
	r.tt.Until("the picture", r.v.feed.HasPicture)

	// The queue reaches another video: the surface still holds the first
	// one's last picture until the engine has the next ready.
	r.play(store.StatePlaying, func(s *services.PlaybackState) { s.Current = videoItem(2) })
	r.tt.Until("the old picture to go", func() bool { return !r.v.feed.HasPicture() })
	time.Sleep(20 * time.Millisecond)
	r.tt.Drain()
	if r.v.feed.HasPicture() {
		t.Fatal("the earlier entry's picture came back")
	}
	if !r.tt.HasText("Loading the video…") {
		t.Errorf("no loading status: %q", r.tt.Texts())
	}
	r.surface.Show(64, 36, 0, grey(64, 36, 16))
	r.tt.Until("the new picture", func() bool { return r.shows(black) && r.v.feed.HasPicture() })
}

func TestPictureOnShowBeforeTheVideoNamedItselfIsNotShown(t *testing.T) {
	r := viewRig(t)
	// The item before was a video: its last picture is still on the surface
	// when a play opens the view and Go's state names the new video.
	r.surface.Show(64, 36, 0, grey(64, 36, 235))
	r.play(store.StatePaused, func(s *services.PlaybackState) { s.Current = &services.QueueItem{UID: 9, ID: "s.1", Name: "A Song"} })
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Advance(time.Second)
	r.play(store.StateLoading)
	r.tt.Until("the view", r.tt.Shows("Sky Video", "Loading the video…"))
	time.Sleep(30 * time.Millisecond)
	r.tt.Drain()
	if r.v.feed.HasPicture() {
		t.Fatal("the picture of the item before was shown")
	}
	r.tt.Advance(fadeIn) // faded in: drawn, so pictures are converted
	r.surface.Show(64, 36, 0, grey(64, 36, 16))
	r.tt.Until("the new picture", r.v.feed.HasPicture)
}

func TestViewWaitsForTheVideoThatOpenedItThenShowsIt(t *testing.T) {
	r := viewRig(t)
	// A play opened the view before Go's state named the video.
	r.play(store.StatePaused, func(s *services.PlaybackState) { s.Current = &services.QueueItem{UID: 9, ID: "s.1", Name: "A Song"} })
	r.env.Video.Open()
	r.tt.Frame()
	if r.tt.HasText("Music video") && r.tt.HasText("Close video") {
		t.Fatal("the view showed before there was a video")
	}
	if !r.env.Video.IsOpen() || r.v.layerOpen() {
		t.Fatal("while it waits the view is open but covers nothing")
	}
	r.tt.Advance(time.Second)
	// Go's state arrives: the video plays.
	r.play(store.StateLoading)
	r.tt.Frame()
	r.tt.Until("the view", r.tt.Shows("Sky Video", "Close video"))
	if !r.v.layerOpen() {
		t.Error("the view does not cover the window")
	}
	// A picture on show before it named itself is the item before's.
	if r.v.feed.HasPicture() {
		t.Error("a picture from before")
	}
}

func TestViewClosesWhenThePlayThatOpenedItNeverArrives(t *testing.T) {
	r := viewRig(t)
	r.play(store.StateIdle, func(s *services.PlaybackState) { s.Current = nil })
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Advance(1500 * time.Millisecond)
	if !r.env.Video.IsOpen() {
		t.Fatal("closed before the wait was over")
	}
	r.tt.Advance(time.Second)
	if r.env.Video.IsOpen() {
		t.Error("still open after the 2 s wait")
	}
}

func TestViewClosesWhenTheQueueMovesOnFromTheVideo(t *testing.T) {
	r := viewRig(t)
	r.open()
	r.play(store.StatePlaying, func(s *services.PlaybackState) {
		s.Current = &services.QueueItem{UID: 2, ID: "s.2", Name: "A Song"}
	})
	r.tt.Frame()
	if r.env.Video.IsOpen() {
		t.Error("the view stayed open for a song")
	}
}

func TestViewKeysCallTheService(t *testing.T) {
	r := viewRig(t)
	r.open()

	r.tt.Key(0, ui.KeySpace)
	r.tt.Key(0, ui.KeyK)
	r.tt.Key(ui.Shift, ui.KeyK)
	if got := r.svc.wait(t, 3); !slices.Equal(got, []string{"toggle", "toggle", "toggle"}) {
		t.Errorf("Space, K, Shift+K: %v", got)
	}

	r.tt.Key(0, ui.KeyRight)
	r.svc.wait(t, 1)
	r.tt.Key(0, ui.KeyLeft)
	r.svc.wait(t, 1)
	r.svc.mu.Lock()
	seeks := slices.Clone(r.svc.seeks)
	r.svc.mu.Unlock()
	if !slices.Equal(seeks, []int64{35_000, 30_000}) {
		t.Errorf("Right then Left seek to %v, want 35000 then 30000 (the mirror shows each at once)", seeks)
	}

	// Shift+arrows are the player's, not the view's; a key with Ctrl or Alt
	// held is no key of the view.
	r.tt.Key(ui.Shift, ui.KeyRight)
	r.svc.quiet(t, "Shift+Right")
	r.tt.Key(ui.Cmd, ui.KeyK)
	r.tt.Key(ui.Alt, ui.KeyLeft)
	r.tt.Key(ui.Cmd, ui.KeyF)
	r.svc.quiet(t, "keys with a modifier held")
	if r.win.full {
		t.Error("Ctrl+F went full screen")
	}

	// M mutes where the platform sets the volume.
	r.tt.Key(0, ui.KeyM)
	r.svc.wait(t, 1)
	r.svc.mu.Lock()
	vols := slices.Clone(r.svc.vols)
	r.svc.mu.Unlock()
	if !slices.Equal(vols, []float64{0}) {
		t.Errorf("M set the volume to %v", vols)
	}
	r.tt.Key(0, ui.KeyM) // and returns to the volume before
	r.svc.wait(t, 1)
	r.svc.mu.Lock()
	vols = slices.Clone(r.svc.vols)
	r.svc.mu.Unlock()
	if len(vols) != 2 || vols[1] != 0.6 {
		t.Errorf("M again set %v, want back to 0.6", vols)
	}
}

func TestMIsIgnoredWithoutAVolumeCapability(t *testing.T) {
	r := viewRig(t)
	r.caps.Volume = false
	r.open()
	r.tt.Key(0, ui.KeyM)
	r.svc.quiet(t, "M without volume")
	if r.tt.HasText("Mute") {
		t.Error("a mute button without a volume")
	}
	if _, ok := r.tt.Find("Music video"); !ok {
		t.Error("the view is not named")
	}
}

func TestViewButtonsCallTheService(t *testing.T) {
	r := viewRig(t)
	r.open()
	for _, c := range []struct{ label, call string }{{"Pause", "toggle"}, {"Previous", "prev"}, {"Next", "next"}} {
		if err := r.tt.Click(c.label); err != nil {
			t.Fatalf("%s: %v", c.label, err)
		}
		if got := r.svc.wait(t, 1); got[len(got)-1] != c.call {
			t.Errorf("%s called %v, want %s", c.label, got, c.call)
		}
	}
	if err := r.tt.Click("Mute"); err != nil {
		t.Fatal(err)
	}
	r.svc.wait(t, 1)
}

func TestTransportButtonsFollowTheControls(t *testing.T) {
	r := viewRig(t)
	r.play(store.StatePlaying, func(s *services.PlaybackState) { s.Controls = services.PlaybackControls{} })
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Until("the view", r.tt.Shows("Sky Video"))
	_ = r.tt.Click("Previous")
	_ = r.tt.Click("Next")
	r.svc.quiet(t, "disabled previous and next")
	// The play disc says what it will do.
	r.play(store.StatePaused, func(s *services.PlaybackState) { s.Controls = services.PlaybackControls{} })
	if !r.tt.HasText("Play") {
		t.Errorf("a paused video has no Play: %q", r.tt.Texts())
	}
}

func TestSeekBarShowsTimesAndSeeksWhenReleased(t *testing.T) {
	r := viewRig(t)
	r.open()
	for _, w := range []string{"0:30", "2:50 left", "Seek"} {
		if !r.tt.HasText(w) {
			t.Errorf("seek bar lacks %q: %q", w, r.tt.Texts())
		}
	}
	box, ok := r.tt.Find("Seek")
	if !ok {
		t.Fatal("no Seek slider")
	}
	r.tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
	r.svc.wait(t, 1)
	r.svc.mu.Lock()
	seeks := slices.Clone(r.svc.seeks)
	r.svc.mu.Unlock()
	if len(seeks) != 1 || seeks[0] < 90_000 || seeks[0] > 110_000 {
		t.Errorf("a click in the middle of 3:20 sought to %v ms, want about 100000", seeks)
	}
}

func TestSeekSliderIsDisabledWithoutADuration(t *testing.T) {
	r := viewRig(t)
	r.play(store.StatePlaying, func(s *services.PlaybackState) { s.DurationMs = 0 })
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Until("the view", r.tt.Shows("Sky Video"))
	box, _ := r.tt.Find("Seek")
	r.tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
	r.svc.quiet(t, "a seek with no duration")
}

func TestFailuresAreToastsTitledByTheCommand(t *testing.T) {
	r := viewRig(t)
	r.open()
	r.svc.fail = services.ErrNotReleased
	r.tt.Key(0, ui.KeySpace)
	r.svc.wait(t, 1)
	r.tt.Until("the toast", r.tt.Shows("Couldn't play or pause"))
}

func TestEscapeLeavesFullScreenThenClosesAndFKeyAndButtonToggle(t *testing.T) {
	r := viewRig(t)
	r.open()

	r.tt.Key(0, ui.KeyF)
	if !r.win.full {
		t.Fatal("F did not go full screen")
	}
	r.tt.Frame()
	if !r.tt.HasText("Exit full screen") {
		t.Errorf("the button does not offer to leave: %q", r.tt.Texts())
	}
	// The first Escape leaves full screen and keeps the view.
	r.tt.Key(0, ui.KeyEscape)
	if r.win.full || !r.env.Video.IsOpen() {
		t.Fatalf("Escape in full screen: full %v open %v", r.win.full, r.env.Video.IsOpen())
	}
	if err := r.tt.Click("Full screen"); err != nil {
		t.Fatal(err)
	}
	if !r.win.full {
		t.Fatal("the Full screen button did not go full screen")
	}
	// Closing the view leaves the full screen it entered.
	r.tt.Key(0, ui.KeyEscape) // out of full screen
	r.tt.Key(0, ui.KeyF)
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Key(0, ui.KeyEscape) // the second Escape closes
	if r.env.Video.IsOpen() {
		t.Error("a second Escape did not close the view")
	}
	if r.win.full {
		t.Error("the closed view left the window full screen")
	}
}

func TestClosingLeavesAFullScreenTheViewEnteredButNotTheUsers(t *testing.T) {
	r := viewRig(t)
	r.win.full = true // the user pressed F11 before
	r.open()
	if err := r.tt.Click("Close video"); err != nil {
		t.Fatal(err)
	}
	if r.env.Video.IsOpen() {
		t.Fatal("Close video did not close")
	}
	if !r.win.full {
		t.Error("closing took the user's full screen away")
	}
}

func TestSpaceOnAFocusedButtonIsTheButtons(t *testing.T) {
	r := viewRig(t)
	r.open()
	// Tab from the region to the first control, the title's link: Space is
	// the link's, not the play and pause toggle.
	r.tt.Key(0, ui.KeyTab)
	r.tt.Key(0, ui.KeySpace)
	if got := r.svc.snapshot(); slices.Contains(got, "toggle") {
		t.Errorf("Space on a focused link toggled playback: %v", got)
	}
	r.open()
	// On a focused button Space is the button's: Full screen, here.
	r.tt.Key(0, ui.KeyTab) // the title
	r.tt.Key(0, ui.KeyTab) // the artist
	r.tt.Key(0, ui.KeyTab)
	if !r.tt.Focused("Full screen") {
		t.Fatal("the third Tab is not on Full screen")
	}
	r.tt.Key(0, ui.KeySpace)
	if !r.win.full {
		t.Error("Space on the Full screen button did not press it")
	}
	if got := r.svc.snapshot(); slices.Contains(got, "toggle") {
		t.Errorf("Space on a focused button toggled playback: %v", got)
	}
}

func TestTabStaysInsideTheView(t *testing.T) {
	r := viewRig(t)
	r.open()
	seen := map[string]bool{}
	for range 40 {
		r.tt.Key(0, ui.KeyTab)
		for _, l := range []string{"Full screen", "Close video", "Seek", "Previous", "Pause", "Next", "Mute", "Volume", "Page behind"} {
			if r.tt.Focused(l) {
				seen[l] = true
			}
		}
	}
	for _, l := range []string{"Full screen", "Close video", "Seek", "Previous", "Pause", "Next", "Mute", "Volume"} {
		if !seen[l] {
			t.Errorf("Tab never reached %q", l)
		}
	}
	if seen["Page behind"] {
		t.Error("Tab reached the page behind the view")
	}
}

func TestTitleAndArtistLinkAndCloseTheView(t *testing.T) {
	r := viewRig(t)
	r.open()
	if err := r.tt.Click("Sky Video"); err != nil {
		t.Fatal(err)
	}
	if r.nav.last() != "/music-video/mv.1" || r.env.Video.IsOpen() {
		t.Errorf("title: pushed %v, open %v", r.nav.pushed, r.env.Video.IsOpen())
	}
	r.open()
	if err := r.tt.Click("Mia Rivers"); err != nil {
		t.Fatal(err)
	}
	if r.nav.last() != "/artist/ar.1" || r.env.Video.IsOpen() {
		t.Errorf("artist: pushed %v, open %v", r.nav.pushed, r.env.Video.IsOpen())
	}
	// An artist with no page of ours is plain text.
	r.env.HasRoute = func(string) bool { return false }
	r.open()
	if r.tt.HasText("Sky Video") {
		t.Log("title shown as text")
	}
	n := len(r.nav.pushed)
	_ = r.tt.Click("Mia Rivers")
	if len(r.nav.pushed) != n {
		t.Error("an unported artist page was linked")
	}
}

func TestSDMarkAndStatus(t *testing.T) {
	r := viewRig(t)
	r.play(store.StateLoading)
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Advance(fadeIn)
	r.tt.Until("the view", r.tt.Shows("Sky Video", "SD", "Loading the video…"))
	// A picture arrives: no more loading, but loading state is buffering.
	r.surface.Show(64, 36, 0, grey(64, 36, 235))
	r.tt.Until("buffering", r.tt.Shows("Buffering…"))
	r.play(store.StatePlaying)
	r.tt.Until("no status", func() bool { return !r.tt.HasText("Buffering…") && !r.tt.HasText("Loading the video…") })
}

func TestPicturesThatStopMeanBuffering(t *testing.T) {
	r := viewRig(t)
	r.open()
	r.surface.Show(64, 36, 0, grey(64, 36, 235))
	r.tt.Until("the picture", r.v.feed.HasPicture)
	if r.tt.HasText("Buffering…") {
		t.Fatal("buffering with a fresh picture")
	}
	r.tt.Advance(1200 * time.Millisecond)
	if !r.tt.HasText("Buffering…") {
		t.Errorf("no buffering after a second without a picture: %q", r.tt.Texts())
	}
	r.surface.Show(64, 36, time.Second, grey(64, 36, 16))
	r.tt.Until("a picture to clear it", func() bool { return !r.tt.HasText("Buffering…") })
}

// pictures feeds the view a picture every half second of the tester's clock
// for d, as a playing video's pictures do: no loading or buffering status.
func (r *rig) pictures(d time.Duration) {
	r.t.Helper()
	for elapsed := time.Duration(0); elapsed < d; elapsed += 500 * time.Millisecond {
		before := r.v.feed.Pictures()
		r.surface.Show(64, 36, elapsed, grey(64, 36, 100))
		r.tt.Until("a picture", func() bool { return r.v.feed.Pictures() > before })
		r.tt.Advance(500 * time.Millisecond)
	}
}

func TestControlsFadeAfterThreeSecondsAndReturnWithThePointer(t *testing.T) {
	r := viewRig(t)
	r.open()
	r.tt.Move(200, 200)
	r.pictures(500 * time.Millisecond) // the first picture: no loading status
	if r.v.view.alpha != 1 {
		t.Fatalf("controls not shown at first: %v", r.v.view.alpha)
	}
	r.pictures(2 * time.Second)
	if r.v.view.alpha != 1 {
		t.Errorf("faded after 2 s: %v", r.v.view.alpha)
	}
	r.pictures(2 * time.Second)
	if r.v.view.alpha != 0 {
		t.Errorf("not faded after 4 s of nothing: %v", r.v.view.alpha)
	}
	if r.tt.Cursor() != ui.CursorNone {
		t.Errorf("the pointer shows over a faded view: %v", r.tt.Cursor())
	}
	// The pointer brings them back.
	r.tt.Move(300, 300)
	r.tt.Advance(400 * time.Millisecond)
	if r.tt.Cursor() == ui.CursorNone {
		t.Error("the pointer is hidden over visible controls")
	}
	if r.v.view.alpha != 1 {
		t.Errorf("the pointer did not bring the controls back: %v", r.v.view.alpha)
	}
	// And they fade again after another 3 s.
	r.pictures(4 * time.Second)
	if r.v.view.alpha != 0 {
		t.Errorf("not faded again: %v", r.v.view.alpha)
	}
	// Any key, even Shift, brings them back.
	r.tt.Key(ui.Shift, ui.KeyRight)
	r.tt.Advance(400 * time.Millisecond)
	if r.v.view.alpha != 1 {
		t.Errorf("a key did not bring the controls back: %v", r.v.view.alpha)
	}
}

func TestControlsStayWhilePausedLoadingBufferingOrFocused(t *testing.T) {
	r := viewRig(t)
	r.play(store.StatePaused)
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Advance(5 * time.Second)
	if r.v.view.alpha != 1 {
		t.Errorf("faded while paused: %v", r.v.view.alpha)
	}
	r.play(store.StateLoading)
	r.tt.Advance(5 * time.Second)
	if r.v.view.alpha != 1 {
		t.Errorf("faded while loading: %v", r.v.view.alpha)
	}
	// Playing, no picture yet: the status keeps them.
	r.play(store.StatePlaying)
	r.tt.Advance(5 * time.Second)
	if r.v.view.alpha != 1 || !r.tt.HasText("Loading the video…") {
		t.Errorf("faded before the first picture: %v", r.v.view.alpha)
	}
	// Playing pictures: they fade; a control focused from the keyboard keeps
	// them (Tab never lands on an unseen control).
	r.pictures(4 * time.Second)
	if r.v.view.alpha != 0 {
		t.Fatalf("not faded with pictures playing: %v", r.v.view.alpha)
	}
	r.tt.Key(0, ui.KeyTab)
	r.pictures(4 * time.Second)
	if r.v.view.alpha != 1 {
		t.Errorf("a focused control faded: %v", r.v.view.alpha)
	}
}

func TestProblemWhenVideosDoNotPlayHere(t *testing.T) {
	r := viewRig(t)
	r.caps.Video = false
	r.cat.vid = services.CatalogVideo{Video: services.MusicVideo{ID: "mv.1", URL: "https://music.apple.com/gb/music-video/x/1"}}
	r.open()
	for _, w := range []string{"Music videos don't play here", "This platform or build of Sona can't play music videos. You can watch it in Apple Music."} {
		if !r.tt.HasText(w) {
			t.Errorf("no %q: %q", w, r.tt.Texts())
		}
	}
	if r.tt.HasText("Try again") {
		t.Error("a Try again where nothing can change")
	}
	if r.tt.HasText("SD") {
		t.Error("the SD mark shows with a problem")
	}
	// Watch in Apple Music once the page (with its link) loaded.
	r.tt.Until("the button", r.tt.Shows("Watch in Apple Music"))
	r.cat.done = make(chan struct{}, 4)
	if err := r.tt.Click("Watch in Apple Music"); err != nil {
		t.Fatal(err)
	}
	r.cat.wait(t)
	r.cat.mu.Lock()
	defer r.cat.mu.Unlock()
	if !slices.Equal(r.cat.opened, []string{"https://music.apple.com/gb/music-video/x/1"}) {
		t.Errorf("opened %v", r.cat.opened)
	}
}

func TestWatchInAppleMusicHasNoButtonUntilTheUrlIsKnown(t *testing.T) {
	r := viewRig(t)
	r.caps.Video = false
	r.cat.err = services.ErrNotReleased
	r.open()
	r.tt.Frame()
	time.Sleep(20 * time.Millisecond)
	r.tt.Drain()
	if r.tt.HasText("Watch in Apple Music") {
		t.Error("a Watch in Apple Music button without a link")
	}
}

func TestPlaybackErrorOffersTryAgainExceptWhereAppleSaysNo(t *testing.T) {
	r := viewRig(t)
	r.play(store.StateError, func(s *services.PlaybackState) { s.Error = services.CodeNetwork })
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Until("the problem", r.tt.Shows("Couldn't play this video", "Try again"))
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	if got := r.svc.wait(t, 1); got[0] != "toggle" {
		t.Errorf("Try again called %v", got)
	}
	for _, code := range []services.ErrorCode{services.CodeVideoHDOnly, services.CodeVideoLicence} {
		r.play(store.StateError, func(s *services.PlaybackState) { s.Error = code })
		r.tt.Frame()
		if r.tt.HasText("Try again") {
			t.Errorf("Try again offered for %s", code)
		}
		if !r.tt.HasText("Couldn't play this video") {
			t.Errorf("no problem for %s: %q", code, r.tt.Texts())
		}
	}
}

func TestViewIsNamed(t *testing.T) {
	r := viewRig(t)
	r.open()
	if _, ok := r.tt.Find("Music video"); !ok {
		t.Error("the view has no name")
	}
}

func TestFitPicture(t *testing.T) {
	cases := []struct {
		pw, ph, bw, bh float32
		w, h           float32
	}{
		{864, 486, 2000, 900, 1600, 900}, // a wider box: letterboxed... pillarboxed at the sides
		{864, 486, 800, 900, 800, 450},   // a taller box: bars above and below
		{16, 9, 0, 0, 0, 0},              // before the box is measured
		{0, 0, 100, 100, 0, 0},           // no picture
		{4, 3, 100, 100, 100, 75},
	}
	for _, c := range cases {
		if w, h := fitPicture(c.pw, c.ph, c.bw, c.bh); w != c.w || h != c.h {
			t.Errorf("fitPicture(%v×%v in %v×%v) = %v×%v, want %v×%v", c.pw, c.ph, c.bw, c.bh, w, h, c.w, c.h)
		}
	}
}

func TestPosterShowsTheCoverUntilThePictureComes(t *testing.T) {
	r := newRigWith(t, nil, viewMount, 1132, 800)
	lib := services.Service[services.LibraryService](r.h.Core)
	page, err := lib.Albums(context.Background(), "dateAdded", true, "", 0, 1)
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("no fixture album: %v", err)
	}
	cover := page.Items[0].Art.Large
	if cover == "" {
		t.Fatal("the fixture album has no art")
	}
	r.quiet() // the Core's own start-up events reach the mirror first
	r.play(store.StatePlaying, func(s *services.PlaybackState) { s.Current.Art.Large = cover })
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Advance(fadeIn)
	// The cover, a generated gradient, fills the 16:9 box before the first
	// picture (not black, not the ground).
	r.tt.Until("the poster", func() bool {
		if r.h.Covers.Hits() == 0 {
			return false
		}
		if !onLinux {
			return true
		}
		c := pixel(r.tt, centreX, centreY)
		return !near(c, black) && !near(c, white)
	})
	r.surface.Show(64, 36, 0, grey(64, 36, 235))
	r.tt.Until("the picture over it", func() bool { return r.shows(white) })
	// Gone for good: the poster does not come back under the picture.
	if r.v.feed.HasPicture() != true {
		t.Error("no picture")
	}
}

func TestViewFadesInOverTwoHundredMilliseconds(t *testing.T) {
	r := viewRig(t)
	r.play(store.StatePlaying)
	r.env.Video.Open()
	r.tt.Frame()
	if r.v.view.shownAt.IsZero() {
		t.Fatal("the view was not built")
	}
	if got := r.v.view.in; got != 0 {
		t.Errorf("opacity %v as the view opens, want 0", got)
	}
	r.tt.Advance(100 * time.Millisecond)
	if got := r.v.view.in; got < 0.3 || got > 0.7 {
		t.Errorf("opacity %v half way through the fade-in, want about 0.5", got)
	}
	r.tt.Advance(150 * time.Millisecond)
	if got := r.v.view.in; got != 1 {
		t.Errorf("opacity %v after the fade-in, want 1", got)
	}
}

func TestViewAppearsAtOnceWhenMotionIsReduced(t *testing.T) {
	theme.SetReducedMotion(true)
	t.Cleanup(func() { theme.SetReducedMotion(false) })
	r := viewRig(t)
	r.play(store.StatePlaying)
	r.env.Video.Open()
	r.tt.Frame()
	if got := r.v.view.in; got != 1 {
		t.Errorf("opacity %v with reduced motion, want 1", got)
	}
}

func TestProblemKeepsThePictureBoxBuilt(t *testing.T) {
	r := viewRig(t)
	r.caps.Video = false
	r.open()
	if !r.v.view.started {
		t.Error("the picture feed stopped under a problem")
	}
}

func TestSeekDragThatLeavesTheSliderSeeksOnceOnRelease(t *testing.T) {
	r := viewRig(t)
	r.open()
	box, ok := r.tt.Find("Seek")
	if !ok {
		t.Fatal("no Seek slider")
	}
	y := box.Y + box.H/2
	r.tt.Press(box.X+box.W*0.2, y)
	for i := range 10 {
		r.tt.Move(box.X+box.W*(0.2+0.05*float32(i)), y+80) // well off the slider, button still down
	}
	r.svc.quiet(t, "a drag off the slider")
	r.tt.Release(box.X+box.W*0.7, y+80)
	r.svc.wait(t, 1)
	r.svc.quiet(t, "after the release")
	r.svc.mu.Lock()
	n := len(r.svc.seeks)
	r.svc.mu.Unlock()
	if n != 1 {
		t.Errorf("a drag made %d seeks, want one on release", n)
	}
}
