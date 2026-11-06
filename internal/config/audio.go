package config

import (
	"math"
	"slices"
	"strings"

	"github.com/coredev-uk/sona-go/internal/playback/equalizer"
)

// Audio is output processing and routing in the playback host.
type Audio struct {
	// Equalizer is the 10-band equaliser.
	Equalizer Equalizer `json:"equalizer"`
	// OutputDevice is the output to play to: a PipeWire sink's node.name
	// on Linux, a Core Audio device UID on macOS; empty follows the system
	// default.
	OutputDevice string `json:"outputDevice"`
	// SoundCheck plays every track at about the same loudness, from the
	// loudness Apple measured for it (or an upload's own Sound Check tag).
	SoundCheck bool `json:"soundCheck"`
}

// Equalizer is the equaliser state: the selected preset, the user's own
// curve (kept while a preset is selected) and a preamp trim.
type Equalizer struct {
	Enabled bool `json:"enabled"`
	// Preset is a built-in preset ID (equalizer.Presets) or "custom" for
	// Custom.
	Preset string `json:"preset"`
	// Custom is the user's curve: one gain in dB per band.
	Custom []float64 `json:"custom"`
	// Preamp is a trim in dB (−12..0) on top of the automatic headroom
	// that keeps boosts from clipping.
	Preamp float64 `json:"preamp"`
}

func defaultAudio() Audio {
	return Audio{Equalizer: Equalizer{Preset: equalizer.Flat, Custom: make([]float64, equalizer.Bands)}}
}

func (a Audio) normalized() Audio {
	a.Equalizer = a.Equalizer.normalized()
	a.OutputDevice = strings.TrimSpace(a.OutputDevice)
	return a
}

// normalized makes Custom exactly one finite, in-range gain per band,
// clamps the preamp and replaces an unknown preset with flat.
func (e Equalizer) normalized() Equalizer {
	custom := make([]float64, equalizer.Bands)
	for i := range min(len(e.Custom), equalizer.Bands) {
		custom[i] = equalizer.Clamp(e.Custom[i])
	}
	e.Custom = custom
	if math.IsNaN(e.Preamp) {
		e.Preamp = 0
	}
	e.Preamp = min(max(e.Preamp, equalizer.MinPreamp), equalizer.MaxPreamp)
	if _, ok := equalizer.Find(e.Preset); !ok && e.Preset != equalizer.Custom {
		e.Preset = equalizer.Flat
	}
	return e
}

// Gains returns the curve in effect: the preset's or the custom one, all
// zero when the equaliser is off.
func (e Equalizer) Gains() [equalizer.Bands]float64 {
	var g [equalizer.Bands]float64
	if !e.Enabled {
		return g
	}
	if p, ok := equalizer.Find(e.Preset); ok {
		return p.Gains
	}
	for i := range min(len(e.Custom), equalizer.Bands) {
		g[i] = equalizer.Clamp(e.Custom[i])
	}
	return g
}

// Pregain returns the level change in dB applied ahead of the filters:
// the curve's headroom plus the preamp trim (0 when the equaliser is off).
func (e Equalizer) Pregain() float64 {
	if !e.Enabled {
		return 0
	}
	preamp := e.Preamp
	if math.IsNaN(preamp) {
		preamp = 0
	}
	return equalizer.Headroom(e.Gains()) + min(max(preamp, equalizer.MinPreamp), equalizer.MaxPreamp)
}

// Equal reports whether e and o configure the same equaliser.
func (e Equalizer) Equal(o Equalizer) bool {
	return e.Enabled == o.Enabled && e.Preset == o.Preset && e.Preamp == o.Preamp && slices.Equal(e.Custom, o.Custom)
}
