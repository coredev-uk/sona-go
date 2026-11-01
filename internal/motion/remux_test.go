package motion

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
)

// be encodes integers big-endian at their own width.
func be(vs ...any) []byte {
	var b []byte
	for _, v := range vs {
		switch v := v.(type) {
		case uint8:
			b = append(b, v)
		case uint16:
			b = binary.BigEndian.AppendUint16(b, v)
		case uint32:
			b = binary.BigEndian.AppendUint32(b, v)
		case int32:
			b = binary.BigEndian.AppendUint32(b, uint32(v))
		case uint64:
			b = binary.BigEndian.AppendUint64(b, v)
		case int: // a run of zero bytes
			b = append(b, make([]byte, v)...)
		case string:
			b = append(b, v...)
		default:
			panic(fmt.Sprintf("be: %T", v))
		}
	}
	return b
}

// mkbox builds a box from its type and body parts.
func mkbox(typ string, parts ...[]byte) []byte {
	body := slices.Concat(parts...)
	return slices.Concat(be(uint32(8+len(body)), typ), body)
}

// frame is one sample of a synthetic fragment.
type frame struct {
	cto  int32 // composition offset
	sync bool
}

// testFile describes a synthetic fragmented MP4 shaped like Apple's motion
// renditions.
type testFile struct {
	movieScale, scale uint32
	base              uint64 // tfdt of the first fragment
	trunVersion       uint8  // 1: signed composition offsets
	frags             [][]frame
}

// payload is a sample's data: unique per sample, of varying size.
func payload(n int) []byte {
	return bytes.Repeat(fmt.Appendf(nil, "frame%04d.", n), 3+n%5)
}

// build writes ftyp, moov (one H.264 video track; trex defaults: duration
// 1, non-sync) and per fragment a moof + mdat. Each traf is based on its
// moof (default-base-is-moof) with a tfhd default size and flags. Its
// first trun has Apple's layout (data offset, sizes, flags, offsets); the
// second carries first-sample flags, durations, sizes and offsets and no
// data offset, so its data follows the first's and its other frames take
// the default flags (non-sync).
func (f testFile) build() []byte {
	stsd := mkbox("stsd", be(uint32(0), uint32(1)), mkbox("avc1", be(6, uint16(1), 70), mkbox("avcC", be("cfg"))))
	trak := mkbox("trak",
		mkbox("tkhd", be(uint32(3), 8, uint32(1), 8, 52, uint32(480<<16), uint32(480<<16))),
		mkbox("mdia",
			mkbox("mdhd", be(uint32(0), 8, f.scale, uint32(0), uint16(0x55c4), uint16(0))),
			mkbox("hdlr", be(uint32(0), uint32(0), "vide", 12, "Video\x00")),
			mkbox("minf",
				mkbox("vmhd", be(uint32(1), 8)),
				mkbox("dinf", mkbox("dref", be(uint32(0), uint32(1)), mkbox("url ", be(uint32(1))))),
				mkbox("stbl", stsd,
					mkbox("stts", be(uint32(0), uint32(0))), mkbox("stsc", be(uint32(0), uint32(0))),
					mkbox("stsz", be(uint32(0), uint32(0), uint32(0))), mkbox("stco", be(uint32(0), uint32(0)))))))
	out := slices.Concat(
		mkbox("ftyp", be("iso6", uint32(0), "iso6", "hlsf")),
		mkbox("moov",
			mkbox("mvhd", be(uint32(0), 8, f.movieScale, uint32(0), 80)),
			trak,
			mkbox("mvex", mkbox("trex", be(uint32(0), uint32(1), uint32(1), uint32(1), uint32(0), uint32(sampleNonSync))))))
	n, dts := 0, f.base
	for i, frames := range f.frags {
		split := (len(frames) + 1) / 2
		var data []byte
		var run1, run2 []byte
		firstFlags := uint32(0)
		for j, fr := range frames {
			p := payload(n)
			n++
			data = append(data, p...)
			flags := uint32(sampleNonSync)
			if fr.sync {
				flags = 0
			}
			switch {
			case j < split:
				run1 = append(run1, be(uint32(len(p)), flags, fr.cto)...)
			case j == split:
				firstFlags = flags
				fallthrough
			default:
				if fr.sync && j > split {
					panic("build: a sync frame after the second run's first")
				}
				run2 = append(run2, be(uint32(1), uint32(len(p)), fr.cto)...)
			}
		}
		moof := func(dataOffset uint32) []byte {
			return mkbox("moof",
				mkbox("mfhd", be(uint32(0), uint32(i+1))),
				mkbox("traf",
					mkbox("tfhd", be(uint32(tfhdBaseIsMoof|tfhdDefaultSize|tfhdDefaultFlags), uint32(1), uint32(9999), uint32(sampleNonSync))),
					mkbox("tfdt", be(uint32(1)<<24, dts)),
					mkbox("trun", be(uint32(f.trunVersion)<<24|trunDataOffset|trunSize|trunFlags|trunCompOffsets, uint32(split), dataOffset), run1),
					mkbox("trun", be(uint32(f.trunVersion)<<24|trunFirstFlags|trunDuration|trunSize|trunCompOffsets, uint32(len(frames)-split), firstFlags), run2)))
		}
		size := len(moof(0))
		out = slices.Concat(out, moof(uint32(size+8)), mkbox("mdat", data))
		dts += uint64(len(frames))
	}
	return out
}

// flat is what a plain MP4 says about its single track.
type flat struct {
	top                []string // top-level box types
	mvex               bool
	movieScale, scale  uint32
	movieDur, mediaDur uint64
	cttsVersion        int // -1 without ctts
	durations          []uint32
	ctts               []int32 // per sample
	sync               []bool
	samples            [][]byte // located through stsc, stco and stsz
}

func u32(b []byte, at int) uint32 { return binary.BigEndian.Uint32(b[at:]) }

// readFlat decodes the sample table of a plain MP4 with one track.
func readFlat(t *testing.T, b []byte) flat {
	t.Helper()
	top, err := boxes(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	var f flat
	for _, x := range top {
		f.top = append(f.top, x.typ)
	}
	moov, ok := first(top, "moov")
	if !ok {
		t.Fatal("no moov")
	}
	kids, err := moov.children()
	if err != nil {
		t.Fatal(err)
	}
	_, f.mvex = first(kids, "mvex")
	get := func(types ...string) []byte {
		t.Helper()
		x, err := descend(kids, types...)
		if err != nil {
			t.Fatal(err)
		}
		return x.body
	}
	header := func(body []byte) (uint32, uint64) {
		if body[0] == 1 {
			return u32(body, 20), binary.BigEndian.Uint64(body[24:])
		}
		return u32(body, 12), uint64(u32(body, 16))
	}
	f.movieScale, f.movieDur = header(get("mvhd"))
	f.scale, f.mediaDur = header(get("trak", "mdia", "mdhd"))
	stbl := []string{"trak", "mdia", "minf", "stbl"}
	entries := func(typ string, fn func(e []byte)) {
		t.Helper()
		body := get(append(stbl, typ)...)
		width := map[string]int{"stts": 8, "ctts": 8, "stss": 4, "stsc": 12, "stco": 4}[typ]
		for i := range int(u32(body, 4)) {
			fn(body[8+i*width:])
		}
	}
	entries("stts", func(e []byte) {
		for range u32(e, 0) {
			f.durations = append(f.durations, u32(e, 4))
		}
	})
	n := len(f.durations)
	f.cttsVersion = -1
	if _, err := descend(kids, append(stbl, "ctts")...); err == nil {
		f.cttsVersion = int(get(append(stbl, "ctts")...)[0])
		entries("ctts", func(e []byte) {
			for range u32(e, 0) {
				f.ctts = append(f.ctts, int32(u32(e, 4)))
			}
		})
	} else {
		f.ctts = make([]int32, n)
	}
	f.sync = make([]bool, n)
	if _, err := descend(kids, append(stbl, "stss")...); err == nil {
		entries("stss", func(e []byte) { f.sync[u32(e, 0)-1] = true })
	} else {
		for i := range f.sync {
			f.sync[i] = true
		}
	}
	stsz := get(append(stbl, "stsz")...)
	if u32(stsz, 4) != 0 || int(u32(stsz, 8)) != n {
		t.Fatalf("stsz: size %d, count %d, want per-sample sizes for %d", u32(stsz, 4), u32(stsz, 8), n)
	}
	var chunks []int
	entries("stco", func(e []byte) { chunks = append(chunks, int(u32(e, 0))) })
	type stsc struct{ first, per int }
	var runs []stsc
	entries("stsc", func(e []byte) { runs = append(runs, stsc{int(u32(e, 0)), int(u32(e, 4))}) })
	s := 0
	for c, off := range chunks {
		per := 0
		for _, r := range runs {
			if r.first <= c+1 {
				per = r.per
			}
		}
		for range per {
			size := int(u32(stsz, 12+4*s))
			f.samples = append(f.samples, b[off:off+size])
			off += size
			s++
		}
	}
	if s != n || len(f.ctts) != n {
		t.Fatalf("%d samples in chunks, %d offsets, %d durations", s, len(f.ctts), n)
	}
	return f
}

// pts returns each sample's presentation time.
func (f flat) pts() []int64 {
	out := make([]int64, len(f.durations))
	var dts int64
	for i, d := range f.durations {
		out[i] = dts + int64(f.ctts[i])
		dts += int64(d)
	}
	return out
}

// gop is a decode-order group: I P B B with signed offsets (presentation
// order I B B P), as Apple's renditions are encoded.
var gop = []frame{{0, true}, {2, false}, {-1, false}, {-1, false}}

func TestFlatten(t *testing.T) {
	frags := [][]frame{slices.Concat(gop, gop), slices.Concat(gop, gop[:3])}
	for _, tc := range []struct {
		name string
		file testFile
		// ctts version and per-sample offsets of the output.
		version int
		offsets []int32
	}{{
		// Apple's shape: timescale 30, the first frame at 10 s, signed
		// offsets with the first frame presented at its decode time.
		name:    "signed",
		file:    testFile{movieScale: 30, scale: 30, base: 300, trunVersion: 1, frags: frags},
		version: 1,
		offsets: []int32{0, 2, -1, -1, 0, 2, -1, -1, 0, 2, -1, -1, 0, 2, -1},
	}, {
		// Unsigned offsets put the first frame at 1 (and the movie in
		// another timescale): rebased to 0, which needs signed offsets.
		name: "unsigned",
		file: testFile{movieScale: 1000, scale: 30, base: 450, trunVersion: 0, frags: [][]frame{
			{{1, true}, {3, false}, {0, false}, {0, false}, {1, true}, {3, false}, {0, false}, {0, false}},
		}},
		version: 1,
		offsets: []int32{0, 2, -1, -1, 0, 2, -1, -1},
	}, {
		name:    "no reordering",
		file:    testFile{movieScale: 30, scale: 30, base: 300, trunVersion: 1, frags: [][]frame{{{0, true}, {0, false}, {0, false}}, {{0, true}, {0, false}}}},
		version: -1,
		offsets: []int32{0, 0, 0, 0, 0},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.file.build()
			out, err := flatten(src)
			if err != nil {
				t.Fatal(err)
			}
			f := readFlat(t, out)
			if !slices.Equal(f.top, []string{"ftyp", "moov", "mdat"}) || f.mvex {
				t.Fatalf("boxes %v (mvex %v), want ftyp moov mdat", f.top, f.mvex)
			}
			n := len(tc.offsets)
			if len(f.durations) != n {
				t.Fatalf("%d frames, want %d", len(f.durations), n)
			}
			if f.scale != tc.file.scale || f.mediaDur != uint64(n) {
				t.Errorf("media duration %d at %d/s, want %d at %d/s", f.mediaDur, f.scale, n, tc.file.scale)
			}
			if want := uint64(n) * uint64(tc.file.movieScale) / uint64(tc.file.scale); f.movieScale != tc.file.movieScale || f.movieDur != want {
				t.Errorf("movie duration %d at %d/s, want %d", f.movieDur, f.movieScale, want)
			}
			pts := f.pts()
			if pts[0] != 0 || slices.Min(pts) != 0 {
				t.Errorf("first frame presented at %d, earliest at %d; want both 0", pts[0], slices.Min(pts))
			}
			if f.cttsVersion != tc.version || !slices.Equal(f.ctts, tc.offsets) {
				t.Errorf("ctts v%d %v, want v%d %v", f.cttsVersion, f.ctts, tc.version, tc.offsets)
			}
			// Every frame keeps its bytes, in decode order, and
			// keyframes stay sync samples.
			i := 0
			for _, frames := range tc.file.frags {
				for _, fr := range frames {
					if !bytes.Equal(f.samples[i], payload(i)) {
						t.Fatalf("frame %d = %q, want %q", i, f.samples[i], payload(i))
					}
					if f.sync[i] != fr.sync {
						t.Errorf("frame %d sync %v, want %v", i, f.sync[i], fr.sync)
					}
					i++
				}
			}
			// A plain file is left as it is.
			again, err := flatten(out)
			if err != nil || !bytes.Equal(again, out) {
				t.Errorf("flatten(plain) changed it (err %v)", err)
			}
		})
	}
}

// Damaged files fail without panicking, and never yield frames from
// outside the file.
func TestFlattenDamaged(t *testing.T) {
	src := testFile{movieScale: 30, scale: 30, base: 300, trunVersion: 1, frags: [][]frame{gop, gop}}.build()
	for n := range len(src) {
		out, err := flatten(src[:n])
		if err != nil {
			continue
		}
		// Only a cut between boxes parses: it keeps the whole fragments.
		for i, s := range readFlat(t, out).samples {
			if !bytes.Equal(s, payload(i)) {
				t.Fatalf("cut at %d: frame %d = %q", n, i, s)
			}
		}
	}
	garble := func(at int, v uint32) []byte {
		b := bytes.Clone(src)
		binary.BigEndian.PutUint32(b[bytes.Index(b, []byte("trun"))+at:], v)
		return b
	}
	if _, err := flatten(garble(8, 1<<30)); err == nil {
		t.Error("a trun claiming 2^30 samples was accepted")
	}
	if _, err := flatten(garble(12, 1<<20)); err == nil {
		t.Error("a trun whose data lies past the end of the file was accepted")
	}
	if _, err := flatten([]byte("not an mp4 at all")); err == nil {
		t.Error("garbage was accepted")
	}
}

// segmented splits a fragmented file the way Apple packages editorial
// motion: an initialisation file (ftyp, moov) and one segment file per
// moof + mdat, each starting with an styp box.
func segmented(t *testing.T, file []byte) [][]byte {
	t.Helper()
	top, err := boxes(file, 0)
	if err != nil {
		t.Fatal(err)
	}
	var init []byte
	var segs [][]byte
	for _, b := range top {
		switch b.typ {
		case "ftyp", "moov":
			init = append(init, b.raw...)
		case "moof":
			segs = append(segs, slices.Concat(mkbox("styp", be("msdh", uint32(0), "msdh", "msix")), b.raw))
		case "mdat":
			segs[len(segs)-1] = append(segs[len(segs)-1], b.raw...)
		}
	}
	return append([][]byte{init}, segs...)
}

// An initialisation file and segment files flatten to the same video as
// the single file they were cut from.
func TestFlattenSegments(t *testing.T) {
	src := testFile{movieScale: 600, scale: 30, base: 300, trunVersion: 1, frags: [][]frame{gop, gop, gop[:3]}}.build()
	want, err := flatten(src)
	if err != nil {
		t.Fatal(err)
	}
	parts := segmented(t, src)
	if len(parts) != 4 {
		t.Fatalf("%d parts, want an init and 3 segments", len(parts))
	}
	got, err := flatten(parts...)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("segments flattened to %d bytes, differing from the single file's %d", len(got), len(want))
	}
	// An initialisation file alone has no frames.
	if _, err := flatten(parts[0], parts[0]); err == nil {
		t.Error("an init file without segments was accepted")
	}
	// A segment that is not an MP4 fails.
	if _, err := flatten(parts[0], []byte("garbage!")); err == nil {
		t.Error("a garbage segment was accepted")
	}
}
