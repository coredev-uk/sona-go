package fmp4

import (
	"bytes"
	"strings"
)

// Loudness is a track's loudness as measured by Apple (ISO/IEC 14496-12
// 'tlou'/'alou' LoudnessBox entries in moov/trak/udta/ludt, values coded
// as in ISO/IEC 23003-4).
type Loudness struct {
	Track    float64 // integrated programme loudness of the track, LUFS
	Album    float64 // of its album, LUFS; HasAlbum reports whether set
	HasAlbum bool
	// TruePeak is the track's true peak in dBTP (falling back to its
	// sample peak in dBFS); HasPeak reports whether either was given.
	TruePeak float64
	HasPeak  bool
}

// LoudnessOf reads the loudness Apple stores in an init segment (or any
// MP4 movie box). ok is false when the file carries none.
func LoudnessOf(init []byte) (l Loudness, ok bool) {
	top, err := boxes(init, 0)
	if err != nil {
		return l, false
	}
	moov, found := find(top, "moov")
	if !found {
		return l, false
	}
	kids, err := children(moov)
	if err != nil {
		return l, false
	}
	for _, trak := range kids {
		if trak.typ != "trak" {
			continue
		}
		ludt, found := within(trak, "udta", "ludt")
		if !found {
			continue
		}
		entries, err := children(ludt)
		if err != nil {
			continue
		}
		for _, e := range entries {
			v, base, valid := loudnessBase(e.body)
			if !valid {
				continue
			}
			switch e.typ {
			case "tlou":
				l.Track, ok = v, true
				if base.hasPeak {
					l.TruePeak, l.HasPeak = base.peak, true
				}
			case "alou":
				l.Album, l.HasAlbum = v, true
			}
		}
		if ok {
			return l, true
		}
	}
	return l, false
}

// within descends from b through nested boxes.
func within(b box, typs ...string) (box, bool) {
	for _, t := range typs {
		kids, err := children(b)
		if err != nil {
			return box{}, false
		}
		var found bool
		if b, found = find(kids, t); !found {
			return box{}, false
		}
	}
	return b, true
}

type loudnessPeak struct {
	peak    float64
	hasPeak bool
}

// loudnessBase decodes the first loudness set without EQ, downmix or DRC
// applied from a LoudnessBaseBox body and returns its programme loudness
// (measurement method 1).
func loudnessBase(body []byte) (lufs float64, p loudnessPeak, ok bool) {
	r := reader{b: body}
	version, _ := r.full()
	count := 1
	if version >= 1 {
		count = int(r.u8() & 0x3f)
	}
	for range count {
		if version >= 1 {
			r.u8() // EQ_set_ID
		}
		ids := r.u16() // reserved(3) downmix_ID(7) DRC_set_ID(6)
		peaks := r.u32()
		n := int(r.u8())
		sample, truePeak := (peaks>>20)&0xfff, (peaks>>8)&0xfff
		var prog float64
		var have bool
		for range n {
			def, val := r.u8(), r.u8()
			r.u8() // measurement_system(4) reliability(4)
			if def == 1 {
				prog, have = -57.75+float64(val)/4, true
			}
		}
		if r.err != nil {
			return 0, p, false
		}
		if ids&0x1fff != 0 || !have || ok {
			continue // a downmix/DRC variant, or not the first plain set
		}
		lufs, ok = prog, true
		switch {
		case truePeak != 0:
			p = loudnessPeak{20 - float64(truePeak)/32, true}
		case sample != 0:
			p = loudnessPeak{20 - float64(sample)/32, true}
		}
	}
	return lufs, p, ok
}

// ITunNORM returns the iTunes Sound Check tag of an MP4 file (moov/udta/
// meta/ilst '----' atom com.apple.iTunes;iTunNORM), if the file's movie box
// lies within b.
func ITunNORM(b []byte) (string, bool) {
	top, err := boxes(prefixBoxes(b), 0)
	if err != nil {
		return "", false
	}
	moov, found := find(top, "moov")
	if !found {
		return "", false
	}
	udta, found := within(moov, "udta")
	if !found {
		return "", false
	}
	kids, err := children(udta)
	if err != nil {
		return "", false
	}
	meta, found := find(kids, "meta")
	if !found {
		return "", false
	}
	// meta is a FullBox in ISO files; QuickTime writes it without the
	// version and flags.
	body := meta.body
	if len(body) >= 8 && string(body[4:8]) != "hdlr" {
		body = body[4:]
	}
	items, err := boxes(body, 0)
	if err != nil {
		return "", false
	}
	ilst, found := find(items, "ilst")
	if !found {
		return "", false
	}
	atoms, err := children(ilst)
	if err != nil {
		return "", false
	}
	for _, a := range atoms {
		if a.typ != "----" {
			continue
		}
		parts, err := children(a)
		if err != nil {
			continue
		}
		var name, data string
		for _, p := range parts {
			if len(p.body) < 4 {
				continue
			}
			switch p.typ {
			case "name":
				name = string(p.body[4:])
			case "data":
				if len(p.body) >= 8 {
					data = string(p.body[8:])
				}
			}
		}
		if name == "iTunNORM" {
			return strings.TrimSpace(data), true
		}
	}
	return "", false
}

// prefixBoxes cuts b after its last complete top-level box, so a file
// downloaded only in part can be searched for a leading movie box.
func prefixBoxes(b []byte) []byte {
	end := 0
	for off := 0; len(b)-off >= 8; {
		size := uint64(b[off])<<24 | uint64(b[off+1])<<16 | uint64(b[off+2])<<8 | uint64(b[off+3])
		if size == 1 && len(b)-off >= 16 {
			size = 0
			for _, c := range b[off+8 : off+16] {
				size = size<<8 | uint64(c)
			}
		}
		if size < 8 || size > uint64(len(b)-off) {
			break
		}
		off += int(size)
		end = off
		if bytes.Equal(b[off-int(size)+4:off-int(size)+8], []byte("moov")) {
			break
		}
	}
	return b[:end]
}
