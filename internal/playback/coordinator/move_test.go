package coordinator

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func historyIDs(s Snapshot) []string {
	var out []string
	for _, it := range s.History {
		out = append(out, it.ID)
	}
	return out
}

// Play next and Move to end put a song first or last among the user's
// upcoming songs; the playing song, the history and the song that
// plays next follow.
func TestMoveNextAndToEnd(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("s60", "b", "c", "d", "e"), 0, ""))
		synctest.Wait()
		h.eng.end() // s60 played: b is playing and s60 is history
		synctest.Wait()
		before := h.c.Snapshot()
		if h.current() != "b" || !slices.Equal(historyIDs(before), []string{"s60"}) {
			t.Fatalf("setup: current %s history %v", h.current(), historyIDs(before))
		}

		must(t, h.c.MoveNext(4)) // e after the playing b
		s := h.c.Snapshot()
		if got := h.ids(); !slices.Equal(got, []string{"s60", "b", "e", "c", "d"}) || s.Index != 1 || h.current() != "b" {
			t.Fatalf("play next: %v index %d current %s", got, s.Index, h.current())
		}
		if s.QueueVersion == before.QueueVersion {
			t.Fatal("queue version unchanged by a move")
		}
		if !slices.Equal(historyIDs(s), []string{"s60"}) {
			t.Fatalf("history changed: %v", historyIDs(s))
		}

		must(t, h.c.MoveToEnd(2)) // e to the back
		if got := h.ids(); !slices.Equal(got, []string{"s60", "b", "c", "d", "e"}) {
			t.Fatalf("move to end: %v", got)
		}

		// A song that is already first or last, or the playing one, stays
		// put and is not a queue change.
		v := h.c.Snapshot().QueueVersion
		must(t, h.c.MoveNext(2))
		must(t, h.c.MoveToEnd(4))
		must(t, h.c.MoveNext(1))
		must(t, h.c.MoveToEnd(1))
		if s := h.c.Snapshot(); s.QueueVersion != v || !slices.Equal(h.ids(), []string{"s60", "b", "c", "d", "e"}) {
			t.Fatalf("no-op moves changed the queue: %v version %d→%d", h.ids(), v, s.QueueVersion)
		}

		// An already played song can be queued again, right after the
		// playing one.
		must(t, h.c.MoveNext(0))
		if s := h.c.Snapshot(); !slices.Equal(h.ids(), []string{"b", "s60", "c", "d", "e"}) || s.Index != 0 || h.current() != "b" {
			t.Fatalf("played song: %v index %d current %s", h.ids(), s.Index, h.current())
		}

		for _, from := range []int{-1, 5} {
			if h.c.MoveNext(from) == nil || h.c.MoveToEnd(from) == nil {
				t.Fatalf("moved queue[%d] of 5", from)
			}
		}
	})
}

// The song after the playing one is the one the engine gets ready (and
// Next plays), as soon as a move changes it.
func TestMoveRetargetsTheNextSong(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("s60", "b", "c", "d"), 0, ""))
		synctest.Wait()
		time.Sleep(46 * time.Second) // within the lead of s60's end
		synctest.Wait()
		if p := h.eng.preloadList(); !slices.Equal(p, []string{"b"}) {
			t.Fatalf("preloads %v, want [b]", p)
		}
		must(t, h.c.MoveNext(3))
		synctest.Wait()
		if p := h.eng.preloadList(); !slices.Equal(p, []string{"b", "d"}) {
			t.Fatalf("preloads after play next %v, want [b d]", p)
		}
		must(t, h.c.MoveToEnd(1)) // d, which is next, goes last: b is next again
		synctest.Wait()
		if p := h.eng.preloadList(); !slices.Equal(p, []string{"b", "d", "b"}) {
			t.Fatalf("preloads after move to end %v, want [b d b]", p)
		}
		must(t, h.c.MoveNext(3))
		must(t, h.c.Next(ctx))
		if h.current() != "d" {
			t.Fatalf("Next played %s, want the moved d", h.current())
		}
	})
}

// AutoPlay's songs keep their place after the user's: Move to end goes
// before them, and they cannot be moved.
func TestMoveStaysBeforeAutoPlay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _ := stationHarness(t, 2)
		ctx := t.Context()
		h.c.SetAutoPlay(true)
		h.c.SetOnline(true)
		album := Container{Kind: ContainerAlbum, ID: "1"}
		must(t, h.c.SetQueue(ctx, from(album, items("a", "b", "c", "d")), 0, "album:1"))
		time.Sleep(autoPlayDelay)
		synctest.Wait()
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c", "d", "ra.cp-d-1", "ra.cp-d-2"}) {
			t.Fatalf("setup: %v", got)
		}

		must(t, h.c.MoveToEnd(1))
		if got := h.ids(); !slices.Equal(got, []string{"a", "c", "d", "b", "ra.cp-d-1", "ra.cp-d-2"}) {
			t.Fatalf("move to end: %v", got)
		}
		if got := autoFlags(h.c.Snapshot()); !slices.Equal(got, []bool{false, false, false, false, true, true}) {
			t.Fatalf("auto flags %v", got)
		}
		must(t, h.c.MoveNext(3))
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c", "d", "ra.cp-d-1", "ra.cp-d-2"}) {
			t.Fatalf("play next: %v", got)
		}
		for _, from := range []int{4, 5} {
			if h.c.MoveNext(from) == nil || h.c.MoveToEnd(from) == nil {
				t.Fatalf("moved AutoPlay slot %d", from)
			}
		}
		if got := h.ids(); !slices.Equal(got, []string{"a", "b", "c", "d", "ra.cp-d-1", "ra.cp-d-2"}) {
			t.Fatalf("after refused moves: %v", got)
		}
	})
}
