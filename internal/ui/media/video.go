package media

import (
	"encoding/binary"
	"image"
	"runtime"
	"sync"
)

// I420Len is the size in bytes of a w×h I420 picture: the luma plane, then
// the two chroma planes (U, then V) of ⌈w/2⌉×⌈h/2⌉, tightly packed, as the
// video surface (internal/playback/video) keeps it.
func I420Len(w, h int) int {
	return w*h + 2*((w+1)/2)*((h+1)/2)
}

// BT.709, limited range (Y 16–235, Cb and Cr 16–240) to full range RGB,
// the picture the CDM's decoder gives for SD music videos and the
// conversion the React player's shader did: luma scaled by 255/219, chroma
// by 255/224 into the matrix's coefficients, in 16.16 fixed point.
const (
	fixShift = 16
	fixHalf  = 1 << (fixShift - 1)
	coefY    = 76309  // 1.1643836
	coefRV   = 117489 // 1.7927411
	coefGU   = 13975  // 0.2132486
	coefGV   = 34925  // 0.5329093
	coefBU   = 138438 // 2.1124018
)

// luma[y] is y's share of every channel, with the rounding.
var luma = func() (t [256]int32) {
	for i := range t {
		t[i] = int32(coefY*(i-16) + fixHalf)
	}
	return t
}()

// clip limits a shifted channel value (-202…456 for any input) to 0…255
// by lookup, which is faster than a branch on noisy pictures: clip[v+clipOff].
const clipOff = 384

var clip = func() (t [1024]uint8) {
	for i := range t {
		t[i] = uint8(min(max(i-clipOff, 0), 255))
	}
	return t
}()

// bandRows is the rows a goroutine converts: pictures of at most a few
// bands' worth stay on one, where starting more costs more than it saves.
const bandRows = 96

// maxBands bounds the goroutines of one picture: 30 pictures a second should
// not take every core.
const maxBands = 4

// I420ToRGBA converts the w×h I420 picture in planes to RGBA (alpha 255)
// into dst, which is returned when it is w×h, else a new image is made. The
// loop is planar and integer: one chroma sample feeds a 2×2 block, and
// tall pictures are converted in bands on several goroutines. It returns nil
// when w or h is not positive or planes is shorter than I420Len(w, h).
//
// An SD frame (864×486) takes about half a millisecond (see
// BenchmarkI420ToRGBA).
func I420ToRGBA(dst *image.RGBA, w, h int, planes []byte) *image.RGBA {
	if w <= 0 || h <= 0 || len(planes) < I420Len(w, h) {
		return nil
	}
	if dst == nil || dst.Rect.Dx() != w || dst.Rect.Dy() != h || dst.Stride != 4*w {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	cw, ch := (w+1)/2, (h+1)/2
	y := planes[:w*h]
	u := planes[w*h : w*h+cw*ch]
	v := planes[w*h+cw*ch : w*h+2*cw*ch]

	bands := min(runtime.GOMAXPROCS(0), maxBands, (h+bandRows-1)/bandRows)
	if bands <= 1 {
		i420Rows(dst.Pix, y, u, v, w, 0, h)
		return dst
	}
	// Bands of whole chroma rows (even row counts), so none shares a sample.
	per := (h/bands + 1) &^ 1
	var wg sync.WaitGroup
	for r0 := per; r0 < h; r0 += per {
		wg.Go(func() { i420Rows(dst.Pix, y, u, v, w, r0, min(r0+per, h)) })
	}
	i420Rows(dst.Pix, y, u, v, w, 0, min(per, h))
	wg.Wait()
	return dst
}

// i420Rows converts rows r0 (even) to r1 of a picture w wide.
func i420Rows(pix, y, u, v []byte, w, r0, r1 int) {
	cw := (w + 1) / 2
	pairs := w / 2
	for r := r0; r < r1; r++ {
		yr := y[r*w : r*w+w]
		cr := (r >> 1) * cw
		ur, vr := u[cr:cr+cw], v[cr:cr+cw]
		ur, vr = ur[:pairs], vr[:pairs] // one length for the loop below to prove its indexes by
		out := pix[r*4*w : r*4*w+4*w]
		for i := range pairs {
			d := int32(ur[i]) - 128
			e := int32(vr[i]) - 128
			rAdd := coefRV * e
			gAdd := -coefGU*d - coefGV*e
			bAdd := coefBU * d
			ys := yr[2*i : 2*i+2 : 2*i+2]
			l0, l1 := luma[ys[0]], luma[ys[1]]
			o := out[8*i : 8*i+8 : 8*i+8]
			binary.LittleEndian.PutUint32(o[0:4], pack(l0, rAdd, gAdd, bAdd))
			binary.LittleEndian.PutUint32(o[4:8], pack(l1, rAdd, gAdd, bAdd))
		}
		if x := 2 * pairs; x < w { // an odd width's last column
			d := int32(u[cr+pairs]) - 128
			e := int32(v[cr+pairs]) - 128
			binary.LittleEndian.PutUint32(out[x*4:x*4+4], pack(luma[yr[x]], coefRV*e, -coefGU*d-coefGV*e, coefBU*d))
		}
	}
}

// pack is a pixel's RGBA bytes as a little-endian word, from its luma term
// and the chroma terms of each channel.
func pack(l, rAdd, gAdd, bAdd int32) uint32 {
	r := clip[((l+rAdd)>>fixShift+clipOff)&1023]
	g := clip[((l+gAdd)>>fixShift+clipOff)&1023]
	b := clip[((l+bAdd)>>fixShift+clipOff)&1023]
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16 | 0xff<<24
}
