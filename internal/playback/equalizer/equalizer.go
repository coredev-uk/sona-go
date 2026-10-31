// Package equalizer holds the equaliser's bands and presets, shared by the
// settings, the service the frontend talks to, and the playback host child
// (GStreamer's equalizer-10bands, one peaking filter per band).
package equalizer

import "math"

// Bands is the number of bands.
const Bands = 10

// Frequencies are the band centres in Hz (equalizer-10bands).
var Frequencies = [Bands]float64{29, 59, 119, 237, 474, 947, 1889, 3770, 7523, 15011}

// Gain and preamp limits in dB. The element accepts −24..+12 dB per band;
// the UI offers a symmetric ±12 dB.
const (
	MinGain   = -12.0
	MaxGain   = 12.0
	MinPreamp = -12.0
	MaxPreamp = 0.0
)

// Preset IDs with special meaning.
const (
	Flat   = "flat"
	Custom = "custom" // the user's own curve
)

// Preset is a named curve.
type Preset struct {
	ID    string
	Name  string
	Gains [Bands]float64
}

// Curve is an equaliser setting as the playback host applies it: the
// pregain (applied ahead of the filters) and each band's gain, at
// Frequencies, in dB. The zero Curve is flat.
type Curve struct {
	Pregain float64
	Gains   [Bands]float64
}

// Presets are the built-in curves, in display order.
var Presets = []Preset{
	{Flat, "Flat", [Bands]float64{}},
	{"bass-boost", "Bass Boost", [Bands]float64{6, 5, 4, 2.5, 1, 0, 0, 0, 0, 0}},
	{"bass-reducer", "Bass Reducer", [Bands]float64{-6, -5, -4, -2.5, -1, 0, 0, 0, 0, 0}},
	{"treble-boost", "Treble Boost", [Bands]float64{0, 0, 0, 0, 0, 1, 2.5, 4, 5, 6}},
	{"treble-reducer", "Treble Reducer", [Bands]float64{0, 0, 0, 0, 0, -1, -2.5, -4, -5, -6}},
	{"vocal", "Vocal", [Bands]float64{-2, -3, -3, 1, 4, 4, 3, 1, 0, -2}},
	{"loudness", "Loudness", [Bands]float64{5, 4, 1, 0, -1, -1, 0, 1, 4, 5}},
	{"acoustic", "Acoustic", [Bands]float64{4, 4, 3, 1, 2, 2, 3, 3, 3, 2}},
	{"electronic", "Electronic", [Bands]float64{4, 3.5, 1, 0, -2, 2, 1, 1, 4, 5}},
	{"hip-hop", "Hip-Hop", [Bands]float64{5, 4, 1.5, 3, -1, -1, 1.5, -0.5, 2, 3}},
	{"rock", "Rock", [Bands]float64{5, 4, 3, 1.5, -0.5, -1, 0.5, 2.5, 3.5, 4.5}},
	{"classical", "Classical", [Bands]float64{4.5, 3.5, 3, 2.5, -1.5, -1.5, 0, 2, 3, 4}},
	{"jazz", "Jazz", [Bands]float64{4, 3, 1.5, 2, -1.5, -1.5, 0, 1.5, 3, 4}},
	{"piano", "Piano", [Bands]float64{3, 2, 0, 2.5, 3, 1.5, 3.5, 4.5, 3, 3.5}},
	{"spoken-word", "Spoken Word", [Bands]float64{-3.5, -0.5, 0, 0.5, 3.5, 4.5, 4.5, 4, 2.5, 0}},
}

// Find returns the built-in preset id.
func Find(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Clamp limits a band gain to [MinGain, MaxGain]; NaN becomes 0.
func Clamp(g float64) float64 {
	if math.IsNaN(g) {
		return 0
	}
	return min(max(g, MinGain), MaxGain)
}

// Headroom is the attenuation (≤ 0 dB) applied ahead of the filters so the
// largest boost cannot push a full-scale signal into clipping.
func Headroom(gains [Bands]float64) float64 {
	peak := 0.0
	for _, g := range gains {
		peak = max(peak, g)
	}
	return -peak
}

// IsFlat reports whether gains leave the signal untouched.
func IsFlat(gains [Bands]float64) bool { return gains == [Bands]float64{} }
