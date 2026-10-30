package playback

import (
	"math"
	"testing"
)

func TestVolumeGainClamps(t *testing.T) {
	for v, want := range map[float64]float64{
		math.NaN():   0,
		math.Inf(-1): 0,
		-0.5:         0,
		0.5:          0.125,
		1:            1,
		math.Inf(1):  1,
	} {
		if got := VolumeGain(v); got != want {
			t.Errorf("VolumeGain(%v) = %v, want %v", v, got, want)
		}
	}
}
