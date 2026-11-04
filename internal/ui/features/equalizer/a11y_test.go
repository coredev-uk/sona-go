package equalizer

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

func stopLabels(stops []access.Stop) []string {
	out := make([]string, len(stops))
	for i, s := range stops {
		out[i] = s.Node.Label
	}
	return out
}

func inOrder(got, want []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}

// The page's controls are on the tree with names, the faders say their dB
// as their value text, the switch is the one control for On/Off (its word is
// not a second, invisible Tab stop) and Tab walks switch, presets, preamp,
// bands in reading order without a trap.
func TestA11yEqualiserTreeAndTabWalk(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("page", r.tt.Shows("Bands"))
	tt := r.tt.Tester
	access.NoNestedControls(t, tt)
	tr := access.Read(t, tt)

	if h, ok := tr.Find(ui.RoleHeading, "Equaliser"); !ok || h.Level != 1 {
		t.Errorf("page heading: %+v", h)
	}
	if sw, ok := tr.Find(ui.RoleSwitch, "Off"); !ok || sw.Has(access.Checked) {
		t.Errorf("the switch: %+v", sw)
	}
	if g, ok := tr.Find(ui.RoleRadioGroup, "Preset"); !ok || len(tr.Children(g)) < 2 {
		t.Errorf("the preset group: %+v", g)
	}
	if n, ok := tr.Find(ui.RoleRadio, "Flat"); !ok || !n.Has(access.Checked) {
		t.Errorf("the chosen preset: %+v", n)
	}
	if _, ok := tr.Find(ui.RoleGroup, "Bands"); !ok {
		t.Error("no Bands region")
	}
	sliders := tr.Roles(ui.RoleSlider)
	if len(sliders) != 11 {
		t.Fatalf("%d sliders, want the preamp and ten bands:\n%s", len(sliders), tr.Dump())
	}
	for _, s := range sliders {
		if s.Label == "" || s.Description != "0 dB" {
			t.Errorf("slider %q reads %q", s.Label, s.Description)
		}
	}

	stops := stopLabels(access.TabWalk(t, tt, 60))
	want := []string{"Off", "Flat", "Preamp", "29 Hz", "119 Hz", "947 Hz", "15k Hz"}
	if !inOrder(stops, want) {
		t.Errorf("Tab walk %q, want in order %q", stops, want)
	}
	if len(stops) != 1+1+11 {
		t.Errorf("Tab stops %q: want the switch, the chosen preset and the 11 faders", stops)
	}
}

// Turning the equaliser on is the same press from the word as from the
// switch, and the switch's name follows it.
func TestA11yEqualiserSwitchNameFollowsItsState(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("page", r.tt.Shows("Bands"))
	r.focus("Off")
	r.tt.Key(0, ui.KeySpace)
	r.tt.Until("on", func() bool {
		_, ok := access.Read(t, r.tt.Tester).Find(ui.RoleSwitch, "On")
		return ok
	})
	if n, _ := access.Read(t, r.tt.Tester).Find(ui.RoleSwitch, "On"); !n.Has(access.Checked) {
		t.Errorf("the switch is not checked: %+v", n)
	}
}

// Where playback has no equaliser the page says so and has no control.
func TestA11yEqualiserUnsupported(t *testing.T) {
	r := newRig(t, false, nil)
	r.tt.Until("page", r.tt.Shows("Not available on this platform yet"))
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleHeading, "Equaliser"); !ok {
		t.Error("no heading")
	}
	if len(tr.Roles(ui.RoleSlider)) != 0 || len(tr.Roles(ui.RoleSwitch)) != 0 {
		t.Errorf("controls on an unsupported platform:\n%s", tr.Dump())
	}
}

// A failed load shows an error block whose Retry is on the tree.
func TestA11yEqualiserLoadFailure(t *testing.T) {
	r := newRig(t, true, func(f *fake) { f.failInfo = errors.New("boom") })
	r.tt.Until("error", r.tt.Shows("Try again"))
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleButton, "Try again"); !ok {
		t.Errorf("no Try again button:\n%s", tr.Dump())
	}
	access.NoNestedControls(t, r.tt.Tester)
	access.TabWalk(t, r.tt.Tester, 10)
}
