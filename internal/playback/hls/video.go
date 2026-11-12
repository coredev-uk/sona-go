package hls

import (
	"fmt"
	"strings"
)

// Music videos: Apple's master playlists list the video variants
// (EXT-X-STREAM-INF, each naming an AUDIO group) and the audio renditions
// (EXT-X-MEDIA) separately; a player fetches one of each.

// Rendition is one EXT-X-MEDIA of a master playlist.
type Rendition struct {
	Type     string // "AUDIO", "SUBTITLES", "CLOSED-CAPTIONS", "VIDEO"
	GroupID  string // e.g. "audio-stereo-256"
	Name     string
	Language string
	URI      string // "" for one muxed into the variant
	Default  bool
}

// ParseRenditions parses a master playlist's EXT-X-MEDIA renditions in
// order (none is not an error).
func ParseRenditions(data []byte) ([]Rendition, error) {
	sc, err := playlistScanner(data)
	if err != nil {
		return nil, err
	}
	var out []Rendition
	for sc.Scan() {
		value, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "#EXT-X-MEDIA:")
		if !ok {
			continue
		}
		attrs, err := ParseAttributes(value)
		if err != nil {
			return nil, fmt.Errorf("hls: EXT-X-MEDIA: %w", err)
		}
		out = append(out, Rendition{
			Type: attrs["TYPE"], GroupID: attrs["GROUP-ID"], Name: attrs["NAME"],
			Language: attrs["LANGUAGE"], URI: attrs["URI"], Default: attrs["DEFAULT"] == "YES",
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("hls: %w", err)
	}
	return out, nil
}

// SelectVideo returns the video variant a software Widevine CDM plays
// best: of the H.264 ("avc1") variants it may play (SoftwareWidevine)
// that need no HDCP (HDCP-LEVEL NONE or absent), the one with the highest
// bandwidth; ties keep playlist order. HEVC variants never qualify: the
// CDM's own video decoder, the only one that may use video keys
// (SW_SECURE_DECODE), takes H.264, VP8, VP9 and AV1 only. It reports
// false when no variant qualifies.
func SelectVideo(vs []Variant) (Variant, bool) {
	best := -1
	for i, v := range vs {
		if v.Width == 0 || !v.SoftwareWidevine() || (v.HDCPLevel != "" && v.HDCPLevel != "NONE") {
			continue
		}
		if !strings.Contains(v.Codecs, "avc1") {
			continue
		}
		if best < 0 || v.Bandwidth > vs[best].Bandwidth {
			best = i
		}
	}
	if best < 0 {
		return Variant{}, false
	}
	return vs[best], true
}

// stereoAAC256 is the group of Apple's 256 kbps stereo AAC-LC renditions.
const stereoAAC256 = "audio-stereo-256"

// SelectAudio returns the audio rendition to play with video variant v:
// the 256 kbps stereo AAC-LC one ("audio-stereo-256") when the master
// lists it, else one of v's AUDIO group; within a group the DEFAULT one,
// else the first. Only renditions with a URI count. It reports false when
// none qualifies.
func SelectAudio(rs []Rendition, v Variant) (Rendition, bool) {
	for _, group := range []string{stereoAAC256, v.Audio} {
		if group == "" {
			continue
		}
		var pick *Rendition
		for i := range rs {
			r := &rs[i]
			if r.Type != "AUDIO" || r.GroupID != group || r.URI == "" {
				continue
			}
			if pick == nil || (r.Default && !pick.Default) {
				pick = r
			}
		}
		if pick != nil {
			return *pick, true
		}
	}
	return Rendition{}, false
}
