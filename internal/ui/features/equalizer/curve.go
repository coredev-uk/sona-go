package equalizer

import (
	"math"
	"slices"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// custom is the preset ID of the user's own curve (config.Equalizer.Custom).
const custom = "custom"

// gainsOf is the curve the setting describes: the selected preset's, or the
// user's own.
func gainsOf(eq config.Equalizer, presets []services.EqualizerPreset) []float64 {
	for _, p := range presets {
		if p.ID == eq.Preset {
			return p.Gains
		}
	}
	return eq.Custom
}

// withBand is eq with band i at db. Editing a preset starts the user's own
// curve from it; the preset itself never changes.
func withBand(eq config.Equalizer, presets []services.EqualizerPreset, i int, db float64) config.Equalizer {
	c := slices.Clone(gainsOf(eq, presets))
	if i >= len(c) {
		c = append(c, make([]float64, i+1-len(c))...)
	}
	c[i] = db
	eq.Preset, eq.Custom = custom, c
	return eq
}

// headroom is the automatic attenuation (dB, at most 0) that keeps the
// largest boost from clipping.
func headroom(gains []float64) float64 {
	top := 0.0
	for _, g := range gains {
		top = max(top, g)
	}
	return -top
}

// pregainOf is the level change ahead of the filters, as Go applies it
// (config.Equalizer.Pregain).
func pregainOf(eq config.Equalizer, gains []float64) float64 {
	if !eq.Enabled {
		return 0
	}
	return headroom(gains) + eq.Preamp
}

// formatDb writes a dB value as the faders label it: "+3", "0", "−4.5" (a
// true minus), in the shown language's digits.
func formatDb(db float64) string {
	r := math.Round(db*2) / 2
	if r == 0 {
		return "0"
	}
	sign := "+"
	if r < 0 {
		sign = "−"
	}
	return sign + i18n.FormatNumber(math.Abs(r))
}

// formatHz writes a band's frequency: "29", "947", "1.9k", "15k" (SI "k"
// for kHz).
func formatHz(hz float64) string {
	if hz < 1000 {
		return i18n.FormatNumber(math.Round(hz))
	}
	k := hz / 1000
	if k < 10 {
		return i18n.FormatNumber(math.Round(k*10)/10) + "k"
	}
	return i18n.FormatNumber(math.Round(k)) + "k"
}

// presetName is a preset's name in the shown language. Go names its
// built-in curves in English (internal/playback/equalizer); an ID this list
// lacks keeps Go's name.
func presetName(id, goName string) string {
	switch id {
	case custom:
		return i18n.T("Your curve")
	case "flat":
		// translators: Equaliser preset
		return i18n.T("Flat")
	case "bass-boost":
		return i18n.T("Bass Boost")
	case "bass-reducer":
		return i18n.T("Bass Reducer")
	case "treble-boost":
		return i18n.T("Treble Boost")
	case "treble-reducer":
		return i18n.T("Treble Reducer")
	case "vocal":
		// translators: Equaliser preset
		return i18n.T("Vocal")
	case "loudness":
		// translators: Equaliser preset
		return i18n.T("Loudness")
	case "acoustic":
		// translators: Equaliser preset
		return i18n.T("Acoustic")
	case "electronic":
		// translators: Equaliser preset
		return i18n.T("Electronic")
	case "hip-hop":
		// translators: Equaliser preset
		return i18n.T("Hip-Hop")
	case "rock":
		// translators: Equaliser preset
		return i18n.T("Rock")
	case "classical":
		// translators: Equaliser preset
		return i18n.T("Classical")
	case "jazz":
		// translators: Equaliser preset
		return i18n.T("Jazz")
	case "piano":
		// translators: Equaliser preset
		return i18n.T("Piano")
	case "spoken-word":
		return i18n.T("Spoken Word")
	}
	return goName
}
