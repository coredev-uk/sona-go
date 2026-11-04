package player

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// fakeTransport records what the bar asked of PlaybackService.
type fakeTransport struct {
	mu    sync.Mutex
	calls []string
	seeks []int64
	fail  error
	done  chan struct{}
}

func newFake() *fakeTransport { return &fakeTransport{done: make(chan struct{}, 16)} }

func (f *fakeTransport) record(name string) error {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	err := f.fail
	f.mu.Unlock()
	f.done <- struct{}{}
	return err
}

func (f *fakeTransport) Toggle(context.Context) error { return f.record("toggle") }
func (f *fakeTransport) Next(context.Context) error   { return f.record("next") }
func (f *fakeTransport) Prev(context.Context) error   { return f.record("prev") }
func (f *fakeTransport) Seek(_ context.Context, ms int64) error {
	f.mu.Lock()
	f.seeks = append(f.seeks, ms)
	f.mu.Unlock()
	return f.record("seek")
}
func (f *fakeTransport) SetVolume(context.Context, float64) error { return f.record("volume") }
func (f *fakeTransport) SetShuffle(on bool) {
	if on {
		_ = f.record("shuffle on")
	} else {
		_ = f.record("shuffle off")
	}
}
func (f *fakeTransport) SetRepeat(mode string) error { return f.record("repeat " + mode) }

// wait blocks until n calls have been recorded (they run on goroutines).
func (f *fakeTransport) wait(t *testing.T, n int) []string {
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

func (f *fakeTransport) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// mainThread is a Poster run where the test says, as the app's runs on the
// main thread.
type mainThread struct{ q chan func() }

func (m *mainThread) post(fn func()) { m.q <- fn }

func (m *mainThread) drain() {
	for {
		select {
		case fn := <-m.q:
			fn()
		default:
			return
		}
	}
}

type rig struct {
	p    *player
	tt   *ui.Tester
	fake *fakeTransport
	mt   *mainThread
	env  *module.Env
	now  time.Time
}

func newRig(t *testing.T, volume bool) *rig {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	mt := &mainThread{q: make(chan func(), 64)}
	bus := store.NewBus(nil)
	bus.SetPoster(mt.post)
	r := &rig{fake: newFake(), mt: mt, now: time.UnixMilli(1_000_000)}
	art := kit.NewArtLoader(nil, mt.post) // no cover has a source here
	t.Cleanup(art.Close)
	r.env = &module.Env{Bus: bus, Post: mt.post, Art: art, Play: store.NewPlayback(bus, func() time.Time { return r.now })}
	r.p = newPlayer(r.env, r.fake, func() bool { return volume })
	r.p.now = func() time.Time { return r.now }
	r.tt = ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).Grow(1)
			r.p.bar(c)
		})
	}, 1132, 400)
	return r
}

// seed shows a playing song 30 s into a 3 min one.
func (r *rig) seed(mutate ...func(*services.PlaybackState)) {
	s := store.EmptyState()
	s.State = store.StatePlaying
	s.Current = &services.QueueItem{ID: "i.1", Name: "Song Title", Artist: "The Artist", Album: "The Album"}
	s.DurationMs, s.PositionMs, s.At = 180_000, 30_000, r.now.UnixMilli()
	s.Controls = services.PlaybackControls{Previous: true, Next: true, Seek: true, Shuffle: true, Repeat: true}
	for _, m := range mutate {
		m(&s)
	}
	r.env.Play.Seed(s, services.QueueState{Index: -1})
	r.tt.Frame()
}

func TestBarIsIdleWithNothingPlaying(t *testing.T) {
	r := newRig(t, true)
	for _, w := range []string{"Not playing", "Choose a song, album or playlist to start", "–:––", "Player"} {
		if !r.tt.HasText(w) {
			t.Errorf("idle bar lacks %q: %q", w, r.tt.Texts())
		}
	}
	// Nothing to play or skip.
	_ = r.tt.Click("Play")
	_ = r.tt.Click("Next")
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("an idle bar sent %v", got)
	}
}

func TestBarFollowsThePlaybackMirror(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Preview = true })
	for _, w := range []string{"Song Title", "The Artist · The Album", "Pause", "Preview", "0:30", "3:00"} {
		if !r.tt.HasText(w) {
			t.Errorf("bar lacks %q: %q", w, r.tt.Texts())
		}
	}
	// Go sends a new snapshot: paused, another song.
	s := r.env.Play.State()
	s.State = store.StatePaused
	s.Current = &services.QueueItem{ID: "i.2", Name: "Another", Artist: "Someone"}
	r.env.Bus.Emit(services.EventPlaybackState, s)
	r.mt.drain()
	r.tt.Frame()
	for _, w := range []string{"Another", "Someone", "Play"} {
		if !r.tt.HasText(w) {
			t.Errorf("after the event the bar lacks %q: %q", w, r.tt.Texts())
		}
	}
	if r.tt.HasText("Song Title") || r.tt.HasText("Pause") {
		t.Errorf("the bar still shows the old song: %q", r.tt.Texts())
	}
}

func TestTheTimeTicksWhilePlaying(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	r.now = r.now.Add(12 * time.Second)
	r.tt.Advance(time.Second)
	if !r.tt.HasText("0:42") {
		t.Errorf("30 s + 12 s did not show 0:42: %q", r.tt.Texts())
	}
}

func TestTransportButtonsCallThePlaybackService(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	for _, c := range []struct{ button, call string }{
		{"Pause", "toggle"},
		{"Next", "next"},
		{"Previous", "prev"},
		{"Shuffle off", "shuffle on"},
		{"Repeat off", "repeat all"},
	} {
		if err := r.tt.Click(c.button); err != nil {
			t.Fatalf("%s: %v", c.button, err)
		}
		got := r.fake.wait(t, 1)
		if last := got[len(got)-1]; last != c.call {
			t.Errorf("%s sent %q, want %q", c.button, last, c.call)
		}
		r.mt.drain()
		r.seed() // the mirror as Go reports it
	}
}

func TestEndOfQueueAndLiveStationsDisableControls(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Controls.Next = false })
	_ = r.tt.Click("Next")
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("Next at the end of the queue sent %v", got)
	}

	r.seed(func(s *services.PlaybackState) {
		s.Current.Live = true
		s.Controls = services.PlaybackControls{}
	})
	if !r.tt.HasText("Live") {
		t.Errorf("a live station shows no marker: %q", r.tt.Texts())
	}
	if r.tt.HasText("0:30") {
		t.Error("a live station shows a seek bar")
	}
	if got := unavailable(r.env.Play.State(), ctlShuffle); got != "Not available while listening to a live station" {
		t.Errorf("reason %q", got)
	}
}

func TestSeekingCommitsWhereThePointerLetsGo(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	b, _ := r.tt.Find("Seek")
	r.tt.ClickAt(b.X+b.W*3/4, b.Y+b.H/2)
	r.fake.wait(t, 1)
	r.fake.mu.Lock()
	seeks := slices.Clone(r.fake.seeks)
	r.fake.mu.Unlock()
	if len(seeks) != 1 || seeks[0] < 125_000 || seeks[0] > 145_000 {
		t.Fatalf("a click three quarters along a 3:00 song sought %v ms", seeks)
	}
	// The mirror shows the new position at once.
	if got := r.env.Play.State().PositionMs; got != seeks[0] {
		t.Errorf("the mirror shows %d ms, want %d", got, seeks[0])
	}
}

func TestVolume(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Volume = 0.6 })
	if !r.tt.HasText("Mute") {
		t.Fatalf("no mute button: %q", r.tt.Texts())
	}
	_ = r.tt.Click("Mute")
	r.fake.wait(t, 1)
	if v := r.env.Play.State().Volume; v != 0 {
		t.Errorf("muting left the volume at %v", v)
	}
	r.tt.Frame()
	if !r.tt.HasText("Unmute") {
		t.Errorf("after muting the button should offer Unmute: %q", r.tt.Texts())
	}
	_ = r.tt.Click("Unmute")
	r.fake.wait(t, 1)
	if v := r.env.Play.State().Volume; v != 0.6 {
		t.Errorf("unmuting returned to %v, want 0.6", v)
	}

	// Where Sona does not set the volume the bar has no control for it.
	r2 := newRig(t, false)
	r2.seed()
	if r2.tt.HasText("Mute") || r2.tt.HasText("Volume") {
		t.Errorf("volume controls shown where unsupported: %q", r2.tt.Texts())
	}
}

func TestACommandThatFailsToastsItsCodeWorded(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	r.fake.fail = services.ErrNotReleased
	_ = r.tt.Click("Pause")
	r.fake.wait(t, 1)
	r.mt.drain()
	r.tt.Frame()
	for _, w := range []string{"Couldn't play or pause", i18n.ErrorText(services.CodeNotReleased)} {
		if !r.tt.HasText(w) {
			t.Errorf("no toast saying %q: %q", w, r.tt.Texts())
		}
	}
	if services.CodeOf(errors.New("boom")) == services.CodeNotReleased {
		t.Fatal("test premise")
	}
}

func TestShortcutsKeepV020IDsAndKeys(t *testing.T) {
	r := newRig(t, true)
	want := map[string]string{
		"player.toggle": "Space", "player.next": "Ctrl+ArrowRight", "player.previous": "Ctrl+ArrowLeft",
		"player.seek-forward": "Shift+ArrowRight", "player.seek-back": "Shift+ArrowLeft",
		"player.volume-up": "Ctrl+ArrowUp", "player.volume-down": "Ctrl+ArrowDown",
		"player.mute": "M", "player.shuffle": "S", "player.repeat": "R",
	}
	got := map[string]module.Shortcut{}
	for _, s := range r.p.shortcuts() {
		got[s.ID] = s
		if len(s.Keys) != 1 || s.Keys[0] != want[s.ID] {
			t.Errorf("%s has keys %v, want %q", s.ID, s.Keys, want[s.ID])
		}
		if s.Group != "playback" || s.Label() == "" || s.Run == nil {
			t.Errorf("%s: group %q, label %q", s.ID, s.Group, s.Label())
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d shortcuts, want %d", len(got), len(want))
	}

	r.seed()
	got["player.toggle"].Run()
	got["player.next"].Run()
	got["player.seek-forward"].Run()
	got["player.volume-down"].Run()
	got["player.repeat"].Run()
	calls := r.fake.wait(t, 5)
	slices.Sort(calls)
	if !slices.Equal(calls, []string{"next", "repeat all", "seek", "toggle", "volume"}) {
		t.Errorf("shortcuts sent %v", calls)
	}
	r.fake.mu.Lock()
	seek := r.fake.seeks[0]
	r.fake.mu.Unlock()
	if seek < 39_000 || seek > 41_000 {
		t.Errorf("Skip ahead 10 seconds from 0:30 sought %d ms", seek)
	}

	// Next at the end of a queue (no Controls.Next) is off, with no reason
	// to give; a live station's controls say why.
	r.seed(func(s *services.PlaybackState) { s.Current.Live, s.Controls = true, services.PlaybackControls{} })
	if why := got["player.next"].Unavailable(); why == "" {
		t.Error("Next while a live station plays is not unavailable")
	}
	if why := got["player.seek-forward"].Unavailable(); why == "" {
		t.Error("seeking a live station is not unavailable")
	}
}

func TestClock(t *testing.T) {
	for in, want := range map[time.Duration]string{0: "0:00", 59 * time.Second: "0:59", 61 * time.Second: "1:01", 3725 * time.Second: "1:02:05", -time.Second: "0:00"} {
		if got := clock(in); got != want {
			t.Errorf("clock(%v) = %q, want %q", in, got, want)
		}
	}
}
