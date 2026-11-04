package equalizer

// The equaliser's frequency response, computed like GStreamer's
// equalizer-10bands (gstiirequalizer.c): band 0 is a low shelf, the last
// band a high shelf, the others peaking filters, each as wide as its slice
// of 20 Hz–20 kHz on a log scale. Used to draw the curve the playback host
// really applies.

import "math"

const (
	sampleRate = 48_000
	lowest     = 20
	highest    = 20_000
)

type filterKind int

const (
	peak filterKind = iota
	lowShelf
	highShelf
)

// biquad holds the numerator b0..b2 and denominator a0..a2 (a0 = 1 after
// normalising).
type biquad struct{ b, a [3]float64 }

// bandWidth is the width in Hz of band i of n (gstiirequalizernbands.c).
func bandWidth(i, n int) float64 {
	step := math.Pow(highest/float64(lowest), 1/float64(n))
	f0 := lowest * math.Pow(step, float64(i))
	return f0*step - f0
}

func newFilter(kind filterKind, freq, width, gainDb float64) biquad {
	g := math.Pow(10, gainDb/40)
	w := 2 * math.Pi * freq / sampleRate
	cos := math.Cos(w)
	alpha := math.Tan(math.Pi * width / sampleRate)
	if kind == peak {
		d := 1 + alpha/g
		return biquad{
			b: [3]float64{(1 + alpha*g) / d, -2 * cos / d, (1 - alpha*g) / d},
			a: [3]float64{1, -2 * cos / d, (1 - alpha/g) / d},
		}
	}
	egm := g - 1
	egp := g + 1
	delta := 2 * math.Sqrt(g) * alpha
	if kind == lowShelf {
		d := egp + egm*cos + delta
		return biquad{
			b: [3]float64{
				(egp - egm*cos + delta) * g / d,
				(egm - egp*cos) * 2 * g / d,
				(egp - egm*cos - delta) * g / d,
			},
			a: [3]float64{1, -(egm + egp*cos) * 2 / d, (egp + egm*cos - delta) / d},
		}
	}
	d := egp - egm*cos + delta
	return biquad{
		b: [3]float64{
			(egp + egm*cos + delta) * g / d,
			(egm + egp*cos) * -2 * g / d,
			(egp + egm*cos - delta) * g / d,
		},
		a: [3]float64{1, (egm - egp*cos) * 2 / d, (egp - egm*cos - delta) / d},
	}
}

// magnitudeDb is |H(e^jw)| of one biquad, in dB.
func (f biquad) magnitudeDb(freq float64) float64 {
	w := 2 * math.Pi * freq / sampleRate
	c1, s1 := math.Cos(w), math.Sin(w)
	c2, s2 := math.Cos(2*w), math.Sin(2*w)
	nr := f.b[0] + f.b[1]*c1 + f.b[2]*c2
	ni := -(f.b[1]*s1 + f.b[2]*s2)
	dr := f.a[0] + f.a[1]*c1 + f.a[2]*c2
	di := -(f.a[1]*s1 + f.a[2]*s2)
	return 10 * math.Log10((nr*nr+ni*ni)/(dr*dr+di*di))
}

// response is the response in dB at each frequency in at for band gains
// gains (centred on freqs), including pregainDb. A band at 0 dB adds no
// filter.
func response(freqs, gains []float64, pregainDb float64, at []float64) []float64 {
	n := len(freqs)
	var filters []biquad
	for i, f := range freqs {
		if i >= len(gains) || gains[i] == 0 {
			continue
		}
		kind := peak
		switch i {
		case 0:
			kind = lowShelf
		case n - 1:
			kind = highShelf
		}
		filters = append(filters, newFilter(kind, f, bandWidth(i, n), gains[i]))
	}
	out := make([]float64, len(at))
	for k, f := range at {
		db := pregainDb
		for _, bq := range filters {
			db += bq.magnitudeDb(f)
		}
		out[k] = db
	}
	return out
}

// logFrequencies are count frequencies spaced evenly on a log scale over
// 20 Hz–20 kHz.
func logFrequencies(count int) []float64 {
	out := make([]float64, count)
	for i := range out {
		out[i] = lowest * math.Pow(highest/float64(lowest), float64(i)/float64(count-1))
	}
	return out
}

// logPosition is where freq sits on the 20 Hz–20 kHz log axis, 0..1.
func logPosition(freq float64) float64 {
	return math.Log(freq/lowest) / math.Log(highest/float64(lowest))
}
