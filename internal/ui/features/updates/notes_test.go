package updates

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

const notes = "## [0.3.0](https://github.com/coredev-uk/sona-go/compare/v0.2.0...v0.3.0) (2026-11-26)\n\n### Features\n\n* **library:** one place for **favourite** songs ([#621](https://github.com/coredev-uk/sona-go/issues/621))\n"

// published is 2026-11-26 12:00 UTC, in Unix milliseconds.
const published = 1795694400000

func (r *rig) offer(st services.UpdateStatus) {
	r.tt.SetSize(900, 700)
	st.Current, st.Checkable, st.Latest, st.Available = "0.2.0", true, "0.3.0", true
	r.tt.Emit(services.EventUpdateAvailable, st)
}

// With notes the row has What's new, and without them it is as it was.
func TestRowOffersWhatsNewOnlyWithNotes(t *testing.T) {
	for _, tc := range []struct {
		name string
		st   services.UpdateStatus
		want []string
		not  []string
	}{
		{"self-updating", services.UpdateStatus{SelfUpdate: true, Notes: notes}, []string{"Install and restart", "What's new"}, []string{"View release"}},
		{"self-updating, no notes", services.UpdateStatus{SelfUpdate: true}, []string{"Install and restart", "View release"}, []string{"What's new"}},
		{"elsewhere", services.UpdateStatus{Notes: notes}, []string{"What's new", "View release"}, []string{"Install and restart"}},
		{"elsewhere, no notes", services.UpdateStatus{}, []string{"View"}, []string{"What's new", "View release"}},
		{"a failed install", services.UpdateStatus{SelfUpdate: true, Notes: notes, InstallError: services.CodeUpdateFailed}, []string{"Try again", "View release"}, []string{"What's new"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.offer(tc.st)
			for _, w := range tc.want {
				if !r.tt.HasText(w) {
					t.Errorf("row lacks %q: %q", w, r.tt.Texts())
				}
			}
			for _, w := range tc.not {
				if r.tt.HasText(w) {
					t.Errorf("row has %q: %q", w, r.tt.Texts())
				}
			}
		})
	}
}

func TestWhatsNewOpensTheNotesAndInstallFromThemStartsTheInstall(t *testing.T) {
	r := newRig(t)
	r.offer(services.UpdateStatus{SelfUpdate: true, Notes: notes, Published: published})
	if r.tt.HasText("What's new in Sona 0.3.0") {
		t.Fatal("the dialog is open before it was asked for")
	}
	if err := r.tt.Click("What's new"); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"What's new in Sona 0.3.0", "Released", "Features", "favourite", "Close", "View release"} {
		if !r.tt.HasText(w) {
			t.Errorf("the dialog lacks %q: %q", w, r.tt.Texts())
		}
	}
	// View release goes to the page and leaves the dialog.
	if err := r.tt.Click("View release"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the release to open", func() bool { return r.opens.Load() == 1 })
	if !r.tt.HasText("What's new in Sona 0.3.0") {
		t.Error("View release closed the dialog")
	}
	// Install and restart installs, and closes the dialog: the row carries the progress.
	if err := r.tt.Click("Install and restart"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the install to start", func() bool { return r.installs.Load() == 1 })
	r.tt.Advance(300e6)
	if r.tt.HasText("What's new in Sona 0.3.0") || r.h.Env.Surfaces.ReleaseNotesOpen() {
		t.Errorf("the dialog stays open over the install: %q", r.tt.Texts())
	}
}

// Where Sona doesn't update itself, the release page is the dialog's one
// action, and there is no install.
func TestNotesDialogWhereSonaDoesNotUpdateItselfLinksTheRelease(t *testing.T) {
	r := newRig(t)
	r.offer(services.UpdateStatus{Notes: notes})
	if err := r.tt.Click("What's new"); err != nil {
		t.Fatal(err)
	}
	if r.tt.HasText("Install and restart") || r.tt.HasText("Released") {
		t.Errorf("an install, or a date nobody gave: %q", r.tt.Texts())
	}
	if err := r.tt.Click("View release"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the release to open", func() bool { return r.opens.Load() == 1 })
}

func TestNotesLinksOpenThroughTheAppsOpenURL(t *testing.T) {
	r := newRig(t)
	r.offer(services.UpdateStatus{Notes: "See [the issue](https://github.com/coredev-uk/sona-go/issues/621) and [a file](file:///etc/passwd)."})
	if err := r.tt.Click("What's new"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"the issue", "a file"} {
		rect, ok := r.tt.Find(text)
		if !ok {
			t.Fatalf("no %q link: %q", text, r.tt.Texts())
		}
		r.tt.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	}
	if got := r.h.Opened(); !slices.Equal(got, []string{"https://github.com/coredev-uk/sona-go/issues/621"}) {
		t.Fatalf("opened %q", got)
	}
}

// The status is the dialog's source: a later check that finds nothing
// newer, or notes that change, are what it shows.
func TestNotesDialogFollowsTheStatus(t *testing.T) {
	r := newRig(t)
	r.offer(services.UpdateStatus{SelfUpdate: true, Notes: notes})
	if err := r.tt.Click("What's new"); err != nil {
		t.Fatal(err)
	}
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.1", Available: true, Notes: "### Fixes\n\n* a newer fix\n"})
	if !r.tt.HasText("What's new in Sona 0.3.1") || !r.tt.HasText("a newer fix") || r.tt.HasText("favourite") {
		t.Errorf("the dialog does not follow the new status: %q", r.tt.Texts())
	}
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.3.1", Checkable: true, Latest: "0.3.1"})
	if r.tt.HasText("What's new in Sona") || r.h.Env.Surfaces.ReleaseNotesOpen() {
		t.Errorf("the dialog stays for an update that is gone: %q", r.tt.Texts())
	}
}

// Settings reaches it for a version the notice doesn't offer (the one the
// user went back from).
func TestNotesOpenForAnUpdateTheNoticeHoldsBack(t *testing.T) {
	r := newRig(t)
	r.offer(services.UpdateStatus{Notes: notes, Declined: "0.3.0"})
	if r.tt.HasText("What's new") {
		t.Fatalf("the notice offers what was declined: %q", r.tt.Texts())
	}
	r.h.Env.Surfaces.OpenReleaseNotes()
	r.tt.Frame()
	if !r.tt.HasText("What's new in Sona 0.3.0") {
		t.Fatalf("the dialog does not open for a declined update: %q", r.tt.Texts())
	}
}

func TestA11yNotesDialogIsNamedAndFocusReturnsToTheRow(t *testing.T) {
	r := newRig(t)
	r.offer(services.UpdateStatus{SelfUpdate: true, Notes: notes, Published: published})
	r.tt.UseScreenReader()
	if err := r.tt.Click("What's new"); err != nil {
		t.Fatal(err)
	}
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleDialog, "What's new in Sona 0.3.0"); !ok {
		t.Fatalf("no dialog named by its title:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleHeading, "What's new in Sona 0.3.0"); !ok {
		t.Errorf("the title is not a heading:\n%s", tr.Dump())
	}
	access.NoNestedControls(t, r.tt.Tester)
	// Tab order: the notes, their links, Close, View release, Install and restart, the X.
	stops := access.StopLabels(access.TabWalk(t, r.tt.Tester, 12))
	if !slices.Contains(stops, "Release notes") || !slices.Contains(stops, "Install and restart") || !slices.Contains(stops, "Close") {
		t.Errorf("Tab stops %q lack the notes, Close or Install", stops)
	}
	// The walk has been on links, whose tooltips (the address) Escape dismisses
	// first, as it does anywhere in MyGo; the next one closes the dialog.
	for range 2 {
		r.tt.Key(0, ui.KeyEscape)
		r.tt.Advance(300e6)
	}
	if r.tt.HasText("What's new in Sona 0.3.0") {
		t.Fatal("Escape did not close the dialog")
	}
	if !r.tt.Focused("What's new") {
		t.Error("the focus did not return to the row's What's new")
	}
}
