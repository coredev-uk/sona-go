package motion

import (
	"bytes"
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
		want     string
	}{
		{100, "video_360.m3u8"},
		// The smallest resolution at least that wide, its best bit rate.
		{480, "video_486.m3u8"},
		{720, "video_768.m3u8"},
		// Nothing wide enough: the largest, under the bit-rate cap
		// (both 1080 renditions exceed it: the lower one).
		{2000, "video_1080.m3u8"},
	} {
		v, ok := choose(vs, tc.minWidth)
		if !ok || v.URI != tc.want {
			t.Errorf("choose(%d) = %q, %v; want %q", tc.minWidth, v.URI, ok, tc.want)
		}
	}
	if _, ok := choose(parseMaster("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,CODECS=\"hvc1.2\",RESOLUTION=768x768\nhevc.m3u8\n"), 480); ok {
		t.Error("chose a rendition from an HEVC-only master")
	}
}

func TestParseMedia(t *testing.T) {
	base, _ := url.Parse("https://mvod.itunes.apple.com/a/b/video.m3u8")
	single := "#EXTM3U\n#EXT-X-MAP:URI=\"v.mp4\",BYTERANGE=\"895@0\"\n#EXTINF:2.8,\n#EXT-X-BYTERANGE:622920@895\nv.mp4\n#EXTINF:2.8,\n#EXT-X-BYTERANGE:717742@623815\nv.mp4\n#EXT-X-ENDLIST\n"
	u, err := parseMedia(single, base)
	if err != nil || u.String() != "https://mvod.itunes.apple.com/a/b/v.mp4" {
		t.Errorf("single file = %v, %v", u, err)
	}
	if _, err := parseMedia("#EXTM3U\n#EXTINF:2,\nseg0.mp4\n#EXTINF:2,\nseg1.mp4\n", base); err == nil {
		t.Error("accepted a playlist of separate segment files")
	}
	if _, err := parseMedia("#EXTM3U\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"skd://k\"\n#EXTINF:2,\nv.mp4\n", base); err == nil {
		t.Error("accepted an encrypted playlist")
	}
}

// fakeCDN serves master.m3u8 → video_486.m3u8 / video_768.m3u8 → one
// fragmented MP4 each, counting requests.
func fakeCDN(t *testing.T) (*httptest.Server, *atomic.Int32, map[string][]byte) {
	file := func(gops int) []byte {
		return testFile{movieScale: 30, scale: 30, base: 300, trunVersion: 1, frags: slices.Repeat([][]frame{gop}, gops)}.build()
	}
	files := map[string][]byte{
		"/v/v486.mp4": file(2),
		"/v/v768.mp4": file(5),
	}
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/v/master.m3u8":
			_, _ = io.WriteString(w, master)
		case "/v/video_486.m3u8", "/v/video_768.m3u8":
			name := "v486.mp4"
			if strings.Contains(r.URL.Path, "768") {
				name = "v768.mp4"
			}
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-MAP:URI=\""+name+"\",BYTERANGE=\"100@0\"\n#EXTINF:2,\n#EXT-X-BYTERANGE:400@100\n"+name+"\n")
		default:
			b, ok := files[r.URL.Path]
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

	// A header cover (400 device px) gets the 486 px rendition, remuxed.
	small := strings.Replace(u, "/720.mp4", "/400.mp4", 1)
	w := get(c, small, nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), flattened(t, files["/v/v486.mp4"])) || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("small: %d %q (%d bytes)", w.Code, w.Header().Get("Content-Type"), w.Body.Len())
	}
	// The now-playing cover gets the 768 px one.
	w = get(c, u, nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), flattened(t, files["/v/v768.mp4"])) {
		t.Fatalf("large: %d (%d bytes)", w.Code, w.Body.Len())
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

	// Only handed-out keys are fetched.
	if w := get(c, "/motion/0123456789abcdef01234567/400.mp4", nil); w.Code != http.StatusNotFound {
		t.Errorf("unknown key: %d", w.Code)
	}
	for _, bad := range []string{"/motion/xyz/400.mp4", strings.Replace(u, "720.mp4", "4000.mp4", 1), strings.Replace(u, "720.mp4", "0400.mp4", 1)} {
		if w := get(c, bad, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
}

func TestEvict(t *testing.T) {
	srv, _, files := fakeCDN(t)
	n486, n768 := len(flattened(t, files["/v/v486.mp4"])), len(flattened(t, files["/v/v768.mp4"]))
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
	if _, err := os.Stat(c.path(strings.Split(u, "/")[2], 720)); err != nil {
		t.Error(err)
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
