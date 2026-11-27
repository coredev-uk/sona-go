package album

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// #322: the page of a pre-added library album left open while it comes out.
// The library is the real one over a fake Apple (see
// TestPreAddedAlbumIsSyncedWhenItComesOut in features/library): Apple publishes
// the songs, the library syncs once at the release by its own timer, and the
// page is out, playable and lists the songs by the names Apple gave them.
func TestPreAddedAlbumPageComesOutWithItsSongsAfterTheReleaseSync(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The bubble's clock starts on 2000-01-01 and moves when the test sleeps.
		release := time.Date(2000, 1, 3, 0, 0, 0, 0, time.Local)
		apple := uitest.NewPreAddedApple(release.Format(time.DateOnly))
		r := newRig(t, "/album/"+uitest.PreAddedID, bubbleW, bubbleH, uitest.Empty(), uitest.Apple(apple), uitest.ServiceClock(time.Now))
		uitest.PreAdded(t, r.h.DB, release.Format(time.DateOnly))
		r.now = time.Now()
		lib := services.Service[services.LibraryService](r.h.Core)
		r.a.lib = lib
		r.open()
		r.shows(uitest.PreAddedName, "Later", "Last")
		r.wantNoTexts("Released", "Rising", "Falling")

		waitSyncs := func(n int) {
			t.Helper()
			for range 1000 {
				if st, err := lib.SyncStatus(t.Context()); err != nil {
					t.Fatal(err)
				} else if apple.Syncs() == n && !st.Running {
					return
				}
				time.Sleep(time.Second)
			}
			t.Fatalf("Apple read %d times, want %d", apple.Syncs(), n)
		}

		// The start-up sync (the gate opens by itself after an hour): the album
		// is still not out, and the page shows what Apple lists.
		time.Sleep(time.Hour + time.Minute)
		waitSyncs(1)
		apple.Publish()

		// The release and its delay: one sync, by the library's timer.
		time.Sleep(time.Until(release.Add(2*time.Minute + 2*time.Second)))
		waitSyncs(2)
		r.now = time.Now()
		r.tt.Advance(clockStep + time.Second)
		r.tt.Until("the released page", r.tt.Shows("Released", "Play "+uitest.PreAddedName, "Rising", "Falling"))
		r.enabled("Play " + uitest.PreAddedName)
		r.wantNoTexts("Track 2", "Track 3", "Unavailable")
		r.idle()
	})
}
