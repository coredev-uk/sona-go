package coordinator

import (
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

func explicit(its []Item, ids ...string) []Item {
	for i := range its {
		if slices.Contains(ids, its[i].ID) {
			its[i].Explicit = true
		}
	}
	return its
}

func TestRestrictedSkipsExplicit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		h.c.SetRestricted(true)
		q := explicit(items("a", "x", "b", "y"), "x", "y")

		// Starting on an explicit item plays the next allowed one.
		must(t, h.c.SetQueue(ctx, q, 1, ""))
		synctest.Wait()
		if h.current() != "b" {
			t.Fatalf("current %q, want b", h.current())
		}
		// Nothing allowed follows b: the engine has no next armed, and
		// the end of b ends the queue rather than playing y.
		if n := h.eng.armed(); n.ID != "" {
			t.Fatalf("armed %+v", n)
		}
		h.eng.end()
		synctest.Wait()
		if s := h.c.Snapshot(); s.State != playback.StateEnded || s.Current.Item.ID != "b" {
			t.Fatalf("after b: %s %v", s.State, s.Current)
		}
		must(t, h.c.Jump(ctx, 0))
		synctest.Wait()
		if n := h.eng.armed(); n.ID != "b" {
			t.Fatalf("armed after a: %+v, want b (x skipped)", n)
		}
		must(t, h.c.Next(ctx))
		if h.current() != "b" {
			t.Fatalf("Next played %q, want b", h.current())
		}
		if got := h.eng.playList(); slices.Contains(got, "x") || slices.Contains(got, "y") {
			t.Fatalf("explicit items played: %v", got)
		}

		// A queue of explicit items only does not play at all.
		if err := h.c.SetQueue(ctx, explicit(items("x", "y"), "x", "y"), 0, ""); !errors.Is(err, ErrRestricted) {
			t.Fatalf("all explicit: %v", err)
		}

		// Unrestricted, explicit items play.
		h.c.SetRestricted(false)
		must(t, h.c.SetQueue(ctx, q, 1, ""))
		if h.current() != "x" {
			t.Fatalf("unrestricted current %q", h.current())
		}
	})
}

func TestContainersAndModesReachActivity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			got []Activity
			log activityLog
		)
		h, st := stationHarness(t, 2)
		h.c.ObserveActivity(func(a Activity) {
			log.mu.Lock()
			got = append(got, a)
			log.mu.Unlock()
		})
		ctx := t.Context()
		album := Container{Kind: ContainerAlbum, ID: "1440"}
		h.c.SetShuffle(true)
		must(t, h.c.SetRepeat(RepeatAll))
		must(t, h.c.SetQueue(ctx, from(album, items("a")), 0, ""))
		must(t, h.c.PlayStation(ctx, Station{ID: "ra.9"}))
		synctest.Wait()
		_ = st
		log.mu.Lock()
		defer log.mu.Unlock()
		if len(got) != 3 {
			t.Fatalf("activities %+v", got)
		}
		if got[0].Item.Container != album || got[0].Mode != (PlayMode{Shuffle: true, Repeat: RepeatAll, AutoPlay: AutoPlayOff}) {
			t.Fatalf("album start %+v", got[0])
		}
		if c := got[2].Item.Container; got[2].Kind != ActivityStart || c != (Container{Kind: ContainerStation, ID: "ra.9"}) {
			t.Fatalf("station start %+v", got[2])
		}
	})
}

func TestGaplessAdvance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &activityLog{}
		h := newHarness(t, func(o *Options) { o.OnActivity = log.add })
		ctx := t.Context()
		its := items("a", "b", "c")
		its[0].Album, its[1].Album, its[2].Album = "One", "One", "Two"
		must(t, h.c.SetQueue(ctx, its, 0, ""))
		synctest.Wait()
		if n := h.eng.armed(); n != (playback.Next{ID: "b", Gapless: true}) {
			t.Fatalf("armed %+v", n)
		}
		time.Sleep(100 * time.Second)
		h.eng.advance()
		expect(t, log, "start a 0s", "end a 1m40s natural", "start b 0s")
		if s := h.c.Snapshot(); h.current() != "b" || s.State != playback.StatePlaying || s.History[0].ID != "a" {
			t.Fatalf("after advance %+v", s)
		}
		if n := h.eng.armed(); n != (playback.Next{ID: "c"}) {
			t.Fatalf("armed %+v", n)
		}
		if p := h.playedList(); len(p) != 1 || !p[0].completed {
			t.Fatalf("history %v", p)
		}
	})
}

func TestSleepAfterFadesPausesAndRestores(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetVolume(ctx, 0.8))
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		must(t, h.c.SetSleep(SleepAfter, 30*time.Second))
		if s := h.c.Snapshot().Sleep; s == nil || s.Mode != SleepAfter || !s.At.Equal(time.Now().Add(30*time.Second)) {
			t.Fatalf("sleep %+v", s)
		}
		time.Sleep(19 * time.Second)
		synctest.Wait()
		if v := h.eng.volumeList(); !slices.Equal(v, []float64{0.8}) {
			t.Fatalf("volume before the fade: %v", v)
		}
		time.Sleep(5900 * time.Millisecond) // 24.9 s: halfway through the fade
		synctest.Wait()
		v := h.eng.volumeList()
		if last := v[len(v)-1]; last < 0.38 || last > 0.46 {
			t.Fatalf("mid-fade volume %g (%v)", last, v)
		}
		for i := 2; i < len(v); i++ {
			if v[i] > v[i-1] {
				t.Fatalf("fade rises: %v", v)
			}
		}
		if s := h.c.Snapshot(); s.Volume != 0.8 {
			t.Fatalf("snapshot volume during fade %g", s.Volume)
		}
		time.Sleep(6 * time.Second)
		synctest.Wait()
		s := h.c.Snapshot()
		if s.State != playback.StatePaused || s.Sleep != nil || s.Volume != 0.8 || h.current() != "a" {
			t.Fatalf("after the timer %+v", s)
		}
		if st := h.eng.Status(); st.State != playback.StatePaused || st.Volume != 0.8 {
			t.Fatalf("engine %+v", st)
		}
	})
}

func TestSleepPauseDuringFadeRestoresVolume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a"), 0, ""))
		must(t, h.c.SetSleep(SleepAfter, 15*time.Second))
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if v := h.eng.Status().Volume; v >= 1 {
			t.Fatalf("not fading: %g", v)
		}
		must(t, h.c.Pause(ctx))
		synctest.Wait()
		if v := h.eng.Status().Volume; v != 1 {
			t.Fatalf("paused at faded volume %g", v)
		}
		// The deadline passes while paused: the timer just clears.
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if s := h.c.Snapshot(); s.Sleep != nil || s.State != playback.StatePaused {
			t.Fatalf("after deadline %+v", s)
		}
	})
}

func TestSleepEndOfTrack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("s30", "s40"), 0, ""))
		must(t, h.c.SetSleep(SleepTrack, 0))
		synctest.Wait()
		if n := h.eng.armed(); n.ID != "" {
			t.Fatalf("next armed although playback ends with this item: %+v", n)
		}
		time.Sleep(25 * time.Second)
		synctest.Wait()
		if v := h.eng.Status().Volume; v > 0.55 || v < 0.45 {
			t.Fatalf("fade at 5 s left: %g", v)
		}
		time.Sleep(5 * time.Second)
		h.eng.end()
		synctest.Wait()
		s := h.c.Snapshot()
		if s.State != playback.StatePaused || h.current() != "s40" || s.Position != 0 || s.Sleep != nil {
			t.Fatalf("after the track %+v", s)
		}
		if got := h.eng.playList(); !slices.Equal(got, []string{"s30"}) {
			t.Fatalf("plays %v", got)
		}
		if v := h.eng.Status().Volume; v != 1 {
			t.Fatalf("volume not restored: %g", v)
		}
		// Resume continues with the next item.
		must(t, h.c.Resume(ctx))
		if h.current() != "s40" || h.c.Snapshot().State != playback.StatePlaying {
			t.Fatalf("resume %+v", h.c.Snapshot())
		}
	})
}

func TestSleepEndOfQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _ := stationHarness(t, 2)
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		must(t, h.c.SetRepeat(RepeatAll))
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		must(t, h.c.SetSleep(SleepQueue, 0))
		h.eng.end()
		synctest.Wait()
		if h.current() != "b" || h.c.Snapshot().State != playback.StatePlaying {
			t.Fatalf("stopped before the last item: %+v", h.c.Snapshot())
		}
		h.eng.end()
		synctest.Wait()
		s := h.c.Snapshot()
		// Repeat all would wrap; the timer stops with the queue's first item selected.
		if s.State != playback.StatePaused || h.current() != "a" || s.Sleep != nil {
			t.Fatalf("after the queue %+v", s)
		}
		if got := h.eng.playList(); !slices.Equal(got, []string{"a", "b"}) {
			t.Fatalf("plays %v", got)
		}
	})
}

func TestUndoReplace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _ := stationHarness(t, 3)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b", "c"), 0, "album:1"))
		h.c.Append(items("d", "e"))
		h.c.PlayNext(items("n"))
		must(t, h.c.Next(ctx)) // n
		time.Sleep(10 * time.Second)
		before := h.c.Snapshot()
		if h.c.Snapshot().CanUndoReplace {
			t.Fatal("undo before a replace")
		}
		var added []string
		for _, e := range before.Queue {
			if e.Added {
				added = append(added, e.Item.ID)
			}
		}
		if !slices.Equal(added, []string{"n", "d", "e"}) {
			t.Fatalf("added %v", added)
		}

		must(t, h.c.PlayStation(ctx, Station{ID: "ra.1"}))
		if s := h.c.Snapshot(); !s.CanUndoReplace || s.Station == nil {
			t.Fatalf("after replace %+v", s)
		}
		ok, err := h.c.UndoReplace(ctx)
		must(t, err)
		s := h.c.Snapshot()
		if !ok || !slices.Equal(h.ids(), []string{"a", "n", "b", "c", "d", "e"}) || h.current() != "n" ||
			s.State != playback.StatePlaying || s.Position != 10*time.Second || s.Station != nil || s.Context != "album:1" || s.CanUndoReplace {
			t.Fatalf("undone %v %+v", h.ids(), s)
		}
		if !slices.Equal(uidsOf(s.Queue), uidsOf(before.Queue)) || !s.Queue[1].Added {
			t.Fatal("slots changed identity or lost their added mark")
		}
		if calls := h.eng.callList(); calls[len(calls)-1] != "play n 10s" {
			t.Fatalf("calls %v", calls)
		}

		// A paused queue comes back paused, without touching the engine's
		// new item beyond stopping it.
		must(t, h.c.Pause(ctx))
		must(t, h.c.SetQueue(ctx, items("x"), 0, ""))
		ok, err = h.c.UndoReplace(ctx)
		must(t, err)
		if s := h.c.Snapshot(); !ok || h.current() != "n" || s.State != playback.StatePaused || s.Position != 10*time.Second {
			t.Fatalf("paused undo %+v", s)
		}
		if calls := h.eng.callList(); calls[len(calls)-1] != "stop" {
			t.Fatalf("calls %v", calls)
		}
		if ok, _ := h.c.UndoReplace(ctx); ok {
			t.Fatal("undid twice")
		}
		must(t, h.c.SetQueue(ctx, items("x"), 0, ""))
		must(t, h.c.Stop(ctx))
		if ok, _ := h.c.UndoReplace(ctx); ok || h.c.Snapshot().CanUndoReplace {
			t.Fatal("Stop kept the undo")
		}
	})
}

func uidsOf(es []Entry) []uint64 {
	out := make([]uint64, len(es))
	for i, e := range es {
		out[i] = e.UID
	}
	return out
}

func TestInsertClampsToUserSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _ := stationHarness(t, 2)
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		synctest.Wait()            // AutoPlay slots follow b
		h.c.Insert(0, items("p"))  // before the current item: clamped after it
		h.c.Insert(99, items("q")) // past the station slots: clamped before them
		h.c.Insert(2, items("r"))  // between p and b
		if got := h.ids(); !slices.Equal(got[:5], []string{"a", "p", "r", "b", "q"}) || len(got) != 7 {
			t.Fatalf("queue %v", got)
		}
		s := h.c.Snapshot()
		if !s.Queue[1].Added || s.Queue[3].Added || !s.Queue[5].Auto {
			t.Fatalf("flags %+v", s.Queue)
		}
	})
}
