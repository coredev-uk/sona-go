package album

import (
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// #322: a library album added before its release, synced before it, comes out
// while its page is open. The library is the real one (SQLite, LibraryService,
// the mapper), its rows as a sync stores them: the catalog's date and flag, a
// song with playParams and a date, two with neither. Only the clocks move (the
// page's and the services'); no sync runs, as none does at the release. It
// pins the date flip, not what Apple will publish then, nor playing the songs
// by library ID.
func TestPreAddedLibraryAlbumComesOutAgainstTheRealLibrary(t *testing.T) {
	release := time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local)
	now := release.Add(-10 * time.Minute)
	r := newRig(t, "/album/"+uitest.PreAddedID, 1132, 1000, uitest.Empty(), uitest.ServiceClock(func() time.Time { return now }))
	uitest.PreAdded(t, r.h.DB, release.Format(time.DateOnly))
	r.now = now
	real := services.Service[services.LibraryService](r.h.Core)
	r.a.lib = real
	r.open()

	badge := kit.ReleaseBadge(services.Album{Prerelease: true, ReleaseAt: release.UnixMilli()}, now)
	r.shows(uitest.PreAddedName, "Out in 10 minutes", badge)
	r.wantNoTexts("Updating the songs", "Released")
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("announced %q before the release", got)
	}
	detail := func() services.AlbumDetail {
		t.Helper()
		d, err := real.Album(t.Context(), uitest.PreAddedID)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	playable := func(d services.AlbumDetail) (out []bool) {
		for _, s := range d.Tracks {
			out = append(out, s.Playable)
		}
		return out
	}
	if d := detail(); !d.Album.Prerelease || len(d.Tracks) != 4 || !slices.Equal(playable(d), []bool{true, false, false, true}) {
		t.Fatalf("before the release: prerelease %v, playable %v", d.Album.Prerelease, playable(d))
	}

	// Midnight: both clocks, then the page's minute tick.
	now = release.Add(time.Second)
	r.now = now
	r.tt.Advance(10*time.Minute + time.Second)
	// The library says every song is out at midnight, whether or not Apple has
	// published them: the page goes straight to an ordinary album page.
	r.tt.Until("the released page", r.tt.Shows("Released", "Play "+uitest.PreAddedName))
	r.enabled("Play " + uitest.PreAddedName)
	if d := detail(); d.Album.Prerelease || !slices.Equal(playable(d), []bool{true, true, true, true}) {
		t.Errorf("after the release: prerelease %v, playable %v, want all four out", d.Album.Prerelease, playable(d))
	}
	heard := countTexts(r.tt.Announcements(), uitest.PreAddedName+" is out now")
	// Later minutes say nothing more aloud.
	for range 3 {
		now = now.Add(clockStep + time.Second)
		r.now = now
		r.tt.Advance(clockStep + time.Second)
		r.tt.Drain()
		heard += countTexts(r.tt.Announcements(), uitest.PreAddedName+" is out now")
	}
	if heard != 1 {
		t.Errorf("announced %d times, want once", heard)
	}
	r.wantNoTexts("Out in 10 minutes", "Pre-added", "Updating the songs", "Unavailable", badge)
}
