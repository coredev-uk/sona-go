package library

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// #322: a pre-added album comes out while Sona stays open. The library is the
// real one (SQLite, LibraryService, the mapper, the sync) over a fake Apple
// that lists the album as not out until the test publishes it. The sync at
// start-up (the first paint) stores the album as it is before the release;
// Apple publishes (names, playParams, covers); nothing else is done: the
// timer the library set for the release syncs once, a little after it, and the
// Albums tile is out and playable with the songs Apple gave.
func TestPreAddedAlbumIsSyncedWhenItComesOut(t *testing.T) {
	inBubble(t, preAddedAlbumIsSyncedWhenItComesOut)
}

func preAddedAlbumIsSyncedWhenItComesOut(t *testing.T) {
	// The bubble's clock starts on 2000-01-01 and moves when the test sleeps.
	release := time.Date(2000, 1, 3, 0, 0, 0, 0, time.Local)
	apple := uitest.NewPreAddedApple(release.Format(time.DateOnly))
	r := newPageRig(t, albumsPath, 1132, 1000, func(l *library) {
		l.env.Players.Now = time.Now
		l.env.Players.Register(services.ItemAlbum, func(services.Item) {})
	}, uitest.Empty(), uitest.Apple(apple), uitest.ServiceClock(time.Now))
	uitest.PreAdded(t, r.h.DB, release.Format(time.DateOnly))
	r.l.env.Queries.Invalidate(store.KeyLibrary)
	lib := services.Service[services.LibraryService](r.h.Core)
	const play = "Play " + uitest.PreAddedName
	badge := "Out Jan 3"
	r.tt.Until("the album", r.tt.Shows(uitest.PreAddedName, "1 album", badge))

	// waitSyncs waits until Apple has been read n times and the sync ended.
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
	detail := func() services.AlbumDetail {
		t.Helper()
		d, err := lib.Album(t.Context(), uitest.PreAddedID)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	// The first paint never comes in a test: the gate opens by itself after an
	// hour, and the library, never synced, syncs. Apple still lists the album as
	// not out.
	time.Sleep(time.Hour + time.Minute)
	waitSyncs(1)
	before := detail()
	if !before.Album.Prerelease || before.Tracks[1].Name != "Track 2" || before.Tracks[1].Playable {
		t.Fatalf("before the release: prerelease %v, song %q playable %v; want the placeholder of an album not out", before.Album.Prerelease, before.Tracks[1].Name, before.Tracks[1].Playable)
	}

	// Apple publishes before the release. Nothing syncs until the release and
	// its delay (the timer's wake-ups every minute ask nothing of Apple).
	apple.Publish()
	time.Sleep(time.Until(release.Add(2*time.Minute - time.Second)))
	if n := apple.Syncs(); n != 1 {
		t.Fatalf("Apple read %d times before the release and its delay, want 1", n)
	}
	if got := detail().Tracks[1].Name; got != "Track 2" {
		t.Fatalf("the library changed without a sync: %q", got)
	}

	time.Sleep(2 * time.Second)
	waitSyncs(2)
	after := detail()
	want := []string{"Out already", "Rising", "Falling", "Single"}
	if after.Album.Prerelease {
		t.Error("the album is still a pre-release after its sync")
	}
	for i, s := range after.Tracks {
		if s.Name != want[i] || !s.Playable || s.Art.Medium == "" {
			t.Errorf("song %d after the sync: %q playable %v art %q; want %q, playable, with a cover", i+1, s.Name, s.Playable, s.Art.Medium, want[i])
		}
	}
	rows, err := r.h.DB.R.QueryContext(t.Context(), `SELECT catalog_id, no_play_params FROM tracks ORDER BY track_number`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id *string
		var none int
		if err := rows.Scan(&id, &none); err != nil {
			t.Fatal(err)
		}
		if id == nil || *id == "" || none != 0 {
			t.Errorf("a song stored with catalogue ID %v and no_play_params %d", id, none)
		} else {
			ids = append(ids, *id)
		}
	}
	if len(ids) != 4 {
		t.Errorf("catalogue IDs %v, want four", ids)
	}

	// The tile: the badge goes, and Play comes.
	r.tt.Until("the badge to go", func() bool { return !r.tt.HasText(badge) })
	r.tt.Until("Play", func() bool {
		if n, ok := r.tt.Find(uitest.PreAddedArtist); ok {
			r.tt.Move(n.X+n.W/2, n.Y-80)
			r.tt.Drain()
		}
		_, ok := r.tt.Find(play)
		return ok
	})

	// Apple has published: the timer has nothing left to wait for.
	time.Sleep(48 * time.Hour)
	if n := apple.Syncs(); n != 2 {
		t.Errorf("Apple read %d times in all, want the start-up's and the release's", n)
	}
}
