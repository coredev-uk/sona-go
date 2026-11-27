package services

import (
	"context"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/library"
)

// Sync at a release. A pre-added album's songs, covers, catalogue IDs and
// playParams reach the library only through a sync (refreshUpcomingAlbums and
// refreshUnplayableSongs ask Apple again for what is still ahead), and a
// sync runs at start-up when stale, after edits, on sign-in and on "Sync
// now". The one exception to "nothing syncs on a timer while Sona stays open"
// is this: one timer, set for the earliest release among the library's
// albums that are not out, runs the ordinary incremental sync (the entry
// point the sync after an add uses: it queues behind a running sync and
// never overlaps it) a little after that release, so Apple has published.
//
// If Apple's data still shows the album as not out after that sync (still
// flagged pre-release, or songs with no playParams), the sync is repeated at
// releaseSyncBackoff from the last one, and then no more until the next
// start-up's staleness rule (the album's stages are over).
//
// The stages of an album are fixed offsets from its release
// (releaseSyncOffsets), not counted in memory, so a restart within them
// goes on where it was: the next stage is the first one after the last sync
// (the library's oldest domain success, or the last run of this timer, whichever
// is later).
//
// The timer is recomputed after every sync, at start-up once the network
// opens (the first paint), when the library loses an item, and when signing
// in; it is cancelled at sign-out and shutdown. Go's timers do not run while
// the computer sleeps, and a clock change does not move them, so a wait is
// cut at releaseSyncCheck and the wall clock read again, as the reminders do:
// a release that passed while asleep, or when the clock jumped, syncs at the
// first wake after (the cached due time is compared, nothing is read from the
// database).
const (
	// releaseSyncDelay is how long after the release the first sync waits.
	releaseSyncDelay = 2 * time.Minute
	// releaseSyncCheck bounds one wait of the timer.
	releaseSyncCheck = time.Minute
)

// releaseSyncBackoff is how much later each repeat is than the one before.
var releaseSyncBackoff = []time.Duration{15 * time.Minute, time.Hour, 6 * time.Hour}

// releaseSyncOffsets are the times after a release at which a sync is due.
var releaseSyncOffsets = func() []time.Duration {
	out := []time.Duration{releaseSyncDelay}
	for _, b := range releaseSyncBackoff {
		out = append(out, out[len(out)-1]+b)
	}
	return out
}()

// releaseSync is LibraryService's release timer.
type releaseSync struct {
	s *LibraryService
	// after starts the timer: it calls fn once after d and returns its stop.
	// time.AfterFunc's, except in tests (a timer on the test's clock).
	after func(d time.Duration, fn func()) (stop func() bool)
	// run starts the sync: the ordinary incremental one, now or after the
	// one that runs.
	run func()

	mu      sync.Mutex
	started bool // the network is open (after the first paint)
	off     bool // signed out
	stopped bool // shut down
	stop    func() bool
	gen     uint64    // the timer armed last: an older one does nothing
	due     time.Time // when the timer syncs; zero when none is set
	last    time.Time // when the timer last started a sync
}

func newReleaseSync(s *LibraryService) *releaseSync {
	return &releaseSync{
		s:     s,
		after: func(d time.Duration, fn func()) func() bool { return time.AfterFunc(d, fn).Stop },
		run:   func() { s.syncSoon(library.AllDomains...) },
	}
}

// begin lets the timer be set: the network is open.
func (r *releaseSync) begin() {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
	r.rearm()
}

// enable follows a sign-in.
func (r *releaseSync) enable() {
	r.mu.Lock()
	r.off = false
	r.mu.Unlock()
	r.rearm()
}

// disable cancels the timer at sign-out.
func (r *releaseSync) disable() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.off = true
	r.cancelLocked()
}

// close cancels the timer for good.
func (r *releaseSync) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	r.cancelLocked()
}

func (r *releaseSync) cancelLocked() {
	r.gen++
	r.due = time.Time{}
	if r.stop != nil {
		r.stop()
		r.stop = nil
	}
}

// rearm sets the timer for the next release sync from the library as it is
// now, or cancels it when there is none: signed out, or no pre-added album
// with a stage ahead.
func (r *releaseSync) rearm() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelLocked()
	if r.stopped || r.off || !r.started || !r.s.signedIn() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), remindersTimeout)
	defer cancel()
	now := r.s.o.Now()
	due, ok, err := r.next(ctx, now)
	if err != nil {
		r.s.o.Logger.Warn("library: release sync: read albums", "err", err)
		return
	}
	if !ok {
		return
	}
	r.due = due
	r.armLocked(now)
}

// next is when the next release sync is due.
func (r *releaseSync) next(ctx context.Context, now time.Time) (time.Time, bool, error) {
	rows, err := r.s.o.Library.PendingReleases(ctx, now)
	if err != nil {
		return time.Time{}, false, err
	}
	ds, err := r.s.o.Library.SyncStatus(ctx)
	if err != nil {
		return time.Time{}, false, err
	}
	last := time.UnixMilli(syncStatus(ds, false).LastSuccess)
	if r.last.After(last) {
		last = r.last
	}
	due, ok := nextReleaseSync(rows, last, now)
	return due, ok, nil
}

// nextReleaseSync is the earliest stage after last (when the library last
// synced) of the albums in rows, by the release the UI shows for each
// (libraryRelease); an album with no full release day has no time to wait
// for.
func nextReleaseSync(rows []library.PendingRelease, last, now time.Time) (time.Time, bool) {
	var due time.Time
	for _, a := range rows {
		date, _ := libraryRelease(a.ReleaseDate, a.CatalogReleaseDate, a.Prerelease, now)
		at := releaseAt(date)
		if at == 0 {
			continue
		}
		for _, off := range releaseSyncOffsets {
			if t := time.UnixMilli(at).Add(off); t.After(last) {
				if due.IsZero() || t.Before(due) {
					due = t
				}
				break
			}
		}
	}
	return due, !due.IsZero()
}

// armLocked starts the timer for due, cut at releaseSyncCheck.
func (r *releaseSync) armLocked(now time.Time) {
	r.gen++
	gen := r.gen
	r.stop = r.after(min(max(r.due.Sub(now), 0), releaseSyncCheck), func() { r.fire(gen) })
}

// fire is the timer: it syncs when the due time has come (by the clock, not
// by the timer), and waits on otherwise.
func (r *releaseSync) fire(gen uint64) {
	r.mu.Lock()
	if gen != r.gen || r.due.IsZero() {
		r.mu.Unlock()
		return
	}
	now := r.s.o.Now()
	if now.Before(r.due) {
		r.armLocked(now)
		r.mu.Unlock()
		return
	}
	r.cancelLocked()
	r.last = now
	r.mu.Unlock()
	r.s.o.Logger.Info("library: release sync")
	r.run()
	r.rearm()
}

// rearmRelease sets the release timer again from the library; a service
// built without the timer has none to set.
func (s *LibraryService) rearmRelease() {
	if s.rel != nil {
		s.rel.rearm()
	}
}
