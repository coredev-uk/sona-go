//go:build (linux || darwin) && cgo

package media

import "testing"

func TestCodecPrefersAVCOverHEVCAndFallsBackToHEVCOnly(t *testing.T) {
	base := []string{"qtdemux", "appsink", "decodebin", "videoconvert"}
	set := func(names ...string) func(string) bool {
		m := map[string]bool{}
		for _, n := range append(append([]string{}, base...), names...) {
			m[n] = true
		}
		return func(f string) bool { return m[f] }
	}
	avc, hevc := decoders["avc"][0], decoders["hevc"][0]
	for _, c := range []struct {
		name string
		has  func(string) bool
		want string
	}{
		{"both decode: H.264, the cheapest", set("h264parse", avc, "h265parse", hevc), "avc"},
		{"only HEVC decodes", set("h265parse", hevc), "hevc"},
		{"H.264 without its parser falls to HEVC", set(avc, "h265parse", hevc), "hevc"},
		{"a parser alone decodes nothing", set("h264parse", "h265parse"), ""},
		{"no MP4 demuxer: no motion at all", func(f string) bool {
			return f != "qtdemux" && (f == "h264parse" || f == avc || f == "appsink" || f == "decodebin" || f == "videoconvert")
		}, ""},
	} {
		if got := pickCodec(c.has); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
