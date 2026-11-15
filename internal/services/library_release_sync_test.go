package services

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

// The library syncs at the next release of an album it holds, so the real
// songs of a pre-added album replace its placeholders with no manual sync,
// and asks again while Apple still flags the album that is out.
func TestNextReleaseSync(t *testing.T) {
	day := localDay(t, "2026-11-20")
	soon := dbq.UpcomingLibraryAlbumsRow{IsPrerelease: 1, CatalogReleaseDate: "2026-11-20"}
	tests := []struct {
		name string
		rows []dbq.UpcomingLibraryAlbumsRow
		now  time.Time
		want time.Duration
		ok   bool
	}{
		{"nothing ahead", nil, day, 0, false},
		{"the day ahead", []dbq.UpcomingLibraryAlbumsRow{soon}, day.Add(-18 * 24 * time.Hour), 18*24*time.Hour + releaseSlack, true},
		{"the nearest of two", []dbq.UpcomingLibraryAlbumsRow{{ReleaseDate: "2026-12-25"}, soon}, day.Add(-time.Hour), time.Hour + releaseSlack, true},
		{"out by the clock, still flagged", []dbq.UpcomingLibraryAlbumsRow{soon}, day.Add(3 * time.Hour), releaseRetry, true},
		{"flagged and out for good", []dbq.UpcomingLibraryAlbumsRow{soon}, day.Add(releaseGiveUp + time.Hour), 0, false},
		{"a year only", []dbq.UpcomingLibraryAlbumsRow{{IsPrerelease: 1, CatalogReleaseDate: "2027"}}, day, 0, false},
		{"flagged, no date", []dbq.UpcomingLibraryAlbumsRow{{IsPrerelease: 1}}, day, 0, false},
		{"a retry beats a later release", []dbq.UpcomingLibraryAlbumsRow{soon, {ReleaseDate: "2027-01-01"}}, day.Add(time.Hour), releaseRetry, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := nextReleaseSync(tc.rows, tc.now)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("nextReleaseSync = %v, %v; want %v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// The timer is set for at most releaseCheck and the wall clock decides when it
// fires: a computer that slept through the wait (timers do not count it) syncs
// at the first check after the release, and one that wakes early waits again.
func TestArmReleaseSyncChecksTheWallClockAndSyncsAtTheRelease(t *testing.T) {
	d := preAddedStore(t) // l.soon, out on 2026-11-20
	release := localDay(t, "2026-11-20")
	now := localDay(t, "2026-11-01")
	e := &Env{Deps: Deps{DB: d, Logger: discard(), Now: func() time.Time { return now }}}
	var waits []time.Duration
	var fire func()
	s := &LibraryService{o: e, after: func(d time.Duration, f func()) *time.Timer {
		waits, fire = append(waits, d), f
		return time.NewTimer(time.Hour)
	}}
	var synced []string
	s.syncForRelease = func(domains ...library.Domain) {
		for _, d := range domains {
			synced = append(synced, string(d))
		}
	}

	s.armReleaseSync()
	if len(waits) != 1 || waits[0] != releaseCheck || s.releaseTimer == nil {
		t.Fatalf("waits = %v, want one of %v: a long wait is made of short ones", waits, releaseCheck)
	}

	// Woken before the day: nothing syncs, and it is set again.
	now = release.Add(-time.Hour)
	fire()
	if len(synced) != 0 || len(waits) != 2 || waits[1] != releaseCheck {
		t.Fatalf("early: synced %v, waits %v", synced, waits)
	}

	// The last wait is the time left, and the first check after the day syncs
	// the albums and songs, then waits for Apple to clear the flag.
	now = release.Add(-time.Minute)
	s.armReleaseSync()
	if want := time.Minute + releaseSlack; waits[2] != want {
		t.Fatalf("last wait = %v, want %v", waits[2], want)
	}
	now = release.Add(time.Hour) // the computer slept through it
	fire()
	if len(synced) != 2 || synced[0] != "albums" || synced[1] != "songs" {
		t.Fatalf("synced %v, want the albums and songs domains", synced)
	}
	// Apple still flags it: the retry (30 minutes) is waited in checks.
	if last := waits[len(waits)-1]; len(waits) != 4 || last != releaseCheck {
		t.Fatalf("after the sync the wait is %v (%v), want a check %v while Apple still flags it", last, waits, releaseCheck)
	}

	// Long out: no timer.
	now = localDay(t, "2026-11-25")
	s.armReleaseSync()
	if len(waits) != 4 || s.releaseTimer != nil {
		t.Fatalf("waits = %v, timer = %v; want none set once the album is long out", waits, s.releaseTimer)
	}
}
