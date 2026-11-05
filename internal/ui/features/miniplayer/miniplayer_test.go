package miniplayer

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// fakeSvc records what the mini player asked of the playback service.
type fakeSvc struct {
	mu    sync.Mutex
	calls []string
	seeks []int64
	fail  error
	done  chan struct{}
}

func newSvc() *fakeSvc { return &fakeSvc{done: make(chan struct{}, 32)} }

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

// wait blocks until n calls were made (they run on goroutines).
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

// fakeDesk records the window calls.
type fakeDesk struct {
	mu    sync.Mutex
	calls []string
	done  chan struct{}
}

func newDesk() *fakeDesk { return &fakeDesk{done: make(chan struct{}, 16)} }

func (d *fakeDesk) record(name string) {
	d.mu.Lock()
	d.calls = append(d.calls, name)
	d.mu.Unlock()
	d.done <- struct{}{}
}

func (d *fakeDesk) ShowMainWindow()   { d.record("show main") }
func (d *fakeDesk) CloseMiniPlayer()  { d.record("close mini") }
func (d *fakeDesk) ToggleMiniPlayer() { d.record("toggle mini") }
func (d *fakeDesk) snapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.calls)
}

func (d *fakeDesk) wait(t *testing.T) []string {
	t.Helper()
	select {
	case <-d.done:
	case <-time.After(5 * time.Second):
		t.Fatal("no window call")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.calls)
}

// fakeLyrics answers the lyrics query.
type fakeLyrics struct {
	mu sync.Mutex
	l  services.Lyrics
}

func (f *fakeLyrics) Lyrics(context.Context, string) (services.Lyrics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.l, nil
}

func (f *fakeLyrics) set(l services.Lyrics) {
	f.mu.Lock()
	f.l = l
	f.mu.Unlock()
}

type rig struct {
	m    *mini
	tt   *uitest.Session
	svc  *fakeSvc
	desk *fakeDesk
	lyr  *fakeLyrics
	h    *uitest.Harness
}

func newRig(t *testing.T) *rig {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	r := &rig{h: h, svc: newSvc(), desk: newDesk(), lyr: &fakeLyrics{}}
	r.m = newMini(h.Env)
	r.m.svc, r.m.desk, r.m.lyr, r.m.now = r.svc, r.desk, r.lyr, h.Clock.Now
	lyricsync.Session().Clear()
	t.Cleanup(lyricsync.Session().Clear)
	// The mini window is 420×132, frameless; the shell draws nothing else in it.
	r.tt = h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen fake clock never finishes a transition
		r.m.content(c)
	}, 420, 132)
	return r
}

// show seeds the mirror with a song playing at ms into its 3 minutes.
func (r *rig) show(ms int64, mutate ...func(*services.PlaybackState)) {
	s := store.EmptyState()
	s.State = store.StatePlaying
	s.Current = &services.QueueItem{ID: "i.1", CatalogID: "1001", Name: "Song Title", Artist: "The Artist", Album: "The Album"}
	s.DurationMs, s.PositionMs, s.At = 180_000, ms, r.h.Clock.Now().UnixMilli()
	s.Controls = services.PlaybackControls{Previous: true, Next: true, Seek: true, Shuffle: true, Repeat: true}
	for _, m := range mutate {
		m(&s)
	}
	r.h.Env.Play.Seed(s, services.QueueState{Index: -1})
	r.tt.Frame()
}

// station is a live station playing.
func station(s *services.PlaybackState) {
	s.Current = &services.QueueItem{ID: "ra.1", Name: "KISS", Live: true}
	s.Controls = services.PlaybackControls{}
	s.DurationMs = 0
}

// advance moves the song and the tester's clock on by d.
func (r *rig) advance(d time.Duration) {
	r.h.Clock.Advance(d)
	r.tt.Advance(d)
	r.tt.Frame()
}

func (r *rig) want(t *testing.T, texts ...string) {
	t.Helper()
	for _, w := range texts {
		if !r.tt.HasText(w) {
			t.Errorf("no %q in %q", w, r.tt.Texts())
		}
	}
}

func (r *rig) wantNot(t *testing.T, texts ...string) {
	t.Helper()
	for _, w := range texts {
		if r.tt.HasText(w) {
			t.Errorf("unexpected %q in %q", w, r.tt.Texts())
		}
	}
}

func TestTheMiniPlayerIsIdleWithNothingPlaying(t *testing.T) {
	r := newRig(t)
	r.want(t, "Sona mini player", "Not playing", "Choose music in the main window", "–:––", "Open Sona", "Close mini player")
	for _, b := range []string{"Play", "Previous", "Next"} {
		_ = r.tt.Click(b)
	}
	if got := r.svc.snapshot(); len(got) != 0 {
		t.Errorf("an idle mini player sent %v", got)
	}
}

func TestTheMiniPlayerShowsTheSongAndFollowsThePlaybackMirror(t *testing.T) {
	r := newRig(t)
	r.show(30_000)
	r.want(t, "Song Title", "The Artist", "The Album", "Pause", "−2:30", "Seek")
	r.advance(10 * time.Second)
	r.want(t, "−2:20")

	// Go sends a new snapshot: paused, another song.
	s := r.h.Env.Play.State()
	s.State = store.StatePaused
	s.Current = &services.QueueItem{ID: "i.2", Name: "Another", Artist: "Someone", Album: "Other Album"}
	r.h.Env.Bus.Emit(services.EventPlaybackState, s)
	r.h.Main.Drain()
	r.tt.Frame()
	r.want(t, "Another", "Someone", "Other Album", "Play")
	r.wantNot(t, "Song Title", "Pause")
}

func TestTheThirdLineSaysWhatNeedsTheUserOrWhatPlays(t *testing.T) {
	for name, c := range map[string]struct {
		mutate func(*services.PlaybackState)
		want   string
	}{
		"a music video": {func(s *services.PlaybackState) { s.Current.Video = true }, "Music video"},
		"signed out":    {func(s *services.PlaybackState) { s.AuthRequired = true }, "Sign in again in Sona to keep playing"},
		"no CDM":        {func(s *services.PlaybackState) { s.NoCDM = true }, "Waiting for Widevine: see Settings → Playback in Sona"},
		"a live station": {func(s *services.PlaybackState) {
			station(s)
		}, "Live radio"},
		"a song that failed": {func(s *services.PlaybackState) {
			s.State, s.Error = store.StateError, services.CodeNotReleased
		}, "Couldn't play this song. Press play to try again."},
		"a video that failed": {func(s *services.PlaybackState) {
			s.State, s.Error = store.StateError, services.CodeNotReleased
			s.Current.Video = true
		}, "Couldn't play this video."},
	} {
		r := newRig(t)
		r.show(30_000, c.mutate)
		if !r.tt.HasText(c.want) {
			t.Errorf("%s: no %q in %q", name, c.want, r.tt.Texts())
		}
	}
}

func TestTransportButtonsCallThePlaybackService(t *testing.T) {
	r := newRig(t)
	r.show(30_000)
	for _, c := range []struct{ button, call string }{
		{"Pause", "toggle"},
		{"Next", "next"},
		{"Previous", "prev"},
	} {
		if err := r.tt.Click(c.button); err != nil {
			t.Fatalf("%s: %v", c.button, err)
		}
		got := r.svc.wait(t, 1)
		if last := got[len(got)-1]; last != c.call {
			t.Errorf("%s sent %q, want %q", c.button, last, c.call)
		}
		r.h.Main.Drain()
	}
}

func TestPlayShowsLoadingAndOffersPlayWhenPaused(t *testing.T) {
	r := newRig(t)
	r.show(0, func(s *services.PlaybackState) { s.State = store.StatePaused })
	r.want(t, "Play")
	if err := r.tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	if got := r.svc.wait(t, 1); got[0] != "toggle" {
		t.Errorf("sent %v", got)
	}
}

func TestEndOfQueueDisablesNextWithoutAReason(t *testing.T) {
	r := newRig(t)
	r.show(30_000, func(s *services.PlaybackState) { s.Controls.Next = false })
	_ = r.tt.Click("Next")
	if got := r.svc.snapshot(); len(got) != 0 {
		t.Errorf("Next at the end of the queue sent %v", got)
	}
	r.want(t, "Next")
}

func TestALiveStationKeepsPreviousAndNextOffSayingWhyAndPauseWorking(t *testing.T) {
	r := newRig(t)
	r.show(0, station)
	reason := "Not available while listening to a live station"
	tr := access.Read(t, r.tt.Tester)
	for _, name := range []string{"Previous", "Next"} {
		n, ok := tr.Find(ui.RoleButton, name)
		if !ok || n.Description != reason || n.Has(access.Disabled) || !n.Has(access.Focusable) {
			t.Errorf("%s: %+v, want an enabled, focusable button described by the reason", name, n)
		}
		_ = r.tt.Click(name)
	}
	if got := r.svc.snapshot(); len(got) != 0 {
		t.Fatalf("a live station's Previous/Next sent %v", got)
	}
	r.want(t, "Live")
	r.wantNot(t, "Seek")
	if err := r.tt.Click("Pause"); err != nil {
		t.Fatal(err)
	}
	if got := r.svc.wait(t, 1); got[0] != "toggle" {
		t.Errorf("sent %v", got)
	}
}

func TestSeekingCommitsWhereThePointerLetsGo(t *testing.T) {
	r := newRig(t)
	r.show(30_000)
	b, ok := r.tt.Find("Seek")
	if !ok {
		t.Fatal("no seek slider")
	}
	r.tt.ClickAt(b.X+b.W*3/4, b.Y+b.H/2)
	r.svc.wait(t, 1)
	r.svc.mu.Lock()
	seeks := slices.Clone(r.svc.seeks)
	r.svc.mu.Unlock()
	if len(seeks) != 1 || seeks[0] < 125_000 || seeks[0] > 145_000 {
		t.Fatalf("a click three quarters along a 3:00 song sought %v ms", seeks)
	}
	// The mirror shows the new position at once.
	if got := r.h.Env.Play.State().PositionMs; got != seeks[0] {
		t.Errorf("the mirror shows %d ms, want %d", got, seeks[0])
	}
}

func TestSeekingByKeyCommitsOnce(t *testing.T) {
	r := newRig(t)
	r.show(30_000)
	for range 12 {
		r.tt.Key(0, ui.KeyTab)
		if r.tt.Focused("Seek") {
			break
		}
	}
	if !r.tt.Focused("Seek") {
		t.Fatal("Tab never reached the seek slider")
	}
	r.tt.Key(0, ui.KeyRight)
	r.svc.wait(t, 1)
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	if len(r.svc.seeks) != 1 || r.svc.seeks[0] != 31_000 {
		t.Errorf("an arrow sought %v, want one seek to 31000", r.svc.seeks)
	}
}

func TestExpandAndCloseCallTheDesktopService(t *testing.T) {
	r := newRig(t)
	r.show(30_000)
	if err := r.tt.Click("Open Sona"); err != nil {
		t.Fatal(err)
	}
	if got := r.desk.wait(t); got[len(got)-1] != "show main" {
		t.Errorf("Open Sona called %v", got)
	}
	if err := r.tt.Click("Close mini player"); err != nil {
		t.Fatal(err)
	}
	if got := r.desk.wait(t); got[len(got)-1] != "close mini" {
		t.Errorf("Close called %v", got)
	}
}

func TestCloseIsOutermostOnTheSideThePlatformPutsIt(t *testing.T) {
	r := newRig(t)
	r.show(30_000)
	x := func(label string) float32 {
		b, ok := r.tt.Find(label)
		if !ok {
			t.Fatalf("no %q", label)
		}
		return b.X
	}
	if x("Close mini player") < x("Open Sona") {
		t.Error("Close is not the outer button at the end by default")
	}
	// macOS's inset title bar, and a desktop that puts Close first, lead.
	for name, wc := range map[string]services.WindowControls{
		"inset":      {Native: "inset"},
		"start side": {Native: "frame", Start: []string{"close"}, End: []string{"minimize"}},
	} {
		store.Set(r.h.Env.Queries, store.KeyCapabilities, services.Capabilities{WindowControls: wc})
		r.tt.Frame()
		if x("Close mini player") > x("Open Sona") {
			t.Errorf("%s: Close is not outermost at the start", name)
		}
		if x("Open Sona") > x("Song Title") {
			t.Errorf("%s: the buttons are not before the title", name)
		}
	}
}

func TestAFailedCommandIsSaidOnTheThirdLineForFourSecondsAndAnnouncedOnce(t *testing.T) {
	r := newRig(t)
	r.show(30_000)
	r.tt.Announcements()
	r.svc.fail = services.ErrNotReleased
	_ = r.tt.Click("Pause")
	r.svc.wait(t, 1)
	why := "Couldn't play or pause: " + i18n.ErrorText(services.CodeNotReleased)
	r.tt.Until("the notice", r.tt.Shows(why))
	r.wantNot(t, "The Album")
	got := r.tt.Announcements()
	if !slices.Contains(got, why) || countOf(got, why) != 1 {
		t.Errorf("announced %q, want %q once", got, why)
	}
	r.advance(3 * time.Second)
	r.want(t, why)
	if again := r.tt.Announcements(); slices.Contains(again, why) {
		t.Errorf("announced again: %q", again)
	}
	r.advance(1100 * time.Millisecond)
	r.wantNot(t, why)
	r.want(t, "The Album")
}

func countOf(l []string, s string) (n int) {
	for _, v := range l {
		if v == s {
			n++
		}
	}
	return n
}

func TestASignInFailureSaysToSignInInSona(t *testing.T) {
	r := newRig(t)
	r.show(30_000)
	r.svc.fail = auth.ErrNotSignedIn
	_ = r.tt.Click("Next")
	r.svc.wait(t, 1)
	r.tt.Until("the notice", r.tt.Shows("Couldn't skip: sign in to Apple Music in Sona"))
}

func TestARepeatedFailureStartsItsFourSecondsAgain(t *testing.T) {
	r := newRig(t)
	r.show(30_000)
	r.svc.fail = errors.New("boom")
	_ = r.tt.Click("Next")
	r.svc.wait(t, 1)
	msg := "Couldn't skip: " + i18n.ErrorText("")
	r.tt.Until("the notice", r.tt.Shows(msg))
	r.advance(3 * time.Second)
	_ = r.tt.Click("Next")
	r.svc.wait(t, 1)
	r.tt.Until("a second notice", func() bool { return r.m.noticeID == 2 })
	r.advance(3 * time.Second)
	r.want(t, msg)
}

func TestTheMiniPlayerAnnouncesANewTrackPolitelyOnce(t *testing.T) {
	r := newRig(t)
	r.tt.Announcements()
	r.show(0, func(s *services.PlaybackState) {
		s.Current = &services.QueueItem{ID: "a", Name: "Yellow", Artist: "Coldplay"}
	})
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Now playing: Yellow by Coldplay"}) {
		t.Fatalf("announced %q", got)
	}
	r.tt.Frame()
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("announced again: %q", got)
	}
	// A preview says so, and waits for the engine's report while loading.
	r.show(0, func(s *services.PlaybackState) {
		s.Current = &services.QueueItem{ID: "b", Name: "Clocks"}
		s.State = store.StateLoading
	})
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Fatalf("announced while loading: %q", got)
	}
	r.show(0, func(s *services.PlaybackState) {
		s.Current = &services.QueueItem{ID: "b", Name: "Clocks"}
		s.Preview = true
	})
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Now playing a preview: Clocks"}) {
		t.Errorf("announced %q", got)
	}
}

// The sung line.

// lineAt is a lyric line from begin to end ms.
func lineAt(text string, begin, end int64) services.LyricLine {
	return services.LyricLine{BeginMs: begin, EndMs: end, Text: text, Agent: "v1", AgentType: "person"}
}

// withWords gives a line its text as words of 500 ms from its begin.
func withWords(l services.LyricLine) services.LyricLine {
	for i, w := range []string{"Hello ", "there ", "friend"} {
		b := l.BeginMs + int64(i)*500
		l.Words = append(l.Words, services.LyricWord{BeginMs: b, EndMs: b + 500, Text: w})
	}
	return l
}

func TestTheSungLineShowsTheLineBeingSungAndFallsBackBetweenLines(t *testing.T) {
	r := newRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: []services.LyricLine{
		lineAt("First line of the song", 1000, 3000),
		lineAt("Second line of the song", 5000, 7000),
		lineAt("Third line after a break", 15_000, 17_000),
	}})
	r.show(0)
	r.want(t, "The Album") // before the first line
	r.advance(2 * time.Second)
	r.tt.Until("the first line", r.tt.Shows("First line of the song"))
	r.wantNot(t, "The Album")

	// The next line, on the timer that waits for it.
	r.advance(3 * time.Second)
	r.want(t, "Second line of the song")
	r.wantNot(t, "First line of the song")

	// A break of three seconds or more rests the voice: the album is back.
	r.advance(4 * time.Second)
	r.want(t, "The Album")
	r.wantNot(t, "Second line of the song")
	r.advance(6 * time.Second)
	r.want(t, "Third line after a break")
}

func TestTheSungLineFollowsTheTimingOffset(t *testing.T) {
	r := newRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: []services.LyricLine{
		lineAt("First line of the song", 1000, 3000),
		lineAt("Second line of the song", 5000, 7000),
	}})
	r.show(4000)
	r.tt.Until("the first line", r.tt.Shows("First line of the song"))
	// Shown half a second later each press: two presses push the second line
	// past the position.
	lyricsync.Session().Nudge("1001", 1)
	lyricsync.Session().Nudge("1001", 1)
	r.h.Main.Drain()
	r.advance(600 * time.Millisecond)
	r.want(t, "First line of the song")
	lyricsync.Session().Reset("1001")
	r.advance(200 * time.Millisecond)
	r.want(t, "Second line of the song")
}

func TestTheSungLineIsLeftToSyncedLyricsOfSongs(t *testing.T) {
	// Plain lyrics, a live station, a music video and a signed-out player show
	// the third line as it is.
	plain := services.Lyrics{Timing: "None", Lines: []services.LyricLine{{Text: "Plain words"}}}
	synced := services.Lyrics{Timing: "Line", Lines: []services.LyricLine{lineAt("Sung words", 0, 100_000)}}
	for name, c := range map[string]struct {
		lyr    services.Lyrics
		mutate func(*services.PlaybackState)
	}{
		"plain lyrics": {plain, nil},
		"a station":    {synced, station},
		"a video":      {synced, func(s *services.PlaybackState) { s.Current.Video = true }},
		"signed out":   {synced, func(s *services.PlaybackState) { s.AuthRequired = true }},
		"a failure":    {synced, func(s *services.PlaybackState) { s.State, s.Error = store.StateError, services.CodeNotReleased }},
	} {
		r := newRig(t)
		r.lyr.set(c.lyr)
		var mutate []func(*services.PlaybackState)
		if c.mutate != nil {
			mutate = append(mutate, c.mutate)
		}
		r.show(1000, mutate...)
		r.tt.Drain()
		r.tt.Frame()
		r.wantNot(t, "Sung words", "Plain words")
		if t.Failed() {
			t.Fatalf("%s showed lyrics", name)
		}
	}
}

func TestTheSungLineLightsWordsWholeAsTheyBegin(t *testing.T) {
	words := withWords(lineAt("Hello there friend", 1000, 4000))
	for _, c := range []struct {
		at       int64
		lit      int
		wantNext int64
	}{
		{999, 0, 1},
		{1000, 1, 500},
		{1499, 1, 1},
		{1500, 2, 500},
		{2000, 3, lyricsync.Never},
		{9000, 3, lyricsync.Never},
	} {
		lit, next := litWords(words.Words, c.at)
		if lit != c.lit || next != c.wantNext {
			t.Errorf("at %d ms: %d lit, next in %d; want %d, %d", c.at, lit, next, c.lit, c.wantNext)
		}
	}
	// And the line shows as its words, whole.
	r := newRig(t)
	r.lyr.set(services.Lyrics{Timing: "Word", Lines: []services.LyricLine{words}})
	r.show(1200)
	r.tt.Until("the line", r.tt.Shows("Hello", "there", "friend"))
}

// The shell's contract.

func TestTheModuleOffersTheContentTheButtonAndTheShortcut(t *testing.T) {
	env := uitest.New(t, uitest.Bare()).Env
	mod := New(env)
	if mod.ID != "miniplayer" || mod.Slots.Mini == nil {
		t.Fatalf("module %q, mini content %v", mod.ID, mod.Slots.Mini != nil)
	}
	if len(mod.Shortcuts) != 1 {
		t.Fatalf("shortcuts %d", len(mod.Shortcuts))
	}
	s := mod.Shortcuts[0]
	if s.ID != "miniplayer.toggle" || s.Label() != "Show the mini player" || s.Group != "view" ||
		!slices.Equal(s.Keys, []string{"Ctrl+Shift+M"}) || s.Keywords() != "compact small window" || s.Run == nil {
		t.Errorf("shortcut %+v", s)
	}
	if env.PlayerControls.Empty() {
		t.Error("no Mini player button in the player bar's controls")
	}
}

// barRig is the player bar's controls as the bar builds them, with the toasts.
type barRig struct {
	tt   *uitest.Session
	desk *fakeDesk
	mod  module.Module
}

func newBarRig(t *testing.T) *barRig {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	b := &barRig{desk: newDesk()}
	b.mod = New(h.Env)
	// The module's own state, over fakes.
	m := newMini(h.Env)
	m.desk = b.desk
	h.Env.PlayerControls = module.Point{}
	h.Env.PlayerControls.Add(controlOrder, m.button)
	b.mod.Shortcuts[0].Run = m.toggle
	b.tt = h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		ui.Row(c).Fill().Children(func() { h.Env.PlayerControls.Build(c) })
		kit.Toaster(c)
	}, 400, 100)
	return b
}

func TestTheBarsMiniPlayerButtonTogglesTheMiniPlayer(t *testing.T) {
	b := newBarRig(t)
	if err := b.tt.Click("Mini player"); err != nil {
		t.Fatal(err)
	}
	if got := b.desk.wait(t); got[len(got)-1] != "toggle mini" {
		t.Errorf("called %v", got)
	}
	// The shortcut does the same.
	b.mod.Shortcuts[0].Run()
	if got := b.desk.wait(t); len(got) != 2 || got[1] != "toggle mini" {
		t.Errorf("the shortcut called %v", got)
	}
}

// The mini player over the real playback coordinator, as the bar's flow test.

func TestTheMiniPlayerPlaysPausesAndSkipsThroughTheCoordinator(t *testing.T) {
	h := uitest.New(t)
	mod := New(h.Env)
	mod.Init()
	pb := services.Service[services.PlaybackService](h.Env.Core)
	h.Env.Play.Seed(pb.State(), pb.Queue())
	tt := h.Tester(t, func(c *ui.Context) { mod.Slots.Mini(c) }, 420, 132)
	refs := []services.TrackRef{
		{ID: fixture.SongLibraryID(1, 0), Source: services.SourceLibrary},
		{ID: fixture.SongLibraryID(1, 1), Source: services.SourceLibrary},
	}
	if err := pb.PlayItems(context.Background(), refs, 0, services.PlayFrom{Kind: "album", ID: fixture.AlbumLibraryID(1)}); err != nil {
		t.Fatal(err)
	}
	tt.Until("the first song playing", tt.Shows("Pause"))
	hasCall := func(prefix string) bool {
		for _, c := range h.Engine.Calls() {
			if len(c) >= len(prefix) && c[:len(prefix)] == prefix {
				return true
			}
		}
		return false
	}
	if err := tt.Click("Pause"); err != nil {
		t.Fatal(err)
	}
	tt.Until("the engine paused and the mini player offering Play", func() bool { return hasCall("pause") && tt.HasText("Play") })
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	tt.Until("the engine resumed", func() bool { return hasCall("resume") && tt.HasText("Pause") })
	before := len(h.Engine.Played())
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	tt.Until("the second song played", func() bool { return len(h.Engine.Played()) > before })
}

// The button follows the player bar's queue button (v0.2.0's order: Private,
// Sleep, Lyrics, Queue, Mini player, Equaliser).
func TestTheButtonComesAfterTheQueueButton(t *testing.T) {
	if controlOrder < module.ControlsAfterQueue {
		t.Errorf("order %d puts the Mini player button before the queue's", controlOrder)
	}
}
