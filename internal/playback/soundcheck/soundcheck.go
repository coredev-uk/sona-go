// Package soundcheck turns the loudness a track carries into the gain
// that plays every track at about the same loudness (Apple's Sound Check).
//
// Catalog tracks: Apple measures every track (BS.1770 integrated loudness
// and true peak) and stores it in the fMP4 init segment's 'tlou' box
// (fmp4.LoudnessOf). Neither webPlayback, the HLS playlists nor MusicKit
// JS carry or apply it; see docs/apple-music-reference.md.
//
// Library uploads are the user's own files as uploaded: their gain comes
// from the iTunes Sound Check tag (iTunNORM) iTunes/Music wrote into them,
// in an MP4 'ilst' atom or an MP3 ID3v2 comment; files without one play at
// unity gain.
package soundcheck

import (
	"bytes"
	"encoding/binary"
	"math"
	"strconv"
	"strings"

	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
)

// Target is the loudness tracks are brought to, in LUFS (the level Apple
// Music normalises to).
const Target = -16.0

// Ceiling is the highest true peak a positive gain may raise a track to,
// in dBTP, so normalisation never clips.
const Ceiling = -1.0

// maxBoost and maxCut bound any gain, in dB.
const (
	maxBoost = 12.0
	maxCut   = -24.0
)

// FromLoudness returns the linear gain for a track measured at l.
func FromLoudness(l fmp4.Loudness) float64 {
	g := Target - l.Track
	if g > 0 && l.HasPeak {
		g = min(g, max(Ceiling-l.TruePeak, 0))
	}
	return linear(g)
}

// FromITunNORM returns the linear gain an iTunNORM tag asks for, and
// whether the tag parsed. Its first two words are 1000·10^(−gain/10) for
// the left and right channel (the louder one wins). Words 7 and 8 are the
// channels' peak sample (16-bit full scale is 32768): a positive gain is
// capped so the peak stays at Ceiling, like FromLoudness's true peak.
func FromITunNORM(tag string) (float64, bool) {
	words := strings.Fields(tag)
	if len(words) < 2 {
		return 0, false
	}
	hex := func(w string) (uint64, bool) {
		v, err := strconv.ParseUint(w, 16, 32)
		return v, err == nil
	}
	var loud uint64
	for _, w := range words[:2] {
		v, ok := hex(w)
		if !ok {
			return 0, false
		}
		loud = max(loud, v)
	}
	if loud == 0 {
		return 0, false
	}
	g := -10 * math.Log10(float64(loud)/1000)
	if len(words) >= 8 && g > 0 {
		l, okL := hex(words[6])
		r, okR := hex(words[7])
		if peak := max(l, r); okL && okR && peak > 0 {
			g = min(g, max(Ceiling-20*math.Log10(float64(peak)/32768), 0))
		}
	}
	return linear(g), true
}

// FromFile returns the gain of an uploaded file from its leading bytes
// (MP4 with its movie box first, or MP3 with an ID3v2 tag), and whether a
// Sound Check tag was found.
func FromFile(head []byte) (float64, bool) {
	if tag, ok := fmp4.ITunNORM(head); ok {
		return FromITunNORM(tag)
	}
	if tag, ok := id3ITunNORM(head); ok {
		return FromITunNORM(tag)
	}
	return 0, false
}

func linear(db float64) float64 {
	return math.Pow(10, min(max(db, maxCut), maxBoost)/20)
}

// id3ITunNORM finds the iTunNORM comment in an ID3v2.2–2.4 tag at the
// start of b ("COM"/"COMM" frame with description "iTunNORM").
func id3ITunNORM(b []byte) (string, bool) {
	if len(b) < 10 || string(b[:3]) != "ID3" {
		return "", false
	}
	major, flags := b[3], b[5]
	size := syncsafe(b[6:10])
	if flags&0x80 != 0 || 10+size > len(b) { // unsynchronised, or cut off
		return "", false
	}
	tag := b[10 : 10+size]
	if flags&0x40 != 0 && major >= 3 && len(tag) >= 4 { // skip the extended header
		ext := int(binary.BigEndian.Uint32(tag))
		if major == 4 {
			ext = syncsafe(tag[:4])
		} else {
			ext += 4
		}
		if ext > len(tag) {
			return "", false
		}
		tag = tag[ext:]
	}
	idLen, hdr := 4, 10
	if major == 2 {
		idLen, hdr = 3, 6
	}
	for len(tag) >= hdr && tag[0] != 0 {
		id := string(tag[:idLen])
		var n int
		switch major {
		case 2:
			n = int(tag[3])<<16 | int(tag[4])<<8 | int(tag[5])
		case 3:
			n = int(binary.BigEndian.Uint32(tag[4:8]))
		default:
			n = syncsafe(tag[4:8])
		}
		if n < 0 || hdr+n > len(tag) {
			return "", false
		}
		body := tag[hdr : hdr+n]
		tag = tag[hdr+n:]
		if id != "COM" && id != "COMM" {
			continue
		}
		if desc, text, ok := comment(body); ok && desc == "iTunNORM" {
			return strings.TrimSpace(text), true
		}
	}
	return "", false
}

// comment splits a comment frame body (encoding, language, description,
// text) into its description and text.
func comment(body []byte) (desc, text string, ok bool) {
	if len(body) < 4 {
		return "", "", false
	}
	enc, rest := body[0], body[4:]
	sep, width := []byte{0}, 1
	if enc == 1 || enc == 2 {
		sep, width = []byte{0, 0}, 2
	}
	i := 0
	for {
		j := bytes.Index(rest[i:], sep)
		if j < 0 {
			return "", "", false
		}
		if (i+j)%width == 0 {
			i += j
			break
		}
		i += j + 1
	}
	return decode(enc, rest[:i]), decode(enc, rest[i+width:]), true
}

// decode returns ID3 text as Go text; the tags this package reads are
// ASCII, so UTF-16 is narrowed by dropping the high bytes.
func decode(enc byte, b []byte) string {
	if enc != 1 && enc != 2 {
		return strings.TrimRight(string(b), "\x00")
	}
	if len(b) >= 2 && (b[0] == 0xff && b[1] == 0xfe || b[0] == 0xfe && b[1] == 0xff) {
		b = b[2:]
	}
	var sb strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		c := b[i]
		if c == 0 {
			c = b[i+1]
		}
		if c != 0 {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

func syncsafe(b []byte) int {
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}
