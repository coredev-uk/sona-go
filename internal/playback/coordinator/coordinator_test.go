package coordinator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
)

// fakeEngine plays instantly: Play goes straight to playing. Tests end or
// fail the current item explicitly. Events are latest-wins like the real
// contract.
type fakeEngine struct {
	mu       sync.Mutex
	st       playback.Status
	events   chan playback.Event
	calls    []string
	plays    []string
	preloads []string
	failPlay map[string]error
	next     playback.Next // armed by SetNext
	volumes  []float64     // SetVolume calls
}

func newFake() *fakeEngine {
	return &fakeEngine{
		st:       playback.Status{State: playback.StateIdle, Volume: 1},
		events:   make(chan playback.Event, 1),
		failPlay: map[string]error{},
	}
}

func (f *fakeEngine) emitLocked(kind playback.EventKind) {
	ev := playback.Event{Kind: kind, Status: f.st}
	select {
	case f.events <- ev:
	default:
		select {
		case <-f.events:
		default:
		}
		f.events <- ev
	}
}

func (f *fakeEngine) posLocked() time.Duration {
	p := f.st.Position
	if f.st.State == playback.StatePlaying {
		p += time.Since(f.st.UpdatedAt)
	}
	return min(p, f.st.Duration)
}

func (f *fakeEngine) Play(_ context.Context, id string, start time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next = playback.Next{}
	f.calls = append(f.calls, fmt.Sprintf("play %s %s", id, start))
	f.plays = append(f.plays, id)
	if err := f.failPlay[id]; err != nil {
		f.st = playback.Status{State: playback.StateError, ItemID: id, Volume: f.st.Volume, UpdatedAt: time.Now(), Err: err.Error()}
		return err
	}
	f.st = playback.Status{State: playback.StatePlaying, ItemID: id, Position: start, Duration: durationOf(id), Volume: f.st.Volume, UpdatedAt: time.Now()}
	f.emitLocked(playback.EventItem)
	return nil
}

func (f *fakeEngine) Preload(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.preloads = append(f.preloads, id)
	return nil
}

func (f *fakeEngine) set(s playback.State, call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	f.st.Position, f.st.UpdatedAt, f.st.State = f.posLocked(), time.Now(), s
	f.emitLocked(playback.EventState)
}

func (f *fakeEngine) Pause(context.Context) error { f.set(playback.StatePaused, "pause"); return nil }

func (f *fakeEngine) Resume(context.Context) error {
	f.set(playback.StatePlaying, "resume")
	return nil
}

func (f *fakeEngine) Stop(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "stop")
	f.next = playback.Next{}
	f.st = playback.Status{State: playback.StateIdle, Volume: f.st.Volume, UpdatedAt: time.Now()}
	f.emitLocked(playback.EventState)
	return nil
}

func (f *fakeEngine) Seek(_ context.Context, pos time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("seek %s", pos))
	f.st.Position, f.st.UpdatedAt = pos, time.Now()
	f.emitLocked(playback.EventState)
	return nil
}

func (f *fakeEngine) SetVolume(_ context.Context, v float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("volume %g", v))
	f.volumes = append(f.volumes, v)
	f.st.Volume = v
	f.emitLocked(playback.EventState)
	return nil
}

func (f *fakeEngine) Status() playback.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st
}

func (f *fakeEngine) Events() <-chan playback.Event       { return f.events }
func (f *fakeEngine) Capabilities() playback.Capabilities { return playback.Capabilities{} }
func (f *fakeEngine) Close() error                        { return nil }

func (f *fakeEngine) SetNext(n playback.Next) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next = n
}

func (f *fakeEngine) armed() playback.Next {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.next
}

// advance plays the current item out into the armed next, like a
// gapless engine; it reports whether one was armed.
func (f *fakeEngine) advance() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.next.ID == "" {
		return false
	}
	id := f.next.ID
	f.next = playback.Next{}
	f.plays = append(f.plays, id)
	f.st = playback.Status{State: playback.StatePlaying, ItemID: id, Duration: durationOf(id), Volume: f.st.Volume, UpdatedAt: time.Now()}
	f.emitLocked(playback.EventAdvance)
	return true
}

func (f *fakeEngine) volumeList() []float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.volumes)
}

// end finishes the current item.
func (f *fakeEngine) end() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.State, f.st.Position, f.st.UpdatedAt = playback.StateEnded, f.st.Duration, time.Now()
	f.emitLocked(playback.EventState)
}

// fail makes the current item fail mid-playback.
func (f *fakeEngine) fail(msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.State, f.st.Err, f.st.UpdatedAt = playback.StateError, msg, time.Now()
	f.emitLocked(playback.EventError)
}

func (f *fakeEngine) authFail() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.Position, f.st.UpdatedAt = f.posLocked(), time.Now()
	f.emitLocked(playback.EventAuth)
}

func (f *fakeEngine) playList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.plays)
}

func (f *fakeEngine) callList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeEngine) preloadList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.preloads)
}

// Items named "sN" are N seconds long; everything else is 100 s.
func durationOf(id string) time.Duration {
	var n int
	if _, err := fmt.Sscanf(id, "s%d", &n); err == nil {
		return time.Duration(n) * time.Second
	}
	return 100 * time.Second
}

func items(ids ...string) []Item {
	out := make([]Item, len(ids))
	for i, id := range ids {
		out[i] = Item{ID: id, Title: "T " + id, Duration: durationOf(id)}
	}
	return out
}

// from sets c as the container of its.
func from(c Container, its []Item) []Item {
	for i := range its {
		its[i].Container = c
	}
	return its
}

type played struct {
	id        string
	for_      time.Duration
	completed bool
}

type harness struct {
	t      *testing.T
	eng    *fakeEngine
	c      *Coordinator
	mu     sync.Mutex
	played []played
}

func newHarness(t *testing.T, mod func(*Options)) *harness {
	h := &harness{t: t, eng: newFake()}
	opts := Options{
		Engine: h.eng,
		Rand:   rand.New(rand.NewPCG(1, 2)),
		OnPlayed: func(it Item, d time.Duration, done bool) {
			h.mu.Lock()
			h.played = append(h.played, played{it.ID, d, done})
			h.mu.Unlock()
		},
	}
	if mod != nil {
		mod(&opts)
	}
	c, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	h.c = c
	t.Cleanup(func() { _ = c.Close() })
	return h
}

func (h *harness) playedList() []played {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.played)
}

func (h *harness) current() string {
	h.t.Helper()
	s := h.c.Snapshot()
	if s.Current == nil {
		return ""
	}
	return s.Current.Item.ID
}

func (h *harness) ids() []string {
	var out []string
	for _, e := range h.c.Snapshot().Queue {
		out = append(out, e.Item.ID)
	}
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoAdvance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b", "c"), 0, "album:1"))
		synctest.Wait()
		for _, want := range []string{"b", "c"} {
			h.eng.end()
			synctest.Wait()
			if got := h.current(); got != want {
				t.Fatalf("current %q, want %q", got, want)
			}
			if s := h.c.Snapshot(); s.State != playback.StatePlaying {
				t.Fatalf("state %s", s.State)
			}
		}
		h.eng.end()
		synctest.Wait()
		s := h.c.Snapshot()
		if s.State != playback.StateEnded || s.Current.Item.ID != "c" {
			t.Fatalf("end of queue: %s %v", s.State, s.Current)
		}
		if got := h.eng.playList(); !slices.Equal(got, []string{"a", "b", "c"}) {
			t.Fatalf("plays %v", got)
		}
		p := h.playedList()
		if len(p) != 3 || !p[0].completed || !p[2].completed {
			t.Fatalf("history %v", p)
		}
		if s.Context != "album:1" || len(s.History) != 3 || s.History[0].ID != "c" {
			t.Fatalf("snapshot %+v", s)
		}
		// Resume after the end replays the last item.
		must(t, h.c.Resume(ctx))
		if got := h.eng.playList(); got[len(got)-1] != "c" || len(got) != 4 {
			t.Fatalf("plays %v", got)
		}
	})
}

func TestRepeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetRepeat(RepeatOne))
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		h.eng.end()
		synctest.Wait()
		h.eng.end()
		synctest.Wait()
		if got := h.eng.playList(); !slices.Equal(got, []string{"a", "a", "a"}) {
			t.Fatalf("repeat one plays %v", got)
		}
		// A user skip leaves the repeated item.
		must(t, h.c.Next(ctx))
		if h.current() != "b" {
			t.Fatalf("next under repeat one: %s", h.current())
		}

		must(t, h.c.SetRepeat(RepeatAll))
		h.eng.end()
		synctest.Wait()
		if h.current() != "a" {
			t.Fatalf("repeat all did not wrap: %s", h.current())
		}
		must(t, h.c.Previous(ctx)) // at index 0, wraps back to the last item
		if h.current() != "b" {
			t.Fatalf("previous under repeat all: %s", h.current())
		}
		if err := h.c.SetRepeat("sometimes"); err == nil {
			t.Fatal("invalid repeat accepted")
		}
	})
}

func TestShuffle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ids := make([]string, 20)
		for i := range ids {
			ids[i] = fmt.Sprintf("t%02d", i)
		}
		run := func() []string {
			h := newHarness(t, nil)
			ctx := t.Context()
			must(t, h.c.SetQueue(ctx, items(ids...), 5, ""))
			before := h.c.Snapshot()
			h.c.SetShuffle(true)
			s := h.c.Snapshot()
			if s.Index != 0 || s.Current.Item.ID != "t05" || s.Current.UID != before.Current.UID {
				t.Fatalf("current moved: index %d %v", s.Index, s.Current)
			}
			got := h.ids()
			if slices.Equal(got, ids) {
				t.Fatal("shuffle kept the order")
			}
			if !slices.Equal(slices.Sorted(slices.Values(got)), ids) {
				t.Fatalf("shuffle changed the items: %v", got)
			}
			if len(h.eng.playList()) != 1 {
				t.Fatal("shuffle restarted playback")
			}
			h.c.Append(items("extra"))
			h.c.SetShuffle(false)
			s = h.c.Snapshot()
			want := append(slices.Clone(ids), "extra")
			if !slices.Equal(h.ids(), want) || s.Index != 5 || s.Current.Item.ID != "t05" {
				t.Fatalf("unshuffle: %v index %d", h.ids(), s.Index)
			}
			return got
		}
		if a, b := run(), run(); !slices.Equal(a, b) {
			t.Fatalf("same seed, different orders:\n%v\n%v", a, b)
		}

		// SetQueue with shuffle on plays the chosen item first.
		h := newHarness(t, nil)
		h.c.SetShuffle(true)
		must(t, h.c.SetQueue(t.Context(), items(ids...), 7, ""))
		if s := h.c.Snapshot(); s.Index != 0 || s.Current.Item.ID != "t07" || h.eng.playList()[0] != "t07" {
			t.Fatalf("shuffled SetQueue: %v", s.Current)
		}
		h.c.SetShuffle(false)
		if s := h.c.Snapshot(); s.Index != 7 || !slices.Equal(h.ids(), ids) {
			t.Fatalf("unshuffled SetQueue: index %d %v", s.Index, h.ids())
		}
	})
}

func TestPreviousThreshold(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b"), 1, ""))
		time.Sleep(2 * time.Second)
		must(t, h.c.Previous(ctx))
		if h.current() != "a" {
			t.Fatalf("previous within 3 s should go back, at %s", h.current())
		}
		must(t, h.c.Next(ctx))
		time.Sleep(5 * time.Second)
		must(t, h.c.Previous(ctx))
		calls := h.eng.callList()
		if h.current() != "b" || calls[len(calls)-1] != "seek 0s" {
			t.Fatalf("previous after 5 s should restart b: %s %v", h.current(), calls)
		}
		if p := h.c.Snapshot().Position; p != 0 {
			t.Fatalf("position after restart %s", p)
		}
		// At the first item with repeat off, Previous restarts.
		must(t, h.c.Jump(ctx, 0))
		must(t, h.c.Previous(ctx))
		if calls := h.eng.callList(); h.current() != "a" || calls[len(calls)-1] != "seek 0s" {
			t.Fatalf("previous at first item: %s %v", h.current(), calls)
		}
	})
}

func TestPreloadTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("s60", "b", "c"), 0, ""))
		synctest.Wait()
		// No events arrive while playing; the deadline is extrapolated.
		time.Sleep(44 * time.Second)
		synctest.Wait()
		if p := h.eng.preloadList(); len(p) != 0 {
			t.Fatalf("preloaded %v 16 s before the end", p)
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if p := h.eng.preloadList(); !slices.Equal(p, []string{"b"}) {
			t.Fatalf("preloads %v, want [b] 14 s before the end", p)
		}
		// Pausing stops the clock: on b, pause at 10 s, wait, no preload.
		h.eng.end()
		synctest.Wait()
		time.Sleep(10 * time.Second)
		must(t, h.c.Pause(ctx))
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		if p := h.eng.preloadList(); len(p) != 1 {
			t.Fatalf("preloaded while paused: %v", p)
		}
		// Seeking into the last 15 s preloads at once.
		must(t, h.c.Resume(ctx))
		must(t, h.c.Seek(ctx, 90*time.Second))
		synctest.Wait()
		if p := h.eng.preloadList(); !slices.Equal(p, []string{"b", "c"}) {
			t.Fatalf("preloads after seek %v", p)
		}
		// A queue change re-targets the preload.
		h.c.PlayNext(items("x"))
		synctest.Wait()
		if p := h.eng.preloadList(); !slices.Equal(p, []string{"b", "c", "x"}) {
			t.Fatalf("preloads after PlayNext %v", p)
		}
	})
}

func TestFailureSkipping(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		h.eng.failPlay["a"] = errors.New("no asset")
		h.eng.failPlay["b"] = errors.New("licence denied")
		sub, cancel := h.c.Subscribe()
		defer cancel()
		var sawErr sync.WaitGroup
		var errs []string
		sawErr.Go(func() {
			for s := range sub {
				if s.Error != "" {
					errs = append(errs, s.Error)
				}
			}
		})
		must(t, h.c.SetQueue(ctx, items("a", "b", "c", "d", "e"), 0, ""))
		if got := h.eng.playList(); !slices.Equal(got, []string{"a", "b", "c"}) || h.current() != "c" {
			t.Fatalf("plays %v current %s", got, h.current())
		}
		if s := h.c.Snapshot(); s.State != playback.StatePlaying || s.Error != "" {
			t.Fatalf("after recovery: %s %q", s.State, s.Error)
		}
		// A mid-playback failure skips too.
		h.eng.fail("decrypt error")
		synctest.Wait()
		if h.current() != "d" {
			t.Fatalf("mid-playback failure: at %s", h.current())
		}

		// N consecutive failures stop playback.
		for _, id := range []string{"a", "b", "c", "d", "e"} {
			h.eng.failPlay[id] = errors.New("down")
		}
		err := h.c.Jump(ctx, 0)
		if err == nil {
			t.Fatal("Jump succeeded with every item failing")
		}
		if got := h.eng.playList(); !slices.Equal(got[len(got)-3:], []string{"a", "b", "c"}) || len(got) != 7 {
			t.Fatalf("plays %v, want stop after 3 failures", got)
		}
		s := h.c.Snapshot()
		if s.State != playback.StateError || s.Error == "" {
			t.Fatalf("stopped state %s %q", s.State, s.Error)
		}
		h.c.Close()
		sawErr.Wait()
		if len(errs) == 0 {
			t.Fatal("subscribers never saw an error")
		}
	})
}

func TestAuthPauses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		time.Sleep(10 * time.Second)
		h.eng.authFail()
		synctest.Wait()
		s := h.c.Snapshot()
		if !s.AuthRequired || s.State != playback.StatePaused || h.current() != "a" {
			t.Fatalf("after auth event: %+v", s)
		}
		if calls := h.eng.callList(); calls[len(calls)-1] != "pause" {
			t.Fatalf("engine not paused: %v", calls)
		}
		must(t, h.c.Resume(ctx))
		if s := h.c.Snapshot(); s.AuthRequired || s.State != playback.StatePlaying {
			t.Fatalf("after resume: %+v", s)
		}

		// A 403 from Play pauses instead of burning through the queue.
		h.eng.failPlay["b"] = fmt.Errorf("webPlayback: %w", &applemusic.StatusError{Code: 403})
		if err := h.c.Next(ctx); err == nil {
			t.Fatal("Next succeeded")
		}
		s = h.c.Snapshot()
		if !s.AuthRequired || s.State != playback.StatePaused || h.current() != "b" || len(h.eng.playList()) != 2 {
			t.Fatalf("after 403: %+v plays %v", s, h.eng.playList())
		}
	})
}

// An engine that cannot play on this platform pauses on the selected item
// with the reason, instead of skipping through the queue as failures.
func TestUnsupportedEnginePausesWithoutFailing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &activityLog{}
		h := newHarness(t, func(o *Options) { o.OnActivity = log.add })
		ctx := t.Context()
		for _, id := range []string{"a", "b", "c"} {
			h.eng.failPlay[id] = playback.ErrUnsupported
		}
		check := func(step, want string, plays int) {
			t.Helper()
			s := h.c.Snapshot()
			if h.current() != want || s.State != playback.StatePaused || s.Error != playback.ErrUnsupported.Error() {
				t.Fatalf("%s: current %q state %s error %q", step, h.current(), s.State, s.Error)
			}
			if got := h.eng.playList(); len(got) != plays {
				t.Fatalf("%s: plays %v", step, got)
			}
		}
		if err := h.c.SetQueue(ctx, items("a", "b", "c"), 0, ""); !errors.Is(err, playback.ErrUnsupported) {
			t.Fatalf("SetQueue: %v, want ErrUnsupported", err)
		}
		check("SetQueue", "a", 1)
		if err := h.c.Next(ctx); !errors.Is(err, playback.ErrUnsupported) {
			t.Fatalf("Next: %v, want ErrUnsupported", err)
		}
		check("Next", "b", 2)
		if err := h.c.Resume(ctx); !errors.Is(err, playback.ErrUnsupported) {
			t.Fatalf("Resume: %v, want ErrUnsupported", err)
		}
		check("Resume", "b", 3)
		synctest.Wait()
		if p := h.playedList(); len(p) != 0 {
			t.Errorf("listens reported: %v", p)
		}
		expect(t, log)
	})
}

// Without a CDM ready (still downloading) the item is kept, paused, with
// NoCDM set and nothing counted; once the CDM is ready, play works and
// NoCDM clears.
func TestNoCDMPausesUntilTheCDMIsReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		noCDM := fmt.Errorf("cdm: still downloading: %w", playback.ErrNoCDM)
		h.eng.failPlay["a"] = noCDM
		if err := h.c.SetQueue(ctx, items("a", "b"), 0, ""); !errors.Is(err, playback.ErrNoCDM) {
			t.Fatalf("SetQueue: %v, want ErrNoCDM", err)
		}
		s := h.c.Snapshot()
		if h.current() != "a" || s.State != playback.StatePaused || !s.NoCDM || s.Error != noCDM.Error() {
			t.Fatalf("without a CDM: current %q %+v", h.current(), s)
		}
		h.eng.mu.Lock()
		delete(h.eng.failPlay, "a")
		h.eng.mu.Unlock()
		must(t, h.c.Resume(ctx))
		s = h.c.Snapshot()
		if h.current() != "a" || s.State != playback.StatePlaying || s.NoCDM {
			t.Fatalf("with the CDM: current %q %+v", h.current(), s)
		}
	})
}

func TestHistoryThresholds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b", "s40", "c", "d", "e"), 0, ""))

		time.Sleep(10 * time.Second) // a: 10 s of 100 s
		must(t, h.c.Next(ctx))
		time.Sleep(31 * time.Second) // b: 31 s
		must(t, h.c.Next(ctx))
		time.Sleep(21 * time.Second) // s40: over half
		must(t, h.c.Next(ctx))
		time.Sleep(20 * time.Second) // c: 20 s + 5 s around a long pause
		must(t, h.c.Pause(ctx))
		time.Sleep(time.Minute)
		must(t, h.c.Resume(ctx))
		time.Sleep(5 * time.Second)
		must(t, h.c.Next(ctx))
		h.eng.end() // d: completes after 0 s of listening
		synctest.Wait()

		want := []played{{"b", 31 * time.Second, false}, {"s40", 21 * time.Second, false}, {"d", 0, true}}
		if got := h.playedList(); !slices.Equal(got, want) {
			t.Fatalf("played %v, want %v", got, want)
		}
		hist := h.c.Snapshot().History
		if len(hist) != 3 || hist[0].ID != "d" || hist[2].ID != "b" {
			t.Fatalf("session history %v", hist)
		}
	})
}

func TestRestore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := FileStore{Path: filepath.Join(t.TempDir(), "state", "queue.json")}
		withStore := func(o *Options) { o.Store = store }

		h := newHarness(t, withStore)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b", "c"), 0, "playlist:p.1"))
		h.c.SetShuffle(true)
		must(t, h.c.SetRepeat(RepeatAll))
		must(t, h.c.SetVolume(ctx, 0.4))
		must(t, h.c.Next(ctx))
		must(t, h.c.Seek(ctx, 42*time.Second))
		time.Sleep(3 * time.Second)
		must(t, h.c.Pause(ctx))
		before := h.c.Snapshot()
		must(t, h.c.Close())

		h2 := newHarness(t, withStore)
		must(t, h2.c.Restore(ctx))
		s := h2.c.Snapshot()
		if s.State != playback.StatePaused || s.Current == nil || s.Current.Item.ID != before.Current.Item.ID ||
			s.Position != 45*time.Second || s.Index != before.Index {
			t.Fatalf("restored %+v, before %+v", s, before)
		}
		if !slices.Equal(h2.ids(), h.ids()) || !s.Shuffle || s.Repeat != RepeatAll || s.Volume != 0.4 || s.Context != "playlist:p.1" {
			t.Fatalf("restored queue %v %+v", h2.ids(), s)
		}
		if calls := h2.eng.callList(); len(calls) != 0 {
			t.Fatalf("restore touched the engine: %v", calls)
		}
		must(t, h2.c.Resume(ctx))
		calls := h2.eng.callList()
		if !slices.Equal(calls, []string{"volume 0.4", "play " + s.Current.Item.ID + " 45s"}) {
			t.Fatalf("resume after restore: %v", calls)
		}
		h2.c.SetShuffle(false)
		if !slices.Equal(h2.ids(), []string{"a", "b", "c"}) {
			t.Fatalf("restored unshuffle order %v", h2.ids())
		}
	})
}

func TestQueueOps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b", "c"), 1, ""))
		h.c.PlayNext(items("n1", "n2"))
		h.c.Append(items("z"))
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "n1", "n2", "c", "z"}) {
			t.Fatalf("after PlayNext/Append %v", got)
		}
		must(t, h.c.Remove(ctx, 0))
		if s := h.c.Snapshot(); s.Index != 0 || s.Current.Item.ID != "b" {
			t.Fatalf("remove before current: %d", s.Index)
		}
		must(t, h.c.Move(4, 0)) // z to the front
		if s := h.c.Snapshot(); !slices.Equal(h.ids(), []string{"z", "b", "n1", "n2", "c"}) || s.Index != 1 {
			t.Fatalf("move: %v index %d", h.ids(), s.Index)
		}
		must(t, h.c.Remove(ctx, 1)) // current while playing: n1 takes over
		if h.current() != "n1" || h.eng.playList()[1] != "n1" {
			t.Fatalf("remove current: %s %v", h.current(), h.eng.playList())
		}
		must(t, h.c.Jump(ctx, 3))
		if h.current() != "c" {
			t.Fatalf("jump: %s", h.current())
		}
		if err := h.c.Jump(ctx, 9); err == nil {
			t.Fatal("jump out of range accepted")
		}
		h.c.Clear()
		if s := h.c.Snapshot(); !slices.Equal(h.ids(), []string{"c"}) || s.Index != 0 || s.State != playback.StatePlaying {
			t.Fatalf("clear: %v %d %s", h.ids(), s.Index, s.State)
		}
		must(t, h.c.Stop(ctx))
		if s := h.c.Snapshot(); len(s.Queue) != 0 || s.Current != nil || s.State != playback.StateIdle {
			t.Fatalf("stop: %+v", s)
		}
		if calls := h.eng.callList(); calls[len(calls)-1] != "stop" {
			t.Fatalf("engine not stopped: %v", calls)
		}
	})
}

func TestStaleEventsIgnored(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		synctest.Wait()
		// An Ended for a that is delivered after the user moved on to b
		// must not advance past b.
		must(t, h.c.Next(ctx))
		h.eng.events <- playback.Event{Kind: playback.EventState, Status: playback.Status{State: playback.StateEnded, ItemID: "a"}}
		synctest.Wait()
		if h.current() != "b" || len(h.eng.playList()) != 2 {
			t.Fatalf("stale event advanced: %s %v", h.current(), h.eng.playList())
		}
	})
}

// NaN passes min/max clamps; it must not reach the engine, the snapshot
// (JSON cannot encode it) or the saved settings.
func TestSetVolumeNaN(t *testing.T) {
	h := newHarness(t, nil)
	must(t, h.c.SetVolume(t.Context(), 0.5))
	must(t, h.c.SetVolume(t.Context(), math.NaN()))
	if v := h.c.Snapshot().Volume; v != 0 {
		t.Fatalf("snapshot volume %v", v)
	}
	h.eng.mu.Lock()
	defer h.eng.mu.Unlock()
	if !slices.Equal(h.eng.volumes, []float64{0.5, 0}) {
		t.Fatalf("engine volumes %v", h.eng.volumes)
	}
}
