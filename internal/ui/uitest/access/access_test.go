package access

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestReadSeesTheTree is the seam's own check: if a MyGo bump moves the
// fields Read reads, this fails first. It also pins what the helpers
// report: a control beside an opener is on the tree, one inside it is not.
func TestReadSeesTheTree(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(8).Gap(8).Children(func() {
			ui.Button(c.Key("beside"), "Beside").Clicked()
			ui.Row(c.Key("row")).Children(func() {
				opener := ui.ButtonBase(c.Key("opener")).Label("Opener")
				opener.Children(func() {
					ui.Button(c.Key("inner"), "Inner").Clicked()
				})
			})
			ui.Link(c.Key("link"), "A link", "https://example.org").Clicked()
			ui.Text(c.Key("title"), "Title").Role(ui.RoleHeading).Level(2)
		})
	}, 320, 240)
	tr := Read(t, tt)
	for _, w := range []struct {
		role  ui.Role
		label string
	}{{ui.RoleButton, "Beside"}, {ui.RoleButton, "Opener"}, {ui.RoleLink, "A link"}, {ui.RoleHeading, "Title"}} {
		if _, ok := tr.Find(w.role, w.label); !ok {
			t.Errorf("no %s %q on the tree:\n%s", roleName(w.role), w.label, tr.Dump())
		}
	}
	if _, ok := tr.Find(ui.RoleButton, "Inner"); ok {
		t.Errorf("a button inside a button is on the tree; MyGo's roles changed:\n%s", tr.Dump())
	}
	if h, _ := tr.Find(ui.RoleHeading, "Title"); h.Level != 2 {
		t.Errorf("heading level %d", h.Level)
	}
	nested := NestedControls(t, tt)
	if len(nested) != 1 || nested[0].Inner != "Inner" || nested[0].Opener != `button "Opener"` {
		t.Errorf("nested controls %v, want the Inner button inside Opener", nested)
	}
}

func TestTabWalkListsTheStopsAndFlagsWhatTheTreeHides(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(8).Gap(8).Children(func() {
			ui.Button(c.Key("a"), "First").Clicked()
			ui.ButtonBase(c.Key("opener")).Label("Opener").Children(func() {
				ui.Button(c.Key("inner"), "Inner").Clicked()
			})
		})
	}, 320, 240)
	ft := &fakeT{TB: t}
	stops := TabWalk(ft, tt, 10)
	if !ft.failed {
		t.Fatalf("a Tab stop inside an opener was not flagged: %q", labels(stops))
	}
	if len(stops) < 2 || stops[0].Node.Label != "First" || stops[1].Node.Label != "Opener" {
		t.Errorf("stops %q", labels(stops))
	}
}

// fakeT records a failure instead of failing the test.
type fakeT struct {
	testing.TB
	failed bool
}

func (f *fakeT) Errorf(string, ...any) { f.failed = true }
