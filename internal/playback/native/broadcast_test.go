//go:build linux || (darwin && cgo)

package native

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

// broadcastServer is Apple's catalog and /v1/play/assets for the
// broadcaster station "ra.bcast" (hasDrm false, a clear stream URL without
// a key server), the DRM station "ra.tracks" that is not a stream, and
// the broadcaster's Icecast stream: ADTS AAC with an ICY StreamTitle after
// the first metaint bytes, held open until the client goes.
type broadcastServer struct {
	srv  *httptest.Server
	head []byte // the stream's audio before the title
	tail []byte // after it

	// silent: the stream names nothing on air (an empty StreamTitle).
	silent bool
	// describe is TuneIn's Describe.ashx answer for the station "s100001".
	describe string

	mu    sync.Mutex
	paths []string
	open  int // stream connections still open
}

// onAirTitle is the stream's ICY StreamTitle.
const onAirTitle = "Artist - Song"

// adtsFrames returns n ADTS AAC-LC 44.1 kHz stereo frames of 16 bytes
// (1024 samples each).
func adtsFrames(n int) []byte {
	const size = 16
	f := []byte{0xff, 0xf1, 1<<6 | 4<<2, 2<<6 | size>>11, size >> 3 & 0xff, size&7<<5 | 0x1f, 0xfc}
	f = append(f, make([]byte, size-len(f))...)
	return bytes.Repeat(f, n)
}

// adtsDuration is the stream time of n adtsFrames.
func adtsDuration(n int) time.Duration { return time.Duration(n) * 1024 * time.Second / 44100 }

func newBroadcastServer(t *testing.T, headFrames int) *broadcastServer {
	// The tail takes the body past the 512 bytes Play sniffs before it
	// plays anything.
	bs := &broadcastServer{head: adtsFrames(headFrames), tail: adtsFrames(40)}
	bs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bs.mu.Lock()
		bs.paths = append(bs.paths, r.URL.Path)
		bs.mu.Unlock()
		switch r.URL.Path {
		case "/v1/catalog/gb/stations":
			var out []string
			for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
				switch id {
				case "ra.bcast":
					out = append(out, `{"id":"ra.bcast","type":"stations","attributes":{"name":"Local FM","playParams":{"id":"ra.bcast","kind":"radioStation","format":"stream","hasDrm":false,"mediaType":0,"streamingKind":2}}}`)
				case "ra.tunein":
					out = append(out, `{"id":"ra.tunein","type":"stations","attributes":{"name":"Tune FM","stationProviderName":"TuneIn","playParams":{"id":"ra.tunein","kind":"radioStation","format":"stream","hasDrm":false,"mediaType":0,"streamingKind":2}}}`)
				case "ra.tracks":
					out = append(out, `{"id":"ra.tracks","type":"stations","attributes":{"name":"Mix","playParams":{"id":"ra.tracks","kind":"radioStation","format":"tracks"}}}`)
				}
			}
			_, _ = fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(out, ","))
		case "/v1/play/assets":
			if r.URL.Query().Get("hasDrm") != "false" {
				http.NotFound(w, r)
				return
			}
			switch r.URL.Query().Get("id") {
			case "ra.bcast":
				_, _ = fmt.Fprint(w, `{"results":{"assets":[{"url":"https://stream.local.test/localfm"}]}}`)
			case "ra.tunein":
				// TuneIn's playlist, naming its station in the ad parameters (which
				// TuneIn escapes a second time).
				u := url.URL{Scheme: "https", Host: "pls.tunein.com", Path: "/pre/v1/playlist.pls", RawQuery: url.Values{
					"ti_ad_params": {url.QueryEscape("aw_0_1st.bundleId=tunein.com&aw_0_1st.stationId=s100001")},
					"url":          {"https://stream.local.test/localfm"},
				}.Encode()}
				_, _ = fmt.Fprintf(w, `{"results":{"assets":[{"url":%q}]}}`, u.String())
			default:
				http.NotFound(w, r)
			}
		case "/pre/v1/playlist.pls":
			w.Header().Set("Content-Type", "audio/x-scpls")
			_, _ = fmt.Fprint(w, "[playlist]\nNumberOfEntries=1\nFile1=https://stream.local.test/localfm\n")
		case "/Describe.ashx":
			_, _ = fmt.Fprint(w, bs.describe)
		case "/v1/catalog/gb/search":
			// One hit that is the on-air song, one that only looks like it.
			_, _ = fmt.Fprint(w, `{"results":{"songs":{"data":[
				{"id":"1","type":"songs","attributes":{"name":"Song (Radio Edit)","artistName":"Artist","artwork":{"url":"https://is1-ssl.mzstatic.com/cover/{w}x{h}bb.jpg","width":600,"height":600}}}]}}}`)
		case "/localfm":
			bs.mu.Lock()
			bs.open++
			bs.mu.Unlock()
			defer func() {
				bs.mu.Lock()
				bs.open--
				bs.mu.Unlock()
			}()
			title := onAirTitle
			if bs.silent {
				title = ""
			}
			meta := []byte("StreamTitle='" + title + "';")
			meta = append(meta, make([]byte, (16-len(meta)%16)%16)...)
			w.Header().Set("Content-Type", "audio/aac")
			w.Header().Set("icy-metaint", strconv.Itoa(len(bs.head)))
			// A metadata block after every metaint audio bytes: the
			// title after the head, then empty ones (unchanged).
			body := append(append(append([]byte(nil), bs.head...), byte(len(meta)/16)), meta...)
			for rest := bs.tail; len(rest) > 0; {
				n := min(len(rest), len(bs.head))
				body = append(body, rest[:n]...)
				if rest = rest[n:]; len(rest) > 0 {
					body = append(body, 0)
				}
			}
			_, _ = w.Write(body)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(bs.srv.Close)
	return bs
}

// requests counts the requests for path so far.
func (bs *broadcastServer) requests(path string) int {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	n := 0
	for _, p := range bs.paths {
		if p == path {
			n++
		}
	}
	return n
}

func (bs *broadcastServer) requested() []string {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return append([]string(nil), bs.paths...)
}

func (bs *broadcastServer) connections() int {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.open
}

// newBroadcastHarness is an engine over bs, in storefront "gb", with a CDM
// ready (broadcasts must not use it).
func newBroadcastHarness(t *testing.T, bs *broadcastServer) *harness {
	hc := &http.Client{Transport: rewrite{bs.srv}}
	api := applemusic.New(hc, dev{}, func() string { return "mut" })
	h := &harness{cdm: &fakeCDM{lib: testCDM}}
	h.e = New(Options{Client: api, HTTP: hc, IdleTimeout: time.Minute, CDM: h.cdm, Storefront: func() string { return "gb" }})
	h.e.retry = []time.Duration{time.Millisecond}
	h.e.spawn = func(_ context.Context, lib string) (host, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		f := newFake(100 + len(h.hosts))
		h.hosts = append(h.hosts, f)
		h.libs = append(h.libs, lib)
		return f, nil
	}
	t.Cleanup(func() { _ = h.e.Close() })
	return h
}

// waitFed waits until the fake host's current file holds n bytes.
func waitFed(t *testing.T, f *fakeHost, n int) fakeAudio {
	t.Helper()
	var a fakeAudio
	eventually(t, fmt.Sprintf("%d bytes fed", n), func() bool {
		a = f.audioSnapshot()
		return len(a.file) >= n
	})
	return a
}

// A broadcaster's station is looked up in the catalog, its clear asset
// asked for without DRM, and its stream fed to the host as a live file:
// no licence, no CDM session, no certificate, no HLS load.
func TestBroadcastLoadsLiveWithoutLicence(t *testing.T) {
	bs := newBroadcastServer(t, 4)
	h := newBroadcastHarness(t, bs)
	if err := h.e.Play(ctx(t), "ra.bcast", 0); err != nil {
		t.Fatal(err)
	}
	if st := h.e.Status(); st.State != playback.StatePlaying || st.ItemID != "ra.bcast" || st.Duration != 0 {
		t.Errorf("status %+v, want ra.bcast playing without a duration", st)
	}
	f := h.host(0)
	want := len(bs.head) + len(bs.tail) // the audio without the ICY block
	a := waitFed(t, f, want)
	if len(a.fileLoads) != 1 || a.fileLoads[0].size != gst.Live || a.fileLoads[0].start != 0 {
		t.Fatalf("file loads %+v, want one live load from 0", a.fileLoads)
	}
	if wantAudio := append(append([]byte(nil), bs.head...), bs.tail...); !bytes.Equal(a.file, wantAudio) {
		t.Errorf("host got %d bytes, want the stream's %d bytes of audio", len(a.file), len(wantAudio))
	}
	loads, _, sessions := f.snapshot()
	f.mu.Lock()
	certs := f.certs
	f.mu.Unlock()
	if len(loads) != 0 || sessions != 0 || certs != 0 {
		t.Errorf("host got %d HLS loads, %d CDM sessions, %d certificates", len(loads), sessions, certs)
	}
	for _, p := range bs.requested() {
		switch p {
		case "/v1/catalog/gb/stations", "/v1/play/assets", "/localfm":
		case strings.TrimPrefix(applemusic.WidevineCertURL, "https://play.itunes.apple.com"):
			// prefetched for whatever plays next; never installed
		default:
			if strings.Contains(p, "lic") || strings.Contains(p, "key") {
				t.Errorf("broadcast requested %s", p)
			}
		}
	}
	if bs.requests("/v1/catalog/gb/stations") != 1 || bs.requests("/v1/play/assets") != 1 || bs.requests("/localfm") != 1 {
		t.Errorf("requests %v, want the station, its asset and its stream once each", bs.requested())
	}
}

// A station that is not a stream station is not played as one.
func TestBroadcastNotAStreamStation(t *testing.T) {
	bs := newBroadcastServer(t, 4)
	h := newBroadcastHarness(t, bs)
	for _, id := range []string{"ra.tracks", "ra.missing"} {
		if err := h.e.Play(ctx(t), id, 0); !errors.Is(err, applemusic.ErrNotPlayable) {
			t.Errorf("Play(%s) = %v, want ErrNotPlayable", id, err)
		}
	}
	if n := bs.requests("/v1/play/assets"); n != 0 {
		t.Errorf("%d asset requests for stations that are not streams", n)
	}
}

// Pausing a broadcast lets the stream go; Resume joins it live again with
// a new connection and a new live load. Seeking does nothing.
func TestBroadcastPauseRejoinsAndSeekIsANoOp(t *testing.T) {
	bs := newBroadcastServer(t, 4)
	h := newBroadcastHarness(t, bs)
	if err := h.e.Play(ctx(t), "ra.bcast", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	waitFed(t, f, len(bs.head)+len(bs.tail))

	if err := h.e.Seek(ctx(t), 30*time.Second); err != nil {
		t.Fatalf("Seek = %v, want a no-op", err)
	}
	if a := f.audioSnapshot(); len(a.fileLoads) != 1 {
		t.Errorf("seek reloaded: %+v", a.fileLoads)
	}
	if st := h.e.Status(); st.State != playback.StatePlaying || st.Position > 10*time.Second {
		t.Errorf("after seek: %+v", st)
	}

	if err := h.e.Pause(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if st := h.e.Status(); st.State != playback.StatePaused {
		t.Errorf("after pause: %+v", st)
	}
	eventually(t, "stream released on pause", func() bool { return bs.connections() == 0 })

	if err := h.e.Resume(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if st := h.e.Status(); st.State != playback.StatePlaying || st.ItemID != "ra.bcast" {
		t.Errorf("after resume: %+v", st)
	}
	if n := bs.requests("/localfm"); n != 2 {
		t.Errorf("%d stream requests, want a new one on resume", n)
	}
	a := waitFed(t, f, len(bs.head)+len(bs.tail))
	if len(a.fileLoads) != 2 || a.fileLoads[1].size != gst.Live || a.fileLoads[1].start != 0 || a.fileLoads[1].track == a.fileLoads[0].track {
		t.Errorf("file loads %+v, want a second live load as a new track", a.fileLoads)
	}
	if h.nhosts() != 1 {
		t.Errorf("%d hosts, want the one", h.nhosts())
	}
}

// What the stream says is on air becomes Status.OnAir only once playback
// reaches the stream time it was sent at, with an EventState.
func TestBroadcastOnAirWhenHeard(t *testing.T) {
	const frames = 35 // ≈ 0.81 s before the title
	at := adtsDuration(frames)
	bs := newBroadcastServer(t, frames)
	h := newBroadcastHarness(t, bs)
	if err := h.e.Play(ctx(t), "ra.bcast", 0); err != nil {
		t.Fatal(err)
	}
	drainEvents(h.e)
	f := h.host(0)
	waitFed(t, f, len(bs.head)+len(bs.tail)) // the title has been read
	if st := h.e.Status(); st.OnAir != (playback.OnAir{}) && heard(st) < at {
		t.Fatalf("on air %+v at %v, before its stream time %v", st.OnAir, heard(st), at)
	}

	var got playback.Event
	deadline := time.After(5 * time.Second)
	for got.Status.OnAir == (playback.OnAir{}) {
		if st := h.e.Status(); st.OnAir != (playback.OnAir{}) && heard(st) < at {
			t.Fatalf("on air %+v at %v, before its stream time %v", st.OnAir, heard(st), at)
		}
		select {
		case ev := <-h.e.Events():
			if ev.Status.OnAir != (playback.OnAir{}) {
				if ev.Kind != playback.EventState {
					t.Fatalf("on air arrived with %v, want EventState", ev.Kind)
				}
				got = ev
			}
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			t.Fatal("on air never announced")
		}
	}
	if want := (playback.OnAir{Title: "Song", Artist: "Artist"}); !sameSong(got.Status.OnAir, want) {
		t.Errorf("OnAir = %+v, want %+v", got.Status.OnAir, want)
	}
	if p := heard(got.Status); p < at {
		t.Errorf("announced at position %v, before its stream time %v", p, at)
	}
	if st := h.e.Status(); st.OnAir.Title != "Song" {
		t.Errorf("status %+v lost what is on air", st)
	}
	// The catalog's cover for that song arrives with a second state event;
	// the search names only the song, and nothing else is asked of Apple.
	const cover = "https://is1-ssl.mzstatic.com/cover/{w}x{h}bb.jpg"
	eventually(t, "the song's cover", func() bool { return h.e.Status().OnAir.Art == cover })
	if n := bs.requests("/v1/catalog/gb/search"); n != 1 {
		t.Errorf("%d catalog searches, want one", n)
	}
	if n := bs.requests("/Describe.ashx"); n != 0 {
		t.Errorf("%d TuneIn requests for a station that is not TuneIn's", n)
	}
}

const tuneInProfile = `{"head":{"status":"200"},"body":[{"element":"station","guide_id":"s100001","name":"Tune FM","has_song":%t,"current_song":"Tune Song","current_artist":"Tune Artist","genre_name":"Pop","location":"Exampletown","description":"Hits.","url":"https://www.example.com/"}]}`

// A TuneIn station's profile is read once for what Now Playing shows about
// it; the song TuneIn names stands in only while the stream names none.
func TestBroadcastTuneIn(t *testing.T) {
	info := playback.StationInfo{Description: "Hits.", Genre: "Pop", Location: "Exampletown", Website: "https://www.example.com/"}

	t.Run("the stream names nothing", func(t *testing.T) {
		bs := newBroadcastServer(t, 4)
		bs.silent, bs.describe = true, fmt.Sprintf(tuneInProfile, true)
		h := newBroadcastHarness(t, bs)
		if err := h.e.Play(ctx(t), "ra.tunein", 0); err != nil {
			t.Fatal(err)
		}
		eventually(t, "TuneIn's song on air", func() bool {
			st := h.e.Status()
			return st.OnAir.Title == "Tune Song" && st.OnAir.Artist == "Tune Artist" && st.Info == info
		})
		if n := bs.requests("/Describe.ashx"); n != 1 {
			t.Errorf("%d TuneIn requests, want one", n)
		}
	})

	t.Run("the stream names the song", func(t *testing.T) {
		bs := newBroadcastServer(t, 4)
		bs.describe = fmt.Sprintf(tuneInProfile, true)
		h := newBroadcastHarness(t, bs)
		if err := h.e.Play(ctx(t), "ra.tunein", 0); err != nil {
			t.Fatal(err)
		}
		eventually(t, "the profile", func() bool { return h.e.Status().Info == info })
		eventually(t, "the stream's song on air", func() bool { return h.e.Status().OnAir.Title == "Song" })
		// TuneIn's song never replaces the stream's.
		time.Sleep(50 * time.Millisecond)
		if st := h.e.Status(); st.OnAir.Title != "Song" || st.OnAir.Artist != "Artist" {
			t.Errorf("on air %+v, want the stream's song", st.OnAir)
		}
	})

	t.Run("a station TuneIn does not know", func(t *testing.T) {
		bs := newBroadcastServer(t, 4)
		bs.silent, bs.describe = true, `{"head":{"status":"400","fault":"Station not found"},"body":[]}`
		h := newBroadcastHarness(t, bs)
		if err := h.e.Play(ctx(t), "ra.tunein", 0); err != nil {
			t.Fatal(err)
		}
		eventually(t, "the profile asked for", func() bool { return bs.requests("/Describe.ashx") == 1 })
		waitFed(t, h.host(0), len(bs.head)+len(bs.tail))
		if st := h.e.Status(); st.OnAir != (playback.OnAir{}) || st.Info != (playback.StationInfo{}) {
			t.Errorf("status %+v, want nothing on air", st)
		}
		if n := bs.requests("/Describe.ashx"); n != 1 {
			t.Errorf("%d TuneIn requests, want one (an unknown station is not retried)", n)
		}
	})
}

func TestMatchArt(t *testing.T) {
	cands := []artCandidate{
		{"Song (Live)", "Other Band", "other"},
		{"Song", "Artist", ""}, // no cover
		{"SONG (Radio Edit)", "Artist & Friend", "match"},
		{"Song", "Artist", "later"},
	}
	for _, c := range []struct {
		name string
		m    playback.OnAir
		want string
	}{
		{"title and artist match, bracketed suffix ignored", playback.OnAir{Artist: "Artist", Title: "Song"}, "match"},
		{"no artist named", playback.OnAir{Title: "Song"}, ""},
		{"wrong artist", playback.OnAir{Artist: "Nobody", Title: "Song"}, ""},
		{"wrong title", playback.OnAir{Artist: "Artist", Title: "Other Song"}, ""},
		{"short artist is not a substring match", playback.OnAir{Artist: "Art", Title: "Song"}, ""},
		{"punctuation and case", playback.OnAir{Artist: "ARTIST!", Title: "song."}, "match"},
	} {
		if got := matchArt(c.m, cands); got != c.want {
			t.Errorf("%s: matchArt = %q, want %q", c.name, got, c.want)
		}
	}
}

// heard is the position st was playing at when it was reported.
func heard(st playback.Status) time.Duration {
	if st.State != playback.StatePlaying {
		return st.Position
	}
	return st.Position + time.Since(st.UpdatedAt)
}
