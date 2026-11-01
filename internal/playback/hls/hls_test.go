package hls

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// Shape of Apple's 28:ctrp256 playlists (trimmed).
const applePlaylist = `#EXTM3U
#EXT-X-TARGETDURATION:10
#EXT-X-VERSION:7
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-KEY:METHOD=ISO-23001-7,URI="data:;base64,AAAAAEIk6OAAHbBdgj4o5Q=="
#EXT-X-MAP:URI="mzaf_A1.rmhq.aac.wa.mp4",BYTERANGE="1006@0"
#EXTINF:9.98458,	
#EXT-X-BYTERANGE:336058@1006
mzaf_A1.rmhq.aac.wa.mp4
#EXTINF:9.98458,	
#EXT-X-BYTERANGE:334838@337064
mzaf_A1.rmhq.aac.wa.mp4
#EXTINF:8.71256,	
#EXT-X-BYTERANGE:284472@671902
mzaf_A1.rmhq.aac.wa.mp4
#EXT-X-ENDLIST
`

func TestParseApple(t *testing.T) {
	p, err := Parse([]byte(applePlaylist))
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 7 || p.TargetDuration != 10*time.Second || !p.Ended {
		t.Fatalf("header: %+v", p)
	}
	if len(p.Segments) != 3 {
		t.Fatalf("segments: %d", len(p.Segments))
	}
	s := p.Segments[1]
	if s.URI != "mzaf_A1.rmhq.aac.wa.mp4" || s.Range != (ByteRange{337064, 334838}) {
		t.Fatalf("segment 1: %+v", s)
	}
	if s.Start != 9984580*time.Microsecond || s.Duration != 9984580*time.Microsecond {
		t.Fatalf("segment 1 timing: %v %v", s.Start, s.Duration)
	}
	if s.Map == nil || s.Map.Range != (ByteRange{0, 1006}) {
		t.Fatalf("map: %+v", s.Map)
	}
	want := []byte{0, 0, 0, 0, 0x42, 0x24, 0xe8, 0xe0, 0, 0x1d, 0xb0, 0x5d, 0x82, 0x3e, 0x28, 0xe5}
	if s.Key == nil || s.Key.Method != "ISO-23001-7" || !bytes.Equal(s.Key.KID, want) {
		t.Fatalf("key: %+v", s.Key)
	}
	if got := p.Duration(); got != 28681720*time.Microsecond {
		t.Fatalf("duration %v", got)
	}
	if h := s.Range.Header(); h != "bytes=337064-671901" {
		t.Fatalf("range header %q", h)
	}
}

func TestImplicitByteRangeOffsets(t *testing.T) {
	p, err := Parse([]byte(`#EXTM3U
#EXT-X-MAP:URI="a.mp4",BYTERANGE="100"
#EXTINF:1,
#EXT-X-BYTERANGE:50
a.mp4
#EXTINF:1,
#EXT-X-BYTERANGE:70
a.mp4
#EXTINF:1,
#EXT-X-BYTERANGE:30@1000
a.mp4
#EXTINF:1,
#EXT-X-BYTERANGE:5
a.mp4
#EXTINF:1,
b.mp4
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []ByteRange{{100, 50}, {150, 70}, {1000, 30}, {1030, 5}, {}}
	for i, s := range p.Segments {
		if s.Range != want[i] {
			t.Errorf("segment %d: %+v, want %+v", i, s.Range, want[i])
		}
	}
	if p.Segments[4].Range.Header() != "" {
		t.Error("whole-resource segment must not send a Range header")
	}
}

func TestImplicitOffsetErrors(t *testing.T) {
	for name, pl := range map[string]string{
		"no previous range": "#EXTM3U\n#EXTINF:1,\n#EXT-X-BYTERANGE:5\na.mp4\n",
		"other uri":         "#EXTM3U\n#EXTINF:1,\n#EXT-X-BYTERANGE:5@0\na.mp4\n#EXTINF:1,\n#EXT-X-BYTERANGE:5\nb.mp4\n",
		"after whole":       "#EXTM3U\n#EXTINF:1,\na.mp4\n#EXTINF:1,\n#EXT-X-BYTERANGE:5\na.mp4\n",
	} {
		if _, err := Parse([]byte(pl)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for name, pl := range map[string]string{
		"no header":    "#EXTINF:1,\na.mp4\n",
		"no extinf":    "#EXTM3U\na.mp4\n",
		"empty":        "#EXTM3U\n#EXT-X-ENDLIST\n",
		"bad duration": "#EXTM3U\n#EXTINF:x,\na.mp4\n",
		"bad range":    "#EXTM3U\n#EXTINF:1,\n#EXT-X-BYTERANGE:0@0\na.mp4\n",
		"master":       "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nv.m3u8\n",
		"bad key":      "#EXTM3U\n#EXT-X-KEY:URI=\"x\n#EXTINF:1,\na.mp4\n",
		"bad data uri": "#EXTM3U\n#EXT-X-KEY:METHOD=ISO-23001-7,URI=\"data:;base64,!!\"\n#EXTINF:1,\na.mp4\n",
	} {
		if _, err := Parse([]byte(pl)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestKeyAttributes(t *testing.T) {
	p, err := Parse([]byte(`#EXTM3U
#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://a,b",KEYFORMAT="com.apple.streamingkeydelivery",KEYFORMATVERSIONS="1"
#EXTINF:1,
a.mp4
#EXT-X-KEY:METHOD=NONE
#EXTINF:1,
b.mp4
`))
	if err != nil {
		t.Fatal(err)
	}
	k := p.Segments[0].Key
	if k == nil || k.URI != "skd://a,b" || k.KeyFormat != "com.apple.streamingkeydelivery" || k.KeyFormatVersions != "1" || k.KID != nil {
		t.Fatalf("key: %+v", k)
	}
	if p.Segments[1].Key != nil {
		t.Fatal("METHOD=NONE must clear the key")
	}
}

func TestSegmentAt(t *testing.T) {
	p, err := Parse([]byte(applePlaylist))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		at   time.Duration
		want int
	}{{-time.Second, 0}, {0, 0}, {9 * time.Second, 0}, {9984580 * time.Microsecond, 1}, {20 * time.Second, 2}, {time.Hour, 2}} {
		if got := p.SegmentAt(c.at); got != c.want {
			t.Errorf("SegmentAt(%v) = %d, want %d", c.at, got, c.want)
		}
	}
}

func TestResolve(t *testing.T) {
	got, err := Resolve("https://aod.example/a/b/list.m3u8?x=1", "seg.mp4")
	if err != nil || got != "https://aod.example/a/b/seg.mp4" {
		t.Fatalf("%q %v", got, err)
	}
	if got, _ := Resolve("https://aod.example/a/list.m3u8", "https://cdn.example/s.mp4"); !strings.HasPrefix(got, "https://cdn.example/") {
		t.Fatalf("absolute: %q", got)
	}
}

func TestExtinfTitle(t *testing.T) {
	p, err := Parse([]byte(`#EXTM3U
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:7
#EXTINF:10.0,title="Hello, World",artist="Adele"
a.ts
#EXTINF:10,  Plain title  
b.ts
#EXTINF:10
c.ts
#EXTINF:10,
d.ts
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`title="Hello, World",artist="Adele"`, "Plain title", "", ""}
	if len(p.Segments) != len(want) {
		t.Fatalf("segments: %d", len(p.Segments))
	}
	for i, s := range p.Segments {
		if s.Title != want[i] {
			t.Errorf("segment %d title %q, want %q", i, s.Title, want[i])
		}
		if s.Duration != 10*time.Second || s.Seq != 7+i {
			t.Errorf("segment %d: %v seq %d", i, s.Duration, s.Seq)
		}
	}
	attrs, err := ParseAttributes(p.Segments[0].Title)
	if err != nil || attrs["title"] != "Hello, World" || attrs["artist"] != "Adele" {
		t.Fatalf("attributes %v %v", attrs, err)
	}
}

func TestParseAttributes(t *testing.T) {
	for _, c := range []struct {
		in   string
		want map[string]string
	}{
		{`A=1,B="x, y",C=z`, map[string]string{"A": "1", "B": "x, y", "C": "z"}},
		{` title="t" , artist=a `, map[string]string{"title": "t", "artist": "a"}},
		{"", map[string]string{}},
	} {
		got, err := ParseAttributes(c.in)
		if err != nil || len(got) != len(c.want) {
			t.Errorf("%q: %v %v", c.in, got, err)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%q: %s=%q, want %q", c.in, k, got[k], v)
			}
		}
	}
	for _, in := range []string{"A", `A="x`, `A="x"y`} {
		if _, err := ParseAttributes(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}
