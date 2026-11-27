package fmp4

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func be32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func be16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }
func be64(v uint64) []byte { return binary.BigEndian.AppendUint64(nil, v) }

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func mkbox(typ string, parts ...[]byte) []byte {
	body := cat(parts...)
	return cat(be32(uint32(8+len(body))), []byte(typ), body)
}

func mkfull(typ string, version uint8, flags uint32, parts ...[]byte) []byte {
	return mkbox(typ, append([][]byte{be32(uint32(version)<<24 | flags)}, parts...)...)
}

var testKID = []byte{0, 0, 0, 0, 0x42, 0x24, 0xe8, 0xe0, 0, 0x1d, 0xb0, 0x5d, 0x82, 0x3e, 0x28, 0xe5}

// esds with AudioSpecificConfig asc, using 4-byte descriptor lengths like Apple.
func mkesds(asc []byte) []byte {
	dlen := func(n int) []byte { return []byte{0x80, 0x80, 0x80, byte(n)} }
	dsi := cat([]byte{0x05}, dlen(len(asc)), asc)
	dcd := cat([]byte{0x04}, dlen(13+len(dsi)), []byte{0x40, 0x15, 0, 0x18, 0}, be32(320000), be32(256000), dsi)
	es := cat([]byte{0x03}, dlen(3+len(dcd)), be16(0), []byte{0}, dcd)
	return mkfull("esds", 0, 0, es)
}

// mkinit builds an init segment; trakExtra boxes follow the tkhd.
func mkinit(encrypted bool, asc []byte, trakExtra ...[]byte) []byte {
	entryHdr := cat(make([]byte, 6), be16(1), make([]byte, 8), be16(2), be16(16), be16(0), be16(0), be32(44100<<16))
	var entry []byte
	if encrypted {
		tenc := mkfull("tenc", 0, 0, []byte{0, 0, 1, 8}, testKID)
		sinf := mkbox("sinf", mkbox("frma", []byte("mp4a")), mkfull("schm", 0, 0, []byte("cenc"), be32(0x10000)), mkbox("schi", tenc))
		entry = mkbox("enca", entryHdr, mkesds(asc), sinf)
	} else {
		entry = mkbox("mp4a", entryHdr, mkesds(asc))
	}
	stsd := mkfull("stsd", 0, 0, be32(1), entry)
	mdhd := mkfull("mdhd", 0, 0, be32(0), be32(0), be32(44100), be32(0), be16(0x55c4), be16(0))
	tkhd := mkfull("tkhd", 0, 7, be32(0), be32(0), be32(1), make([]byte, 68))
	trak := mkbox("trak", tkhd, cat(trakExtra...), mkbox("mdia", mdhd, mkfull("hdlr", 0, 0, be32(0), []byte("soun"), make([]byte, 13)),
		mkbox("minf", mkbox("stbl", stsd, mkfull("stts", 0, 0, be32(0))))))
	trex := mkfull("trex", 0, 0, be32(1), be32(1), be32(1024), be32(6), be32(0))
	return cat(mkbox("ftyp", []byte("iso8"), be32(0)), mkbox("moov", mkfull("mvhd", 0, 0, make([]byte, 96)), trak, mkbox("mvex", trex)))
}

var lcStereo44 = []byte{0x12, 0x10}

func TestParseInit(t *testing.T) {
	in, err := ParseInit(mkinit(true, lcStereo44))
	if err != nil {
		t.Fatal(err)
	}
	if in.TrackID != 1 || in.Timescale != 44100 || in.Codec != "mp4a" || in.ObjectType != 0x40 {
		t.Fatalf("track: %+v", in)
	}
	if !bytes.Equal(in.AudioConfig, lcStereo44) || in.AudioObject != 2 || in.SampleRate != 44100 || in.Channels != 2 {
		t.Fatalf("audio: %+v", in)
	}
	if !in.Protected || in.Scheme != "cenc" || in.IVSize != 8 || !bytes.Equal(in.KID, testKID) {
		t.Fatalf("encryption: %+v", in)
	}
	if in.DefaultSampleDuration != 1024 || in.DefaultSampleSize != 6 {
		t.Fatalf("trex: %+v", in)
	}
}

func TestParseInitClearAndExplicitRate(t *testing.T) {
	// AOT 5 (SBR), frequency index 15 with an explicit 24-bit rate of 48000, mono.
	asc := []byte{0x2f, 0x80, 0x5d, 0xc0, 0x08}
	in, err := ParseInit(mkinit(false, asc))
	if err != nil {
		t.Fatal(err)
	}
	if in.Protected || in.Scheme != "" || in.KID != nil {
		t.Fatalf("clear init marked protected: %+v", in)
	}
	if in.AudioObject != 5 || in.SampleRate != 48000 || in.Channels != 1 {
		t.Fatalf("asc: aot %d rate %d ch %d", in.AudioObject, in.SampleRate, in.Channels)
	}
}

func TestPriming(t *testing.T) {
	// Apple's init segments have no edit list: the encoder delay applies,
	// in the media timescale (a 22.05 kHz HE-AAC core doubles it in time).
	in, err := ParseInit(mkinit(true, lcStereo44))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := in.Priming(), Time(2112, 44100); got != want {
		t.Errorf("no edit list: priming %v, want %v", got, want)
	}
	// An edit list starting at media time 1024 after an empty edit (-1).
	elst := mkbox("edts", mkfull("elst", 0, 0, be32(2),
		be32(500), be32(0xffffffff), be32(1<<16),
		be32(44100*10), be32(1024), be32(1<<16)))
	if in, err = ParseInit(mkinit(true, lcStereo44, elst)); err != nil {
		t.Fatal(err)
	}
	if got, want := in.Priming(), Time(1024, 44100); got != want {
		t.Errorf("edit list: priming %v, want %v", got, want)
	}
	elst64 := mkbox("edts", mkfull("elst", 1, 0, be32(1), be64(44100*10), be64(2112), be32(1<<16)))
	if in, err = ParseInit(mkinit(true, lcStereo44, elst64)); err != nil {
		t.Fatal(err)
	}
	if !in.HasEdit || in.EditStart != 2112 {
		t.Errorf("elst v1: %+v", in)
	}
}

func TestParseInitErrors(t *testing.T) {
	good := mkinit(true, lcStereo44)
	for name, b := range map[string][]byte{
		"no moov":   mkbox("ftyp", []byte("iso8")),
		"truncated": good[:len(good)-10],
		"bad asc":   mkinit(true, nil),
	} {
		if _, err := ParseInit(b); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

// TestParseInitEmptyStsdEntryList: a stsd that declares one entry but holds
// none used to index an empty slice and panic.
func TestParseInitEmptyStsdEntryList(t *testing.T) {
	b := mkinit(false, lcStereo44)
	// Cut the single sample entry out of the stsd and fix up the box sizes.
	entryLen := len(mkbox("mp4a", cat(make([]byte, 6), be16(1), make([]byte, 8), be16(2), be16(16), be16(0), be16(0), be32(44100<<16)), mkesds(lcStereo44)))
	i := bytes.Index(b, []byte("stsd"))
	end := i + 4 + 8 + entryLen
	b = cat(b[:end-entryLen], b[end:])
	for _, typ := range []string{"stsd", "stbl", "minf", "mdia", "trak", "moov"} {
		at := bytes.Index(b, []byte(typ)) - 4
		binary.BigEndian.PutUint32(b[at:], binary.BigEndian.Uint32(b[at:])-uint32(entryLen))
	}
	if _, err := ParseInit(b); err == nil {
		t.Fatal("expected an error for an stsd without entries")
	}
}

// TestParseFragmentWrappingSaioOffset: saio offset 2^64-1 plus a one-byte aux
// entry wrapped to 0 and passed the bounds check, then sliced out of range.
func TestParseFragmentWrappingSaioOffset(t *testing.T) {
	in := testInit(t)
	tfhd := mkfull("tfhd", 0, 0x020008, be32(1), be32(1024))
	trun := mkfull("trun", 0, trunDataOffset|trunSize, be32(1), be32(0), be32(1))
	saiz := mkfull("saiz", 0, 0, []byte{1}, be32(1))
	saio := mkfull("saio", 1, 0, be32(1), be64(^uint64(0)))
	b := cat(mkbox("moof", mkfull("mfhd", 0, 0, be32(1)), mkbox("traf", tfhd, saiz, saio, trun)), mkbox("mdat", []byte{0, 0, 0, 0}))
	if _, err := ParseFragment(b, in); err == nil {
		t.Fatal("expected an error for a saio offset outside the fragment")
	}
}

type testSample struct {
	data []byte
	iv   []byte
	subs []Subsample
}

// mkfrag builds moof+mdat for samples. senc=false stores the auxiliary
// information at the start of mdat and points saiz/saio at it instead.
func mkfrag(base uint64, samples []testSample, senc bool, durations []uint32) []byte {
	var aux []byte
	var sizes []byte
	for _, s := range samples {
		e := cat(s.iv)
		if s.subs != nil {
			e = append(e, be16(uint16(len(s.subs)))...)
			for _, ss := range s.subs {
				e = cat(e, be16(uint16(ss.Clear)), be32(ss.Cipher))
			}
		}
		aux = append(aux, e...)
		sizes = append(sizes, byte(len(e)))
	}
	sencFlags := uint32(0)
	if samples[0].subs != nil {
		sencFlags = 2
	}
	trunFlags := uint32(trunDataOffset | trunSize)
	entries := func() []byte {
		var b []byte
		for i, s := range samples {
			if durations != nil {
				b = append(b, be32(durations[i])...)
			}
			b = append(b, be32(uint32(len(s.data)))...)
		}
		return b
	}
	if durations != nil {
		trunFlags |= trunDuration
	}
	build := func(dataOffset, auxOffset uint32) []byte {
		tfhd := mkfull("tfhd", 0, 0x020008, be32(1), be32(1024))
		tfdt := mkfull("tfdt", 1, 0, be64(base))
		trun := mkfull("trun", 0, trunFlags, be32(uint32(len(samples))), be32(dataOffset), entries())
		var enc []byte
		if senc {
			enc = mkfull("senc", 0, sencFlags, be32(uint32(len(samples))), aux)
		} else {
			enc = cat(mkfull("saiz", 0, 0, []byte{0}, be32(uint32(len(samples))), sizes), mkfull("saio", 0, 0, be32(1), be32(auxOffset)))
		}
		return mkbox("moof", mkfull("mfhd", 0, 0, be32(1)), mkbox("traf", tfhd, tfdt, enc, trun))
	}
	var data []byte
	for _, s := range samples {
		data = append(data, s.data...)
	}
	moofLen := uint32(len(build(0, 0)))
	if senc {
		return cat(build(moofLen+8, 0), mkbox("mdat", data))
	}
	return cat(build(moofLen+8+uint32(len(aux)), moofLen+8), mkbox("mdat", aux, data))
}

func testInit(t *testing.T) *Init {
	t.Helper()
	in, err := ParseInit(mkinit(true, lcStereo44))
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestParseFragmentSubsamples(t *testing.T) {
	in := testInit(t)
	samples := []testSample{
		{data: []byte("aaaaaaaaaa"), iv: []byte("IV000001"), subs: []Subsample{{Clear: 2, Cipher: 5}, {Clear: 3, Cipher: 0}}},
		{data: []byte("bbbbbb"), iv: []byte("IV000002"), subs: []Subsample{{Clear: 0, Cipher: 6}}},
	}
	for _, senc := range []bool{true, false} {
		got, err := ParseFragment(mkfrag(13650944, samples, senc, nil), in)
		if err != nil {
			t.Fatalf("senc=%v: %v", senc, err)
		}
		if len(got) != 2 {
			t.Fatalf("senc=%v: %d samples", senc, len(got))
		}
		for i, s := range got {
			want := samples[i]
			if !bytes.Equal(s.Data, want.data) || !bytes.Equal(s.IV, want.iv) || len(s.Subsamples) != len(want.subs) {
				t.Fatalf("senc=%v sample %d: %+v", senc, i, s)
			}
			for j := range want.subs {
				if s.Subsamples[j] != want.subs[j] {
					t.Fatalf("senc=%v sample %d subsample %d: %+v", senc, i, j, s.Subsamples[j])
				}
			}
			if s.DecodeTime != 13650944+uint64(i)*1024 || s.Duration != 1024 || s.PTS() != int64(s.DecodeTime) || !s.Encrypted() {
				t.Fatalf("senc=%v sample %d timing: %+v", senc, i, s)
			}
		}
	}
}

func TestParseFragmentWholeSampleIVsAndDurations(t *testing.T) {
	in := testInit(t)
	samples := []testSample{{data: []byte("xyz"), iv: []byte("12345678")}, {data: []byte("uvw!"), iv: []byte("abcdefgh")}}
	got, err := ParseFragment(mkfrag(0, samples, true, []uint32{1000, 1024}), in)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Subsamples != nil || !bytes.Equal(got[1].IV, []byte("abcdefgh")) {
		t.Fatalf("encryption: %+v", got)
	}
	if got[0].Duration != 1000 || got[1].DecodeTime != 1000 {
		t.Fatalf("per-sample durations: %+v", got)
	}
}

func TestParseFragmentMultipleMoofs(t *testing.T) {
	in := testInit(t)
	a := mkfrag(0, []testSample{{data: []byte("one"), iv: []byte("11111111")}}, true, nil)
	b := mkfrag(1024, []testSample{{data: []byte("two"), iv: []byte("22222222")}}, true, nil)
	got, err := ParseFragment(cat(a, b), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || string(got[1].Data) != "two" || got[1].DecodeTime != 1024 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseFragmentErrors(t *testing.T) {
	in := testInit(t)
	one := []testSample{{data: []byte("abcd"), iv: []byte("12345678"), subs: []Subsample{{Clear: 1, Cipher: 2}}}}
	frag := mkfrag(0, one, true, nil)

	noEnc := mkfrag(0, []testSample{{data: []byte("abcd"), iv: []byte("12345678")}}, true, nil)
	noEnc = bytes.Replace(noEnc, []byte("senc"), []byte("free"), 1)

	short := mkfrag(0, []testSample{{data: []byte("abcd"), iv: []byte("12345678")}}, true, nil)
	short = short[:len(short)-2] // mdat shorter than the trun sample size

	for name, b := range map[string][]byte{
		"subsamples do not cover sample": frag,
		"missing senc and saiz":          noEnc,
		"sample past mdat":               short,
		"no samples":                     mkbox("styp", []byte("msdh")),
	} {
		if _, err := ParseFragment(b, in); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := ParseFragment(frag[:20], in); !errors.Is(err, ErrTruncated) {
		t.Errorf("truncated moof: %v", err)
	}
}

func TestParseFragmentClear(t *testing.T) {
	in, err := ParseInit(mkinit(false, lcStereo44))
	if err != nil {
		t.Fatal(err)
	}
	frag := mkfrag(0, []testSample{{data: []byte("clear"), iv: []byte("12345678")}}, true, nil)
	got, err := ParseFragment(frag, in)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Encrypted() || string(got[0].Data) != "clear" {
		t.Fatalf("clear sample: %+v", got[0])
	}
}

func TestPaddedIV(t *testing.T) {
	var buf [16]byte
	got := PaddedIV(&buf, []byte("12345678"))
	if !bytes.Equal(got, append([]byte("12345678"), make([]byte, 8)...)) {
		t.Fatalf("%x", got)
	}
}

// Apple's enhancedHls renditions (Spatial Audio's binaural AAC) list two
// 'cbcs' sample entries with their own constant IVs, and the fragments after
// the first select the second (tfhd sample_description_index 2) where the key
// changes: each fragment's samples take the IV of its own entry.
func TestParseFragmentTakesItsSampleEntrysConstantIV(t *testing.T) {
	iv1, iv2 := bytes.Repeat([]byte{1}, 16), bytes.Repeat([]byte{2}, 16)
	entryHdr := cat(make([]byte, 6), be16(1), make([]byte, 8), be16(2), be16(16), be16(0), be16(0), be32(48000<<16))
	enca := func(iv []byte) []byte {
		// tenc v1: pattern 1:9, protected, IV size 0, zero KID, constant IV.
		tenc := mkfull("tenc", 1, 0, []byte{0, 0x19, 1, 0}, make([]byte, 16), []byte{16}, iv)
		sinf := mkbox("sinf", mkbox("frma", []byte("mp4a")), mkfull("schm", 0, 0, []byte("cbcs"), be32(0x10000)), mkbox("schi", tenc))
		return mkbox("enca", entryHdr, mkesds([]byte{0x11, 0x90}), sinf)
	}
	stsd := mkfull("stsd", 0, 0, be32(2), enca(iv1), enca(iv2))
	mdhd := mkfull("mdhd", 0, 0, be32(0), be32(0), be32(48000), be32(0), be16(0x55c4), be16(0))
	tkhd := mkfull("tkhd", 0, 7, be32(0), be32(0), be32(1), make([]byte, 68))
	trak := mkbox("trak", tkhd, mkbox("mdia", mdhd, mkfull("hdlr", 0, 0, be32(0), []byte("soun"), make([]byte, 13)),
		mkbox("minf", mkbox("stbl", stsd, mkfull("stts", 0, 0, be32(0))))))
	trex := mkfull("trex", 0, 0, be32(1), be32(1), be32(1024), be32(0), be32(0))
	in, err := ParseInit(cat(mkbox("ftyp", []byte("iso8"), be32(0)), mkbox("moov", mkfull("mvhd", 0, 0, make([]byte, 96)), trak, mkbox("mvex", trex))))
	if err != nil {
		t.Fatal(err)
	}
	if in.Scheme != "cbcs" || !bytes.Equal(in.ConstantIV, iv1) || in.CryptByte != 1 || in.SkipByte != 9 {
		t.Fatalf("init = scheme %q iv %x pattern %d:%d", in.Scheme, in.ConstantIV, in.CryptByte, in.SkipByte)
	}
	data := []byte("0123456789abcdef!")
	frag := func(desc uint32) []byte {
		flags, hdr := uint32(0x020000), be32(1)
		if desc != 0 {
			flags, hdr = flags|tfhdSampleDescIndex, cat(hdr, be32(desc))
		}
		build := func(dataOffset uint32) []byte {
			senc := mkfull("senc", 0, 2, be32(1), be16(1), be16(1), be32(16))
			trun := mkfull("trun", 0, trunDataOffset|trunSize, be32(1), be32(dataOffset), be32(uint32(len(data))))
			return mkbox("moof", mkfull("mfhd", 0, 0, be32(1)), mkbox("traf", mkfull("tfhd", 0, flags, hdr), mkfull("tfdt", 1, 0, be64(0)), senc, trun))
		}
		return cat(build(uint32(len(build(0)))+8), mkbox("mdat", data))
	}
	for _, tc := range []struct {
		desc uint32
		iv   []byte
	}{{0, iv1}, {1, iv1}, {2, iv2}, {3, iv1}} {
		got, err := ParseFragment(frag(tc.desc), in)
		if err != nil {
			t.Fatalf("description %d: %v", tc.desc, err)
		}
		if !bytes.Equal(got[0].IV, tc.iv) || len(got[0].Subsamples) != 1 || got[0].Subsamples[0] != (Subsample{Clear: 1, Cipher: 16}) {
			t.Errorf("description %d: IV %x subsamples %+v, want IV %x", tc.desc, got[0].IV, got[0].Subsamples, tc.iv)
		}
	}
}

// A trun without per-sample fields passes the length check whatever its
// sample count, and a default size of 0 passes the data check: 2^32-1
// samples used to append ~88 bytes each (hundreds of GB).
func TestParseFragmentBoundsSampleCount(t *testing.T) {
	in := testInit(t)
	for name, n := range map[string]uint32{"max": 0xFFFFFFFF, "just over the cap": maxSamples + 1} {
		tfhd := mkfull("tfhd", 0, 0x020010, be32(1), be32(0))
		trun := mkfull("trun", 0, 0, be32(n))
		b := cat(mkbox("moof", mkfull("mfhd", 0, 0, be32(1)), mkbox("traf", tfhd, trun)), mkbox("mdat", []byte{0}))
		if _, err := ParseFragment(b, in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseFragmentRejectsEmptySamples(t *testing.T) {
	in := testInit(t)
	tfhd := mkfull("tfhd", 0, 0x020010, be32(1), be32(0))
	trun := mkfull("trun", 0, 0, be32(3))
	b := cat(mkbox("moof", mkfull("mfhd", 0, 0, be32(1)), mkbox("traf", tfhd, trun)), mkbox("mdat", []byte{0}))
	if _, err := ParseFragment(b, in); err == nil {
		t.Fatal("expected an error for zero-size samples")
	}
}

// mkTrak is a track of an init segment: its ID, handler type, timescale and
// one sample entry.
func mkTrak(id uint32, handler string, timescale uint32, entry []byte) []byte {
	tkhd := mkfull("tkhd", 0, 7, be32(0), be32(0), be32(id), make([]byte, 68))
	mdhd := mkfull("mdhd", 0, 0, be32(0), be32(0), be32(timescale), be32(0), be16(0x55c4), be16(0))
	hdlr := mkfull("hdlr", 0, 0, be32(0), []byte(handler), make([]byte, 13))
	stsd := mkfull("stsd", 0, 0, be32(1), entry)
	return mkbox("trak", tkhd, mkbox("mdia", mdhd, hdlr, mkbox("minf", mkbox("stbl", stsd))))
}

// mkMoov is an init segment of traks with a trex (default duration 3600)
// for each of ids.
func mkMoov(traks [][]byte, ids ...uint32) []byte {
	var trexes []byte
	for _, id := range ids {
		trexes = append(trexes, mkfull("trex", 0, 0, be32(id), be32(1), be32(3600), be32(0), be32(0))...)
	}
	return cat(mkbox("ftyp", []byte("iso8"), be32(0)), mkbox("moov", mkfull("mvhd", 0, 0, make([]byte, 96)), cat(traks...), mkbox("mvex", trexes)))
}

// testAVC1 is a clear 640×360 H.264 sample entry.
var testAVC1 = func() []byte {
	avcC := mkbox("avcC", []byte{1, 100, 0, 30, 0xff, 0xe1, 0, 4, 0x67, 100, 0, 30, 1, 0, 4, 0x68, 0xee, 0x3c, 0x80})
	return mkbox("avc1", make([]byte, 6), be16(1), make([]byte, 16), be16(640), be16(360),
		be32(0x480000), be32(0x480000), be32(0), be16(1), make([]byte, 32), be16(0x18), be16(0xffff), avcC)
}()

// testC608 is the sample entry of a CEA-608 closed-caption track, which
// Apple's video renditions carry beside the picture. Its body is not a
// real sample entry: a track ParseInit ignores is not parsed.
var testC608 = mkbox("c608", []byte("not parsed"))

func TestParseInitPicksTheMediaTrack(t *testing.T) {
	audio := mkTrak(1, "soun", 44100, mkbox("mp4a", cat(make([]byte, 6), be16(1), make([]byte, 8), be16(2), be16(16), be16(0), be16(0), be32(44100<<16)), mkesds(lcStereo44)))
	for name, tc := range map[string]struct {
		traks [][]byte
		ids   []uint32
		id    uint32
		video bool
	}{
		"video then captions": {[][]byte{mkTrak(1, "vide", 90000, testAVC1), mkTrak(2, "clcp", 90000, testC608)}, []uint32{1, 2}, 1, true},
		"captions then video": {[][]byte{mkTrak(1, "clcp", 90000, testC608), mkTrak(2, "vide", 90000, testAVC1)}, []uint32{1, 2}, 2, true},
		"video, text, meta":   {[][]byte{mkTrak(1, "vide", 90000, testAVC1), mkTrak(2, "text", 1000, testC608), mkTrak(3, "meta", 1000, testC608)}, []uint32{1, 2, 3}, 1, true},
		"subtitles":           {[][]byte{mkTrak(5, "subt", 1000, testC608), mkTrak(6, "sbtl", 1000, testC608), mkTrak(7, "vide", 90000, testAVC1)}, []uint32{5, 6, 7}, 7, true},
		"audio and metadata":  {[][]byte{audio, mkTrak(2, "meta", 1000, testC608)}, []uint32{1, 2}, 1, false},
	} {
		in, err := ParseInit(mkMoov(tc.traks, tc.ids...))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if in.TrackID != tc.id || in.IsVideo() != tc.video || in.DefaultSampleDuration != 3600 {
			t.Errorf("%s: track %d, video %v, default duration %d", name, in.TrackID, in.IsVideo(), in.DefaultSampleDuration)
		}
		if tc.video && (in.Codec != "avc1" || in.Width != 640 || in.Height != 360 || in.Timescale != 90000) {
			t.Errorf("%s: %+v", name, in)
		}
		if !tc.video && (in.Codec != "mp4a" || in.Timescale != 44100 || in.Channels != 2) {
			t.Errorf("%s: %+v", name, in)
		}
	}
}

func TestParseInitMediaTrackErrors(t *testing.T) {
	video := func(id uint32) []byte { return mkTrak(id, "vide", 90000, testAVC1) }
	audio := mkTrak(3, "soun", 44100, mkbox("mp4a"))
	noHandler := mkbox("trak", mkfull("tkhd", 0, 7, be32(0), be32(0), be32(1), make([]byte, 68)))
	noTkhd := mkbox("trak", mkbox("mdia", mkfull("mdhd", 0, 0, be32(0), be32(0), be32(90000), be32(0), be16(0), be16(0)),
		mkfull("hdlr", 0, 0, be32(0), []byte("vide"), make([]byte, 13)),
		mkbox("minf", mkbox("stbl", mkfull("stsd", 0, 0, be32(1), testAVC1)))))
	for name, b := range map[string][]byte{
		"two video tracks":         mkMoov([][]byte{video(1), mkTrak(2, "clcp", 90000, testC608), video(3)}, 1, 2, 3),
		"video and audio":          mkMoov([][]byte{video(1), audio}, 1, 3),
		"only captions":            mkMoov([][]byte{mkTrak(2, "clcp", 90000, testC608)}, 2),
		"no track":                 mkMoov(nil),
		"track without a handler":  mkMoov([][]byte{noHandler}, 1),
		"media track without tkhd": mkMoov([][]byte{noTkhd}, 1),
		"media track with ID zero": mkMoov([][]byte{video(0)}, 0),
		"video entry without avcC": mkMoov([][]byte{mkTrak(1, "vide", 90000, mkbox("avc1", make([]byte, 78)))}, 1),
		"bad entry of the media":   mkMoov([][]byte{mkTrak(1, "vide", 90000, testC608)}, 1),
		"junk trak only":           mkMoov([][]byte{mkbox("trak", []byte{0, 0, 0})}, 1),
	} {
		if _, err := ParseInit(b); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// mkTwoTracks is a moof with two trafs and one mdat that holds the
// captions' data first, then the video's: the traf of captions has one
// sample (track capID), the video's (track videoID) the given samples in
// two truns, the second continuing where the first ends. The traf of
// captions comes first unless videoFirst.
func mkTwoTracks(videoID, capID uint32, videoFirst bool, caps []byte, video [][]byte) []byte {
	var vdata, rest []byte
	for _, s := range video {
		vdata = append(vdata, s...)
	}
	for _, s := range video[1:] {
		rest = append(rest, be32(uint32(len(s)))...)
	}
	build := func(payload uint32) []byte {
		capTraf := mkbox("traf", mkfull("tfhd", 0, 0x020000, be32(capID)), mkfull("tfdt", 1, 0, be64(500)),
			mkfull("trun", 0, trunDataOffset|trunSize, be32(1), be32(payload), be32(uint32(len(caps)))))
		vidTraf := mkbox("traf", mkfull("tfhd", 0, 0x020000, be32(videoID)), mkfull("tfdt", 1, 0, be64(90000)),
			mkfull("trun", 0, trunDataOffset|trunSize, be32(1), be32(payload+uint32(len(caps))), be32(uint32(len(video[0])))),
			mkfull("trun", 0, trunSize, be32(uint32(len(video)-1)), rest))
		trafs := cat(capTraf, vidTraf)
		if videoFirst {
			trafs = cat(vidTraf, capTraf)
		}
		return mkbox("moof", mkfull("mfhd", 0, 0, be32(1)), trafs)
	}
	return cat(build(uint32(len(build(0)))+8), mkbox("mdat", caps, vdata))
}

// A moof with a traf for the video and one for the captions yields the
// video's samples only, at their own data offsets, whichever traf comes
// first and whichever track number the video has.
func TestParseFragmentReadsOnlyItsTrack(t *testing.T) {
	video := [][]byte{[]byte("frame-one"), []byte("f2"), []byte("frame-three!")}
	caps := []byte("CC608")
	for _, tc := range []struct {
		videoID, capID uint32
		videoFirst     bool
	}{{1, 2, false}, {1, 2, true}, {2, 1, false}, {2, 1, true}} {
		in, err := ParseInit(mkMoov([][]byte{mkTrak(tc.capID, "clcp", 90000, testC608), mkTrak(tc.videoID, "vide", 90000, testAVC1)}, tc.capID, tc.videoID))
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseFragment(mkTwoTracks(tc.videoID, tc.capID, tc.videoFirst, caps, video), in)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if len(got) != len(video) {
			t.Fatalf("%+v: %d samples, want %d", tc, len(got), len(video))
		}
		for i, s := range got {
			if !bytes.Equal(s.Data, video[i]) || s.Encrypted() {
				t.Errorf("%+v: sample %d = %q", tc, i, s.Data)
			}
			if s.DecodeTime != 90000+uint64(i)*3600 || s.Duration != 3600 {
				t.Errorf("%+v: sample %d timing %d +%d", tc, i, s.DecodeTime, s.Duration)
			}
		}
	}
}

// A fragment of the other track's alone has no samples for this one.
func TestParseFragmentOfAnotherTrack(t *testing.T) {
	in, err := ParseInit(mkMoov([][]byte{mkTrak(1, "vide", 90000, testAVC1), mkTrak(2, "clcp", 90000, testC608)}, 1, 2))
	if err != nil {
		t.Fatal(err)
	}
	caps := mkbox("moof", mkfull("mfhd", 0, 0, be32(1)), mkbox("traf", mkfull("tfhd", 0, 0x020000, be32(2)),
		mkfull("trun", 0, trunDataOffset|trunSize, be32(1), be32(1<<30), be32(5))))
	_, err = ParseFragment(cat(caps, mkbox("mdat", []byte("CC608"))), in)
	if err == nil || err.Error() != "fmp4: fragment has no samples" {
		t.Fatalf("err = %v, want no samples: the only traf is another track's, and its offsets are not read", err)
	}
}
