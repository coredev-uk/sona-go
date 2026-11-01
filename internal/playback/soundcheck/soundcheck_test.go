package soundcheck

import (
	"math"
	"testing"

	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/id3/id3test"
)

func db(g float64) float64 { return 20 * math.Log10(g) }

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestFromLoudness(t *testing.T) {
	for _, c := range []struct {
		name string
		l    fmp4.Loudness
		want float64 // dB
	}{
		{"loud master is cut to the target", fmp4.Loudness{Track: -7.25, TruePeak: 1.03, HasPeak: true}, -8.75},
		{"quiet track is raised up to the target", fmp4.Loudness{Track: -19.5, TruePeak: -6, HasPeak: true}, 3.5},
		{"raise stops at the true-peak ceiling", fmp4.Loudness{Track: -18.5, TruePeak: -2, HasPeak: true}, 1},
		{"track already over the ceiling is not raised", fmp4.Loudness{Track: -17, TruePeak: 0.5, HasPeak: true}, 0},
		{"no peak known: full raise", fmp4.Loudness{Track: -19.5}, 3.5},
		{"absurd measurement is bounded", fmp4.Loudness{Track: -60}, 12},
	} {
		if got := db(FromLoudness(c.l)); !near(got, c.want) {
			t.Errorf("%s: %.2f dB, want %.2f", c.name, got, c.want)
		}
	}
}

func TestFromITunNORM(t *testing.T) {
	// From an uploaded .m4a: 0xD3 = 211 asks for +6.76 dB, but its peak
	// 0x7E88 is −0.1 dBFS: any boost would clip, so it plays at unity.
	g, ok := FromITunNORM(" 000000D3 000000D3 000006C0 000006C0 0 0 00007E88 00007E88 17 17")
	if !ok || !near(db(g), 0) {
		t.Errorf("peak-limited gain %.3f dB, %v", db(g), ok)
	}
	// Peak 0x4000 (−6.02 dBFS): the boost stops at the −1 dB ceiling.
	if g, _ := FromITunNORM("000000D3 000000D3 0 0 0 0 00004000 00002000 0 0"); !near(db(g), 5.021) {
		t.Errorf("capped gain %.3f dB, want 5.02", db(g))
	}
	// Peak 0x2000 (−12 dBFS) leaves room for the whole +6.76 dB.
	if g, _ := FromITunNORM("000000D3 000000D3 0 0 0 0 00002000 00002000 0 0"); !near(db(g), 6.757) {
		t.Errorf("gain %.3f dB, want 6.76", db(g))
	}
	// The louder channel decides: 0x7D0 = 2000 → −3.01 dB.
	if g, ok := FromITunNORM("000003E8 000007D0"); !ok || !near(db(g), -3.01) {
		t.Errorf("stereo gain %.3f dB, %v", db(g), ok)
	}
	for _, bad := range []string{"", "000003E8", "zz 000003E8", "0 0"} {
		if _, ok := FromITunNORM(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

func latin1Comment(desc, text string) []byte {
	return append(append([]byte{0, 'e', 'n', 'g'}, append([]byte(desc), 0)...), text...)
}

func utf16Comment(desc, text string) []byte {
	enc := func(s string) []byte {
		out := []byte{0xff, 0xfe}
		for _, c := range []byte(s) {
			out = append(out, c, 0)
		}
		return out
	}
	b := append([]byte{1, 'e', 'n', 'g'}, enc(desc)...)
	b = append(b, 0, 0)
	return append(b, enc(text)...)
}

func TestFromFileMP3(t *testing.T) {
	const norm = " 000007D0 000003E8 00000000 00000000 0 0 0 0 0 0"
	smpb := latin1Comment("iTunSMPB", " 00000000 00000210")
	for name, tag := range map[string][]byte{
		"v2.2":       id3test.Tag(2, id3test.Frame(2, "TSS", []byte{0, 'x'}), id3test.Frame(2, "COM", smpb), id3test.Frame(2, "COM", latin1Comment("iTunNORM", norm))),
		"v2.3 utf16": id3test.Tag(3, id3test.Frame(3, "COMM", utf16Comment("iTunNORM", norm))),
		"v2.4":       id3test.Tag(4, id3test.Frame(4, "COMM", latin1Comment("", "a comment")), id3test.Frame(4, "COMM", latin1Comment("iTunNORM", norm))),
	} {
		mp3 := append(tag, 0xff, 0xfb, 0x90, 0x00)
		g, ok := FromFile(mp3)
		if !ok || !near(db(g), -3.01) {
			t.Errorf("%s: %.3f dB, %v", name, db(g), ok)
		}
	}
	if _, ok := FromFile(id3test.Tag(3, id3test.Frame(3, "TIT2", []byte{0, 'x'}))); ok {
		t.Error("untagged mp3 reported a gain")
	}
	if _, ok := FromFile(id3test.Tag(3, id3test.Frame(3, "COMM", latin1Comment("iTunNORM", norm)))[:20]); ok {
		t.Error("truncated tag reported a gain")
	}
}
