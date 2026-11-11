package kit

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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
		{
			"cover grid", func(t *testing.T) *ui.Tester {
				_, tt := newCoverApp(t, 3, 700, 500)
				return tt
			},
			[]want{{ui.RoleTable, "Albums"}, {ui.RoleLink, "Album 0"}, {ui.RoleButton, "Play Album 0"}, {ui.RoleButton, "Play Album 2"}},
			[]string{"Album 0", "Play Album 0", "Album 1"},
		},
		{"cover shelf", func(t *testing.T) *ui.Tester {
			_, tt := newCollShelfApp(t, 3, 700)
			return tt
		}, []want{{ui.RoleHeading, "Recently Added"}, {ui.RoleLink, "Album 0"}}, []string{"Album 0", "Album 1", "Album 2"}},
		{
			"item shelf", func(t *testing.T) *ui.Tester {
				_, tt, _ := shelfApp(t, "New Releases", albumItems(5), ItemShelfOpts{})
				return tt
			},
			[]want{{ui.RoleDisclosure, "Show all 5"}, {ui.RoleLink, "Album 1"}, {ui.RoleButton, "Play Album 1"}, {ui.RoleMenuButton, "More for Album 1"}},
			[]string{"Show all 5", "Album 1", "Play Album 1", "Album 2"},
		},
		{
			"item grid", func(t *testing.T) *ui.Tester {
				_, tt := newGridApp(t, grid)
				tt.Frame()
				return tt
			},
			[]want{{ui.RoleTable, "Albums"}, {ui.RoleLink, "Album 4"}, {ui.RoleButton, "Play Album 4"}, {ui.RoleMenuButton, "More for Album 4"}},
			[]string{"Album 1", "Play Album 1", "Album 2"},
		},
		{
			"track shelf", func(t *testing.T) *ui.Tester {
				_, tt := trackApp(t, trackItems(3), "More", nil)
				return tt
			},
			[]want{{ui.RoleList, "Top Songs"}, {ui.RoleButton, "Play Song 1"}, {ui.RoleLink, "Artist 1"}, {ui.RoleMenuButton, "More for Song 1"}},
			[]string{"Song 1", "Song 2", "Song 3"},
		},
		{
			"station list", func(t *testing.T) *ui.Tester {
				_, tt, _ := stationApp(t, stations, "", nil)
				return tt
			},
			[]want{{ui.RoleList, "Local Broadcasters"}, {ui.RoleButton, "Capital Play"}, {ui.RoleMenuButton, "More for Capital"}, {ui.RoleButton, "Heart UK Play"}},
			[]string{"Capital Play", "Heart UK Play"},
		},
		{
			"link list", func(t *testing.T) *ui.Tester {
				_, tt := linkListApp(t, 500, "Explore", exploreItems())
				return tt
			},
			[]want{{ui.RoleHeading, "Explore"}, {ui.RoleList, "Explore"}, {ui.RoleLink, "Charts"}},
			[]string{"Concerts Opens in your browser", "Browse by Genre", "Charts", "Music Videos"},
		},
		{
			"wide row", func(t *testing.T) *ui.Tester {
				_, tt, _ := wideApp(t, nil, []services.Item{wideItem("1", "", "")}, 4, false)
				tt.Frame()
				return tt
			},
			[]want{{ui.RoleLink, "Item 1"}, {ui.RoleButton, "Play Item 1"}},
			[]string{"Item 1", "Play Item 1"},
		},
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

// tileCell is the cell of the tile named title: its opener's parent.
func tileCell(t *testing.T, tr access.Tree, title string) access.Node {
	t.Helper()
	op, ok := tr.Find(ui.RoleLink, title)
	if !ok {
		t.Fatalf("no tile %q on the tree:\n%s", title, tr.Dump())
	}
	cell := tr.Nodes[op.Parent]
	if cell.Role != ui.RoleCell {
		t.Fatalf("the tile %q is in a %v, not a cell", title, cell)
	}
	return cell
}

func TestCoverGridTilesSayTheirPlaceAmongAllTheTilesNotTheBuiltOnes(t *testing.T) {
	const total = 5000
	a, tt := newCoverApp(t, total, 1000, 700)
	tt.Frame()
	tt.Frame()
	if cell := tileCell(t, access.Read(t, tt), "Album 0"); cell.PosInSet != 1 || cell.SetSize != total {
		t.Errorf("the first tile is %d of %d, want 1 of %d", cell.PosInSet, cell.SetSize, total)
	}
	// Scrolled far down, a built tile is still its place in all 5000: only
	// the rows in view are built, so its row does not say it.
	tt.Scroll(500, 300, 0, 9000)
	tt.Frame()
	tt.Frame()
	last := a.ranges[len(a.ranges)-1]
	if last[0] < 100 {
		t.Fatalf("the grid built tiles %v after scrolling", last)
	}
	for _, n := range []int{last[0], (last[0] + last[1]) / 2, last[1]} {
		title := "Album " + strconv.Itoa(n)
		if cell := tileCell(t, access.Read(t, tt), title); cell.PosInSet != n+1 || cell.SetSize != total {
			t.Errorf("%q is %d of %d, want %d of %d", title, cell.PosInSet, cell.SetSize, n+1, total)
		}
	}
}

func TestCoverShelfTilesSayTheirPlaceAmongTheTilesTheShelfShows(t *testing.T) {
	for _, tc := range []struct {
		name     string
		n, width int
		cut      bool // the shelf shows fewer tiles than it was given
	}{{"all fit", 3, 700, false}, {"more than one row", 12, 700, true}} {
		t.Run(tc.name, func(t *testing.T) {
			_, tt := newCollShelfApp(t, tc.n, tc.width)
			tt.Frame()
			tt.Frame()
			cells := access.Read(t, tt).Roles(ui.RoleCell)
			if len(cells) == 0 || tc.cut != (len(cells) < tc.n) {
				t.Fatalf("the shelf built %d of %d tiles", len(cells), tc.n)
			}
			for i, cell := range cells {
				if cell.PosInSet != i+1 || cell.SetSize != len(cells) {
					t.Errorf("tile %d is %d of %d, want %d of %d", i, cell.PosInSet, cell.SetSize, i+1, len(cells))
				}
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
	if link.Description != "Artist One" {
		t.Errorf("the link is described %q, want its subtitle:\n%s", link.Description, tr.Dump())
	}
	more, ok := tr.Find(ui.RoleMenuButton, "More for Item 1")
	if !ok {
		t.Fatalf("no more button:\n%s", tr.Dump())
	}
	if tr.Within(more, ui.RoleLink) {
		t.Errorf("the more button is inside the link")
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
		{ui.RoleColumnHeader, "#"},
		{ui.RoleColumnHeader, "Duration"},
		{ui.RoleColumnHeader, "Actions"},
		{ui.RoleButton, "Title"},
		{ui.RoleButton, "Artist"},
		{ui.RoleButton, "Album"},
		{ui.RoleRow, "Song 0"},
		{ui.RoleButton, "Play Song 0"},
		{ui.RoleButton, "Play Song 3"},
		{ui.RoleLink, "Artist 0"},
		{ui.RoleLink, "Album 0"},
		{ui.RoleMenuButton, "More for Song 0"},
	})
	// The play button is pressed by assistive technology though the pointer
	// ignores it at rest.
	title, _ := tr.Find(ui.RoleButton, "Title")
	if title.Value != "" {
		t.Errorf("the Title sort button carries the value %q; the direction is the header's sort state", title.Value)
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

// TestTrackListHeadersAreColumnHeadersHoldingTheirSortButton: a sortable
// header is a column header (named by its button) with the button inside it.
func TestTrackListHeadersAreColumnHeadersHoldingTheirSortButton(t *testing.T) {
	_, tt := newCollTrackApp(t, 4, 1000, 500)
	tt.Frame()
	access.NoNestedControls(t, tt)
	tr := access.Read(t, tt)
	for _, title := range []string{"Title", "Artist", "Album"} {
		h, ok := tr.Find(ui.RoleColumnHeader, title)
		if !ok {
			t.Fatalf("no column header %q:\n%s", title, tr.Dump())
		}
		b, ok := tr.Find(ui.RoleButton, title)
		if !ok {
			t.Fatalf("no sort button %q:\n%s", title, tr.Dump())
		}
		if b.Parent != h.Index {
			t.Errorf("the %q sort button is not inside its column header:\n%s", title, tr.Dump())
		}
		if !b.Can(access.Press) {
			t.Errorf("the %q sort button cannot be pressed by assistive technology", title)
		}
	}
}

// trackSortApp is a track list whose sort state follows OnSort the way a page
// does: the same column again turns the order round.
func trackSortApp(t *testing.T, n int) (*collTrackApp, *ui.Tester) {
	t.Helper()
	a, tt := newCollTrackApp(t, n, 1000, 500)
	sort := a.opts.Sort
	sort.OnSort = func(key string) {
		a.sorted = append(a.sorted, key)
		if sort.Key == key {
			sort.Desc = !sort.Desc
			return
		}
		sort.Key, sort.Desc = key, false
	}
	return a, tt
}

// sortOf is the sort state of the column header named title, "" for none.
func sortOf(t *testing.T, tr access.Tree, title string) string {
	t.Helper()
	h, ok := tr.Find(ui.RoleColumnHeader, title)
	if !ok {
		t.Fatalf("no column header %q:\n%s", title, tr.Dump())
	}
	asc, desc, other := h.Has(access.SortAscending), h.Has(access.SortDescending), h.Has(access.SortOther)
	switch {
	case asc && !desc && !other:
		return "ascending"
	case desc && !asc && !other:
		return "descending"
	case !asc && !desc && !other:
		return ""
	}
	t.Errorf("column header %q: states %s", title, h.States)
	return "?"
}

func TestTrackListSortedHeaderReportsItsDirectionAndChangesWithAClick(t *testing.T) {
	a, tt := trackSortApp(t, 4)
	tt.Frame()
	tr := access.Read(t, tt)
	if got := [3]string{sortOf(t, tr, "Title"), sortOf(t, tr, "Artist"), sortOf(t, tr, "Album")}; got != [3]string{"ascending", "", ""} {
		t.Errorf("sort states at first (Title, Artist, Album): %q", got)
	}
	if err := tt.Click("Artist"); err != nil {
		t.Fatal(err)
	}
	tr = access.Read(t, tt)
	if got := [3]string{sortOf(t, tr, "Title"), sortOf(t, tr, "Artist"), sortOf(t, tr, "Album")}; got != [3]string{"", "ascending", ""} {
		t.Errorf("sort states after clicking Artist (Title, Artist, Album): %q (sorted %v)", got, a.sorted)
	}
	if err := tt.Click("Artist"); err != nil {
		t.Fatal(err)
	}
	tr = access.Read(t, tt)
	if got := sortOf(t, tr, "Artist"); got != "descending" {
		t.Errorf("Artist after a second click is %q, want descending (sorted %v)", got, a.sorted)
	}
}

func TestTrackListSortedHeaderChangesWithTheKeyboard(t *testing.T) {
	a, tt := trackSortApp(t, 4)
	tt.Frame()
	tt.Key(0, ui.KeyTab) // Title
	tt.Key(0, ui.KeyTab) // Artist
	tt.Key(0, ui.KeyTab) // Album
	tt.Key(0, ui.KeyEnter)
	if !slices.Equal(a.sorted, []string{SortAlbum}) {
		t.Fatalf("Enter on the focused sort button sorted %v, want Album", a.sorted)
	}
	tr := access.Read(t, tt)
	if got := [3]string{sortOf(t, tr, "Title"), sortOf(t, tr, "Artist"), sortOf(t, tr, "Album")}; got != [3]string{"", "", "ascending"} {
		t.Errorf("sort states after Enter on Album (Title, Artist, Album): %q", got)
	}
	tt.Key(0, ui.KeyEnter)
	if got := sortOf(t, access.Read(t, tt), "Album"); got != "descending" {
		t.Errorf("Album after a second Enter is %q, want descending", got)
	}
}

// The duration header sorts only for a list that asks for it, and says its
// direction like the others.
func TestTrackListDurationHeaderIsASortButtonWhenAsked(t *testing.T) {
	a, tt := trackSortApp(t, 4)
	a.opts.Sort.Duration = true
	tt.Frame()
	tr := access.Read(t, tt)
	h, ok := tr.Find(ui.RoleColumnHeader, "Duration")
	if !ok {
		t.Fatalf("no Duration column header:\n%s", tr.Dump())
	}
	b, ok := tr.Find(ui.RoleButton, "Duration")
	if !ok || b.Parent != h.Index || !b.Can(access.Press) {
		t.Fatalf("the Duration header holds no sort button:\n%s", tr.Dump())
	}
	access.NoNestedControls(t, tt)
	if got := sortOf(t, tr, "Duration"); got != "" {
		t.Errorf("Duration at first is %q", got)
	}
	if err := tt.Click("Duration"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.sorted, []string{SortDuration}) {
		t.Errorf("sorted %v, want Duration", a.sorted)
	}
	if got := sortOf(t, access.Read(t, tt), "Duration"); got != "ascending" {
		t.Errorf("Duration after a click is %q, want ascending", got)
	}
	if got := sortOf(t, access.Read(t, tt), "Title"); got != "" {
		t.Errorf("Title after a click on Duration is %q", got)
	}

	// Not asked for: a clock, no button.
	_, tt = trackSortApp(t, 4)
	tt.Frame()
	tr = access.Read(t, tt)
	if _, ok := tr.Find(ui.RoleButton, "Duration"); ok {
		t.Errorf("a list that does not sort by duration has a Duration button:\n%s", tr.Dump())
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
	for _, title := range []string{"Title", "Artist", "Album"} {
		if got := sortOf(t, tr, title); got != "" {
			t.Errorf("column header %q of a list that does not sort is %q", title, got)
		}
	}
}

func TestTrackListIsAMultiselectableTableWithItsFullSize(t *testing.T) {
	a, tt := newCollTrackApp(t, 5, 1000, 500)
	a.opts.AddedBy = func(int) *services.SocialProfile { return nil }
	tt.Frame()
	tr := access.Read(t, tt)
	table, ok := tr.Find(ui.RoleTable, "Songs in Test")
	if !ok {
		t.Fatalf("no table:\n%s", tr.Dump())
	}
	if !table.Has(access.Multiselectable) {
		t.Errorf("the track table is not multiselectable: %s", table.States)
	}
	// Six rows with the header; seven columns: #, title, artist, album,
	// added by, duration and actions.
	if table.RowCount != 6 || table.ColCount != 7 {
		t.Errorf("the table says %d rows and %d columns, want 6 and 7", table.RowCount, table.ColCount)
	}
	if h, ok := tr.Find(ui.RoleColumnHeader, "Title"); !ok || h.ColIndex != 2 {
		t.Errorf("the Title header is column %d, want 2", h.ColIndex)
	}
	if h, ok := tr.Find(ui.RoleColumnHeader, "Added by"); !ok || h.ColIndex != 5 {
		t.Errorf("the Added by header is column %d, want 5", h.ColIndex)
	}
	if h, ok := tr.Find(ui.RoleColumnHeader, "Actions"); !ok || h.ColIndex != 7 {
		t.Errorf("the Actions header is column %d, want 7", h.ColIndex)
	}
	var header access.Node
	for _, r := range tr.Roles(ui.RoleRow) {
		if r.RowIndex == 1 {
			header = r
		}
	}
	if header.Index == 0 || !tr.Within(header, ui.RoleTable) {
		t.Errorf("no header row at index 1:\n%s", tr.Dump())
	}
	for i := range 5 {
		row, ok := tr.Find(ui.RoleRow, "Song "+strconv.Itoa(i))
		if !ok || row.RowIndex != i+2 {
			t.Errorf("Song %d is row %d, want %d", i, row.RowIndex, i+2)
		}
	}
}

// A list larger than the window builds only the rows in view, and says how
// many there are and which of them each built row is.
func TestTrackListVirtualisedRowsSayTheirTrueIndexAndTheFullCount(t *testing.T) {
	const n = 2000
	_, tt := newCollTrackApp(t, n, 1000, 500)
	tt.Frame()
	tr := access.Read(t, tt)
	rows := tr.Roles(ui.RoleRow)
	if len(rows) >= n/10 {
		t.Fatalf("%d rows built for a list of %d: the list is not virtualised", len(rows), n)
	}
	table, ok := tr.Find(ui.RoleTable, "Songs in Test")
	if !ok || table.RowCount != n+1 {
		t.Fatalf("the table says %d rows, want %d (the songs and the header)", table.RowCount, n+1)
	}
	// Scroll a long way down: the rows built are a window far from the top,
	// each still saying its own place.
	tt.Scroll(500, 300, 0, 20000)
	tt.Frame()
	tr = access.Read(t, tt)
	table, _ = tr.Find(ui.RoleTable, "Songs in Test")
	if table.RowCount != n+1 {
		t.Errorf("after scrolling the table says %d rows, want %d", table.RowCount, n+1)
	}
	var first, built int
	for _, r := range tr.Roles(ui.RoleRow) {
		if r.RowIndex == 1 || r.Label == "" {
			continue // the header
		}
		i, err := strconv.Atoi(strings.TrimPrefix(r.Label, "Song "))
		if err != nil {
			t.Fatalf("row %q", r.Label)
		}
		if r.RowIndex != i+2 {
			t.Errorf("%q says it is row %d, want %d", r.Label, r.RowIndex, i+2)
		}
		if built == 0 || i < first {
			first = i
		}
		built++
	}
	if built == 0 || first < 300 {
		t.Errorf("after scrolling the first row built is Song %d of %d built; the window did not move", first, built)
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
		Select(c.Key("sel"), &choice, []SelectItem{SelectOption("a", "A")}, SelectOpts{A11yLabel: "Kind", Invalid: true, Error: "Pick another kind"})
	})
	tt.Frame()
	tr := access.Read(t, tt)
	if n, ok := tr.Find(ui.RoleTextField, "Code"); !ok || n.Description != "That code is not right" || !n.Has(access.Invalid) {
		t.Errorf("Code: %+v\n%s", n, tr.Dump())
	}
	if n, ok := tr.Find(ui.RolePopUpButton, "Kind"); !ok || !n.Has(access.Invalid) || n.Description != "Pick another kind" {
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

func TestUnavailableIsAnnouncedOncePerHold(t *testing.T) {
	a, tt := newToastApp(t)
	const reason = "Not available while listening to a live station"
	a.show = func() { Unavailable(reason) }
	for range 5 {
		_ = tt.Click("Notify")
	}
	n := 0
	for _, s := range tt.Announcements() {
		if s == reason {
			n++
		}
	}
	if n != 1 {
		t.Errorf("announced %d times, want once for a held key", n)
	}
	// Once it has gone, the next refusal says it again.
	tt.Advance(10 * time.Second)
	_ = tt.Click("Notify")
	if got := tt.Announcements(); !slices.Contains(got, reason) {
		t.Errorf("a new refusal after the toast expired is not announced: %v", got)
	}
}

func TestSelectOptionsAreOptionsOfAListBoxNamedByTheirSelect(t *testing.T) {
	f := newControlsFields()
	tt := controlsTester(t, 420, 900, f.view)
	if err := tt.Click("Plan"); err != nil {
		t.Fatal(err)
	}
	tr := access.Read(t, tt)
	requireNodes(t, tr, []want{{ui.RoleListBox, "Plan"}, {ui.RoleOption, "Free"}, {ui.RoleOption, "Pro"}, {ui.RoleOption, "Team"}})
	if n, _ := tr.Find(ui.RoleOption, "Pro"); !n.Has(access.Checked) {
		t.Errorf("the chosen option is not checked: %s", n.States)
	}
	if n, _ := tr.Find(ui.RoleOption, "Free"); n.Has(access.Checked) {
		t.Errorf("an option that is not chosen is checked")
	}
	if n, ok := tr.Find(ui.RoleOption, "Enterprise"); !ok || !n.Has(access.Disabled) {
		t.Errorf("a disabled option is not disabled: %+v", n)
	}
	// Escape closes it and the focus returns to the select.
	tt.Key(0, ui.KeyEscape)
	if !tt.Focused("Plan") {
		t.Errorf("focus did not return to the Plan select")
	}
}

// A button with a Reason is disabled for assistive technology and still has
// the focus and its place in the Tab order; it says why, and the caller is
// the one that ignores the press.
func TestButtonReasonIsDisabledButFocusableAndSaysWhy(t *testing.T) {
	const reason = "Not available while listening to a live station"
	withReason := true
	tt := controlsTester(t, 400, 200, func(c *ui.Context) {
		o := ButtonOpts{Variant: Outline, Size: SizeSM}
		if withReason {
			o.Reason = reason
		}
		Button(c.Key("next"), "Next", o)
	})
	tt.Frame()
	tr := access.Read(t, tt)
	n, ok := tr.Find(ui.RoleButton, "Next")
	if !ok || !n.Has(access.Disabled|access.Focusable) || n.Can(access.Press) || n.Description != reason {
		t.Errorf("a button with a reason: %+v", n)
	}
	if stops := stopLabels(access.TabWalk(t, tt, 4)); !slices.Equal(stops, []string{"Next"}) {
		t.Errorf("Tab stops %q, want the button", stops)
	}
	withReason = false
	tt.Frame()
	n, _ = access.Read(t, tt).Find(ui.RoleButton, "Next")
	if n.Has(access.Disabled) || !n.Can(access.Press) || n.Description != "" {
		t.Errorf("a button with no reason: %+v", n)
	}
}

// "Read more" says it controls the text it shows more of, and whether it is
// open.
func TestNotesToggleControlsItsText(t *testing.T) {
	long := strings.Repeat("A sentence that goes on and on about nothing in particular. ", 12) + "\n\nSecond paragraph."
	tt := controlsTester(t, 700, 600, func(c *ui.Context) { Notes(c, long, 0) })
	tt.Advance(100 * time.Millisecond)
	for _, open := range []bool{false, true} {
		label := "Read more"
		if open {
			label = "Show less"
		}
		tr := access.Read(t, tt)
		toggle, ok := tr.Find(ui.RoleButton, label)
		if !ok {
			t.Fatalf("no %q:\n%s", label, tr.Dump())
		}
		if toggle.Has(access.Expanded) != open {
			t.Errorf("%q: expanded %v, want %v", label, toggle.Has(access.Expanded), open)
		}
		text := tr.ControlledBy(toggle)
		if len(text) != 1 || len(tr.Children(text[0])) == 0 {
			t.Fatalf("%q controls %v, want the one group holding the text:\n%s", label, text, tr.Dump())
		}
		if slices.ContainsFunc(tr.Ancestors(toggle), func(a access.Node) bool { return a.ID == text[0].ID }) {
			t.Errorf("%q is inside what it controls", label)
		}
		if !open {
			controlsClick(t, tt, "Read more")
			tt.Advance(100 * time.Millisecond)
		}
	}
}

// An error block's message is an alert; its Try again button is beside it,
// not read out with it.
func TestErrorBlockMessageIsAnAlertWithoutItsButton(t *testing.T) {
	tt := controlsTester(t, 600, 200, func(c *ui.Context) {
		ErrorBlock(c, "Couldn't load this album", errors.New("boom"), ErrorBlockOpts{OnRetry: func() {}})
	})
	tt.Frame()
	tr := access.Read(t, tt)
	alert, ok := tr.Find(ui.RoleAlert, "Couldn't load this album")
	if !ok {
		t.Fatalf("no alert:\n%s", tr.Dump())
	}
	if retry, ok := tr.Find(ui.RoleButton, "Try again"); !ok || tr.Within(retry, ui.RoleAlert) {
		t.Errorf("Try again %+v is missing or inside the alert %v", retry, alert)
	}
	if got := tt.Announcements(); len(got) != 0 {
		t.Errorf("announced %q: an alert is read by MyGo", got)
	}
}

// TabPanel is a tab panel named by its tab, and the controls built inside
// it are on the tree under it.
func TestA11yTabPanelIsATabPanelNamedByItsTab(t *testing.T) {
	sel := 1
	labels := []string{"General", "Playback"}
	tt := controlsTester(t, 400, 200, func(c *ui.Context) {
		Tabs(c, &sel, labels, TabsOpts{A11yLabel: "Settings"})
		TabPanel(c, labels[sel]).Children(func() {
			Button(c.Key("go"), "Go", ButtonOpts{})
		})
	})
	tr := access.Read(t, tt)
	panel, ok := tr.Find(ui.RoleTabPanel, "Playback")
	if !ok {
		t.Fatalf("no tab panel named Playback:\n%s", tr.Dump())
	}
	if btn, ok := tr.Find(ui.RoleButton, "Go"); !ok || !tr.Within(btn, ui.RoleTabPanel) {
		t.Errorf("the panel's button is not inside it (panel %+v):\n%s", panel, tr.Dump())
	}
}
