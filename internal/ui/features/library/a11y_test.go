package library

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// a11yRoutes is every page the library's tiles and rows open, as the
// shell registers them.
func a11yRoutes(l *library) {
	l.env.HasRoute = func(p string) bool {
		for _, prefix := range []string{"/album/", "/artist/", "/playlist/", "/music-video/", "/song/"} {
			if strings.HasPrefix(p, prefix) {
				return true
			}
		}
		return false
	}
}

// inOrder fails the test unless want is a subsequence of got.
func inOrder(t *testing.T, got, want []string) {
	t.Helper()
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i < len(want) {
		t.Errorf("Tab reached %q, then not %q; the walk was %q", want[:i], want[i], got)
	}
}

// walkLabels is the names of the Tab stops, in order.
func walkLabels(t *testing.T, tt *uitest.Session, limit int) []string {
	t.Helper()
	var out []string
	for _, s := range access.TabWalk(t, tt.Tester, limit) {
		out = append(out, s.Node.Label)
	}
	return out
}

// tabTo presses Tab until the element named label has the focus.
func tabTo(t *testing.T, tt *uitest.Session, label string, limit int) {
	t.Helper()
	tt.SetFocused(true)
	for range limit {
		tt.Key(0, ui.KeyTab)
		if tt.Focused(label) {
			return
		}
	}
	t.Fatalf("Tab never reached %q in %d presses", label, limit)
}

type a11yNode struct {
	role  ui.Role
	label string
}

type a11yHeading struct {
	label string
	level int
}

// TestA11yPages is the audit of every library page and its states: every
// control is on the accessibility tree (none nested in a button or a link),
// the page's headings, lists and links are named, and Tab reaches the
// page's controls in reading order and never traps.
func TestA11yPages(t *testing.T) {
	grid := func(path string, opts ...uitest.Option) func(t *testing.T) *uitest.Session {
		return func(t *testing.T) *uitest.Session {
			r := newGridRig(t, path, func(l *library, _ *gridStub) { a11yRoutes(l) }, opts...)
			return r.tt
		}
	}
	cases := []struct {
		name     string
		build    func(t *testing.T) *uitest.Session
		ready    []string
		headings []a11yHeading
		present  []a11yNode
		absent   []a11yNode
		tab      []string
	}{
		{
			name: "recently added", build: grid(recentPath),
			ready:    []string{"Fieldnotes", "Sunday morning"},
			headings: []a11yHeading{{"Recently added", 1}},
			present: []a11yNode{
				{ui.RoleTable, "Recently added"},
				{ui.RoleLink, "Fieldnotes"},
				{ui.RoleMenuButton, "More for Fieldnotes"},
				{ui.RoleLink, "Sunday morning"},
				{ui.RoleMenuButton, "More for Sunday morning"},
			},
			tab: []string{"Fieldnotes", "Low Tide"},
		},
		{
			name: "albums", build: grid(albumsPath),
			ready:    []string{"24 albums"},
			headings: []a11yHeading{{"Albums", 1}},
			present: []a11yNode{
				{ui.RoleGroup, "Album type"},
				{ui.RoleToggleButton, "All"},
				{ui.RoleToggleButton, "Albums"},
				{ui.RoleToggleButton, "Singles & EPs"},
				{ui.RolePopUpButton, "Sort"},
				{ui.RoleTable, "Albums in your library"},
				{ui.RoleLink, "Fieldnotes"},
			},
			tab: []string{"All", "Albums", "Singles & EPs", "Sort", "Fieldnotes", "Low Tide"},
		},
		{
			name: "playlists", build: grid(playlistsPath),
			ready:    []string{"3 playlists"},
			headings: []a11yHeading{{"Playlists", 1}},
			present: []a11yNode{
				{ui.RolePopUpButton, "Sort"},
				{ui.RoleTable, "Playlists in your library"},
				{ui.RoleLink, "Sunday morning"},
				{ui.RoleMenuButton, "More for Sunday morning"},
			},
			absent: []a11yNode{{ui.RoleButton, "Play Sunday morning"}}, // a playlist plays from its page
			tab:    []string{"Sort", "Sunday morning", "Focus", "Road trip"},
		},
		{
			name: "music videos", build: func(t *testing.T) *uitest.Session {
				r, _ := newVideosRig(t, videoFixtures(3), true)
				a11yRoutes(r.l)
				return r.tt
			},
			ready:    []string{"3 music videos"},
			headings: []a11yHeading{{"Music videos", 1}},
			present: []a11yNode{
				{ui.RoleTable, "Music videos in your library"},
				{ui.RoleLink, "Video 0"},
				{ui.RoleButton, "Play Video 0"},
				{ui.RoleLink, "Video 2"},
				{ui.RoleButton, "Play Video 2"},
			},
			tab: []string{"Video 0", "Play Video 0", "Video 1", "Play Video 1", "Video 2", "Play Video 2"},
		},
		{
			name: "songs", build: func(t *testing.T) *uitest.Session {
				return newSongsRig(t, songsPath, a11yRoutes).tt
			},
			ready:    []string{"14 songs"},
			headings: []a11yHeading{{"Songs", 1}},
			present: []a11yNode{
				{ui.RolePopUpButton, "Sort"},
				{ui.RoleButton, "Play your songs"},
				{ui.RoleButton, "Shuffle"},
				{ui.RoleTable, "Songs in your library"},
				{ui.RoleButton, "Title"},
				{ui.RoleButton, "Artist"},
				{ui.RoleButton, "Album"},
				{ui.RoleButton, "Play Apple Orchard song 2"},
				{ui.RoleMenuButton, "More for Apple Orchard song 2"},
				{ui.RoleLink, "Zed"},
				{ui.RoleLink, "Apple Orchard"},
			},
			tab: []string{"Sort", "Play your songs", "Shuffle", "Title", "Artist", "Album", "Apple Orchard song 1"},
		},
		{
			name: "favourite songs", build: func(t *testing.T) *uitest.Session {
				r := newFavouritesRig(t, a11yRoutes)
				r.tt.Until("no favourites", r.tt.Shows("No favourite songs yet"))
				favouriteSong(r, 1, 0, 10)
				favouriteSong(r, 4, 1, 20)
				r.l.env.Queries.Invalidate(keyFavourites)
				return r.tt
			},
			ready:    []string{"2 songs", "Blue Hour song 2"},
			headings: []a11yHeading{{"Favourite songs", 1}},
			present: []a11yNode{
				{ui.RoleButton, "Play your favourite songs"},
				{ui.RoleButton, "Shuffle"},
				{ui.RoleTable, "Favourite songs"},
				{ui.RoleButton, "Play Blue Hour song 2"},
				{ui.RoleMenuButton, "More for Blue Hour song 2"},
			},
			tab: []string{"Play your favourite songs", "Shuffle", "Blue Hour song 2"},
		},
		{
			name: "history", build: func(t *testing.T) *uitest.Session {
				r, _, _ := historyRig(t, fixed(twoDays()))
				a11yRoutes(r.l)
				return r.tt
			},
			ready:    []string{"Today", "Yesterday", "Alpha"},
			headings: []a11yHeading{{"History", 1}, {"Today", 2}, {"Yesterday", 2}},
			present: []a11yNode{
				{ui.RoleGroup, "Today"},
				{ui.RoleTable, "Played Today"},
				{ui.RoleButton, "Play Alpha"},
				{ui.RoleTable, "Played Yesterday"},
				{ui.RoleButton, "Play Beta"},
			},
			tab: []string{"Alpha", "Beta"},
		},
		{
			name: "artists", build: func(t *testing.T) *uitest.Session {
				r := newPageRig(t, artistsPath, 1132, 900, a11yRoutes)
				r.tt.Until("the list and the detail", r.tt.Shows("Filter artists", "Albums by Dara Finch in your library"))
				return r.tt
			},
			ready:    []string{"12 artists"},
			headings: []a11yHeading{{"Artists", 1}, {"Dara Finch", 2}, {"Albums", 3}, {"Singles and EPs", 3}},
			present: []a11yNode{
				{ui.RoleComboBox, "Filter artists"},
				{ui.RoleList, "Artists in your library"},
				{ui.RoleListItem, "Dara Finch, 2 albums, 4 songs"},
				{ui.RoleButton, "Play Dara Finch"},
				{ui.RoleButton, "Shuffle"},
				{ui.RoleMenuButton, "More for Dara Finch"},
				{ui.RoleLink, "Full artist page"},
				{ui.RoleTable, "Albums by Dara Finch in your library"},
				{ui.RoleLink, "Low Tide"},
			},
			// The filter and the list keep the focus themselves: the tree
			// reports the row each has marked.
			tab: []string{"Dara Finch, 2 albums, 4 songs", "Dara Finch, 2 albums, 4 songs", "Play Dara Finch", "Shuffle", "Full artist page", "More for Dara Finch", "Low Tide", "Cold Brew"},
		},
		{
			name: "artist alone in a narrow window", build: func(t *testing.T) *uitest.Session {
				r := newPageRig(t, artistsAt("r.a2"), 700, 900, a11yRoutes)
				r.tt.Until("the artist alone", r.tt.Shows("Albums by Mira Oyelaran in your library"))
				r.tt.Until("Back has the focus", func() bool { return r.tt.Focused("Artists") })
				return r.tt
			},
			ready:    []string{"Mira Oyelaran"},
			headings: []a11yHeading{{"Mira Oyelaran", 1}, {"Albums", 2}},
			present:  []a11yNode{{ui.RoleButton, "Artists"}, {ui.RoleButton, "Play Mira Oyelaran"}},
			absent:   []a11yNode{{ui.RoleComboBox, "Filter artists"}},
			tab:      []string{"Play Mira Oyelaran", "Shuffle", "Static Honey", "Artists"},
		},
		{
			name: "signed out", build: func(t *testing.T) *uitest.Session {
				r := newPageRig(t, albumsPath, 1132, 800, a11yRoutes, uitest.Empty())
				return r.tt
			},
			ready:    []string{"Sign in to see your albums"},
			headings: []a11yHeading{{"Albums", 1}, {"Sign in to see your albums", 2}},
			present:  []a11yNode{{ui.RoleButton, "Sign in"}, {ui.RolePopUpButton, "Sort"}},
			tab:      []string{"All", "Albums", "Singles & EPs", "Sort", "Sign in"},
		},
		{
			name: "syncing", build: func(t *testing.T) *uitest.Session {
				r := newPageRig(t, songsPath, 1132, 800, func(l *library) { fakeAuth(l, accountAPI{"signed_in"}) }, uitest.Empty())
				return r.tt
			},
			ready:    []string{"Your library is syncing"},
			headings: []a11yHeading{{"Songs", 1}, {"Your library is syncing", 2}},
			present:  []a11yNode{{ui.RoleStatus, "Library sync"}, {ui.RoleButton, "Sync now"}, {ui.RoleButton, "Play your songs"}},
			tab:      []string{"Sort", "Sync now"},
		},
		{
			name: "empty favourites", build: func(t *testing.T) *uitest.Session {
				return newFavouritesRig(t, nil).tt
			},
			ready:    []string{"No favourite songs yet"},
			headings: []a11yHeading{{"Favourite songs", 1}, {"No favourite songs yet", 2}},
			present:  []a11yNode{{ui.RoleButton, "Play your favourite songs"}, {ui.RoleButton, "Shuffle"}},
			tab:      []string{"Sync now"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.build(t)
			tt.Until("the page", tt.Shows(tc.ready...))
			tt.Settle()
			access.NoNestedControls(t, tt.Tester)
			tr := access.Read(t, tt.Tester)
			for _, h := range tc.headings {
				n, ok := tr.Find(ui.RoleHeading, h.label)
				switch {
				case !ok:
					t.Errorf("no heading %q:\n%s", h.label, tr.Dump())
				case n.Level != h.level:
					t.Errorf("heading %q is level %d, want %d", h.label, n.Level, h.level)
				}
			}
			for _, n := range tc.present {
				if _, ok := tr.Find(n.role, n.label); !ok {
					t.Errorf("the tree has no role %d %q", n.role, n.label)
				}
			}
			for _, n := range tc.absent {
				if _, ok := tr.Find(n.role, n.label); ok {
					t.Errorf("the tree has role %d %q", n.role, n.label)
				}
			}
			inOrder(t, walkLabels(t, tt, 400), tc.tab)
		})
	}
}

// TestA11yPlayButtonsSitBesideTheirTiles: the more control of a
// tile or row (and the play control of those that have one) is a sibling of the link that opens it, never inside it, and
// each has a name that contains its item's.
func TestA11yPlayButtonsSitBesideTheirTiles(t *testing.T) {
	r := newGridRig(t, albumsPath, func(l *library, _ *gridStub) { a11yRoutes(l) })
	r.tt.Until("the albums", r.tt.Shows("24 albums", "Fieldnotes"))
	tr := access.Read(t, r.tt.Tester)
	link, ok := tr.Find(ui.RoleLink, "Fieldnotes")
	if !ok {
		t.Fatalf("no link Fieldnotes:\n%s", tr.Dump())
	}
	for _, label := range []string{"More for Fieldnotes"} {
		var found bool
		for _, n := range tr.Nodes {
			if n.Label == label && (n.Role == ui.RoleButton || n.Role == ui.RoleMenuButton) {
				found = true
				if n.Parent != link.Parent {
					t.Errorf("%q is not a sibling of the link Fieldnotes (parents %d, %d)", label, n.Parent, link.Parent)
				}
			}
		}
		if !found {
			t.Errorf("no control %q at rest", label)
		}
	}
}

// TestA11yItemMenuEscapeReturnsFocus: the item menu of a tile or row opens
// from the keyboard (Shift+F10: the "More" buttons are not Tab stops), and
// Escape closes it with the focus back on the tile or row.
func TestA11yItemMenuEscapeReturnsFocus(t *testing.T) {
	for _, tc := range []struct {
		name, path, opener, ready string
	}{
		{"tile", albumsPath, "Apple Orchard", "7 albums"},
		{"song row", songsPath, "Apple Orchard song 1", "14 songs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newSongsRig(t, tc.path, a11yRoutes)
			r.tt.Until("the page", r.tt.Shows(tc.ready, tc.opener))
			r.tt.Settle()
			tabTo(t, r.tt, tc.opener, 60)
			r.tt.Key(ui.Shift, ui.KeyF10)
			r.tt.Drain()
			tr := access.Read(t, r.tt.Tester)
			if len(tr.Roles(ui.RoleMenu)) == 0 || len(tr.Roles(ui.RoleMenuItem)) == 0 {
				t.Fatalf("Shift+F10 on %q opened no menu:\n%s", tc.opener, tr.Dump())
			}
			r.tt.Key(0, ui.KeyEscape)
			r.tt.Drain()
			if got := len(access.Read(t, r.tt.Tester).Roles(ui.RoleMenu)); got != 0 {
				t.Errorf("Escape left %d menus open", got)
			}
			if !r.tt.Focused(tc.opener) {
				t.Errorf("the focus did not return to %q", tc.opener)
			}
		})
	}
}

// TestA11ySortSelectEscapeReturnsFocus: the Sort select opens from the
// keyboard with its choices shown and Escape returns to it.
func TestA11ySortSelectEscapeReturnsFocus(t *testing.T) {
	r := newGridRig(t, albumsPath, nil)
	r.tt.Until("the page", r.tt.Shows("24 albums"))
	r.tt.Settle()
	tabTo(t, r.tt, "Sort", 10)
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	tr := access.Read(t, r.tt.Tester)
	if len(tr.Roles(ui.RoleListBox)) == 0 || !r.tt.HasText("Release date") {
		t.Fatalf("Enter on Sort showed no choices:\n%s", tr.Dump())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Drain()
	if !r.tt.Focused("Sort") {
		t.Error("the focus did not return to Sort")
	}
}

// TestA11yAlbumTypeIsAnnounced: choosing a type reads what the list shows
// (v0.2.0's role=status).
func TestA11yAlbumTypeIsAnnounced(t *testing.T) {
	r := newGridRig(t, albumsPath, nil)
	r.tt.Until("the page", r.tt.Shows("24 albums"))
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("the unfiltered page announced %q", got)
	}
	if err := r.tt.Click("Singles & EPs"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the singles", r.tt.Shows("Singles and EPs in your library"))
	var heard []string
	r.tt.Until("the announcement", func() bool {
		heard = append(heard, r.tt.Announcements()...)
		return len(heard) > 0
	})
	if !strings.Contains(strings.Join(heard, "|"), "single") {
		t.Errorf("announced %q, want the count of singles and EPs", heard)
	}
	tr := access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleToggleButton, "Singles & EPs"); !ok || !n.Has(access.Checked) {
		t.Errorf("the chosen type is not checked:\n%s", tr.Dump())
	}
}

// TestA11yArtistsFilter: the filter is a combobox that says its list is
// shown, hands the arrows to the list's rows, and reads out its result.
func TestA11yArtistsFilter(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 900, nil)
	artistsFilter(t, r)
	tr := access.Read(t, r.tt.Tester)
	box, ok := tr.Find(ui.RoleComboBox, "Filter artists")
	if !ok {
		t.Fatalf("no combobox:\n%s", tr.Dump())
	}
	if !box.Has(access.Expanded) {
		t.Errorf("the filter does not say its list shows: %s", box.States)
	}
	if _, ok := tr.Find(ui.RoleListItem, "Dara Finch, 2 albums, 4 songs"); !ok {
		t.Errorf("no row for Dara Finch:\n%s", tr.Dump())
	}
	r.tt.Announcements()
	r.tt.Type("zzz")
	r.tt.Until("no match", r.tt.Shows("No artist matches “zzz”."))
	tr = access.Read(t, r.tt.Tester)
	if box, _ := tr.Find(ui.RoleComboBox, "Filter artists"); box.Has(access.Expanded) {
		t.Errorf("the filter says a list shows with no match: %s", box.States)
	}
	if got := r.tt.Announcements(); len(got) == 0 || got[len(got)-1] != "No artist matches “zzz”" {
		t.Errorf("announced %q", got)
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Type("kof")
	r.tt.Until("one match", r.tt.Shows("1 of 12 artists"))
	if got := r.tt.Announcements(); len(got) == 0 || got[len(got)-1] != "1 artist matches" {
		t.Errorf("announced %q", got)
	}
}

// TestA11yArtistsDetailEscape: Escape in the artist's detail returns the
// focus to the filter (beside the list) or to Back (alone).
func TestA11yArtistsDetailEscape(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 900, nil)
	artistsFilter(t, r)
	r.tt.Type("wre")
	r.tt.Until("the filter has it", func() bool { return r.tt.HasText("1 of 12 artists") })
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the detail has the focus", func() bool { return !r.tt.Focused("Filter artists") })
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Until("the filter again", func() bool { return r.tt.Focused("Filter artists") })

	narrow := newPageRig(t, artistsAt("r.a2"), 700, 900, nil)
	narrow.tt.Until("Back has the focus", func() bool { return narrow.tt.Focused("Artists") })
	tabTo(t, narrow.tt, "Play Mira Oyelaran", 10)
	narrow.tt.Key(0, ui.KeyEscape)
	narrow.tt.Until("Back again", func() bool { return narrow.tt.Focused("Artists") })
}

// TestA11yLoadingAndErrorAreStatuses: a page still reading says so to
// assistive technology, and one that failed says what failed once.
func TestA11yLoadingAndErrorAreStatuses(t *testing.T) {
	t.Run("history loading", func(t *testing.T) {
		hold := make(chan struct{})
		t.Cleanup(func() { close(hold) })
		r, _, _ := historyRig(t, func(int, int) (services.ListeningHistory, error) {
			<-hold
			return services.ListeningHistory{}, nil
		})
		for range 3 {
			r.tt.Drain()
		}
		tr := access.Read(t, r.tt.Tester)
		if _, ok := tr.Find(ui.RoleStatus, "Loading your listening history"); !ok {
			t.Errorf("no loading status:\n%s", tr.Dump())
		}
	})
	t.Run("history error", func(t *testing.T) {
		r, _, _ := historyRig(t, func(int, int) (services.ListeningHistory, error) {
			return services.ListeningHistory{}, fmt.Errorf("unreadable")
		})
		var heard []string
		r.tt.Until("the error", func() bool {
			heard = append(heard, r.tt.Announcements()...)
			return r.tt.Shows("Couldn't read your listening history")()
		})
		tr := access.Read(t, r.tt.Tester)
		if _, ok := tr.Find(ui.RoleAlert, "Couldn't read your listening history"); !ok {
			t.Errorf("the error is not an alert:\n%s", tr.Dump())
		}
		heard = append(heard, r.tt.Announcements()...)
		if slices.ContainsFunc(heard, func(s string) bool { return strings.HasPrefix(s, "Couldn't read your listening history") }) {
			t.Errorf("announced %q: the error is an alert, read by MyGo", heard)
		}
		access.NoNestedControls(t, r.tt.Tester)
		inOrder(t, walkLabels(t, r.tt, 20), []string{"Try again", "Sync now"})
	})
	t.Run("albums error", func(t *testing.T) {
		f := &failingAlbums{}
		f.fail.Store(true)
		r := newPageRig(t, albumsPath, 1132, 800, func(l *library) { f.libraryAPI = l.lib; l.lib = f })
		var heard []string
		r.tt.Until("the error", func() bool {
			heard = append(heard, r.tt.Announcements()...)
			return r.tt.Shows("Couldn't read your albums from this computer")()
		})
		heard = append(heard, r.tt.Announcements()...)
		if slices.ContainsFunc(heard, func(s string) bool { return strings.HasPrefix(s, "Couldn't read your albums from this computer") }) {
			t.Errorf("announced %q: the error is an alert, read by MyGo", heard)
		}
		if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleAlert, "Couldn't read your albums from this computer"); !ok {
			t.Error("the error is not an alert")
		}
		inOrder(t, walkLabels(t, r.tt, 20), []string{"All", "Albums", "Singles & EPs", "Sort", "Try again"})
	})
}

// TestA11yHistoryShowEarlierDaysDisablesWhileLoading: the button is really
// disabled (no focus, no press) while the longer list loads, as v0.2.0's
// `disabled` was.
func TestA11yHistoryShowEarlierDaysDisablesWhileLoading(t *testing.T) {
	more := twoDays()
	more.More = true
	hold := make(chan struct{})
	r, _, _ := historyRig(t, func(days, _ int) (services.ListeningHistory, error) {
		if days > historyPageDays {
			<-hold
		}
		return more, nil
	})
	t.Cleanup(func() { close(hold) })
	r.tt.Until("the days", r.tt.Shows("Alpha", "Show earlier days"))
	tr := access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleButton, "Show earlier days"); !ok || n.Has(access.Disabled) {
		t.Fatalf("Show earlier days is missing or disabled:\n%s", tr.Dump())
	}
	if err := r.tt.Click("Show earlier days"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("loading", r.tt.Shows("Loading…"))
	tr = access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleButton, "Loading…"); !ok || !n.Has(access.Disabled) {
		t.Errorf("the button is not disabled while loading:\n%s", tr.Dump())
	}
}

// TestA11ySyncPhasesAreAnnounced: the sidebar's sync line read out its
// changes as a polite live region in v0.2.0: a sync starting, ending and
// failing are announced once each; the count moving is not.
func TestA11ySyncPhasesAreAnnounced(t *testing.T) {
	r := newPageRig(t, albumsPath, 1132, 800, func(l *library) { fakeAuth(l, accountAPI{"signed_in"}) })
	r.tt.Until("the page", r.tt.Shows("Library sync", "Sync now"))
	r.tt.Announcements()
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleStatus, "Library sync"); !ok {
		t.Fatalf("the sync line is not a status:\n%s", tr.Dump())
	}
	heard := func() []string {
		r.tt.Drain()
		return r.tt.Announcements()
	}
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "songs", Done: 1, Total: 10})
	if got := heard(); len(got) != 1 || got[0] != "Syncing your library" {
		t.Errorf("a sync starting announced %q", got)
	}
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "songs", Done: 5, Total: 10})
	if got := heard(); len(got) != 0 {
		t.Errorf("the count moving announced %q", got)
	}
	r.h.Env.Bus.Emit(services.EventSyncDone, services.SyncDone{OK: false, ErrorCode: "network"})
	if got := heard(); len(got) != 1 || got[0] != "Library sync failed" {
		t.Errorf("a failed sync announced %q", got)
	}
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "songs", Done: 1, Total: 10})
	r.h.Env.Bus.Emit(services.EventSyncDone, services.SyncDone{OK: true, FinishedAt: r.tt.Now().UnixMilli()})
	got := heard()
	if len(got) == 0 || !strings.HasPrefix(got[len(got)-1], "Synced ") {
		t.Errorf("a finished sync announced %q", got)
	}
}
