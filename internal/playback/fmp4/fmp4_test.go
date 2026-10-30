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
