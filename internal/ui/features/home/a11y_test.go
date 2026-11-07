package home

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// What assistive technology reads of Home, and the keyboard's way through it.

// walk is the labels of the Tab stops of the window, in order; the walk
// fails the test on a trap and on a stop that is not on the tree.
func walk(t *testing.T, r *rig) []string {
	t.Helper()
	var labels []string
	for _, s := range access.TabWalk(t, r.tt.Tester, 300) {
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

// registerItemActions gives albums, playlists and stations a menu, so that
// every row and tile builds its more button.
func registerItemActions(r *rig) {
	for _, k := range []kit.ItemKind{kit.KindAlbum, kit.KindPlaylist, kit.KindStation} {
		r.home.env.Actions.Register(kit.ActionDef{
			ID: "test." + string(k), Target: k, Group: kit.GroupItem,
			Label: func(*kit.ActionContext) string { return "Do something" },
			Run:   func(*kit.ActionContext) {},
		})
	}
}

// itemOf is the list item (or cell) of the tree holding a node of role and
// label, and what it holds.
func itemOf(t *testing.T, tr access.Tree, list access.Node, label string) []access.Node {
	t.Helper()
	for _, it := range tr.Children(list) {
		if it.Role != ui.RoleListItem {
			t.Errorf("the list %q holds a %v, not a list item", list.Label, it)
			continue
		}
		for _, n := range tr.Children(it) {
			if n.Label == label {
				return tr.Children(it)
			}
		}
	}
	t.Fatalf("no list item of %q holds %q", list.Label, label)
	return nil
}

func TestA11yHomeHasNoControlInsideAnotherAndEverySectionIsNamed(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	registerItemActions(r)
	r.until("the page", "Top Picks for You", "Because You Like Rock")
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)

	// Headings by level: the page title, then the sections.
	for label, level := range map[string]int{
		"For you": 1, "Top Picks for You": 2, "Recently Played": 2, "Made for You": 2, "Find Your Mood": 2, "More for you": 2,
	} {
		h, ok := tr.Find(ui.RoleHeading, label)
		if !ok {
			t.Errorf("no heading %q", label)
		} else if h.Level != level {
			t.Errorf("heading %q is level %d, want %d", label, h.Level, level)
		}
	}
	// Each section is a group named by its title, and the hero card's is "Featured".
	for _, label := range []string{"Top Picks for You", "Recently Played", "Featured", "Made for You", "Find Your Mood", "More for you"} {
		if _, ok := tr.Find(ui.RoleGroup, label); !ok {
			t.Errorf("no group %q", label)
		}
	}
	// The refresh control is a button and the lead a link, both beside their play buttons.
	for role, label := range map[ui.Role]string{
		ui.RoleButton: "Refresh", ui.RoleLink: "Lead Album", ui.RoleMenuButton: "More for Lead Album",
	} {
		if _, ok := tr.Find(role, label); !ok {
			t.Errorf("no %v %q", role, label)
		}
	}
	if b, ok := tr.Find(ui.RoleButton, "Play Lead Album"); !ok || !b.Has(access.Focusable) {
		t.Errorf("the lead's Play is not a button on the tree: %v", b)
	}
	// The disclosure of an index line says whether it is open.
	line, ok := tr.Find(ui.RoleDisclosure, "Fans Also Like, 3 albums · 1 playlist")
	if !ok {
		t.Fatal("no disclosure for the index line")
	}
	if line.Has(access.Expanded) {
		t.Error("the index line is open at rest")
	}
	if len(line.Controls) != 0 {
		t.Errorf("the closed index line controls %v", line.Controls)
	}
}

// An open index line says it controls the shelf that shows under it.
func TestA11yAnOpenIndexLineControlsItsShelf(t *testing.T) {
	r := newRig(t, "/", 1132, 1600, withHome(indexPage()))
	r.until("the page", "Fans Also Like", "3 albums · 1 playlist")
	r.click("Fans Also Like")
	r.tt.Until("the shelf", r.tt.Shows("Fan 1", "Fan 3", "Fan Mix", "See all"))
	tr := access.Read(t, r.tt.Tester)
	line, ok := tr.Find(ui.RoleDisclosure, "Fans Also Like, 3 albums · 1 playlist")
	if !ok {
		t.Fatalf("no disclosure for the index line:\n%s", tr.Dump())
	}
	panel := tr.ControlledBy(line)
	if !line.Has(access.Expanded) || len(panel) != 1 || panel[0].Role != ui.RoleGroup || panel[0].Label != "Fans Also Like" {
		t.Errorf("the open index line: %+v controls %v, want the group of its shelf", line, panel)
	}
	inPanel := false
	for _, n := range tr.Nodes {
		if n.Label == "Fan 1" && slices.ContainsFunc(tr.Ancestors(n), func(a access.Node) bool { return len(panel) == 1 && a.ID == panel[0].ID }) {
			inPanel = true
		}
	}
	if !inPanel {
		t.Errorf("the shelf is not in the group the line controls:\n%s", tr.Dump())
	}
	// Closed again, it controls nothing.
	r.click("Fans Also Like")
	r.tt.Until("closed", r.gone("Fan 1", "Fan 3"))
	if line, _ := access.Read(t, r.tt.Tester).Find(ui.RoleDisclosure, "Fans Also Like, 3 albums · 1 playlist"); len(line.Controls) != 0 || line.Has(access.Expanded) {
		t.Errorf("the closed index line: %+v", line)
	}
}

func TestA11yHomeRowsAreAListOfItemsEachWithItsControlsBesideTheOpener(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	registerItemActions(r)
	r.until("the page", "Top Picks for You", "Recent One")
	tr := access.Read(t, r.tt.Tester)

	// The lead's picks: a list named by the shelf, holding only list items.
	picks, ok := tr.Find(ui.RoleList, "Top Picks for You")
	if !ok {
		t.Fatalf("no list of the picks:\n%s", tr.Dump())
	}
	if picks.Description == "" {
		t.Error("the list does not say how its rows are used")
	}
	row := itemOf(t, tr, picks, "Pick 2")
	roles := map[string]ui.Role{}
	for _, n := range row {
		roles[n.Label] = n.Role
	}
	want := map[string]ui.Role{"Pick 2": ui.RoleLink, "Play Pick 2": ui.RoleButton, "More for Pick 2": ui.RoleMenuButton}
	for label, role := range want {
		if roles[label] != role {
			t.Errorf("row of Pick 2: %q is %v, want %v; row %v", label, roles[label], role, row)
		}
	}
	// The link carries what is drawn under the name: it is one piece to assistive technology.
	link, _ := tr.Find(ui.RoleLink, "Pick 2")
	if link.Description != "Artist Pick" {
		t.Errorf("Pick 2's description is %q, want its artist", link.Description)
	}
	// Play and more are on the tree at rest, and neither is a Tab stop: the list is one.
	for _, label := range []string{"Play Pick 2", "More for Pick 2"} {
		for _, n := range row {
			if n.Label == label && n.Has(access.Focusable) {
				t.Errorf("%q is a Tab stop", label)
			}
		}
	}
	// A station with no page plays when it is opened: its button is named by its text, then the action.
	recent, _ := tr.Find(ui.RoleList, "Recently Played")
	itemOf(t, tr, recent, "Recent Station Play")
}

func TestA11yHomeKeyboardWalkReachesThePageInReadingOrderAndNeverTraps(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	registerItemActions(r)
	r.until("the page", "Top Picks for You", "Because You Like Rock")
	stops := walk(t, r)
	inOrder(t, stops,
		"Refresh", "Show all 12", "Lead Album", "Play Lead Album", "More for Lead Album",
		"Pick 2", "Recent One", "Featured Station", "Play Featured Station", "More for Featured Station",
		"See all", "Mix 1", "Play Mix 1", "Energy",
		"Indie 1", "Jazz 1",
		"Fans Also Like, 3 albums · 1 playlist", "Because You Like Rock, 2 albums")
	// The rows are one Tab stop each list: neither their play nor more buttons, nor their other rows.
	for _, label := range []string{"Play Pick 2", "More for Pick 2", "Pick 3", "Recent Three", "Play Recent One", "Focus"} {
		if slices.Contains(stops, label) {
			t.Errorf("%q is a Tab stop: the list should be one", label)
		}
	}
}

func TestA11yHomeSignedOutIsReadAndWalkedInOrder(t *testing.T) {
	r := newRig(t, "/", 1132, 1253,
		withAuth(services.AuthStatus{State: "signed_out"}),
		withLib(func(l *fakeLib) {
			l.albums = []services.Album{{ID: "a1", Name: "Shelf Album", Artist: "Shelf Artist"}}
			l.playlists = []services.Playlist{{ID: "p1", Name: "Shelf Mix", TrackCount: 12}}
		}))
	r.until("the notice", "Recommendations need Apple Music", "Shelf Album")
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)
	if h, ok := tr.Find(ui.RoleHeading, "For you"); !ok || h.Level != 1 {
		t.Errorf("no level 1 heading \"For you\": %v", h)
	}
	if h, ok := tr.Find(ui.RoleHeading, "Recommendations need Apple Music"); !ok || h.Level != 2 {
		t.Errorf("the notice has no level 2 heading: %v", h)
	}
	if _, ok := tr.Find(ui.RoleButton, "Sign in"); !ok {
		t.Error("no Sign in button")
	}
	inOrder(t, walk(t, r), "Sign in", "See all", "Shelf Album", "See all", "Shelf Mix")
}

func TestA11yHomeLoadingIsAStatusAndTheFailureIsAnnouncedOnce(t *testing.T) {
	gate := make(chan struct{})
	r := newRig(t, "/", 1132, 1000, withHome(samplePage()), withDisc(func(f *fakeDiscover) { f.homeGate = gate }))
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})
	r.until("the header", "For you")
	for range 3 {
		r.tt.Drain()
	}
	if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleStatus, "Loading"); !ok {
		t.Errorf("the loading lead is not a status labelled Loading:\n%s", access.Read(t, r.tt.Tester).Dump())
	}
	close(gate)
	r.until("the page", "Top Picks for You")

	// A failure is an alert, read by MyGo and not announced as well, with the
	// way to try again.
	bad := newRig(t, "/", 1132, 1253, withDisc(func(f *fakeDiscover) { f.err = errDown }))
	bad.until("the error", "Couldn't load your recommendations", "Try again")
	tr := access.Read(t, bad.tt.Tester)
	if _, ok := tr.Find(ui.RoleAlert, "Couldn't load your recommendations"); !ok {
		t.Errorf("the error is not an alert:\n%s", tr.Dump())
	}
	if said := bad.tt.Announcements(); slices.ContainsFunc(said, func(s string) bool { return strings.HasPrefix(s, "Couldn't load your recommendations") }) {
		t.Errorf("the failure was announced as well as being an alert: %q", said)
	}
	for range 3 {
		bad.tt.Drain()
	}
	if again := bad.tt.Announcements(); len(again) != 0 {
		t.Errorf("the failure was announced again: %q", again)
	}
	inOrder(t, walk(t, bad), "Refresh", "Try again")
}

func TestA11yRefreshKeepsTheFocusSaysItIsRefreshingAndIsDisabledWhileItRuns(t *testing.T) {
	fresh := samplePage()
	fresh.Sections[0].Title = "Fresh Picks"
	gate := make(chan struct{})
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()), withDisc(func(f *fakeDiscover) { f.refresh, f.refreshGate = fresh, gate }))
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})
	r.until("the page", "Top Picks for You", "Refresh")
	r.tt.Key(0, ui.KeyTab)
	if !r.tt.Focused("Refresh") {
		t.Fatal("the first Tab stop is not Refresh")
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("refreshing", r.tt.Shows("Refreshing…"))
	tr := access.Read(t, r.tt.Tester)
	b, ok := tr.Find(ui.RoleButton, "Refreshing…")
	if !ok || !b.Has(access.Disabled) {
		t.Errorf("the running refresh is not a disabled button: %v\n%s", b, tr.Dump())
	}
	if tr.Focus == nil || tr.Focus.Label != "Refreshing…" {
		t.Errorf("the focus left the refresh button: %v", tr.Focus)
	}
	if said := r.tt.Announcements(); !slices.Contains(said, "Refreshing…") {
		t.Errorf("the refresh was announced as %q", said)
	}
	close(gate)
	r.tt.Until("the fresh page", r.tt.Shows("Fresh Picks", "Refresh"))
	if !r.tt.Focused("Refresh") {
		t.Error("the focus did not stay on the Refresh button")
	}
}

func TestA11yShelfPageIsALabelledGridUnderItsHeadingAndLoadsAsAStatus(t *testing.T) {
	gate := make(chan struct{})
	r := newRig(t, "/for-you/made", 1132, 1000, withDisc(func(f *fakeDiscover) {
		f.shelves = map[string]services.Section{"made": shelfOf("made", "Made for You", 14)}
		f.shelfGate = gate
	}))
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})
	for range 3 {
		r.tt.Drain()
	}
	if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleStatus, "Loading"); !ok {
		t.Errorf("the loading shelf is not a status:\n%s", access.Read(t, r.tt.Tester).Dump())
	}
	close(gate)
	r.until("the shelf", "Made for You", "Shelf 1", "Shelf 14")
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)
	if h, ok := tr.Find(ui.RoleHeading, "Made for You"); !ok || h.Level != 1 {
		t.Errorf("the shelf has no level 1 heading of its title: %v\n%s", h, tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleTable, "Made for You"); !ok {
		t.Errorf("the grid is not named by the shelf's title:\n%s", tr.Dump())
	}
	inOrder(t, walk(t, r), "Shelf 1", "Play Shelf 1", "Shelf 2")
}

func TestA11yEscapeClosesARowsMenuAndTheFocusReturnsToTheRow(t *testing.T) {
	r := newRowsRig(t, moodStations(), rowsOpts{Label: "Find Your Mood", Rows: 3}, oneColumn, 600)
	var ran []string
	registerStationActions(r, &ran)
	r.tt.Frame()
	r.key(ui.KeyTab)
	r.key(ui.KeyDown)
	r.focused("Focus")
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.HasText("Do something") {
		t.Fatalf("Shift+F10 opened no menu; texts %q", r.tt.Texts())
	}
	r.key(ui.KeyEscape)
	r.tt.Advance(300e6)
	r.tt.Frame()
	if r.tt.HasText("Do something") {
		t.Fatal("Escape did not close the menu")
	}
	r.focused("Focus")
	access.NoNestedControls(t, r.tt.Tester)
}

func TestA11yEscapeClosesTheLeadsMenuAndTheFocusReturnsToItsButton(t *testing.T) {
	r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
	registerItemActions(r)
	r.until("the page", "Top Picks for You", "Because You Like Rock")
	for range 8 {
		r.tt.Key(0, ui.KeyTab)
		if r.tt.Focused("More for Lead Album") {
			break
		}
	}
	if !r.tt.Focused("More for Lead Album") {
		t.Fatal("Tab did not reach the lead's more button")
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	if !r.tt.HasText("Do something") {
		t.Fatalf("Enter opened no menu; texts %q", r.tt.Texts())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300e6)
	r.tt.Drain()
	if r.tt.HasText("Do something") {
		t.Fatal("Escape did not close the menu")
	}
	if !r.tt.Focused("More for Lead Album") {
		t.Error("the focus did not return to the lead's more button")
	}
}
