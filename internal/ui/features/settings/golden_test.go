package settings

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The page's sections over a Linux desktop where everything works, light
// and dark: the pictures that say the look did not move by accident.
// `go test ./internal/ui/features/settings -update` accepts a change on
// purpose.
func TestSettingsGolden(t *testing.T) {
	for _, tc := range []struct {
		name    string
		section string
		ready   string
		opts    []uitest.Option
	}{
		{"settings-general-light", "general", "Stay awake while playing", nil},
		{"settings-general-dark", "general", "Stay awake while playing", []uitest.Option{uitest.Dark()}},
		{"settings-appearance-light", "appearance", "Motion artwork", nil},
		{"settings-appearance-dark", "appearance", "Motion artwork", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caps := linuxCaps()
			r := newRigCaps(t, 1132, 1253, nil, &caps, tc.opts...)
			r.router.Replace(settingsPath + "?section=" + tc.section)
			r.tt.Until("the section", r.tt.Shows(tc.ready))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}
