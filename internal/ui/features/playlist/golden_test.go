package playlist

import (
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
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

// goldenNow is the page's clock in the goldens.
var goldenNow = time.Date(2026, 11, 10, 14, 30, 0, 0, time.Local)

func goldenDetail() services.PlaylistDetail {
	d := libraryDetail("p.1", "Road Trip",
		pgSong("a", "Glass Harbour", "Mia", "Salt Flats"),
		pgSong("b", "Paper Planes", "Di", "Low Tide"),
		pgSong("c", "Blue Hour", "Al", "Lantern"),
		pgSong("d", "Mango Season", "Kay", "Salt Flats"),
		pgSong("e", "Lone Wolf", "Bo", "Single Malt"),
		pgSong("f", "Apple Orchard", "Zed", "Low Tide"))
	d.Playlist.Description = "Songs for the long way round."
	d.Playlist.LastModified = goldenNow.Add(-3 * time.Hour).UnixMilli() // "3 hr. ago"
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
			r.p.now = func() time.Time { return goldenNow }
			theme.SetMode(tc.mode)
			r.shows("Road Trip", "Glass Harbour", "Apple Orchard")
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}

// The keyboard on "Updated …" shows the exact date and time in its tooltip.
func TestPlaylistUpdatedTooltipGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{{"playlist-updated-light", theme.ModeLight}, {"playlist-updated-dark", theme.ModeDark}} {
		t.Run(tc.name, func(t *testing.T) {
			goldenFont(t)
			d := goldenDetail()
			d.Playlist.LastModified = time.Date(2026, 11, 9, 21, 30, 0, 0, time.Local).UnixMilli()
			r := newPageRig(t, "p.1", d)
			r.p.now = func() time.Time { return goldenNow }
			theme.SetMode(tc.mode)
			r.shows("Road Trip", "Updated yesterday")
			r.tt.SetFocused(true)
			for range 12 { // the actions come first
				if r.tt.Focused("Updated yesterday") {
					break
				}
				r.tt.Key(0, ui.KeyTab)
			}
			r.tt.Advance(time.Second)
			r.shows("Monday, November 9, 2026 at 9:30 PM")
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}

// The songs found and sorted: the count beside the field and the arrow of the
// sorted column.
func TestPlaylistPageFoundAndSortedGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{{"playlist-page-found-light", theme.ModeLight}, {"playlist-page-found-dark", theme.ModeDark}} {
		t.Run(tc.name, func(t *testing.T) {
			goldenFont(t)
			r := newPageRig(t, "p.1", goldenDetail())
			r.p.now = func() time.Time { return goldenNow }
			theme.SetMode(tc.mode)
			r.shows("Road Trip", "Glass Harbour", "Apple Orchard")
			r.p.page.query = "salt"
			r.p.page.sort.Key, r.p.page.sort.Desc = kit.SortTitle, true
			r.tt.Frame()
			r.shows("2 of 6 songs")
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
			// An open folder: its contents hang from a hairline.
			if err := e.tt.Click("Running"); err != nil {
				t.Fatal(err)
			}
			e.shows("Tempo")
			e.tt.Settle()
			uitest.Golden(t, e.tt.Image(), tc.name)
		})
	}
}

// golden is one page picture: the playlist in a window of width w, after act.
func golden(t *testing.T, name string, w int, mode theme.Mode, d services.PlaylistDetail, setup func(*fakeAPI), act func(*pageRig)) {
	t.Helper()
	goldenFont(t)
	rigWidth = w
	t.Cleanup(func() { rigWidth = 1132 })
	r := newPageRigWith(t, d.Playlist.ID, setup, d)
	r.p.now = func() time.Time { return goldenNow }
	theme.SetMode(mode)
	r.shows(d.Playlist.Name)
	if act != nil {
		act(r)
	}
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), name)
}

// The page at the narrowest width Sona keeps (the cover shrinks, the finder
// wraps under the facts when they do not fit), in edit mode with the
// "starts at" column following the order, with no songs and the suggestions
// as the playlist's next rows, and a catalogue playlist's small shelf of
// featured artists.
func TestPlaylistPageStatesGolden(t *testing.T) {
	t.Run("narrow", func(t *testing.T) {
		golden(t, "playlist-page-narrow-light", 707, theme.ModeLight, goldenDetail(), nil, nil)
	})
	t.Run("edit", func(t *testing.T) {
		golden(t, "playlist-edit-dark", 1132, theme.ModeDark, goldenDetail(), nil, func(r *pageRig) {
			r.click("Edit")
			r.shows("Done", "Starts")
		})
	})
	t.Run("empty", func(t *testing.T) {
		d := libraryDetail("p.1", "Road Trip")
		d.Playlist.Description = "Nothing yet."
		golden(t, "playlist-empty-light", 1132, theme.ModeLight, d, func(a *fakeAPI) {
			a.suggestions["p.1"] = []services.Song{
				pgSong("s1", "Open Road", "Neon", "Drive"),
				pgSong("s2", "Late Train", "Mira", "Night Shift"),
				pgSong("s3", "Salt Air", "Wren", "Coast"),
			}
		}, func(r *pageRig) { r.shows("This playlist is empty", "Suggested songs", "Salt Air") })
	})
	t.Run("featured", func(t *testing.T) {
		d := catalogDetail("pl.1", "Apple's Mix", goldenDetail().Tracks...)
		d.Curator = services.PlaylistCurator{Name: "Apple Music Pop"}
		d.FeaturedArtists = []services.Artist{{ID: "a1", Name: "Mia"}, {ID: "a2", Name: "Di Lemma"}, {ID: "a3", Name: "Al"}}
		d.MoreByCurator = []services.Playlist{{ID: "pl.2", Name: "Another Mix", Curator: "Apple Music Pop"}}
		golden(t, "playlist-featured-dark", 1132, theme.ModeDark, d, nil, func(r *pageRig) { r.shows("Featured artists", "Another Mix") })
	})
}
