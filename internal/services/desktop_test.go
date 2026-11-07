package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/config"
)

// desktopEnv builds a DesktopService over a fresh settings store with the
// XDG autostart entry (Linux's) in a temporary XDG_CONFIG_HOME. It is
// wired like newDesktopService, with the start-up reconcile run
// synchronously.
func desktopEnv(t *testing.T, prepare func(*config.Settings)) (*DesktopService, *config.Store, string) {
	return desktopEnvWith(t, Platform{Autostart: app.XDGAutostart{}}, prepare)
}

func desktopEnvWith(t *testing.T, plat Platform, prepare func(*config.Settings)) (*DesktopService, *config.Store, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPIMAGE", "")
	entry, err := app.AutostartPath()
	if err != nil {
		t.Fatal(err)
	}
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if prepare != nil {
		if _, err := st.Update(prepare); err != nil {
			t.Fatal(err)
		}
	}
	e := &Env{Deps: Deps{Settings: st, Emit: &recorder{}, Logger: discard(), Platform: plat}}
	t.Cleanup(e.close)
	d := &DesktopService{o: e}
	st.OnChange(d.settingsChanged)
	d.reconcileAutostart()
	return d, st, entry
}

func readEntry(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("autostart entry: %v", err)
	}
	return string(b)
}

func TestAutostartFollowsTheSettings(t *testing.T) {
	_, st, entry := desktopEnv(t, nil)
	if app.AutostartExists(entry) {
		t.Fatal("entry exists before autostart was turned on")
	}

	if _, err := st.Update(func(s *config.Settings) { s.Desktop.Autostart = true }); err != nil {
		t.Fatal(err)
	}
	// Defaults start hidden in the tray.
	if body := readEntry(t, entry); !strings.Contains(body, " "+app.HiddenFlag+"\n") {
		t.Fatalf("entry does not start hidden:\n%s", body)
	}
	if _, err := st.Update(func(s *config.Settings) { s.Desktop.StartHidden = false }); err != nil {
		t.Fatal(err)
	}
	if body := readEntry(t, entry); strings.Contains(body, app.HiddenFlag) {
		t.Fatalf("entry still starts hidden:\n%s", body)
	}
	if _, err := st.Update(func(s *config.Settings) { s.Desktop.Autostart = false }); err != nil {
		t.Fatal(err)
	}
	if app.AutostartExists(entry) {
		t.Fatal("entry left after turning autostart off")
	}
}

func TestAutostartRemovedOutsideSonaTurnsTheSettingOff(t *testing.T) {
	// The setting says on, but there is no entry (removed in the
	// desktop's startup settings): the file wins.
	_, st, _ := desktopEnv(t, func(s *config.Settings) { s.Desktop.Autostart = true })
	if st.Get().Desktop.Autostart {
		t.Fatal("autostart still on without an entry")
	}
}

func TestAutostartWithoutPlatformSupport(t *testing.T) {
	// A setting carried over from Linux (an imported settings file) is
	// turned off where there is no autostart; turning it on is refused and
	// reverted, so the toggle never shows an entry that doesn't exist.
	_, st, entry := desktopEnvWith(t, Platform{}, func(s *config.Settings) { s.Desktop.Autostart = true })
	if st.Get().Desktop.Autostart {
		t.Fatal("autostart still on without platform support")
	}
	if _, err := st.Update(func(s *config.Settings) { s.Desktop.Autostart = true }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for st.Get().Desktop.Autostart {
		if time.Now().After(deadline) {
			t.Fatal("turning autostart on was not reverted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if app.AutostartExists(entry) {
		t.Fatal("wrote an XDG entry without platform support")
	}
}

func TestSetLastRouteOnlyWhileResuming(t *testing.T) {
	d, st, _ := desktopEnv(t, nil)
	if err := d.SetLastRoute("/album/1"); err != nil {
		t.Fatal(err)
	}
	if got := st.Get().Desktop.LastRoute; got != "" {
		t.Fatalf("saved %q with resume off", got)
	}
	if _, err := st.Update(func(s *config.Settings) { s.Desktop.ResumeLastRoute = true }); err != nil {
		t.Fatal(err)
	}
	if err := d.SetLastRoute("/album/1?song=2"); err != nil {
		t.Fatal(err)
	}
	if got := st.Get().Desktop.StartRoute(); got != "/album/1?song=2" {
		t.Fatalf("start route = %q", got)
	}
}

// fakeWindows is the shell's window control: it records nothing but the
// developer tools' answer and the tray.
type fakeWindows struct {
	// devtoolsErr is OpenDeveloperTools' answer.
	devtoolsErr error
	// tray is HasTray's answer.
	tray bool
}

func (*fakeWindows) ShowMain()                   {}
func (*fakeWindows) ToggleMini()                 {}
func (*fakeWindows) CloseMini()                  {}
func (*fakeWindows) ResetWindowSize()            {}
func (f *fakeWindows) HasTray() bool             { return f.tray }
func (*fakeWindows) DeveloperTools() bool        { return false }
func (f *fakeWindows) OpenDeveloperTools() error { return f.devtoolsErr }

func windowsEnv(t *testing.T, w *fakeWindows) (*DesktopService, *recorder) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	e := &Env{Deps: Deps{Settings: st, Emit: rec, Logger: discard(), Windows: w}}
	t.Cleanup(e.close)
	return newDesktopService(e), rec
}

// The inspector opens when the window has it on; otherwise the call says
// unsupported, which the page words as not available (it only offers the
// button while Capabilities.DeveloperTools holds).
func TestOpenDeveloperTools(t *testing.T) {
	d, _ := windowsEnv(t, &fakeWindows{})
	if err := d.OpenDeveloperTools(); err != nil {
		t.Fatalf("OpenDeveloperTools = %v", err)
	}
	d, _ = windowsEnv(t, &fakeWindows{devtoolsErr: errors.ErrUnsupported})
	if err := d.OpenDeveloperTools(); CodeOf(err) != CodeUnsupported {
		t.Fatalf("OpenDeveloperTools = %v (code %q), want code %q", err, CodeOf(err), CodeUnsupported)
	}
}

// holdEmitter records the settings events and holds the announcement that
// follows the observer's own (the second one saying autostart is on) until
// the undo's has been delivered, or a moment has passed. Where the
// announcement is not ordered with its save the undo overtakes it; where it
// is, the undo cannot start before it is delivered and the wait runs out.
type holdEmitter struct {
	mu       sync.Mutex
	settings []config.Settings
	on       int
}

func (h *holdEmitter) Emit(name string, data ...any) bool {
	if name != EventSettingsChanged {
		return false
	}
	s := data[0].(config.Settings)
	h.mu.Lock()
	hold := false
	if s.Desktop.Autostart {
		h.on++
		hold = h.on == 2
	}
	h.mu.Unlock()
	if hold {
		for deadline := time.Now().Add(250 * time.Millisecond); time.Now().Before(deadline) && !h.sawOff(); {
			time.Sleep(time.Millisecond)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.settings = append(h.settings, s)
	return false
}

func (h *holdEmitter) sawOff() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.settings) > 0 && !h.settings[len(h.settings)-1].Desktop.Autostart
}

func (h *holdEmitter) last() config.Settings {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.settings[len(h.settings)-1]
}

// Turning autostart on where there is no backend is refused and undone on
// another goroutine. The UI keeps the latest settings:changed, so the last
// event must be the undone setting: the one UpdateSettings announces may not
// arrive after the undo's.
func TestRefusedAutostartIsAnnouncedBeforeItsUndo(t *testing.T) {
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	em := &holdEmitter{}
	e := &Env{Deps: Deps{Settings: st, Emit: em, Logger: discard()}}
	newDesktopService(e)
	sys := &SystemService{o: e}

	next := st.Get()
	next.Desktop.Autostart = true
	if _, err := sys.UpdateSettings(next); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for st.Get().Desktop.Autostart {
		if time.Now().After(deadline) {
			t.Fatal("turning autostart on was not reverted")
		}
		time.Sleep(time.Millisecond)
	}
	// An Update returns once the undo, announcement included, is done.
	if _, err := st.Update(func(*config.Settings) {}); err != nil {
		t.Fatal(err)
	}
	if em.last().Desktop.Autostart {
		t.Fatal("the last settings:changed still says autostart is on")
	}
	e.close()
}

// fakeLinks is a desktop that keeps its own choice (keeps) or gives Sona the
// links when asked.
type fakeLinks struct {
	sona  bool
	other string
	keeps bool
}

func (f *fakeLinks) Default() (bool, string, error) { return f.sona, f.other, nil }

func (f *fakeLinks) MakeDefault(context.Context) error {
	if !f.keeps {
		f.sona, f.other = true, ""
	}
	return nil
}

// The answer to a change is what the desktop says afterwards: it may have
// asked the user, who said no.
func TestSetLinkHandlerReportsWhatTheDesktopDid(t *testing.T) {
	for _, tc := range []struct {
		name string
		keep bool
		want LinkHandlerStatus
	}{
		{"taken", false, LinkHandlerStatus{Sona: true}},
		{"kept by the desktop", true, LinkHandlerStatus{Other: "Music"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _, _ := desktopEnvWith(t, Platform{LinkHandler: &fakeLinks{other: "Music", keeps: tc.keep}}, nil)
			got, err := d.SetLinkHandler(context.Background())
			if err != nil || got != tc.want {
				t.Fatalf("SetLinkHandler = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestLinkHandlerWithoutPlatformSupportIsUnsupported(t *testing.T) {
	d, _, _ := desktopEnvWith(t, Platform{}, nil)
	if _, err := d.LinkHandler(); CodeOf(err) != CodeUnsupported {
		t.Errorf("LinkHandler: %v, want code unsupported", err)
	}
	if _, err := d.SetLinkHandler(context.Background()); CodeOf(err) != CodeUnsupported {
		t.Errorf("SetLinkHandler: %v, want code unsupported", err)
	}
}
