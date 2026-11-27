package updates

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

type rig struct {
	h        *uitest.Harness
	tt       *uitest.Session
	u        *updates
	opens    atomic.Int32
	installs atomic.Int32
	fail     atomic.Bool
}

func newRig(t *testing.T) *rig {
	t.Helper()
	h := uitest.New(t)
	r := &rig{h: h}
	r.u = newUpdates(h.Env, func() error {
		r.opens.Add(1)
		if r.fail.Load() {
			return errors.New("no browser")
		}
		return nil
	}, func() error { r.installs.Add(1); return nil })
	m := r.u.module()
	m.Init()
	r.tt = h.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).Grow(1)
			ui.Column(c).Width(360).Children(func() {
				// As the notification centre builds it: Status on every
				// frame, the row while it is shown.
				if a := m.Slots.Activity; a.Status(c).Shown {
					a.Build(c)
				}
			})
			kit.Toaster(c)
			m.Slots.Layer(c)
		})
	}, 360, 300)
	return r
}

func TestNoticeShowsOnlyWhileANewerReleaseExists(t *testing.T) {
	r := newRig(t)
	r.tt.Drain()
	// Nothing published, or already the newest: no notice.
	if r.tt.HasText("Sona 0.2.0 is available") || r.tt.HasText("View") {
		t.Fatalf("a notice with no update: %q", r.tt.Texts())
	}
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.2.0", Available: false})
	if r.tt.HasText("Sona 0.2.0 is available") {
		t.Errorf("notice for the current release: %q", r.tt.Texts())
	}
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.0", Available: true, URL: "https://example.test/r"})
	for _, w := range []string{"Sona 0.3.0 is available", "Update it the way you installed Sona.", "View"} {
		if !r.tt.HasText(w) {
			t.Errorf("notice lacks %q: %q", w, r.tt.Texts())
		}
	}
	// A later result that finds nothing newer hides it again.
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.3.0", Checkable: true, Latest: "0.3.0"})
	if r.tt.HasText("Sona 0.3.0 is available") {
		t.Errorf("notice stays after the update: %q", r.tt.Texts())
	}
}

func TestSelfUpdatingInstallsInPlaceWithProgress(t *testing.T) {
	r := newRig(t)
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.0", Available: true, SelfUpdate: true})
	if !r.tt.HasText("Install and restart") || !r.tt.HasText("View release") || r.tt.HasText("Update it the way you installed Sona.") {
		t.Fatalf("notice = %q", r.tt.Texts())
	}
	if err := r.tt.Click("Install and restart"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the install to start", func() bool { return r.installs.Load() == 1 })
	// The events carry the download; the button gives way to the bar.
	r.h.Bus.Emit(services.EventUpdateInstall, services.UpdateStatus{Latest: "0.3.0", Available: true, SelfUpdate: true, Installing: true, Downloaded: 1, Total: 2})
	r.tt.Drain()
	if !r.tt.HasText("Downloading Sona 0.3.0…") || r.tt.HasText("Install and restart") {
		t.Fatalf("installing = %q", r.tt.Texts())
	}
	r.h.Bus.Emit(services.EventUpdateInstall, services.UpdateStatus{Latest: "0.3.0", Available: true, SelfUpdate: true, Installed: true})
	r.tt.Drain()
	if !r.tt.HasText("Restarting into Sona 0.3.0…") {
		t.Fatalf("installed = %q", r.tt.Texts())
	}
	r.h.Bus.Emit(services.EventUpdateInstall, services.UpdateStatus{Latest: "0.3.0", Available: true, SelfUpdate: true, InstallError: services.CodeUpdateFailed})
	r.tt.Drain()
	// A failure says so and offers a way out besides retrying: the release
	// page. "Install and restart" is not offered again as if nothing happened.
	if !r.tt.HasText("Couldn't install Sona 0.3.0") || !r.tt.HasText("Try again") || !r.tt.HasText("View release") || r.tt.HasText("Install and restart") {
		t.Fatalf("failed = %q", r.tt.Texts())
	}
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the second install to start", func() bool { return r.installs.Load() == 2 })
	if err := r.tt.Click("View release"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the release to open", func() bool { return r.opens.Load() == 1 })
}

func TestViewOpensTheReleaseAndAFailureToasts(t *testing.T) {
	r := newRig(t)
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Latest: "0.3.0", Available: true})
	if err := r.tt.Click("View"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the release to open", func() bool { return r.opens.Load() == 1 })
	if r.tt.HasText("Couldn't open the release") {
		t.Error("a toast for a release that opened")
	}
	r.fail.Store(true)
	if err := r.tt.Click("View"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the failure toast", r.tt.Shows("Couldn't open the release"))
}

// After "Restore…" in Settings → Diagnostics the restored version doesn't
// nag about the version it came back from; a newer one is still offered.
func TestNoticeSkipsTheVersionTheUserWentBackFrom(t *testing.T) {
	r := newRig(t)
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.0", Available: true, Declined: "0.3.0"})
	if r.tt.HasText("Sona 0.3.0 is available") {
		t.Fatalf("the notice offers the version just left: %q", r.tt.Texts())
	}
	r.tt.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.1", Available: true, Declined: "0.3.0"})
	if !r.tt.HasText("Sona 0.3.1 is available") {
		t.Fatalf("a newer release is held back too: %q", r.tt.Texts())
	}
}
