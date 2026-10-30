package scrobble

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

var t0 = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func song(id string, d time.Duration) coordinator.Item {
	return coordinator.Item{ID: id, Title: "Title " + id, Artist: "Artist", Album: "Album", Duration: d}
}

// step is one activity; at is seconds after t0, positions in seconds.
type step struct {
	kind     coordinator.ActivityKind
	item     coordinator.Item
	at       int
	pos      int
	from     int
	reason   coordinator.EndReason
	wantNow  bool
	wantDone bool
}

func start(it coordinator.Item, at, pos int) step {
	return step{kind: coordinator.ActivityStart, item: it, at: at, pos: pos, wantNow: true}
}

func seek(it coordinator.Item, at, from, to int) step {
	return step{kind: coordinator.ActivitySeek, item: it, at: at, from: from, pos: to}
}

func end(it coordinator.Item, at, pos int, r coordinator.EndReason, counts bool) step {
	return step{kind: coordinator.ActivityEnd, item: it, at: at, pos: pos, reason: r, wantDone: counts}
}

func sec(n int) time.Duration { return time.Duration(n) * time.Second }

func TestTrackerRules(t *testing.T) {
	short := song("1", sec(30))
	threeMin := song("2", sec(180))
	long := song("3", sec(600))
	two := song("4", sec(200))
	radio := coordinator.Item{ID: "ra.978194965", Title: "Apple Music 1", Artist: "Apple", Duration: 0}
	noArtist := coordinator.Item{ID: "5", Title: "Untitled", Duration: sec(200)}

	nat, skip, paused := coordinator.EndNatural, coordinator.EndSkipForward, coordinator.EndPaused
	for name, steps := range map[string][]step{
		"played to the end":           {start(threeMin, 0, 0), end(threeMin, 180, 180, nat, true)},
		"30 s track never counts":     {start(short, 0, 0), end(short, 30, 30, nat, false)},
		"just under half":             {start(threeMin, 0, 0), end(threeMin, 89, 89, skip, false)},
		"half counts":                 {start(threeMin, 0, 0), end(threeMin, 90, 90, skip, true)},
		"four minutes of a long one":  {start(long, 0, 0), end(long, 240, 240, skip, true)},
		"under four minutes, long":    {start(long, 0, 0), end(long, 239, 239, skip, false)},
		"seek forward skips the part": {start(threeMin, 0, 0), seek(threeMin, 10, 10, 170), end(threeMin, 20, 180, nat, false)},
		"seek back replays": {
			start(two, 0, 0), seek(two, 90, 90, 0), end(two, 110, 20, skip, true),
		},
		"resumed from a restored position": {start(two, 0, 120), end(two, 80, 200, nat, false)},
		"pause and resume continue the listen": {
			start(two, 0, 0), end(two, 60, 60, paused, false),
			start(two, 600, 60), end(two, 650, 110, skip, true),
		},
		"counted at the pause, not again": {
			start(two, 0, 0), end(two, 120, 120, paused, true),
			start(two, 300, 120), end(two, 380, 200, nat, false),
		},
		"repeat one is a new listen": {
			start(threeMin, 0, 0), end(threeMin, 180, 180, nat, true),
			start(threeMin, 180, 0), end(threeMin, 360, 180, nat, true),
		},
		"resume of another item starts afresh": {
			start(two, 0, 0), end(two, 60, 60, paused, false),
			start(threeMin, 100, 0), end(threeMin, 160, 60, skip, false),
		},
		"live radio is ignored": {
			{kind: coordinator.ActivityStart, item: radio}, {kind: coordinator.ActivityEnd, item: radio, at: 3600, pos: 3600, reason: coordinator.EndStopped},
		},
		"no artist is ignored": {
			{kind: coordinator.ActivityStart, item: noArtist}, {kind: coordinator.ActivityEnd, item: noArtist, at: 200, pos: 200, reason: nat},
		},
		"end of another item is ignored": {
			start(two, 0, 0), end(threeMin, 150, 150, nat, false), end(two, 160, 160, nat, true),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var tr Tracker
			var startedAt time.Time
			for i, s := range steps {
				a := coordinator.Activity{
					Kind: s.kind, Item: s.item, At: t0.Add(sec(s.at)), Position: sec(s.pos), From: sec(s.from),
					Duration: s.item.Duration, Reason: s.reason,
				}
				if s.kind == coordinator.ActivityStart && startedAt.IsZero() {
					startedAt = a.At
				}
				now, done := tr.Observe(a)
				if (now != nil) != s.wantNow {
					t.Fatalf("step %d: now playing = %v, want %v", i, now, s.wantNow)
				}
				if now != nil && (now.Artist != "Artist" || now.Title != s.item.Title || now.Duration != s.item.Duration) {
					t.Fatalf("step %d: now playing %+v", i, now)
				}
				if (done != nil) != s.wantDone {
					t.Fatalf("step %d: listen = %+v, want %v", i, done, s.wantDone)
				}
				if done != nil && (done.Title != s.item.Title || done.Album != "Album") {
					t.Fatalf("step %d: listen %+v", i, done)
				}
				if done != nil && name == "pause and resume continue the listen" && !done.ListenedAt.Equal(startedAt) {
					t.Fatalf("listen stamped %v, want the first start %v", done.ListenedAt, startedAt)
				}
			}
		})
	}
}
