// Package hls parses the HLS playlists Apple serves for Widevine streams:
// songs (VOD, fMP4 with EXT-X-MAP, byte-range segments of one resource and
// an ISO-23001-7 key with the key ID inline as a data URI) and radio (live
// sliding windows and episodes, SAMPLE-AES keys in several KEYFORMATs,
// master playlists of AAC variants), and the clear live playlists of local
// broadcasters (MPEG-TS or packed audio segments, EXTINF titles).
package hls

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ByteRange is a sub-range of a resource. Length 0 means the whole resource.
type ByteRange struct {
	Offset, Length int64
}

// Header returns the HTTP Range header value for r, or "" for a whole resource.
func (r ByteRange) Header() string {
	if r.Length <= 0 {
		return ""
	}
	return fmt.Sprintf("bytes=%d-%d", r.Offset, r.Offset+r.Length-1)
}

// Map is an EXT-X-MAP media initialisation section.
type Map struct {
	URI   string
	Range ByteRange
}

// WidevineKeyFormat is the KEYFORMAT of Widevine EXT-X-KEYs.
const WidevineKeyFormat = "urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"

// Key is an EXT-X-KEY.
type Key struct {
	Method            string // "ISO-23001-7" (songs) or "SAMPLE-AES" (radio)
	URI               string
	KeyFormat         string
	KeyFormatVersions string
	// InitData is the payload of a base64 data URI: a 16-byte key ID
	// (songs) or a whole Widevine pssh box (radio); nil otherwise.
	InitData []byte
	// KID is InitData when it is a 16-byte key ID, otherwise nil.
	KID []byte
}

// Segment is one media segment.
type Segment struct {
	URI      string
	Range    ByteRange
	Start    time.Duration // sum of the preceding EXTINF durations
	Duration time.Duration
	Seq      int  // media sequence number
	Key      *Key // nil when unencrypted; the Widevine key when several apply
	Map      *Map
	// Discontinuity: EXT-X-DISCONTINUITY precedes the segment.
	Discontinuity bool
	// Title is the EXTINF title, the text after the duration's comma
	// (broadcasters put attributes there: title="…",artist="…").
	Title string
}

// Playlist is a parsed media playlist.
type Playlist struct {
	Version        int
	TargetDuration time.Duration
	MediaSequence  int
	Ended          bool // EXT-X-ENDLIST
	Segments       []Segment
}

// Duration is the sum of all segment durations.
func (p *Playlist) Duration() time.Duration {
	if len(p.Segments) == 0 {
		return 0
	}
	last := p.Segments[len(p.Segments)-1]
	return last.Start + last.Duration
}

// SegmentAt returns the index of the segment containing t, clamped to the
// first and last segment. It returns -1 for an empty playlist.
func (p *Playlist) SegmentAt(t time.Duration) int {
	n := len(p.Segments)
	if n == 0 {
		return -1
	}
	lo, hi := 0, n-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if p.Segments[mid].Start <= t {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// Parse parses a media playlist. Segment byte ranges without an offset
// continue where the previous segment of the same URI ended; the first such
// segment continues after an EXT-X-MAP range of the same URI. When several
// EXT-X-KEYs (one per KEYFORMAT) apply, a segment's Key is the Widevine
// one, else the one set last.
func Parse(data []byte) (*Playlist, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !sc.Scan() || strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff")) != "#EXTM3U" {
		return nil, errors.New("hls: missing #EXTM3U")
	}
	p := &Playlist{}
	var (
		keys    = map[string]*Key{} // by KEYFORMAT
		key     *Key                // the last one set
		disc    bool
		mp      *Map
		pending Segment
		haveInf bool
		haveRng bool
		rngLen  int64
		rngOff  int64 = -1
		start   time.Duration
		lastURI string
		lastEnd int64 = -1
		lineNo        = 1
	)
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		tag, value, _ := strings.Cut(line, ":")
		var err error
		switch {
		case !strings.HasPrefix(line, "#"):
			if !haveInf {
				return nil, fmt.Errorf("hls: line %d: segment %q without #EXTINF", lineNo, line)
			}
			pending.URI = line
			pending.Key, pending.Map = key, mp
			if wv := keys[WidevineKeyFormat]; wv != nil {
				pending.Key = wv
			}
			pending.Seq = p.MediaSequence + len(p.Segments)
			pending.Discontinuity, disc = disc, false
			pending.Start = start
			if haveRng {
				off := rngOff
				if off < 0 {
					switch {
					case lastURI == line && lastEnd >= 0:
						off = lastEnd
					case lastURI == "" && mp != nil && mp.URI == line && mp.Range.Length > 0:
						off = mp.Range.Offset + mp.Range.Length
					default:
						return nil, fmt.Errorf("hls: line %d: byte range without offset has no preceding range of %q", lineNo, line)
					}
				}
				pending.Range = ByteRange{Offset: off, Length: rngLen}
				lastEnd = off + rngLen
			} else {
				lastEnd = -1
			}
			lastURI = line
			p.Segments = append(p.Segments, pending)
			start += pending.Duration
			pending, haveInf, haveRng, rngOff = Segment{}, false, false, -1
		case tag == "#EXTINF":
			d, title, _ := strings.Cut(value, ",")
			pending.Title = strings.TrimSpace(title)
			pending.Duration, err = seconds(d)
			haveInf = true
		case tag == "#EXT-X-BYTERANGE":
			rngLen, rngOff, err = parseRange(value)
			haveRng = true
		case tag == "#EXT-X-KEY":
			if key, err = parseKey(value); err == nil {
				if key == nil {
					clear(keys)
				} else {
					keys[key.KeyFormat] = key
				}
			}
		case tag == "#EXT-X-DISCONTINUITY":
			disc = true
		case tag == "#EXT-X-MAP":
			mp, err = parseMap(value)
		case tag == "#EXT-X-VERSION":
			p.Version, err = strconv.Atoi(value)
		case tag == "#EXT-X-TARGETDURATION":
			p.TargetDuration, err = seconds(value)
		case tag == "#EXT-X-MEDIA-SEQUENCE":
			p.MediaSequence, err = strconv.Atoi(value)
		case tag == "#EXT-X-ENDLIST":
			p.Ended = true
		case tag == "#EXT-X-STREAM-INF":
			return nil, errors.New("hls: master playlist where a media playlist was expected")
		}
		if err != nil {
			return nil, fmt.Errorf("hls: line %d: %s: %w", lineNo, tag, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("hls: %w", err)
	}
	if len(p.Segments) == 0 {
		return nil, errors.New("hls: no segments")
	}
	return p, nil
}

// Resolve resolves ref against the playlist URL base.
func Resolve(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(r).String(), nil
}

func seconds(s string) (time.Duration, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	if f < 0 {
		return 0, fmt.Errorf("negative duration %v", f)
	}
	return time.Duration(math.Round(f * float64(time.Second))), nil
}

// parseRange parses "n[@o]"; off is -1 when o is absent.
func parseRange(s string) (length, off int64, err error) {
	l, o, hasOff := strings.Cut(strings.TrimSpace(s), "@")
	if length, err = strconv.ParseInt(l, 10, 64); err != nil {
		return 0, 0, err
	}
	if length <= 0 {
		return 0, 0, fmt.Errorf("byte range length %d", length)
	}
	off = -1
	if hasOff {
		if off, err = strconv.ParseInt(o, 10, 64); err != nil {
			return 0, 0, err
		}
		if off < 0 {
			return 0, 0, fmt.Errorf("byte range offset %d", off)
		}
	}
	return length, off, nil
}

func parseMap(s string) (*Map, error) {
	attrs, err := ParseAttributes(s)
	if err != nil {
		return nil, err
	}
	m := &Map{URI: attrs["URI"]}
	if m.URI == "" {
		return nil, errors.New("missing URI")
	}
	if r, ok := attrs["BYTERANGE"]; ok {
		l, o, err := parseRange(r)
		if err != nil {
			return nil, err
		}
		m.Range = ByteRange{Offset: max(o, 0), Length: l}
	}
	return m, nil
}

func parseKey(s string) (*Key, error) {
	attrs, err := ParseAttributes(s)
	if err != nil {
		return nil, err
	}
	k := &Key{
		Method:            attrs["METHOD"],
		URI:               attrs["URI"],
		KeyFormat:         attrs["KEYFORMAT"],
		KeyFormatVersions: attrs["KEYFORMATVERSIONS"],
	}
	if k.Method == "NONE" {
		return nil, nil
	}
	if k.Method == "" {
		return nil, errors.New("missing METHOD")
	}
	if data, ok := strings.CutPrefix(k.URI, "data:"); ok {
		if _, b64, ok := strings.Cut(data, ";base64,"); ok {
			b, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return nil, fmt.Errorf("key data URI: %w", err)
			}
			k.InitData = b
			switch {
			case len(b) == 16:
				k.KID = b
			case k.KeyFormat == WidevineKeyFormat:
				k.KID = psshKeyID(b)
			}
		}
	}
	return k, nil
}

// psshKeyID returns the first key ID of a Widevine 'pssh' box (from the
// box's KID list, version 1, else WidevinePsshData.key_id, field 2), or nil.
func psshKeyID(b []byte) []byte {
	if len(b) < 32 || string(b[4:8]) != "pssh" {
		return nil
	}
	off := 28
	if b[8] >= 1 { // version 1: KID_count, KIDs
		n := int(binary.BigEndian.Uint32(b[off:]))
		if n > 0 && len(b) >= off+4+16 {
			return b[off+4 : off+20]
		}
		off += 4 + 16*n
	}
	if len(b) < off+4 {
		return nil
	}
	data := b[off+4:]
	if size := int(binary.BigEndian.Uint32(b[off:])); size < len(data) {
		data = data[:size]
	}
	for len(data) > 0 {
		tag, n := binary.Uvarint(data)
		if n <= 0 {
			return nil
		}
		data = data[n:]
		switch tag & 7 {
		case 0:
			if _, n = binary.Uvarint(data); n <= 0 {
				return nil
			}
			data = data[n:]
		case 2:
			l, n := binary.Uvarint(data)
			if n <= 0 || uint64(len(data)-n) < l {
				return nil
			}
			v := data[n : n+int(l)]
			if tag>>3 == 2 && len(v) == 16 {
				return v
			}
			data = data[n+int(l):]
		default:
			return nil
		}
	}
	return nil
}

// Variant is one EXT-X-STREAM-INF of a master playlist.
type Variant struct {
	URI       string
	Bandwidth int
	Codecs    string
	Audio     string // AUDIO rendition group, e.g. "audio-alac-stereo-44100-16"
}

// IsMaster reports whether data is a master playlist.
func IsMaster(data []byte) bool { return bytes.Contains(data, []byte("#EXT-X-STREAM-INF:")) }

// ParseMaster parses a master playlist's variants in order.
func ParseMaster(data []byte) ([]Variant, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !sc.Scan() || strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff")) != "#EXTM3U" {
		return nil, errors.New("hls: missing #EXTM3U")
	}
	var (
		out     []Variant
		pending *Variant
	)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			attrs, err := ParseAttributes(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"))
			if err != nil {
				return nil, fmt.Errorf("hls: EXT-X-STREAM-INF: %w", err)
			}
			bw, _ := strconv.Atoi(attrs["BANDWIDTH"])
			pending = &Variant{Bandwidth: bw, Codecs: attrs["CODECS"], Audio: attrs["AUDIO"]}
		case strings.HasPrefix(line, "#"):
		case pending != nil:
			pending.URI = line
			out = append(out, *pending)
			pending = nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("hls: %w", err)
	}
	if len(out) == 0 {
		return nil, errors.New("hls: no variants")
	}
	return out, nil
}

// BestAAC returns the variant to play: the highest-bandwidth AAC-LC
// ("mp4a.40.2") one, else the highest-bandwidth one. Ties keep playlist
// order (Apple lists its primary CDN first).
func BestAAC(vs []Variant) Variant {
	best := -1
	score := func(v Variant) int {
		s := v.Bandwidth
		if strings.Contains(v.Codecs, "mp4a.40.2") {
			s += 1 << 40
		}
		return s
	}
	for i, v := range vs {
		if best < 0 || score(v) > score(vs[best]) {
			best = i
		}
	}
	return vs[best]
}

// ParseAttributes parses an HLS attribute list (KEY=VALUE,KEY="quoted, value").
func ParseAttributes(s string) (map[string]string, error) {
	attrs := map[string]string{}
	for s = strings.TrimSpace(s); s != ""; {
		name, rest, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("attribute %q without value", s)
		}
		name = strings.TrimSpace(name)
		var val string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				return nil, fmt.Errorf("unterminated quoted value for %s", name)
			}
			val, rest = rest[1:1+end], rest[2+end:]
			rest = strings.TrimSpace(rest)
			if rest != "" && !strings.HasPrefix(rest, ",") {
				return nil, fmt.Errorf("junk after %s value", name)
			}
			rest = strings.TrimPrefix(rest, ",")
		} else {
			val, rest, _ = strings.Cut(rest, ",")
			val = strings.TrimSpace(val)
		}
		attrs[name] = val
		s = strings.TrimSpace(rest)
	}
	return attrs, nil
}
