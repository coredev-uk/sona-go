package concerts

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

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

// TestA11yPages is the audit of the Concerts page and its states, and of
// the shelves the other pages show: no control nested in a button or a
// link, headings, lists and links named, Tab reaching the controls in
// reading order and never trapping.
func TestA11yPages(t *testing.T) {
	const tickets1 = "Get tickets for Example Band with Opener Duo from Example Tickets, opens in your browser"
	cases := []struct {
		name     string
		build    func(t *testing.T) *rig
		ready    []string
		headings []a11yHeading
		present  []a11yNode
		tab      []string
	}{
		{
			name: "agenda",
			build: func(t *testing.T) *rig {
				return newRig(t, "/concerts", 1132, 1000, exampleBand, sampleSinger, anotherAct)
			},
			ready:    []string{"April 2027", "June 2027", "Another Act"},
			headings: []a11yHeading{{"Concerts", 1}, {"April 2027", 2}, {"June 2027", 2}},
			present: []a11yNode{
				{ui.RoleButton, "Set location"},
				{ui.RoleGroup, "Filter concerts"},
				{ui.RolePopUpButton, "Dates"},
				{ui.RolePopUpButton, "Genres"},
				{ui.RoleSwitch, "Your artists (2)"},
				{ui.RoleGroup, "April 2027"},
				{ui.RoleList, "Wed, Apr 14"},
				{ui.RoleList, "Tue, Jun 1"},
				{ui.RoleLink, "Example Band with Opener Duo"},
				{ui.RoleButton, tickets1},
				{ui.RoleLink, "Sample Singer"},
				{ui.RoleMenuButton, "Get tickets for Sample Singer"},
				{ui.RoleLink, "Another Act"},
			},
			// The whole agenda is one Tab stop: its first control.
			tab: []string{"Set location", "Dates", "Genres", "Your artists (2)", "Example Band with Opener Duo"},
		},
		{
			name: "date range",
			build: func(t *testing.T) *rig {
				return newRig(t, "/concerts?when=dates&from=2027-01-01", 1132, 1000, exampleBand, sampleSinger, anotherAct)
			},
			ready:   []string{"Choose both dates", "April 2027"},
			present: []a11yNode{{ui.RoleTextField, "From"}, {ui.RoleTextField, "To"}},
			tab:     []string{"Set location", "Dates", "From", "To", "Genres", "Your artists (2)", "Example Band with Opener Duo"},
		},
		{
			name: "nothing matches",
			build: func(t *testing.T) *rig {
				return newRig(t, "/concerts?when=week", 1132, 1000, exampleBand, sampleSinger, anotherAct)
			},
			ready:    []string{"No concerts match", "Clear filters"},
			headings: []a11yHeading{{"Concerts", 1}, {"No concerts match", 2}},
			present:  []a11yNode{{ui.RoleButton, "Clear filters"}},
			tab:      []string{"Set location", "Dates", "Clear filters"},
		},
		{
			name: "nothing near the city",
			build: func(t *testing.T) *rig {
				return newRig(t, "/concerts", 1132, 1000)
			},
			ready:    []string{"No concerts near London", "Choose another city"},
			headings: []a11yHeading{{"No concerts near London", 2}},
			present:  []a11yNode{{ui.RoleButton, "Choose another city"}},
			tab:      []string{"Set location", "Choose another city"},
		},
		{
			name: "error",
			build: func(t *testing.T) *rig {
				amp := &fakeApple{}
				amp.set(exampleBand)
				amp.fail.Store(true)
				return newRigAmp(t, amp, "/concerts", 1132, 1000, nil, nil)
			},
			ready:   []string{"Couldn't load the concerts", "Try again"},
			present: []a11yNode{{ui.RoleStatus, "Couldn't load the concerts"}, {ui.RoleButton, "Try again"}},
			tab:     []string{"Try again"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.build(t)
			r.tt.Until("the page", r.tt.Shows(tc.ready...))
			r.tt.Settle()
			access.NoNestedControls(t, r.tt.Tester)
			tr := access.Read(t, r.tt.Tester)
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
					t.Errorf("the tree has no role %d %q:\n%s", n.role, n.label, tr.Dump())
				}
			}
			inOrder(t, walkLabels(t, r.tt, 100), tc.tab)
		})
	}
}

// TestA11yShelvesNameTheirLists: Home's concerts shelf and an artist's
// Upcoming concerts list are lists named for their shelf, holding items.
func TestA11yShelvesNameTheirLists(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(t *testing.T) *rig
		ready []string
		list  string
		link  string
	}{
		{"home", func(t *testing.T) *rig {
			return shelfRig(t, 1132, false, func(c *ui.Context, r *rig) { ConcertsShelf(c, r.h.Env, true) }, exampleBand, sampleSinger, anotherAct)
		}, []string{"Concerts in London", "Example Hall"}, "Concerts in London", "See all"},
		{"artist", func(t *testing.T) *rig {
			return shelfRig(t, 1132, false, func(c *ui.Context, r *rig) { ArtistConcerts(c, r.h.Env, "900001", true) }, artistShows...)
		}, []string{"Upcoming concerts", "Example Hall"}, "Upcoming concerts", "More concerts near London"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.build(t)
			r.shows(tc.ready...)
			r.tt.Settle()
			access.NoNestedControls(t, r.tt.Tester)
			tr := access.Read(t, r.tt.Tester)
			list, ok := tr.Find(ui.RoleList, tc.list)
			if !ok {
				t.Fatalf("no list %q:\n%s", tc.list, tr.Dump())
			}
			for _, c := range tr.Children(list) {
				if c.Role != ui.RoleListItem {
					t.Errorf("list %q holds %s", tc.list, c)
				}
			}
			if _, ok := tr.Find(ui.RoleHeading, tc.list); !ok {
				t.Errorf("no heading %q", tc.list)
			}
			if _, ok := tr.Find(ui.RoleLink, tc.link); !ok {
				t.Errorf("no link %q:\n%s", tc.link, tr.Dump())
			}
			walkLabels(t, r.tt, 100)
		})
	}
}

// TestA11yAgendaRovesBetweenALinkAndItsTickets: the agenda is one Tab stop;
// Right moves from a row's artist link to its tickets control, which is a
// named button on the tree beside the link, never inside it.
func TestA11yAgendaRovesBetweenALinkAndItsTickets(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 1000, exampleBand, sampleSinger, anotherAct)
	r.shows("April 2027", "Another Act")
	r.tt.Settle()
	tabTo(t, r.tt, "Example Band with Opener Duo", 10)
	r.tt.Key(0, ui.KeyRight)
	tr := access.Read(t, r.tt.Tester)
	if tr.Focus == nil || tr.Focus.Role != ui.RoleButton || !strings.HasPrefix(tr.Focus.Label, "Get tickets") {
		t.Fatalf("Right focused %v, want the tickets button", tr.Focus)
	}
	link, _ := tr.Find(ui.RoleLink, "Example Band with Opener Duo")
	if tr.Focus.Parent != link.Parent {
		t.Errorf("the tickets button is not a sibling of the link (parents %d, %d)", tr.Focus.Parent, link.Parent)
	}
	// Down goes to the same control of the next row: Sample Singer's menu
	// button, which says it opens a menu.
	r.tt.Key(0, ui.KeyDown)
	tr = access.Read(t, r.tt.Tester)
	if tr.Focus == nil || tr.Focus.Role != ui.RoleMenuButton || tr.Focus.Label != "Get tickets for Sample Singer" {
		t.Fatalf("Down focused %v, want Sample Singer's tickets menu", tr.Focus)
	}
	if tr.Focus.Has(access.Expanded) {
		t.Errorf("the closed menu button says it is expanded: %s", tr.Focus.States)
	}
	// Enter opens the menu with its sellers on the tree; Escape closes it
	// with the focus back on the button.
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	tr = access.Read(t, r.tt.Tester)
	if len(tr.Roles(ui.RoleMenu)) == 0 {
		t.Fatalf("Enter opened no menu:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleMenuItem, "First Seller"); !ok {
		t.Errorf("the menu lacks First Seller:\n%s", tr.Dump())
	}
	if n, ok := tr.Find(ui.RoleMenuButton, "Get tickets for Sample Singer"); !ok || !n.Has(access.Expanded) {
		t.Errorf("the open menu's button does not say it is expanded:\n%s", tr.Dump())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Drain()
	if got := len(access.Read(t, r.tt.Tester).Roles(ui.RoleMenu)); got != 0 {
		t.Errorf("Escape left %d menus open", got)
	}
	if tr := access.Read(t, r.tt.Tester); tr.Focus == nil || tr.Focus.Label != "Get tickets for Sample Singer" {
		t.Errorf("the focus did not return to the tickets button: %v", tr.Focus)
	}
}

// TestA11yDateFieldSaysWhyItIsInvalid: a day the field cannot take is marked
// invalid with the form it wants.
func TestA11yDateFieldSaysWhyItIsInvalid(t *testing.T) {
	r := newRig(t, "/concerts?when=dates&from=2027-01-01", 1132, 1000, exampleBand)
	r.shows("Choose both dates")
	tr := access.Read(t, r.tt.Tester)
	from, ok := tr.Find(ui.RoleTextField, "From")
	if !ok || !from.Has(access.Invalid) {
		t.Fatalf("a From day before today is not invalid:\n%s", tr.Dump())
	}
	if !strings.Contains(from.Description, "YYYY-MM-DD") {
		t.Errorf("the invalid field says %q, want the form of a day", from.Description)
	}
	if to, _ := tr.Find(ui.RoleTextField, "To"); to.Has(access.Invalid) {
		t.Errorf("an empty To is invalid: %s", to.States)
	}
}

// TestA11yLocationDialog: the dialog is a named dialog that opens on its
// field, lists the cities as named buttons with the chosen one marked, says
// when none match, and gives the focus back to Set location on Escape.
func TestA11yLocationDialog(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 900, exampleBand)
	r.shows("Set location", "Example Band with Opener Duo")
	r.tt.Settle()
	tabTo(t, r.tt, "Set location", 5)
	r.tt.Key(0, ui.KeyEnter)
	r.shows("Where are you?", "Find a city", "Manchester")
	r.tt.Settle()
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)
	dlg, ok := tr.Find(ui.RoleDialog, "Where are you?")
	if !ok {
		t.Fatalf("no dialog:\n%s", tr.Dump())
	}
	if !strings.Contains(dlg.Description, "Concerts are listed near the city you choose") {
		t.Errorf("the dialog's description is %q", dlg.Description)
	}
	if _, ok := tr.Find(ui.RoleHeading, "Where are you?"); !ok {
		t.Error("the dialog has no heading")
	}
	if tr.Focus == nil || tr.Focus.Role != ui.RoleTextField || tr.Focus.Label != "Find a city" {
		t.Errorf("the dialog did not open on its field: focus %v", tr.Focus)
	}
	list, ok := tr.Find(ui.RoleList, "Suggested cities")
	if !ok {
		t.Fatalf("no Suggested cities list:\n%s", tr.Dump())
	}
	for _, c := range tr.Children(list) {
		if c.Role != ui.RoleButton || !strings.Contains(c.Label, ", United Kingdom") {
			t.Errorf("a city is %s", c)
		}
	}
	inOrder(t, walkLabels(t, r.tt, 40), []string{"London, United Kingdom", "Manchester, United Kingdom", "Close"})

	// Typing narrows the list under its other name; a term nothing matches
	// is announced once.
	tabTo(t, r.tt, "Find a city", 30)
	r.tt.Announcements()
	r.tt.Type("manch")
	r.tt.Until("the matches", func() bool {
		return r.tt.Shows("Manchester")() && !r.tt.HasText("Birmingham")
	})
	tr = access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleList, "Cities that match"); !ok {
		t.Errorf("no Cities that match list:\n%s", tr.Dump())
	}
	r.tt.Type("zzzz")
	var heard []string
	r.tt.Until("no match", func() bool {
		heard = append(heard, r.tt.Announcements()...)
		return r.tt.HasText("No cities match “manchzzzz”. Try the name of a bigger city nearby.")
	})
	heard = append(heard, r.tt.Announcements()...)
	if n := slices.Index(heard, "No cities match “manchzzzz”."); n < 0 {
		t.Errorf("announced %q, want the empty search read out", heard)
	}

	// Escape closes it, and the focus returns to the button that opened it.
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Drain()
	r.tt.Settle()
	if r.tt.HasText("Where are you?") {
		t.Fatal("Escape left the dialog open")
	}
	if !r.tt.Focused("Set location") {
		t.Errorf("the focus did not return to Set location; focus %v", access.Read(t, r.tt.Tester).Focus)
	}
}

// TestA11yLocationDialogMarksTheChosenCity: the city in use says so as the
// current item (v0.2.0's aria-current), keeping its name.
func TestA11yLocationDialogMarksTheChosenCity(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 900, exampleBand)
	r.shows("Set location", "Example Band with Opener Duo")
	r.click("Set location")
	r.shows("Where are you?", "Manchester")
	tr := access.Read(t, r.tt.Tester)
	var current []string
	for _, n := range tr.Roles(ui.RoleButton) {
		if strings.HasPrefix(n.Description, "Current") {
			current = append(current, n.Label)
		}
	}
	if len(current) != 1 || current[0] != "London, United Kingdom" {
		t.Errorf("the cities marked current: %q", current)
	}
}

// TestA11yFilterChangeIsBusyThenAnnounced: while a changed filter's listing
// loads the old one stays, marked busy; when it arrives its size is read
// out once.
func TestA11yFilterChangeIsBusyThenAnnounced(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 1000, exampleBand, sampleSinger, anotherAct)
	r.shows("April 2027", "Another Act")
	r.tt.Settle()
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("opening the page announced %q", got)
	}
	if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleStatus, "Loading"); ok {
		t.Error("the loaded page is marked busy")
	}
	release := r.amp.hold()
	r.router.Replace("/concerts?genre=21")
	for range 5 {
		r.tt.Drain()
	}
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleStatus, "Loading"); !ok {
		t.Errorf("no busy status while the filter loads:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleHeading, "April 2027"); !ok {
		t.Errorf("the last list went away while loading:\n%s", tr.Dump())
	}
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("announced %q while loading", got)
	}
	release()
	var heard []string
	r.tt.Until("the filtered list", func() bool {
		heard = append(heard, r.tt.Announcements()...)
		return !r.tt.HasText("Example Band with Opener Duo") && r.tt.HasText("Another Act")
	})
	r.tt.Settle()
	heard = append(heard, r.tt.Announcements()...)
	if len(heard) != 1 || heard[0] != "2 concerts" {
		t.Errorf("announced %q, want the size of the list once", heard)
	}
	if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleStatus, "Loading"); ok {
		t.Error("the page is still marked busy")
	}

	// A filter that leaves nothing says so.
	heard = nil
	r.router.Replace("/concerts?when=week")
	r.tt.Until("nothing", func() bool {
		heard = append(heard, r.tt.Announcements()...)
		return r.tt.HasText("No concerts match")
	})
	heard = append(heard, r.tt.Announcements()...)
	if len(heard) != 1 || heard[0] != "No concerts match" {
		t.Errorf("announced %q for an empty filter", heard)
	}
}

// TestA11yLoadingAndErrorAreStatuses: the first load is a labelled status,
// and a failed one is a status that is read out once.
func TestA11yLoadingAndErrorAreStatuses(t *testing.T) {
	amp := &fakeApple{}
	amp.set(exampleBand)
	release := amp.hold()
	r := newRigAmp(t, amp, "/concerts", 1132, 1000, nil, nil)
	for range 3 {
		r.tt.Drain()
	}
	if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleStatus, "Loading"); !ok {
		t.Errorf("no Loading status on the first load:\n%s", access.Read(t, r.tt.Tester).Dump())
	}
	release()
	r.shows("Example Band with Opener Duo")

	failing := &fakeApple{}
	failing.set(exampleBand)
	failing.fail.Store(true)
	f := newRigAmp(t, failing, "/concerts", 1132, 1000, nil, nil)
	var heard []string
	f.tt.Until("the error", func() bool {
		heard = append(heard, f.tt.Announcements()...)
		return f.tt.HasText("Couldn't load the concerts")
	})
	heard = append(heard, f.tt.Announcements()...)
	if len(heard) != 1 || !strings.HasPrefix(heard[0], "Couldn't load the concerts") {
		t.Errorf("announced %q, want the error once", heard)
	}
}
