package motion

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// variant is one video rendition of an HLS master playlist.
type variant struct {
	URI       string
	Bandwidth int
	Codecs    string
	Width     int
	Height    int
	SDR       bool
}

// parseMaster returns the video variants of an HLS master playlist (the
// #EXT-X-STREAM-INF entries; I-frame playlists are skipped).
func parseMaster(body string) []variant {
	var out []variant
	var pending *variant
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			a := attributes(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"))
			v := variant{Codecs: a["CODECS"], SDR: a["VIDEO-RANGE"] == "" || a["VIDEO-RANGE"] == "SDR"}
			v.Bandwidth, _ = strconv.Atoi(a["BANDWIDTH"])
			if w, h, ok := strings.Cut(a["RESOLUTION"], "x"); ok {
				v.Width, _ = strconv.Atoi(w)
				v.Height, _ = strconv.Atoi(h)
			}
			pending = &v
		case strings.HasPrefix(line, "#"):
		case pending != nil:
			pending.URI = line
			out = append(out, *pending)
			pending = nil
		}
	}
	return out
}

// attributes parses an HLS attribute list (KEY=value,KEY="quoted, value").
func attributes(s string) map[string]string {
	out := map[string]string{}
	for s != "" {
		key, rest, ok := strings.Cut(s, "=")
		if !ok {
			break
		}
		var val string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				break
			}
			val, rest = rest[1:1+end], rest[2+end:]
		} else {
			val, rest, _ = strings.Cut(rest, ",")
			rest = "," + rest
		}
		out[strings.TrimSpace(key)] = val
		s = strings.TrimPrefix(rest, ",")
	}
	return out
}

// maxBandwidth caps the bit rate chosen at one resolution: motion artwork
// is a muted loop behind a cover, not a film.
const maxBandwidth = 4_000_000

// Codec families a rendition is requested in. Apple's motion artwork has
// H.264 and HEVC renditions of every size; H.264 is the one WebKitGTK's
// GStreamer decodes almost everywhere (gst-libav), so HEVC is asked for
// only where the web view can play HEVC but not H.264.
const (
	CodecH264 = "avc"
	CodecHEVC = "hevc"
)

// family returns the codec family of an HLS CODECS attribute (the video
// codec among, e.g., "avc1.64001f,mp4a.40.2"), "" for any other.
func family(codecs string) string {
	for c := range strings.SplitSeq(codecs, ",") {
		c = strings.TrimSpace(c)
		switch {
		case strings.HasPrefix(c, "avc1."), strings.HasPrefix(c, "avc3."):
			return CodecH264
		case strings.HasPrefix(c, "hvc1."), strings.HasPrefix(c, "hev1."):
			return CodecHEVC
		}
	}
	return ""
}

// choose picks the SDR variant of codec family fam for a cover drawn
// minWidth device pixels wide: the smallest resolution at least that wide
// (the largest when none is), and at that resolution the highest bit rate
// up to maxBandwidth (the lowest when all exceed it).
func choose(vs []variant, minWidth int, fam string) (variant, bool) {
	var ok []variant
	for _, v := range vs {
		if v.SDR && v.Width > 0 && family(v.Codecs) == fam {
			ok = append(ok, v)
		}
	}
	if len(ok) == 0 {
		return variant{}, false
	}
	width := 0
	for _, v := range ok {
		if v.Width >= minWidth && (width == 0 || v.Width < width) {
			width = v.Width
		}
	}
	if width == 0 {
		for _, v := range ok {
			width = max(width, v.Width)
		}
	}
	var best variant
	for _, v := range ok {
		if v.Width != width {
			continue
		}
		switch {
		case best.URI == "":
			best = v
		case best.Bandwidth > maxBandwidth:
			if v.Bandwidth < best.Bandwidth {
				best = v
			}
		case v.Bandwidth <= maxBandwidth && v.Bandwidth > best.Bandwidth:
			best = v
		}
	}
	return best, true
}

// describe names a variant for logs and Diagnostics: "avc1.64001f
// 486×486, 1.7 Mbit/s".
func (v variant) describe() string {
	return fmt.Sprintf("%s %d×%d, %.1f Mbit/s", v.Codecs, v.Width, v.Height, float64(v.Bandwidth)/1e6)
}

// parseMedia returns the files a media playlist reads, in order: its
// #EXT-X-MAP initialisation section first, then each segment file once.
// Apple's album motion is one fragmented MP4 read by byte ranges (one
// file); editorial playlists' is an initialisation file plus .m4s
// segment files. Byte ranges are not needed: every file is fetched whole.
func parseMedia(body string, base *url.URL) ([]*url.URL, error) {
	var files []string
	seen := map[string]bool{}
	use := func(uri string) {
		if uri != "" && !seen[uri] {
			seen[uri] = true
			files = append(files, uri)
		}
	}
	maps := 0
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXT-X-KEY:"):
			if a := attributes(strings.TrimPrefix(line, "#EXT-X-KEY:")); a["METHOD"] != "NONE" {
				return nil, fmt.Errorf("encrypted media playlist (%s)", a["METHOD"])
			}
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			if maps++; maps > 1 {
				return nil, errSeveralInits
			}
			uri := attributes(strings.TrimPrefix(line, "#EXT-X-MAP:"))["URI"]
			if len(files) > 0 && !seen[uri] {
				// The initialisation section must come before the frames.
				return nil, errSeveralInits
			}
			use(uri)
		case strings.HasPrefix(line, "#"):
		default:
			use(line)
		}
	}
	if len(files) == 0 {
		return nil, errors.New("media playlist has no segments")
	}
	out := make([]*url.URL, 0, len(files))
	for _, f := range files {
		u, err := base.Parse(f)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

var errSeveralInits = errors.New("media playlist changes its initialisation section")
