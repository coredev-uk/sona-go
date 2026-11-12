package equalizer

import (
	"math"
	"testing"
)

var testFreqs = []float64{29, 59, 119, 237, 474, 947, 1889, 3770, 7523, 15011}

// only is a curve of ten flat bands but band i at db.
func only(i int, db float64) []float64 {
	g := make([]float64, len(testFreqs))
	g[i] = db
	return g
}

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v ± %v", what, got, want, tol)
	}
}

func TestResponseIsFlatWhenEveryBandIsZero(t *testing.T) {
	for _, db := range response(testFreqs, make([]float64, len(testFreqs)), logFrequencies(40)) {
		near(t, "response", db, 0, 1e-9)
	}
}

func TestResponsePeaksAtAMidBandsCentreAndFadesAway(t *testing.T) {
	r := response(testFreqs, only(5, 6), []float64{947, 1894, 15000})
	atCentre, oneOctaveUp, farAway := r[0], r[1], r[2]
	near(t, "at the centre", atCentre, 6, 0.05)
	if oneOctaveUp <= 0 || oneOctaveUp >= atCentre {
		t.Errorf("an octave up %v, want between 0 and %v", oneOctaveUp, atCentre)
	}
	if math.Abs(farAway) >= 0.1 {
		t.Errorf("far away %v, want within 0.1 of 0", farAway)
	}
}

func TestResponseShelvesTheLowestBandBelowAndTheHighestAbove(t *testing.T) {
	low := response(testFreqs, only(0, -9), []float64{20, 5000})
	if low[0] >= -6 {
		t.Errorf("low shelf at 20 Hz = %v, want below -6", low[0])
	}
	if math.Abs(low[1]) >= 0.1 {
		t.Errorf("low shelf at 5 kHz = %v, want within 0.1 of 0", low[1])
	}
	high := response(testFreqs, only(9, 9), []float64{20_000, 100})
	if high[0] <= 6 {
		t.Errorf("high shelf at 20 kHz = %v, want above 6", high[0])
	}
	if math.Abs(high[1]) >= 0.1 {
		t.Errorf("high shelf at 100 Hz = %v, want within 0.1 of 0", high[1])
	}
}

func TestLogPositionPlacesTheBandCentresEvenly(t *testing.T) {
	for i := 1; i < len(testFreqs); i++ {
		near(t, "gap between band centres", logPosition(testFreqs[i])-logPosition(testFreqs[i-1]), 0.1, 0.005)
	}
}

func TestLogFrequenciesSpanTheAudibleRange(t *testing.T) {
	f := logFrequencies(160)
	if len(f) != 160 {
		t.Fatalf("%d points", len(f))
	}
	near(t, "first", f[0], 20, 1e-9)
	near(t, "last", f[159], 20_000, 1e-6)
	near(t, "position of the last", logPosition(f[159]), 1, 1e-12)
}
