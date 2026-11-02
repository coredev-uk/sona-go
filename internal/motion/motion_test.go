package motion

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// master is trimmed from a live editorialVideo master playlist: I-frame
// playlists, H.264 and HEVC renditions at several sizes and bit rates.
const master = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-INDEPENDENT-SEGMENTS

#EXT-X-I-FRAME-STREAM-INF:AVERAGE-BANDWIDTH=532758,BANDWIDTH=556195,VIDEO-RANGE=SDR,CODECS="avc1.64001f",RESOLUTION=486x486,URI="iframe_486.m3u8"

#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=2114569,BANDWIDTH=2251189,VIDEO-RANGE=SDR,CLOSED-CAPTIONS=NONE,CODECS="avc1.64001f",FRAME-RATE=30.000,RESOLUTION=768x768
video_768.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=1674948,BANDWIDTH=1936668,VIDEO-RANGE=SDR,CLOSED-CAPTIONS=NONE,CODECS="hvc1.2.20000000.L123.B0",FRAME-RATE=30.000,RESOLUTION=768x768
hevc_768.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=259170,BANDWIDTH=289412,VIDEO-RANGE=SDR,CLOSED-CAPTIONS=NONE,CODECS="avc1.64001f",FRAME-RATE=30.000,RESOLUTION=360x360
video_360.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=505815,BANDWIDTH=558461,VIDEO-RANGE=SDR,CLOSED-CAPTIONS=NONE,CODECS="avc1.64001f",FRAME-RATE=30.000,RESOLUTION=486x486
video_486_low.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=1518683,BANDWIDTH=1628801,VIDEO-RANGE=SDR,CLOSED-CAPTIONS=NONE,CODECS="avc1.64001f",FRAME-RATE=30.000,RESOLUTION=486x486
video_486.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=10114488,BANDWIDTH=10953358,VIDEO-RANGE=SDR,CLOSED-CAPTIONS=NONE,CODECS="avc1.640020",FRAME-RATE=30.000,RESOLUTION=1080x1080
video_1080_high.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=5072963,BANDWIDTH=5429622,VIDEO-RANGE=SDR,CLOSED-CAPTIONS=NONE,CODECS="avc1.640020",FRAME-RATE=30.000,RESOLUTION=1080x1080
video_1080.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=4000000,BANDWIDTH=4200000,VIDEO-RANGE=PQ,CODECS="avc1.640020",RESOLUTION=1080x1080
video_1080_hdr.m3u8
`

func TestChoose(t *testing.T) {
	vs := parseMaster(master)
	if len(vs) != 8 {
		t.Fatalf("parsed %d variants, want 8 (no I-frame playlists): %+v", len(vs), vs)
	}
	for _, tc := range []struct {
		minWidth int
		codec    string
		want     string
	}{
		{100, CodecH264, "video_360.m3u8"},
		{360, CodecH264, "video_360.m3u8"},
		// The smallest resolution at least that wide, its best bit rate.
		{480, CodecH264, "video_486.m3u8"},
		{720, CodecH264, "video_768.m3u8"},
		// Nothing wide enough: the largest, under the bit-rate cap
		// (both 1080 renditions exceed it: the lower one).
		{2000, CodecH264, "video_1080.m3u8"},
		// A web view that plays HEVC but not H.264 gets HEVC.
		{480, CodecHEVC, "hevc_768.m3u8"},
	} {
		v, ok := choose(vs, tc.minWidth, tc.codec)
		if !ok || v.URI != tc.want {
			t.Errorf("choose(%d, %s) = %q, %v; want %q", tc.minWidth, tc.codec, v.URI, ok, tc.want)
		}
	}
	hevcOnly := parseMaster("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,CODECS=\"hvc1.2\",RESOLUTION=768x768\nhevc.m3u8\n")
	if _, ok := choose(hevcOnly, 480, CodecH264); ok {
		t.Error("chose an H.264 rendition from an HEVC-only master")
	}
	// Dolby Vision and HDR renditions are never chosen.
	if _, ok := choose(parseMaster("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,CODECS=\"dvh1.05.01\",RESOLUTION=768x768\ndv.m3u8\n"), 480, CodecHEVC); ok {
		t.Error("chose a Dolby Vision rendition")
	}
	// Renditions with muxed audio still count by their video codec.
	muxed := parseMaster("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1628801,VIDEO-RANGE=SDR,CODECS=\"avc1.64001f,mp4a.40.2\",RESOLUTION=486x486\nmuxed.m3u8\n")
	if v, ok := choose(muxed, 480, CodecH264); !ok || v.URI != "muxed.m3u8" || v.describe() != "avc1.64001f,mp4a.40.2 486×486, 1.6 Mbit/s" {
		t.Errorf("muxed: %+v, %v (%s)", v, ok, v.describe())
	}
}

func TestFamily(t *testing.T) {
	for codecs, want := range map[string]string{
		"avc1.64001f":                  CodecH264,
		"avc3.640028":                  CodecH264,
		"mp4a.40.2, avc1.64001e":       CodecH264,
		"hvc1.2.20000000.L123.B0":      CodecHEVC,
		"hev1.1.6.L93.B0,mp4a.40.2":    CodecHEVC,
		"dvh1.05.01":                   "",
		"mp4a.40.2":                    "",
		"":                             "",
		"avc1":                         "",
		"hvc1.2.20000000.H150.B0,ec-3": CodecHEVC,
	} {
		if got := family(codecs); got != want {
			t.Errorf("family(%q) = %q, want %q", codecs, got, want)
		}
	}
}

func TestParseMedia(t *testing.T) {
	base, _ := url.Parse("https://mvod.itunes.apple.com/a/b/video.m3u8")
	str := func(us []*url.URL) []string {
		var out []string
		for _, u := range us {
			out = append(out, u.String())
		}
		return out
	}
	// Albums: one file, read by byte ranges.
	single := "#EXTM3U\n#EXT-X-MAP:URI=\"v.mp4\",BYTERANGE=\"895@0\"\n#EXTINF:2.8,\n#EXT-X-BYTERANGE:622920@895\nv.mp4\n#EXTINF:2.8,\n#EXT-X-BYTERANGE:717742@623815\nv.mp4\n#EXT-X-ENDLIST\n"
	us, err := parseMedia(single, base)
	if err != nil || !slices.Equal(str(us), []string{"https://mvod.itunes.apple.com/a/b/v.mp4"}) {
		t.Errorf("single file = %v, %v", str(us), err)
	}
	// Editorial playlists: an initialisation file and segment files
	// (as served for "Essentials" playlists).
	segmented := "#EXTM3U\n#EXT-X-TARGETDURATION:5\n#EXT-X-VERSION:7\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		"#EXT-X-MAP:URI=\"segmentis_VIDEO_CLIP_480x480_AVC_1_0.mp4\"\n#EXTINF:5.00000,\t\n#EXT-X-BITRATE:260\nsegmentis_VIDEO_CLIP_480x480_AVC_1_1.m4s\n" +
		"#EXTINF:5.00000,\t\nsegmentis_VIDEO_CLIP_480x480_AVC_1_2.m4s\n#EXT-X-ENDLIST\n"
	us, err = parseMedia(segmented, base)
	want := []string{
		"https://mvod.itunes.apple.com/a/b/segmentis_VIDEO_CLIP_480x480_AVC_1_0.mp4",
		"https://mvod.itunes.apple.com/a/b/segmentis_VIDEO_CLIP_480x480_AVC_1_1.m4s",
		"https://mvod.itunes.apple.com/a/b/segmentis_VIDEO_CLIP_480x480_AVC_1_2.m4s",
	}
	if err != nil || !slices.Equal(str(us), want) {
		t.Errorf("segmented = %v, %v; want %v", str(us), err, want)
	}
	for name, body := range map[string]string{
		"encrypted":         "#EXTM3U\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"skd://k\"\n#EXTINF:2,\nv.mp4\n",
		"two init sections": "#EXTM3U\n#EXT-X-MAP:URI=\"a.mp4\"\n#EXTINF:2,\n1.m4s\n#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"b.mp4\"\n#EXTINF:2,\n2.m4s\n",
		"init after frames": "#EXTM3U\n#EXTINF:2,\n1.m4s\n#EXT-X-MAP:URI=\"a.mp4\"\n",
		"empty":             "#EXTM3U\n#EXT-X-ENDLIST\n",
	} {
		if _, err := parseMedia(body, base); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// fakeCDN serves master.m3u8 → video_486.m3u8 / video_768.m3u8 /
// hevc_768.m3u8 → one fragmented MP4 each, and video_360.m3u8 → an
// initialisation file and two segment files (editorial packaging);
// hdr/master.m3u8 has HDR renditions only. It counts requests. files
// holds each rendition's whole fragmented file by name (v486, v768, h768,
// v360).
func fakeCDN(t *testing.T) (*httptest.Server, *atomic.Int32, map[string][]byte) {
	file := func(gops int) []byte {
		return testFile{movieScale: 30, scale: 30, base: 300, trunVersion: 1, frags: slices.Repeat([][]frame{gop}, gops)}.build()
	}
	files := map[string][]byte{"v486": file(2), "v768": file(5), "h768": file(4), "v360": file(3)}
	served := map[string][]byte{"/v/v486.mp4": files["v486"], "/v/v768.mp4": files["v768"], "/v/h768.mp4": files["h768"]}
	parts := segmented(t, files["v360"])
	segments := "#EXTM3U\n#EXT-X-MAP:URI=\"s360_0.mp4\"\n"
	served["/v/s360_0.mp4"] = parts[0]
	for i, p := range parts[1:] {
		name := fmt.Sprintf("s360_%d.m4s", i+1)
		served["/v/"+name] = p
		segments += "#EXTINF:1,\n" + name + "\n"
	}
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/v/master.m3u8":
			_, _ = io.WriteString(w, master)
		case "/hdr/master.m3u8":
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,VIDEO-RANGE=PQ,CODECS=\"avc1.640020\",RESOLUTION=1080x1080\nhdr.m3u8\n")
		case "/v/video_360.m3u8":
			_, _ = io.WriteString(w, segments)
		case "/v/video_486.m3u8", "/v/video_768.m3u8", "/v/hevc_768.m3u8":
			name := map[string]string{"/v/video_486.m3u8": "v486.mp4", "/v/video_768.m3u8": "v768.mp4", "/v/hevc_768.m3u8": "h768.mp4"}[r.URL.Path]
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-MAP:URI=\""+name+"\",BYTERANGE=\"100@0\"\n#EXTINF:2,\n#EXT-X-BYTERANGE:400@100\n"+name+"\n")
		default:
			b, ok := served[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(b)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, files
}

func newTestCache(t *testing.T, srv *httptest.Server, max int64) *Cache {
	c, err := New(Options{Dir: t.TempDir(), HTTP: srv.Client(), MaxBytes: max})
	if err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(srv.URL, "https://")
	c.allow = func(u *url.URL) bool { return u.Scheme == "https" && u.Host == host }
	return c
}

func get(c *Cache, path string, header http.Header) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	return w
}

// flattened is what the cache serves for the fragmented file b.
func flattened(t *testing.T, b []byte) []byte {
	t.Helper()
	out, err := flatten(b)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestServe(t *testing.T) {
	srv, hits, files := fakeCDN(t)
	c := newTestCache(t, srv, 0)
	if got := c.URL("http://example.com/master.m3u8"); got != "" {
		t.Errorf("URL(foreign) = %q", got)
	}
	u := c.URL(srv.URL + "/v/master.m3u8")
	if !strings.HasPrefix(u, "/motion/") || !strings.HasSuffix(u, "/720.mp4") {
		t.Fatalf("URL = %q", u)
	}
	key := strings.Split(u, "/")[2]

	// A header cover (400 device px) gets the 486 px rendition, remuxed.
	small := strings.Replace(u, "/720.mp4", "/400.mp4", 1)
	w := get(c, small, nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), flattened(t, files["v486"])) || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("small: %d %q (%d bytes)", w.Code, w.Header().Get("Content-Type"), w.Body.Len())
	}
	// The now-playing cover gets the 768 px one.
	w = get(c, u, nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), flattened(t, files["v768"])) {
		t.Fatalf("large: %d (%d bytes)", w.Code, w.Body.Len())
	}
	// A cover tile (300 device px) gets the 360 px one, packaged as an
	// initialisation file and segments.
	w = get(c, strings.Replace(u, "/720.mp4", "/300.mp4", 1), nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), flattened(t, files["v360"])) {
		t.Fatalf("tile: %d (%d bytes) %s", w.Code, w.Body.Len(), w.Body.String())
	}
	// A web view that plays only HEVC asks for it; it is a file of its own.
	w = get(c, u+"?codec=hevc", nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), flattened(t, files["h768"])) {
		t.Fatalf("hevc: %d (%d bytes)", w.Code, w.Body.Len())
	}
	if _, err := os.Stat(c.path(key, 720, CodecHEVC)); err != nil {
		t.Errorf("hevc file: %v", err)
	}
	if s := c.Stats(); s.Files != 4 || s.Bytes == 0 || s.Last.Err != nil || s.Last.Codec != CodecHEVC ||
		s.Last.Rendition != "hvc1.2.20000000.L123.B0 768×768, 1.9 Mbit/s" || s.Last.Size != 720 || s.Last.Key != key {
		t.Errorf("stats = %+v", s)
	}

	// Loops and seeks read the disk copy, by range.
	before := hits.Load()
	w = get(c, small, http.Header{"Range": {"bytes=100-199"}})
	if w.Code != http.StatusPartialContent || w.Body.Len() != 100 {
		t.Errorf("range: %d, %d bytes", w.Code, w.Body.Len())
	}
	if hits.Load() != before {
		t.Errorf("a cached rendition went to the network (%d requests)", hits.Load()-before)
	}

	// Only handed-out keys are fetched, in a codec Sona knows.
	if w := get(c, "/motion/0123456789abcdef01234567/400.mp4", nil); w.Code != http.StatusNotFound {
		t.Errorf("unknown key: %d", w.Code)
	}
	for _, bad := range []string{"/motion/xyz/400.mp4", strings.Replace(u, "720.mp4", "4000.mp4", 1), strings.Replace(u, "720.mp4", "0400.mp4", 1), u + "?codec=vp9"} {
		if w := get(c, bad, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}

	// A failed download says why, to the web view and in Stats.
	hdr := c.URL(srv.URL + "/hdr/master.m3u8")
	if w := get(c, hdr, nil); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "no H.264 rendition") {
		t.Errorf("HDR only: %d %q", w.Code, w.Body.String())
	}
	if s := c.Stats(); s.Files != 4 || !errors.Is(s.Last.Err, ErrNoRendition) || s.Last.Rendition != "" {
		t.Errorf("stats after a failure = %+v", s)
	}
}

func TestEvict(t *testing.T) {
	srv, _, files := fakeCDN(t)
	n486, n768 := len(flattened(t, files["v486"])), len(flattened(t, files["v768"]))
	c := newTestCache(t, srv, int64(n486+n768-1))
	u := c.URL(srv.URL + "/v/master.m3u8")
	small := strings.Replace(u, "/720.mp4", "/400.mp4", 1)
	if w := get(c, small, nil); w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	if w := get(c, u, nil); w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	// Both files exceed the cap: the older one goes.
	names, _ := filepath.Glob(filepath.Join(c.dir, "*.mp4"))
	if len(names) != 1 || !strings.HasSuffix(names[0], "-720.mp4") {
		t.Errorf("after eviction: %v", names)
	}
	if _, err := os.Stat(c.path(strings.Split(u, "/")[2], 720, CodecH264)); err != nil {
		t.Error(err)
	}
}

// The file just downloaded is never the one evicted, even when an older file
// looks newer (equal or skewed times): the request it answers still reads it.
func TestEvictKeepsTheNewDownload(t *testing.T) {
	srv, _, files := fakeCDN(t)
	n486, n768 := len(flattened(t, files["v486"])), len(flattened(t, files["v768"]))
	c := newTestCache(t, srv, int64(n486+n768-1))
	u := c.URL(srv.URL + "/v/master.m3u8")
	small := strings.Replace(u, "/720.mp4", "/400.mp4", 1)
	if w := get(c, small, nil); w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	key := strings.Split(u, "/")[2]
	first, _ := filepath.Glob(filepath.Join(c.dir, "*.mp4"))
	if len(first) != 1 {
		t.Fatalf("after the first download: %v", first)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(first[0], future, future); err != nil {
		t.Fatal(err)
	}
	if w := get(c, u, nil); w.Code != http.StatusOK {
		t.Fatalf("new download: %d", w.Code)
	}
	if _, err := os.Stat(c.path(key, 720, CodecH264)); err != nil {
		t.Errorf("the new download was evicted: %v", err)
	}
	if _, err := os.Stat(first[0]); !os.IsNotExist(err) {
		t.Errorf("the other file stayed: %v", err)
	}
}

// A redirect from the allowed host is vetted like the first URL: neither
// another https host nor plain http is fetched.
func TestRedirectLeavingAllowList(t *testing.T) {
	var evilHits atomic.Int32
	evil := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evilHits.Add(1)
		_, _ = io.WriteString(w, master)
	})
	evilTLS := httptest.NewTLSServer(evil)
	t.Cleanup(evilTLS.Close)
	evilPlain := httptest.NewServer(evil)
	t.Cleanup(evilPlain.Close)
	for _, target := range []string{evilTLS.URL, evilPlain.URL} {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target+r.URL.Path, http.StatusFound)
		}))
		t.Cleanup(srv.Close)
		c := newTestCache(t, srv, 0)
		u := c.URL(srv.URL + "/v/master.m3u8")
		if w := get(c, u, nil); w.Code == http.StatusOK {
			t.Errorf("redirect to %s: served %d bytes", target, w.Body.Len())
		}
	}
	if n := evilHits.Load(); n != 0 {
		t.Errorf("redirect target fetched %d times", n)
	}
}

func TestAppleMedia(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://mvod.itunes.apple.com/a/master.m3u8":     true,
		"https://is1-ssl.mzstatic.com/a/v.mp4":            true,
		"https://mvod.itunes.apple.com:443/a/master.m3u8": true,
		"http://mvod.itunes.apple.com/a/master.m3u8":      false,
		"https://mvod.itunes.apple.com:8080/a":            false,
		"https://user:pw@mvod.itunes.apple.com/a":         false,
		"https://user@mvod.itunes.apple.com/a":            false,
		"https://apple.com.evil.example/a":                false,
		"https://127.0.0.1/a":                             false,
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := appleMedia(u); got != want {
			t.Errorf("appleMedia(%s) = %v, want %v", raw, got, want)
		}
	}
}
