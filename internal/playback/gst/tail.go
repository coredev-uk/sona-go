//go:build linux || (darwin && cgo)

package gst

/*
#include "gst.h"
#include "tail.h"
*/
import "C"

import (
	"math"
	"unsafe"
)

// EQBands is the equaliser's band count (equalizer-10bands).
const EQBands = C.SONA_EQ_BANDS

// SetVolume sets the linear volume (0..1; values up to 10 amplify).
func (p *Pipeline) SetVolume(v float64) {
	if p.use() {
		C.sona_tail_set_volume(C.sona_gst_tail(p.p), C.double(v))
		p.life.RUnlock()
	}
}

// SetEqualizer sets the level change ahead of the equaliser (pregain, dB)
// and its band gains (dB). It applies to the next buffer the output
// processes, and to every rebuilt output.
func (p *Pipeline) SetEqualizer(pregain float64, bands [EQBands]float64) {
	var cb [EQBands]C.double
	for i, b := range bands {
		cb[i] = C.double(b)
	}
	if p.use() {
		C.sona_tail_set_eq(C.sona_gst_tail(p.p), C.double(pregain), &cb[0])
		p.life.RUnlock()
	}
}

// OutputSettings is what the live output applies, read back from its
// elements.
type OutputSettings struct {
	Volume  float64 // linear
	Pregain float64 // dB
	Bands   [EQBands]float64
	HasEQ   bool // the equaliser element is installed and in the output
}

// Output reads the live output's settings back from its elements.
func (p *Pipeline) Output() (OutputSettings, bool) {
	if !p.use() {
		return OutputSettings{}, false
	}
	defer p.life.RUnlock()
	return readTail(C.sona_gst_tail(p.p))
}

func readTail(t *C.sona_tail) (OutputSettings, bool) {
	var vol, pre C.double = -1, -1
	var cb [EQBands]C.double
	eq := C.sona_tail_read(t, &vol, &pre, (*C.double)(unsafe.Pointer(&cb[0])))
	if vol < 0 {
		return OutputSettings{}, false
	}
	out := OutputSettings{Volume: float64(vol), Pregain: 20 * math.Log10(float64(pre)), HasEQ: eq != 0}
	for i, b := range cb {
		out.Bands[i] = float64(b)
	}
	return out, true
}
