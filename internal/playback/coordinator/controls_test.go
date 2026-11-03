package coordinator

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

// liveItem is a live radio stream: a station ID with no duration.
func liveItem(id string) Item { return Item{ID: id, Title: "Live " + id} }

func TestControlsPerItemKind(t *testing.T) {
	on := Controls{Previous: true, Next: true, Seek: true, Shuffle: true, Repeat: true, AutoPlay: true, Queue: true}
	for _, tc := range []struct {
		name  string
		queue []Item
		start int
		setup func(*Coordinator)
		want  Controls
	}{
		{"nothing playing", nil, 0, nil, Controls{Shuffle: true, Repeat: true, AutoPlay: true, Queue: true}},
		{"a song in the middle", items("a", "b", "c"), 1, nil, on},
		{"the last song", items("a", "b"), 1, nil, Controls{Previous: true, Seek: true, Shuffle: true, Repeat: true, AutoPlay: true, Queue: true}},
		{"the last song with repeat all", items("a", "b"), 1, func(c *Coordinator) { _ = c.SetRepeat(RepeatAll) }, Controls{Previous: true, Next: true, Seek: true, Shuffle: true, Repeat: true, AutoPlay: true, Queue: true}},
		{"the last song with AutoPlay", items("a", "b"), 1, func(c *Coordinator) { c.SetAutoPlay(true) }, on},
		// An episode ("ra.…" with a length) is a recording.
		{"a station episode", []Item{{ID: "ra.ep", Title: "Episode", Duration: 30 * time.Minute}}, 0, nil, Controls{Previous: true, Seek: true, Shuffle: true, Repeat: true, AutoPlay: true, Queue: true}},
		{"a live station", []Item{liveItem("ra.live")}, 0, nil, Controls{}},
		{"a live station, repeat and shuffle on", []Item{liveItem("ra.live")}, 0, func(c *Coordinator) { c.SetShuffle(true); _ = c.SetRepeat(RepeatAll) }, Controls{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness(t, nil)
				if tc.setup != nil {
					tc.setup(h.c)
				}
				if tc.queue != nil {
					must(t, h.c.SetQueue(t.Context(), tc.queue, tc.start, ""))
					synctest.Wait()
				}
				if got := h.c.Snapshot().Controls; got != tc.want {
					t.Errorf("controls = %+v, want %+v", got, tc.want)
				}
			})
		})
	}
}

// A station of songs is endless: Next and the seek bar stay (they are real
// tracks), Previous only restarts the song, and the modes and queueing
// don't apply. AutoPlay's station is the user's queue and changes nothing.
func TestControlsTrackStation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _ := stationHarness(t, 3)
		must(t, h.c.PlayStation(t.Context(), Station{ID: "ra.1", Name: "One"}))
		synctest.Wait()
		want := Controls{Previous: true, Next: true, Seek: true}
		if c := h.c.Snapshot().Controls; c != want {
			t.Errorf("station of songs: %+v, want %+v", c, want)
		}
		s := h.c.Snapshot()
		if err := s.Refusal(func(c Controls) bool { return c.Shuffle }); !errors.Is(err, ErrStation) {
			t.Errorf("shuffle refusal = %v", err)
		}
		if err := s.Refusal(func(c Controls) bool { return c.Next }); err != nil {
			t.Errorf("next refusal = %v", err)
		}
	})
}

// Previous on a station does nothing in the song's first moments and
// restarts the song after them; it never goes back to an earlier song.
func TestStationPreviousOnlyRestarts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _ := stationHarness(t, 3)
		ctx := t.Context()
		must(t, h.c.PlayStation(ctx, Station{ID: "ra.1", Name: "One"}))
		synctest.Wait()
		must(t, h.c.Next(ctx))
		synctest.Wait()
		second := h.current()
		before := h.eng.callList()
		must(t, h.c.Previous(ctx))
		if after := h.eng.callList(); len(after) != len(before) {
			t.Errorf("Previous in the first seconds reached the engine: %v", after[len(before):])
		}
		time.Sleep(10 * time.Second)
		must(t, h.c.Previous(ctx))
		calls := h.eng.callList()
		if h.current() != second || calls[len(calls)-1] != "seek 0s" {
			t.Errorf("Previous later: current %q (was %q), last call %q", h.current(), second, calls[len(calls)-1])
		}
	})
}

// The controls follow the current item when the station is replaced by songs and back.
func TestControlsFollowTheCurrentItem(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		synctest.Wait()
		must(t, h.c.SetQueue(ctx, []Item{liveItem("ra.live")}, 0, "ra.live"))
		synctest.Wait()
		if c := h.c.Snapshot().Controls; c != (Controls{}) {
			t.Errorf("on the station: %+v", c)
		}
		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		synctest.Wait()
		if c := h.c.Snapshot().Controls; !c.Next || !c.Seek {
			t.Errorf("back on songs: %+v", c)
		}
	})
}

// Skipping, rewinding and seeking a live station are refused and change
// nothing: the engine hears nothing of them.
func TestLiveStationRefusesTransport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, []Item{liveItem("ra.live")}, 0, "ra.live"))
		synctest.Wait()
		before := h.eng.callList()
		for name, err := range map[string]error{
			"Next":     h.c.Next(ctx),
			"Previous": h.c.Previous(ctx),
			"Seek":     h.c.Seek(ctx, time.Minute),
		} {
			if !errors.Is(err, ErrLiveStation) {
				t.Errorf("%s = %v, want ErrLiveStation", name, err)
			}
		}
		if after := h.eng.callList(); len(after) != len(before) {
			t.Errorf("engine calls after refused controls: %v, before %v", after, before)
		}
		if h.current() != "ra.live" || h.c.Snapshot().State == "ended" {
			t.Errorf("a refused Next left %q (%s)", h.current(), h.c.Snapshot().State)
		}
		// Pause and resume are what a live station has.
		must(t, h.c.Pause(ctx))
		must(t, h.c.Resume(ctx))
	})
}
