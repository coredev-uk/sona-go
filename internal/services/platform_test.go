package services

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/discord"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/playback"
)

// A platform with no integrations and no engine (macOS and Windows today)
// reports every capability absent, so the frontend hides those controls.
func TestCapabilitiesWithoutPlatformSupport(t *testing.T) {
	eng := playback.NewUnsupported()
	t.Cleanup(func() { _ = eng.Close() })
	got := newPlatformService(&Env{Deps: Deps{Engine: eng}}).Capabilities()
	want := Capabilities{OS: runtime.GOOS, DiscordPresence: discord.Supported}
	if got != want {
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
	got := newPlatformService(&Env{Deps: Deps{Engine: eng, Platform: plat}}).Capabilities()
	if !got.Playback.Available || got.Playback.Engine != playback.KindCDM || !got.Playback.Volume || got.Playback.Equalizer {
		t.Errorf("playback = %+v, want the engine's capabilities", got.Playback)
	}
	if !got.MediaControls || !got.Autostart || !got.FileChooser || !got.Tray || got.Notifications || got.SleepInhibit || got.PowerState || got.CLI {
		t.Errorf("capabilities = %+v, want exactly the platform's seams", got)
	}
}

func TestUnsupportedIsItsOwnErrorCode(t *testing.T) {
	for _, err := range []error{
		playback.ErrUnsupported,
		fmt.Errorf("coordinator: play: %w", playback.ErrUnsupported),
		unsupported("the file chooser"),
		discord.ErrUnsupported,
	} {
		if got := ErrorCode(err); got != CodeUnsupported {
			t.Errorf("ErrorCode(%v) = %q, want %q", err, got, CodeUnsupported)
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
	d := &DesktopService{o: s.o}
	if _, err := d.ChooseCDMPath(context.Background()); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("ChooseCDMPath = %v, want unsupported", err)
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
