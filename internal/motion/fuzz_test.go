package motion

import (
	"bytes"
	"cmp"
	"math"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// The seeds of these targets are in testdata/fuzz: files built by this
// package's test helpers (Apple's motion renditions as one fragmented file
// and as an init file with segment files, B-frame offsets, damaged boxes), and
// the shape of GHSA-587h-hjxq-f77g (a tfhd base-data-offset near MaxInt64).

// FuzzFlatten feeds flatten the fragmented MP4 of a motion rendition: one
// file (the second argument empty), or an initialisation file followed by a
// segment file (testdata/fuzz/FuzzFlatten: single files, with and without
// signed composition offsets; init and segment files; a base-data-offset of
// 2^63-1, GHSA-587h-hjxq-f77g; a trun of many empty samples).
//
// It checks that flatten does not panic or change its input, and that a
// file it rewrites (one with fragments; a plain MP4 comes back as it is)
// is the plain MP4 it claims to be:
//   - ftyp, moov and mdat, with no mvex, and a sample table that locates
//     every sample (readFlat's walk of stts, ctts, stss, stsc, stco and
//     stsz), at most maxSamples of them;
//   - the samples' bytes are what the input held: together at most the
//     input's size (a hostile trun cannot make the output larger than its
//     input by pointing its samples at the same bytes again), and the
//     first few are found in the input;
//   - the output is bounded by the input: its size, plus 64 bytes for each
//     sample, plus 4 KiB for the headers;
//   - the timeline starts at 0: the earliest presentation time is 0, the
//     media duration is the sum of the sample durations, and an offset is
//     negative only in a version 1 ctts;
//   - the same input gives the same output.
func FuzzFlatten(f *testing.F) {
	f.Fuzz(func(t *testing.T, a, b []byte) {
		files := [][]byte{bytes.Clone(a)}
		if len(b) > 0 {
			files = append(files, bytes.Clone(b))
		}
		out, err := flatten(files...)
		for i, file := range files {
			if !bytes.Equal(file, [][]byte{a, b}[i]) {
				t.Fatalf("flatten changed input %d", i)
			}
		}
		if err != nil {
			if out != nil {
				t.Fatalf("%d bytes with %v", len(out), err)
			}
			return
		}
		if len(files) == 1 && bytes.Equal(out, a) {
			return // a plain MP4, returned as it is
		}
		again, err := flatten(files...)
		if err != nil || !bytes.Equal(again, out) {
			t.Fatalf("flatten differs between runs: %v", err)
		}
		checkFlat(t, out, files)
	})
}

func checkFlat(t *testing.T, out []byte, files [][]byte) {
	t.Helper()
	input := 0
	for _, f := range files {
		input += len(f)
	}
	f := readFlat(t, out)
	if !slices.Equal(f.top, []string{"ftyp", "moov", "mdat"}) || f.mvex {
		t.Fatalf("boxes %v, mvex %v", f.top, f.mvex)
	}
	n := len(f.samples)
	if n == 0 || n > maxSamples {
		t.Fatalf("%d samples", n)
	}
	payload := 0
	for _, s := range f.samples {
		payload += len(s)
	}
	if payload > input {
		t.Fatalf("%d bytes of samples from %d bytes of input", payload, input)
	}
	if len(out) > input+64*n+4096 {
		t.Fatalf("%d bytes from %d bytes of input and %d samples", len(out), input, n)
	}
	for _, s := range f.samples[:min(n, 8)] {
		if !slices.ContainsFunc(files, func(file []byte) bool { return bytes.Contains(file, s) }) {
			t.Fatalf("a sample of %d bytes is not in the input", len(s))
		}
	}
	var dur uint64
	for _, d := range f.durations {
		dur += uint64(d)
	}
	if f.mediaDur != dur {
		t.Fatalf("media duration %d, samples last %d", f.mediaDur, dur)
	}
	if first := slices.Min(f.pts()); first != 0 {
		t.Fatalf("the first presentation time is %d", first)
	}
	if f.cttsVersion == 0 && slices.Min(f.ctts) < 0 {
		t.Fatalf("a negative offset in a version 0 ctts: %v", f.ctts)
	}
}

// FuzzParsePlaylists feeds the playlist parsers parseMaster, parseMedia and
// attributes, and the choice made from a master (testdata/fuzz/
// FuzzParsePlaylists: Apple's editorial master with H.264, HEVC and HDR
// renditions, the album and the editorial media playlists, an encrypted one,
// two init sections).
//
// It checks that none of them panics, and that:
//   - every variant has a URI that is not a tag;
//   - choose returns, for each minimum width and codec, a variant of the
//     playlist that is SDR, of that codec and has a picture, of the smallest
//     width that is wide enough (the largest if none is), and at it the
//     highest bandwidth up to maxBandwidth (the lowest when all exceed it);
//   - parseMedia returns at least one file, each of them resolved from
//     the base URL, and never reads a playlist that has an encryption key.
func FuzzParsePlaylists(f *testing.F) {
	base, err := url.Parse("https://mvod.itunes.apple.com/a/b/video.m3u8")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, body string) {
		vs := parseMaster(body)
		for i, v := range vs {
			if v.URI == "" || strings.HasPrefix(v.URI, "#") {
				t.Fatalf("variant %d URI %q", i, v.URI)
			}
		}
		for _, fam := range []string{CodecH264, CodecHEVC} {
			for _, minWidth := range []int{math.MinInt, 0, 360, 486, 1000, math.MaxInt} {
				checkChoose(t, vs, minWidth, fam)
			}
		}
		us, err := parseMedia(body, base)
		if err != nil {
			if us != nil {
				t.Fatalf("%d files with %v", len(us), err)
			}
			return
		}
		if len(us) == 0 {
			t.Fatal("no files")
		}
		for _, u := range us {
			if u == nil || u.String() == "" {
				t.Fatalf("file %v", u)
			}
		}
		for line := range strings.Lines(body) {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "#EXT-X-KEY:"); ok && attributes(rest)["METHOD"] != "NONE" {
				t.Fatalf("read a playlist with %q", line)
			}
		}
	})
}

func checkChoose(t *testing.T, vs []variant, minWidth int, fam string) {
	t.Helper()
	got, ok := choose(vs, minWidth, fam)
	var cands []variant
	for _, v := range vs {
		if v.SDR && v.Width > 0 && family(v.Codecs) == fam {
			cands = append(cands, v)
		}
	}
	if ok != (len(cands) > 0) {
		t.Fatalf("choose(%d, %s) ok = %v with %d candidates", minWidth, fam, ok, len(cands))
	}
	if !ok {
		return
	}
	width := 0
	for _, v := range cands {
		if v.Width >= minWidth && (width == 0 || v.Width < width) {
			width = v.Width
		}
	}
	if width == 0 {
		for _, v := range cands {
			width = max(width, v.Width)
		}
	}
	var under, all []variant
	for _, v := range cands {
		if v.Width != width {
			continue
		}
		all = append(all, v)
		if v.Bandwidth <= maxBandwidth {
			under = append(under, v)
		}
	}
	want := slices.MinFunc(all, func(x, y variant) int { return cmp.Compare(x.Bandwidth, y.Bandwidth) }).Bandwidth
	if len(under) > 0 {
		want = slices.MaxFunc(under, func(x, y variant) int { return cmp.Compare(x.Bandwidth, y.Bandwidth) }).Bandwidth
	}
	if got.Width != width || got.Bandwidth != want || !slices.Contains(vs, got) {
		t.Fatalf("choose(%d, %s) = %+v, want width %d, bandwidth %d", minWidth, fam, got, width, want)
	}
}
