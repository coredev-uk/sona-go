package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/config"
)

// desktopEnv builds a DesktopService over a fresh settings store with the
// autostart directory in a temporary XDG_CONFIG_HOME. It is wired like
// newDesktopService, with the start-up reconcile run synchronously.
func desktopEnv(t *testing.T, prepare func(*config.Settings)) (*DesktopService, *config.Store, string) {
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
	e := &Env{Deps: Deps{Settings: st, Emit: &recorder{}, Logger: discard()}}
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
