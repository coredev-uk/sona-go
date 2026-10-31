package fmp4

import (
	"math"
	"time"
)

// MovieDuration reads the presentation length from the movie header
// (moov/mvhd) of a plain MP4 file whose movie box comes first: b is the
// file or its head, cut anywhere after the movie box. Apple's song
// previews need it: their iTunSMPB tag counts the samples of the whole
// song, so a demuxer that honours it reports the song's length, while
// the file holds 30 seconds.
func MovieDuration(b []byte) (time.Duration, bool) {
	top, err := boxes(prefixBoxes(b), 0)
	if err != nil {
		return 0, false
	}
	moov, found := find(top, "moov")
	if !found {
		return 0, false
	}
	mvhd, found := within(moov, "mvhd")
	if !found {
		return 0, false
	}
	r := reader{b: mvhd.body}
	var (
		timescale uint32
		units     uint64
	)
	if v, _ := r.full(); v == 1 {
		r.take(16) // creation and modification times
		timescale, units = r.u32(), r.u64()
	} else {
		r.take(8)
		timescale, units = r.u32(), uint64(r.u32())
	}
	// All ones is an unknown duration (ISO/IEC 14496-12).
	if r.err != nil || timescale == 0 || units == 0 || units == math.MaxUint32 || units > math.MaxInt64 {
		return 0, false
	}
	return Time(int64(units), timescale), true
}
