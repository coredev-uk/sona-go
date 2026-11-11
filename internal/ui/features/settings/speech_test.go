package settings

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// MyGo reads a status when its text changes (v0.9.0), so a line that is
// also announced by hand says everything twice. With a screen reader on,
// the updates row says each change once, and says the words, not the
// buttons beside them.
func TestA11yUpdatesRowIsReadOncePerChange(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.tt.UseScreenReader()
	g.tt.Announcements()

	store.Set(g.h.Env.Queries, store.KeyUpdates, services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true, SelfUpdate: true})
	g.until("available", g.tt.Shows("Sona 1.1.0 is available", "Install and restart"))
	g.tt.Drain()
	want := "Sona 1.1.0 is available Install it now and Sona restarts into it. Your settings, sign-in and queue stay as they are."
	if got := g.tt.Announcements(); !slices.Equal(got, []string{want}) {
		t.Errorf("the update turning up announced %q, want once %q", got, want)
	}

	store.Set(g.h.Env.Queries, store.KeyUpdates, services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true, SelfUpdate: true, Installing: true})
	g.until("downloading", g.tt.Shows("Downloading Sona 1.1.0…"))
	g.tt.Drain()
	if got := g.tt.Announcements(); !slices.Equal(got, []string{"Downloading Sona 1.1.0…"}) {
		t.Errorf("the download starting announced %q, want once %q", got, "Downloading Sona 1.1.0…")
	}
}

// A button's label changing beside the status ("Check now", "Checking…")
// is not news.
func TestA11yUpdatesRowIsSilentWhenOnlyAButtonChanges(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.up.status = services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true}
	store.Set(g.h.Env.Queries, store.KeyUpdates, g.up.status)
	g.until("verdict", g.tt.Shows("Sona 1.1.0 is available", "Check now"))
	g.tt.UseScreenReader()
	g.tt.Announcements()
	g.toEnd()
	g.click("Check now")
	g.until("done", g.tt.Shows("Check now"))
	g.tt.Drain()
	if got := g.tt.Announcements(); len(got) != 0 {
		t.Errorf("the Check now button changing announced %q", got)
	}
}
