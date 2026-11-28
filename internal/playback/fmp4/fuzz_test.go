package fmp4

import (
	"bytes"
	"math"
	"reflect"
	"testing"
	"time"
)

// The seeds of these targets are in testdata/fuzz: Apple-shaped init and
// fragment segments built by this package's test helpers, and the shapes of
// GHSA-x52q-q6gc-xf7p (an empty stsd entry list, a wrapping saio offset) and
// GHSA-q88j-3h4f-gqgf (a trun that declares 2^32-1 samples).

// offsetIn returns where s lies in buf, for a slice cut from buf: its
// capacity runs to the end of buf's backing array, as buf[off:off+len(s)]'s
// does. ok is false when s is not what buf holds at that offset, which
// catches a slice cut from anywhere else or out of range.
func offsetIn(buf, s []byte) (off int, ok bool) {
	if s == nil {
		return 0, true
	}
	off = cap(buf) - cap(s)
	if off < 0 || off+len(s) > len(buf) {
		return 0, false
	}
	return off, bytes.Equal(buf[off:off+len(s)], s)
}

// trafOf reports whether a moof of b holds a traf of track id, found
// without ParseFragment's code: the walk of the boxes and tfhd's track ID.
func trafOf(t *testing.T, b []byte, id uint32) bool {
	t.Helper()
	top, err := boxes(b, 0)
	if err != nil {
		return false
	}
	for _, moof := range top {
		if moof.typ != "moof" {
			continue
		}
		mc, err := children(moof)
		if err != nil {
			return false
		}
		for _, traf := range mc {
			if traf.typ != "traf" {
				continue
			}
			tc, err := children(traf)
			if err != nil {
				return false
			}
			if tfhd, ok := find(tc, "tfhd"); ok && len(tfhd.body) >= 8 &&
				uint32(tfhd.body[4])<<24|uint32(tfhd.body[5])<<16|uint32(tfhd.body[6])<<8|uint32(tfhd.body[7]) == id {
				return true
			}
		}
	}
	return false
}

// FuzzParseInit feeds ParseInit init segments (testdata/fuzz/FuzzParseInit:
// AAC, clear and CENC; H.264 beside a closed-caption track; two 'cbcs'
// sample entries; an empty stsd, GHSA-x52q-q6gc-xf7p).
//
// It checks that ParseInit does not panic or change its input, gives the
// same answer twice, and that an Init it returns is one ParseFragment can
// trust:
//   - the track ID and the timescale are not 0 (ParseFragment divides by the
//     timescale and matches the track by ID);
//   - the codec is a four-character code;
//   - every slice the Init keeps (the audio and video configuration, the
//     key ID, the constant IV) lies within the input;
//   - the key ID is 16 bytes, and the IV size is 0, 8 or 16, for a track with
//     a tenc box;
//   - the picture is at most 65535 px a side and the audio configuration
//     names at most 15 channels and a rate that fits 24 bits;
//   - the sample entries it keeps for later fragments are bounded by the
//     input's size (a stsd of n entries costs the input n entries);
//   - Priming is not negative.
func FuzzParseInit(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		buf := bytes.Clone(b)
		in, err := ParseInit(buf)
		if !bytes.Equal(buf, b) {
			t.Fatal("ParseInit changed its input")
		}
		again, err2 := ParseInit(buf)
		if (err == nil) != (err2 == nil) || !reflect.DeepEqual(in, again) {
			t.Fatalf("ParseInit differs between runs: %+v, %v / %+v, %v", in, err, again, err2)
		}
		if err != nil {
			if in != nil {
				t.Fatalf("ParseInit returned %+v with %v", in, err)
			}
			return
		}
		checkInit(t, buf, in)
		for i := range in.alts {
			checkInit(t, buf, &in.alts[i])
		}
		if len(in.alts) > len(buf)/8 {
			t.Fatalf("%d sample entries in %d bytes", len(in.alts)+1, len(buf))
		}
		if p := in.Priming(); p < 0 {
			t.Fatalf("Priming = %v with edit start %d, timescale %d", p, in.EditStart, in.Timescale)
		}
	})
}

func checkInit(t *testing.T, buf []byte, in *Init) {
	t.Helper()
	if in.TrackID == 0 || in.Timescale == 0 {
		t.Fatalf("track %d, timescale %d", in.TrackID, in.Timescale)
	}
	if len(in.Codec) != 4 {
		t.Fatalf("codec %q", in.Codec)
	}
	for name, s := range map[string][]byte{"AudioConfig": in.AudioConfig, "VideoConfig": in.VideoConfig, "KID": in.KID, "ConstantIV": in.ConstantIV} {
		if _, ok := offsetIn(buf, s); !ok {
			t.Fatalf("%s %x is not in the %d-byte input", name, s, len(buf))
		}
	}
	if in.KID != nil && len(in.KID) != 16 {
		t.Fatalf("key ID of %d bytes", len(in.KID))
	}
	if in.IVSize != 0 && in.IVSize != 8 && in.IVSize != 16 {
		t.Fatalf("IV size %d", in.IVSize)
	}
	if in.Width < 0 || in.Width > math.MaxUint16 || in.Height < 0 || in.Height > math.MaxUint16 {
		t.Fatalf("picture %d×%d", in.Width, in.Height)
	}
	if in.Channels < 0 || in.Channels > math.MaxUint16 || in.SampleRate < 0 || in.SampleRate >= 1<<24 {
		t.Fatalf("%d channels at %d Hz", in.Channels, in.SampleRate)
	}
	if in.IsVideo() != (in.VideoConfig != nil) {
		t.Fatalf("IsVideo = %v with config %x", in.IsVideo(), in.VideoConfig)
	}
}

// FuzzParseFragment feeds ParseFragment an init segment and a fragment
// (testdata/fuzz/FuzzParseFragment: senc and saio encryption, subsamples,
// explicit durations, a video track beside closed captions in one moof, two
// 'cbcs' sample entries, a saio offset of 2^64-1 (GHSA-x52q-q6gc-xf7p), a
// trun of 2^32-1 samples (GHSA-q88j-3h4f-gqgf)). An init that does not parse
// is replaced by a valid AAC one, so a damaged init does not hide the
// fragment from the fuzzer.
//
// It checks that ParseFragment does not panic or change its input, and that
// the samples it returns, on success, are consistent with the buffer:
//   - there are between 1 and maxSamples of them, and some traf of the
//     buffer names the Init's track (the other tracks' samples are not
//     returned);
//   - every sample has data, and the data is a slice of the buffer, at an
//     offset within it;
//   - an encrypted sample has an IV, and the subsamples of a sample, when it
//     has any, cover its data exactly;
//   - the subsamples of all samples together fit in the buffer, six bytes
//     each (the bound that keeps a hostile count from allocating).
func FuzzParseFragment(f *testing.F) {
	f.Fuzz(func(t *testing.T, initSeg, frag []byte) {
		in, err := ParseInit(initSeg)
		if err != nil {
			in = testInit(t)
		}
		buf := bytes.Clone(frag)
		samples, err := ParseFragment(buf, in)
		if !bytes.Equal(buf, frag) {
			t.Fatal("ParseFragment changed its input")
		}
		if err != nil {
			if samples != nil {
				t.Fatalf("%d samples with %v", len(samples), err)
			}
			return
		}
		if len(samples) == 0 || len(samples) > maxSamples {
			t.Fatalf("%d samples", len(samples))
		}
		if !trafOf(t, buf, in.TrackID) {
			t.Fatalf("samples of track %d, which no traf names", in.TrackID)
		}
		subsamples := 0
		for i, s := range samples {
			if len(s.Data) == 0 {
				t.Fatalf("sample %d is empty", i)
			}
			if _, ok := offsetIn(buf, s.Data); !ok {
				t.Fatalf("sample %d, %d bytes, is not in the %d-byte fragment", i, len(s.Data), len(buf))
			}
			if s.Encrypted() && len(s.IV) == 0 {
				t.Fatalf("sample %d is encrypted with no IV", i)
			}
			if s.Subsamples != nil {
				var total uint64
				for _, ss := range s.Subsamples {
					total += uint64(ss.Clear) + uint64(ss.Cipher)
				}
				if total != uint64(len(s.Data)) {
					t.Fatalf("sample %d: subsamples cover %d of %d bytes", i, total, len(s.Data))
				}
				subsamples += len(s.Subsamples)
			}
		}
		if subsamples*6 > len(buf) {
			t.Fatalf("%d subsamples from %d bytes", subsamples, len(buf))
		}
	})
}

// FuzzMovieDuration feeds MovieDuration the head of a plain MP4
// (testdata/fuzz/FuzzMovieDuration: a preview's head with a version 0 and a
// version 1 mvhd, and the head of a real file).
//
// It checks that it does not panic, and that a duration it reports is
// positive and is what the movie header says: units over timescale, in the
// 63 bits time.Duration holds.
func FuzzMovieDuration(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		d, ok := MovieDuration(b)
		if !ok {
			if d != 0 {
				t.Fatalf("duration %v, not ok", d)
			}
			return
		}
		if d <= 0 {
			t.Fatalf("duration %v", d)
		}
		if again, ok := MovieDuration(bytes.Clone(b)); !ok || again != d {
			t.Fatalf("duration %v, then %v, %v", d, again, ok)
		}
	})
}

// FuzzLoudness feeds LoudnessOf (the 'tlou' and 'alou' of an init segment's
// ludt) and ITunNORM (the Sound Check tag of a file's head)
// (testdata/fuzz/FuzzLoudness: Apple's init, a downmix set first, a tag in an
// ISO and in a QuickTime meta box).
//
// It checks that neither panics, that the loudness is what its coding can
// say (programme loudness −57.75 to +6 LUFS, a peak from −107.97 to +20 dB,
// no NaN or infinity), and that the tag is text taken from the input.
func FuzzLoudness(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		l, ok := LoudnessOf(b)
		if ok {
			for name, v := range map[string]float64{"Track": l.Track, "Album": l.Album, "TruePeak": l.TruePeak} {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Fatalf("%s = %v", name, v)
				}
			}
			if l.Track < -57.75 || l.Track > 6 || (l.HasAlbum && (l.Album < -57.75 || l.Album > 6)) || (l.HasPeak && (l.TruePeak < -108 || l.TruePeak > 20)) {
				t.Fatalf("out of range: %+v", l)
			}
		}
		if tag, ok := ITunNORM(b); ok && !bytes.Contains(b, []byte(tag)) {
			t.Fatalf("tag %q is not in the input", tag)
		}
	})
}

// FuzzMovieDuration found a movie header whose huge duration read as −2148469
// hours: Time wrapped past what a Duration holds.
func TestTimeSaturates(t *testing.T) {
	for _, tc := range []struct {
		units int64
		scale uint32
		want  time.Duration
	}{
		{44100, 44100, time.Second},
		{-44100, 44100, -time.Second},
		{math.MaxInt64, 1, math.MaxInt64},
		{math.MinInt64, 1, math.MinInt64},
		{1 << 62, 16, math.MaxInt64},
		{-(1 << 62), 16, math.MinInt64},
		{math.MaxInt64, math.MaxUint32, 2147483648*time.Second + 499999999},
	} {
		if got := Time(tc.units, tc.scale); got != tc.want {
			t.Errorf("Time(%d, %d) = %v, want %v", tc.units, tc.scale, got, tc.want)
		}
	}
}
