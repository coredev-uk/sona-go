package fmp4

import (
	"errors"
	"fmt"
)

// tfhd flags.
const (
	tfhdBaseDataOffset  = 0x000001
	tfhdSampleDescIndex = 0x000002
	tfhdDefaultDuration = 0x000008
	tfhdDefaultSize     = 0x000010
	tfhdDefaultFlags    = 0x000020
)

// trun flags.
const (
	trunDataOffset       = 0x000001
	trunFirstSampleFlags = 0x000004
	trunDuration         = 0x000100
	trunSize             = 0x000200
	trunFlags            = 0x000400
	trunCTO              = 0x000800
)

// ParseFragment parses every moof + mdat pair in b (one HLS byte range) and
// returns the samples of init's track in decode order. Per-sample
// encryption comes from senc, or from saiz/saio auxiliary information when
// senc is absent. Sample, IV and subsample slices alias b.
func ParseFragment(b []byte, in *Init) ([]Sample, error) {
	top, err := boxes(b, 0)
	if err != nil {
		return nil, err
	}
	var out []Sample
	for _, moof := range top {
		if moof.typ != "moof" {
			continue
		}
		mc, err := children(moof)
		if err != nil {
			return nil, err
		}
		for _, traf := range mc {
			if traf.typ != "traf" {
				continue
			}
			if out, err = parseTraf(b, moof, traf, in, out); err != nil {
				return nil, err
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("fmp4: fragment has no samples")
	}
	return out, nil
}

func parseTraf(b []byte, moof, traf box, in *Init, out []Sample) ([]Sample, error) {
	tc, err := children(traf)
	if err != nil {
		return nil, err
	}
	tfhd, ok := find(tc, "tfhd")
	if !ok {
		return nil, errors.New("fmp4: traf without tfhd")
	}
	r := reader{b: tfhd.body}
	_, flags := r.full()
	trackID := r.u32()
	if in.TrackID != 0 && trackID != in.TrackID {
		return out, nil
	}
	if flags&tfhdBaseDataOffset != 0 {
		// An absolute file offset; Apple's fragments use default-base-is-moof.
		return nil, errors.New("fmp4: tfhd base-data-offset is not supported")
	}
	if flags&tfhdSampleDescIndex != 0 {
		r.u32()
	}
	defDur, defSize := in.DefaultSampleDuration, in.DefaultSampleSize
	if flags&tfhdDefaultDuration != 0 {
		defDur = r.u32()
	}
	if flags&tfhdDefaultSize != 0 {
		defSize = r.u32()
	}
	if flags&tfhdDefaultFlags != 0 {
		r.u32()
	}
	if r.err != nil {
		return nil, fmt.Errorf("fmp4: tfhd: %w", r.err)
	}
	// Without base-data-offset, data offsets are relative to the moof (also
	// the rule for the first traf when default-base-is-moof is unset).
	base := moof.start

	var decodeTime uint64
	if tfdt, ok := find(tc, "tfdt"); ok {
		r := reader{b: tfdt.body}
		if v, _ := r.full(); v == 1 {
			decodeTime = r.u64()
		} else {
			decodeTime = uint64(r.u32())
		}
		if r.err != nil {
			return nil, fmt.Errorf("fmp4: tfdt: %w", r.err)
		}
	}

	first := len(out)
	dataEnd := base
	for _, trun := range tc {
		if trun.typ != "trun" {
			continue
		}
		r := reader{b: trun.body}
		v, f := r.full()
		n := int(r.u32())
		pos := dataEnd
		if f&trunDataOffset != 0 {
			pos = base + int(int32(r.u32()))
		}
		if f&trunFirstSampleFlags != 0 {
			r.u32()
		}
		per := 0
		for _, bit := range []uint32{trunDuration, trunSize, trunFlags, trunCTO} {
			if f&bit != 0 {
				per += 4
			}
		}
		if r.err != nil || n < 0 || len(r.b)-r.off < n*per {
			return nil, fmt.Errorf("fmp4: trun with %d samples: %w", n, ErrTruncated)
		}
		for range n {
			s := Sample{DecodeTime: decodeTime, Duration: defDur}
			size := defSize
			if f&trunDuration != 0 {
				s.Duration = r.u32()
			}
			if f&trunSize != 0 {
				size = r.u32()
			}
			if f&trunFlags != 0 {
				r.u32()
			}
			if f&trunCTO != 0 {
				c := r.u32()
				if v == 0 {
					s.CTO = int32(min(c, 1<<31-1))
				} else {
					s.CTO = int32(c)
				}
			}
			if pos < 0 || uint64(pos)+uint64(size) > uint64(len(b)) {
				return nil, fmt.Errorf("fmp4: sample data [%d,+%d) outside fragment of %d bytes", pos, size, len(b))
			}
			s.Data = b[pos : pos+int(size)]
			pos += int(size)
			decodeTime += uint64(s.Duration)
			out = append(out, s)
		}
		dataEnd = pos
	}
	samples := out[first:]
	if !in.Protected || len(samples) == 0 {
		return out, nil
	}
	if senc, ok := find(tc, "senc"); ok {
		return out, parseSenc(senc.body, in, samples)
	}
	if err := parseAux(b, tc, base, in, samples); err != nil {
		return nil, err
	}
	return out, nil
}

// parseSenc reads a SampleEncryptionBox into samples.
func parseSenc(body []byte, in *Init, samples []Sample) error {
	r := reader{b: body}
	_, flags := r.full()
	n := int(r.u32())
	if r.err != nil {
		return fmt.Errorf("fmp4: senc: %w", r.err)
	}
	if n == 0 && len(in.ConstantIV) > 0 && in.IVSize == 0 {
		// 'cbcs' with a constant IV and no subsamples (Apple's radio):
		// every sample is encrypted whole with the tenc IV.
		for i := range samples {
			samples[i].IV = in.ConstantIV
		}
		return nil
	}
	if n != len(samples) {
		return fmt.Errorf("fmp4: senc has %d entries for %d samples", n, len(samples))
	}
	ivSize := in.IVSize
	if ivSize == 0 && len(in.ConstantIV) == 0 && !hasSubsampleFlag(flags) && n > 0 {
		// No tenc IV size: infer it from the payload.
		ivSize = (len(r.b) - r.off) / n
	}
	for i := range samples {
		if err := readAuxEntry(&r, ivSize, hasSubsampleFlag(flags), in, &samples[i]); err != nil {
			return fmt.Errorf("fmp4: senc sample %d: %w", i, err)
		}
	}
	return nil
}

func hasSubsampleFlag(flags uint32) bool { return flags&0x2 != 0 }

// readAuxEntry reads one CencSampleAuxiliaryDataFormat entry.
func readAuxEntry(r *reader, ivSize int, subsamples bool, in *Init, s *Sample) error {
	if ivSize > 0 {
		s.IV = r.take(ivSize)
	} else {
		s.IV = in.ConstantIV
	}
	if subsamples {
		n := int(r.u16())
		if r.err == nil && len(r.b)-r.off < n*6 {
			return ErrTruncated
		}
		if n > 0 {
			s.Subsamples = make([]Subsample, n)
		}
		total := uint64(0)
		for j := range s.Subsamples {
			s.Subsamples[j] = Subsample{Clear: uint32(r.u16()), Cipher: r.u32()}
			total += uint64(s.Subsamples[j].Clear) + uint64(s.Subsamples[j].Cipher)
		}
		if r.err == nil && total != uint64(len(s.Data)) {
			return fmt.Errorf("subsamples cover %d of %d bytes", total, len(s.Data))
		}
	}
	if r.err != nil {
		return r.err
	}
	if s.IV == nil {
		return errors.New("no IV")
	}
	return nil
}

// parseAux reads per-sample encryption from saiz/saio auxiliary information.
func parseAux(b []byte, tc []box, base int, in *Init, samples []Sample) error {
	saiz, ok1 := find(tc, "saiz")
	saio, ok2 := find(tc, "saio")
	if !ok1 || !ok2 {
		return errors.New("fmp4: protected fragment without senc or saiz/saio")
	}
	r := reader{b: saiz.body}
	_, flags := r.full()
	if flags&1 != 0 {
		r.take(8) // aux_info_type, aux_info_type_parameter
	}
	defSize := int(r.u8())
	n := int(r.u32())
	var sizes []byte
	if defSize == 0 {
		sizes = r.take(n)
	}
	if r.err != nil {
		return fmt.Errorf("fmp4: saiz: %w", r.err)
	}
	if n != len(samples) {
		return fmt.Errorf("fmp4: saiz has %d entries for %d samples", n, len(samples))
	}
	o := reader{b: saio.body}
	v, oflags := o.full()
	if oflags&1 != 0 {
		o.take(8)
	}
	if cnt := o.u32(); cnt != 1 {
		return fmt.Errorf("fmp4: saio with %d offsets is not supported", cnt)
	}
	var off uint64
	if v == 0 {
		off = uint64(o.u32())
	} else {
		off = o.u64()
	}
	if o.err != nil {
		return fmt.Errorf("fmp4: saio: %w", o.err)
	}
	pos := uint64(base) + off
	for i := range samples {
		size := defSize
		if sizes != nil {
			size = int(sizes[i])
		}
		if pos+uint64(size) > uint64(len(b)) {
			return fmt.Errorf("fmp4: aux info of sample %d outside fragment", i)
		}
		entry := reader{b: b[pos : pos+uint64(size)]}
		ivSize := in.IVSize
		if ivSize > size {
			return fmt.Errorf("fmp4: aux info of sample %d: %d bytes for a %d-byte IV", i, size, ivSize)
		}
		if err := readAuxEntry(&entry, ivSize, size > ivSize, in, &samples[i]); err != nil {
			return fmt.Errorf("fmp4: aux info of sample %d: %w", i, err)
		}
		pos += uint64(size)
	}
	return nil
}

// PaddedIV returns iv as the 16-byte counter block the CDM expects: CENC
// 8-byte IVs are the high half, followed by a zero block counter.
func PaddedIV(dst *[16]byte, iv []byte) []byte {
	*dst = [16]byte{}
	copy(dst[:], iv)
	return dst[:]
}
