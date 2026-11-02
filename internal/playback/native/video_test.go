//go:build linux || (darwin && cgo)

package native

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/video"
)

// fakeVideo records a fakeHost's video calls.
type fakeVideo struct {
	loads []videoLoad
	feeds []feedCall
	ends  []uint64
}

type videoLoad struct {
	track uint64
	init  []byte
	kid   []byte
	shift time.Duration
	start time.Duration
}

func (f *fakeHost) videoCalls() fakeVideo {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fakeVideo{
		loads: append([]videoLoad(nil), f.video.loads...),
		feeds: append([]feedCall(nil), f.video.feeds...),
		ends:  append([]uint64(nil), f.video.ends...),
	}
}

// videoTestID is the fixture's music video (catalog ID).
const videoTestID = "1440841363"

// The fixture video's key IDs: the playlists' Widevine keys name them;
// the video init segment's tenc carries an all-zero one, as Apple's do.
var (
	videoKID = []byte{0x9c, 0x6f, 0x2a, 0x11, 0x41, 0x0b, 0x4e, 0x2d, 0x8f, 0x67, 0x3a, 0xe2, 0x14, 0x5d, 0x0c, 0x77}
	audioKID = []byte{0x3e, 0x51, 0x07, 0xc4, 0x2b, 0x99, 0x40, 0x1a, 0xb2, 0x6d, 0x58, 0x0f, 0xe1, 0x34, 0x7c, 0x02}
)

func mp4Box(typ string, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	return append(binary.BigEndian.AppendUint32(nil, uint32(8+len(body))), append([]byte(typ), body...)...)
}

func be32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func be16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }

// testWidevinePSSH is a version 0 Widevine pssh box naming kid.
func testWidevinePSSH(kid []byte) []byte {
	sysID := []byte{0xed, 0xef, 0x8b, 0xa9, 0x79, 0xd6, 0x4a, 0xce, 0xa3, 0xc8, 0x27, 0xdc, 0xd5, 0x1d, 0x21, 0xed}
	data := append([]byte{0x12, byte(len(kid))}, kid...)
	return mp4Box("pssh", be32(0), sysID, be32(uint32(len(data))), data)
}

// videoInitSeg is an 864×486 'cbcs' encv/avc1 init segment (H.264 High)
// with an all-zero default key ID.
var videoInitSeg = func() []byte {
	avcC := mp4Box("avcC", []byte{1, 100, 0, 30, 0xff, 0xe1, 0, 4, 0x67, 100, 0, 30, 1, 0, 4, 0x68, 0xee, 0x3c, 0x80})
	tenc := mp4Box("tenc", be32(1<<24), []byte{0, 0x19, 1, 0}, make([]byte, 16), []byte{16}, bytes.Repeat([]byte{7}, 16))
	sinf := mp4Box("sinf", mp4Box("frma", []byte("avc1")), mp4Box("schm", be32(0), []byte("cbcs"), be32(0x10000)), mp4Box("schi", tenc))
	entry := mp4Box("encv", make([]byte, 6), be16(1), make([]byte, 16), be16(864), be16(486),
		be32(0x480000), be32(0x480000), be32(0), be16(1), make([]byte, 32), be16(0x18), be16(0xffff), avcC, sinf)
	stsd := mp4Box("stsd", be32(0), be32(1), entry)
	mdhd := mp4Box("mdhd", be32(0), be32(0), be32(0), be32(90000), be32(0), be32(0))
	tkhd := mp4Box("tkhd", be32(7), be32(0), be32(0), be32(1), make([]byte, 68))
	trak := mp4Box("trak", tkhd, mp4Box("mdia", mdhd, mp4Box("minf", mp4Box("stbl", stsd))))
	return append(mp4Box("ftyp", []byte("iso8"), be32(0)), mp4Box("moov", trak)...)
}()

// audioFragment is a one-sample AAC fragment of the fixture's audio init
// segment (track 1, 44.1 kHz) whose decode time is at.
func audioFragment(at time.Duration) []byte {
	tfhd := mp4Box("tfhd", be32(0x020000), be32(1))
	tfdt := mp4Box("tfdt", be32(0), be32(uint32(at*44100/time.Second)))
	payload := []byte{0x21, 0x10, 0x04, 0x60, 0x8c, 0x1c}
	trun := func(off uint32) []byte {
		return mp4Box("trun", be32(0x000301), be32(1), be32(off), be32(1024), be32(uint32(len(payload))))
	}
	moof := func(off uint32) []byte {
		return mp4Box("moof", mp4Box("mfhd", be32(0), be32(1)), mp4Box("traf", tfhd, tfdt, trun(off)))
	}
	n := len(moof(0))
	return append(moof(uint32(n+8)), mp4Box("mdat", payload)...)
}

// videoOffset is where the fixture video's timestamps start, as Apple's
// do (10 s).
const videoOffset = 10 * time.Second

// videoSegment is the fixture video's (opaque) video segment: the engine
// passes it through to the host.
var videoSegment = []byte("video segment 0")

// videoFiles are the fixture video's master playlist (one SD H.264
// variant a software CDM may play, and a stereo AAC rendition), its media
// playlists with the three SAMPLE-AES keys, init segments and segments.
var videoFiles = func() map[string][]byte {
	keys := func(kid []byte) string {
		return `#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://itunes.apple.com/P000000000/s1/e1",KEYFORMAT="com.apple.streamingkeydelivery",KEYFORMATVERSIONS="1"` + "\n" +
			`#EXT-X-KEY:METHOD=SAMPLE-AES,URI="data:text/plain;charset=UTF-16;base64,vgEAAAEAAQ==",KEYFORMAT="com.microsoft.playready",KEYFORMATVERSIONS="1"` + "\n" +
			`#EXT-X-KEY:METHOD=SAMPLE-AES,URI="data:text/plain;base64,` + base64.StdEncoding.EncodeToString(testWidevinePSSH(kid)) +
			`",KEYFORMAT="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed",KEYFORMATVERSIONS="1"` + "\n"
	}
	media := func(kid []byte, init, seg string) []byte {
		return []byte("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:6\n#EXT-X-PLAYLIST-TYPE:VOD\n" + keys(kid) +
			`#EXT-X-MAP:URI="` + init + `"` + "\n#EXTINF:6,\n" + seg + "\n#EXT-X-ENDLIST\n")
	}
	return map[string][]byte{
		"/mv/master.m3u8": []byte(`#EXTM3U
#EXT-X-VERSION:7
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-stereo-256",NAME="English",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES,CHANNELS="2",URI="audio/prog.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=2100000,CODECS="avc1.64001f,mp4a.40.2",RESOLUTION=864x486,FRAME-RATE=25,HDCP-LEVEL=NONE,AUDIO="audio-stereo-256",ALLOWED-CPC="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_SOFTWARE"
video/prog.m3u8
`),
		"/mv/video/prog.m3u8": media(videoKID, "init.mp4", "seg0.mp4"),
		"/mv/video/init.mp4":  videoInitSeg,
		"/mv/video/seg0.mp4":  videoSegment,
		"/mv/audio/prog.m3u8": media(audioKID, "init.mp4", "seg0.mp4"),
		"/mv/audio/init.mp4":  initSeg,
		"/mv/audio/seg0.mp4":  audioFragment(videoOffset),
	}
}()

func TestVideoCapability(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Options
		want bool
	}{
		{name: "surface", o: Options{Video: video.NewSurface()}, want: true},
		{name: "no surface", o: Options{}},
		{name: "preview", o: Options{Video: video.NewSurface(), Preview: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := New(tc.o)
			t.Cleanup(func() { _ = e.Close() })
			if got := e.Capabilities().Video; got != tc.want {
				t.Fatalf("Capabilities().Video = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPlayVideo: a music video plays its audio rendition as the track and
// its SD H.264 rendition through LoadVideo and FeedVideo, each licensed in
// a session of its own with the playlist's key IDs, its pictures timed
// from the offset its first audio segment starts at.
func TestPlayVideo(t *testing.T) {
	in, err := fmp4.ParseInit(videoInitSeg)
	if err != nil || !in.IsVideo() || in.Codec != "avc1" || in.Width != 864 || !bytes.Equal(in.KID, make([]byte, 16)) {
		t.Fatalf("fixture video init %+v: %v", in, err)
	}
	ain, err := fmp4.ParseInit(initSeg)
	if err != nil {
		t.Fatal(err)
	}

	h := newHarness(t, -1, false)
	surface := video.NewSurface()
	h.e.surface = surface // as Options.Video sets it
	id := playback.VideoID(videoTestID)
	if err := h.e.Play(ctx(t), id, 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	eventually(t, "video fed to its end", func() bool { return len(f.videoCalls().ends) == 1 })
	v := f.videoCalls()
	loads, _, sessions := f.snapshot()
	if sessions != 2 {
		t.Fatalf("%d licence sessions, want the audio's and the video's", sessions)
	}
	if len(loads) != 1 || len(v.loads) != 1 {
		t.Fatalf("loads %+v, video loads %+v", loads, v.loads)
	}
	tok := loads[0].track
	if want := 6*time.Second - ain.Priming(); loads[0].dur != want {
		t.Fatalf("track duration %v, want %v", loads[0].dur, want)
	}
	vl := v.loads[0]
	if vl.track != tok || vl.start != 0 || !bytes.Equal(vl.init, videoInitSeg) {
		t.Fatalf("video load %+v of track %d", vl, tok)
	}
	if !bytes.Equal(vl.kid, videoKID) {
		t.Fatalf("video KID %x, want the playlist's %x (not the init segment's)", vl.kid, videoKID)
	}
	if want := videoOffset + ain.Priming(); vl.shift != want {
		t.Fatalf("video shift %v, want %v", vl.shift, want)
	}
	if len(v.feeds) != 1 || v.feeds[0].track != tok || !bytes.Equal(v.feeds[0].data, videoSegment) || v.ends[0] != tok {
		t.Fatalf("video feeds %+v, ends %v", v.feeds, v.ends)
	}
	eventually(t, "audio fed", func() bool { return f.fedTrack(tok) == 1 })
	if _, feeds, _ := f.snapshot(); !bytes.Equal(feeds[0].data, videoFiles["/mv/audio/seg0.mp4"]) {
		t.Fatalf("audio feed %x", feeds[0].data)
	}
	f.emit(cdmhost.Event{Kind: cdmhost.EventStarted, Track: tok, At: time.Now()})

	// Pictures of the loaded video reach the surface; others do not.
	pic := make([]byte, cdmhost.I420Size(2, 2))
	h.e.showFrame(cdmhost.VideoFrame{Track: tok + 1, Width: 2, Height: 2, Data: pic})
	h.e.showFrame(cdmhost.VideoFrame{Track: tok, Position: time.Second, Width: 2, Height: 2, Data: pic})
	fr, ok := surface.Next(ctx(t), 0)
	if !ok || fr.Seq != 1 || fr.Position != time.Second {
		t.Fatalf("surface %+v %v, want only the loaded video's picture", fr, ok)
	}

	// A video is never joined to the next item.
	eventually(t, "audio fed to its end", func() bool {
		h.e.mu.Lock()
		defer h.e.mu.Unlock()
		return h.e.cur != nil && h.e.cur.fed
	})
	h.e.SetNext(playback.Next{ID: "B", Gapless: true})
	h.e.mu.Lock()
	pending := h.e.cur.pending
	h.e.mu.Unlock()
	if pending != nil {
		t.Fatal("a next item was armed behind a video")
	}

	// Stopping takes the picture off the surface.
	if err := h.e.Stop(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if fr, ok := surface.Next(ctx(t), 1); ok || fr.Seq != 2 {
		t.Fatalf("surface after Stop %+v %v", fr, ok)
	}
	if n := len(f.nextCalls()); n != 0 {
		t.Fatalf("%d Next calls", n)
	}
}

// TestNoVideoArmedBehindSong: a music video is never queued gaplessly
// behind a song.
func TestNoVideoArmedBehindSong(t *testing.T) {
	h := newHarness(t, -1, false)
	h.e.surface = video.NewSurface()
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	eventually(t, "A fed to its end", func() bool {
		h.e.mu.Lock()
		defer h.e.mu.Unlock()
		return h.e.cur != nil && h.e.cur.fed
	})
	h.e.SetNext(playback.Next{ID: playback.VideoID(videoTestID), Gapless: true})
	h.e.mu.Lock()
	pending := h.e.cur.pending
	h.e.mu.Unlock()
	if pending != nil {
		t.Fatal("a video was armed behind a song")
	}
}

// TestVideoUnsupportedWithoutSurface: an engine without a video surface
// plays no videos.
func TestVideoUnsupportedWithoutSurface(t *testing.T) {
	h := newHarness(t, -1, false)
	err := h.e.Play(ctx(t), playback.VideoID(videoTestID), 0)
	if !errors.Is(err, playback.ErrUnsupported) {
		t.Fatalf("Play: %v, want ErrUnsupported", err)
	}
	if n := h.fx.requests("/mv/master.m3u8", ""); n != 0 {
		t.Fatalf("master playlist fetched %d times", n)
	}
}
