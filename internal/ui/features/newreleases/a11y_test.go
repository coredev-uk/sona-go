package newreleases

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The page is a heading, three groups each under its own h2 and holding a
// table of rows, every row one link (the opener) named by the release's
// title and described by its artist, kind, date and whether it is unread;
// the buttons beside it are controls of their own, and the list is one Tab
// stop with the arrows moving between rows.
func TestA11yRowsAreNamedLinksInGroupedTables(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 900)
	r.until("the list", r.tt.Shows("Winter Rooms", "Slow Harbour"))
	tt := r.tt.Tester
	access.NoNestedControls(t, tt)
	tr := access.Read(t, tt)
	if _, ok := tr.Find(ui.RoleHeading, "New from Artists You Love"); !ok {
		t.Fatalf("no h1:\n%s", tr.Dump())
	}
	for _, g := range []string{"Coming soon", "This week", "Earlier"} {
		if _, ok := tr.Find(ui.RoleTable, g); !ok {
			t.Errorf("no table for %q:\n%s", g, tr.Dump())
		}
	}
	link, ok := tr.Find(ui.RoleLink, "Night Signals")
	if !ok {
		t.Fatalf("no link for the unread EP:\n%s", tr.Dump())
	}
	if want := "Unread · Mia Vale · EP · Nov 14"; link.Description != want {
		t.Errorf("description %q, want %q", link.Description, want)
	}
	if read, ok := tr.Find(ui.RoleLink, "Paper Boats"); !ok || read.Description != "Hollow Pines · Single · Nov 13" {
		t.Errorf("a read release: %+v", read)
	}
	// The header's controls and each row's opener are focusable; the list is
	// one stop.
	stops := access.TabWalk(t, tt, 12)
	var labels []string
	for _, s := range stops {
		labels = append(labels, s.Node.Label)
	}
	want := []string{"Mark all as read", "Clear list", "Winter Rooms", "Night Signals", "Paper Boats", "Lantern Season", "Slow Harbour"}
	if !slices.Equal(labels[:min(len(labels), len(want))], want) {
		t.Errorf("Tab stops %v, want the header's buttons then one stop for each row", labels)
	}
}

func TestA11yHoveredRowOffersRemove(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 900)
	r.until("the list", r.tt.Shows("Paper Boats"))
	r.tt.Hover("Paper Boats")
	r.tt.Drain()
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleButton, "Remove Paper Boats"); !ok {
		t.Fatalf("no Remove button on the hovered row:\n%s", tr.Dump())
	}
}

// The shelf is a group named by its h2, holding one table of rows, each row a
// link named like the page's; "See all" comes first in the Tab order, then one
// stop for the list.
func TestA11yTheShelfIsAGroupWithAHeadingAndRowsAsLinks(t *testing.T) {
	r := newShelfRig(t, sample(), true, 1132, 700)
	r.until("the shelf", r.tt.Shows("Winter Rooms", "See all"))
	tt := r.tt.Tester
	access.NoNestedControls(t, tt)
	tr := access.Read(t, tt)
	if _, ok := tr.Find(ui.RoleHeading, "New from Artists You Love"); !ok {
		t.Fatalf("no heading:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleGroup, "New from Artists You Love"); !ok {
		t.Errorf("no group:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleTable, "New from Artists You Love"); !ok {
		t.Errorf("no table:\n%s", tr.Dump())
	}
	link, ok := tr.Find(ui.RoleLink, "Night Signals")
	if !ok || link.Description != "Unread · Mia Vale · EP · Nov 14" {
		t.Errorf("the unread EP: %+v\n%s", link, tr.Dump())
	}
	var labels []string
	for _, s := range access.TabWalk(t, tt, 8) {
		labels = append(labels, s.Node.Label)
	}
	want := []string{"See all", "Winter Rooms", "Night Signals", "Lantern Season"}
	if !slices.Equal(labels[:min(len(labels), len(want))], want) {
		t.Errorf("Tab stops %v, want %v", labels, want)
	}
}
