package equalizer

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The page with the Rock curve on and a preamp trim, light and dark, at the
// size of the page column of a 1132×1253 window: the few pictures that say
// the look did not move by accident. `go test ./internal/ui/features/equalizer
// -update` accepts a change on purpose.
func TestPageGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []uitest.Option
	}{
		{"equalizer-light", nil},
		{"equalizer-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, true, func(f *fake) {
				eq := &f.settings.Audio.Equalizer
				eq.Enabled, eq.Preset, eq.Preamp = true, "rock", -2
			}, tc.opts...)
			r.tt.Until("the page", r.tt.Shows("Rock · shapes everything Sona plays", "Preamp"))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}

// Off, the curve and the faders are drawn quieter but stay editable.
func TestOffPageGolden(t *testing.T) {
	r := newRig(t, true, func(f *fake) {
		eq := &f.settings.Audio.Equalizer
		eq.Preset, eq.Custom = custom, []float64{3, 2, 1, 0, -1, -2, 0, 1, 2, 3}
	})
	r.tt.Until("the page", r.tt.Shows("Off · Sona plays the music as mastered", "Preamp"))
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "equalizer-off-light")
}

// At the narrowest the window and sidebar leave the page (940 wide with the
// sidebar's 233 and the gutters): the chips wrap onto more rows and the ten
// faders share what is left.
func TestNarrowPageGolden(t *testing.T) {
	r := newRigAt(t, 707, true, func(f *fake) {
		eq := &f.settings.Audio.Equalizer
		eq.Enabled, eq.Preset = true, "bass-boost"
	})
	r.tt.Until("the page", r.tt.Shows("Bass Boost · shapes everything Sona plays", "Preamp"))
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "equalizer-narrow-light")
}

// Where playback has no equaliser the panel's place says so.
func TestUnavailablePageGolden(t *testing.T) {
	r := newRig(t, false, nil, uitest.Dark())
	r.tt.Until("the notice", r.tt.Shows("Not available on this platform yet"))
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "equalizer-unavailable-dark")
}
