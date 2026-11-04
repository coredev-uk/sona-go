package player

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeTransport records what the bar asked of PlaybackService.
type fakeTransport struct {
	mu    sync.Mutex
	calls []string
	seeks []int64
	refs  []services.TrackRef
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
func (f *fakeTransport) SetAutoPlay(on bool) error {
	if on {
		return f.record("autoplay on")
	}
	return f.record("autoplay off")
}
func (f *fakeTransport) SetSleep(mode string, minutes int) error {
	return f.record(fmt.Sprintf("sleep %s %d", mode, minutes))
}
func (f *fakeTransport) Jump(_ context.Context, i int) error { return f.record(fmt.Sprint("jump ", i)) }
func (f *fakeTransport) Remove(_ context.Context, i int) error {
	return f.record(fmt.Sprint("remove ", i))
}
func (f *fakeTransport) Move(from, to int) error { return f.record(fmt.Sprint("move ", from, " ", to)) }
func (f *fakeTransport) MoveNext(i int) error    { return f.record(fmt.Sprint("movenext ", i)) }
func (f *fakeTransport) MoveToEnd(i int) error   { return f.record(fmt.Sprint("movetoend ", i)) }
func (f *fakeTransport) Clear()                  { _ = f.record("clear") }
func (f *fakeTransport) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	f.mu.Lock()
	f.refs = append(f.refs, refs...)
	f.mu.Unlock()
	return f.record(fmt.Sprintf("playitems %d %d %s:%s", len(refs), start, from.Kind, from.ID))
}
func (f *fakeTransport) PlayNext(_ context.Context, refs []services.TrackRef) error {
	f.mu.Lock()
	f.refs = append(f.refs, refs...)
	f.mu.Unlock()
	return f.record("playnext")
}
func (f *fakeTransport) Append(_ context.Context, refs []services.TrackRef) error {
	f.mu.Lock()
	f.refs = append(f.refs, refs...)
	f.mu.Unlock()
	return f.record("append")
}
func (f *fakeTransport) Insert(_ context.Context, at int, refs []services.TrackRef) error {
	f.mu.Lock()
	f.refs = append(f.refs, refs...)
	f.mu.Unlock()
	return f.record(fmt.Sprint("insert ", at))
}
func (f *fakeTransport) UndoReplace(context.Context) (bool, error) {
	return true, f.record("undoreplace")
}

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

type rig struct {
	p    *player
	tt   *uitest.Session
	fake *fakeTransport
	h    *uitest.Harness
	env  *module.Env
}

func newRig(t *testing.T, volume bool) *rig {
	t.Helper()
	h := uitest.New(t, uitest.Bare()) // the bar reads the mirror and calls the fake transport
	r := &rig{fake: newFake(), h: h, env: h.Env}
	r.p = newPlayer(r.env, r.fake, func() bool { return volume })
	r.p.now = h.Clock.Now
	r.tt = h.Tester(t, func(c *ui.Context) {
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
	s.DurationMs, s.PositionMs, s.At = 180_000, 30_000, r.h.Clock.Now().UnixMilli()
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
	r.h.Main.Drain()
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
	r.h.Clock.Advance(12 * time.Second)
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
		r.h.Main.Drain()
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
	// The failure comes back through the Poster after the call returns.
	r.tt.Until("the failure's toast", r.tt.Shows("Couldn't play or pause", i18n.ErrorText(services.CodeNotReleased)))
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
		"player.favourite": "F", "player.queue": "Q",
	}
	got := map[string]module.Shortcut{}
	for _, s := range r.p.shortcuts() {
		got[s.ID] = s
		if len(s.Keys) != 1 || s.Keys[0] != want[s.ID] {
			t.Errorf("%s has keys %v, want %q", s.ID, s.Keys, want[s.ID])
		}
		group := module.ShortcutGroup("playback")
		if s.ID == "player.queue" {
			group = "view"
		}
		if s.Group != group || s.Label() == "" || s.Run == nil {
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

func TestOtherModulesBuildIntoTheBarsRightGroupAndNoticeLine(t *testing.T) {
	r := newRig(t, true)
	r.env.PlayerControls.Add(20, func(c *ui.Context) { ui.Button(c, "Second control") })
	r.env.PlayerControls.Add(10, func(c *ui.Context) { ui.Button(c, "First control") })
	r.env.PlayerNotice.Add(0, func(c *ui.Context) { ui.Text(c, "A notice") })
	r.seed()
	for _, w := range []string{"First control", "Second control", "Mute"} {
		if !r.tt.HasText(w) {
			t.Errorf("bar lacks %q: %q", w, r.tt.Texts())
		}
	}
	first, _ := r.tt.Find("First control")
	second, _ := r.tt.Find("Second control")
	mute, _ := r.tt.Find("Mute")
	if !(first.X < second.X && second.X < mute.X) {
		t.Errorf("controls out of order: %v %v then mute %v", first.X, second.X, mute.X)
	}
	if r.tt.HasText("A notice") {
		t.Error("the CDM notice shows while the song plays")
	}
	r.seed(func(s *services.PlaybackState) { s.NoCDM = true })
	if !r.tt.HasText("A notice") {
		t.Errorf("a song waiting for the CDM shows no notice: %q", r.tt.Texts())
	}
}
