package settings

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The page's sections over a Linux desktop where everything works, light
// and dark: the pictures that say the look did not move by accident.
// `go test ./internal/ui/features/settings -update` accepts a change on
// purpose.
func TestSettingsGolden(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		section string
		ready   string
		opts    []uitest.Option
		width   int
		height  int
	}{
		{"settings-general-light", "general", "Stay awake while playing", nil, 1132, 1253},
		{"settings-general-dark", "general", "Stay awake while playing", []uitest.Option{uitest.Dark()}, 1132, 1253},
		{"settings-appearance-light", "appearance", "Motion artwork", nil, 1132, 1253},
		{"settings-appearance-dark", "appearance", "Motion artwork", []uitest.Option{uitest.Dark()}, 1132, 1253},
		{"settings-playback-light", "playback", "Crossfade", nil, 1132, 1253},
		{"settings-general-narrow-dark", "general", "Stay awake while playing", []uitest.Option{uitest.Dark()}, 707, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caps := linuxCaps()
			r := newRigCaps(t, tc.width, tc.height, nil, &caps, tc.opts...)
			r.router.Replace(settingsPath + "?section=" + tc.section)
			r.tt.Until("the section", r.tt.Shows(tc.ready))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}

// Storage and Diagnostics over fakes: the figures and the verdict in the
// margin frame.
func TestSettingsGoldenSections(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	t.Run("settings-storage-light", func(t *testing.T) {
		f := &fakeStorage{usage: services.StorageUsage{
			ArtworkBytes: 128 * mebi, CatalogBytes: 3 * mebi, CatalogEntries: 42, DatabaseBytes: 20 * mebi, DatabaseFreeBytes: 2 * mebi,
		}}
		r := storageRig(t, f)
		r.tt.Until("usage", r.tt.Shows("Compact now"))
		r.tt.Settle()
		uitest.Golden(t, r.tt.Image(), "settings-storage-light")
	})
	t.Run("settings-diagnostics-light", func(t *testing.T) {
		g := newDiagRig(t, linuxCaps(), engineReport(), nil)
		g.router.Replace(settingsPath + "?section=diagnostics")
		g.tt.Until("the verdict", g.tt.Shows("Everything Sona needs is working"))
		g.tt.Settle()
		uitest.Golden(t, g.tt.Image(), "settings-diagnostics-light")
	})
}
