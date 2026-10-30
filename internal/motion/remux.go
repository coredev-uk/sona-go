package motion

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/bits"
)

// Apple's motion renditions are fragmented MP4 (ftyp, a moov with mvex,
// then moof + mdat pairs) whose first frame sits at the fragments' base
// decode time, 10 s into Apple's timeline. WebKitGTK's GStreamer player
// reports "playing" on such a file but never advances (currentTime stays
// 0). flatten rewrites it as a plain MP4 (ftyp, moov with the whole sample
// table, one mdat) whose timeline starts at 0, which it plays and loops.

// maxSamples bounds the frames flatten accepts: over an hour at 60 fps,
// far beyond a motion loop, so a corrupt sample count cannot exhaust
// memory.
const maxSamples = 1 << 18

var errTruncated = errors.New("mp4: truncated box")

// box is one ISO BMFF box of a buffer.
type box struct {
	typ  string
	at   int    // offset of the box's header in the file
	raw  []byte // the whole box, header included
	body []byte
}

// boxes splits b, which starts at file offset at, into boxes.
func boxes(b []byte, at int) ([]box, error) {
	var out []box
	for i := 0; i < len(b); {
		if len(b)-i < 8 {
			return nil, errTruncated
		}
		size := uint64(binary.BigEndian.Uint32(b[i:]))
		typ := string(b[i+4 : i+8])
		hdr := 8
		switch size {
		case 0: // to the end of the container
			size = uint64(len(b) - i)
		case 1:
			if len(b)-i < 16 {
				return nil, errTruncated
			}
			size, hdr = binary.BigEndian.Uint64(b[i+8:]), 16
		}
		if size < uint64(hdr) || size > uint64(len(b)-i) {
			return nil, fmt.Errorf("mp4: %q box size %d out of range", typ, size)
		}
		end := i + int(size)
		out = append(out, box{typ: typ, at: at + i, raw: b[i:end], body: b[i+hdr : end]})
		i = end
	}
	return out, nil
}

func (b box) children() ([]box, error) { return boxes(b.body, b.at+len(b.raw)-len(b.body)) }

func first(bs []box, typ string) (box, bool) {
	for _, b := range bs {
		if b.typ == typ {
			return b, true
		}
	}
	return box{}, false
}

// descend finds the first box at the end of a chain of box types.
func descend(bs []box, types ...string) (box, error) {
	var cur box
	for i, t := range types {
		if i > 0 {
			var err error
			if bs, err = cur.children(); err != nil {
				return box{}, err
			}
		}
		var ok bool
		if cur, ok = first(bs, t); !ok {
			return box{}, fmt.Errorf("mp4: no %s box", t)
		}
	}
	return cur, nil
}

// reader is a bounds-checked big-endian cursor; a short read sets bad and
// returns zeros.
type reader struct {
	b   []byte
	bad bool
}

func (r *reader) take(n int) []byte {
	if r.bad || n > len(r.b) {
		r.bad = true
		return make([]byte, n)
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

func (r *reader) u16() uint16 { return binary.BigEndian.Uint16(r.take(2)) }
func (r *reader) u32() uint32 { return binary.BigEndian.Uint32(r.take(4)) }
func (r *reader) u64() uint64 { return binary.BigEndian.Uint64(r.take(8)) }

// full reads a FullBox version and flags.
func (r *reader) full() (version uint8, flags uint32) {
	v := r.u32()
	return uint8(v >> 24), v & 0xffffff
}

// defaults are a track's sample defaults (trex, overridden per fragment by
// tfhd).
type defaults struct {
	desc, duration, size, flags uint32
}

// track is the video track of the init section.
type track struct {
	id            uint32
	movieScale    uint32 // mvhd timescale
	scale         uint32 // mdhd timescale
	language      uint16 // mdhd packed ISO-639-2 code
	width, height uint32 // tkhd, 16.16 fixed point
	// Copied unchanged into the output.
	hdlr, vmhd, dinf, stsd []byte
	defaults               defaults
}

type sample struct {
	duration uint32
	cto      int64 // composition offset: presentation minus decode time
	sync     bool
	data     []byte // aliases the source file
}

// run is a trun's samples, stored contiguously: one output chunk.
type run struct {
	n    int
	desc uint32 // sample description index
}

// sampleNonSync is the sample_is_non_sync_sample bit of sample flags.
const sampleNonSync = 0x00010000

// flatten rewrites the fragmented MP4 src as a plain one: ftyp, moov, then
// one mdat with every frame of the video track in decode order. Decode
// times start at 0, and composition offsets are rebased so the earliest
// frame is presented at 0 with no edit list (signed offsets, ctts version
// 1, when B-frames need them). A file without fragments is returned as
// is. Other tracks are dropped.
func flatten(src []byte) ([]byte, error) {
	top, err := boxes(src, 0)
	if err != nil {
		return nil, err
	}
	moov, ok := first(top, "moov")
	if !ok {
		return nil, errors.New("mp4: no moov box")
	}
	if _, ok := first(top, "moof"); !ok {
		return src, nil
	}
	tr, err := parseInit(moov)
	if err != nil {
		return nil, err
	}
	var samples []sample
	var runs []run
	for _, b := range top {
		if b.typ != "moof" {
			continue
		}
		if samples, runs, err = parseFragment(src, b, tr, samples, runs); err != nil {
			return nil, err
		}
	}
	if len(samples) == 0 {
		return nil, errors.New("mp4: no video frames")
	}
	return write(tr, samples, runs)
}

// parseInit reads the video track and its defaults from moov.
func parseInit(moov box) (*track, error) {
	kids, err := moov.children()
	if err != nil {
		return nil, err
	}
	mvhd, err := descend(kids, "mvhd")
	if err != nil {
		return nil, err
	}
	tr := &track{}
	r := reader{b: mvhd.body}
	if v, _ := r.full(); v == 1 {
		r.take(16)
	} else {
		r.take(8)
	}
	tr.movieScale = r.u32()
	if r.bad || tr.movieScale == 0 {
		return nil, errors.New("mp4: bad mvhd")
	}
	found := false
	for _, k := range kids {
		if k.typ != "trak" {
			continue
		}
		if found, err = tr.parseTrak(k); err != nil {
			return nil, err
		}
		if found {
			break
		}
	}
	if !found {
		return nil, errors.New("mp4: no video track")
	}
	if mvex, ok := first(kids, "mvex"); ok {
		ex, err := mvex.children()
		if err != nil {
			return nil, err
		}
		for _, b := range ex {
			if b.typ != "trex" {
				continue
			}
			r := reader{b: b.body}
			r.full()
			if r.u32() != tr.id {
				continue
			}
			tr.defaults = defaults{desc: r.u32(), duration: r.u32(), size: r.u32(), flags: r.u32()}
			if r.bad {
				return nil, errors.New("mp4: bad trex")
			}
		}
	}
	return tr, nil
}

// parseTrak fills tr from trak when it is a video track.
func (tr *track) parseTrak(trak box) (bool, error) {
	kids, err := trak.children()
	if err != nil {
		return false, err
	}
	hdlr, err := descend(kids, "mdia", "hdlr")
	if err != nil {
		return false, err
	}
	r := reader{b: hdlr.body}
	r.take(8) // version, flags, pre_defined
	if string(r.take(4)) != "vide" {
		return false, nil
	}
	tkhd, err := descend(kids, "tkhd")
	if err != nil {
		return false, err
	}
	r = reader{b: tkhd.body}
	if v, _ := r.full(); v == 1 {
		r.take(16)
		tr.id = r.u32()
		r.take(12)
	} else {
		r.take(8)
		tr.id = r.u32()
		r.take(8)
	}
	r.take(52) // reserved, layer, alternate_group, volume, reserved, matrix
	tr.width, tr.height = r.u32(), r.u32()
	mdhd, err := descend(kids, "mdia", "mdhd")
	if err != nil {
		return false, err
	}
	m := reader{b: mdhd.body}
	if v, _ := m.full(); v == 1 {
		m.take(16)
		tr.scale = m.u32()
		m.take(8)
	} else {
		m.take(8)
		tr.scale = m.u32()
		m.take(4)
	}
	tr.language = m.u16()
	if r.bad || m.bad || tr.scale == 0 {
		return false, errors.New("mp4: bad tkhd or mdhd")
	}
	tr.hdlr = hdlr.raw
	for _, p := range []struct {
		dst   *[]byte
		types []string
	}{
		{&tr.vmhd, []string{"mdia", "minf", "vmhd"}},
		{&tr.dinf, []string{"mdia", "minf", "dinf"}},
		{&tr.stsd, []string{"mdia", "minf", "stbl", "stsd"}},
	} {
		b, err := descend(kids, p.types...)
		if err != nil {
			return false, err
		}
		*p.dst = b.raw
	}
	return true, nil
}

// tfhd and trun flags.
const (
	tfhdBaseDataOffset  = 0x000001
	tfhdDescIndex       = 0x000002
	tfhdDefaultDuration = 0x000008
	tfhdDefaultSize     = 0x000010
	tfhdDefaultFlags    = 0x000020
	tfhdBaseIsMoof      = 0x020000

	trunDataOffset  = 0x000001
	trunFirstFlags  = 0x000004
	trunDuration    = 0x000100
	trunSize        = 0x000200
	trunFlags       = 0x000400
	trunCompOffsets = 0x000800
)

// parseFragment appends the video track's samples of one moof (their data
// in src) and one run per trun.
func parseFragment(src []byte, moof box, tr *track, samples []sample, runs []run) ([]sample, []run, error) {
	trafs, err := moof.children()
	if err != nil {
		return nil, nil, err
	}
	// The data of a traf without an explicit base follows the previous
	// traf's, the first one's the moof.
	next := int64(moof.at)
	for _, traf := range trafs {
		if traf.typ != "traf" {
			continue
		}
		kids, err := traf.children()
		if err != nil {
			return nil, nil, err
		}
		tfhd, err := descend(kids, "tfhd")
		if err != nil {
			return nil, nil, err
		}
		r := reader{b: tfhd.body}
		_, flags := r.full()
		if r.u32() != tr.id {
			continue
		}
		d := tr.defaults
		base := next
		switch {
		case flags&tfhdBaseDataOffset != 0:
			base = int64(r.u64())
		case flags&tfhdBaseIsMoof != 0:
			base = int64(moof.at)
		}
		if flags&tfhdDescIndex != 0 {
			d.desc = r.u32()
		}
		if flags&tfhdDefaultDuration != 0 {
			d.duration = r.u32()
		}
		if flags&tfhdDefaultSize != 0 {
			d.size = r.u32()
		}
		if flags&tfhdDefaultFlags != 0 {
			d.flags = r.u32()
		}
		if r.bad || base < 0 {
			return nil, nil, errors.New("mp4: bad tfhd")
		}
		pos := base
		for _, trun := range kids {
			if trun.typ != "trun" {
				continue
			}
			r := reader{b: trun.body}
			version, flags := r.full()
			n := r.u32()
			if flags&trunDataOffset != 0 {
				pos = base + int64(int32(r.u32()))
			}
			firstFlags, hasFirst := d.flags, flags&trunFirstFlags != 0
			if hasFirst {
				firstFlags = r.u32()
			}
			fields := bits.OnesCount32(flags & (trunDuration | trunSize | trunFlags | trunCompOffsets))
			if r.bad || uint64(n)*uint64(4*fields) > uint64(len(r.b)) || len(samples)+int(n) > maxSamples {
				return nil, nil, errors.New("mp4: bad trun")
			}
			for i := range int(n) {
				s := sample{duration: d.duration}
				size, sf := d.size, d.flags
				if flags&trunDuration != 0 {
					s.duration = r.u32()
				}
				if flags&trunSize != 0 {
					size = r.u32()
				}
				if flags&trunFlags != 0 {
					sf = r.u32()
				}
				if i == 0 && hasFirst {
					sf = firstFlags
				}
				if flags&trunCompOffsets != 0 {
					if v := r.u32(); version == 0 {
						s.cto = int64(v)
					} else {
						s.cto = int64(int32(v))
					}
				}
				s.sync = sf&sampleNonSync == 0
				if pos < 0 || pos+int64(size) > int64(len(src)) {
					return nil, nil, errors.New("mp4: sample data outside the file")
				}
				s.data = src[pos : pos+int64(size)]
				pos += int64(size)
				samples = append(samples, s)
			}
			if n > 0 {
				runs = append(runs, run{n: int(n), desc: max(d.desc, 1)})
			}
		}
		next = pos
	}
	return samples, runs, nil
}

var unityMatrix = [9]uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000}

// write lays out ftyp, moov and mdat. Chunk offsets depend on the moov's
// size, which does not depend on their values: the moov is built once to
// measure it, then with the real offsets.
func write(tr *track, samples []sample, runs []run) ([]byte, error) {
	var dts, payload int64
	minPTS := int64(math.MaxInt64)
	for _, s := range samples {
		minPTS = min(minPTS, dts+s.cto)
		dts += int64(s.duration)
		payload += int64(len(s.data))
	}
	signed, offsets := false, false
	for i := range samples {
		cto := samples[i].cto - minPTS
		if cto < math.MinInt32 || cto > math.MaxInt32 {
			return nil, errors.New("mp4: composition offset out of range")
		}
		samples[i].cto = cto
		signed = signed || cto < 0
		offsets = offsets || cto != 0
	}
	l := layout{tr: tr, samples: samples, runs: runs, duration: uint64(dts), signed: signed, offsets: offsets}
	ftyp := l.ftyp()
	base := int64(len(ftyp)) + int64(len(l.moov(0))) + 8
	if base+payload > math.MaxUint32 {
		return nil, errors.New("mp4: video too large")
	}
	out := make([]byte, 0, base+payload)
	out = append(out, ftyp...)
	out = append(out, l.moov(uint32(base))...)
	out = binary.BigEndian.AppendUint32(out, uint32(8+payload))
	out = append(out, "mdat"...)
	for _, s := range samples {
		out = append(out, s.data...)
	}
	return out, nil
}

type layout struct {
	tr       *track
	samples  []sample
	runs     []run
	duration uint64 // in the media timescale
	signed   bool   // a composition offset is negative
	offsets  bool   // a composition offset is not 0
}

func (l *layout) ftyp() []byte {
	var w writer
	w.box("ftyp", func() {
		w.str("isom")
		w.u32(0x200)
		w.str("isom")
		w.str("iso2")
		if l.signed {
			w.str("iso4") // ctts version 1
		}
		w.str("mp41")
	})
	return w.b
}

// moov builds the movie box with chunk offsets counted from mdat, the
// file offset of the first sample.
func (l *layout) moov(mdat uint32) []byte {
	movieDur := scaleDuration(l.duration, l.tr.movieScale, l.tr.scale)
	var w writer
	w.box("moov", func() {
		w.fullBox("mvhd", version(movieDur), 0, func() {
			w.times(movieDur, l.tr.movieScale)
			w.u32(0x00010000) // rate 1.0
			w.u16(0x0100)     // volume 1.0
			w.zeros(10)
			w.matrix()
			w.zeros(24)
			w.u32(l.tr.id + 1) // next_track_ID
		})
		w.box("trak", func() {
			w.fullBox("tkhd", version(movieDur), 0x3, func() { // enabled, in movie
				if version(movieDur) == 1 {
					w.zeros(16)
					w.u32(l.tr.id)
					w.zeros(4)
					w.u64(movieDur)
				} else {
					w.zeros(8)
					w.u32(l.tr.id)
					w.zeros(4)
					w.u32(uint32(movieDur))
				}
				w.zeros(16) // reserved, layer, alternate_group, volume, reserved
				w.matrix()
				w.u32(l.tr.width)
				w.u32(l.tr.height)
			})
			w.box("mdia", func() {
				w.fullBox("mdhd", version(l.duration), 0, func() {
					w.times(l.duration, l.tr.scale)
					w.u16(l.tr.language)
					w.u16(0)
				})
				w.raw(l.tr.hdlr)
				w.box("minf", func() {
					w.raw(l.tr.vmhd)
					w.raw(l.tr.dinf)
					w.box("stbl", func() { l.stbl(&w, mdat) })
				})
			})
		})
	})
	return w.b
}

func (l *layout) stbl(w *writer, mdat uint32) {
	w.raw(l.tr.stsd)
	type entry struct{ count, value uint32 }
	// rle run-length encodes value(i) over the samples.
	rle := func(value func(sample) uint32) []entry {
		var es []entry
		for _, s := range l.samples {
			v := value(s)
			if n := len(es); n > 0 && es[n-1].value == v {
				es[n-1].count++
			} else {
				es = append(es, entry{1, v})
			}
		}
		return es
	}
	table := func(typ string, version uint8, es []entry) {
		w.fullBox(typ, version, 0, func() {
			w.u32(uint32(len(es)))
			for _, e := range es {
				w.u32(e.count)
				w.u32(e.value)
			}
		})
	}
	table("stts", 0, rle(func(s sample) uint32 { return s.duration }))
	if l.offsets {
		v := uint8(0)
		if l.signed {
			v = 1
		}
		table("ctts", v, rle(func(s sample) uint32 { return uint32(int32(s.cto)) }))
	}
	syncs := 0
	for _, s := range l.samples {
		if s.sync {
			syncs++
		}
	}
	if syncs < len(l.samples) {
		w.fullBox("stss", 0, 0, func() {
			w.u32(uint32(syncs))
			for i, s := range l.samples {
				if s.sync {
					w.u32(uint32(i + 1))
				}
			}
		})
	}
	w.fullBox("stsc", 0, 0, func() {
		at := len(w.b)
		w.u32(0)
		entries := 0
		for i, r := range l.runs {
			if i > 0 && r == l.runs[i-1] {
				continue
			}
			w.u32(uint32(i + 1)) // first_chunk
			w.u32(uint32(r.n))
			w.u32(r.desc)
			entries++
		}
		binary.BigEndian.PutUint32(w.b[at:], uint32(entries))
	})
	w.fullBox("stsz", 0, 0, func() {
		w.u32(0) // sizes vary
		w.u32(uint32(len(l.samples)))
		for _, s := range l.samples {
			w.u32(uint32(len(s.data)))
		}
	})
	w.fullBox("stco", 0, 0, func() {
		w.u32(uint32(len(l.runs)))
		off, i := mdat, 0
		for _, r := range l.runs {
			w.u32(off)
			for _, s := range l.samples[i : i+r.n] {
				off += uint32(len(s.data))
			}
			i += r.n
		}
	})
}

// scaleDuration converts d from timescale from to timescale to.
func scaleDuration(d uint64, to, from uint32) uint64 {
	if to == from {
		return d
	}
	hi, lo := bits.Mul64(d, uint64(to))
	if hi >= uint64(from) {
		return math.MaxUint64
	}
	q, _ := bits.Div64(hi, lo, uint64(from))
	return q
}

func version(duration uint64) uint8 {
	if duration > math.MaxUint32 {
		return 1
	}
	return 0
}

// writer appends boxes.
type writer struct{ b []byte }

func (w *writer) u16(v uint16) { w.b = binary.BigEndian.AppendUint16(w.b, v) }
func (w *writer) u32(v uint32) { w.b = binary.BigEndian.AppendUint32(w.b, v) }
func (w *writer) u64(v uint64) { w.b = binary.BigEndian.AppendUint64(w.b, v) }
func (w *writer) str(s string) { w.b = append(w.b, s...) }
func (w *writer) raw(b []byte) { w.b = append(w.b, b...) }
func (w *writer) zeros(n int)  { w.b = append(w.b, make([]byte, n)...) }
func (w *writer) box(typ string, body func()) {
	at := len(w.b)
	w.u32(0)
	w.str(typ)
	body()
	binary.BigEndian.PutUint32(w.b[at:], uint32(len(w.b)-at))
}

func (w *writer) fullBox(typ string, version uint8, flags uint32, body func()) {
	w.box(typ, func() {
		w.u32(uint32(version)<<24 | flags)
		body()
	})
}

// times writes the creation and modification times (0) and the timescale
// and duration of an mvhd or mdhd of the given version.
func (w *writer) times(duration uint64, scale uint32) {
	if version(duration) == 1 {
		w.zeros(16)
		w.u32(scale)
		w.u64(duration)
		return
	}
	w.zeros(8)
	w.u32(scale)
	w.u32(uint32(duration))
}

func (w *writer) matrix() {
	for _, v := range unityMatrix {
		w.u32(v)
	}
}
