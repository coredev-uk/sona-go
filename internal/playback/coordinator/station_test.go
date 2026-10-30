package coordinator

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// fakeStations hands out batches of `batch` items named "<station>-<n>".
type fakeStations struct {
	mu        sync.Mutex
	batch     int
	n         map[string]int
	next      []string // NextTracks calls (station IDs)
	cont      [][]string
	fail      int           // fail this many NextTracks calls
	gate      chan struct{} // when set, NextTracks waits for it
	contLabel string
}

func newStations(batch int) *fakeStations {
	return &fakeStations{batch: batch, n: map[string]int{}}
}

func (f *fakeStations) NextTracks(ctx context.Context, id string) ([]Item, error) {
	f.mu.Lock()
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next = append(f.next, id)
	if f.fail > 0 {
		f.fail--
		return nil, errors.New("station unavailable")
	}
	out := make([]string, f.batch)
	for i := range out {
		f.n[id]++
		out[i] = fmt.Sprintf("%s-%d", id, f.n[id])
	}
	return items(out...), nil
}

func (f *fakeStations) Continue(_ context.Context, seeds []Item, label string) (Station, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, len(seeds))
	for i, s := range seeds {
		ids[i] = s.ID
	}
	f.cont = append(f.cont, ids)
	f.contLabel = label
	return Station{ID: "ra.cp-" + ids[0], Name: "Continue"}, nil
}

func (f *fakeStations) calls() (next []string, cont [][]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.next), slices.Clone(f.cont)
}

func stationHarness(t *testing.T, batch int) (*harness, *fakeStations) {
	st := newStations(batch)
	h := newHarness(t, func(o *Options) { o.Stations = st })
	return h, st
}

func autoFlags(s Snapshot) []bool {
	out := make([]bool, len(s.Queue))
	for i, e := range s.Queue {
		out[i] = e.Auto
	}
	return out
}

func TestStationRefill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 3)
		ctx := t.Context()
		must(t, h.c.PlayStation(ctx, Station{ID: "ra.1", Name: "One"}))
		synctest.Wait()
		// The first batch leaves 2 after the current item: a refill starts at once.
		if got := h.ids(); !slices.Equal(got, []string{"ra.1-1", "ra.1-2", "ra.1-3", "ra.1-4", "ra.1-5", "ra.1-6"}) {
			t.Fatalf("queue %v", got)
		}
		s := h.c.Snapshot()
		if s.Station == nil || s.Station.ID != "ra.1" || s.Station.AutoPlay || s.Context != "ra.1" {
			t.Fatalf("station %+v context %q", s.Station, s.Context)
		}
		if slices.Contains(autoFlags(s), false) {
			t.Fatalf("station slots not auto: %v", autoFlags(s))
		}
		// Play through: nothing refills while more than 2 remain.
		for range 2 {
			h.eng.end()
			synctest.Wait()
		}
		if got, _ := st.calls(); len(got) != 2 {
			t.Fatalf("next-tracks calls %v", got)
		}
		h.eng.end() // ra.1-4 plays, 2 remain
		synctest.Wait()
		if got, _ := st.calls(); len(got) != 3 {
			t.Fatalf("next-tracks calls %v", got)
		}
		if h.current() != "ra.1-4" || len(h.ids()) != 9 {
			t.Fatalf("current %s queue %v", h.current(), h.ids())
		}
		// Repeat all does not wrap a station: it keeps going.
		must(t, h.c.SetRepeat(RepeatAll))
		for range 5 {
			must(t, h.c.Next(ctx))
		}
		synctest.Wait()
		if h.current() != "ra.1-9" || len(h.ids()) != 12 {
			t.Fatalf("current %s queue %v", h.current(), h.ids())
		}
		// Previous goes back through played station slots.
		must(t, h.c.Previous(ctx))
		if h.current() != "ra.1-8" {
			t.Fatalf("previous: %s", h.current())
		}
	})
}

// playToLast ends items until the last queued one plays.
func playToLast(h *harness) {
	for {
		s := h.c.Snapshot()
		if s.Index == len(s.Queue)-1 {
			return
		}
		h.eng.end()
		synctest.Wait()
	}
}

func TestStationQueueEndWaitsForRefill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 3)
		ctx := t.Context()
		must(t, h.c.PlayStation(ctx, Station{ID: "ra.2"}))
		synctest.Wait()
		// Background refills fail (and back off) from here on.
		st.mu.Lock()
		st.fail = 100
		st.mu.Unlock()
		playToLast(h)
		if h.current() != "ra.2-6" {
			t.Fatalf("current %s queue %v", h.current(), h.ids())
		}
		// The queue runs out: the end waits for a forced refill (no backoff).
		st.mu.Lock()
		st.fail = 0
		st.mu.Unlock()
		h.eng.end()
		synctest.Wait()
		if h.current() != "ra.2-7" {
			t.Fatalf("after running out: current %s queue %v", h.current(), h.ids())
		}
		if s := h.c.Snapshot(); s.State != playback.StatePlaying {
			t.Fatalf("state %s", s.State)
		}
	})
}

func TestStationFailingStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 3)
		ctx := t.Context()
		must(t, h.c.PlayStation(ctx, Station{ID: "ra.3"}))
		synctest.Wait()
		st.mu.Lock()
		st.fail = 100
		st.mu.Unlock()
		playToLast(h)
		h.eng.end() // the forced refill fails too: playback ends on the last item
		synctest.Wait()
		s := h.c.Snapshot()
		if s.State != playback.StateEnded || s.Current.Item.ID != "ra.3-6" {
			t.Fatalf("state %s current %v", s.State, s.Current)
		}
		// Next with nothing to continue stops as at the end of a queue.
		must(t, h.c.Next(ctx))
		if s := h.c.Snapshot(); s.State != playback.StateEnded {
			t.Fatalf("state %s", s.State)
		}
	})
}

func TestPlayStationFailureKeepsQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 2)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, "album:1"))
		st.fail = 1
		if err := h.c.PlayStation(ctx, Station{ID: "ra.x"}); err == nil {
			t.Fatal("no error")
		}
		if h.current() != "a" || h.c.Snapshot().Station != nil {
			t.Fatalf("queue changed: %v", h.ids())
		}
	})
}

func TestAutoPlay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 2)
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		must(t, h.c.SetQueue(ctx, items("a", "b", "c", "d"), 0, "album:1"))
		synctest.Wait()
		if _, cont := st.calls(); len(cont) != 0 {
			t.Fatalf("AutoPlay started with 3 user items left: %v", cont)
		}
		h.eng.end() // b: 2 left
		synctest.Wait()
		_, cont := st.calls()
		if len(cont) != 1 || !slices.Equal(cont[0], []string{"d"}) || st.contLabel != "album:1" {
			t.Fatalf("continue calls %v label %q", cont, st.contLabel)
		}
		s := h.c.Snapshot()
		if s.Station == nil || s.Station.ID != "ra.cp-d" || !s.Station.AutoPlay || !s.AutoPlay {
			t.Fatalf("station %+v", s.Station)
		}
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c", "d", "ra.cp-d-1", "ra.cp-d-2"}) {
			t.Fatalf("queue %v", got)
		}
		// User items go before the AutoPlay slots.
		h.c.Append(items("e"))
		h.c.PlayNext(items("n"))
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "n", "c", "d", "e", "ra.cp-d-1", "ra.cp-d-2"}) {
			t.Fatalf("queue %v", got)
		}
		if got := autoFlags(h.c.Snapshot()); !slices.Equal(got, []bool{false, false, false, false, false, false, true, true}) {
			t.Fatalf("auto %v", got)
		}
		// Clear keeps the upcoming AutoPlay slots.
		h.c.Clear()
		if got := h.ids(); !slices.Equal(got, []string{"b", "ra.cp-d-1", "ra.cp-d-2"}) {
			t.Fatalf("after clear %v", got)
		}
		// The user queue ends: AutoPlay continues, refilling from the same station.
		h.eng.end()
		synctest.Wait()
		if h.current() != "ra.cp-d-1" {
			t.Fatalf("current %s", h.current())
		}
		next, cont := st.calls()
		if len(cont) != 1 || next[len(next)-1] != "ra.cp-d" || len(h.ids()) != 5 {
			t.Fatalf("next %v cont %v queue %v", next, cont, h.ids())
		}
		// Turning AutoPlay off drops the upcoming AutoPlay slots.
		h.c.SetAutoPlay(false)
		s = h.c.Snapshot()
		if s.Station != nil || s.AutoPlay {
			t.Fatalf("station %+v autoplay %v", s.Station, s.AutoPlay)
		}
		if got := h.ids(); !slices.Equal(got, []string{"b", "ra.cp-d-1"}) {
			t.Fatalf("queue %v", got)
		}
	})
}

func TestAutoPlayQueueEnd(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 2)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a"), 0, ""))
		synctest.Wait()
		// Turned on while the last song plays: the continuation is fetched now.
		h.c.SetAutoPlay(true)
		synctest.Wait()
		// Refills chain until more than 2 follow the current item.
		if got := h.ids(); !slices.Equal(got, []string{"a", "ra.cp-a-1", "ra.cp-a-2", "ra.cp-a-3", "ra.cp-a-4"}) {
			t.Fatalf("queue %v", got)
		}
		h.eng.end()
		synctest.Wait()
		if h.current() != "ra.cp-a-1" {
			t.Fatalf("current %s", h.current())
		}
		if _, cont := st.calls(); len(cont) != 1 {
			t.Fatalf("continue calls %v", cont)
		}
	})
}

func TestAutoPlayRespectsRepeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 2)
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		must(t, h.c.SetRepeat(RepeatAll))
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		h.eng.end()
		synctest.Wait()
		h.eng.end()
		synctest.Wait()
		if h.current() != "a" {
			t.Fatalf("repeat all did not wrap: %s", h.current())
		}
		if _, cont := st.calls(); len(cont) != 0 {
			t.Fatalf("AutoPlay ran under repeat: %v", cont)
		}
	})
}

func TestStaleRefillDropped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 1)
		ctx := t.Context()
		must(t, h.c.PlayStation(ctx, Station{ID: "ra.4"}))
		synctest.Wait()
		// Hold the next refill, then replace the queue while it is in flight.
		gate := make(chan struct{})
		st.mu.Lock()
		st.gate = gate
		st.mu.Unlock()
		h.eng.end()
		synctest.Wait()
		must(t, h.c.SetQueue(ctx, items("a", "b", "c", "d"), 0, ""))
		close(gate)
		synctest.Wait()
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c", "d"}) {
			t.Fatalf("stale refill landed: %v", got)
		}
		if h.c.Snapshot().Station != nil {
			t.Fatal("station kept after SetQueue")
		}
	})
}

func TestStationShuffleAndRestore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &memStore{}
		st := newStations(2)
		h := newHarness(t, func(o *Options) { o.Stations = st; o.Store = store })
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		must(t, h.c.SetQueue(ctx, items("a", "b", "c"), 1, ""))
		synctest.Wait()
		// b plays with 1 left: AutoPlay slots follow c.
		h.c.SetShuffle(true)
		s := h.c.Snapshot()
		if got := autoFlags(s); !slices.Equal(got, []bool{false, false, false, true, true}) || s.Queue[0].Item.ID != "b" {
			t.Fatalf("shuffled %v auto %v", h.ids(), got)
		}
		if got := h.ids()[3:]; !slices.Equal(got, []string{"ra.cp-c-1", "ra.cp-c-2"}) {
			t.Fatalf("station slots reordered: %v", got)
		}
		h.c.SetShuffle(false)
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c", "ra.cp-c-1", "ra.cp-c-2"}) {
			t.Fatalf("unshuffled %v", got)
		}
		must(t, h.c.Close())
		saved, _ := store.LoadQueue(ctx)
		if saved == nil || saved.Station == nil || saved.Station.ID != "ra.cp-c" || !slices.Equal(saved.Auto, []int{3, 4}) {
			t.Fatalf("saved %+v", saved)
		}

		h2 := newHarness(t, func(o *Options) { o.Stations = st; o.Store = store })
		must(t, h2.c.Restore(ctx))
		s = h2.c.Snapshot()
		if s.Station == nil || s.Station.ID != "ra.cp-c" || !slices.Equal(autoFlags(s), []bool{false, false, false, true, true}) {
			t.Fatalf("restored %+v %v", s.Station, autoFlags(s))
		}
		// Restoring makes no station requests (cold start stays local).
		synctest.Wait()
		if next, _ := st.calls(); len(next) != 1 {
			t.Fatalf("restore fetched: %v", next)
		}
	})
}

// memStore keeps the saved queue in memory.
type memStore struct {
	mu sync.Mutex
	s  *Saved
}

func (m *memStore) LoadQueue(context.Context) (*Saved, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s == nil {
		return nil, nil
	}
	c := *m.s
	return &c, nil
}

func (m *memStore) SaveQueue(_ context.Context, s Saved) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s = &s
	return nil
}
