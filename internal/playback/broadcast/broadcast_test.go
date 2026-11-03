package broadcast

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/id3/id3test"
)

type metaAt struct {
	at time.Duration
	m  playback.OnAir
}

// recorder is a Sink keeping what it gets. onAudio, if set, runs after
// each Audio with the bytes so far.
type recorder struct {
	mu      sync.Mutex
	audio   []byte
	metas   []metaAt
	onAudio func(total int)
}

func (r *recorder) Audio(_ context.Context, b []byte) error {
	r.mu.Lock()
	r.audio = append(r.audio, b...)
	n := len(r.audio)
	r.mu.Unlock()
	if r.onAudio != nil {
		r.onAudio(n)
	}
	return nil
}

func (r *recorder) Meta(at time.Duration, m playback.OnAir) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metas = append(r.metas, metaAt{at, m})
}

func (r *recorder) check(t *testing.T, audio []byte, metas []metaAt) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !bytes.Equal(r.audio, audio) {
		t.Errorf("audio: %d bytes, want %d", len(r.audio), len(audio))
	}
	if !slices.Equal(r.metas, metas) {
		t.Errorf("meta:\n got %v\nwant %v", r.metas, metas)
	}
}

// requests records the paths a test server was asked for.
type requests struct {
	mu    sync.Mutex
	paths []string
}

func (q *requests) add(p string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paths = append(q.paths, p)
}

func (q *requests) count(p string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, s := range q.paths {
		if s == p {
			n++
		}
	}
	return n
}

// play plays path on srv.
func play(ctx context.Context, srv *httptest.Server, path string, sink Sink) error {
	return Play(ctx, Options{HTTP: srv.Client(), UserAgent: "sona-test"}, srv.URL+path, sink)
}

func livePlaylist(first, last int, ended bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:%d\n", first)
	for n := first; n <= last; n++ {
		fmt.Fprintf(&b, "#EXTINF:2.0,\nseg%d.ts\n", n)
	}
	if ended {
		b.WriteString("#EXT-X-ENDLIST\n")
	}
	return b.String()
}

// A live HLS playlist of transport stream segments with timed ID3: play
// starts three segments from the live edge, follows the window as it
// slides and stops at ENDLIST.
func TestPlayHLSLiveTS(t *testing.T) {
	tags := map[int][]byte{12: onAirTag("Song 12", "Artist"), 14: onAirTag("Song 14", "Artist"), 15: onAirTag("Song 14", "Artist")}
	var (
		reqs     requests
		reloads  int
		reloadMu sync.Mutex
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.add(r.URL.Path)
		if r.URL.Path == "/live/index.m3u8" {
			reloadMu.Lock()
			n := reloads
			reloads++
			reloadMu.Unlock()
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			if n == 0 {
				_, _ = w.Write([]byte(livePlaylist(10, 14, false)))
			} else {
				_, _ = w.Write([]byte(livePlaylist(11, 15, true)))
			}
			return
		}
		var n int
		if _, err := fmt.Sscanf(r.URL.Path, "/live/seg%d.ts", &n); err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write(tsSegment(aacFrames(2, byte(n)), tags[n]))
	}))
	defer srv.Close()

	var rec recorder
	if err := play(context.Background(), srv, "/live/index.m3u8", &rec); err != nil {
		t.Fatal(err)
	}
	rec.check(t, slices.Concat(aacFrames(2, 12), aacFrames(2, 13), aacFrames(2, 14), aacFrames(2, 15)), []metaAt{
		{0, playback.OnAir{Title: "Song 12", Artist: "Artist"}},
		{4 * aacDur, playback.OnAir{Title: "Song 14", Artist: "Artist"}},
	})
	for n := 10; n <= 15; n++ {
		want := 1
		if n < 12 {
			want = 0
		}
		if got := reqs.count(fmt.Sprintf("/live/seg%d.ts", n)); got != want {
			t.Errorf("segment %d fetched %d times, want %d", n, got, want)
		}
	}
}

// A master playlist of packed AAC with ID3 tags and EXTINF titles: the
// AAC-LC variant plays from its first segment; segments gone from the
// server are skipped.
func TestPlayHLSPackedAAC(t *testing.T) {
	const master = `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=320000,CODECS="mp4a.40.5"
he/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=128000,CODECS="mp4a.40.2"
lc/index.m3u8
`
	const media = `#EXTM3U
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:100
#EXTINF:2,
s0.aac
#EXTINF:2,
s1.aac
#EXTINF:2,title="Title 2",artist="Artist 2"
s2.aac
#EXTINF:2,title="Title 2",artist="Artist 2"
s3.aac
#EXTINF:2,title="Gone"
s4.aac
#EXTINF:2,Station ident
s5.aac
#EXT-X-ENDLIST
`
	v22 := id3test.Tag(2, id3test.Frame(2, "TT2", id3test.Text("Title 5")))
	segments := map[string][]byte{
		"/lc/s0.aac": slices.Concat(timestampTag(), aacFrames(2, 0)),
		"/lc/s1.aac": slices.Concat(onAirTag("Title 1", "Artist 1"), aacFrames(2, 1)),
		"/lc/s2.aac": aacFrames(2, 2),
		"/lc/s3.aac": slices.Concat(timestampTag(), aacFrames(2, 3)),
		"/lc/s5.aac": slices.Concat(v22, aacFrames(2, 5)),
	}
	var reqs requests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.add(r.URL.Path)
		switch p := r.URL.Path; p {
		case "/master.m3u8":
			_, _ = w.Write([]byte(master)) // no playlist content type: sniffed
		case "/lc/index.m3u8":
			w.Header().Set("Content-Type", "audio/mpegurl")
			_, _ = w.Write([]byte(media))
		default:
			b, ok := segments[p]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "audio/aac")
			_, _ = w.Write(b)
		}
	}))
	defer srv.Close()

	var rec recorder
	if err := play(context.Background(), srv, "/master.m3u8", &rec); err != nil {
		t.Fatal(err)
	}
	rec.check(t, slices.Concat(aacFrames(2, 0), aacFrames(2, 1), aacFrames(2, 2), aacFrames(2, 3), aacFrames(2, 5)), []metaAt{
		{2 * aacDur, playback.OnAir{Title: "Title 1", Artist: "Artist 1"}},
		{4 * aacDur, playback.OnAir{Title: "Title 2", Artist: "Artist 2"}},
		{8 * aacDur, playback.OnAir{Title: "Title 5"}},
	})
	if reqs.count("/he/index.m3u8") != 0 || reqs.count("/lc/s4.aac") != 1 {
		t.Errorf("requests: %q", reqs.paths)
	}
}

func TestExtinfOnAir(t *testing.T) {
	for _, c := range []struct {
		title string
		want  playback.OnAir
		ok    bool
	}{
		{`title="Song",artist="Artist"`, playback.OnAir{Title: "Song", Artist: "Artist"}, true},
		{`offset=0,title=" Song ",artist=" "`, playback.OnAir{Title: "Song"}, true},
		{`artist="Only"`, playback.OnAir{Artist: "Only"}, true},
		{`title=""`, playback.OnAir{}, true},
		{"Station ident", playback.OnAir{}, false},
		{"", playback.OnAir{}, false},
		{"A = B", playback.OnAir{}, false},
		{`title="unterminated`, playback.OnAir{}, false},
	} {
		got, ok := extinfOnAir(c.title)
		if got != c.want || ok != c.ok {
			t.Errorf("extinfOnAir(%q) = %+v, %v; want %+v, %v", c.title, got, ok, c.want, c.ok)
		}
	}
}

func TestTagOnAir(t *testing.T) {
	for name, c := range map[string]struct {
		tag  []byte
		want playback.OnAir
		ok   bool
	}{
		"v2.4":          {onAirTag("Song", "Artist"), playback.OnAir{Title: "Song", Artist: "Artist"}, true},
		"v2.3 title":    {id3test.Tag(3, id3test.Frame(3, "TIT2", id3test.Text("Song"))), playback.OnAir{Title: "Song"}, true},
		"v2.2 artist":   {id3test.Tag(2, id3test.Frame(2, "TP1", id3test.Text("Artist"))), playback.OnAir{Artist: "Artist"}, true},
		"empty title":   {id3test.Tag(4, id3test.Frame(4, "TIT2", id3test.Text(""))), playback.OnAir{}, true},
		"timestamp":     {timestampTag(), playback.OnAir{}, false},
		"not a tag":     {aacFrames(1, 0), playback.OnAir{}, false},
		"truncated tag": {onAirTag("Song", "Artist")[:12], playback.OnAir{}, false},
	} {
		got, ok := tagOnAir(c.tag)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: %+v, %v; want %+v, %v", name, got, ok, c.want, c.ok)
		}
	}
}

// icyHandler serves an ICY stream of chunks (audio and metadata blocks)
// with icy-metaint, when the request asks for metadata.
func icyHandler(t *testing.T, metaint int, chunks ...[]byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Icy-MetaData") != "1" || r.Header.Get("User-Agent") != "sona-test" {
			t.Errorf("stream request headers: %v", r.Header)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("icy-metaint", strconv.Itoa(metaint))
		for _, c := range chunks {
			_, _ = w.Write(c)
			w.(http.Flusher).Flush()
		}
	}
}

// TuneIn's PLS: the station ident (an MP3 file starting with an ID3 tag),
// then the Icecast stream with ICY metadata.
func TestPlayPLSIcecast(t *testing.T) {
	const metaint = 2 * mp3Size
	ident := slices.Concat(id3test.Tag(3, id3test.Frame(3, "TIT2", id3test.Text("Ident"))), mp3Frames(3, 0x01))
	mux := http.NewServeMux()
	mux.HandleFunc("/tunein.pls", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-scpls")
		_, _ = fmt.Fprintf(w, "[playlist]\nNumberOfEntries=2\nFile1=/files/ident.mp3\nFile2=/stream\n")
	})
	mux.HandleFunc("/files/ident.mp3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(ident)
	})
	mux.HandleFunc("/stream", icyHandler(t, metaint,
		mp3Frames(2, 0x02), icyBlock("StreamTitle='Artist - Song';StreamUrl='';"),
		mp3Frames(2, 0x03), []byte{0},
		mp3Frames(2, 0x04), icyBlock("StreamTitle='';"),
		mp3Frames(1, 0x05),
	))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var rec recorder
	if err := play(context.Background(), srv, "/tunein.pls", &rec); err != nil {
		t.Fatal(err)
	}
	rec.check(t, slices.Concat(mp3Frames(3, 0x01), mp3Frames(2, 0x02), mp3Frames(2, 0x03), mp3Frames(2, 0x04), mp3Frames(1, 0x05)), []metaAt{
		{5 * mp3Dur, playback.OnAir{Title: "Song", Artist: "Artist"}},
		{9 * mp3Dur, playback.OnAir{}},
	})
}

// A playlist's entries are tried in turn: one that fails moves on to the
// next.
func TestPlayPlaylistFailover(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/list.m3u", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1,Station\n/missing\n/page\n%s/stream\n", "http://"+r.Host)
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	})
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil // no content type (and no sniffing)
		_, _ = w.Write(mp3Frames(2, 9))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var rec recorder
	if err := play(context.Background(), srv, "/list.m3u", &rec); err != nil {
		t.Fatal(err)
	}
	rec.check(t, mp3Frames(2, 9), nil)
}

func TestPlayErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html></html>"))
	})
	mux.HandleFunc("/empty.pls", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[playlist]\nNumberOfEntries=0\n"))
	})
	mux.HandleFunc("/loop.m3u", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		_, _ = w.Write([]byte("/loop.m3u\n"))
	})
	mux.HandleFunc("/video.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\nv.ts\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/v.ts", func(w http.ResponseWriter, r *http.Request) {
		seg := append(tsPSI(0, patSection()), tsPSI(pidPMT, pmtSection(esEntry{0x1b, pidVideo, nil}))...)
		_, _ = w.Write(seg)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for _, c := range []struct {
		path string
		want func(error) bool
	}{
		{"/page", func(err error) bool { return errors.Is(err, ErrUnsupported) }},
		{"/empty.pls", func(err error) bool { return err != nil && strings.Contains(err.Error(), "no entries") }},
		{"/loop.m3u", func(err error) bool { return err != nil && strings.Contains(err.Error(), "nested too deep") }},
		{"/video.m3u8", func(err error) bool { return err != nil && strings.Contains(err.Error(), "segment 0") }},
		{"/missing", func(err error) bool {
			var se *statusError
			return errors.As(err, &se) && se.code == http.StatusNotFound
		}},
	} {
		var rec recorder
		if err := play(context.Background(), srv, c.path, &rec); !c.want(err) {
			t.Errorf("%s: %v", c.path, err)
		}
	}
}

// A stream that drops is reopened; what it says on air is stamped with
// the stream time across connections. Play ends when its context does.
func TestPlayReconnect(t *testing.T) {
	const metaint = 2 * mp3Size
	var (
		mu    sync.Mutex
		conns int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		conns++
		n := conns
		mu.Unlock()
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("icy-metaint", strconv.Itoa(metaint))
		switch n {
		case 1:
			// Promise more than is sent: the connection drops.
			body := slices.Concat(mp3Frames(2, 1), icyBlock("StreamTitle='One';"), mp3Frames(2, 2))
			w.Header().Set("Content-Length", strconv.Itoa(len(body)+metaint))
			_, _ = w.Write(body)
		case 2:
			_, _ = w.Write(slices.Concat(mp3Frames(2, 3), icyBlock("StreamTitle='Two';"), mp3Frames(2, 4)))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			t.Errorf("connection %d", n)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	want := slices.Concat(mp3Frames(2, 1), mp3Frames(2, 2), mp3Frames(2, 3), mp3Frames(2, 4))
	rec := recorder{onAudio: func(n int) {
		if n >= len(want) {
			cancel()
		}
	}}
	done := make(chan error, 1)
	go func() { done <- play(ctx, srv, "/live", &rec) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Play: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Play did not end")
	}
	rec.check(t, want, []metaAt{
		{2 * mp3Dur, playback.OnAir{Title: "One"}},
		{6 * mp3Dur, playback.OnAir{Title: "Two"}},
	})
	mu.Lock()
	defer mu.Unlock()
	if conns != 2 {
		t.Errorf("%d connections", conns)
	}
}

// Planet Radio (Bauer) metadata names the artist as the title, and links
// its event on Planet Radio's listen API: on air puts the two the right
// way round; other metadata is left as it is.
func TestPlanetRadioTitleArtist(t *testing.T) {
	const event = "https://listenapi.planetradio.co.uk/api9.2/eventdata/425565949"
	swapped := playback.OnAir{Title: "Man I Need", Artist: "Olivia Dean"}
	for _, c := range []struct {
		extinf string
		want   playback.OnAir
	}{
		{`title="Olivia Dean",artist="Man I Need",url="` + event + `"`, swapped},
		{`title="Espresso",artist="Sabrina Carpenter",url="https://example.com/planetradio.co.uk/1"`, playback.OnAir{Title: "Espresso", Artist: "Sabrina Carpenter"}},
		{`title="Espresso",artist="Sabrina Carpenter",url="https://notplanetradio.co.uk/1"`, playback.OnAir{Title: "Espresso", Artist: "Sabrina Carpenter"}},
		{`title="Espresso",artist="Sabrina Carpenter"`, playback.OnAir{Title: "Espresso", Artist: "Sabrina Carpenter"}},
	} {
		if got, ok := extinfOnAir(c.extinf); !ok || got != c.want {
			t.Errorf("extinfOnAir(%s) = %+v, %v; want %+v", c.extinf, got, ok, c.want)
		}
	}
	wxxx := id3test.Frame(4, "WXXX", append([]byte{0}, "URL\x00"+event...))
	tag := id3test.Tag(4, id3test.Frame(4, "TIT2", id3test.Text("Olivia Dean")), id3test.Frame(4, "TPE1", id3test.Text("Man I Need")), wxxx)
	if got, ok := tagOnAir(tag); !ok || got != swapped {
		t.Errorf("tagOnAir(Planet Radio tag) = %+v, %v", got, ok)
	}
}
