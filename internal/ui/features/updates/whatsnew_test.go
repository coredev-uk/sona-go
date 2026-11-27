package updates

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

var justInstalled = services.WhatsNew{
	Version: "0.3.0", Published: published,
	Notes: "### Features\n\n* **library:** one place for **favourite** songs ([#621](https://github.com/coredev-uk/sona-go/issues/621))",
}

// seen records what the module told Go it showed.
type seen struct {
	mu       sync.Mutex
	versions []string
}

func (s *seen) mark(v string) { s.mu.Lock(); s.versions = append(s.versions, v); s.mu.Unlock() }
func (s *seen) got() []string { s.mu.Lock(); defer s.mu.Unlock(); return slices.Clone(s.versions) }

func newWhatsNewRig(t *testing.T) (*rig, *seen) {
	t.Helper()
	r := newRig(t)
	s := &seen{}
	r.u.markSeen = s.mark
	r.tt.SetSize(900, 700)
	return r, s
}

// The notes Go sends open the dialog: the version, the date, the notes and
// Close, and nothing that acts on an update. Showing it marks the version
// seen, once; Close ends it and it does not come back.
func TestWhatsNewOpensOnceWithTheNotesAndClose(t *testing.T) {
	r, s := newWhatsNewRig(t)
	r.tt.Emit(services.EventWhatsNew, justInstalled)
	for _, w := range []string{"What's New in Sona 0.3.0", "Released", "Features", "favourite", "Close"} {
		if !r.tt.HasText(w) {
			t.Errorf("the dialog lacks %q: %q", w, r.tt.Texts())
		}
	}
	for _, w := range []string{"Install and restart", "View release"} {
		if r.tt.HasText(w) {
			t.Errorf("the dialog has %q, which is for an update on offer", w)
		}
	}
	if got := s.got(); !slices.Equal(got, []string{"0.3.0"}) {
		t.Fatalf("marked seen %q, want 0.3.0 once", got)
	}
	r.tt.Frame()
	r.tt.Frame()
	if got := s.got(); len(got) != 1 {
		t.Errorf("marked seen %q after redraws", got)
	}
	if err := r.tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	r.tt.Advance(300e6)
	if r.tt.HasText("What's New in Sona 0.3.0") {
		t.Fatalf("Close left the dialog open: %q", r.tt.Texts())
	}
	r.tt.Frame()
	if r.tt.HasText("What's New in Sona 0.3.0") || len(s.got()) != 1 {
		t.Errorf("the dialog came back, or was marked again: %q, %q", r.tt.Texts(), s.got())
	}
}

// What's New waits for the first-run setup, the account dialog and an update's
// release notes, and opens when the window is idle.
func TestWhatsNewWaitsForTheWindowToBeIdle(t *testing.T) {
	for _, tc := range []struct {
		name         string
		offer        bool
		busy, finish func(*store.Surfaces)
	}{
		{"the first-run setup", false, func(s *store.Surfaces) { s.OpenSetup(store.SetupWelcome) }, (*store.Surfaces).CloseSetup},
		{"the account dialog", false, func(s *store.Surfaces) { s.OpenAccount() }, (*store.Surfaces).CloseAccount},
		{"the release notes of an update", true, (*store.Surfaces).OpenReleaseNotes, (*store.Surfaces).CloseReleaseNotes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, s := newWhatsNewRig(t)
			if tc.offer {
				r.offer(services.UpdateStatus{Notes: "### Fixes\n\n* an update fix\n"}) // the release notes close with no update on offer
			}
			tc.busy(r.h.Env.Surfaces)
			r.tt.Emit(services.EventWhatsNew, justInstalled)
			r.tt.Frame()
			if r.tt.HasText("favourite") || len(s.got()) != 0 {
				t.Fatalf("opened over %s: %q, marked %q", tc.name, r.tt.Texts(), s.got())
			}
			tc.finish(r.h.Env.Surfaces)
			r.tt.Frame()
			if !r.tt.HasText("favourite") || !slices.Equal(s.got(), []string{"0.3.0"}) {
				t.Errorf("did not open when idle: %q, marked %q", r.tt.Texts(), s.got())
			}
		})
	}
}

// Held back by another dialog, the notes ask again on a timer, not on every
// frame: the window is not redrawn for as long as that dialog is open.
func TestWhatsNewBlockedByADialogDoesNotRedrawEveryFrame(t *testing.T) {
	r, s := newWhatsNewRig(t)
	var open atomic.Bool
	var asks atomic.Int32
	open.Store(true)
	r.u.dialogShown = func() bool { asks.Add(1); return open.Load() }
	r.tt.Emit(services.EventWhatsNew, justInstalled)
	r.tt.Drain()
	before := asks.Load()
	for range 25 { // 400 ms in 16 ms steps: 25 frames if one were drawn per step
		r.tt.Advance(16e6)
	}
	// Each build asks twice (idle and its reason); a recheck is due at 500 ms.
	if n := asks.Load() - before; n > 4 {
		t.Errorf("the dialog was asked about %d times in 400 ms: it redraws every frame", n)
	}
	if r.tt.HasText("favourite") || len(s.got()) != 0 {
		t.Fatal("opened over another dialog")
	}
	open.Store(false)
	r.tt.Advance(600e6)
	if !r.tt.HasText("favourite") || !slices.Equal(s.got(), []string{"0.3.0"}) {
		t.Errorf("did not open after the other dialog closed: %q, marked %q", r.tt.Texts(), s.got())
	}
}

// Settings opens the running version's notes at once, and that is not "the
// dialog after an update": nothing is marked.
func TestWhatsNewOpenedBySettingsMarksNothing(t *testing.T) {
	r, s := newWhatsNewRig(t)
	r.h.Env.Surfaces.OpenWhatsNew(justInstalled)
	r.tt.Frame()
	if !r.tt.HasText("What's New in Sona 0.3.0") || !r.tt.HasText("favourite") {
		t.Fatalf("the dialog did not open: %q", r.tt.Texts())
	}
	if got := s.got(); len(got) != 0 {
		t.Errorf("marked seen %q", got)
	}
}

func TestA11yWhatsNewIsANamedDialogAndEscapeClosesIt(t *testing.T) {
	r, _ := newWhatsNewRig(t)
	r.tt.UseScreenReader()
	r.tt.Emit(services.EventWhatsNew, justInstalled)
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleDialog, "What's New in Sona 0.3.0"); !ok {
		t.Fatalf("no dialog named by its title:\n%s", tr.Dump())
	}
	access.NoNestedControls(t, r.tt.Tester)
	stops := access.StopLabels(access.TabWalk(t, r.tt.Tester, 8))
	if !slices.Contains(stops, "Close") {
		t.Errorf("Tab stops %q lack Close", stops)
	}
	for range 2 { // the walk may be on a link, whose tooltip Escape dismisses first
		r.tt.Key(0, ui.KeyEscape)
		r.tt.Advance(300e6)
	}
	if r.tt.HasText("What's New in Sona 0.3.0") {
		t.Fatal("Escape did not close the dialog")
	}
}
