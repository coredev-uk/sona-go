package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is the Client's clock, moved by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestClient() (*Client, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	cl := NewClient(nil)
	cl.now = clk.now
	return cl, clk
}

// eventually polls cond: fetches run on goroutines.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// counter is a fetch that counts its runs and answers with the count.
type counter struct{ n atomic.Int32 }

func (c *counter) fetch(context.Context) (int, error) { return int(c.n.Add(1)), nil }

func TestUseFetchesOnceAndCaches(t *testing.T) {
	cl, _ := newTestClient()
	var c counter
	key := Key{"library", "albums"}
	r := Use(cl, key, c.fetch, Options{Stale: Forever})
	if r.HasData {
		t.Error("data before the fetch returned")
	}
	eventually(t, "the first fetch", func() bool { return Use(cl, key, c.fetch, Options{Stale: Forever}).HasData })
	for range 20 { // frames
		r = Use(cl, key, c.fetch, Options{Stale: Forever})
	}
	if !r.HasData || r.Data != 1 || r.Loading || r.Fetching || r.Err != nil {
		t.Errorf("result %+v", r)
	}
	if n := c.n.Load(); n != 1 {
		t.Errorf("fetched %d times, want once", n)
	}
}

func TestUseReportsLoadingThenData(t *testing.T) {
	cl, _ := newTestClient()
	release := make(chan struct{})
	fetch := func(context.Context) (string, error) { <-release; return "x", nil }
	r := Use(cl, Key{"a"}, fetch)
	if !r.Loading || !r.Fetching || r.HasData {
		t.Errorf("while fetching: %+v", r)
	}
	close(release)
	eventually(t, "the data", func() bool { return Use(cl, Key{"a"}, fetch).HasData })
	if r := Use(cl, Key{"a"}, fetch); r.Data != "x" || r.Loading {
		t.Errorf("after: %+v", r)
	}
}

func TestUseDedupesFetchesInFlight(t *testing.T) {
	cl, _ := newTestClient()
	var runs atomic.Int32
	release := make(chan struct{})
	fetch := func(context.Context) (int, error) { runs.Add(1); <-release; return 1, nil }
	for range 50 {
		Use(cl, Key{"slow"}, fetch)
	}
	close(release)
	eventually(t, "the data", func() bool { return Use(cl, Key{"slow"}, fetch).HasData })
	if n := runs.Load(); n != 1 {
		t.Errorf("%d fetches for one key", n)
	}
}

func TestStaleDataRefetchesWhenAViewMounts(t *testing.T) {
	cl, clk := newTestClient()
	var c counter
	o := Options{Stale: 10 * time.Second}
	key := Key{"q"}
	use := func() Result[int] { return Use(cl, key, c.fetch, o) }
	use()
	eventually(t, "the first result", func() bool { return use().HasData })

	// Frames of the same mount, even long after the data went stale, don't fetch.
	for range 5 {
		clk.advance(200 * time.Millisecond)
		use()
	}
	clk.advance(30 * time.Second) // stale now, but the page stays: frames follow each other
	if c.n.Load() != 1 {
		t.Fatalf("fetched %d times", c.n.Load())
	}
	// A view built again after a gap is a new mount; the data is over 30 s
	// old, so it refetches.
	clk.advance(5 * time.Second)
	r := use()
	if !r.HasData || r.Data != 1 || !r.Fetching || r.Loading {
		t.Errorf("stale data must show while it refetches: %+v", r)
	}
	eventually(t, "the refetch", func() bool { return use().Data == 2 })

	// A mount within Stale keeps the data.
	clk.advance(5 * time.Second)
	use()
	time.Sleep(10 * time.Millisecond)
	if c.n.Load() != 2 {
		t.Errorf("fresh data refetched: %d", c.n.Load())
	}

	// Forever never refetches on a mount; zero always does.
	var f counter
	for range 2 {
		Use(cl, Key{"forever"}, f.fetch, Options{Stale: Forever, Keep: 3 * time.Hour})
		eventually(t, "forever data", func() bool {
			return Use(cl, Key{"forever"}, f.fetch, Options{Stale: Forever, Keep: 3 * time.Hour}).HasData
		})
		clk.advance(time.Hour)
	}
	if f.n.Load() != 1 {
		t.Errorf("Forever fetched %d times", f.n.Load())
	}
	var z counter
	for i := 1; i <= 3; i++ {
		Use(cl, Key{"zero"}, z.fetch)
		eventually(t, "zero data", func() bool { return Use(cl, Key{"zero"}, z.fetch).Data == i })
		clk.advance(2 * time.Second)
	}
}

func TestInvalidateFollowsKeyPrefixes(t *testing.T) {
	cl, clk := newTestClient()
	var albums, songs, lists counter
	keys := map[string]struct {
		key Key
		c   *counter
	}{
		"albums": {Key{"library", "albums", "title"}, &albums},
		"songs":  {Key{"library", "songs"}, &songs},
		"lists":  {Key{"playlists"}, &lists},
	}
	use := func() {
		for _, k := range keys {
			Use(cl, k.key, k.c.fetch, Options{Stale: Forever})
		}
	}
	settle := func(want int32) {
		t.Helper()
		eventually(t, "settled fetches", func() bool {
			use()
			for _, k := range keys {
				if k.c.n.Load() < want {
					return false
				}
			}
			return true
		})
		time.Sleep(5 * time.Millisecond)
	}
	counts := func() [3]int32 { return [3]int32{albums.n.Load(), songs.n.Load(), lists.n.Load()} }
	use()
	settle(1)
	if counts() != [3]int32{1, 1, 1} {
		t.Fatalf("start %v", counts())
	}

	cl.Invalidate(Key{"library", "albums"})
	eventually(t, "the albums refetch", func() bool { return albums.n.Load() == 2 })
	if counts() != [3]int32{2, 1, 1} {
		t.Errorf("a longer prefix: %v", counts())
	}

	cl.Invalidate(Key{"library"})
	eventually(t, "the library refetch", func() bool { return albums.n.Load() == 3 && songs.n.Load() == 2 })
	if lists.n.Load() != 1 {
		t.Errorf("a sibling refetched: %v", counts())
	}

	cl.Invalidate(Key{"libr"}, Key{"library", "albums", "title", "more"}, Key{"nothing"})
	time.Sleep(10 * time.Millisecond)
	if counts() != [3]int32{3, 2, 1} {
		t.Errorf("non-prefixes matched: %v", counts())
	}

	cl.Invalidate() // everything
	eventually(t, "everything", func() bool { return counts() == [3]int32{4, 3, 2} })

	// A query nobody used for a while is only marked: it refetches when used again.
	clk.advance(10 * time.Minute)
	before := counts()
	cl.Invalidate(Key{"playlists"})
	time.Sleep(10 * time.Millisecond)
	if counts() != before {
		t.Errorf("an idle query refetched at once: %v", counts())
	}
}

func TestInvalidatedIdleQueryRefetchesWhenUsedAgain(t *testing.T) {
	cl, clk := newTestClient()
	var c counter
	o := Options{Stale: Forever, Keep: time.Hour}
	Use(cl, Key{"q"}, c.fetch, o)
	eventually(t, "data", func() bool { return Use(cl, Key{"q"}, c.fetch, o).HasData })
	clk.advance(5 * time.Minute)
	cl.Invalidate(Key{"q"})
	r := Use(cl, Key{"q"}, c.fetch, o)
	if !r.HasData || r.Data != 1 || !r.Fetching {
		t.Errorf("old data shows while it refetches: %+v", r)
	}
	eventually(t, "refetch", func() bool { return Use(cl, Key{"q"}, c.fetch, o).Data == 2 })
}

func TestInvalidateCancelsAFetchInFlightAndIgnoresItsResult(t *testing.T) {
	cl, _ := newTestClient()
	var runs atomic.Int32
	first := make(chan struct{})
	cancelled := make(chan struct{})
	fetch := func(ctx context.Context) (string, error) {
		if runs.Add(1) == 1 {
			select {
			case <-ctx.Done():
				close(cancelled)
			case <-first:
			}
			return "old", nil
		}
		return "new", nil
	}
	Use(cl, Key{"q"}, fetch)
	eventually(t, "the first fetch", func() bool { return runs.Load() == 1 })
	cl.Invalidate(Key{"q"})
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the fetch in flight was not cancelled")
	}
	eventually(t, "the new result", func() bool { return Use(cl, Key{"q"}, fetch).Data == "new" })
	close(first)
	time.Sleep(10 * time.Millisecond)
	if Use(cl, Key{"q"}, fetch).Data != "new" {
		t.Error("the cancelled fetch's result replaced the new one")
	}
}

// frames runs d of frames, one a second (a frame per mountGap), each
// calling use: what a window in use does.
func frames(clk *fakeClock, d time.Duration, use func()) {
	for range int(d / time.Second) {
		clk.advance(time.Second)
		use()
	}
}

func (cl *Client) sweepNow() int {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.sweepLocked(true)
	return len(cl.entries)
}

func TestKeepDropsUnusedQueriesAndCancelsTheirFetch(t *testing.T) {
	cl, clk := newTestClient()
	started := make(chan struct{})
	gone := make(chan struct{})
	fetch := func(ctx context.Context) (int, error) {
		close(started)
		<-ctx.Done()
		close(gone)
		return 0, ctx.Err()
	}
	o := Options{Keep: time.Minute}
	Use(cl, Key{"slow"}, fetch, o)
	<-started
	var c counter
	od := Options{Keep: time.Minute, Stale: Forever}
	Use(cl, Key{"done"}, c.fetch, od)
	eventually(t, "the other query", func() bool { return Use(cl, Key{"done"}, c.fetch, od).HasData })

	use := func() { Use(cl, Key{"done"}, c.fetch, od) } // kept in use
	frames(clk, 58*time.Second, use)
	cl.sweepNow()
	select {
	case <-gone:
		t.Fatal("dropped before Keep")
	default:
	}

	frames(clk, 3*time.Second, use)
	n := cl.sweepNow()
	select {
	case <-gone:
	case <-time.After(5 * time.Second):
		t.Fatal("the dropped query's context was not cancelled")
	}
	if n != 1 {
		t.Errorf("%d queries kept, want only the one in use", n)
	}
	// Used again, a dropped query fetches anew.
	if r := Use(cl, Key{"slow"}, c.fetch, o); r.HasData {
		t.Errorf("dropped data came back: %+v", r)
	}
}

func TestUseSweepsOnItsOwn(t *testing.T) {
	cl, clk := newTestClient()
	var c counter
	o := Options{Keep: 5 * time.Second}
	Use(cl, Key{"a"}, c.fetch, o)
	eventually(t, "a", func() bool { return Use(cl, Key{"a"}, c.fetch, o).HasData })
	frames(clk, 10*time.Second, func() { Use(cl, Key{"b"}, c.fetch, o) }) // the sweeps drop a
	cl.mu.Lock()
	_, kept := cl.entries[Key{"a"}.id()]
	cl.mu.Unlock()
	if kept {
		t.Error("an entry unused past Keep survived the Uses")
	}
}

func TestAnIdleWindowKeepsItsQueries(t *testing.T) {
	cl, clk := newTestClient()
	var c counter
	o := Options{Keep: time.Minute, Stale: Forever}
	Use(cl, Key{"page"}, c.fetch, o)
	eventually(t, "the page", func() bool { return Use(cl, Key{"page"}, c.fetch, o).HasData })
	// No frames for far longer than Keep: the timer sweeps, and the first
	// frame after still finds the data.
	clk.advance(time.Hour)
	cl.tick()
	cl.mu.Lock()
	cl.timer.Stop()
	cl.mu.Unlock()
	if r := Use(cl, Key{"page"}, c.fetch, o); !r.HasData {
		t.Fatal("an idle window lost its data")
	}
}

func TestEveryQueryOfAPageSurvivesAnIdlePause(t *testing.T) {
	cl, clk := newTestClient()
	var c counter
	o := Options{Keep: time.Minute, Stale: Forever}
	both := func() (a, b Result[int]) {
		return Use(cl, Key{"a"}, c.fetch, o), Use(cl, Key{"b"}, c.fetch, o)
	}
	both()
	eventually(t, "both", func() bool { a, b := both(); return a.HasData && b.HasData })
	clk.advance(10 * time.Minute) // no frames
	both()                        // the first frame after the pause
	if a, b := both(); !a.HasData || !b.HasData {
		t.Fatalf("a query was dropped by the pause: a %+v, b %+v", a, b)
	}
}

func TestAPageLeftPastKeepIsDroppedWhileOthersAreUsed(t *testing.T) {
	cl, clk := newTestClient()
	var c counter
	o := Options{Keep: time.Minute, Stale: Forever}
	Use(cl, Key{"left"}, c.fetch, o)
	eventually(t, "left", func() bool { return Use(cl, Key{"left"}, c.fetch, o).HasData })
	frames(clk, 90*time.Second, func() { Use(cl, Key{"other"}, c.fetch, o) })
	cl.mu.Lock()
	_, kept := cl.entries[Key{"left"}.id()]
	cl.mu.Unlock()
	if kept {
		t.Error("a page left for more than Keep survived while another was used")
	}
}

func TestErrorsKeepDataAndRetrySlowly(t *testing.T) {
	cl, clk := newTestClient()
	boom := errors.New("boom")
	var runs atomic.Int32
	fail := true
	fetch := func(context.Context) (int, error) {
		runs.Add(1)
		if fail {
			return 0, boom
		}
		return 7, nil
	}
	Use(cl, Key{"q"}, fetch)
	eventually(t, "the failure", func() bool { return Use(cl, Key{"q"}, fetch).Err != nil })
	r := Use(cl, Key{"q"}, fetch)
	if r.HasData || r.Loading || !errors.Is(r.Err, boom) {
		t.Errorf("failed: %+v", r)
	}
	for range 100 { // frames: no hot loop on a broken service
		Use(cl, Key{"q"}, fetch)
	}
	if runs.Load() != 1 {
		t.Fatalf("%d fetches in a burst of frames", runs.Load())
	}
	fail = false
	clk.advance(errorRetry)
	Use(cl, Key{"q"}, fetch)
	eventually(t, "the retry", func() bool { return Use(cl, Key{"q"}, fetch).Data == 7 })
	if r := Use(cl, Key{"q"}, fetch); r.Err != nil {
		t.Errorf("error stays after success: %v", r.Err)
	}

	// Data survives a later failure.
	fail = true
	cl.Invalidate(Key{"q"})
	eventually(t, "second failure", func() bool { return Use(cl, Key{"q"}, fetch).Err != nil })
	if r := Use(cl, Key{"q"}, fetch); r.Data != 7 || !r.HasData {
		t.Errorf("data lost on error: %+v", r)
	}
}

func TestAPanickingFetchIsAnError(t *testing.T) {
	cl, _ := newTestClient()
	fetch := func(context.Context) (int, error) { panic("oops") }
	Use(cl, Key{"p"}, fetch)
	eventually(t, "the error", func() bool { return Use(cl, Key{"p"}, fetch).Err != nil })
}

func TestSetWritesDataAndRedraws(t *testing.T) {
	var posts atomic.Int32
	cl := NewClient(func(fn func()) { posts.Add(1); fn() })
	Set(cl, Key{"library", "counts"}, 42)
	if posts.Load() == 0 {
		t.Error("a Set must redraw")
	}
	var c counter
	r := Use(cl, Key{"library", "counts"}, c.fetch, Options{Stale: Forever})
	if !r.HasData || r.Data != 42 || r.Fetching {
		t.Errorf("Set data: %+v", r)
	}
	// A value of another type is not the query's data.
	Set(cl, Key{"mixed"}, "text")
	r2 := Use(cl, Key{"mixed"}, c.fetch, Options{Stale: Forever})
	if r2.HasData && r2.Data == 0 {
		t.Errorf("a string was read as an int: %+v", r2)
	}
}

func TestResetDropsEverythingAndCancels(t *testing.T) {
	cl, _ := newTestClient()
	started, gone := make(chan struct{}), make(chan struct{})
	Use(cl, Key{"a"}, func(ctx context.Context) (int, error) {
		close(started)
		<-ctx.Done()
		close(gone)
		return 0, ctx.Err()
	})
	<-started
	var c counter
	Set(cl, Key{"b"}, 1)
	cl.Reset()
	select {
	case <-gone:
	case <-time.After(5 * time.Second):
		t.Fatal("Reset left a fetch running")
	}
	if r := Use(cl, Key{"b"}, c.fetch, Options{Stale: Forever}); r.HasData {
		t.Errorf("data survived Reset: %+v", r)
	}
}

func TestKeyPrefixSemantics(t *testing.T) {
	type opts struct {
		Sort string
		Desc bool
	}
	k := Key{"library", "albums", opts{"title", false}, 3}
	for _, tc := range []struct {
		prefix Key
		want   bool
	}{
		{nil, true},
		{Key{"library"}, true},
		{Key{"library", "albums"}, true},
		{Key{"library", "albums", opts{"title", false}}, true},
		{Key{"library", "albums", opts{"title", true}}, false},
		{Key{"library", "albums", opts{"title", false}, 3}, true},
		{Key{"library", "albums", opts{"title", false}, 3, "x"}, false},
		{Key{"lib"}, false},
		{Key{"albums"}, false},
		{Key{"library", "albums", opts{"title", false}, int64(3)}, false},
	} {
		if got := k.hasPrefix(tc.prefix); got != tc.want {
			t.Errorf("%v has prefix %v: %v, want %v", k, tc.prefix, got, tc.want)
		}
	}
	// Elements that cannot be compared with == still compare.
	if !(Key{[]string{"a"}}).hasPrefix(Key{[]string{"a"}}) || (Key{[]string{"a"}}).hasPrefix(Key{[]string{"b"}}) {
		t.Error("slice elements")
	}
	if (Key{"a", 1}).id() == (Key{"a", "1"}).id() {
		t.Error("1 and \"1\" are one key")
	}
	// The fast path's types stay apart from each other and from their
	// named and unnamed kin, and a string's quotes cannot forge another key.
	type name string
	keys := []Key{
		{1}, {int64(1)}, {uint64(1)}, {"1"}, {name("1")}, {true}, {"true"},
		{"a", "b"}, {"a\x00string:\"b\""}, {`a" "b`}, {opts{"a", false}}, {[]string{"a"}},
	}
	seen := map[string]Key{}
	for _, k := range keys {
		if o, dup := seen[k.id()]; dup {
			t.Errorf("%#v and %#v are one key", o, k)
		}
		seen[k.id()] = k
		if k.id() != (Key)(append(Key(nil), k...)).id() {
			t.Errorf("%#v: the id is not stable", k)
		}
	}
}

func TestNoRetryKeepsTheErrorUntilInvalidateOrRemount(t *testing.T) {
	cl, clk := newTestClient()
	boom := errors.New("boom")
	var runs atomic.Int32
	fetch := func(context.Context) (int, error) { runs.Add(1); return 0, boom }
	o := Options{NoRetry: true}
	Use(cl, Key{"q"}, fetch, o)
	eventually(t, "the failure", func() bool { return Use(cl, Key{"q"}, fetch, o).Err != nil })
	// A view that keeps drawing it (a frame every 200 ms for a minute) is
	// never asked again: the 5 s retry of a plain query does not apply.
	for range 300 {
		clk.advance(200 * time.Millisecond)
		Use(cl, Key{"q"}, fetch, o)
	}
	if runs.Load() != 1 {
		t.Fatalf("a NoRetry query was asked %d times while on screen", runs.Load())
	}
	if r := Use(cl, Key{"q"}, fetch, o); !errors.Is(r.Err, boom) || r.HasData {
		t.Errorf("error not kept: %+v", r)
	}
	// Invalidate asks again.
	cl.Invalidate(Key{"q"})
	// Wait for the fetch to finish, not just start: a mount while it is in
	// flight asks nothing, and nothing would ask again.
	eventually(t, "the refetch after Invalidate", func() bool {
		return runs.Load() == 2 && !Use(cl, Key{"q"}, fetch, o).Fetching
	})
	// So does a view that mounts it again after a gap.
	clk.advance(10 * time.Second)
	Use(cl, Key{"q"}, fetch, o)
	eventually(t, "the refetch on a new mount", func() bool { return runs.Load() == 3 })
}
