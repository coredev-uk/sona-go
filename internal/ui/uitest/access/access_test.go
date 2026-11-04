package access

import (
	"os"
	"strings"
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

// TestPinnedMyGo fails when MyGo is bumped: the helper mirrors constants of
// MyGo's ui/element.go (flag bits), ui/access.go (leaf roles) and
// internal/platform/access.go (states, actions), which a bump may move.
// Re-check them against the new version, then update this pin.
func TestPinnedMyGo(t *testing.T) {
	mod, err := os.ReadFile("../../../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mod), "github.com/egoist/mygo v0.7.4\n") {
		t.Fatal("MyGo is not v0.7.4 any more: re-check the constants of internal/ui/uitest/access/access.go against ui/element.go, ui/access.go and internal/platform/access.go, then update this pin")
	}
}

// routed is a page of a router, as the features' pages are.
func routed(c *ui.Context, r *ui.Router, page func()) {
	r.View(c, func(rt *ui.Route) {
		switch {
		case rt.Match("/a"), rt.Match("/b"):
			page()
		}
	})
}

// TestNestedControlsAreFoundInRoutedPages is the canary of the nested-control
// check: it once skipped every routed page (the wrong bit for "inert").
func TestNestedControlsAreFoundInRoutedPages(t *testing.T) {
	r := ui.NewRouter("/a")
	tt := ui.NewTester(func(c *ui.Context) {
		routed(c, r, func() {
			ui.ButtonBase(c.Key("opener")).Label("Opener").Children(func() {
				ui.Button(c.Key("inner"), "Inner").Clicked()
			})
			// A clickable that takes no focus is found too.
			ui.ButtonBase(c.Key("second")).Label("Second").Children(func() {
				ui.Row(c.Key("quiet")).Label("Quiet").Clicked()
			})
		})
	}, 320, 240)
	got := NestedControls(t, tt)
	if len(got) != 2 || got[0].Inner != "Inner" || got[1].Inner != "Quiet" {
		t.Errorf("nested controls %v, want Inner and Quiet", got)
	}
}

// TestALeavingPageIsNotChecked: a page going away is inert, off the tree.
func TestALeavingPageIsNotChecked(t *testing.T) {
	r := ui.NewRouter("/a")
	tt := ui.NewTester(func(c *ui.Context) {
		routed(c, r, func() {
			if r.Path() == "/a" {
				ui.ButtonBase(c.Key("old")).Label("Old").Children(func() { ui.Button(c.Key("x"), "In old").Clicked() })
				return
			}
			ui.Button(c.Key("fresh"), "Fresh").Clicked()
		})
	}, 320, 240)
	r.Push("/b")
	tt.Frame() // the old page fades out, inert, beside the new one
	if got := NestedControls(t, tt); len(got) != 0 {
		t.Errorf("a leaving page was checked: %v", got)
	}
	if tr := Read(t, tt); len(tr.FindAll(ui.RoleButton, "Fresh")) != 1 {
		t.Errorf("the new page is not on the tree:\n%s", tr.Dump())
	}
}

func TestInvisibleHiddenAndIgnoredInputAreHandledAsMyGoDoes(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.ButtonBase(c.Key("a")).Label("A").Children(func() {
				ui.Button(c.Key("hidden"), "Hidden").Hidden(true).Clicked()
				ui.Button(c.Key("invisible"), "Invisible").Invisible().Clicked()
			})
			// Ignoring the pointer and Tab still leaves the control on the
			// tree as a sibling.
			ui.Button(c.Key("ignored"), "Ignored").IgnoreInput(true).Opacity(0).Clicked()
		})
	}, 320, 240)
	if got := NestedControls(t, tt); len(got) != 0 {
		t.Errorf("hidden and invisible elements were reported: %v", got)
	}
	tr := Read(t, tt)
	n, ok := tr.Find(ui.RoleButton, "Ignored")
	if !ok || !n.Can(Press) || n.Has(Focusable) {
		t.Errorf("an ignored-input button: %+v (want on the tree, pressable, not focusable)", n)
	}
}

func TestStatesAndActionsAreReadInMyGosOrder(t *testing.T) {
	on, off, vol := true, false, 30.0
	text := "x"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Gap(4).Children(func() {
			ui.Switch(c.Key("on"), &on).Label("On")
			ui.Switch(c.Key("off"), &off).Label("Off").Disabled(true)
			ui.Slider(c.Key("slider"), &vol, 0, 100).Label("Vol")
			ui.ButtonBase(c.Key("exp")).Label("Menu").Expanded(true)
			ui.TextInput(c.Key("in"), &text).Label("Field").Error("bad")
		})
	}, 320, 400)
	tr := Read(t, tt)
	check := func(role ui.Role, label string, has, not State) {
		t.Helper()
		n, ok := tr.Find(role, label)
		if !ok {
			t.Fatalf("no %q:\n%s", label, tr.Dump())
		}
		if !n.Has(has) || n.States&not != 0 {
			t.Errorf("%s: states %s, want %s and not %s", n, n.States, has, not)
		}
	}
	check(ui.RoleSwitch, "On", Checked|Focusable, Disabled)
	check(ui.RoleSwitch, "Off", Disabled, Checked|Focusable)
	check(ui.RoleButton, "Menu", Expanded|Focusable, 0)
	check(ui.RoleTextField, "Field", Invalid|Focusable, Disabled)
	if n, _ := tr.Find(ui.RoleSlider, "Vol"); !n.Can(Increment|Decrement|Focus) || n.Now != 30 {
		t.Errorf("slider: %+v", n)
	}
}
