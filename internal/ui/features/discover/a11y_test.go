package discover

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// What assistive technology reads of Discover's pages, and the keyboard's
// way through them.

// walk is the labels of the Tab stops of the window, in order; the walk
// fails the test on a trap and on a stop that is not on the tree.
func walk(t *testing.T, r *rig) []string {
	t.Helper()
	var labels []string
	for _, s := range access.TabWalk(t, r.tt.Tester, 400) {
		labels = append(labels, s.Node.Label)
	}
	return labels
}

// inOrder fails the test unless want is a subsequence of got.
func inOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	at := 0
	for _, w := range want {
		i := slices.Index(got[at:], w)
		if i < 0 {
			t.Errorf("%q is not a Tab stop after %q; stops %q", w, got[max(0, at-1)], got)
			return
		}
		at += i + 1
	}
}

// click presses the element showing label and lets the page settle.
func (r *rig) click(label string) {
	r.t.Helper()
	if err := r.tt.Click(label); err != nil {
		r.t.Fatalf("click %q: %v (texts %q)", label, err, r.tt.Texts())
	}
	r.tt.Drain()
}

// registerItemActions gives every kind of card a menu, so that each builds
// its more button.
func (r *rig) registerItemActions() {
	for _, k := range []kit.ItemKind{kit.KindAlbum, kit.KindPlaylist, kit.KindStation, kit.KindSong} {
		r.env.Actions.Register(kit.ActionDef{
			ID: "test." + string(k), Target: k, Group: kit.GroupItem,
			Label: func(*kit.ActionContext) string { return "Do something" },
			Run:   func(*kit.ActionContext) {},
		})
	}
}

// listItems are the nodes below the list, failing the test for any that is not
// a list item: a list holds items only.
func listItems(t *testing.T, tr access.Tree, list access.Node) []access.Node {
	t.Helper()
	items := tr.Children(list)
	for _, it := range items {
		if it.Role != ui.RoleListItem {
			t.Errorf("the list %q holds %v, not a list item", list.Label, it)
		}
	}
	return items
}

// frontRig is the front page over playlists that play: sixteen cards, so that
// the list has pages.
func frontRig(t *testing.T) *rig {
	t.Helper()
	api := &fakeAPI{}
	cards := make([]services.Item, 16)
	songsOf := map[string][]services.Song{}
	for i := range cards {
		cards[i] = playlistItem(fmt.Sprint(i+1), fmt.Sprintf("List %d", i+1))
		cards[i].Wide = services.Art{Medium: bannerOf(i + 1)}
		cards[i].Subtitle = fmt.Sprintf("Curator %d", i+1)
		songsOf["playlist:"+fmt.Sprint(i+1)] = songs(2)
	}
	r := newRig(t, "/new", withAPI(api), wideWindow(), withTracks(fakeTracks{songs: songsOf}))
	r.registerItemActions()
	newPageWith(api, r.h.Clock.Now(), heroSection(cards))
	r.tt.Until("the front page", r.tt.Shows("Featured", "Play List 1"))
	r.d.front.paused = true // the rotation does not change the page under the test
	r.tt.Drain()
	r.tt.Drain()
	return r
}

func TestA11yFrontKeepsThePlayAndMoreButtonsBesideTheLeadAndEveryRow(t *testing.T) {
	r := frontRig(t)
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)

	if _, ok := tr.Find(ui.RoleGroup, "Featured"); !ok {
		t.Error("the front page is not a group named Featured")
	}
	// The lead: its opener, with the line under the title as its description, and its two buttons.
	lead, ok := tr.Find(ui.RoleLink, "List 1")
	if !ok {
		t.Fatal("no link for the lead")
	}
	if lead.Description == "" {
		t.Error("the lead's link does not carry the line under its title")
	}
	if !lead.Has(access.Focusable) {
		t.Error("the lead's link is not focusable")
	}
	// A play button and a more button for the lead and another for its row.
	if n := len(tr.FindAll(ui.RoleButton, "Play List 1")); n != 2 {
		t.Errorf("%d Play buttons for the lead, want 2 (the lead's and its row's)", n)
	}
	if n := len(tr.FindAll(ui.RoleMenuButton, "More for List 1")); n != 2 {
		t.Errorf("%d more buttons for the lead, want 2", n)
	}

	// The list holds items only, and the lead's row is the current one.
	list, ok := tr.Find(ui.RoleList, "Featured")
	if !ok {
		t.Fatal("no list of the featured cards")
	}
	items := listItems(t, tr, list)
	if len(items) == 0 {
		t.Fatal("the list is empty")
	}
	current := func(it access.Node) string {
		for _, n := range tr.Children(it) {
			if n.Role == ui.RoleButton && !strings.HasPrefix(n.Label, "Play ") {
				return n.Description
			}
		}
		return ""
	}
	if d := current(items[0]); !strings.HasPrefix(d, i18n.T("Current")) || !strings.Contains(d, "Curator 1") {
		t.Errorf("the lead's row says %q, want its line and %q", d, i18n.T("Current"))
	}
	if d := current(items[1]); strings.Contains(d, i18n.T("Current")) || !strings.Contains(d, "Curator 2") {
		t.Errorf("another row says %q", d)
	}
	// Choosing a row moves the mark, and the row keeps its name.
	x, y := r.rowCentre(2)
	r.tt.ClickAt(x, y)
	r.tt.Drain()
	tr = access.Read(t, r.tt.Tester)
	list, _ = tr.Find(ui.RoleList, "Featured")
	items = listItems(t, tr, list)
	if d := current(items[2]); !strings.Contains(d, i18n.T("Current")) {
		t.Errorf("the chosen row says %q", d)
	}
	if d := current(items[0]); strings.Contains(d, i18n.T("Current")) {
		t.Errorf("the old lead's row still says %q", d)
	}
	if _, ok := tr.Find(ui.RoleLink, "List 3"); !ok {
		t.Error("the lead did not become List 3")
	}
}

func TestA11yFrontTabWalkGoesLeadThenListThenThePagerThenOnToTheContents(t *testing.T) {
	r := frontRig(t)
	stops := walk(t, r)
	inOrder(t, stops, "List 1", "Play List 1", "List 1", "Play List 1", "List 2", "Play List 2",
		"Play featured rotation", "Previous in Featured", "Next in Featured")
	// The more buttons are not stops: Shift+F10 on a card does the same.
	if slices.Contains(stops, "More for List 1") {
		t.Error("a more button is a Tab stop")
	}
}

func TestA11yFrontRotationAndPagerAreNamedByTheirStateAndSayWhatTheUserDid(t *testing.T) {
	r := frontRig(t)
	r.d.front.paused = false
	r.tt.Drain()
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleButton, "Pause featured rotation"); !ok {
		t.Fatalf("no Pause button:\n%s", tr.Dump())
	}
	r.tt.Announcements() // what the first frames said
	r.click("Pause featured rotation")
	tr = access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleButton, "Play featured rotation"); !ok {
		t.Errorf("the button is not named by its new state:\n%s", tr.Dump())
	}
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Rotation paused"}) {
		t.Errorf("pausing said %q", got)
	}
	r.click("Next in Featured")
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"2 of 3"}) {
		t.Errorf("turning the page said %q", got)
	}
}

func TestA11yFrontCardMenuClosesWithEscapeAndTheFocusReturnsToTheCard(t *testing.T) {
	r := frontRig(t)
	r.tt.Key(0, ui.KeyTab) // the lead's link
	if !r.tt.Focused("List 1") {
		t.Fatal("the first Tab stop is not the lead")
	}
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Drain()
	if !r.d.front.menus.IsOpen() || !r.tt.HasText("Do something") {
		t.Fatalf("Shift+F10 opened no menu: %q", r.tt.Texts())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.advance(300e6)
	r.tt.Drain()
	if r.d.front.menus.IsOpen() {
		t.Fatal("Escape did not close the menu")
	}
	if !r.tt.Focused("List 1") {
		t.Error("the focus did not return to the lead's card")
	}
}

func TestA11yDiscoverReadsAsHeadingsGroupsAndListsOfItems(t *testing.T) {
	r := newRig(t, "/new", withApple(&fakeApple{t: t}), wideWindow(), withSize(1132, 1253))
	r.registerItemActions()
	r.tt.Until("the page", r.tt.Shows("On this page", "Best New Songs", "New This Week"))
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)

	for label, level := range map[string]int{
		"Discover": 1, "On this page": 2, "Songs and albums": 2, "Best New Songs": 2, "New This Week": 2, "Radio": 2, "Browse": 2,
	} {
		if h, ok := tr.Find(ui.RoleHeading, label); !ok {
			t.Errorf("no heading %q", label)
		} else if h.Level != level {
			t.Errorf("heading %q is level %d, want %d", label, h.Level, level)
		}
	}
	for _, label := range []string{"Featured", "On this page", "Songs and albums", "Radio", "Browse", "Best New Songs"} {
		if _, ok := tr.Find(ui.RoleGroup, label); !ok {
			t.Errorf("no group %q", label)
		}
	}
	// The contents: a button per band, and a list of items, each named by its shelf, per band.
	contents, ok := tr.Find(ui.RoleList, "Songs and albums")
	if !ok {
		t.Fatal("no list of the shelves of Songs and albums")
	}
	var names []string
	for _, it := range listItems(t, tr, contents) {
		names = append(names, it.Label)
		if strings.Contains(it.Label, "·") {
			t.Errorf("the separator is read in %q", it.Label)
		}
	}
	if want := []string{"Best New Songs", "New This Week"}; !slices.Equal(names, want) {
		t.Errorf("the shelves of Songs and albums are %q, want %q", names, want)
	}
	if _, ok := tr.Find(ui.RoleButton, "Songs and albums"); !ok {
		t.Error("no button jumping to the band")
	}
	// The songs shelf is a list of rows with play buttons; the albums a table of tiles.
	if _, ok := tr.Find(ui.RoleList, "Best New Songs"); !ok {
		t.Error("no list of the best new songs")
	}
	if _, ok := tr.Find(ui.RoleTable, "New This Week"); !ok {
		t.Error("no table of the new releases")
	}
	// The tiles' play buttons are on the tree at rest.
	if b, ok := tr.Find(ui.RoleButton, "Play Sessions"); !ok {
		t.Error("the tile's play button is not on the tree at rest")
	} else if !b.Has(access.Focusable) {
		t.Error("the tile's play button is not a stop")
	}

	inOrder(t, walk(t, r),
		"Pause featured rotation",
		"Songs and albums", "Best New Songs", "New This Week", "Radio", "Listen to Live Radio Now", "Browse", "New for October",
		"See all", "Sessions")
}

func TestA11yContentsJumpMovesTheFocusToANamedShelfOrBand(t *testing.T) {
	for _, name := range []string{"New This Week", "Radio"} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, "/new", withApple(&fakeApple{t: t}), wideWindow(), withSize(1132, 1253))
			r.tt.Until("the page", r.tt.Shows("On this page", "Best New Songs", "New This Week"))
			r.click(name)
			r.tt.Drain()
			r.tt.Drain()
			tr := access.Read(t, r.tt.Tester)
			if tr.Focus == nil || tr.Focus.Role != ui.RoleGroup || tr.Focus.Label != name {
				t.Errorf("after the jump the focus is %v, want the group %q", tr.Focus, name)
			}
		})
	}
}

// pages is every other page of the feature, built over its fakes and walked.
func TestA11yPagesHaveNoControlInsideAnotherAndAWalkInReadingOrder(t *testing.T) {
	tracksRoom := services.RoomPage{Title: "New This Week", Kind: services.SectionTracks, Items: songItems(0, 6), Sorts: roomSorts, Sort: "featured"}
	albumsRoom := services.RoomPage{Title: "Albums Room", Kind: services.SectionShelf, Items: albumItems(0, 4), Sorts: roomSorts, Sort: "featured"}
	curator := editorial("1", "A Curator", shelfOf("s", "Shelf", albumItem("1", "An Album")))
	curator.Art = services.Art{Large: "/art/0123456789abcdef0123456789ab0001/600x600.jpg"}
	curator.Subtitle = "Curator kind"
	campaign := editorial("9", "A Campaign", shelfOf("s", "Shelf", albumItem("1", "An Album")))
	campaign.Wide = services.Art{Large: "/art/0123456789abcdef0123456789ab0009/2400x600.jpg"}
	campaign.Subtitle, campaign.Notes = "A tagline", "What the campaign is about."

	tests := []struct {
		name     string
		build    func(t *testing.T) *rig
		ready    []string
		headings map[string]int
		stops    []string
	}{
		{
			name: "a room of songs",
			build: func(t *testing.T) *rig {
				r, _ := newRoom(t, "/room/1", tracksRoom, nil)
				return r
			},
			ready:    []string{"New This Week", "Song 0", "Order"},
			headings: map[string]int{"New This Week": 1},
			stops:    []string{"Play New This Week", "Shuffle", "Order", "Song 0"},
		},
		{
			name: "a room of albums",
			build: func(t *testing.T) *rig {
				r, _ := newRoom(t, "/room/1", albumsRoom, nil)
				return r
			},
			ready:    []string{"Albums Room", "Album 0", "Order"},
			headings: map[string]int{"Albums Room": 1},
			stops:    []string{"Order", "Album 0", "Play Album 0", "Album 1", "Play Album 1"},
		},
		{
			name: "the charts",
			build: func(t *testing.T) *rig {
				r, _ := newCharts(t, "/charts", chartsFixture())
				return r
			},
			ready:    []string{"Charts", "Top songs", "Song 9", "Genre"},
			headings: map[string]int{"Charts": 1, "Top songs": 2, "Top albums": 2},
			stops:    []string{"Genre", "Play Top songs", "Shuffle", "See all", "Song 0", "See all", "See all", "Album 0"},
		},
		{
			name: "a chart in full",
			build: func(t *testing.T) *rig {
				r, _ := newCharts(t, "/charts/songs", chartsFixture())
				return r
			},
			ready:    []string{"Top songs", "Song 9", "Genre"},
			headings: map[string]int{"Top songs": 1},
			stops:    []string{"Play Top songs", "Shuffle", "Genre", "Song 0"},
		},
		{
			name: "a curator",
			build: func(t *testing.T) *rig {
				return newRig(t, "/curator/1", withAPI(&fakeAPI{pages: map[string]services.EditorialPage{"curator/1": curator}}))
			},
			ready:    []string{"A Curator", "Curator kind", "Shelf"},
			headings: map[string]int{"A Curator": 1, "Shelf": 2},
			stops:    []string{"An Album", "Play An Album"},
		},
		{
			name: "a campaign",
			build: func(t *testing.T) *rig {
				api := &fakeAPI{pages: map[string]services.EditorialPage{"multiroom/9": campaign}}
				return newRig(t, "/multiroom/9", withAPI(api))
			},
			ready:    []string{"A Campaign", "A tagline", "Shelf"},
			headings: map[string]int{"A Campaign": 1, "Shelf": 2},
			stops:    []string{"An Album", "Play An Album"},
		},
		{
			name: "a hub",
			build: func(t *testing.T) *rig {
				api := &fakeAPI{pages: map[string]services.EditorialPage{"grouping/9": campaign}}
				return newRig(t, "/grouping/9", withAPI(api))
			},
			ready:    []string{"A Campaign", "Shelf"},
			headings: map[string]int{"A Campaign": 1},
			stops:    []string{"An Album", "Play An Album"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.build(t)
			r.tt.Until("the page", r.tt.Shows(tc.ready...))
			access.NoNestedControls(t, r.tt.Tester)
			tr := access.Read(t, r.tt.Tester)
			for label, level := range tc.headings {
				if h, ok := tr.Find(ui.RoleHeading, label); !ok {
					t.Errorf("no heading %q:\n%s", label, tr.Dump())
				} else if h.Level != level {
					t.Errorf("heading %q is level %d, want %d", label, h.Level, level)
				}
			}
			inOrder(t, walk(t, r), tc.stops...)
		})
	}
}

func TestA11yLoadingIsAStatusAndAFailureIsAnnouncedWithTheWayToTryAgain(t *testing.T) {
	for path, title := range map[string]string{
		"/new":         "Couldn't load what's new",
		"/room/1":      "Couldn't load this list",
		"/charts":      "Couldn't load the charts",
		"/curator/1":   "Couldn't load this curator",
		"/multiroom/1": "Couldn't load this collection",
		"/grouping/1":  "Couldn't load this page",
	} {
		t.Run(path, func(t *testing.T) {
			slow := &fakeAPI{block: make(chan struct{})}
			r := newRig(t, path, withAPI(slow))
			r.tt.Until("the skeleton", r.tt.Shows("Loading"))
			if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleStatus, "Loading"); !ok {
				t.Errorf("the loading page has no status labelled Loading:\n%s", access.Read(t, r.tt.Tester).Dump())
			}
			close(slow.block)

			bad := &fakeAPI{err: applemusic.ErrNotFound}
			rr := newRig(t, path, withAPI(bad))
			rr.tt.Until("the error", rr.tt.Shows(title, "Try again"))
			tr := access.Read(t, rr.tt.Tester)
			if _, ok := tr.Find(ui.RoleStatus, title); !ok {
				t.Errorf("the failure is not a status named %q:\n%s", title, tr.Dump())
			}
			if said := rr.tt.Announcements(); !slices.ContainsFunc(said, func(s string) bool { return strings.HasPrefix(s, title) }) {
				t.Errorf("the failure was announced as %q", said)
			}
			for range 3 {
				rr.tt.Drain()
			}
			if again := rr.tt.Announcements(); len(again) != 0 {
				t.Errorf("the failure was announced again: %q", again)
			}
			if got := walk(t, rr); !slices.Contains(got, "Try again") {
				t.Errorf("Try again is not a Tab stop: %q", got)
			}
		})
	}
}

func TestA11yEscapeClosesTheOrderAndGenreSelectsAndTheFocusReturnsToThem(t *testing.T) {
	room, _ := newRoom(t, "/room/1", services.RoomPage{Title: "New This Week", Kind: services.SectionTracks, Items: songItems(0, 6), Sorts: roomSorts, Sort: "featured"}, nil)
	charts, _ := newCharts(t, "/charts", chartsFixture())
	for _, tc := range []struct {
		name  string
		r     *rig
		label string
		item  string
	}{
		{"order", room, "Order", "A to Z"},
		{"genre", charts, "Genre", "Pop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.r.tt.Until("the page", tc.r.tt.Shows(tc.label))
			tr := access.Read(t, tc.r.tt.Tester)
			sel, ok := tr.Find(ui.RolePopUpButton, tc.label)
			if !ok {
				t.Fatalf("no pop-up button %q:\n%s", tc.label, tr.Dump())
			}
			if sel.Value == "" {
				t.Errorf("%q does not tell the choice it holds", tc.label)
			}
			for range 10 {
				tc.r.tt.Key(0, ui.KeyTab)
				if tc.r.tt.Focused(tc.label) {
					break
				}
			}
			if !tc.r.tt.Focused(tc.label) {
				t.Fatalf("Tab did not reach %q", tc.label)
			}
			tc.r.tt.Key(0, ui.KeyEnter)
			tc.r.tt.Drain()
			if !tc.r.tt.HasText(tc.item) {
				t.Fatalf("Enter opened no list with %q; texts %q", tc.item, tc.r.tt.Texts())
			}
			tc.r.tt.Key(0, ui.KeyEscape)
			tc.r.advance(300e6)
			tc.r.tt.Drain()
			if tc.r.tt.HasText(tc.item) {
				t.Fatal("Escape did not close the list")
			}
			if !tc.r.tt.Focused(tc.label) {
				t.Errorf("the focus did not return to %q", tc.label)
			}
		})
	}
}
