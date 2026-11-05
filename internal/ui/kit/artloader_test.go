package kit

import (
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// began waits for n requests to begin waiting in the fake source.
func began(t *testing.T, src *fakeArt, n int) {
	t.Helper()
	for range n {
		select {
		case <-src.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("a request the loader should have started did not begin")
		}
	}
}

func (f *fakeArt) cancelled() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cancels
}

// A cover is a request of a distinct width, so the order the source saw them in
// says which the loader took first.
func askAs(l *ArtLoader, v *artWindow, r artRank, w int) *cover {
	return l.cover(v, r, "/art/k1/x.jpg", float32(w), 1)
}

func widths(src *fakeArt) []int {
	var out []int
	for _, r := range src.requests() {
		out = append(out, r[0])
	}
	return out
}

func TestLoaderHasSeveralCoversInFlightAtOnce(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{}), entered: make(chan struct{}, 16)}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 4)
	defer l.Close()
	if cap(l.decode) != runtime.GOMAXPROCS(0) {
		t.Errorf("%d decodes at once, want one per CPU (%d)", cap(l.decode), runtime.GOMAXPROCS(0))
	}
	for i := range 6 {
		askAs(l, nil, rankBuilt, 10+i)
	}
	// Four requests wait at the source together; the other two wait for a
	// worker, none of which is free while the four wait.
	began(t, src, 4)
	select {
	case <-src.entered:
		t.Fatal("a fifth request began with four workers")
	default:
	}
	if l.Pending() != 6 {
		t.Errorf("%d covers pending, want 6", l.Pending())
	}
	close(src.gate)
	mt.run(t, 6)
	if l.Pending() != 0 || len(src.requests()) != 6 {
		t.Errorf("%d covers pending after the answers, %d requests", l.Pending(), len(src.requests()))
	}
}

func TestLoaderTakesTheMostWantedFirstThenTheEarliestAsked(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{}), entered: make(chan struct{}, 16)}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 1)
	defer l.Close()
	askAs(l, nil, rankBuilt, 10) // the only worker takes it at once
	began(t, src, 1)
	askAs(l, nil, rankAhead, 20)
	askAs(l, nil, rankBuilt, 30)
	askAs(l, nil, rankBuilt, 31)
	askAs(l, nil, rankView, 40)
	askAs(l, nil, rankAhead, 21)
	askAs(l, nil, rankView, 21)  // wanted after all: on screen now
	askAs(l, nil, rankAhead, 40) // asked ahead again: never less than it was
	for range 6 {
		src.gate <- struct{}{} // one request at a time
		mt.run(t, 1)
	}
	want := []int{10, 40, 21, 30, 31, 20}
	got := widths(src)
	if len(got) != len(want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("requests %v, want %v: on screen first, then the page, then what was prefetched, each in the order asked", got, want)
		}
	}
}

func TestLoaderCallsOffWhatAPassDoesNotAskAgain(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{}), entered: make(chan struct{}, 16)}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 1)
	defer l.Close()
	v := &artWindow{}
	a, b, c := askAs(l, v, rankBuilt, 10), askAs(l, v, rankBuilt, 20), askAs(l, v, rankBuilt, 30)
	began(t, src, 1) // a is in flight, b and c are queued
	l.painted(v, 1)  // the pass asked for all three
	if l.Pending() != 3 {
		t.Fatalf("%d pending after a pass that asked for every cover", l.Pending())
	}

	// The next pass asks for a and c: b left the screen, and goes when the
	// pass is painted, not before.
	askAs(l, v, rankBuilt, 10)
	askAs(l, v, rankBuilt, 30)
	if b.state != coverQueued {
		t.Fatalf("b is %v before the pass was painted", b.state)
	}
	l.painted(v, 1)
	if b.state != coverIdle || a.state != coverQueued || c.state != coverQueued || l.Pending() != 2 {
		t.Errorf("states a=%v b=%v c=%v, %d pending; want b called off and the others still wanted", a.state, b.state, c.state, l.Pending())
	}
	close(src.gate)
	mt.run(t, 2)
	if got := widths(src); len(got) != 2 || got[0] != 10 || got[1] != 30 {
		t.Errorf("requests %v: a cover called off while queued was still fetched", got)
	}
	if a.bitmap() == nil || c.bitmap() == nil || b.bitmap() != nil {
		t.Error("the covers still wanted were not published, or the one called off was")
	}
}

func TestLoaderCancelsACoverInFlightAndFetchesItAgainWhenAsked(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{}), entered: make(chan struct{}, 16)}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 2)
	defer l.Close()
	v := &artWindow{}
	a := askAs(l, v, rankView, 10)
	began(t, src, 1)
	l.painted(v, 1)

	// The next pass wants another cover only.
	b := askAs(l, v, rankView, 20)
	began(t, src, 1)
	l.painted(v, 1)
	if a.state != coverIdle {
		t.Fatalf("a is %v after a pass without it", a.state)
	}
	mt.run(t, 1) // the worker's answer: the request ended with its context
	if got := src.cancelled(); got != 1 {
		t.Errorf("%d requests ended by their context, want 1", got)
	}
	if a.bitmap() != nil || a.state != coverIdle || a.fails != 0 {
		t.Errorf("a cover that was called off is %v with %d failures: it must not look failed", a.state, a.fails)
	}

	// Back on screen: it is asked for again, and arrives.
	askAs(l, v, rankView, 10)
	began(t, src, 1)
	close(src.gate)
	// Both requests are recorded only once they pass the gate: wait for
	// both covers, not only a's, or b's Get may not have counted yet.
	for a.bitmap() == nil || b.bitmap() == nil {
		mt.run(t, 1)
	}
	if got := widths(src); len(got) != 2 {
		t.Errorf("requests %v: the cover asked for again, and the one still wanted", got)
	}
}

func TestLoaderEndsPassesOnlyWhenPaintedAndPerWindow(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{}), entered: make(chan struct{}, 16)}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 1)
	defer l.Close()
	one, two := &artWindow{}, &artWindow{}
	a := askAs(l, one, rankBuilt, 10)
	b := askAs(l, one, rankBuilt, 20)
	y := askAs(l, two, rankBuilt, 30)
	began(t, src, 1)
	l.painted(one, 1)
	l.painted(two, 1)

	// A pass of one window that is not painted ends nothing, whatever it left
	// out, and another window's passes never touch its covers.
	l.begin(two)
	l.painted(two, 1)
	l.begin(one)
	if a.state != coverQueued || b.state != coverQueued || y.state != coverIdle {
		t.Errorf("states a=%v b=%v y=%v: want y called off by its own window's painted pass, and a and b left alone by that and by their own unpainted one", a.state, b.state, y.state)
	}
	askAs(l, one, rankBuilt, 10)
	l.painted(one, 1)
	if a.state != coverQueued || b.state != coverIdle {
		t.Errorf("states a=%v b=%v after the painted pass that asked for a only", a.state, b.state)
	}
	close(src.gate)
}

func TestLoaderCallsOffCoversWhenThePageThatFollowsHasNone(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{}), entered: make(chan struct{}, 16)}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 1)
	defer l.Close()
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	covers := true
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		end := l.Pass(c)
		root := ui.Column(c).Fill()
		root.Draw(func(p *ui.Painter, _ ui.Rect) { end(p) })
		root.Children(func() {
			if covers {
				Artwork(c.Key("a"), l, "/art/k1/300x300.jpg", 50, ArtOpts{})
				Artwork(c.Key("b"), l, "/art/k2/300x300.jpg", 50, ArtOpts{})
			} else {
				ui.Text(c, "Settings")
			}
		})
	}, 200, 200)
	tt.Frame()
	began(t, src, 1)
	if l.Pending() != 2 {
		t.Fatalf("%d covers pending on the page with two", l.Pending())
	}
	covers = false
	tt.Frame()
	if l.Pending() != 0 {
		t.Errorf("%d covers pending on a page that builds none: the pass ended without being told", l.Pending())
	}
	mt.run(t, 1)
	if got := src.cancelled(); got != 1 {
		t.Errorf("%d requests cancelled, want the one in flight", got)
	}
}

func TestArtworkOnScreenIsWantedBeforeTheRestOfThePage(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{})}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 1)
	defer l.Close()
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Scroll(c).Fill().Children(func() {
			for i := range 10 {
				Artwork(c.Key(i), l, "/art/k"+strconv.Itoa(i)+"/x.jpg", 100, ArtOpts{})
			}
		})
	}, 200, 250)
	tt.Frame() // the first frame cannot tell what shows
	tt.Frame()
	rankOf := func(i int) artRank {
		return l.covers[artKey{"/art/k" + strconv.Itoa(i) + "/x.jpg", 100, 100}].rank
	}
	if first, last := rankOf(0), rankOf(9); first != rankView || last != rankBuilt {
		t.Errorf("ranks: first cover %v, last %v; want on screen (%v) and built (%v)", first, last, rankView, rankBuilt)
	}
	close(src.gate)
}

func TestPrefetchAsksAtTheSizeArtworkWouldAndDrawsNothing(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{})}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 1)
	defer l.Close()
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	shown := false
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Fill().Children(func() {
			PrefetchArtwork(c, l, "/art/k1/x.jpg", 40, ArtOpts{Aspect: 2})
			PrefetchArtwork(c, l, "", 40, ArtOpts{})
			PrefetchArtwork(c, nil, "/art/k1/x.jpg", 40, ArtOpts{})
			if shown {
				Artwork(c.Key("a"), l, "/art/k1/x.jpg", 40, ArtOpts{Aspect: 2})
			}
		})
	}, 100, 100)
	tt.Frame()
	cv := l.covers[artKey{"/art/k1/x.jpg", 40, 20}]
	if cv == nil || len(l.covers) != 1 || cv.rank != rankAhead || cv.state != coverQueued {
		t.Fatalf("covers %v: want the one Artwork would show (40×20), queued as ahead", l.covers)
	}
	if got := len(tt.Texts()); got != 0 {
		t.Errorf("prefetching built %d texts", got)
	}
	// What a pass shows is not asked ahead of itself: the same cover, built.
	shown = true
	tt.Frame()
	if len(l.covers) != 1 || cv.rank == rankAhead || l.covers[artKey{"/art/k1/x.jpg", 40, 20}] != cv {
		t.Errorf("a cover shown and prefetched is still only ahead: %v", cv.rank)
	}
	close(src.gate)
}

func TestCoverGridPrefetchesTheTwoRowsPastItsBuiltOnes(t *testing.T) {
	a, tt := newCoverApp(t, 5000, 1000, 700)
	tt.Frame()
	tt.Frame()
	asked := func(i int) *cover {
		for k, cv := range a.art.covers {
			if k.src == "/art/k"+strconv.Itoa(i)+"/x.jpg" {
				return cv
			}
		}
		return nil
	}
	// 5 columns: rows 0..5 are built, rows 6 and 7 (tiles 30..39) ahead.
	if cv := asked(29); cv == nil || cv.rank == rankAhead {
		t.Errorf("tile 29 of the last built row: %v", cv)
	}
	for _, i := range []int{30, 39} {
		if cv := asked(i); cv == nil || cv.rank != rankAhead {
			t.Errorf("tile %d is not asked ahead: %v", i, cv)
		}
	}
	if cv := asked(40); cv != nil {
		t.Errorf("tile 40, a third row past, was asked for")
	}
	if tt.HasText("Artist 30") {
		t.Error("tile 30 was built: only its cover is asked for")
	}
}

func TestCoverRowPrefetchesTheNextRowWhileTheShelfIsOnScreen(t *testing.T) {
	a, tt := newCollShelfApp(t, 20, 1000)
	tt.Frame()
	tt.Frame()
	tt.Frame()
	asked := func(i int) bool {
		for k := range a.art.covers {
			if k.src == "/art/k"+strconv.Itoa(i)+"/x.jpg" {
				return true
			}
		}
		return false
	}
	// 5 columns: tiles 0..4 show, 5..9 are what "Show all" opens first.
	if !asked(4) || !asked(5) || !asked(9) {
		t.Error("the shown row and the next were not all asked for")
	}
	if asked(10) {
		t.Error("the row after next was asked for")
	}
	if got := collArtists(tt); got != 5 {
		t.Errorf("%d tiles built, want only the row that shows", got)
	}
}

func TestLoaderPrefetchWaitsForTheCoversOnScreen(t *testing.T) {
	// No workers: what the queue would hand out, by hand.
	l := newArtLoader(&fakeArt{}, nil, 0)
	defer l.Close()
	ahead, built, view := askAs(l, nil, rankAhead, 10), askAs(l, nil, rankBuilt, 20), askAs(l, nil, rankView, 30)
	at := func(cv *cover) int { return slices.Index(l.jobs.queue, cv) }
	pick := func(onScreen int) int {
		l.jobs.Lock()
		defer l.jobs.Unlock()
		l.jobs.onScreen = onScreen
		return l.pick()
	}
	if got := pick(1); got != at(view) {
		t.Errorf("picked %d while a cover on screen is in flight, want the one queued for the screen (%d)", got, at(view))
	}
	l.jobs.queue = slices.DeleteFunc(l.jobs.queue, func(cv *cover) bool { return cv == view })
	if got := pick(1); got != at(built) {
		t.Errorf("picked %d, want the page's cover (%d): only prefetching waits", got, at(built))
	}
	l.jobs.queue = slices.DeleteFunc(l.jobs.queue, func(cv *cover) bool { return cv == built })
	if got := pick(1); got != -1 {
		t.Errorf("picked %d: a prefetch started while a cover on screen is in flight", got)
	}
	if got := pick(0); got != at(ahead) {
		t.Errorf("picked %d with nothing on screen in flight, want the prefetch (%d)", got, at(ahead))
	}
}

func TestLoaderStartsPrefetchingOnceTheLastCoverOnScreenIsDone(t *testing.T) {
	src := &fakeArt{gate: make(chan struct{}), entered: make(chan struct{}, 16)}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 2)
	defer l.Close()
	view := askAs(l, nil, rankView, 10)
	began(t, src, 1)
	ahead := askAs(l, nil, rankAhead, 20) // a worker is free but waits
	src.gate <- struct{}{}                // the cover on screen is done: the free worker starts the prefetch
	began(t, src, 1)
	src.gate <- struct{}{}
	for view.bitmap() == nil || ahead.bitmap() == nil {
		mt.run(t, 1)
	}
	if got := widths(src); len(got) != 2 || got[0] != 10 || got[1] != 20 {
		t.Errorf("requests %v, want the cover on screen then the prefetch", got)
	}
}

// The loader's memory is bounded by what the covers weigh, not by how many
// there are: a library's Albums grid scrolled to its end (739 covers) used to
// stay decoded, texture and JPEG, until the 800th cover.
func TestLoaderDropsTheLeastRecentlyAskedCoversOverItsByteBudget(t *testing.T) {
	src := &fakeArt{}
	mt := newMainThread()
	l := newArtLoader(src, mt.post, 1)
	defer l.Close()
	// Each 100×100 cover weighs its 40 000 pixel bytes and its JPEG: three of
	// them fit, four do not.
	l.maxBytes = 3*40_000 + 3_000
	key := func(i int) artKey { return artKey{"/art/k" + strconv.Itoa(i) + "/x.jpg", 100, 100} }
	ask := func(i int) {
		t.Helper()
		l.cover(nil, rankBuilt, "/art/k"+strconv.Itoa(i)+"/x.jpg", 100, 100)
	}
	ready := func() (n int) {
		for _, cv := range l.covers {
			if cv.state == coverReady {
				n++
			}
		}
		return n
	}
	load := func(i int) {
		t.Helper()
		ask(i)
		mt.run(t, 1)
		if cv := l.covers[key(i)]; cv == nil || cv.state != coverReady {
			t.Fatalf("cover %d is not ready after its answer", i)
		}
	}
	for i := range 3 {
		load(i)
	}
	if l.bytes > l.maxBytes || ready() != 3 {
		t.Fatalf("%d covers ready weighing %d, budget %d: three fit", ready(), l.bytes, l.maxBytes)
	}
	// Cover 0 is asked for again, so cover 1 is the least recently asked for
	// when cover 3 comes over the budget.
	ask(0)
	load(3)
	if l.covers[key(1)] != nil || l.covers[key(0)] == nil || l.covers[key(3)] == nil {
		t.Errorf("covers kept: 0 %v, 1 %v, 2 %v, 3 %v; want 1 dropped (the oldest ask) and the rest kept down to 3/4 of the budget",
			l.covers[key(0)] != nil, l.covers[key(1)] != nil, l.covers[key(2)] != nil, l.covers[key(3)] != nil)
	}
	for i := 4; i < 12; i++ {
		load(i)
		var sum int64
		for _, cv := range l.covers {
			if cv.state == coverReady {
				sum += cv.cost
			}
		}
		if l.bytes != sum || l.bytes > l.maxBytes {
			t.Fatalf("after cover %d the loader counts %d bytes, the ready covers weigh %d, budget %d", i, l.bytes, sum, l.maxBytes)
		}
	}
	if l.covers[key(11)] == nil || l.covers[key(2)] != nil {
		t.Error("the newest cover must be kept and an old one dropped")
	}
	// A dropped cover comes back when asked for again.
	load(2)
}
