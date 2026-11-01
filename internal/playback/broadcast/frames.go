package broadcast

import "time"

// clock measures how much audio has gone through it by walking the frames
// of the elementary stream (ADTS AAC or MPEG audio), across chunk
// boundaries: what the playback host decodes from the same bytes. Bytes
// that do not start a frame are skipped up to the next frame header.
type clock struct {
	hdr  [frameHeader]byte
	have int // bytes in hdr
	skip int // bytes of the current frame still to pass
	d    time.Duration
}

// frameHeader is the longest frame header read (ADTS without CRC).
const frameHeader = 7

func (c *clock) write(b []byte) {
	for len(b) > 0 {
		if c.skip > 0 {
			n := min(c.skip, len(b))
			c.skip -= n
			b = b[n:]
			continue
		}
		n := copy(c.hdr[c.have:], b)
		c.have += n
		b = b[n:]
		if c.have < frameHeader {
			return
		}
		size, dur, ok := parseFrame(c.hdr[:])
		if !ok {
			// Not a frame: look for one from the next byte.
			copy(c.hdr[:], c.hdr[1:])
			c.have--
			continue
		}
		c.d += dur
		c.skip, c.have = size-frameHeader, 0
	}
}

// parseFrame reads the ADTS or MPEG audio frame header at the start of h:
// the frame's length in bytes and its duration.
func parseFrame(h []byte) (size int, dur time.Duration, ok bool) {
	if h[0] != 0xff {
		return 0, 0, false
	}
	switch {
	case h[1]&0xf6 == 0xf0: // ADTS: 12-bit sync, layer 00
		rate := adtsRates[h[2]>>2&0x0f]
		size = int(h[3]&0x03)<<11 | int(h[4])<<3 | int(h[5])>>5
		if rate == 0 || size < frameHeader {
			return 0, 0, false
		}
		samples := 1024 * (int(h[6]&0x03) + 1)
		return size, time.Duration(samples) * time.Second / time.Duration(rate), true
	case h[1]&0xe0 == 0xe0: // MPEG audio: 11-bit sync
		version, layer := h[1]>>3&0x03, h[1]>>1&0x03 // version 3 = MPEG-1, 2 = MPEG-2, 0 = MPEG-2.5
		bi, ri, pad := int(h[2]>>4), int(h[2]>>2&0x03), int(h[2]>>1&0x01)
		if version == 1 || layer == 0 || bi == 0 || bi == 15 || ri == 3 {
			return 0, 0, false
		}
		v1 := version == 3
		rate := mpegRates[ri]
		switch version {
		case 2:
			rate /= 2
		case 0:
			rate /= 4
		}
		// MPEG-1 layers I–III use tables 0–2; MPEG-2 and 2.5 use 3 for
		// layer I and 4 for layers II and III.
		table := [4]int{0, 2, 1, 0}[layer]
		if !v1 {
			table = [4]int{0, 4, 4, 3}[layer]
		}
		kbps, samples := mpegBitrates[table][bi-1], 1152
		switch {
		case layer == 3: // layer I
			samples = 384
			size = (12*kbps*1000/rate + pad) * 4
		case layer == 1 && !v1: // layer III, MPEG-2 and 2.5
			samples = 576
			size = samples/8*kbps*1000/rate + pad
		default: // layer II, and layer III of MPEG-1
			size = 144*kbps*1000/rate + pad
		}
		if size < frameHeader {
			return 0, 0, false
		}
		return size, time.Duration(samples) * time.Second / time.Duration(rate), true
	}
	return 0, 0, false
}

var adtsRates = [16]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

var mpegRates = [3]int{44100, 48000, 32000}

// mpegBitrates (kbps) by table (MPEG-1 layer I, II, III; MPEG-2/2.5
// layer I; MPEG-2/2.5 layers II and III) and bitrate index − 1.
var mpegBitrates = [5][14]int{
	{32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448},
	{32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},
	{32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},
	{32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},
	{8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
}
