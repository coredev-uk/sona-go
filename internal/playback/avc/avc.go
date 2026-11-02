// Package avc prepares H.264 access units from fragmented MP4 (ISO/IEC
// 14496-15: length-prefixed NAL units, parameter sets in the avcC box) for
// a decoder that takes an Annex B byte stream, as the Widevine CDM's own
// video decoder does (Chromium feeds it the same way): each NAL unit gets
// a start code instead of its length, and every IDR picture is preceded by
// the sequence and picture parameter sets. For encrypted samples the
// subsample map is rewritten so the encrypted bytes keep their place:
// length prefixes and NAL headers always lie in clear bytes ('cenc' and
// 'cbcs' both require it), so only clear byte counts change.
package avc

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
)

// NAL unit types (ITU-T H.264 table 7-1) Sona looks at.
const (
	nalIDR = 5
	nalSPS = 7
)

var startCode = []byte{0, 0, 0, 1}

// Config is an AVCDecoderConfigurationRecord (the avcC box body).
type Config struct {
	Profile       uint8 // AVCProfileIndication (profile_idc): 66 Baseline, 77 Main, 100 High, …
	Compatibility uint8 // profile_compatibility
	Level         uint8 // AVCLevelIndication
	// LengthSize is the size of each NAL unit's length prefix in
	// samples: 1, 2 or 4 bytes.
	LengthSize int
	SPS, PPS   [][]byte
	// paramSets is every SPS, then every PPS, each after a start code:
	// what goes before an IDR picture that does not carry its own.
	paramSets []byte
}

// Errors returned by ParseConfig and AnnexB.
var (
	ErrConfig = errors.New("avc: bad decoder configuration record")
	ErrSample = errors.New("avc: bad access unit")
)

// ParseConfig parses an avcC box body.
func ParseConfig(b []byte) (*Config, error) {
	if len(b) < 7 || b[0] != 1 {
		return nil, ErrConfig
	}
	c := &Config{Profile: b[1], Compatibility: b[2], Level: b[3], LengthSize: int(b[4]&3) + 1}
	if c.LengthSize == 3 {
		return nil, fmt.Errorf("%w: 3-byte NAL lengths", ErrConfig)
	}
	off := 5
	sets := func(n int) ([][]byte, error) {
		out := make([][]byte, 0, n)
		for range n {
			if off+2 > len(b) {
				return nil, ErrConfig
			}
			l := int(binary.BigEndian.Uint16(b[off:]))
			off += 2
			if l == 0 || off+l > len(b) {
				return nil, ErrConfig
			}
			out = append(out, b[off:off+l:off+l])
			off += l
		}
		return out, nil
	}
	nsps := int(b[off] & 0x1f)
	off++
	var err error
	if c.SPS, err = sets(nsps); err != nil {
		return nil, err
	}
	if off >= len(b) {
		return nil, ErrConfig
	}
	npps := int(b[off])
	off++
	if c.PPS, err = sets(npps); err != nil {
		return nil, err
	}
	if len(c.SPS) == 0 || len(c.PPS) == 0 {
		return nil, fmt.Errorf("%w: no parameter sets", ErrConfig)
	}
	for _, s := range c.SPS {
		c.paramSets = append(append(c.paramSets, startCode...), s...)
	}
	for _, s := range c.PPS {
		c.paramSets = append(append(c.paramSets, startCode...), s...)
	}
	return c, nil
}

// AnnexB converts one access unit to Annex B, appending it to dst[:0].
// subs is the sample's subsample map (nil for a clear sample, which gets
// nil back); the rewritten map is appended to subsOut[:0]. A sample that
// is encrypted whole (a non-nil empty map does not occur in fMP4) cannot
// be converted: its length prefixes are encrypted.
func (c *Config) AnnexB(dst, sample []byte, subs, subsOut []fmp4.Subsample) ([]byte, []fmp4.Subsample, error) {
	L := c.LengthSize
	// First pass: check the NAL layout and look for an IDR picture and
	// in-band parameter sets.
	idr, sps := false, false
	n := 0
	for off := 0; off < len(sample); n++ {
		l, ok := nalLength(sample, off, L)
		if !ok {
			return nil, nil, fmt.Errorf("%w: NAL unit at %d runs past the %d-byte sample", ErrSample, off, len(sample))
		}
		if l > 0 {
			switch sample[off+L] & 0x1f {
			case nalIDR:
				idr = true
			case nalSPS:
				sps = true
			}
		}
		off += L + l
	}
	insert := idr && !sps
	out := dst[:0]
	if insert {
		out = append(out, c.paramSets...)
	}
	for off := 0; off < len(sample); {
		l, _ := nalLength(sample, off, L)
		out = append(out, startCode...)
		out = append(out, sample[off+L:off+L+l]...)
		off += L + l
	}
	if subs == nil {
		return out, nil, nil
	}
	if len(subs) == 0 {
		return nil, nil, fmt.Errorf("%w: encrypted whole, its NAL lengths unreadable", ErrSample)
	}
	so := append(subsOut[:0], subs...)
	total := 0
	for _, s := range subs {
		total += int(s.Clear) + int(s.Cipher)
	}
	if total != len(sample) {
		return nil, nil, fmt.Errorf("%w: subsamples cover %d of %d bytes", ErrSample, total, len(sample))
	}
	// Second pass: each prefix (and the NAL header after it) must lie in
	// a subsample's clear bytes, which grow by what the start code adds.
	grow := uint32(len(startCode) - L)
	k, start := 0, 0 // current subsample and its first byte
	for off := 0; off < len(sample); {
		l, _ := nalLength(sample, off, L)
		for k < len(subs) && off >= start+int(subs[k].Clear)+int(subs[k].Cipher) {
			start += int(subs[k].Clear) + int(subs[k].Cipher)
			k++
		}
		head := L
		if l > 0 {
			head++
		}
		if k == len(subs) || off+head > start+int(subs[k].Clear) {
			return nil, nil, fmt.Errorf("%w: NAL unit at %d starts in encrypted bytes", ErrSample, off)
		}
		so[k].Clear += grow
		off += L + l
	}
	if insert {
		so[0].Clear += uint32(len(c.paramSets))
	}
	return out, so, nil
}

// nalLength reads the L-byte length prefix at off and checks that the
// NAL unit it announces fits in b.
func nalLength(b []byte, off, L int) (int, bool) {
	if off+L > len(b) {
		return 0, false
	}
	var l int
	for _, x := range b[off : off+L] {
		l = l<<8 | int(x)
	}
	return l, off+L+l <= len(b)
}
