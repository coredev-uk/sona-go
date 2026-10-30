// Package fmp4 parses the fragmented MP4 audio Apple serves for Widevine
// HLS: the init segment (AAC configuration, timescale, CENC defaults) and
// media fragments (moof + mdat) into samples with timing and per-sample
// encryption data. Sample, IV and subsample data alias the parsed buffer.
package fmp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// Init describes the single audio track of an init segment.
type Init struct {
	TrackID   uint32
	Timescale uint32 // media timescale (mdhd)
	Codec     string // sample entry format before encryption, e.g. "mp4a"

	ObjectType  uint8  // esds objectTypeIndication (0x40 MPEG-4 audio)
	AudioConfig []byte // AudioSpecificConfig (esds DecoderSpecificInfo)
	AudioObject int    // AAC audio object type from AudioConfig (2 = LC)
	SampleRate  int    // from AudioConfig, else the sample entry
	Channels    int    // from AudioConfig, else the sample entry

	Protected  bool   // tenc default_isProtected
	Scheme     string // "cenc", "cbcs", …; "" when unencrypted
	KID        []byte // tenc default key ID
	IVSize     int    // tenc default per-sample IV size (0 means ConstantIV)
	ConstantIV []byte
	CryptByte  uint8 // pattern encryption (tenc v1)
	SkipByte   uint8

	// trex defaults for fragments.
	DefaultSampleDuration uint32
	DefaultSampleSize     uint32
}

// Subsample is a clear/encrypted byte run within a sample (senc order).
type Subsample struct {
	Clear  uint32
	Cipher uint32
}

// Sample is one access unit of a fragment.
type Sample struct {
	DecodeTime uint64 // in Init.Timescale units
	CTO        int32  // composition offset; PTS = DecodeTime + CTO
	Duration   uint32 // in Init.Timescale units
	Data       []byte
	// IV is the per-sample IV (8 or 16 bytes), or Init.ConstantIV. It is nil
	// for unencrypted samples.
	IV []byte
	// Subsamples is nil when the whole sample is encrypted.
	Subsamples []Subsample
}

// Encrypted reports whether s needs decryption.
func (s *Sample) Encrypted() bool { return s.IV != nil }

// PTS returns the presentation time in Init.Timescale units.
func (s *Sample) PTS() int64 { return int64(s.DecodeTime) + int64(s.CTO) }

// Time converts units of timescale to a duration without overflowing for
// the large decode times of live streams.
func Time(units int64, timescale uint32) time.Duration {
	ts := int64(timescale)
	return time.Duration(units/ts)*time.Second + time.Duration(units%ts)*time.Second/time.Duration(ts)
}

// ErrTruncated reports a box that runs past its container.
var ErrTruncated = errors.New("fmp4: truncated box")

type box struct {
	typ    string
	start  int    // offset of the box header in the parsed buffer
	bodyAt int    // offset of body in the parsed buffer
	body   []byte // payload after the header
}

// boxes splits b into boxes; base is b's offset in the outer buffer.
func boxes(b []byte, base int) ([]box, error) {
	var out []box
	for off := 0; off < len(b); {
		if len(b)-off < 8 {
			return nil, ErrTruncated
		}
		size := uint64(binary.BigEndian.Uint32(b[off:]))
		typ := string(b[off+4 : off+8])
		hdr := uint64(8)
		switch size {
		case 0:
			size = uint64(len(b) - off)
		case 1:
			if len(b)-off < 16 {
				return nil, ErrTruncated
			}
			size, hdr = binary.BigEndian.Uint64(b[off+8:]), 16
		}
		if size < hdr || size > uint64(len(b)-off) {
			return nil, fmt.Errorf("%w: %q size %d at %d", ErrTruncated, typ, size, base+off)
		}
		out = append(out, box{typ: typ, start: base + off, bodyAt: base + off + int(hdr), body: b[off+int(hdr) : off+int(size)]})
		off += int(size)
	}
	return out, nil
}

func children(b box) ([]box, error) { return boxes(b.body, b.bodyAt) }

func find(bs []box, typ string) (box, bool) {
	for _, b := range bs {
		if b.typ == typ {
			return b, true
		}
	}
	return box{}, false
}

// reader is a bounds-checked big-endian cursor.
type reader struct {
	b   []byte
	off int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || len(r.b)-r.off < n {
		r.err = ErrTruncated
		return nil
	}
	p := r.b[r.off : r.off+n]
	r.off += n
	return p
}

func (r *reader) u8() uint8 {
	if p := r.take(1); p != nil {
		return p[0]
	}
	return 0
}

func (r *reader) u16() uint16 {
	if p := r.take(2); p != nil {
		return binary.BigEndian.Uint16(p)
	}
	return 0
}

func (r *reader) u32() uint32 {
	if p := r.take(4); p != nil {
		return binary.BigEndian.Uint32(p)
	}
	return 0
}

func (r *reader) u64() uint64 {
	if p := r.take(8); p != nil {
		return binary.BigEndian.Uint64(p)
	}
	return 0
}

// full reads a FullBox version and flags.
func (r *reader) full() (version uint8, flags uint32) {
	v := r.u32()
	return uint8(v >> 24), v & 0xffffff
}
