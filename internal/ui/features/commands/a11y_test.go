package commands

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The palette (a dialog named for it, the field a combobox, the results a
// list of items under group headings) and the help sheet (a dialog, an h2,
// h3 groups, Settings a link and Close a button, both reached by Tab) are
// on the tree; Escape closes each and the focus goes back to the opener.
func TestA11yPaletteAndHelp(t *testing.T) {
	t.Run("palette", func(t *testing.T) {
		r := newRig(t)
		r.focusOpener()
		r.tt.Key(ui.Ctrl, ui.KeyK)
		tt := r.tt.Tester
		access.NoNestedControls(t, tt)
		tr := access.Read(t, tt)
		if _, ok := tr.Find(ui.RoleDialog, "Command palette"); !ok {
			t.Fatalf("no dialog:\n%s", tr.Dump())
		}
		if f, ok := tr.Find(ui.RoleComboBox, "Find a page, action or music in your library"); !ok || !f.Has(access.Focusable) {
			t.Errorf("the field: %+v", f)
		}
		if _, ok := tr.Find(ui.RoleListBox, "Results"); !ok {
			t.Error("the results are not a named list")
		}
		for _, g := range []string{"Playback", "View", "Pages", "General"} {
			if _, ok := tr.Find(ui.RoleHeading, g); !ok {
				t.Errorf("no heading %q", g)
			}
		}
		for _, c := range []string{"Play or pause", "Next track", "Library · Albums"} {
			if _, ok := tr.Find(ui.RoleOption, c); !ok {
				t.Errorf("no command %q", c)
			}
		}
		// The field keeps the focus; the highlighted option is its active
		// descendant, and the arrows move it.
		if tr.Focus == nil || tr.Focus.Role != ui.RoleOption || (!tr.Focus.Has(access.Checked) && !tr.Focus.Has(access.Selected)) {
			t.Fatalf("the active option: %+v\n%s", tr.Focus, tr.Dump())
		}
		first := tr.Focus.Label
		r.tt.Key(0, ui.KeyDown)
		r.settle()
		tr = access.Read(t, tt)
		if tr.Focus == nil || tr.Focus.Role != ui.RoleOption || tr.Focus.Label == first {
			t.Errorf("Down left the active option at %q: %+v", first, tr.Focus)
		}
		if !r.tt.Focused("Find a page, action or music in your library") {
			t.Error("the focus left the field")
		}
		access.TabWalk(t, tt, 20)
		r.tt.Key(0, ui.KeyEscape)
		r.settle()
		if r.m.pal.IsOpen() || !r.tt.Focused("Opener") {
			t.Errorf("Escape: open %v, focus back %v", r.m.pal.IsOpen(), r.tt.Focused("Opener"))
		}
	})
	t.Run("help", func(t *testing.T) {
		r := newRig(t)
		r.focusOpener()
		r.tt.Key(ui.Shift, ui.KeySlash)
		tt := r.tt.Tester
		access.NoNestedControls(t, tt)
		tr := access.Read(t, tt)
		if _, ok := tr.Find(ui.RoleDialog, "Keyboard shortcuts"); !ok {
			t.Fatalf("no dialog:\n%s", tr.Dump())
		}
		if h, ok := tr.Find(ui.RoleHeading, "Keyboard shortcuts"); !ok || h.Level != 2 {
			t.Errorf("title heading: %+v", h)
		}
		if h, ok := tr.Find(ui.RoleHeading, "Playback"); !ok || h.Level != 3 {
			t.Errorf("group heading: %+v", h)
		}
		if _, ok := tr.Find(ui.RoleGroup, "Shortcuts"); !ok {
			t.Error("the list is not a region named Shortcuts")
		}
		if _, ok := tr.Find(ui.RoleListItem, "Play or pause"); !ok {
			t.Error("a shortcut is not a list item")
		}
		// The filter has the focus: Tab goes on to Settings and Close, then round to it.
		stops := access.TabWalk(t, tt, 20)
		var got []string
		for _, s := range stops {
			got = append(got, s.Node.Label)
		}
		if !slices.Equal(got, []string{"Settings", "Close", "Filter shortcuts"}) {
			t.Errorf("Tab stops %q, want Settings, Close, then the filter", got)
		}
		if n, _ := tr.Find(ui.RoleLink, "Settings"); n.Role != ui.RoleLink {
			t.Error("Settings is not a link")
		}
		r.tt.Key(0, ui.KeyEscape)
		r.settle()
		if r.m.help.open || !r.tt.Focused("Opener") {
			t.Errorf("Escape: open %v, focus back %v", r.m.help.open, r.tt.Focused("Opener"))
		}
	})
}
