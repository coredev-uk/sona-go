package search

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
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

type a11yNode struct {
	role  ui.Role
	label string
}

type a11yHeading struct {
	label string
	level int
}

// TestA11yPages is the audit of every page of the feature and its states:
// no control nested in a button or a link, headings, lists and links named,
// Tab reaching the page's controls in reading order and never trapping.
func TestA11yPages(t *testing.T) {
	cases := []struct {
		name     string
		build    func(t *testing.T) *pageRig
		ready    []string
		headings []a11yHeading
		present  []a11yNode
		absent   []a11yNode
		tab      []string
		walk     func(t *testing.T, got []string)
	}{
		{
			name: "empty search with browse categories",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/search", rigOpts{signedIn: true})
			},
			ready:    []string{"Browse categories", "Alternative"},
			headings: []a11yHeading{{"Search", 1}, {"Browse categories", 2}},
			present: []a11yNode{
				{ui.RoleGroup, "Search in"},
				{ui.RoleToggleButton, "Apple Music"},
				{ui.RoleToggleButton, "Library"},
				{ui.RoleTable, "Browse categories"},
				{ui.RoleLink, "Alternative"},
			},
			tab: []string{"Apple Music", "Library", "Alternative", "Ibiza"},
		},
		{
			name: "recent searches",
			build: func(t *testing.T) *pageRig {
				r := newPageRig(t, "/search", rigOpts{})
				seedRecent(t, r)
				r.s.refreshRecent()
				return r
			},
			ready:    []string{"Recently searched", "Clear", "alpha", "Blue Band", "Catalog Song"},
			headings: []a11yHeading{{"Search", 1}, {"Recently searched", 2}},
			present: []a11yNode{
				{ui.RoleList, "Recently searched"},
				{ui.RoleButton, "Clear"},
				{ui.RoleLink, "alpha"},
				{ui.RoleLink, "Blue Band Artist · Rock"},
				{ui.RoleButton, "Remove alpha from recent searches"},
				{ui.RoleButton, "Remove Blue Band from recent searches"},
			},
			// Entries are as new as the ms they were remembered in, so their
			// order is not fixed; each is followed by its own Remove.
			tab: []string{"Apple Music", "Library", "Clear"},
			walk: func(t *testing.T, got []string) {
				for _, e := range [][2]string{
					{"Blue Band Artist · Rock", "Remove Blue Band from recent searches"},
					{"Catalog Song Song · Blue Band", "Remove Catalog Song from recent searches"},
					{"alpha", "Remove alpha from recent searches"},
				} {
					if i := slices.Index(got, e[0]); i < 0 || i+1 >= len(got) || got[i+1] != e[1] {
						t.Errorf("%q is not followed by %q in %q", e[0], e[1], got)
					}
				}
			},
		},
		{
			name: "catalogue results",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
			},
			ready:    []string{"Top results", "Blue Mix", "Blue Song 1"},
			headings: []a11yHeading{{"“blue”", 1}, {"Top results", 2}, {"Artists", 2}, {"Albums", 2}, {"Songs", 2}, {"Playlists", 2}},
			present: []a11yNode{
				{ui.RoleList, "Top results for blue"},
				{ui.RoleLink, "Blue Band Artist · Rock"},
				{ui.RoleLink, "Blue Hour Album · Blue Band"},
				{ui.RoleButton, "Blue Song 1, Play"},
				{ui.RoleLink, "See all"},
				{ui.RoleTable, "Songs matching blue"},
				{ui.RoleButton, "Play Blue Song 1"},
			},
			tab: []string{
				"Apple Music", "Library", "Blue Band Artist · Rock", "Blue Hour Album · Blue Band", "Blue Song 1, Play",
				"Blue Band", "Blue Hour", "See all", "Blue Song 1",
			},
		},
		{
			name: "library results",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/search?q=blue&scope=library", rigOpts{})
			},
			ready:    []string{"Blue Hour", "Blue Monday"},
			headings: []a11yHeading{{"“blue”", 1}, {"Songs", 2}, {"Albums", 2}},
			present: []a11yNode{
				{ui.RoleToggleButton, "Library"},
				{ui.RoleTable, "Library songs matching blue"},
				{ui.RoleButton, "Play Blue Hour song 1"},
				{ui.RoleLink, "Blue Hour"},
				{ui.RoleLink, "Al"},
			},
			tab: []string{"Apple Music", "Library", "Blue Hour song 1", "Blue Hour", "Blue Monday"},
		},
		{
			name: "empty library search",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/search?scope=library", rigOpts{})
			},
			ready:    []string{"Search your library"},
			headings: []a11yHeading{{"Search", 1}, {"Search your library", 2}},
			tab:      []string{"Apple Music", "Library"},
		},
		{
			name: "no results",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/search?q=nothing", rigOpts{signedIn: true})
			},
			ready:    []string{"Nothing on Apple Music for “nothing”"},
			headings: []a11yHeading{{"“nothing”", 1}, {"Nothing on Apple Music for “nothing”", 2}},
			tab:      []string{"Apple Music", "Library"},
		},
		{
			name: "catalogue error",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/search?q=blue", rigOpts{signedIn: true, setup: func(a *pageApple) { a.fail["/v1/catalog/gb/search"] = 400 }})
			},
			ready:   []string{"Couldn't search Apple Music", "Try again"},
			present: []a11yNode{{ui.RoleAlert, "Couldn't search Apple Music"}, {ui.RoleButton, "Try again"}},
			tab:     []string{"Apple Music", "Library", "Try again"},
		},
		{
			name: "see all songs",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/search/song?q=blue", rigOpts{signedIn: true})
			},
			ready:    []string{"Blue Song 1", "Apple Music results for"},
			headings: []a11yHeading{{"Songs", 1}},
			present: []a11yNode{
				{ui.RoleLink, "“blue”"}, {ui.RoleTable, "Songs matching blue"}, {ui.RoleButton, "Play Blue Song 1"},
			},
			tab: []string{"“blue”", "Blue Song 1"},
		},
		{
			name: "see all albums",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/search/album?q=blue", rigOpts{signedIn: true})
			},
			ready:    []string{"Blue Hour", "Blue Monday"},
			headings: []a11yHeading{{"Albums", 1}},
			present:  []a11yNode{{ui.RoleLink, "“blue”"}, {ui.RoleTable, "Albums matching blue"}, {ui.RoleLink, "Blue Hour"}},
			tab:      []string{"“blue”", "Blue Hour", "Blue Monday"},
		},
		{
			name: "see all error",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/search/song?q=blue", rigOpts{signedIn: true, setup: func(a *pageApple) { a.fail["/v1/catalog/gb/search"] = 400 }})
			},
			ready:   []string{"Try again"},
			present: []a11yNode{{ui.RoleButton, "Try again"}},
			tab:     []string{"“blue”", "Try again"},
		},
		{
			name: "profile",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/profile/sp.1", rigOpts{signedIn: true, setup: func(a *pageApple) { a.verified = true }})
			},
			ready:    []string{"Taylor Listener", "Shared Mix 1"},
			headings: []a11yHeading{{"Taylor Listener", 1}, {"Public playlists", 2}},
			present:  []a11yNode{{ui.RoleImage, "Verified"}, {ui.RoleTable, "Playlists shared by Taylor Listener"}, {ui.RoleLink, "Shared Mix 1"}},
			tab:      []string{"Shared Mix 1", "Shared Mix 2"},
		},
		{
			name: "profile error",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, "/profile/sp.1", rigOpts{signedIn: true, setup: func(a *pageApple) { a.fail["/v1/social/gb/social-profiles"] = 400 }})
			},
			ready:   []string{"Couldn't load this profile"},
			present: []a11yNode{{ui.RoleButton, "Try again"}},
			tab:     []string{"Try again"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.build(t)
			tt := r.tt
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
					t.Errorf("the tree has no role %d %q:\n%s", n.role, n.label, tr.Dump())
				}
			}
			for _, n := range tc.absent {
				if _, ok := tr.Find(n.role, n.label); ok {
					t.Errorf("the tree has role %d %q", n.role, n.label)
				}
			}
			got := walkLabels(t, tt, 200)
			inOrder(t, got, tc.tab)
			if tc.walk != nil {
				tc.walk(t, got)
			}
		})
	}
}

// TestA11yListsHoldOnlyItems: a result list is not wrapped in another list
// (a list contains list items, nothing else).
func TestA11yListsHoldOnlyItems(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	r.tt.Until("the results", r.tt.Shows("Top results", "Blue Song 1"))
	tr := access.Read(t, r.tt.Tester)
	lists := tr.Roles(ui.RoleList)
	if len(lists) == 0 {
		t.Fatalf("no lists:\n%s", tr.Dump())
	}
	for _, l := range lists {
		if tr.Within(l, ui.RoleList) {
			t.Errorf("list %q is inside another list", l.Label)
		}
		if l.Label == "" {
			t.Errorf("an unnamed list:\n%s", tr.Dump())
		}
		for _, c := range tr.Children(l) {
			if c.Role != ui.RoleListItem {
				t.Errorf("list %q holds a %s %q, not a list item", l.Label, c, c.Label)
			}
		}
	}
}

// TestA11ySearchingIsAStatusAndResultsAreAnnounced: a page still reading
// says so, keeps the last term's results on screen marked busy while the
// next load, and reads out what a search found once.
func TestA11ySearchingIsAStatusAndResultsAreAnnounced(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	var heard []string
	r.tt.Until("the results", func() bool {
		heard = append(heard, r.tt.Announcements()...)
		return r.tt.Shows("Top results", "Blue Song 1")()
	})
	r.tt.Settle()
	heard = append(heard, r.tt.Announcements()...)
	if len(heard) != 1 || heard[0] != "Results for “blue”" {
		t.Errorf("announced %q, want the results once", heard)
	}
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleStatus, "Searching"); ok {
		t.Errorf("the finished page is marked busy:\n%s", tr.Dump())
	}
	if busy := busyNodes(tr); len(busy) != 0 {
		t.Errorf("the finished page has busy regions %v:\n%s", busy, tr.Dump())
	}

	// A new term: the old results stay, marked busy, until the new ones come.
	gate := make(chan struct{})
	r.apple.mu.Lock()
	r.apple.gate = gate
	r.apple.mu.Unlock()
	r.router.Replace("/search?q=slowblue")
	for range 5 {
		r.tt.Drain()
	}
	tr = access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleStatus, "Searching"); !ok {
		t.Errorf("no busy status while the next term loads:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleHeading, "Top results"); !ok {
		t.Errorf("the last results went away while loading:\n%s", tr.Dump())
	}
	if head, ok := tr.Find(ui.RoleHeading, "Top results"); ok {
		if !slices.ContainsFunc(tr.Ancestors(head), func(a access.Node) bool { return a.Has(access.Busy) }) {
			t.Errorf("the last results are not in a busy region:\n%s", tr.Dump())
		}
	}
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("announced %q while loading", got)
	}
	close(gate)
	heard = nil
	r.tt.Until("the next results", func() bool {
		heard = append(heard, r.tt.Announcements()...)
		return len(heard) > 0
	})
	if heard[0] != "Results for “slowblue”" {
		t.Errorf("announced %q, want the new term's results", heard)
	}
	r.tt.Settle()
	if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleStatus, "Searching"); ok {
		t.Error("the page is still marked busy")
	}

	// No results are read out too.
	heard = nil
	r.router.Replace("/search?q=nothing")
	r.tt.Until("nothing", func() bool {
		heard = append(heard, r.tt.Announcements()...)
		return r.tt.Shows("Nothing on Apple Music for “nothing”")()
	})
	heard = append(heard, r.tt.Announcements()...)
	if len(heard) != 1 || heard[0] != "Nothing on Apple Music for “nothing”" {
		t.Errorf("announced %q for an empty search", heard)
	}
}

// holdLibrary is the search service whose library search waits for gate.
type holdLibrary struct {
	searchAPI
	gate chan struct{}
}

func (h holdLibrary) Library(ctx context.Context, term string, perKind int) (services.LibrarySearch, error) {
	select {
	case <-h.gate:
	case <-ctx.Done():
		return services.LibrarySearch{}, ctx.Err()
	}
	return h.searchAPI.Library(ctx, term, perKind)
}

// TestA11yLoadingPagesAreStatuses: every page that is still reading has a
// labelled status.
func TestA11yLoadingPagesAreStatuses(t *testing.T) {
	for _, tc := range []struct {
		name, loc, label string
		signedIn         bool
		holdLibrary      bool
	}{
		{"results", "/search?q=slowblue", "Searching", true, false},
		{"library results", "/search?q=blue&scope=library", "Searching", false, true},
		{"profile", "/profile/sp.1", "Loading", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gate := make(chan struct{})
			ro := rigOpts{signedIn: tc.signedIn, setup: func(a *pageApple) { a.gate = gate }}
			if tc.holdLibrary {
				ro.wrap = func(api searchAPI) searchAPI { return holdLibrary{api, gate} }
			}
			r := newPageRig(t, tc.loc, ro)
			t.Cleanup(func() { close(gate) })
			tr := access.Read(t, r.tt.Tester)
			if _, ok := tr.Find(ui.RoleStatus, tc.label); !ok {
				t.Errorf("no %q status on the first frames:\n%s", tc.label, tr.Dump())
			}
		})
	}
}

// TestA11yFieldIsACombobox: the top bar's field is a combobox named Search
// that says when its list shows, keeps the focus while the arrows move the
// active row, and whose Clear button is beside it.
func TestA11yFieldIsACombobox(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)
	box, ok := tr.Find(ui.RoleComboBox, "Search")
	if !ok {
		t.Fatalf("no combobox:\n%s", tr.Dump())
	}
	if box.Has(access.Expanded) {
		t.Errorf("the closed field says its list shows: %s", box.States)
	}
	if _, ok := tr.Find(ui.RoleButton, "Clear search"); ok {
		t.Error("an empty field has a Clear button")
	}
	inOrder(t, walkLabels(t, r.tt, 10), []string{"Search"})

	r.focusField()
	r.typeAndWait("blu", "Top results")
	access.NoNestedControls(t, r.tt.Tester)
	tr = access.Read(t, r.tt.Tester)
	box, _ = tr.Find(ui.RoleComboBox, "Search")
	if !box.Has(access.Expanded) {
		t.Errorf("the field does not say its list shows: %s", box.States)
	}
	list, ok := tr.Find(ui.RoleListBox, "Suggestions")
	if !ok {
		t.Fatalf("no Suggestions list:\n%s", tr.Dump())
	}
	var rows []string
	for _, n := range tr.Children(list) {
		if n.Role != ui.RoleOption {
			t.Errorf("the list holds a %s", n)
		}
		rows = append(rows, n.Label)
	}
	if len(rows) != 5 || rows[0] != "blue" || rows[1] != "blue band" || rows[2] != "Blue Band" {
		t.Errorf("rows %q", rows)
	}
	if n, ok := tr.Find(ui.RoleOption, "Blue Band"); !ok || n.Description != "Artist" {
		t.Errorf("the artist row lacks its kind as the description: %+v", n)
	}
	if _, ok := tr.Find(ui.RoleButton, "Clear search"); !ok {
		t.Errorf("no Clear search beside the field:\n%s", tr.Dump())
	}

	// The arrows move the active row, which the tree reports as the focus
	// while the field keeps it.
	r.key(ui.KeyDown)
	r.key(ui.KeyDown)
	tr = access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleOption, "blue band"); !ok || (!n.Has(access.Selected) && !n.Has(access.Checked)) {
		t.Errorf("the second row is not the active one:\n%s", tr.Dump())
	}
	if tr.Focus == nil || tr.Focus.Label != "blue band" {
		t.Errorf("the focus is %v, want the active row", tr.Focus)
	}
	if !r.tt.Focused("Search") {
		t.Error("the focus left the field")
	}
	// Up moves the active option back; only one option is selected.
	r.key(ui.KeyUp)
	tr = access.Read(t, r.tt.Tester)
	if tr.Focus == nil || tr.Focus.Role != ui.RoleOption || tr.Focus.Label != "blue" {
		t.Errorf("Up: the focus is %v, want the option blue", tr.Focus)
	}
	if n, _ := tr.Find(ui.RoleOption, "blue band"); n.Has(access.Checked) {
		t.Errorf("the option left behind is still selected: %s", n.States)
	}

	// Escape closes the list (the field keeps the focus), and says so.
	r.key(ui.KeyEscape)
	tr = access.Read(t, r.tt.Tester)
	if box, _ := tr.Find(ui.RoleComboBox, "Search"); box.Has(access.Expanded) {
		t.Errorf("the field says its list shows after Escape: %s", box.States)
	}
	if len(tr.Roles(ui.RoleListBox)) != 0 {
		t.Errorf("the list stayed on the tree:\n%s", tr.Dump())
	}
	if !r.tt.Focused("Search") {
		t.Error("the first Escape took the focus away")
	}
	r.key(ui.KeyEscape)
	if r.tt.Focused("Search") {
		t.Error("the second Escape left the focus in the field")
	}
}

// TestA11yFieldRecentList: the empty field lists the recent searches under
// their own name.
func TestA11yFieldRecentList(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.rememberTerm("alpha", scopeCatalog)
	r.focusField()
	r.tt.Until("the list", r.tt.Shows("Recently searched", "alpha"))
	tr := access.Read(t, r.tt.Tester)
	list, ok := tr.Find(ui.RoleListBox, "Recently searched")
	if !ok {
		t.Fatalf("no Recently searched list:\n%s", tr.Dump())
	}
	if kids := tr.Children(list); len(kids) != 1 || kids[0].Label != "alpha" {
		t.Errorf("the list holds %v", kids)
	}
}

// TestA11yFieldClearIsATabStopAfterTheField: Tab goes from the field to Clear
// search, which empties it and returns to the field.
func TestA11yFieldClearIsATabStopAfterTheField(t *testing.T) {
	r := newFieldRig(t, "/library/albums")
	r.focusField()
	r.typeAndWait("blu", "Top results")
	r.key(ui.KeyTab)
	r.tt.Frame()
	if !r.tt.Focused("Clear search") {
		t.Fatalf("Tab from the field did not reach Clear search")
	}
	if strings.TrimSpace(r.s.field.text) == "" {
		t.Fatal("the field is empty before Clear")
	}
	r.key(ui.KeyEnter)
	r.tt.Frame()
	if r.s.field.text != "" || !r.tt.Focused("Search") {
		t.Errorf("Clear left %q in the field, focus on Search: %v", r.s.field.text, r.tt.Focused("Search"))
	}
}

// busyNodes are the nodes marked busy.
func busyNodes(tr access.Tree) []access.Node {
	var out []access.Node
	for _, n := range tr.Nodes {
		if n.Has(access.Busy) {
			out = append(out, n)
		}
	}
	return out
}
