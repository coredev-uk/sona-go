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

// choose picks the H.264 SDR variant for a cover drawn minWidth device
// pixels wide: the smallest resolution at least that wide (the largest
// when none is), and at that resolution the highest bit rate up to
// maxBandwidth (the lowest when all exceed it). HEVC renditions are
// skipped: WebKitGTK's GStreamer cannot be assumed to decode them.
func choose(vs []variant, minWidth int) (variant, bool) {
	var avc []variant
	for _, v := range vs {
		if v.SDR && v.Width > 0 && strings.HasPrefix(v.Codecs, "avc1") {
			avc = append(avc, v)
		}
	}
	if len(avc) == 0 {
		return variant{}, false
	}
	width := 0
	for _, v := range avc {
		if v.Width >= minWidth && (width == 0 || v.Width < width) {
			width = v.Width
		}
	}
	if width == 0 {
		for _, v := range avc {
			width = max(width, v.Width)
		}
	}
	var best variant
	for _, v := range avc {
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

var errMultiFile = errors.New("media playlist spans several files")

// parseMedia returns the one file a byte-range media playlist reads from
// (its #EXT-X-MAP initialisation section and every segment), resolved
// against base. Apple's motion renditions are single fragmented MP4 files;
// a playlist of separate segment files is not supported.
func parseMedia(body string, base *url.URL) (*url.URL, error) {
	file := ""
	use := func(uri string) error {
		if file == "" {
			file = uri
		} else if uri != file {
			return errMultiFile
		}
		return nil
	}
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
			if err := use(attributes(strings.TrimPrefix(line, "#EXT-X-MAP:"))["URI"]); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "#"):
		default:
			if err := use(line); err != nil {
				return nil, err
			}
		}
	}
	if file == "" {
		return nil, errors.New("media playlist has no segments")
	}
	return base.Parse(file)
}
