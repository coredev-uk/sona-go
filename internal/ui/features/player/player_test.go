package player

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeTransport records what the bar asked of PlaybackService.
type fakeTransport struct {
	*uitest.Transport
	refs []services.TrackRef
}

func newFake() *fakeTransport { return &fakeTransport{Transport: uitest.NewTransport()} }

func (f *fakeTransport) SetShuffle(on bool) {
	if on {
		_ = f.Record("shuffle on")
	} else {
		_ = f.Record("shuffle off")
	}
}
func (f *fakeTransport) SetRepeat(mode string) error { return f.Record("repeat " + mode) }
func (f *fakeTransport) SetAutoPlay(on bool) error {
	if on {
		return f.Record("autoplay on")
	}
	return f.Record("autoplay off")
}

func (f *fakeTransport) SetSleep(mode string, minutes int) error {
	return f.Record(fmt.Sprintf("sleep %s %d", mode, minutes))
}

func (f *fakeTransport) Jump(_ context.Context, i int) error { return f.Record(fmt.Sprint("jump ", i)) }

func (f *fakeTransport) Remove(_ context.Context, i int) error {
	return f.Record(fmt.Sprint("remove ", i))
}

func (f *fakeTransport) Move(from, to int) error { return f.Record(fmt.Sprint("move ", from, " ", to)) }
func (f *fakeTransport) MoveNext(i int) error    { return f.Record(fmt.Sprint("movenext ", i)) }
func (f *fakeTransport) MoveToEnd(i int) error   { return f.Record(fmt.Sprint("movetoend ", i)) }
func (f *fakeTransport) Clear()                  { _ = f.Record("clear") }
func (f *fakeTransport) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	f.Locked(func() { f.refs = append(f.refs, refs...) })
	return f.Record(fmt.Sprintf("playitems %d %d %s:%s", len(refs), start, from.Kind, from.ID))
}

func (f *fakeTransport) PlayNext(_ context.Context, refs []services.TrackRef) error {
	f.Locked(func() { f.refs = append(f.refs, refs...) })
	return f.Record("playnext")
}

func (f *fakeTransport) Append(_ context.Context, refs []services.TrackRef) error {
	f.Locked(func() { f.refs = append(f.refs, refs...) })
	return f.Record("append")
}

func (f *fakeTransport) Insert(_ context.Context, at int, refs []services.TrackRef) error {
	f.Locked(func() { f.refs = append(f.refs, refs...) })
	return f.Record(fmt.Sprint("insert ", at))
}

func (f *fakeTransport) UndoReplace(context.Context) (bool, error) {
	return true, f.Record("undoreplace")
}

type rig struct {
	p    *player
	tt   *uitest.Session
	fake *fakeTransport
	h    *uitest.Harness
	env  *module.Env
}

// queueOver gives a Core-less Env the queue commands of app.New (Env.Queue),
// over the fake transport instead of the Core's playback service.
func queueOver(env *module.Env, svc kit.QueueService) {
	env.Queue = kit.NewQueueCommands(kit.QueueDeps{
		Svc: svc, Play: env.Play, Post: env.Post,
		Unavailable: func() string { return kit.QueueUnavailable(env.Play.State()) },
	})
}

func newRig(t *testing.T, volume bool) *rig {
	t.Helper()
	return newRigWith(t, volume, nil)
}

// newRigWith is newRig with extra built beside the bar (the window's toasts).
func newRigWith(t *testing.T, volume bool, extra func(*ui.Context)) *rig {
	t.Helper()
	return newRigSized(t, volume, 1132, extra)
}

// newRigSized is newRigWith in a window width wide.
func newRigSized(t *testing.T, volume bool, width int, extra func(*ui.Context)) *rig {
	t.Helper()
	h := uitest.New(t, uitest.Bare()) // the bar reads the mirror and calls the fake transport
	r := &rig{fake: newFake(), h: h, env: h.Env}
	queueOver(r.env, r.fake)
	r.p = newPlayer(r.env, r.fake, func() bool { return volume })
	r.p.now = h.Clock.Now
	r.tt = h.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).Grow(1)
			r.p.bar(c)
		})
		if extra != nil {
			extra(c)
		}
	}, width, 400)
	return r
}

// seed shows a playing song 30 s into a 3 min one.
func (r *rig) seed(mutate ...func(*services.PlaybackState)) {
	r.h.Play(30_000, append([]func(*services.PlaybackState){func(s *services.PlaybackState) {
		s.DurationMs = 180_000
		s.Current.CatalogID, s.Current.ArtistID, s.Current.AlbumID = "", "", ""
	}}, mutate...)...)
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
	if got := r.fake.Snapshot(); len(got) != 0 {
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
		got := r.fake.Wait(t, 1)
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
	if got := r.fake.Snapshot(); len(got) != 0 {
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
	r.fake.Wait(t, 1)
	seeks := r.fake.Seeks()
	if len(seeks) != 1 || seeks[0] < 125_000 || seeks[0] > 145_000 {
		t.Fatalf("a click three quarters along a 3:00 song sought %v ms", seeks)
	}
	// The mirror shows the new position at once.
	if got := r.env.Play.State().PositionMs; got != seeks[0] {
		t.Errorf("the mirror shows %d ms, want %d", got, seeks[0])
	}
}

// The seek bar is the bar's top edge, across the whole window, whatever the
// transport and the song take of the row below it; a live station has none,
// and its marker and what is on air stand under its name.
func TestSeekBarRunsAlongTheBarsTopEdge(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	b, ok := r.tt.Find("Seek")
	if !ok {
		t.Fatal("no seek slider")
	}
	const windowW, windowH, barH = 1132, 400, 72
	if b.X != 0 || b.W != windowW || b.Y != windowH-barH || b.H != seekHit {
		t.Errorf("Seek is at %+v, want the top %d DIP of the bar, edge to edge", b, seekHit)
	}
	// The ends of the song are at the window's edges: a click in the first and
	// the last DIPs reaches them.
	r.tt.ClickAt(b.X+2, b.Y+4)
	r.fake.Wait(t, 1)
	seeks := r.fake.Seeks()
	if len(seeks) != 1 || seeks[0] > 3_000 {
		t.Errorf("a click at the left edge sought %v ms", seeks)
	}

	r.seed(func(s *services.PlaybackState) {
		s.Current.Live, s.Current.Name, s.Controls = true, "Apple Music 1", services.PlaybackControls{}
		s.OnAir = &services.OnAir{Title: "Song on air", Artist: "Someone"}
	})
	if _, ok := r.tt.Find("Seek"); ok {
		t.Error("a live station has a Seek slider")
	}
	title, _ := r.tt.Find("Apple Music 1")
	live, ok := r.tt.Find("Live")
	if !ok || live.Y < title.Y+title.H-1 || live.X < title.X {
		t.Errorf("the live marker is at %+v, not under the name at %+v", live, title)
	}
	if !r.tt.HasText("Someone – Song on air") {
		t.Errorf("what is on air is not said: %q", r.tt.Texts())
	}
}

func TestVolume(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Volume = 0.6 })
	if !r.tt.HasText("Mute") {
		t.Fatalf("no mute button: %q", r.tt.Texts())
	}
	_ = r.tt.Click("Mute")
	r.fake.Wait(t, 1)
	if v := r.env.Play.State().Volume; v != 0 {
		t.Errorf("muting left the volume at %v", v)
	}
	r.tt.Frame()
	if !r.tt.HasText("Unmute") {
		t.Errorf("after muting the button should offer Unmute: %q", r.tt.Texts())
	}
	_ = r.tt.Click("Unmute")
	r.fake.Wait(t, 1)
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

// TestVolumeSliderKeepsItsWidthInsideTheWindow: the slider is v0.2.0's
// 96 px (w-24) at every window width, not a share of the spare room, and the
// bar's right group ends inside the window from the 940 minimum up.
func TestVolumeSliderKeepsItsWidthInsideTheWindow(t *testing.T) {
	for _, w := range []int{940, 1132, 1600, 2560} {
		r := newRigSized(t, true, w, nil)
		// What the app builds beside the queue: private listening, the mini
		// player, the equaliser.
		r.env.PlayerControls.Add(10, func(c *ui.Context) { ui.Button(c, "Private") })
		r.env.PlayerControls.Add(module.ControlsAfterQueue+10, func(c *ui.Context) { ui.ButtonBase(c.Key("Mini player")).Size(32, 32).Label("Mini player") })
		r.env.PlayerControls.Add(module.ControlsAfterQueue+20, func(c *ui.Context) { ui.ButtonBase(c.Key("Equaliser")).Size(32, 32).Label("Equaliser") })
		r.seed(func(s *services.PlaybackState) { s.Volume = 0.6 })
		vol, ok := r.tt.Find("Volume")
		if !ok {
			t.Fatalf("%v wide: no volume slider: %q", w, r.tt.Texts())
		}
		if vol.W != volumeWidth {
			t.Errorf("%v wide: the volume slider is %v wide, want %v", w, vol.W, volumeWidth)
		}
		if vol.X < 0 || vol.X+vol.W > float32(w) {
			t.Errorf("%v wide: the volume slider spans %v..%v outside the window", w, vol.X, vol.X+vol.W)
		}
		// The bar pads 16 on each side: the group ends at or before that.
		if vol.X+vol.W > float32(w)-16 {
			t.Errorf("%v wide: the right group ends at %v, past the bar's 16 px padding", w, vol.X+vol.W)
		}
		for _, name := range []string{"Private", "Mini player", "Equaliser", "Mute", "Queue", "Sleep timer"} {
			b, ok := r.tt.Find(name)
			if !ok {
				continue
			}
			if b.X < 0 || b.X+b.W > vol.X {
				t.Errorf("%v wide: %s spans %v..%v, not left of the slider at %v", w, name, b.X, b.X+b.W, vol.X)
			}
		}
	}
}

func TestACommandThatFailsToastsItsCodeWorded(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	r.fake.Fail = services.ErrNotReleased
	_ = r.tt.Click("Pause")
	r.fake.Wait(t, 1)
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
		if strings.HasPrefix(s.ID, "player.sleep-") {
			continue // the sleep timer's commands have no keys (TestSleepTimerShortcutsAreCommandsForThePalette)
		}
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
	if len(got) != len(want)+5 {
		t.Errorf("%d shortcuts, want %d and the five sleep commands", len(got), len(want))
	}

	r.seed()
	got["player.toggle"].Run()
	got["player.next"].Run()
	got["player.seek-forward"].Run()
	got["player.volume-down"].Run()
	got["player.repeat"].Run()
	calls := r.fake.Wait(t, 5)
	slices.Sort(calls)
	if !slices.Equal(calls, []string{"next", "repeat all", "seek", "toggle", "volume"}) {
		t.Errorf("shortcuts sent %v", calls)
	}
	seek := r.fake.Seeks()[0]
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

func TestMuteSharesTheMirrorsUnmutedVolumeWithEveryOtherSurface(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Volume = 0.4 })
	r.p.tr.ToggleMute()
	r.fake.Wait(t, 1)
	if got := r.env.Play.Unmuted(); got != 0.4 {
		t.Errorf("muting left %v to return to, want 0.4", got)
	}
	// Another surface (the video view) changed it meanwhile: unmuting returns there.
	r.env.Play.SetUnmuted(0.9)
	r.p.tr.ToggleMute()
	r.fake.Wait(t, 1)
	if v := r.env.Play.State().Volume; v != 0.9 {
		t.Errorf("unmuting returned to %v, want 0.9", v)
	}
}

func TestSeekCommitsOnceOnReleaseEvenWhenTheDragLeavesTheSlider(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	b, ok := r.tt.Find("Seek")
	if !ok {
		t.Fatal("no seek slider")
	}
	x, y := b.X+b.W/4, b.Y+b.H/2
	r.tt.Press(x, y)
	r.tt.Move(x+30, y)
	r.tt.Frame()
	// Far above the 16 DIP slider, the button still held.
	for i := 1; i <= 5; i++ {
		r.tt.Move(x+30+float32(i)*20, y-float32(i)*40)
		r.tt.Frame()
	}
	if got := r.fake.Snapshot(); len(got) != 0 {
		t.Fatalf("seeks while the pointer is still held: %v", got)
	}
	r.tt.Release(x+130, y-200)
	r.tt.Frame()
	r.tt.Frame()
	got := r.fake.Wait(t, 1)
	r.tt.Frame()
	if len(r.fake.Seeks()) != 1 || len(got) != 1 {
		t.Errorf("seeks %v calls %v, want one seek", r.fake.Seeks(), got)
	}
}
