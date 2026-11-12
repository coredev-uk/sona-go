package equalizer

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

var testPresets = []services.EqualizerPreset{
	{ID: "flat", Name: "Flat", Gains: []float64{0, 0, 0}},
	{ID: "bass", Name: "Bass Boost", Gains: []float64{6, 3, 0}},
}

func TestWithBandStartsTheUsersCurveFromThePreset(t *testing.T) {
	eq := config.Equalizer{Enabled: true, Preset: "bass", Custom: []float64{-1, -1, -1}}
	next := withBand(eq, testPresets, 2, 4)
	if next.Preset != custom {
		t.Errorf("preset %q, want %q", next.Preset, custom)
	}
	if want := []float64{6, 3, 4}; !slices.Equal(next.Custom, want) {
		t.Errorf("custom %v, want %v", next.Custom, want)
	}
	if want := []float64{6, 3, 0}; !slices.Equal(testPresets[1].Gains, want) {
		t.Errorf("the preset changed to %v", testPresets[1].Gains)
	}
	if want := []float64{-1, -1, -1}; !slices.Equal(eq.Custom, want) {
		t.Errorf("the setting passed in changed to %v", eq.Custom)
	}
}

func TestWithBandEditsTheUsersOwnCurve(t *testing.T) {
	eq := config.Equalizer{Enabled: true, Preset: custom, Custom: []float64{1, 2, 3}}
	if got, want := withBand(eq, testPresets, 0, -5).Custom, []float64{-5, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("custom %v, want %v", got, want)
	}
	if got, want := gainsOf(eq, testPresets), []float64{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("gains %v, want %v (an edit left the original alone)", got, want)
	}
}

func TestLabels(t *testing.T) {
	uitest.New(t, uitest.Bare()) // English, whatever the machine says
	var got []string
	for _, db := range []float64{0, 3, -4.5, 0.2} {
		got = append(got, formatDb(db))
	}
	if want := []string{"0", "+3", "−4.5", "0"}; !slices.Equal(got, want) {
		t.Errorf("formatDb %q, want %q", got, want)
	}
	got = nil
	for _, hz := range []float64{29, 947, 1889, 3770, 15011} {
		got = append(got, formatHz(hz))
	}
	if want := []string{"29", "947", "1.9k", "3.8k", "15k"}; !slices.Equal(got, want) {
		t.Errorf("formatHz %q, want %q", got, want)
	}
}

func TestPresetNamesKeepGoNamesForUnknownIDs(t *testing.T) {
	uitest.New(t, uitest.Bare())
	for id, want := range map[string]string{custom: "Your curve", "flat": "Flat", "hip-hop": "Hip-Hop", "spoken-word": "Spoken Word", "new-one": "Go's Name"} {
		if got := presetName(id, "Go's Name"); got != want {
			t.Errorf("presetName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestHeadroomIsTheLargestBoost(t *testing.T) {
	if got := headroom([]float64{-3, 4.5, 2}); got != -4.5 {
		t.Errorf("headroom %v, want -4.5", got)
	}
	if got := headroom([]float64{-3, -1}); got != 0 {
		t.Errorf("headroom of cuts %v, want 0", got)
	}
}
