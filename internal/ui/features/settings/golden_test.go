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
		themeID string // "" keeps Sona's own
		accent  string
	}{
		{"settings-general-light", "general", "Also show a desktop notification", nil, 1132, 1500, "", ""},
		{"settings-general-dark", "general", "Also show a desktop notification", []uitest.Option{uitest.Dark()}, 1132, 1500, "", ""},
		{"settings-appearance-light", "appearance", "Motion artwork", nil, 1132, 1700, "", ""},
		{"settings-appearance-dark", "appearance", "Motion artwork", []uitest.Option{uitest.Dark()}, 1132, 1700, "", ""},
		{"settings-appearance-accent-light", "appearance", "Motion artwork", nil, 1132, 1800, "sona", "rose"},
		{"settings-appearance-accent-dark", "appearance", "Motion artwork", []uitest.Option{uitest.Dark()}, 1132, 1800, "oled", "lime"},
		{"settings-appearance-accent-narrow-dark", "appearance", "Motion artwork", []uitest.Option{uitest.Dark()}, 940, 1900, "oled", "lime"},
		{"settings-appearance-accent-wide-light", "appearance", "Motion artwork", nil, 1600, 1700, "sona", "rose"},
		{"settings-appearance-accent-contrast-narrow-light", "appearance", "Motion artwork", nil, 940, 1900, "contrast", "teal"},
		{"settings-playback-light", "playback", "Crossfade", nil, 1132, 1253, "", ""},
		{"settings-general-narrow-dark", "general", "Also show a desktop notification", []uitest.Option{uitest.Dark()}, 707, 1300, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caps := linuxCaps()
			r := newRigCaps(t, tc.width, tc.height, nil, &caps, tc.opts...)
			withFakeThemes(r.s)
			if tc.themeID != "" {
				t.Cleanup(func() { theme.SetTheme(theme.DefaultThemeID); theme.SetAccent("") })
				theme.SetTheme(tc.themeID)
				theme.SetAccent(tc.accent)
			}
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
