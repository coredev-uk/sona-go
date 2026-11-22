package library

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// #322: the Albums grid lists a pre-added album from the real library (SQLite,
// LibraryService, the mapper) with its "Out ..." badge and no Play. Nothing
// reloads the grid at the release (nothing syncs, and no timer wakes it): the
// clocks pass the date and the next frame drops the badge and offers Play.
func TestAlbumsTileOfAPreAddedAlbumComesOutAgainstTheRealLibrary(t *testing.T) {
	inBubble(t, albumsTileOfAPreAddedAlbumComesOut)
}

func albumsTileOfAPreAddedAlbumComesOut(t *testing.T) {
	// The bubble's clock starts on 2000-01-01 and moves when the test sleeps.
	release := time.Date(2000, 1, 3, 0, 0, 0, 0, time.Local)
	f := &failingAlbums{}
	r := newPageRig(t, albumsPath, 1132, 1000, func(l *library) {
		f.libraryAPI = l.lib
		l.lib = f
		l.env.Players.Now = time.Now
		l.env.Players.Register(services.ItemAlbum, func(services.Item) {})
	}, uitest.Empty(), uitest.ServiceClock(time.Now))
	uitest.PreAdded(t, r.h.DB, release.Format(time.DateOnly))
	r.l.env.Queries.Invalidate(store.KeyLibrary)
	const play = "Play " + uitest.PreAddedName
	r.tt.Until("the album", r.tt.Shows(uitest.PreAddedName, "1 album"))
	badge := "Out Jan 3"
	if !r.tt.HasText(badge) {
		t.Fatalf("no release badge before the release; texts %q", r.tt.Texts())
	}
	// A tile's Play disc shows while the tile is hovered.
	hasPlay := func() bool {
		if n, ok := r.tt.Find(uitest.PreAddedArtist); ok {
			r.tt.Move(n.X+n.W/2, n.Y-80)
			r.tt.Drain()
		}
		_, ok := r.tt.Find(play)
		return ok
	}
	if hasPlay() {
		t.Fatal("a Play disc on an album not out")
	}

	r.tt.Quiesce()
	asked := f.asks.Load()
	time.Sleep(time.Until(release) + time.Second)
	r.tt.Drain()
	r.tt.Until("the badge to go", func() bool { return !r.tt.HasText(badge) })
	r.tt.Until("Play", hasPlay)
	if n := f.asks.Load(); n != asked {
		t.Errorf("the grid read the library %d more times at the release, want none", n-asked)
	}
}
