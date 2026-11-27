package services

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

// testTimers stands for the one time.Timer: a test fires what the release
// timer set on the clock it moves, and reads how long it waits.
type testTimers struct {
	mu    sync.Mutex
	timer []*testTimer
}

type testTimer struct {
	d       time.Duration
	fn      func()
	stopped bool
}

func (ts *testTimers) after(d time.Duration, fn func()) func() bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	t := &testTimer{d: d, fn: fn}
	ts.timer = append(ts.timer, t)
	return func() bool {
		ts.mu.Lock()
		defer ts.mu.Unlock()
		was := !t.stopped
		t.stopped = true
		return was
	}
}

// armed is the timer that waits now (nil when none does).
func (ts *testTimers) armed() *testTimer {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for _, t := range slices.Backward(ts.timer) {
		if !t.stopped {
			return t
		}
	}
	return nil
}

// fire runs the timer that waits now, as the runtime does when its wait is
// over, and says whether there was one.
func (ts *testTimers) fire() bool {
	t := ts.armed()
	if t == nil {
		return false
	}
	ts.mu.Lock()
	t.stopped = true
	ts.mu.Unlock()
	t.fn()
	return true
}

func (ts *testTimers) count() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.timer)
}

// releaseRig is a LibraryService over the pre-added album of preAddedStore
// (out on 2026-11-20) with the release timer on a clock and timers of the
// test's. Its syncs only count.
type releaseRig struct {
	s      *LibraryService
	d      *db.DB
	timers *testTimers
	now    time.Time
	in     bool // signed in
	syncs  int
}

func newReleaseRig(t *testing.T, now time.Time) *releaseRig {
	t.Helper()
	d := preAddedStore(t)
	r := &releaseRig{d: d, timers: &testTimers{}, now: now, in: true}
	e := &Env{Deps: Deps{DB: d, Library: libraryAt(d, &r.now), Logger: discard(), Now: func() time.Time { return r.now }}}
	r.s = &LibraryService{o: e, m: mapper{now: e.Now}, signedIn: func() bool { return r.in }}
	r.s.rel = newReleaseSync(r.s)
	r.s.rel.after = r.timers.after
	r.s.rel.run = func() { r.syncs++ }
	r.s.rel.started = true
	t.Cleanup(r.s.rel.close)
	return r
}

// due is when the timer syncs next, the zero time when it has no sync to wait
// for.
func (r *releaseRig) due() time.Time {
	r.s.rel.mu.Lock()
	defer r.s.rel.mu.Unlock()
	return r.s.rel.due
}

// A fresh library syncs at the release and then at each backoff: a stage
// is an offset from the release.
func TestReleaseSyncOffsets(t *testing.T) {
	want := []time.Duration{2 * time.Minute, 17 * time.Minute, 77 * time.Minute, 437 * time.Minute}
	if !slices.Equal(releaseSyncOffsets, want) {
		t.Fatalf("offsets %v, want %v (release + 2 min, then +15 min, +1 h, +6 h)", releaseSyncOffsets, want)
	}
}

func TestNextReleaseSync(t *testing.T) {
	day := localDay(t, "2026-11-20")
	soon := library.PendingRelease{LibraryID: "l.soon", CatalogReleaseDate: "2026-11-20", Prerelease: true}
	later := library.PendingRelease{LibraryID: "l.later", CatalogReleaseDate: "2026-12-01", Prerelease: true}
	yearOnly := library.PendingRelease{LibraryID: "l.year", CatalogReleaseDate: "2027", Prerelease: true}
	stage := func(i int) time.Time { return day.Add(releaseSyncOffsets[i]) }
	for _, c := range []struct {
		name string
		rows []library.PendingRelease
		last time.Time
		now  time.Time
		want time.Time // zero: none
	}{
		{"none", nil, time.Time{}, day, time.Time{}},
		{"the earliest of several", []library.PendingRelease{later, soon}, time.Time{}, day.Add(-48 * time.Hour), stage(0)},
		{"a year alone has no time to wait for", []library.PendingRelease{yearOnly}, time.Time{}, day, time.Time{}},
		{"a date-only release is local midnight", []library.PendingRelease{soon}, day.Add(-time.Hour), day.Add(-time.Hour), stage(0)},
		{"a sync before the release changes nothing", []library.PendingRelease{soon}, day.Add(-time.Minute), day, stage(0)},
		{"a sync after the first stage", []library.PendingRelease{soon}, stage(0).Add(time.Second), stage(0).Add(time.Second), stage(1)},
		{"a sync between stages", []library.PendingRelease{soon}, stage(1).Add(time.Second), stage(1).Add(time.Hour), stage(2)},
		{"a stage passed unsynced is due", []library.PendingRelease{soon}, day.Add(-time.Hour), stage(1).Add(time.Minute), stage(0)},
		{"the last stage done", []library.PendingRelease{soon}, stage(3).Add(time.Second), stage(3).Add(time.Hour), time.Time{}},
		{"a later album while the earlier is done", []library.PendingRelease{soon, later}, stage(3), stage(3), later2(t)},
	} {
		got, ok := nextReleaseSync(c.rows, c.last, c.now)
		if ok != !c.want.IsZero() || !got.Equal(c.want) {
			t.Errorf("%s: %v, %v; want %v", c.name, got, ok, c.want)
		}
	}
}

// later2 is the first stage of the album due on 2026-12-01.
func later2(t *testing.T) time.Time { return localDay(t, "2026-12-01").Add(releaseSyncOffsets[0]) }

// The timer is set for the earliest release among the library's albums that
// are not out (cut at releaseSyncCheck), and set again, from the library as
// it is then, after a sync and when an album leaves.
func TestReleaseSyncArmsForTheEarliestUpcomingAlbum(t *testing.T) {
	release := localDay(t, "2026-11-20")
	r := newReleaseRig(t, release.Add(-5*time.Hour))
	addPreAdded(t, r.d, "l.later", "Later", "2026-12-01")

	r.s.rel.rearm()
	if got, want := r.due(), release.Add(releaseSyncDelay); !got.Equal(want) {
		t.Fatalf("due %v, want release + %v = %v", got, releaseSyncDelay, want)
	}
	if tm := r.timers.armed(); tm == nil || tm.d != releaseSyncCheck {
		t.Fatalf("timer %+v, want one wait cut at %v", tm, releaseSyncCheck)
	}

	// A sync (any) sets it again: the same time, on a new timer.
	n := r.timers.count()
	r.s.rearmRelease()
	if r.timers.count() != n+1 || !r.due().Equal(release.Add(releaseSyncDelay)) {
		t.Errorf("after a sync: %d timers (was %d), due %v", r.timers.count(), n, r.due())
	}
	live := 0
	for _, tm := range r.timers.timer {
		if !tm.stopped {
			live++
		}
	}
	if live != 1 {
		t.Errorf("%d timers waiting, want one", live)
	}

	// The earlier album leaves the library: the later one is next.
	if _, err := r.d.W.ExecContext(t.Context(), `DELETE FROM library_items WHERE kind = 'album' AND ref = (SELECT id FROM albums WHERE library_id = 'l.soon')`); err != nil {
		t.Fatal(err)
	}
	r.s.rearmRelease()
	if got, want := r.due(), localDay(t, "2026-12-01").Add(releaseSyncDelay); !got.Equal(want) {
		t.Errorf("after the earlier album left: due %v, want %v", got, want)
	}
}

// At the release plus the delay the timer starts one sync, and goes on to
// the next stage: Apple still shows the album as not out.
func TestReleaseSyncFiresOnceAtReleaseAndDelay(t *testing.T) {
	release := localDay(t, "2026-11-20")
	r := newReleaseRig(t, release.Add(-time.Hour))
	r.s.rel.rearm()

	// The wait is cut, and the timer waking before the time syncs nothing.
	r.now = release.Add(time.Minute)
	if !r.timers.fire() || r.syncs != 0 {
		t.Fatalf("a wake before the time: %d syncs, want none", r.syncs)
	}
	if tm := r.timers.armed(); tm == nil || tm.d != time.Minute {
		t.Fatalf("timer %+v, want the minute left to wait", tm)
	}

	r.now = release.Add(releaseSyncDelay)
	if !r.timers.fire() {
		t.Fatal("no timer to fire")
	}
	if r.syncs != 1 {
		t.Fatalf("%d syncs at the release + %v, want one", r.syncs, releaseSyncDelay)
	}
	if got, want := r.due(), release.Add(releaseSyncOffsets[1]); !got.Equal(want) {
		t.Errorf("next due %v, want %v (the first backoff after this sync)", got, want)
	}
	if r.timers.fire() && r.syncs != 1 {
		t.Errorf("a second wake before the retry ran a sync: %d", r.syncs)
	}
}

// While Apple says the album is not out the sync is repeated at +15 minutes,
// +1 hour and +6 hours, and then not again.
func TestReleaseSyncBacksOffThenStops(t *testing.T) {
	release := localDay(t, "2026-11-20")
	r := newReleaseRig(t, release.Add(-time.Hour))
	r.s.rel.rearm()
	for i, off := range releaseSyncOffsets {
		if got, want := r.due(), release.Add(off); !got.Equal(want) {
			t.Fatalf("stage %d: due %v, want %v", i, got, want)
		}
		r.now = release.Add(off)
		r.timers.fire()
		if r.syncs != i+1 {
			t.Fatalf("stage %d: %d syncs, want %d", i, r.syncs, i+1)
		}
	}
	if !r.due().IsZero() || r.timers.armed() != nil {
		t.Errorf("after the last stage: due %v, timer %+v; want none", r.due(), r.timers.armed())
	}
	r.now = release.Add(24 * time.Hour)
	r.s.rearmRelease()
	if r.timers.armed() != nil {
		t.Error("a timer is set again for an album whose stages are over")
	}
}

// Once Apple has published (the album no longer flagged, its songs with
// playParams) nothing is left to wait for.
func TestReleaseSyncStopsWhenAppleHasPublished(t *testing.T) {
	release := localDay(t, "2026-11-20")
	r := newReleaseRig(t, release.Add(-time.Hour))
	r.s.rel.rearm()
	r.now = release.Add(releaseSyncDelay)
	r.timers.fire()
	if r.syncs != 1 || r.due().IsZero() {
		t.Fatalf("%d syncs, due %v; want the first sync and a retry waiting", r.syncs, r.due())
	}
	publish(t, r.d)
	r.s.rearmRelease()
	if !r.due().IsZero() || r.timers.armed() != nil {
		t.Errorf("due %v, timer %+v after the album was published; want none", r.due(), r.timers.armed())
	}
}

// Signed out, or with no album waiting for its release, no timer is set; a
// sign-out and a shutdown cancel the one that is.
func TestReleaseSyncHasNoTimerWithoutSignInOrAlbums(t *testing.T) {
	release := localDay(t, "2026-11-20")
	r := newReleaseRig(t, release.Add(-time.Hour))

	r.in = false
	r.s.rel.rearm()
	if r.timers.count() != 0 {
		t.Fatal("a timer was set while signed out")
	}

	r.in = true
	r.s.rel.rearm()
	if r.timers.armed() == nil {
		t.Fatal("no timer when signed in with an album to wait for")
	}
	r.s.rel.disable() // sign-out
	if r.timers.armed() != nil || !r.due().IsZero() {
		t.Error("the timer survived the sign-out")
	}
	r.s.rel.rearm() // a sync ending after it, still signed in as far as it knows
	if r.timers.armed() != nil {
		t.Error("a sync that ended after the sign-out set the timer")
	}
	r.s.rel.enable() // signed in again
	if r.timers.armed() == nil {
		t.Fatal("no timer after signing in again")
	}
	r.s.rel.close()
	if r.timers.armed() != nil {
		t.Error("the timer survived the shutdown")
	}
	r.s.rel.rearm()
	if r.timers.armed() != nil {
		t.Error("a timer was set after the shutdown")
	}

	empty := newReleaseRig(t, release.Add(-time.Hour))
	if _, err := empty.d.W.ExecContext(t.Context(), `DELETE FROM library_items WHERE kind = 'album'`); err != nil {
		t.Fatal(err)
	}
	empty.s.rel.rearm()
	if empty.timers.armed() != nil {
		t.Error("a timer was set with no album to wait for")
	}

	// Before the first paint nothing is set either.
	early := newReleaseRig(t, release.Add(-time.Hour))
	early.s.rel.started = false
	early.s.rel.rearm()
	if early.timers.armed() != nil {
		t.Error("a timer was set before the network opened")
	}
	early.s.rel.begin()
	if early.timers.armed() == nil {
		t.Error("no timer once the network opened")
	}
}

// A release that passed while the computer slept, or the clock jumped over,
// syncs at the first wake after it; one that passed while Sona was not
// running, at the start-up.
func TestReleaseSyncAfterASuspendOrAClockJump(t *testing.T) {
	release := localDay(t, "2026-11-20")
	r := newReleaseRig(t, release.Add(-6*time.Hour))
	r.s.rel.rearm()
	// The timers stood still in the sleep; the clock did not.
	r.now = release.Add(3 * time.Hour)
	r.timers.fire()
	if r.syncs != 1 {
		t.Fatalf("%d syncs on waking after the release, want one", r.syncs)
	}
	// That sync covers the retries that passed too: they do not run in a
	// burst, the next is the first stage after it.
	if got, want := r.due(), release.Add(releaseSyncOffsets[3]); !got.Equal(want) {
		t.Fatalf("next due %v, want %v", got, want)
	}
	if r.syncs != 1 {
		t.Fatalf("%d syncs, want the passed retries not run again", r.syncs)
	}

	// The clock set back: nothing syncs before the time.
	back := newReleaseRig(t, release.Add(-time.Hour))
	back.s.rel.rearm()
	back.now = release.Add(-48 * time.Hour)
	back.timers.fire()
	if back.syncs != 0 || back.due().IsZero() {
		t.Errorf("clock set back: %d syncs, due %v; want none and the time kept", back.syncs, back.due())
	}

	// Start-up after the release, with no sync since: it is due at once.
	up := newReleaseRig(t, release.Add(30*time.Minute))
	up.s.rel.begin()
	if tm := up.timers.armed(); tm == nil || tm.d != 0 {
		t.Fatalf("start-up: timer %+v, want one due at once", tm)
	}
	up.timers.fire()
	if up.syncs != 1 {
		t.Errorf("start-up: %d syncs, want one", up.syncs)
	}

	// ... but not when the library synced after that stage already.
	done := newReleaseRig(t, release.Add(30*time.Minute))
	for _, dom := range library.AllDomains {
		if _, err := done.d.W.ExecContext(t.Context(), `INSERT INTO sync_state (domain, last_success) VALUES (?, ?)`, string(dom), release.Add(20*time.Minute).UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	done.s.rel.begin()
	if got, want := done.due(), release.Add(releaseSyncOffsets[2]); !got.Equal(want) {
		t.Errorf("synced after the first two stages: due %v, want %v", got, want)
	}
}

// A sync that runs at the time is the one a sync after an add is: it waits
// for the running one and never overlaps it.
func TestReleaseSyncQueuesBehindARunningSync(t *testing.T) {
	release := localDay(t, "2026-11-20")
	r := newReleaseRig(t, release.Add(-time.Hour))
	r.s.rel.close()
	r.s.rel = newReleaseSync(r.s)
	r.s.rel.after = r.timers.after
	r.s.rel.started = true
	t.Cleanup(r.s.rel.close)
	running := make(chan struct{})
	r.s.syncDone = running // a sync is running
	r.s.rel.rearm()
	r.now = release.Add(releaseSyncDelay)
	r.timers.fire()

	r.s.mu.Lock()
	queued := slices.Clone(r.s.resync)
	started := r.s.syncDone == running
	r.s.mu.Unlock()
	if !started {
		t.Fatal("the release sync replaced the running one")
	}
	if !slices.Equal(queued, library.AllDomains) {
		t.Errorf("queued %v, want every domain once the running sync ends", queued)
	}
}

// addPreAdded stores another pre-added album that comes out on date.
func addPreAdded(t *testing.T, d *db.DB, id, name, date string) {
	t.Helper()
	ref, err := d.Q.UpsertLibraryAlbum(t.Context(), dbq.UpsertLibraryAlbumParams{
		LibraryID: db.NullString(id), CatalogID: db.NullString("c" + id), Name: name,
		IsPrerelease: 1, CatalogReleaseDate: date, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Q.MarkLibraryItem(t.Context(), dbq.MarkLibraryItemParams{Kind: "album", Ref: ref, Generation: 1}); err != nil {
		t.Fatal(err)
	}
}

// publish is what a sync after Apple published stores for preAddedStore's
// album: the flag cleared and every song with playParams.
func publish(t *testing.T, d *db.DB) {
	t.Helper()
	for _, q := range []string{
		`UPDATE albums SET is_prerelease = 0 WHERE library_id = 'l.soon'`,
		`UPDATE tracks SET no_play_params = 0 WHERE album_library_id = 'l.soon'`,
	} {
		if _, err := d.W.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
}

// PendingReleases lists the albums ahead of their release or flagged, and
// the ones released since yesterday that still hold a placeholder song; not
// an album released long ago that has a song Apple never gave playParams.
func TestPendingReleasesListsOnlyWhatAReleaseSyncMayChange(t *testing.T) {
	release := localDay(t, "2026-11-20")
	r := newReleaseRig(t, release.Add(2*time.Hour))
	publish(t, r.d) // l.soon: out, every song with playParams
	addSong := func(album, id string) {
		t.Helper()
		ref, err := r.d.Q.UpsertLibraryTrack(t.Context(), dbq.UpsertLibraryTrackParams{
			LibraryID: db.NullString(id), Name: id, AlbumLibraryID: db.NullString(album), NoPlayParams: 1, UpdatedAt: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.d.Q.MarkLibraryItem(t.Context(), dbq.MarkLibraryItemParams{Kind: "song", Ref: ref, Generation: 1}); err != nil {
			t.Fatal(err)
		}
	}
	addOut := func(id, date string) {
		t.Helper()
		ref, err := r.d.Q.UpsertLibraryAlbum(t.Context(), dbq.UpsertLibraryAlbumParams{LibraryID: db.NullString(id), Name: id, ReleaseDate: date, UpdatedAt: 1})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.d.Q.MarkLibraryItem(t.Context(), dbq.MarkLibraryItemParams{Kind: "album", Ref: ref, Generation: 1}); err != nil {
			t.Fatal(err)
		}
		addSong(id, id+".song")
	}
	get := func() []string {
		t.Helper()
		rows, err := r.s.o.Library.PendingReleases(t.Context(), r.now)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, p := range rows {
			ids = append(ids, p.LibraryID)
		}
		slices.Sort(ids)
		return ids
	}
	if got := get(); len(got) != 0 {
		t.Fatalf("an album out with every song published is listed: %v", got)
	}
	addOut("l.old", "2020-01-01")
	addOut("l.today", "2026-11-20")
	addOut("l.yesterday", "2026-11-19")
	addOut("l.last-week", "2026-11-13")
	addOut("l.tomorrow", "2026-11-21")
	if got, want := get(), []string{"l.today", "l.tomorrow", "l.yesterday"}; !slices.Equal(got, want) {
		t.Errorf("listed %v, want %v", got, want)
	}
}
