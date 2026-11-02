package fmp4

import (
	"errors"
	"fmt"
	"time"
)

// ParseInit parses an init segment (ftyp + moov) with one track: audio
// (AAC) or, for music videos, video (H.264 or HEVC).
func ParseInit(b []byte) (*Init, error) {
	top, err := boxes(b, 0)
	if err != nil {
		return nil, err
	}
	moov, ok := find(top, "moov")
	if !ok {
		return nil, errors.New("fmp4: init segment has no moov")
	}
	mc, err := children(moov)
	if err != nil {
		return nil, err
	}
	in := &Init{}
	var trak box
	found := false
	for _, t := range mc {
		if t.typ == "trak" {
			if found {
				return nil, errors.New("fmp4: init segment has more than one track")
			}
			trak, found = t, true
		}
	}
	if !found {
		return nil, errors.New("fmp4: init segment has no trak")
	}
	if err := in.parseTrak(trak); err != nil {
		return nil, err
	}
	if mvex, ok := find(mc, "mvex"); ok {
		mv, err := children(mvex)
		if err != nil {
			return nil, err
		}
		for _, t := range mv {
			if t.typ != "trex" {
				continue
			}
			r := reader{b: t.body}
			r.full()
			id := r.u32()
			r.u32() // default_sample_description_index
			dur, size := r.u32(), r.u32()
			if r.err != nil {
				return nil, fmt.Errorf("fmp4: trex: %w", r.err)
			}
			if id == in.TrackID {
				in.DefaultSampleDuration, in.DefaultSampleSize = dur, size
			}
		}
	}
	return in, nil
}

func (in *Init) parseTrak(trak box) error {
	tc, err := children(trak)
	if err != nil {
		return err
	}
	if tkhd, ok := find(tc, "tkhd"); ok {
		r := reader{b: tkhd.body}
		v, _ := r.full()
		if v == 1 {
			r.take(16)
		} else {
			r.take(8)
		}
		in.TrackID = r.u32()
		if r.err != nil {
			return fmt.Errorf("fmp4: tkhd: %w", r.err)
		}
	}
	if edts, ok := find(tc, "edts"); ok {
		if err := in.parseEdts(edts); err != nil {
			return err
		}
	}
	mdia, ok := find(tc, "mdia")
	if !ok {
		return errors.New("fmp4: trak has no mdia")
	}
	md, err := children(mdia)
	if err != nil {
		return err
	}
	mdhd, ok := find(md, "mdhd")
	if !ok {
		return errors.New("fmp4: mdia has no mdhd")
	}
	r := reader{b: mdhd.body}
	if v, _ := r.full(); v == 1 {
		r.take(16)
	} else {
		r.take(8)
	}
	in.Timescale = r.u32()
	if r.err != nil || in.Timescale == 0 {
		return fmt.Errorf("fmp4: mdhd: bad timescale (%v)", r.err)
	}
	stsd, err := path(md, "minf", "stbl", "stsd")
	if err != nil {
		return err
	}
	r = reader{b: stsd.body}
	r.full()
	if n := r.u32(); r.err != nil || n == 0 {
		return errors.New("fmp4: empty stsd")
	}
	entries, err := boxes(stsd.body[8:], stsd.bodyAt+8)
	if err != nil {
		return err
	}
	return in.parseSampleEntry(entries[0])
}

// AACEncoderDelay is the priming of Apple's AAC encoder in media timescale
// units (2112 samples of the core codec, for AAC-LC and HE-AAC alike).
// Apple's fMP4 init segments carry no edit list or iTunSMPB; their HLS
// playlists count the priming but not the end padding, so a song's
// catalog duration is the playlist's minus these samples (checked on AAC
// 256 and HE-AAC 64 flavors).
const AACEncoderDelay = 2112

// parseEdts reads the priming from an edit list whose first entry starts
// the presentation at media_time.
func (in *Init) parseEdts(edts box) error {
	ec, err := children(edts)
	if err != nil {
		return err
	}
	elst, ok := find(ec, "elst")
	if !ok {
		return nil
	}
	r := reader{b: elst.body}
	v, _ := r.full()
	n := r.u32()
	for range n {
		var mediaTime int64
		if v == 1 {
			r.u64() // segment_duration
			mediaTime = int64(r.u64())
		} else {
			r.u32()
			mediaTime = int64(int32(r.u32()))
		}
		r.take(4) // media_rate
		if r.err != nil {
			return fmt.Errorf("fmp4: elst: %w", r.err)
		}
		if mediaTime >= 0 { // -1 is an empty edit (a delay)
			in.EditStart, in.HasEdit = mediaTime, true
			return nil
		}
	}
	return nil
}

// Priming is the media time of the track's first audible sample: the edit
// list's start, else the AAC encoder delay.
func (in *Init) Priming() time.Duration {
	if in.HasEdit {
		return Time(in.EditStart, in.Timescale)
	}
	return Time(AACEncoderDelay, in.Timescale)
}

// path descends through nested container boxes.
func path(bs []box, names ...string) (box, error) {
	var b box
	for i, n := range names {
		var ok bool
		if b, ok = find(bs, n); !ok {
			return box{}, fmt.Errorf("fmp4: missing %s", n)
		}
		if i < len(names)-1 {
			var err error
			if bs, err = children(b); err != nil {
				return box{}, err
			}
		}
	}
	return b, nil
}

// parseSampleEntry parses the track's sample entry: audio ('mp4a' or
// 'enca') or video ('avc1', 'avc3', 'hvc1', 'hev1' or 'encv').
func (in *Init) parseSampleEntry(e box) error {
	switch e.typ {
	case "encv", "avc1", "avc3", "hvc1", "hev1":
		return in.parseVisualEntry(e)
	}
	in.Codec = e.typ
	const audioEntryHeader = 28 // reserved(6) dref(2) reserved(8) channels(2) size(2) predef(2) reserved(2) rate(4)
	if len(e.body) < audioEntryHeader {
		return fmt.Errorf("fmp4: %s sample entry: %w", e.typ, ErrTruncated)
	}
	r := reader{b: e.body}
	r.take(16)
	in.Channels = int(r.u16())
	r.take(6)
	in.SampleRate = int(r.u32() >> 16)
	kids, err := boxes(e.body[audioEntryHeader:], e.bodyAt+audioEntryHeader)
	if err != nil {
		return err
	}
	if e.typ == "enca" {
		sinf, ok := find(kids, "sinf")
		if !ok {
			return errors.New("fmp4: enca without sinf")
		}
		if err := in.parseSinf(sinf); err != nil {
			return err
		}
	}
	if in.Codec != "mp4a" {
		return fmt.Errorf("fmp4: unsupported codec %q", in.Codec)
	}
	esds, ok := find(kids, "esds")
	if !ok {
		return errors.New("fmp4: mp4a without esds")
	}
	return in.parseESDS(esds.body)
}

// videoConfigBox is the decoder configuration box of each video codec.
var videoConfigBox = map[string]string{"avc1": "avcC", "avc3": "avcC", "hvc1": "hvcC", "hev1": "hvcC"}

// parseVisualEntry parses a VisualSampleEntry: the picture size and the
// codec's decoder configuration (avcC or hvcC).
func (in *Init) parseVisualEntry(e box) error {
	in.Codec = e.typ
	const visualEntryHeader = 78 // reserved(6) dref(2) predef/reserved(16) width(2) height(2) resolution(8) reserved(4) frames(2) compressor(32) depth(2) predef(2)
	if len(e.body) < visualEntryHeader {
		return fmt.Errorf("fmp4: %s sample entry: %w", e.typ, ErrTruncated)
	}
	r := reader{b: e.body}
	r.take(24)
	in.Width, in.Height = int(r.u16()), int(r.u16())
	kids, err := boxes(e.body[visualEntryHeader:], e.bodyAt+visualEntryHeader)
	if err != nil {
		return err
	}
	if e.typ == "encv" {
		sinf, ok := find(kids, "sinf")
		if !ok {
			return errors.New("fmp4: encv without sinf")
		}
		if err := in.parseSinf(sinf); err != nil {
			return err
		}
	}
	name, ok := videoConfigBox[in.Codec]
	if !ok {
		return fmt.Errorf("fmp4: unsupported codec %q", in.Codec)
	}
	cfg, ok := find(kids, name)
	if !ok || len(cfg.body) == 0 {
		return fmt.Errorf("fmp4: %s without %s", in.Codec, name)
	}
	in.VideoConfig = cfg.body
	return nil
}

// IsVideo reports whether the track is video.
func (in *Init) IsVideo() bool { return in.VideoConfig != nil }

func (in *Init) parseSinf(sinf box) error {
	sc, err := children(sinf)
	if err != nil {
		return err
	}
	if frma, ok := find(sc, "frma"); ok && len(frma.body) >= 4 {
		in.Codec = string(frma.body[:4])
	}
	if schm, ok := find(sc, "schm"); ok {
		r := reader{b: schm.body}
		r.full()
		if p := r.take(4); p != nil {
			in.Scheme = string(p)
		}
	}
	tenc, err := path(sc, "schi", "tenc")
	if err != nil {
		return err
	}
	r := reader{b: tenc.body}
	v, _ := r.full()
	r.u8()
	pattern := r.u8()
	if v >= 1 {
		in.CryptByte, in.SkipByte = pattern>>4, pattern&0x0f
	}
	in.Protected = r.u8() != 0
	in.IVSize = int(r.u8())
	in.KID = r.take(16)
	if in.Protected && in.IVSize == 0 {
		n := int(r.u8())
		in.ConstantIV = r.take(n)
	}
	if r.err != nil {
		return fmt.Errorf("fmp4: tenc: %w", r.err)
	}
	if in.IVSize != 0 && in.IVSize != 8 && in.IVSize != 16 {
		return fmt.Errorf("fmp4: tenc: IV size %d", in.IVSize)
	}
	return nil
}

// parseESDS extracts the AudioSpecificConfig from an esds FullBox.
func (in *Init) parseESDS(b []byte) error {
	r := reader{b: b}
	r.full()
	tag, n := descriptor(&r)
	if tag != 0x03 {
		return fmt.Errorf("fmp4: esds: ES_Descriptor tag %#x", tag)
	}
	es := reader{b: r.take(n)}
	es.u16() // ES_ID
	flags := es.u8()
	if flags&0x80 != 0 {
		es.u16()
	}
	if flags&0x40 != 0 {
		es.take(int(es.u8()))
	}
	if flags&0x20 != 0 {
		es.u16()
	}
	tag, n = descriptor(&es)
	if tag != 0x04 {
		return fmt.Errorf("fmp4: esds: DecoderConfigDescriptor tag %#x", tag)
	}
	dc := reader{b: es.take(n)}
	in.ObjectType = dc.u8()
	dc.take(12) // streamType, bufferSizeDB, maxBitrate, avgBitrate
	tag, n = descriptor(&dc)
	if tag != 0x05 {
		return fmt.Errorf("fmp4: esds: DecoderSpecificInfo tag %#x", tag)
	}
	in.AudioConfig = dc.take(n)
	for _, err := range []error{r.err, es.err, dc.err} {
		if err != nil {
			return fmt.Errorf("fmp4: esds: %w", err)
		}
	}
	return in.parseASC()
}

// descriptor reads an ISO 14496-1 expandable descriptor header.
func descriptor(r *reader) (tag uint8, size int) {
	tag = r.u8()
	for range 4 {
		b := r.u8()
		size = size<<7 | int(b&0x7f)
		if b&0x80 == 0 {
			break
		}
	}
	return tag, size
}

var aacRates = [...]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

// parseASC reads object type, sample rate and channel configuration from
// the AudioSpecificConfig (ISO 14496-3 1.6.2.1).
func (in *Init) parseASC() error {
	bits := bitReader{b: in.AudioConfig}
	aot := bits.read(5)
	if aot == 31 {
		aot = 32 + bits.read(6)
	}
	rate := 0
	if idx := bits.read(4); idx == 15 {
		rate = bits.read(24)
	} else if idx < len(aacRates) {
		rate = aacRates[idx]
	}
	ch := bits.read(4)
	if bits.err {
		return errors.New("fmp4: AudioSpecificConfig truncated")
	}
	in.AudioObject = aot
	if rate != 0 {
		in.SampleRate = rate
	}
	if ch != 0 {
		in.Channels = ch
	}
	return nil
}

type bitReader struct {
	b   []byte
	pos int
	err bool
}

func (r *bitReader) read(n int) int {
	v := 0
	for range n {
		if r.pos/8 >= len(r.b) {
			r.err = true
			return 0
		}
		v = v<<1 | int(r.b[r.pos/8]>>(7-r.pos%8)&1)
		r.pos++
	}
	return v
}
