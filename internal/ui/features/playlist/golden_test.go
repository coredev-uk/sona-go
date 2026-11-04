package playlist

import (
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The playlist page and the sidebar's playlist sections, light and dark:
// the pictures that say the layout did not move by accident.
// `go test ./internal/ui/features/playlist -update` accepts a change on
// purpose.

var goldenFonts sync.Once

// goldenFont registers Sona's interface font once, before any text is laid
// out: the pictures would otherwise use whatever sans the machine has.
func goldenFont(t *testing.T) {
	t.Helper()
	goldenFonts.Do(func() {
		if err := theme.RegisterFonts(); err != nil {
			t.Fatal(err)
		}
	})
}

func goldenDetail() services.PlaylistDetail {
	d := libraryDetail("p.1", "Road Trip",
		pgSong("a", "Glass Harbour", "Mia", "Salt Flats"),
		pgSong("b", "Paper Planes", "Di", "Low Tide"),
		pgSong("c", "Blue Hour", "Al", "Lantern"),
		pgSong("d", "Mango Season", "Kay", "Salt Flats"),
		pgSong("e", "Lone Wolf", "Bo", "Single Malt"),
		pgSong("f", "Apple Orchard", "Zed", "Low Tide"))
	d.Playlist.Description = "Songs for the long way round."
	d.Playlist.LastModified = time.Now().Add(-3 * time.Hour).UnixMilli() // "3 hr. ago", whenever it runs
	return d
}

func TestPlaylistPageGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{{"playlist-page-light", theme.ModeLight}, {"playlist-page-dark", theme.ModeDark}} {
		t.Run(tc.name, func(t *testing.T) {
			goldenFont(t)
			r := newPageRig(t, "p.1", goldenDetail())
			theme.SetMode(tc.mode)
			r.shows("Road Trip", "Glass Harbour", "Apple Orchard")
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}

func TestPlaylistSidebarGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{{"playlist-sidebar-light", theme.ModeLight}, {"playlist-sidebar-dark", theme.ModeDark}} {
		t.Run(tc.name, func(t *testing.T) {
			goldenFont(t)
			e := sbSetup(t, sbLibrary(), sbPlaylistPins("p.own"))
			theme.SetMode(tc.mode)
			e.shows("Pinned", "Made for you", "Apple Music", "Playlists", "Running")
			e.tt.Settle()
			uitest.Golden(t, e.tt.Image(), tc.name)
		})
	}
}
