package updates

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The row's title is announced as the update turns up and as it changes,
// each once (the bell is always built, the panel only while open), with View
// a button Tab reaches; a failed open toasts, and the toast is announced.
func TestA11yUpdateIsAnnouncedAndHasAReachableButton(t *testing.T) {
	r := newRig(t)
	r.tt.Drain()
	r.tt.UseScreenReader()
	tt := r.tt.Tester
	if ann := r.tt.Announcements(); len(ann) != 0 {
		t.Errorf("announced with no update: %q", ann)
	}
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.0", Available: true, URL: "https://example.test/r"})
	if ann := r.tt.Announcements(); !slices.Equal(ann, []string{"Sona 0.3.0 is available"}) {
		t.Errorf("announcements %q, want the update once", ann)
	}
	access.NoNestedControls(t, tt)
	tr := access.Read(t, tt)
	if _, ok := tr.Find(ui.RoleGroup, "Sona 0.3.0 is available"); !ok {
		t.Fatalf("no group named by the row's title:\n%s", tr.Dump())
	}
	view, ok := tr.Find(ui.RoleButton, "View")
	if !ok || !view.Has(access.Focusable) {
		t.Errorf("View: %+v, want a focusable button", view)
	}
	stops := access.TabWalk(t, tt, 5)
	if len(stops) != 1 || stops[0].Node.Label != "View" {
		t.Errorf("Tab stops %v", stops)
	}

	// The same update is not announced again on a redraw.
	r.tt.Frame()
	if ann := r.tt.Announcements(); len(ann) != 0 {
		t.Errorf("announced again: %q", ann)
	}
	r.fail.Store(true)
	if err := r.tt.Click("View"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the failure toast", r.tt.Shows("Couldn't open the release"))
	if ann := r.tt.Announcements(); !slices.ContainsFunc(ann, func(s string) bool { return len(s) >= 25 && s[:25] == "Couldn't open the release" }) {
		t.Errorf("the failure is not announced: %q", ann)
	}

	// What changes the title is read once.
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.0", Available: true, Installing: true})
	if ann := r.tt.Announcements(); !slices.Equal(ann, []string{"Downloading Sona 0.3.0…"}) {
		t.Errorf("a download starting announced %q, want it once", ann)
	}
}
