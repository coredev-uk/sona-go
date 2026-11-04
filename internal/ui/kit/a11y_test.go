package kit

import (
	"slices"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The kit's list and item components read to assistive technology: MyGo reads
// a button, a link or a column header as one piece, so every control beside
// an opener must be an element of its own and on the tree at rest.

// want is a node the tree must hold.
type want struct {
	role  ui.Role
	label string
}

func requireNodes(t *testing.T, tr access.Tree, wants []want) {
	t.Helper()
	for _, w := range wants {
		if _, ok := tr.Find(w.role, w.label); !ok {
			t.Errorf("no role %d named %q on the tree:\n%s", w.role, w.label, tr.Dump())
		}
	}
}

// stopLabels are the labels of the Tab stops.
func stopLabels(stops []access.Stop) []string {
	out := make([]string, len(stops))
	for i, s := range stops {
		out[i] = s.Node.Label
	}
	return out
}

// inOrder reports whether want is a subsequence of got.
func inOrder(got, want []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}

func TestItemComponentsHaveEveryControlOnTheTree(t *testing.T) {
	stations := []services.Station{capital, heart}
	var grid []*services.Item
	for i := 1; i <= 4; i++ {
		it := albumItem(i)
		grid = append(grid, &it)
	}
	for _, tc := range []struct {
		name  string
		build func(t *testing.T) *ui.Tester
		nodes []want
		stops []string // Tab stops, in order (a subsequence)
	}{
		{"cover grid", func(t *testing.T) *ui.Tester {
			_, tt := newCoverApp(t, 3, 700, 500)
			return tt
		}, []want{{ui.RoleTable, "Albums"}, {ui.RoleLink, "Album 0"}, {ui.RoleButton, "Play Album 0"}, {ui.RoleButton, "Play Album 2"}},
			[]string{"Album 0", "Play Album 0", "Album 1"}},
		{"cover shelf", func(t *testing.T) *ui.Tester {
			_, tt := newCollShelfApp(t, 3, 700)
			return tt
		}, []want{{ui.RoleHeading, "Recently Added"}, {ui.RoleLink, "Album 0"}}, []string{"Album 0", "Album 1", "Album 2"}},
		{"item shelf", func(t *testing.T) *ui.Tester {
			_, tt, _ := shelfApp(t, "New Releases", albumItems(5), ItemShelfOpts{})
			return tt
		}, []want{{ui.RoleDisclosure, "Show all 5"}, {ui.RoleLink, "Album 1"}, {ui.RoleButton, "Play Album 1"}, {ui.RoleMenuButton, "More for Album 1"}},
			[]string{"Show all 5", "Album 1", "Play Album 1", "Album 2"}},
		{"item grid", func(t *testing.T) *ui.Tester {
			_, tt := newGridApp(t, grid)
			tt.Frame()
			return tt
		}, []want{{ui.RoleTable, "Albums"}, {ui.RoleLink, "Album 4"}, {ui.RoleButton, "Play Album 4"}, {ui.RoleMenuButton, "More for Album 4"}},
			[]string{"Album 1", "Play Album 1", "Album 2"}},
		{"track shelf", func(t *testing.T) *ui.Tester {
			_, tt := trackApp(t, trackItems(3), "More", nil)
			return tt
		}, []want{{ui.RoleList, "Top Songs"}, {ui.RoleButton, "Play Song 1"}, {ui.RoleLink, "Artist 1"}, {ui.RoleMenuButton, "More for Song 1"}},
			[]string{"Song 1", "Song 2", "Song 3"}},
		{"station list", func(t *testing.T) *ui.Tester {
			_, tt, _ := stationApp(t, stations, "", nil)
			return tt
		}, []want{{ui.RoleList, "Local Broadcasters"}, {ui.RoleButton, "Capital Play"}, {ui.RoleMenuButton, "More for Capital"}, {ui.RoleButton, "Heart UK Play"}},
			[]string{"Capital Play", "Heart UK Play"}},
		{"link list", func(t *testing.T) *ui.Tester {
			_, tt := linkListApp(t, 500, "Explore", exploreItems())
			return tt
		}, []want{{ui.RoleHeading, "Explore"}, {ui.RoleList, "Explore"}, {ui.RoleLink, "Charts"}},
			[]string{"Concerts Opens in your browser", "Browse by Genre", "Charts", "Music Videos"}},
		{"wide row", func(t *testing.T) *ui.Tester {
			_, tt, _ := wideApp(t, nil, []services.Item{wideItem("1", "", "")}, 4, false)
			tt.Frame()
			return tt
		}, []want{{ui.RoleLink, "Item 1"}, {ui.RoleButton, "Play Item 1"}},
			[]string{"Item 1", "Play Item 1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tt := tc.build(t)
			tt.Frame()
			access.NoNestedControls(t, tt)
			requireNodes(t, access.Read(t, tt), tc.nodes)
			stops := stopLabels(access.TabWalk(t, tt, 40))
			if !inOrder(stops, tc.stops) {
				t.Errorf("Tab stops %q, want %q in that order", stops, tc.stops)
			}
		})
	}
}

func TestWideCardsMoreButtonAndSubtitleAreReadBesideTheLink(t *testing.T) {
	it := wideItem("1", "", "")
	it.Subtitle = "Artist One"
	_, tt, _ := wideApp(t, nil, []services.Item{it}, 4, false)
	tt.Frame()
	tr := access.Read(t, tt)
	link, ok := tr.Find(ui.RoleLink, "Item 1")
	if !ok {
		t.Fatalf("no link:\n%s", tr.Dump())
	}
	if link.Description == "" && ItemSubtitle(it) != "" {
		t.Errorf("the card's subtitle is not read with the link:\n%s", tr.Dump())
	}
}

// TestTrackListControlsAreOnTheTreeAtRest is the list the kit builds every
// song list from: the play button of a row, the artist and album links and
// the sort buttons are elements of their own, found without the pointer.
func TestTrackListControlsAreOnTheTreeAtRest(t *testing.T) {
	a, tt := newCollTrackApp(t, 4, 1000, 500)
	a.opts.Menus = itemsMenus()
	a.opts.OpenAlbum = func(string) {}
	tt.Frame()
	access.NoNestedControls(t, tt)
	tr := access.Read(t, tt)
	requireNodes(t, tr, []want{
		{ui.RoleTable, "Songs in Test"},
		{ui.RoleColumnHeader, "#"}, {ui.RoleColumnHeader, "Duration"}, {ui.RoleColumnHeader, "Actions"},
		{ui.RoleButton, "Title"}, {ui.RoleButton, "Artist"}, {ui.RoleButton, "Album"},
		{ui.RoleRow, "Song 0"},
		{ui.RoleButton, "Play Song 0"}, {ui.RoleButton, "Play Song 3"},
		{ui.RoleLink, "Artist 0"}, {ui.RoleLink, "Album 0"},
		{ui.RoleMenuButton, "More for Song 0"},
	})
	// The sort buttons say the direction they sort by; the play button is
	// pressed by assistive technology though the pointer ignores it at rest.
	title, _ := tr.Find(ui.RoleButton, "Title")
	artist, _ := tr.Find(ui.RoleButton, "Artist")
	if title.Value != "ascending" || artist.Value != "none" {
		t.Errorf("sort values: Title %q, Artist %q", title.Value, artist.Value)
	}
	play, _ := tr.Find(ui.RoleButton, "Play Song 0")
	if !play.Can(access.Press) {
		t.Errorf("the Play button cannot be pressed by assistive technology: %+v", play)
	}
	// Each row's controls sit inside the row, not beside the table.
	row, _ := tr.Find(ui.RoleRow, "Song 0")
	if !tr.Within(play, ui.RoleRow) || !slices.ContainsFunc(tr.Ancestors(play), func(n access.Node) bool { return n.Index == row.Index }) {
		t.Errorf("the Play button is not inside its row:\n%s", tr.Dump())
	}
	// One Tab stop per list: the sort buttons, then the focused row; the
	// invisible buttons are not stops.
	stops := stopLabels(access.TabWalk(t, tt, 20))
	if want := []string{"Title", "Artist", "Album", "Song 0"}; !inOrder(stops, want) {
		t.Errorf("Tab stops %q, want %q", stops, want)
	}
	for _, s := range stops {
		if s == "Play Song 0" || s == "Artist 0" {
			t.Errorf("%q is a Tab stop; a list is one stop", s)
		}
	}
}

func TestTrackListUnplayableRowsHaveNoPlayButton(t *testing.T) {
	a, tt := newCollTrackApp(t, 3, 1000, 500)
	a.songs[1].Playable = false
	tt.Frame()
	tr := access.Read(t, tt)
	if _, ok := tr.Find(ui.RoleButton, "Play Song 1"); ok {
		t.Errorf("a song that cannot play offers Play:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleButton, "Play Song 0"); !ok {
		t.Errorf("a playable song has no Play button:\n%s", tr.Dump())
	}
}

func TestTrackListHeadersWithoutSortStayColumnHeaders(t *testing.T) {
	a, tt := newCollTrackApp(t, 2, 1000, 500)
	a.opts.Sort = nil
	tt.Frame()
	tr := access.Read(t, tt)
	requireNodes(t, tr, []want{{ui.RoleColumnHeader, "Title"}, {ui.RoleColumnHeader, "Artist"}, {ui.RoleColumnHeader, "Album"}})
	if got := len(tr.Roles(ui.RoleButton)); got != 2 {
		t.Errorf("%d buttons, want the two Play buttons only:\n%s", got, tr.Dump())
	}
}

func TestControlsAreNamedAndTheirStatesAreOnTheTree(t *testing.T) {
	f := newControlsFields()
	tt := controlsTester(t, 420, 900, f.view)
	tt.Frame()
	access.NoNestedControls(t, tt)
	tr := access.Read(t, tt)
	for _, w := range []struct {
		role  ui.Role
		label string
		has   access.State
		not   access.State
	}{
		{ui.RoleTextField, "Name", access.Focusable, access.Disabled | access.Invalid},
		{ui.RoleTextField, "Email", access.Invalid, access.Disabled},
		{ui.RoleTextField, "Off", access.Disabled, access.Focusable},
		{ui.RoleSwitch, "On", access.Checked, access.Disabled},
		{ui.RoleSwitch, "Off", access.Focusable, access.Checked},
		{ui.RoleSwitch, "Disabled", access.Disabled, access.Focusable},
		{ui.RoleSlider, "Volume", access.Focusable, access.Disabled},
		{ui.RoleSlider, "Off", access.Disabled, access.Focusable},
		{ui.RoleTab, "General", access.Checked, 0},
		{ui.RoleTab, "Playback", access.Focusable, access.Checked},
		{ui.RoleToggleButton, "Grid", access.Checked, 0},
		{ui.RoleToggleButton, "List", access.Focusable, access.Checked},
		{ui.RolePopUpButton, "Plan", access.Focusable, 0},
		{ui.RoleComboBox, "Country", access.Focusable, 0},
	} {
		n, ok := tr.Find(w.role, w.label)
		if !ok {
			t.Errorf("no role %d named %q:\n%s", w.role, w.label, tr.Dump())
			continue
		}
		if !n.Has(w.has) || n.States&w.not != 0 {
			t.Errorf("%s: states %s, want %s and not %s", n, n.States, w.has, w.not)
		}
	}
	if n, _ := tr.Find(ui.RoleTextField, "Email"); n.Description != "Invalid" {
		t.Errorf("an invalid field reads %q, want the default message", n.Description)
	}
	if n, _ := tr.Find(ui.RoleSlider, "Volume"); n.Description != "40%" {
		t.Errorf("the slider's value text is %q", n.Description)
	}
	requireNodes(t, tr, []want{{ui.RoleTabList, "Settings"}, {ui.RoleTabList, "Library"}, {ui.RoleProgress, "Download"}, {ui.RoleButton, "Clear search"}})
}

func TestInvalidFieldsReadTheirMessage(t *testing.T) {
	val := "x"
	tt := controlsTester(t, 400, 200, func(c *ui.Context) {
		Input(c.Key("in"), &val, InputOpts{A11yLabel: "Code", Invalid: true, Error: "That code is not right"})
		choice := "a"
		Select(c.Key("sel"), &choice, []SelectItem{SelectOption("a", "A")}, SelectOpts{A11yLabel: "Kind", Invalid: true})
	})
	tt.Frame()
	tr := access.Read(t, tt)
	if n, ok := tr.Find(ui.RoleTextField, "Code"); !ok || n.Description != "That code is not right" || !n.Has(access.Invalid) {
		t.Errorf("Code: %+v\n%s", n, tr.Dump())
	}
	if n, ok := tr.Find(ui.RolePopUpButton, "Kind"); !ok || !n.Has(access.Invalid) {
		t.Errorf("Kind: %+v\n%s", n, tr.Dump())
	}
}

func TestOverlaysAreNamedDialogsAndStatuses(t *testing.T) {
	t.Run("dialog", func(t *testing.T) {
		a, tt := newDialogApp(t, DialogOpts{Title: "Rename playlist", Description: "Pick a new name."})
		a.open = true
		tt.Frame()
		tr := access.Read(t, tt)
		d, ok := tr.Find(ui.RoleDialog, "Rename playlist")
		if !ok || d.Description != "Pick a new name." {
			t.Fatalf("no named dialog:\n%s", tr.Dump())
		}
		requireNodes(t, tr, []want{{ui.RoleHeading, "Rename playlist"}, {ui.RoleButton, "Save"}, {ui.RoleButton, "Close"}})
		if _, ok := tr.Find(ui.RoleButton, "Behind"); ok {
			t.Errorf("the page behind a modal dialog is on the tree:\n%s", tr.Dump())
		}
		if stops := stopLabels(access.TabWalk(t, tt, 10)); !slices.Contains(stops, "Save") || slices.Contains(stops, "Behind") {
			t.Errorf("Tab stops in the dialog: %q", stops)
		}
	})
	t.Run("confirm", func(t *testing.T) {
		a, tt := newConfirmApp(t)
		_ = tt.Click("Delete…")
		tr := access.Read(t, tt)
		if _, ok := tr.Find(ui.RoleAlertDialog, a.o.Title); !ok {
			t.Errorf("a confirmation is not an alert dialog:\n%s", tr.Dump())
		}
	})
	t.Run("toast", func(t *testing.T) {
		a, tt := newToastApp(t)
		a.show = func() { ToastErrorf("Couldn't favourite “Airbag”", "Check your connection.") }
		_ = tt.Click("Notify")
		tr := access.Read(t, tt)
		if _, ok := tr.Find(ui.RoleGroup, "Notifications"); !ok {
			t.Errorf("no Notifications region:\n%s", tr.Dump())
		}
		if got := tr.Labelled(ui.RoleStatus, "Couldn't favourite"); len(got) != 1 {
			t.Errorf("the toast is not one status:\n%s", tr.Dump())
		}
	})
	t.Run("menu", func(t *testing.T) {
		a, tt := newMenuApp(t)
		_ = tt.Click("Open")
		tr := access.Read(t, tt)
		requireNodes(t, tr, []want{{ui.RoleMenu, "Test menu"}, {ui.RoleMenuItem, "Play next"}, {ui.RoleMenuItem, "Delete"}})
		if n, _ := tr.Find(ui.RoleMenuItem, "Add to queue"); !n.Has(access.Disabled) || n.Description == "" {
			t.Errorf("an unavailable item says why: %+v", n)
		}
		// Escape closes the menu and the focus returns to its trigger.
		tt.Key(0, ui.KeyEscape)
		if a.menu.IsOpen() || !tt.Focused("Open") {
			t.Errorf("menu open %v, trigger focused %v", a.menu.IsOpen(), tt.Focused("Open"))
		}
	})
}

func TestEveryTrackShelfRowIsAListItemNamedByTheSong(t *testing.T) {
	_, tt := trackApp(t, trackItems(3), "", nil)
	tr := access.Read(t, tt)
	items := tr.Roles(ui.RoleListItem)
	if len(items) != 3 {
		t.Fatalf("%d list items:\n%s", len(items), tr.Dump())
	}
	for i, it := range items {
		if name := "Song " + strconv.Itoa(i+1); it.Label != name {
			t.Errorf("row %d is named %q, want %q", i+1, it.Label, name)
		}
	}
}

func TestConfirmReturnsTheFocusToItsOpener(t *testing.T) {
	for _, tc := range []struct {
		name string
		do   func(tt *ui.Tester)
	}{
		{"Escape", func(tt *ui.Tester) { tt.Key(0, ui.KeyEscape) }},
		{"Cancel", func(tt *ui.Tester) {
			tt.Key(0, ui.KeyTab) // Delete -> (wraps in the dialog) Cancel
			tt.Key(0, ui.KeyEnter)
		}},
		{"confirming", func(tt *ui.Tester) { tt.Key(0, ui.KeyEnter) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, tt := newConfirmApp(t)
			tt.Key(0, ui.KeyTab)
			if !tt.Focused("Delete…") {
				t.Fatal("the opener is not focused")
			}
			tt.Key(0, ui.KeyEnter)
			if !ConfirmPending() || !tt.Focused("Delete") {
				t.Fatalf("the question did not open with the confirm button focused: %q", tt.Texts())
			}
			tc.do(tt)
			tt.Advance(500e6)
			if len(a.answers) != 1 || ConfirmPending() {
				t.Fatalf("answers %v, pending %v", a.answers, ConfirmPending())
			}
			if !tt.Focused("Delete…") {
				t.Errorf("focus did not return to the button that asked: focus %v", access.Read(t, tt).Focus)
			}
		})
	}
}
