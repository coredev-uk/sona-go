package coordinator

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

// autoPlayDelay is Options.AutoPlayDelay's default.
const autoPlayDelay = 2 * time.Second

func continues(st *fakeStations) [][]string {
	_, cont := st.calls()
	return cont
}

func TestAutoPlayFetchedAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 2)
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		h.c.SetOnline(true)
		album := Container{Kind: ContainerAlbum, ID: "1"}
		must(t, h.c.SetQueue(ctx, from(album, items("a", "b", "c", "d", "e")), 0, "album:1"))
		synctest.Wait()
		if cont := continues(st); len(cont) != 0 {
			t.Fatalf("fetched before the queue settled: %v", cont)
		}
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		// Four user songs still to go: AutoPlay is there already, after them.
		cont := continues(st)
		if len(cont) != 1 || !slices.Equal(cont[0], []string{"e"}) || st.contFrom != album {
			t.Fatalf("continue calls %v from %+v", cont, st.contFrom)
		}
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c", "d", "e", "ra.cp-e-1", "ra.cp-e-2"}) {
			t.Fatalf("queue %v", got)
		}
		if got := autoFlags(h.c.Snapshot()); !slices.Equal(got, []bool{false, false, false, false, false, true, true}) {
			t.Fatalf("auto %v", got)
		}
		if s := h.c.Snapshot(); s.Station == nil || s.Station.ID != "ra.cp-e" || !s.Station.AutoPlay {
			t.Fatalf("station %+v", s.Station)
		}
		// The user's songs play first, with no more requests while more
		// than two songs follow.
		for range 3 {
			h.eng.end()
			synctest.Wait()
		}
		if next, cont := st.calls(); len(next) != 1 || len(cont) != 1 || h.current() != "d" {
			t.Fatalf("current %s, next-tracks %v, continue %v", h.current(), next, cont)
		}
		h.eng.end() // e: two AutoPlay songs left, the station tops them up
		synctest.Wait()
		h.eng.end()
		synctest.Wait()
		if h.current() != "ra.cp-e-1" {
			t.Fatalf("current %s queue %v", h.current(), h.ids())
		}
		if next, cont := st.calls(); !slices.Equal(next, []string{"ra.cp-e", "ra.cp-e"}) || len(cont) != 1 {
			t.Fatalf("next-tracks %v continue %v", next, cont)
		}
	})
}

func TestAutoPlayAheadFollowsUserEdits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 2)
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		h.c.SetOnline(true)
		albumA := Container{Kind: ContainerAlbum, ID: "A"}
		albumB := Container{Kind: ContainerAlbum, ID: "B"}
		listC := Container{Kind: ContainerPlaylist, ID: "pl.C"}
		must(t, h.c.SetQueue(ctx, from(albumA, items("a1", "a2", "a3")), 0, ""))
		time.Sleep(autoPlayDelay)
		synctest.Wait()

		// Edits that keep the user's last song in the same album keep the
		// station: removing its last song.
		must(t, h.c.Remove(ctx, 2))
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if got := h.ids(); len(continues(st)) != 1 || !slices.Equal(got, []string{"a1", "a2", "ra.cp-a3-1", "ra.cp-a3-2"}) {
			t.Fatalf("continue %v queue %v", continues(st), got)
		}

		// User songs go before the AutoPlay slots. A burst of edits that
		// change the last song's container makes one new station, which
		// replaces the old AutoPlay slots.
		h.c.PlayNext(from(listC, items("n")))
		h.c.Append(from(albumB, items("b1")))
		time.Sleep(autoPlayDelay / 2)
		h.c.Append(from(listC, items("c1")))
		time.Sleep(autoPlayDelay / 2)
		synctest.Wait()
		if got := h.ids(); len(continues(st)) != 1 || !slices.Equal(got, []string{"a1", "n", "a2", "b1", "c1", "ra.cp-a3-1", "ra.cp-a3-2"}) {
			t.Fatalf("before the delay: continue %v queue %v", continues(st), got)
		}
		time.Sleep(autoPlayDelay / 2)
		synctest.Wait()
		cont := continues(st)
		if len(cont) != 2 || !slices.Equal(cont[1], []string{"c1"}) || st.contFrom != listC {
			t.Fatalf("continue %v from %+v", cont, st.contFrom)
		}
		if got := h.ids(); !slices.Equal(got, []string{"a1", "n", "a2", "b1", "c1", "ra.cp-c1-1", "ra.cp-c1-2"}) {
			t.Fatalf("queue %v", got)
		}

		// AutoPlay slots keep their place, and user songs stay before them.
		if err := h.c.Move(5, 1); err == nil {
			t.Fatal("moved an AutoPlay slot")
		}
		must(t, h.c.Move(1, 6)) // n (from the same playlist as c1) to the end of the user's songs
		if got := h.ids(); !slices.Equal(got, []string{"a1", "a2", "b1", "c1", "n", "ra.cp-c1-1", "ra.cp-c1-2"}) {
			t.Fatalf("after move %v", got)
		}

		// Shuffling seeds from the unshuffled order: no new station.
		h.c.SetShuffle(true)
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if got := autoFlags(h.c.Snapshot()); len(continues(st)) != 2 || !slices.Equal(got[len(got)-2:], []bool{true, true}) || slices.Contains(got[:len(got)-2], true) {
			t.Fatalf("shuffled: continue %v auto %v", continues(st), got)
		}
		h.c.SetShuffle(false)

		// Playing an AutoPlay song jumps there; from then on the station
		// stays, whatever the user queues.
		must(t, h.c.Jump(ctx, 5))
		h.c.Append(from(albumB, items("b2")))
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if h.current() != "ra.cp-c1-1" || len(continues(st)) != 2 {
			t.Fatalf("current %s continue %v", h.current(), continues(st))
		}
		if got := h.ids(); !slices.Equal(got[5:8], []string{"ra.cp-c1-1", "b2", "ra.cp-c1-2"}) {
			t.Fatalf("queue %v", got)
		}
	})
}

func TestAutoPlayAheadToggles(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 2)
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		h.c.SetOnline(true)
		must(t, h.c.SetQueue(ctx, items("a", "b", "c"), 0, ""))
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c", "ra.cp-c-1", "ra.cp-c-2"}) {
			t.Fatalf("queue %v", got)
		}

		// AutoPlay runs only with repeat off: repeat removes its slots and
		// wraps the user's queue again.
		must(t, h.c.SetRepeat(RepeatAll))
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c"}) || h.c.Snapshot().Station != nil {
			t.Fatalf("repeat all: queue %v station %+v", got, h.c.Snapshot().Station)
		}
		must(t, h.c.Jump(ctx, 2))
		h.eng.end()
		synctest.Wait()
		if h.current() != "a" {
			t.Fatalf("repeat all did not wrap: %s", h.current())
		}
		must(t, h.c.SetRepeat(RepeatOff))
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if got := h.ids(); len(continues(st)) != 2 || !slices.Equal(got, []string{"a", "b", "c", "ra.cp-c-3", "ra.cp-c-4"}) {
			t.Fatalf("repeat off: continue %v queue %v", continues(st), got)
		}

		// Turning AutoPlay off removes its slots.
		h.c.SetAutoPlay(false)
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c"}) {
			t.Fatalf("off: %v", got)
		}
		// A fetch in flight when AutoPlay goes off is dropped.
		gate := make(chan struct{})
		st.mu.Lock()
		st.gate = gate
		st.mu.Unlock()
		h.c.SetAutoPlay(true)
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		h.c.SetAutoPlay(false)
		close(gate)
		synctest.Wait()
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c"}) || h.c.Snapshot().Station != nil {
			t.Fatalf("dropped fetch landed: %v %+v", got, h.c.Snapshot().Station)
		}
	})
}

func TestAutoPlayAheadOffline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, st := stationHarness(t, 2)
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		album := Container{Kind: ContainerAlbum, ID: "1"}
		must(t, h.c.SetQueue(ctx, from(album, items("a", "b", "c", "d")), 0, ""))
		time.Sleep(10 * autoPlayDelay)
		synctest.Wait()
		if cont := continues(st); len(cont) != 0 {
			t.Fatalf("fetched ahead offline: %v", cont)
		}
		// Going offline again before the delay ends cancels the fetch.
		h.c.SetOnline(true)
		time.Sleep(autoPlayDelay / 2)
		h.c.SetOnline(false)
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if cont := continues(st); len(cont) != 0 {
			t.Fatalf("fetched after going offline: %v", cont)
		}
		h.c.SetOnline(true)
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if got := h.ids(); len(continues(st)) != 1 || len(got) != 6 {
			t.Fatalf("online: continue %v queue %v", continues(st), got)
		}

		// A failed fetch is not retried on every edit, only after
		// RefillRetry.
		st.mu.Lock()
		st.fail = 1
		st.mu.Unlock()
		other := Container{Kind: ContainerAlbum, ID: "2"}
		must(t, h.c.SetQueue(ctx, from(other, items("x", "y", "z", "w")), 0, ""))
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		h.c.Append(from(album, items("e")))
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if next, cont := st.calls(); len(cont) != 2 || len(next) != 2 {
			t.Fatalf("retried at once: next %v continue %v", next, cont)
		}
		time.Sleep(30 * time.Second)
		h.c.Append(from(other, items("v")))
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if got := h.ids(); len(continues(st)) != 3 || !slices.Equal(got[len(got)-2:], []string{"ra.cp-v-1", "ra.cp-v-2"}) {
			t.Fatalf("after the retry pause: continue %v queue %v", continues(st), got)
		}
	})
}

func TestAutoPlayAheadAfterRestore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &memStore{}
		st := newStations(2)
		build := func() *harness {
			h := newHarness(t, func(o *Options) { o.Stations = st; o.Store = store })
			h.c.SetAutoPlay(true)
			return h
		}
		ctx := t.Context()
		h := build()
		must(t, h.c.SetQueue(ctx, items("a", "b", "c", "d"), 0, ""))
		must(t, h.c.Close())

		// Cold start stays local; once online, AutoPlay is fetched for the
		// restored queue.
		h = build()
		must(t, h.c.Restore(ctx))
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if cont := continues(st); len(cont) != 0 {
			t.Fatalf("restore fetched: %v", cont)
		}
		h.c.SetOnline(true)
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if got := h.ids(); len(continues(st)) != 1 || !slices.Equal(got, []string{"a", "b", "c", "d", "ra.cp-d-1", "ra.cp-d-2"}) {
			t.Fatalf("continue %v queue %v", continues(st), got)
		}
		must(t, h.c.Close())

		// A restored AutoPlay station still follows the queue: no request.
		h = build()
		must(t, h.c.Restore(ctx))
		h.c.SetOnline(true)
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if next, cont := st.calls(); len(next) != 1 || len(cont) != 1 || len(h.ids()) != 6 {
			t.Fatalf("next %v continue %v queue %v", next, cont, h.ids())
		}
	})
}
