package updates

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The notice is a status named by its sentence, announced as it appears
// (v0.2.0's role="status" live region), with View a button beside the text
// that Tab reaches; a failed open toasts, and the toast is announced.
func TestA11yNoticeIsAnnouncedAndHasAReachableButton(t *testing.T) {
	r := newRig(t)
	r.tt.Drain()
	tt := r.tt.Tester
	if ann := r.tt.Announcements(); len(ann) != 0 {
		t.Errorf("announced with no update: %q", ann)
	}
	r.emit(services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.0", Available: true, URL: "https://example.test/r"})
	if ann := r.tt.Announcements(); !slices.Contains(ann, "Sona 0.3.0 is available") {
		t.Errorf("announcements %q", ann)
	}
	access.NoNestedControls(t, tt)
	tr := access.Read(t, tt)
	st, ok := tr.Find(ui.RoleStatus, "Sona 0.3.0 is available")
	if !ok {
		t.Fatalf("no status:\n%s", tr.Dump())
	}
	view, ok := tr.Find(ui.RoleButton, "View")
	if !ok || !view.Has(access.Focusable) || view.Parent != st.Index {
		t.Errorf("View: %+v", view)
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
}
