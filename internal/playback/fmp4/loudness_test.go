package fmp4

import (
	"encoding/hex"
	"math"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The 'tlou' of Metallica's "Enter Sandman" (catalog 1571968519) as Apple
// serves it: −12 LUFS, sample peak −1 dBFS, true peak −0.94 dBTP, plus
// max-of-range, momentary, short-term and LRA measurements.
const sandmanTlou = "01000000" + "01" + "00" + "0000" + "2a029e23" + "05" + "01b723" + "03bc13" + "04c413" + "05c013" + "061413"

func TestLoudnessOfAppleInit(t *testing.T) {
	tlou := mkbox("tlou", unhex(t, sandmanTlou))
	alou := mkbox("alou", unhex(t, "01000000"+"01"+"00"+"0000"+"2a000000"+"01"+"01b823"))
	// A downmixed variant listed first must not win over the plain set.
	tlouDownmix := unhex(t, "01000000"+"02"+"00"+"0400"+"00000000"+"01"+"01a023"+"00"+"0000"+"2a029e23"+"01"+"01b723")
	init := cat(mkbox("ftyp", []byte("iso6")), mkbox("moov",
		mkbox("udta", mkbox("swre", []byte("x"))),
		mkbox("trak", mkbox("tkhd"), mkbox("udta", mkbox("ludt", tlou, alou))),
	))
	l, ok := LoudnessOf(init)
	if !ok || l.Track != -12 || !l.HasPeak || math.Abs(l.TruePeak+0.9375) > 1e-9 || !l.HasAlbum || l.Album != -11.75 {
		t.Fatalf("LoudnessOf = %+v, %v", l, ok)
	}

	init = mkbox("moov", mkbox("trak", mkbox("udta", mkbox("ludt", mkbox("tlou", tlouDownmix)))))
	if l, ok = LoudnessOf(init); !ok || l.Track != -12 {
		t.Errorf("with a downmix set first: %+v, %v; want −12 LUFS from the plain set", l, ok)
	}
	if _, ok := LoudnessOf(mkbox("moov", mkbox("trak", mkbox("udta")))); ok {
		t.Error("init without ludt reported loudness")
	}
	truncated := mkbox("moov", mkbox("trak", mkbox("udta", mkbox("ludt", mkbox("tlou", unhex(t, "0100000001"))))))
	if _, ok := LoudnessOf(truncated); ok {
		t.Error("truncated tlou reported loudness")
	}
}

func mkfreeform(name, value string) []byte {
	return mkbox("----",
		mkbox("mean", []byte{0, 0, 0, 0}, []byte("com.apple.iTunes")),
		mkbox("name", []byte{0, 0, 0, 0}, []byte(name)),
		mkbox("data", be32(1), be32(0), []byte(value)))
}

func TestITunNORM(t *testing.T) {
	const tag = " 000000D3 000000D3 000006C0 000006C0 0 0 00007E88 00007E88 17 17"
	ilst := mkbox("ilst", mkfreeform("iTunSMPB", " 00000000"), mkfreeform("iTunNORM", tag))
	for name, meta := range map[string][]byte{
		"iso meta":       mkfull("meta", 0, 0, mkbox("hdlr", make([]byte, 20)), ilst),
		"quicktime meta": mkbox("meta", mkbox("hdlr", make([]byte, 20)), ilst),
	} {
		file := cat(mkbox("ftyp", []byte("M4A ")), mkbox("moov", mkbox("mvhd"), mkbox("udta", meta)), mkbox("mdat", make([]byte, 64)))
		// Only the head of the file has been downloaded.
		got, ok := ITunNORM(file[:len(file)-20])
		if !ok || got != tag[1:] {
			t.Errorf("%s: ITunNORM = %q, %v", name, got, ok)
		}
	}
	if _, ok := ITunNORM(cat(mkbox("ftyp", []byte("M4A ")), mkbox("mdat", make([]byte, 64)))); ok {
		t.Error("file with its movie box at the end (not downloaded) reported a tag")
	}
}
