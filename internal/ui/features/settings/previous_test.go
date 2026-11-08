package settings

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
	"github.com/coredev-uk/sona-go/internal/update"
)

// kept is a Diagnostics rig on a Mac whose last update kept version.
func kept(t *testing.T, version string, setup func(*diagPrevious)) *diagRig {
	t.Helper()
	caps := linuxCaps()
	caps.OS = "darwin"
	g := newDiagRig(t, caps, engineReport(), func(g *diagRig) {
		g.prev.prev = services.PreviousVersion{Available: true, Version: version}
		if setup != nil {
			setup(g.prev)
		}
	})
	g.tt.Until("the row", g.tt.Shows("Previous version", "Restore…"))
	g.reveal("previous-version")
	return g
}

// confirm answers the pending question yes (the confirming button has the
// focus).
func (g *diagRig) confirm() {
	g.t.Helper()
	if !kit.ConfirmPending() {
		g.t.Fatal("no question pending")
	}
	g.tt.Key(0, ui.KeyEnter)
	g.tt.Drain()
}

func TestPreviousVersionIsHiddenWhereNoneIsKept(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil) // Linux, a development build, nothing kept
	g.tt.Until("the page", g.tt.Shows("Everything Sona needs is working", "Developer tools"))
	if g.tt.HasText("Previous version") || g.tt.HasText("Restore…") {
		t.Errorf("the row shows with nothing to restore: %q", g.tt.Texts())
	}
	if g.s.seen["previous-version"] {
		t.Error("the row was built")
	}
}

func TestPreviousVersionSaysWhichVersionIsKept(t *testing.T) {
	g := kept(t, "0.3.0-rc.4", nil)
	g.tt.Until("the version", g.tt.Shows("0.3.0-rc.4 is kept from your last update."))
	if !g.s.seen["previous-version"] {
		t.Error("no previous-version row")
	}
}

func TestPreviousVersionWithoutANumberStillOffersTheRestore(t *testing.T) {
	g := kept(t, "", nil)
	g.tt.Until("the row", g.tt.Shows("The version from your last update is kept."))
	g.click("Restore…")
	g.tt.Until("the question", g.tt.Shows("Go back to the previous version?",
		"Sona restarts on the previous version. Your settings, sign-in and library are kept."))
}

func TestRestoreAsksFirstAndCancelRestoresNothing(t *testing.T) {
	g := kept(t, "0.3.0-rc.4", nil)
	g.click("Restore…")
	g.tt.Until("the question", g.tt.Shows("Go back to 0.3.0-rc.4?",
		"Sona restarts on 0.3.0-rc.4. Your settings, sign-in and library are kept.", "Restore and restart", "Cancel"))
	if n := g.prev.restored(); n != 0 {
		t.Fatalf("restored %d times before the answer", n)
	}
	g.click("Cancel")
	g.tt.Until("closed", func() bool { return !kit.ConfirmPending() })
	if n := g.prev.restored(); n != 0 {
		t.Errorf("restored %d times after Cancel", n)
	}
}

func TestConfirmingRestoresAndStaysBusyWhileSonaRestarts(t *testing.T) {
	hold := make(chan struct{})
	g := kept(t, "0.3.0-rc.4", func(p *diagPrevious) { p.hold = hold })
	g.click("Restore…")
	g.tt.Until("the question", g.tt.Shows("Go back to 0.3.0-rc.4?"))
	g.confirm()
	g.tt.Until("called", func() bool { return g.prev.restored() == 1 })
	g.tt.Until("busy", g.tt.Shows("Go back to 0.3.0-rc.4?", "Restoring…"))
	if !kit.ConfirmPending() {
		t.Fatal("the question closed while it restores")
	}
	close(hold) // it worked: Sona is quitting, so the question stays up
	g.tt.Drain()
	if !kit.ConfirmPending() || !g.tt.HasText("Restoring…") {
		t.Error("the question went away before Sona restarted")
	}
	if n := g.prev.restored(); n != 1 {
		t.Errorf("restored %d times", n)
	}
}

func TestAFailedRestoreSaysWhyAndCanBeTriedAgain(t *testing.T) {
	hold := make(chan struct{})
	g := kept(t, "0.3.0-rc.4", func(p *diagPrevious) {
		p.hold = hold
		p.err = fmt.Errorf("%w: rename: permission denied", update.ErrRestore)
	})
	g.click("Restore…")
	g.tt.Until("the question", g.tt.Shows("Go back to 0.3.0-rc.4?"))
	g.confirm()
	g.tt.Until("busy", g.tt.Shows("Restoring…"))
	hold <- struct{}{} // it fails

	const why = "Couldn't restore the previous version. Sona couldn't put the previous version back. " +
		"Check that it can write where it is installed, then try again."
	g.tt.Until("the reason", g.tt.Shows(why))
	if kit.ConfirmPending() {
		t.Error("the question stays up after the failure")
	}
	tr := access.Read(t, g.tt.Tester)
	if _, ok := tr.Find(ui.RoleAlert, why); !ok {
		t.Errorf("the reason is not an alert:\n%s", tr.Dump())
	}
	if g.tt.HasText("Restoring…") {
		t.Error("the row stays busy")
	}
	g.prev.mu.Lock()
	asked := g.prev.asked
	g.prev.mu.Unlock()
	if asked < 2 {
		t.Errorf("the kept version was asked %d times: not read again after the failure", asked)
	}

	// Pressing Restore… again clears the reason and tries again.
	g.prev.set(func(p *diagPrevious) { p.err = nil })
	g.reveal("previous-version")
	g.click("Restore…")
	g.tt.Until("the question again", g.tt.Shows("Go back to 0.3.0-rc.4?"))
	g.confirm()
	g.tt.Until("second call", func() bool { return g.prev.restored() == 2 })
	g.tt.Until("busy again", g.tt.Shows("Restoring…"))
	if g.tt.HasText(why) {
		t.Error("the old reason stays while it tries again")
	}
	close(hold)
}
