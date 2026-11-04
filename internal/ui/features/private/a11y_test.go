package private

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The pill is a toggle button named for what it is, pressed while on, with
// the tip as its description; forced on (SONA_PRIVATE=1) it stays focusable
// and says why a press does nothing (v0.2.0's aria-disabled); the word
// beside the glyph is not a second control.
func TestA11yPillStates(t *testing.T) {
	r := newRig(t, 1200)
	r.settle(t)
	tt := r.tt.Tester
	access.NoNestedControls(t, tt)
	find := func(name string) access.Node {
		t.Helper()
		tr := access.Read(t, tt)
		n, ok := tr.Find(ui.RoleToggleButton, name)
		if !ok {
			t.Fatalf("no toggle button %q:\n%s", name, tr.Dump())
		}
		return n
	}
	n := find("Private listening")
	if n.Has(access.Checked) || !n.Has(access.Focusable) || n.Description != "Turn on private listening" {
		t.Errorf("off: %+v", n)
	}
	if stops := access.TabWalk(t, tt, 5); len(stops) != 1 || stops[0].Node.Label != "Private listening" {
		t.Errorf("Tab stops %v", stops)
	}

	r.emit(services.PrivateState{On: true, Source: "user"})
	n = find("Private listening")
	if !n.Has(access.Checked) || n.Description == "" || n.Description == "Turn on private listening" {
		t.Errorf("on: %+v", n)
	}

	r.emit(services.PrivateState{On: true, Source: "environment"})
	n = find("Private listening (forced by SONA_PRIVATE=1)")
	if !n.Has(access.Checked) || n.Has(access.Disabled) || !n.Has(access.Focusable) ||
		n.Description != "On because Sona was started with SONA_PRIVATE=1" {
		t.Errorf("forced: %+v", n)
	}
}
