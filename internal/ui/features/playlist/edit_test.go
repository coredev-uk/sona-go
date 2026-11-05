package playlist

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/coredev-uk/sona-go/internal/services"
)

func editDetail(ids ...string) services.PlaylistDetail {
	d := services.PlaylistDetail{Playlist: services.Playlist{ID: "p.x", TrackCount: len(ids)}, DurationMs: int64(len(ids)) * 1000}
	for _, id := range ids {
		d.Tracks = append(d.Tracks, services.Song{ID: id, DurationMs: 1000})
	}
	return d
}

func trackIDs(d services.PlaylistDetail) []string {
	var out []string
	for _, t := range d.Tracks {
		out = append(out, t.ID)
	}
	return out
}

func TestWithMove(t *testing.T) {
	for _, c := range []struct {
		name     string
		from, to int
		want     []string
	}{
		{"forward", 0, 2, []string{"b", "c", "a", "d"}},
		{"backward", 3, 0, []string{"d", "a", "b", "c"}},
		{"same", 1, 1, []string{"a", "b", "c", "d"}},
		{"to out of range", 0, 5, []string{"a", "b", "c", "d"}},
		{"from negative", -1, 2, []string{"a", "b", "c", "d"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := editDetail("a", "b", "c", "d")
			if got := trackIDs(withMove(d, c.from, c.to)); !slices.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
			if got := trackIDs(d); !slices.Equal(got, []string{"a", "b", "c", "d"}) {
				t.Errorf("the input changed: %v", got)
			}
		})
	}
}

func TestWithRemoved(t *testing.T) {
	d := withRemoved(editDetail("a", "b", "a"), []int{2})
	if got := trackIDs(d); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("tracks %v (one copy of a duplicate goes)", got)
	}
	if d.DurationMs != 2000 || d.Playlist.TrackCount != 2 {
		t.Errorf("duration %d, count %d", d.DurationMs, d.Playlist.TrackCount)
	}
	all := withRemoved(editDetail("a", "b", "c"), []int{0, 1, 2})
	if len(all.Tracks) != 0 || all.DurationMs != 0 || all.Playlist.TrackCount != 0 {
		t.Errorf("everything removed: %+v", all)
	}
}

type serialOutcome struct {
	name  string
	err   error
	later bool
}

func TestSerialRunsEditsInOrder(t *testing.T) {
	synctest.Test(t, serialRunsEditsInOrder)
}

func serialRunsEditsInOrder(t *testing.T) {
	var e edits
	var mu sync.Mutex
	var order []string
	gate := make(chan struct{})
	outcomes := make(chan serialOutcome, 2)
	step := func(name string, wait <-chan struct{}) func() (services.PlaylistDetail, error) {
		return func() (services.PlaylistDetail, error) {
			if wait != nil {
				<-wait
			}
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return services.PlaylistDetail{}, nil
		}
	}
	done := func(name string) func(services.PlaylistDetail, error, bool) {
		return func(_ services.PlaylistDetail, err error, later bool) { outcomes <- serialOutcome{name, err, later} }
	}
	e.serial("p.s", step("first", gate), done("first"))
	e.serial("p.s", step("second", nil), done("second"))
	if !e.editing("p.s") {
		t.Fatal("not editing with two queued")
	}
	synctest.Wait() // the first is parked on the gate; a second that overtook it has run by now
	mu.Lock()
	if len(order) != 0 {
		t.Errorf("ran %v before the first was released", order)
	}
	mu.Unlock()
	close(gate)
	first := <-outcomes
	if first.name != "first" || !first.later {
		t.Errorf("first outcome %+v: want later=true (the second is queued)", first)
	}
	second := <-outcomes
	if second.name != "second" || second.later {
		t.Errorf("second outcome %+v: want later=false", second)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(order, []string{"first", "second"}) {
		t.Errorf("order %v", order)
	}
	if e.editing("p.s") {
		t.Error("still editing after both finished")
	}
}

// TestSerialHandsOverEachOutcomeBeforeTheNextEditStarts holds the first
// edit's outcome in done: the second edit must not start, so its outcome
// cannot reach the main thread before the first's (the page would end on the
// first answer, a state Go has left). Held deterministically, not by a race.
func TestSerialHandsOverEachOutcomeBeforeTheNextEditStarts(t *testing.T) {
	for _, c := range []struct {
		name   string
		queued bool // the second edit is made before the first answers
	}{
		{"queued behind the first", true},
		{"made after the first answered", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) { serialHandsOverInOrder(t, c.queued) })
		})
	}
}

func serialHandsOverInOrder(t *testing.T, queued bool) {
	var e edits
	var mu sync.Mutex
	var ran []string
	gate, hold := make(chan struct{}), make(chan struct{})
	outcomes := make(chan string, 2)
	step := func(name string, wait <-chan struct{}) func() (services.PlaylistDetail, error) {
		return func() (services.PlaylistDetail, error) {
			if wait != nil {
				<-wait
			}
			mu.Lock()
			ran = append(ran, name)
			mu.Unlock()
			return services.PlaylistDetail{}, nil
		}
	}
	e.serial("p.h", step("first", gate), func(services.PlaylistDetail, error, bool) {
		<-hold // the main thread has not been handed the first outcome yet
		outcomes <- "first"
	})
	second := func() {
		e.serial("p.h", step("second", nil), func(services.PlaylistDetail, error, bool) { outcomes <- "second" })
	}
	if queued {
		second()
	}
	close(gate)
	synctest.Wait() // the first answered and its done is held
	if !queued {
		if e.editing("p.h") {
			t.Error("the count did not drop before done ran")
		}
		second()
		synctest.Wait()
	}
	mu.Lock()
	if !slices.Equal(ran, []string{"first"}) {
		t.Errorf("ran %v while the first outcome was still being handed over", ran)
	}
	mu.Unlock()
	close(hold)
	for _, want := range []string{"first", "second"} {
		if got := <-outcomes; got != want {
			t.Errorf("outcome %q, want %q", got, want)
		}
	}
}

func TestSerialKeepsGoingAfterAFailedEdit(t *testing.T) {
	var e edits
	boom := errors.New("boom")
	out := make(chan serialOutcome, 2)
	e.serial("p.f", func() (services.PlaylistDetail, error) { return services.PlaylistDetail{}, boom },
		func(_ services.PlaylistDetail, err error, later bool) { out <- serialOutcome{"fail", err, later} })
	if got := <-out; !errors.Is(got.err, boom) || got.later {
		t.Errorf("failed edit outcome %+v", got)
	}
	e.serial("p.f", func() (services.PlaylistDetail, error) { return editDetail("a"), nil },
		func(d services.PlaylistDetail, err error, later bool) {
			if len(d.Tracks) != 1 {
				err = errors.New("detail lost")
			}
			out <- serialOutcome{"ok", err, later}
		})
	if got := <-out; got.err != nil || got.later {
		t.Errorf("edit after the failure %+v", got)
	}
	if e.editing("p.f") {
		t.Error("still editing")
	}
}

func TestSerialQueuesPerPlaylist(t *testing.T) {
	var e edits
	gate := make(chan struct{})
	out := make(chan string, 2)
	e.serial("p.one", func() (services.PlaylistDetail, error) { <-gate; return services.PlaylistDetail{}, nil },
		func(services.PlaylistDetail, error, bool) { out <- "one" })
	e.serial("p.two", func() (services.PlaylistDetail, error) { return services.PlaylistDetail{}, nil },
		func(services.PlaylistDetail, error, bool) { out <- "two" })
	if got := <-out; got != "two" {
		t.Errorf("%q finished first: another playlist's edit waited", got)
	}
	if !e.editing("p.one") || e.editing("p.two") {
		t.Errorf("editing one=%v two=%v", e.editing("p.one"), e.editing("p.two"))
	}
	close(gate)
	<-out
}
