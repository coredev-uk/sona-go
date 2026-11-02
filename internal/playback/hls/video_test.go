package hls

import (
	"testing"
)

// videoMaster has the shape of Apple's music video masters: HE-AAC and
// AAC-LC audio groups, I-frame variants, software Widevine variants up to
// 864×486 (H.264 and HEVC) and hardware-only ones above.
const videoMaster = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-HE2-stereo-32",NAME="English",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES,CHANNELS="2",URI="audio/he2-32/prog.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-HE-stereo-64",NAME="English",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES,CHANNELS="2",URI="audio/he-64/prog.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-stereo-128",NAME="English",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES,CHANNELS="2",URI="audio/lc-128/prog.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-stereo-256",NAME="English",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES,CHANNELS="2",URI="audio/lc-256/prog.m3u8"
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=90000,CODECS="avc1.64001f",RESOLUTION=864x486,URI="video/iframe.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=720000,AVERAGE-BANDWIDTH=600000,CODECS="avc1.64001e,mp4a.40.5",RESOLUTION=640x360,FRAME-RATE=23.976,HDCP-LEVEL=NONE,AUDIO="audio-HE-stereo-64",CLOSED-CAPTIONS=NONE,ALLOWED-CPC="com.apple.streamingkeydelivery:AppleBaseline/AppleMain/Main,com.microsoft.playready:SL2000/SL3000,urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_SOFTWARE/WIDEVINE_HARDWARE"
video/360/prog.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2100000,CODECS="avc1.64001f,mp4a.40.2",RESOLUTION=864x486,FRAME-RATE=23.976,HDCP-LEVEL=NONE,AUDIO="audio-stereo-128",CLOSED-CAPTIONS=NONE,ALLOWED-CPC="com.apple.streamingkeydelivery:AppleBaseline/AppleMain/Main,urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_SOFTWARE/WIDEVINE_HARDWARE"
video/486/prog.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2400000,CODECS="hvc1.2.4.L90.B0,mp4a.40.2",RESOLUTION=864x486,FRAME-RATE=23.976,HDCP-LEVEL=NONE,AUDIO="audio-stereo-128",CLOSED-CAPTIONS=NONE,ALLOWED-CPC="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_SOFTWARE/WIDEVINE_HARDWARE"
video/486-hevc/prog.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5000000,CODECS="avc1.640028,mp4a.40.2",RESOLUTION=1920x1080,FRAME-RATE=23.976,HDCP-LEVEL=TYPE-0,AUDIO="audio-stereo-256",CLOSED-CAPTIONS=NONE,ALLOWED-CPC="com.apple.streamingkeydelivery:AppleMain/Main,urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_HARDWARE"
video/1080/prog.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3000000,CODECS="avc1.64001f,mp4a.40.2",RESOLUTION=1280x720,FRAME-RATE=23.976,HDCP-LEVEL=NONE,AUDIO="audio-stereo-256",CLOSED-CAPTIONS=NONE,ALLOWED-CPC="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_HARDWARE"
video/720/prog.m3u8
`

func TestParseVideoMaster(t *testing.T) {
	vs, err := ParseMaster([]byte(videoMaster))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 5 {
		t.Fatalf("%d variants, want 5 (I-frame variants are not playable variants)", len(vs))
	}
	got := vs[1]
	want := Variant{
		URI: "video/486/prog.m3u8", Bandwidth: 2100000, Codecs: "avc1.64001f,mp4a.40.2", Audio: "audio-stereo-128",
		Width: 864, Height: 486, FrameRate: 23.976, HDCPLevel: "NONE",
		AllowedCPC: "com.apple.streamingkeydelivery:AppleBaseline/AppleMain/Main,urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_SOFTWARE/WIDEVINE_HARDWARE",
	}
	if got != want {
		t.Fatalf("variant\n got %+v\nwant %+v", got, want)
	}

	rs, err := ParseRenditions([]byte(videoMaster))
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 4 {
		t.Fatalf("%d renditions, want 4", len(rs))
	}
	if r := rs[3]; r != (Rendition{Type: "AUDIO", GroupID: "audio-stereo-256", Name: "English", Language: "en", URI: "audio/lc-256/prog.m3u8", Default: true}) {
		t.Fatalf("rendition %+v", r)
	}
	if _, err := ParseRenditions([]byte("not a playlist")); err == nil {
		t.Fatal("missing #EXTM3U accepted")
	}
	if rs, err := ParseRenditions([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\na.m3u8\n")); err != nil || len(rs) != 0 {
		t.Fatalf("master without renditions: %v %v", rs, err)
	}
}

func TestSoftwareWidevine(t *testing.T) {
	const wv = "urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"
	for _, tc := range []struct {
		cpc  string
		want bool
	}{
		{"", true},
		{wv + ":WIDEVINE_SOFTWARE", true},
		{wv + ":WIDEVINE_SOFTWARE/WIDEVINE_HARDWARE", true},
		{wv + ":WIDEVINE_HARDWARE", false},
		{"com.apple.streamingkeydelivery:AppleMain/Main," + wv + ":WIDEVINE_HARDWARE", false},
		// Only FairPlay and PlayReady are restricted.
		{"com.apple.streamingkeydelivery:AppleMain/Main,com.microsoft.playready:SL3000", true},
		// Several Widevine entries: any listing software allows it.
		{wv + ":WIDEVINE_HARDWARE," + wv + ":WIDEVINE_SOFTWARE", true},
	} {
		if got := (Variant{AllowedCPC: tc.cpc}).SoftwareWidevine(); got != tc.want {
			t.Errorf("SoftwareWidevine(%q) = %v, want %v", tc.cpc, got, tc.want)
		}
	}
}

func TestSelectVideo(t *testing.T) {
	const (
		sw = "urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_SOFTWARE"
		hw = "urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_HARDWARE"
	)
	v := func(uri, codecs string, bw, w int, hdcp, cpc string) Variant {
		return Variant{URI: uri, Codecs: codecs, Bandwidth: bw, Width: w, Height: w * 9 / 16, HDCPLevel: hdcp, AllowedCPC: cpc}
	}
	for _, tc := range []struct {
		name string
		vs   []Variant
		want string // "" = none
	}{
		{"master", must(ParseMaster([]byte(videoMaster))), "video/486/prog.m3u8"},
		{"H.264 before a higher HEVC", []Variant{
			v("hevc", "hvc1.2.4.L90.B0", 2400000, 864, "NONE", sw),
			v("avc", "avc1.64001f", 1200000, 640, "NONE", sw),
		}, "avc"},
		{"no HEVC: the CDM's decoder takes H.264 only", []Variant{
			v("avc-hw", "avc1.640028", 5000000, 1920, "TYPE-0", hw),
			v("hevc", "hvc1.2.4.L90.B0", 2400000, 864, "NONE", sw),
		}, ""},
		{"highest bandwidth, playlist order on ties", []Variant{
			v("a", "avc1.64001f", 2000000, 864, "", ""),
			v("b", "avc1.64001f", 2000000, 864, "", ""),
			v("low", "avc1.64001e", 700000, 640, "", ""),
		}, "a"},
		{"HDCP refused", []Variant{v("hdcp", "avc1.64001f", 2000000, 864, "TYPE-0", sw)}, ""},
		{"hardware CDM only", []Variant{v("hw", "avc1.64001f", 2000000, 864, "NONE", hw)}, ""},
		{"audio-only variants", []Variant{{URI: "aac", Codecs: "mp4a.40.2", Bandwidth: 256000}}, ""},
		{"unknown video codec", []Variant{v("av1", "av01.0.08M.08", 2000000, 864, "NONE", sw)}, ""},
		{"empty", nil, ""},
	} {
		got, ok := SelectVideo(tc.vs)
		if ok != (tc.want != "") || got.URI != tc.want {
			t.Errorf("%s: got %q (%v), want %q", tc.name, got.URI, ok, tc.want)
		}
	}
}

func TestSelectAudio(t *testing.T) {
	all := must(ParseRenditions([]byte(videoMaster)))
	lc128 := Variant{Audio: "audio-stereo-128"}
	for _, tc := range []struct {
		name string
		rs   []Rendition
		v    Variant
		want string // URI; "" = none
	}{
		{"256 kbps stereo AAC-LC when listed", all, lc128, "audio/lc-256/prog.m3u8"},
		{"else the variant's group", all[:3], lc128, "audio/lc-128/prog.m3u8"},
		{"the group's default", []Rendition{
			{Type: "AUDIO", GroupID: "g", Language: "fr", URI: "fr.m3u8"},
			{Type: "AUDIO", GroupID: "g", Language: "en", URI: "en.m3u8", Default: true},
			{Type: "AUDIO", GroupID: "g", Language: "de", URI: "de.m3u8", Default: true},
		}, Variant{Audio: "g"}, "en.m3u8"},
		{"first without a default", []Rendition{
			{Type: "AUDIO", GroupID: "g", URI: "one.m3u8"},
			{Type: "AUDIO", GroupID: "g", URI: "two.m3u8"},
		}, Variant{Audio: "g"}, "one.m3u8"},
		{"muxed (no URI) and other types skipped", []Rendition{
			{Type: "AUDIO", GroupID: "g", Default: true},
			{Type: "SUBTITLES", GroupID: "g", URI: "subs.m3u8"},
		}, Variant{Audio: "g"}, ""},
		{"variant without a group", all[:3], Variant{}, ""},
	} {
		got, ok := SelectAudio(tc.rs, tc.v)
		if ok != (tc.want != "") || got.URI != tc.want {
			t.Errorf("%s: got %q (%v), want %q", tc.name, got.URI, ok, tc.want)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
