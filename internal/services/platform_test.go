package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/discord"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// A platform with no integrations and no engine (macOS and Windows today)
// reports every capability absent, so the frontend hides those controls.
func TestCapabilitiesWithoutPlatformSupport(t *testing.T) {
	eng := playback.NewUnsupported()
	t.Cleanup(func() { _ = eng.Close() })
	got := newPlatformService(&Env{Deps: Deps{Engine: eng, Settings: testSettings(t)}}).Capabilities()
	want := Capabilities{
		OS: runtime.GOOS, DiscordPresence: discord.Supported,
		WindowControls: WindowControls{Style: config.TitleBarAuto, Start: []string{}, End: []string{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %+v, want %+v", got, want)
	}
}

func TestCapabilitiesFollowThePlatformAndEngine(t *testing.T) {
	plat := Platform{
		MediaControls: func(mpris.Player) (MediaControls, error) { return nil, nil },
		Autostart:     app.XDGAutostart{},
		Files:         app.Portal{},
		Tray:          true,
	}
	eng := &playEngine{}
	got := newPlatformService(&Env{Deps: Deps{Engine: eng, Platform: plat, Settings: testSettings(t)}}).Capabilities()
	if !got.Playback.Available || got.Playback.Engine != playback.KindCDM || !got.Playback.Volume || got.Playback.Equalizer {
		t.Errorf("playback = %+v, want the engine's capabilities", got.Playback)
	}
	if !got.MediaControls || !got.Autostart || !got.FileChooser || !got.Tray || got.Notifications || got.SleepInhibit || got.PowerState || got.CLI {
		t.Errorf("capabilities = %+v, want exactly the platform's seams", got)
	}
}

// The window controls carry the platform's title bar facts, and the
// desktop's button layout is read once, on the first call (after the GUI
// toolkit started), not when the service is built.
func TestCapabilitiesWindowControls(t *testing.T) {
	reads := 0
	plat := Platform{TitleBar: windowchrome.Platform{
		Native:     windowchrome.NativeFrame,
		Automatic:  config.TitleBarSona,
		Desktop:    "GNOME",
		WindowMenu: true,
		Layout: func() windowchrome.Layout {
			reads++
			return windowchrome.ParseGTK("close:minimize,maximize")
		},
	}}
	eng := playback.NewUnsupported()
	t.Cleanup(func() { _ = eng.Close() })
	svc := newPlatformService(&Env{Deps: Deps{Engine: eng, Platform: plat, Settings: testSettings(t)}})
	if reads != 0 {
		t.Fatalf("layout read %d times before Capabilities", reads)
	}
	want := WindowControls{
		Native: "frame", Automatic: "sona", Desktop: "GNOME", Style: "auto", WindowMenu: true,
		Start: []string{"close"}, End: []string{"minimize", "maximize"},
	}
	for range 2 {
		if got := svc.Capabilities().WindowControls; !reflect.DeepEqual(got, want) {
			t.Fatalf("window controls = %+v, want %+v", got, want)
		}
	}
	if reads != 1 {
		t.Errorf("layout read %d times, want once", reads)
	}
}

func TestUnsupportedIsItsOwnCode(t *testing.T) {
	for _, err := range []error{
		playback.ErrUnsupported,
		fmt.Errorf("coordinator: play: %w", playback.ErrUnsupported),
		unsupported("the file chooser"),
		discord.ErrUnsupported,
	} {
		if got := CodeOf(err); got != CodeUnsupported {
			t.Errorf("CodeOf(%v) = %q, want %q", err, got, CodeUnsupported)
		}
	}
}

func TestFileChooserUnsupported(t *testing.T) {
	s, _, _ := systemEnv(t, nil)
	if _, err := s.ExportSettings(context.Background()); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("ExportSettings = %v, want unsupported", err)
	}
	if _, err := s.ImportSettings(context.Background()); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("ImportSettings = %v, want unsupported", err)
	}
}

// Without a sleep inhibitor on the platform, wanting one reports it
// unavailable; not wanting one reports nothing held.
func TestInhibitWithoutPlatformSupport(t *testing.T) {
	s := &IntegrationsService{e: &Env{Deps: Deps{Logger: discard()}}}
	if got := s.inhibit(nil, true); got != IntegrationUnavailable {
		t.Errorf("inhibit(nil, true) = %q, want %q", got, IntegrationUnavailable)
	}
	if got := s.inhibit(nil, false); got != "" {
		t.Errorf("inhibit(nil, false) = %q, want none", got)
	}
}

// testSettings is a fresh settings store with the defaults.
func testSettings(t *testing.T) *config.Store {
	t.Helper()
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return st
}
