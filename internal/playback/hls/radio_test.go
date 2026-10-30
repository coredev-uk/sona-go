package hls

import (
	"bytes"
	"testing"
	"time"
)

// Shape of Apple Music 1's live CMAF playlist (trimmed): three keys per
// programme (PlayReady, Widevine, FairPlay), a programme change with a new
// init segment and key after a discontinuity.
const livePlaylist = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-TARGETDURATION:16
#EXT-X-MEDIA-SEQUENCE:2802150
#EXT-X-DISCONTINUITY-SEQUENCE:388
#EXT-X-KEY:KEYFORMATVERSIONS="1",METHOD=SAMPLE-AES,URI="data:text/plain;mt=UNKNOWN;vendorId=awsBeats1;charset=UTF-16;base64,vgEAAAEAAQ==",KEYFORMAT="com.microsoft.playready"
#EXT-X-KEY:KEYFORMATVERSIONS="1",METHOD=SAMPLE-AES,URI="data:text/plain;mt=UNKNOWN;vendorId=awsBeats1;e=radioEvent;v=v1;base64,AAAAOHBzc2gAAAAA7e+LqXnWSs6jyCfc1R0h7QAAABgSENy3NmoGDTvYki09ylzU9S5I88aJmwY=",IV=0x2348c995f9f2c4276dd2c235a32b0ad1,KEYFORMAT="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"
#EXT-X-KEY:KEYFORMATVERSIONS="1",METHOD=SAMPLE-AES,URI="skd://linear/DCB7366A060D3BD8922D3DCA5CD4F52E?e=radioEvent",IV=0x2348c995f9f2c4276dd2c235a32b0ad1,KEYFORMAT="com.apple.streamingkeydelivery"
#EXT-X-PROGRAM-DATE-TIME:2026-10-30T10:58:15.035Z
#EXT-X-MAP:URI="segment_1_2802046_init.mp4"
#EXTINF:16.02131,
segment_1_2802150.mp4
#EXT-X-PROGRAM-DATE-TIME:2026-10-30T10:58:31.051Z
#EXTINF:16.02131,
segment_1_2802151.mp4
#EXT-X-DISCONTINUITY
#EXT-X-KEY:METHOD=SAMPLE-AES,URI="data:text/plain;base64,AAAAOHBzc2gAAAAA7e+LqXnWSs6jyCfc1R0h7QAAABgSEAAAAAAAAAAAAAAAAAAAAAFI88aJmwY=",KEYFORMAT="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"
#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://linear/00000000000000000000000000000001",KEYFORMAT="com.apple.streamingkeydelivery"
#EXT-X-MAP:URI="segment_1_2802152_init.mp4"
#EXTINF:15.99998,
segment_1_2802152.mp4
`

const radioMaster = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-STREAM-INF:CODECS="mp4a.40.5",AVERAGE-BANDWIDTH=70400,BANDWIDTH=70400,AUDIO="audio_1"
https://a.example/index_2.m3u8
#EXT-X-STREAM-INF:CODECS="mp4a.40.2",AVERAGE-BANDWIDTH=281600,BANDWIDTH=281600,AUDIO="audio_0"
https://a.example/index_1.m3u8
#EXT-X-STREAM-INF:CODECS="mp4a.40.2",AVERAGE-BANDWIDTH=281600,BANDWIDTH=281600,AUDIO="audio_0"
https://backup.example/index_1.m3u8
`

func TestParseLive(t *testing.T) {
	p, err := Parse([]byte(livePlaylist))
	if err != nil {
		t.Fatal(err)
	}
	if p.Ended || p.MediaSequence != 2802150 || p.TargetDuration != 16*time.Second || len(p.Segments) != 3 {
		t.Fatalf("playlist %+v", p)
	}
	s0, s2 := p.Segments[0], p.Segments[2]
	if s0.Seq != 2802150 || s2.Seq != 2802152 || s0.Discontinuity || !s2.Discontinuity {
		t.Fatalf("seq/discontinuity %d %v %d %v", s0.Seq, s0.Discontinuity, s2.Seq, s2.Discontinuity)
	}
	// The Widevine key wins over the FairPlay key set after it.
	if s0.Key == nil || s0.Key.KeyFormat != WidevineKeyFormat || s0.Key.Method != "SAMPLE-AES" {
		t.Fatalf("key %+v", s0.Key)
	}
	if len(s0.Key.InitData) != 56 || !bytes.Equal(s0.Key.InitData[4:8], []byte("pssh")) {
		t.Fatalf("init data %x", s0.Key.InitData)
	}
	if want := []byte{0xdc, 0xb7, 0x36, 0x6a, 0x06, 0x0d, 0x3b, 0xd8, 0x92, 0x2d, 0x3d, 0xca, 0x5c, 0xd4, 0xf5, 0x2e}; !bytes.Equal(s0.Key.KID, want) {
		t.Fatalf("kid %x", s0.Key.KID)
	}
	if bytes.Equal(s2.Key.InitData, s0.Key.InitData) || s2.Map.URI != "segment_1_2802152_init.mp4" || s0.Map.URI != "segment_1_2802046_init.mp4" {
		t.Fatalf("programme change not seen: %+v %+v", s2.Key, s2.Map)
	}
	if s2.Start != 32042620*time.Microsecond {
		t.Fatalf("start %s", s2.Start)
	}
	if _, err := Parse([]byte(radioMaster)); err == nil {
		t.Fatal("master parsed as media playlist")
	}
}

func TestParseMaster(t *testing.T) {
	if !IsMaster([]byte(radioMaster)) || IsMaster([]byte(livePlaylist)) {
		t.Fatal("IsMaster")
	}
	vs, err := ParseMaster([]byte(radioMaster))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 3 || vs[0].Bandwidth != 70400 || vs[0].Codecs != "mp4a.40.5" {
		t.Fatalf("variants %+v", vs)
	}
	// Highest-bandwidth AAC-LC, primary CDN first.
	if got := BestAAC(vs).URI; got != "https://a.example/index_1.m3u8" {
		t.Fatalf("best %s", got)
	}
	if _, err := ParseMaster([]byte("#EXTM3U\n")); err == nil {
		t.Fatal("empty master accepted")
	}
}
