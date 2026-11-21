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
			e.shows("Pinned", "Apple Music", "Playlists", "Running")
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
// wraps under the facts when they do not fit), in edit mode, with no
// songs and the suggestions
// as the playlist's next rows, and a catalogue playlist's small shelf of
// featured artists.
func TestPlaylistPageStatesGolden(t *testing.T) {
	t.Run("narrow", func(t *testing.T) {
		golden(t, "playlist-page-narrow-light", 707, theme.ModeLight, goldenDetail(), nil, nil)
	})
	t.Run("edit", func(t *testing.T) {
		golden(t, "playlist-edit-dark", 1132, theme.ModeDark, goldenDetail(), nil, func(r *pageRig) {
			r.click("Edit")
			r.shows("Done", "Album")
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
		}, func(r *pageRig) {
			r.shows("This playlist is empty", "Suggested Songs", "Show suggestions")
			r.click("Show suggestions")
			r.shows("Salt Air")
		})
	})
	t.Run("featured", func(t *testing.T) {
		d := catalogDetail("pl.1", "Apple's Mix", goldenDetail().Tracks...)
		d.Curator = services.PlaylistCurator{Name: "Apple Music Pop"}
		d.FeaturedArtists = []services.Artist{{ID: "a1", Name: "Mia"}, {ID: "a2", Name: "Di Lemma"}, {ID: "a3", Name: "Al"}}
		d.MoreByCurator = []services.Playlist{{ID: "pl.2", Name: "Another Mix", Curator: "Apple Music Pop"}}
		golden(t, "playlist-featured-dark", 1132, theme.ModeDark, d, nil, func(r *pageRig) { r.shows("Featured Artists", "Another Mix") })
	})
}

func goldenSuggestions() []services.Song {
	return []services.Song{
		pgSong("s1", "Open Road", "Neon", "Drive"),
		pgSong("s2", "Late Train", "Mira", "Night Shift"),
		pgSong("s3", "Salt Air", "Wren", "Coast"),
	}
}

// The Suggested songs shelf under a playlist with songs: closed as the page
// opens, open with its list and Refresh, and empty after the last song was
// added (or none came), in both modes.
func TestPlaylistSuggestionsGolden(t *testing.T) {
	for _, m := range []struct {
		suffix string
		mode   theme.Mode
	}{{"light", theme.ModeLight}, {"dark", theme.ModeDark}} {
		t.Run("collapsed-"+m.suffix, func(t *testing.T) {
			golden(t, "playlist-suggestions-collapsed-"+m.suffix, 1132, m.mode, goldenDetail(), func(a *fakeAPI) {
				a.suggestions["p.1"] = goldenSuggestions()
			}, func(r *pageRig) { r.shows("Suggested Songs", "Show suggestions") })
		})
		t.Run("open-"+m.suffix, func(t *testing.T) {
			golden(t, "playlist-suggestions-open-"+m.suffix, 1132, m.mode, goldenDetail(), func(a *fakeAPI) {
				a.suggestions["p.1"] = goldenSuggestions()
			}, func(r *pageRig) {
				r.openSuggestions()
				r.shows("Salt Air", "Refresh")
			})
		})
		t.Run("empty-"+m.suffix, func(t *testing.T) {
			golden(t, "playlist-suggestions-empty-"+m.suffix, 1132, m.mode, goldenDetail(), nil, func(r *pageRig) {
				r.openSuggestions()
				r.shows("No suggestions right now", "Refresh")
			})
		})
	}
}

// The header of a collaborative playlist the user joined: "Added …" with no
// "Updated" (Apple's date is from before they joined), not "Created".
func TestPlaylistJoinedCollaborativeGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{{"playlist-joined-light", theme.ModeLight}, {"playlist-joined-dark", theme.ModeDark}} {
		t.Run(tc.name, func(t *testing.T) {
			goldenFont(t)
			d := goldenDetail()
			d.Playlist.CatalogID, d.Playlist.CanEdit = "pl.u-9", false
			d.Playlist.DateAdded = time.Date(2024, 9, 8, 10, 0, 0, 0, time.Local).UnixMilli()
			d.Playlist.LastModified = time.Date(2023, 5, 1, 10, 0, 0, 0, time.Local).UnixMilli()
			r := newPageRigWith(t, "p.1", func(api *fakeAPI) {
				api.collab["p.1"] = services.PlaylistCollaboration{
					Collaborative: true,
					OwnerID:       "sp.1",
					Collaborators: []services.SocialProfile{
						{ID: "sp.1", Name: "Ada", Handle: "ada"},
						{ID: "sp.2", Name: "Bo", Handle: "bo"},
					},
					Viewer: services.RoleCollaborator,
				}
			}, d)
			r.p.now = func() time.Time { return goldenNow }
			theme.SetMode(tc.mode)
			r.shows("Road Trip", "Collaborative playlist", "Added September 8, 2024")
			if r.tt.HasText("Updated") {
				t.Errorf("an update from before the join shows: %q", r.tt.Texts())
			}
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}

// "Add to playlist" in a song's menu is a submenu of New playlist… and the
// playlists, a folder opening another beside it.
func TestPlaylistAddMenuGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{{"playlist-add-menu-light", theme.ModeLight}, {"playlist-add-menu-dark", theme.ModeDark}} {
		t.Run(tc.name, func(t *testing.T) {
			goldenFont(t)
			r := newPageRigWith(t, "p.1", func(api *fakeAPI) {
				api.tree = services.PlaylistTree{
					Folders: []services.PlaylistFolder{folder("pf.1", "Running", "", 0), folder("pf.2", "Summer", "pf.1", 0)},
					Playlists: []services.TreePlaylist{
						treePlaylist("p.1", "Sunday morning", "", 0, true),
						treePlaylist("p.2", "Late night", "", 1, true),
						treePlaylist("p.3", "Focus", "", 2, true),
						treePlaylist("p.4", "Tempo", "pf.1", 0, true),
						treePlaylist("p.5", "Long run", "pf.1", 1, true),
						treePlaylist("p.6", "Hot lap", "pf.2", 0, true),
					},
				}
				api.recents = []string{"p.2", "p.4"}
			}, goldenDetail())
			r.p.now = func() time.Time { return goldenNow }
			theme.SetMode(tc.mode)
			r.shows("Road Trip", "Glass Harbour", "Apple Orchard")
			if err := r.tt.RightClick("Paper Planes"); err != nil {
				t.Fatal(err)
			}
			r.shows("Add to playlist")
			r.did("Tree")
			r.did("RecentTargets")
			r.tt.Settle()
			r.hover("Add to playlist")
			r.shows("New playlist…", "Late night", "Running")
			r.hover("Running")
			r.shows("Summer", "Long run")
			r.tt.Advance(400 * time.Millisecond)
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}
