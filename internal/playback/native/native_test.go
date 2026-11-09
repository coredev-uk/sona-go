//go:build linux || (cgo && (darwin || windows))

package native

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
)

// fakeHost stands in for the CDM host process: it records calls and emits
// the events a real child would.
type fakeHost struct {
	pid    int
	mu     sync.Mutex
	events chan cdmhost.Event
	done   chan struct{}
	once   sync.Once
	err    error

	certs    int
	sessions map[string][]byte // session → licence
	loads    []loadCall
	nexts    []nextCall
	next     uint64 // queued with Next
	feeds    []feedCall
	track    uint64
	start    time.Duration
	playing  bool
	fed      bool
	started  bool
	autoEnd  bool            // emit EventEnded on EndOfTrack
	endGate  <-chan struct{} // when set, autoEnd waits for it to close
	audio    fakeAudio       // volume, equaliser, device and file calls (audio_test.go)
	video    fakeVideo       // LoadVideo, FeedVideo, EndOfVideo (video_test.go)
}

type loadCall struct {
	track uint64
	init  []byte
	start time.Duration
	dur   time.Duration
}

type nextCall struct {
	track uint64
	dur   time.Duration
	fade  time.Duration
}

func (f *fakeHost) Next(_ context.Context, track uint64, m cdmhost.Media, fade time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nexts = append(f.nexts, nextCall{track, m.Duration, fade})
	f.next = track
	return f.alive()
}

type feedCall struct {
	track uint64
	data  []byte
}

func newFake(pid int) *fakeHost {
	return &fakeHost{pid: pid, events: make(chan cdmhost.Event, 256), done: make(chan struct{}), sessions: map[string][]byte{}}
}

func (f *fakeHost) Info() cdmhost.Info           { return cdmhost.Info{PID: f.pid} }
func (f *fakeHost) Events() <-chan cdmhost.Event { return f.events }
func (f *fakeHost) Done() <-chan struct{}        { return f.done }
func (f *fakeHost) Err() error                   { f.mu.Lock(); defer f.mu.Unlock(); return f.err }

func (f *fakeHost) exit(err error) {
	f.once.Do(func() {
		f.mu.Lock()
		f.err = err
		f.mu.Unlock()
		close(f.events)
		close(f.done)
	})
}

func (f *fakeHost) alive() error {
	select {
	case <-f.done:
		return cdmhost.ErrExited
	default:
		return nil
	}
}

func (f *fakeHost) emit(ev cdmhost.Event) {
	if f.alive() == nil {
		f.events <- ev
	}
}

func (f *fakeHost) SetServerCertificate(context.Context, []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.certs++
	return f.alive()
}

func (f *fakeHost) CreateSession(_ context.Context, kid []byte) (string, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sid := fmt.Sprintf("s%d", len(f.sessions)+1)
	f.sessions[sid] = nil
	return sid, append([]byte("challenge:"), kid...), f.alive()
}

func (f *fakeHost) UpdateSession(_ context.Context, sid string, lic []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if string(lic) != "licence" {
		return cdmhost.ErrNoUsableKey
	}
	f.sessions[sid] = lic
	return f.alive()
}

func (f *fakeHost) CloseSession(_ context.Context, sid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, sid)
	return nil
}

func (f *fakeHost) Load(_ context.Context, track uint64, m cdmhost.Media, start time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audio.gains = append(f.audio.gains, m.Gain)
	f.loads = append(f.loads, loadCall{track, m.Init, start, m.Duration})
	f.next = 0
	f.track, f.start, f.fed, f.started = track, start, false, false
	return f.alive()
}

func (f *fakeHost) Feed(_ context.Context, track uint64, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.alive(); err != nil {
		return err
	}
	if track != f.track && (track != f.next || track == 0) {
		return cdmhost.ErrStale
	}
	f.feeds = append(f.feeds, feedCall{track, data})
	f.fed = true
	f.maybeStart()
	return nil
}

func (f *fakeHost) FeedStream(ctx context.Context, track uint64, s cdmhost.StreamFragment) error {
	return f.Feed(ctx, track, s.Data)
}

// maybeStart emits EventStarted once loaded data is playing; with mu held.
func (f *fakeHost) maybeStart() {
	if f.playing && f.fed && !f.started {
		f.started = true
		if d := f.audio.fileDur; d > 0 && f.start >= d {
			// An upload started past its end ends at once, as in the child.
			f.emit(cdmhost.Event{Kind: cdmhost.EventEnded, Track: f.track, Position: d, At: time.Now()})
			return
		}
		f.emit(cdmhost.Event{Kind: cdmhost.EventStarted, Track: f.track, Position: f.start, At: time.Now(), Duration: f.audio.fileDur})
	}
}

func (f *fakeHost) EndOfTrack(_ context.Context, track uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if track == f.next && track != 0 {
		return f.alive()
	}
	if track != f.track {
		return cdmhost.ErrStale
	}
	if f.autoEnd {
		if f.endGate == nil {
			f.emit(cdmhost.Event{Kind: cdmhost.EventEnded, Track: track, At: time.Now()})
		} else {
			go f.endAfter(f.endGate, track)
		}
	}
	return f.alive()
}

// endAfter ends track once gate closes, as a real track plays out first.
func (f *fakeHost) endAfter(gate <-chan struct{}, track uint64) {
	select {
	case <-gate:
	case <-f.done:
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emit(cdmhost.Event{Kind: cdmhost.EventEnded, Track: track, At: time.Now()})
}

func (f *fakeHost) Play(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing, f.started = true, false
	f.maybeStart()
	return f.alive()
}

func (f *fakeHost) Pause(context.Context) (time.Duration, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = false
	return f.start + 500*time.Millisecond, true, f.alive()
}

func (f *fakeHost) Stop(context.Context, uint64) error { return f.alive() }
func (f *fakeHost) SetVolume(_ context.Context, v float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audio.volumes = append(f.audio.volumes, v)
	return f.alive()
}
func (f *fakeHost) Close() error { f.exit(cdmhost.ErrExited); return nil }

func (f *fakeHost) LoadVideo(_ context.Context, track uint64, m cdmhost.VideoMedia, start time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.video.loads = append(f.video.loads, videoLoad{track: track, init: m.Init, kid: m.KID, shift: m.Shift, start: start})
	return f.alive()
}

func (f *fakeHost) FeedVideo(_ context.Context, track uint64, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.video.feeds = append(f.video.feeds, feedCall{track: track, data: data})
	return f.alive()
}

func (f *fakeHost) EndOfVideo(_ context.Context, track uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.video.ends = append(f.video.ends, track)
	return f.alive()
}

func (f *fakeHost) snapshot() (loads []loadCall, feeds []feedCall, sessions int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]loadCall(nil), f.loads...), append([]feedCall(nil), f.feeds...), len(f.sessions)
}

// fixture serves webPlayback, the certificate, licences, playlists and
// media for songs "A", "B" (and "403", which Apple refuses), and music
// video videoTestID (videoFiles, video_test.go).
type fixture struct {
	srv   *httptest.Server
	mu    sync.Mutex
	count map[string]int // "path range" → requests
}

// initSeg is a minimal AAC-LC 44.1 kHz stereo init segment.
var initSeg = func() []byte {
	box := func(typ string, parts ...[]byte) []byte {
		body := bytes.Join(parts, nil)
		return append(binary.BigEndian.AppendUint32(nil, uint32(8+len(body))), append([]byte(typ), body...)...)
	}
	u32 := func(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
	u16 := func(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }
	d := func(tag byte, body []byte) []byte { return append([]byte{tag, byte(len(body))}, body...) }
	dcd := d(0x04, append([]byte{0x40, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, d(0x05, []byte{0x12, 0x10})...))
	esds := box("esds", u32(0), d(0x03, append([]byte{0, 0, 0}, dcd...)))
	entry := box("mp4a", make([]byte, 6), u16(1), make([]byte, 8), u16(2), u16(16), u32(0), u32(44100<<16), esds)
	stsd := box("stsd", u32(0), u32(1), entry)
	mdhd := box("mdhd", u32(0), u32(0), u32(0), u32(44100), u32(0), u32(0))
	tkhd := box("tkhd", u32(7), u32(0), u32(0), u32(1), make([]byte, 68))
	trak := box("trak", tkhd, box("mdia", mdhd, box("minf", box("stbl", stsd))))
	return append(box("ftyp", []byte("iso8"), u32(0)), box("moov", trak)...)
}()

// mediaBytes is the fixture's media file: the init segment, then three
// 100-byte segments.
var mediaBytes = func() []byte {
	b := make([]byte, 300)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return append(append([]byte(nil), initSeg...), b...)
}()

var initLen = len(initSeg)

const kidB64 = "AAAAAEIk6OAAHbBdgj4o5Q=="

func newFixture(t *testing.T) *fixture {
	fx := &fixture{count: map[string]int{}}
	fx.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fx.mu.Lock()
		fx.count[r.URL.Path+" "+r.Header.Get("Range")]++
		fx.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/webPlayback"):
			var body struct {
				SalableAdamID string `json:"salableAdamId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			switch body.SalableAdamID {
			case "403":
				w.WriteHeader(http.StatusForbidden)
				return
			case videoTestID:
				_, _ = fmt.Fprint(w, `{"songList":[{"songId":"`+videoTestID+`","hls-key-server-url":"https://play.test/lic","widevine-cert-url":"https://play.test/cert",
					"hls-playlist-url":"https://play.test/mv/master.m3u8?dsid=1"}]}`)
				return
			}
			fmt.Fprintf(w, `{"songList":[{"songId":"%s","hls-key-server-url":"https://play.test/lic","widevine-cert-url":"https://play.test/cert",
				"assets":[{"flavor":"28:ctrp256","URL":"https://aod.test/%s/list.m3u8"},{"flavor":"32:ctrp64","URL":"https://aod.test/%s/64/list.m3u8"}]}]}`,
				body.SalableAdamID, body.SalableAdamID, body.SalableAdamID)
		case r.URL.Path == "/cert":
			w.Write([]byte("cert"))
		case r.URL.Path == "/lic":
			fmt.Fprintf(w, `{"status":0,"license":"%s"}`, base64.StdEncoding.EncodeToString([]byte("licence")))
		case strings.HasSuffix(r.URL.Path, "/list.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-KEY:METHOD=ISO-23001-7,URI=\"data:;base64,%s\"\n"+
				"#EXT-X-MAP:URI=\"media.mp4\",BYTERANGE=\"%d@0\"\n"+
				"#EXTINF:10,\n#EXT-X-BYTERANGE:100@%d\nmedia.mp4\n#EXTINF:10,\n#EXT-X-BYTERANGE:100\nmedia.mp4\n#EXTINF:10,\n#EXT-X-BYTERANGE:100\nmedia.mp4\n#EXT-X-ENDLIST\n", kidB64, initLen, initLen)
		case strings.HasSuffix(r.URL.Path, "/media.mp4"):
			http.ServeContent(w, r, "media.mp4", time.Time{}, bytes.NewReader(mediaBytes))
		default:
			if b, ok := videoFiles[r.URL.Path]; ok {
				_, _ = w.Write(b)
				return
			}
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fx.srv.Close)
	return fx
}

func (fx *fixture) requests(path, rng string) int {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	return fx.count[path+" "+rng]
}

func (fx *fixture) total() int {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	n := 0
	for _, c := range fx.count {
		n += c
	}
	return n
}

// rewrite routes every request to the fixture, keeping the path.
type rewrite struct{ srv *httptest.Server }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(r.srv.URL)
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = u.Scheme, u.Host
	return http.DefaultTransport.RoundTrip(req)
}

type dev struct{}

func (dev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev", Expires: time.Now().Add(time.Hour)}, nil
}
func (dev) Invalidate() {}

type harness struct {
	e       *Engine
	fx      *fixture
	cdm     *fakeCDM
	mu      sync.Mutex
	hosts   []*fakeHost
	libs    []string // the CDM library each host was started with
	autoEnd bool
	endGate chan struct{} // see holdEnds
}

// testCDM is the CDM library the harness's source names.
const testCDM = "/cdm/libwidevinecdm.so"

// fakeCDM is a CDM source: lib, or err when set; it records the reports.
type fakeCDM struct {
	mu      sync.Mutex
	lib     string
	err     error
	reports []error
}

func (c *fakeCDM) Library(context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lib, c.err
}

func (c *fakeCDM) ReportInit(lib string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reports = append(c.reports, err)
}

func (c *fakeCDM) set(lib string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lib, c.err = lib, err
}

// newHarness builds an engine over the fixture and fake hosts. With autoEnd
// the fakes end a track as soon as its last fragment is fed (see holdEnds).
func newHarness(t *testing.T, idle time.Duration, autoEnd bool) *harness {
	fx := newFixture(t)
	hc := &http.Client{Transport: rewrite{fx.srv}}
	api := applemusic.New(hc, dev{}, func() string { return "mut" })
	h := &harness{fx: fx, autoEnd: autoEnd, cdm: &fakeCDM{lib: testCDM}}
	h.e = New(Options{Client: api, HTTP: hc, IdleTimeout: idle, CDM: h.cdm})
	h.e.spawn = func(_ context.Context, lib string) (host, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		f := newFake(100 + len(h.hosts))
		f.autoEnd, f.endGate = h.autoEnd, h.endGate
		h.hosts = append(h.hosts, f)
		h.libs = append(h.libs, lib)
		return f, nil
	}
	t.Cleanup(func() { h.e.Close() })
	return h
}

// holdEnds makes the auto ends of hosts started from now on wait until
// release is called, as a real track plays for a while after its last
// fragment: without it, the end may come before the test sees the item
// playing.
func (h *harness) holdEnds() (release func()) {
	gate := make(chan struct{})
	h.mu.Lock()
	h.endGate = gate
	h.mu.Unlock()
	return sync.OnceFunc(func() { close(gate) })
}

func (h *harness) host(i int) *fakeHost {
	h.mu.Lock()
	defer h.mu.Unlock()
	if i >= len(h.hosts) {
		return nil
	}
	return h.hosts[i]
}

func (h *harness) nhosts() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.hosts)
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func segment(i int) []byte { return mediaBytes[initLen+100*i : initLen+100+100*i] }

func TestPlayFeedsTrackAndEnds(t *testing.T) {
	h := newHarness(t, -1, true)
	release := h.holdEnds()
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	st := h.e.Status()
	// The playlist's 30 s include the encoder's priming, which is trimmed.
	if st.State != playback.StatePlaying || st.ItemID != "A" || st.Duration != 30*time.Second-fmp4.Time(fmp4.AACEncoderDelay, 44100) {
		t.Fatalf("status %+v", st)
	}
	f := h.host(0)
	release()
	eventually(t, "ended", func() bool { return h.e.Status().State == playback.StateEnded })
	loads, feeds, sessions := f.snapshot()
	if len(loads) != 1 || !bytes.Equal(loads[0].init, initSeg) || loads[0].start != 0 || sessions != 1 || f.certs != 1 {
		t.Fatalf("loads %+v sessions %d certs %d", loads, sessions, f.certs)
	}
	if len(feeds) != 3 {
		t.Fatalf("%d feeds", len(feeds))
	}
	for i, fc := range feeds {
		if !bytes.Equal(fc.data, segment(i)) || fc.track != loads[0].track {
			t.Fatalf("feed %d = %v", i, fc.data[:4])
		}
	}
	var kinds []playback.EventKind
	for len(h.e.Events()) > 0 {
		kinds = append(kinds, (<-h.e.Events()).Kind)
	}
	if len(kinds) < 3 || kinds[0] != playback.EventItem {
		t.Fatalf("events %v", kinds)
	}
}

func TestPreloadedPlayNeedsNoNetworkAndPreloadNeverInterrupts(t *testing.T) {
	h := newHarness(t, -1, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Preload(ctx(t), "B"); err != nil {
		t.Fatal(err)
	}
	if st := h.e.Status(); st.ItemID != "A" {
		t.Fatalf("preload changed the current item: %+v", st)
	}
	loads, _, _ := h.host(0).snapshot()
	if len(loads) != 1 {
		t.Fatalf("preload loaded into the pipeline: %+v", loads)
	}
	before := h.fx.total()
	if err := h.e.Play(ctx(t), "B", 0); err != nil {
		t.Fatal(err)
	}
	// Play returned after first audio; only segments 1 and 2 may be fetched since.
	for path, want := range map[string]int{
		"/WebObjects/MZPlay.woa/wa/webPlayback ": 2, "/lic ": 2, "/cert ": 1,
		"/B/list.m3u8 ": 1, fmt.Sprintf("/B/media.mp4 bytes=0-%d", initLen-1): 1,
		fmt.Sprintf("/B/media.mp4 bytes=%d-%d", initLen, initLen+99): 1,
	} {
		p, rng, _ := strings.Cut(path, " ")
		if n := h.fx.requests(p, rng); n != want {
			t.Errorf("%s fetched %d times, want %d", path, n, want)
		}
	}
	if after := h.fx.total(); after-before > 2 {
		t.Errorf("Play of a preloaded item made %d requests", after-before)
	}
	if h.nhosts() != 1 {
		t.Fatalf("%d hosts", h.nhosts())
	}
}

func TestUnauthorizedSurfacesAuth(t *testing.T) {
	h := newHarness(t, -1, false)
	err := h.e.Play(ctx(t), "403", 0)
	if !errors.Is(err, applemusic.ErrUnauthorized) {
		t.Fatalf("Play = %v", err)
	}
	if st := h.e.Status(); st.State != playback.StateError || st.Err == "" {
		t.Fatalf("status %+v", st)
	}
	sawAuth := false
	for len(h.e.Events()) > 0 {
		sawAuth = sawAuth || (<-h.e.Events()).Kind == playback.EventAuth
	}
	if !sawAuth {
		t.Fatal("no EventAuth")
	}
}

func TestSeekFeedsFromContainingSegment(t *testing.T) {
	h := newHarness(t, -1, true)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	eventually(t, "ended", func() bool { return h.e.Status().State == playback.StateEnded })
	if err := h.e.Seek(ctx(t), 15*time.Second); err != nil {
		t.Fatal(err)
	}
	var after []feedCall
	var last loadCall
	eventually(t, "seek feeds the rest", func() bool {
		loads, feeds, _ := f.snapshot()
		last, after = loads[len(loads)-1], nil
		for _, fc := range feeds {
			if fc.track == last.track {
				after = append(after, fc)
			}
		}
		return len(after) == 2
	})
	if last.start != 15*time.Second || len(last.init) != 0 {
		t.Fatalf("seek load %+v", last)
	}
	if !bytes.Equal(after[0].data, segment(1)) || !bytes.Equal(after[1].data, segment(2)) {
		t.Fatalf("seek feeds %v", after)
	}
	sawRestart := false
	for len(h.e.Events()) > 0 {
		ev := <-h.e.Events()
		sawRestart = sawRestart || (ev.Status.State == playback.StatePlaying && ev.Status.Position == 15*time.Second)
	}
	if !sawRestart {
		t.Fatal("no playing event at the seek target")
	}
}

func TestCrashRestartsAndResumes(t *testing.T) {
	h := newHarness(t, -1, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Seek(ctx(t), 12*time.Second); err != nil {
		t.Fatal(err)
	}
	eventually(t, "playing after seek", func() bool { return h.e.Status().State == playback.StatePlaying })
	h.host(0).exit(errors.New("signal: killed"))
	eventually(t, "second host", func() bool { return h.nhosts() == 2 })
	f := h.host(1)
	eventually(t, "resumed", func() bool {
		loads, _, _ := f.snapshot()
		return len(loads) == 1 && h.e.Status().State == playback.StatePlaying
	})
	loads, _, sessions := f.snapshot()
	if loads[0].start < 12*time.Second || loads[0].init == nil || sessions != 1 || f.certs != 1 {
		t.Fatalf("resume load %+v sessions %d certs %d", loads[0], sessions, f.certs)
	}
}

func TestIdleShutdownAndTransparentResume(t *testing.T) {
	h := newHarness(t, 30*time.Millisecond, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Pause(ctx(t)); err != nil {
		t.Fatal(err)
	}
	paused := h.e.Status()
	if paused.State != playback.StatePaused || paused.Position != 500*time.Millisecond {
		t.Fatalf("paused %+v", paused)
	}
	eventually(t, "idle shutdown", func() bool { _, ok := h.e.Host(); return !ok && h.host(0).alive() != nil })
	if err := h.e.Resume(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if h.nhosts() != 2 {
		t.Fatalf("%d hosts", h.nhosts())
	}
	loads, _, _ := h.host(1).snapshot()
	if len(loads) != 1 || loads[0].start != paused.Position {
		t.Fatalf("resume load %+v", loads)
	}
	if st := h.e.Status(); st.State != playback.StatePlaying {
		t.Fatalf("status %+v", st)
	}
}

func TestPauseDuringLoadingDoesNotPlay(t *testing.T) {
	h := newHarness(t, -1, false)
	gate := make(chan struct{})
	inner := h.e.spawn
	h.e.spawn = func(ctx context.Context, lib string) (host, error) {
		<-gate
		return inner(ctx, lib)
	}
	done := make(chan error, 1)
	go func() { done <- h.e.Play(context.Background(), "A", 0) }()
	eventually(t, "loading", func() bool { return h.e.Status().State == playback.StateLoading })
	if err := h.e.Pause(ctx(t)); err != nil {
		t.Fatal(err)
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	f.mu.Lock()
	playing := f.playing
	f.mu.Unlock()
	if playing || h.e.Status().State != playback.StatePaused {
		t.Fatalf("paused-while-loading item plays: host playing %v, status %+v", playing, h.e.Status())
	}
}

func TestFetchRetriesTransientFailures(t *testing.T) {
	retry := []time.Duration{time.Millisecond, time.Millisecond}
	var mu sync.Mutex
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.URL.Path]++
		n := calls[r.URL.Path]
		mu.Unlock()
		switch {
		case r.URL.Path == "/flaky" && n == 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case r.URL.Path == "/limited" && n == 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case r.URL.Path == "/gone":
			http.NotFound(w, r)
		case r.URL.Path == "/down":
			w.WriteHeader(http.StatusBadGateway)
		default:
			http.ServeContent(w, r, "x", time.Time{}, bytes.NewReader(mediaBytes))
		}
	}))
	t.Cleanup(srv.Close)
	e := New(Options{HTTP: srv.Client(), IdleTimeout: -1})
	e.retry = retry
	t.Cleanup(func() { e.Close() })
	for _, p := range []string{"/flaky", "/limited"} {
		b, err := e.fetch(ctx(t), srv.URL+p, hlsRange(int64(initLen), 100))
		if err != nil || !bytes.Equal(b, segment(0)) {
			t.Fatalf("%s: %v", p, err)
		}
	}
	count := func(p string) int {
		mu.Lock()
		defer mu.Unlock()
		return calls[p]
	}
	if _, err := e.fetch(ctx(t), srv.URL+"/gone", hlsRange(0, 1)); err == nil || count("/gone") != 1 {
		t.Fatalf("404 retried or accepted: %v, %d calls", err, count("/gone"))
	}
	if _, err := e.fetch(ctx(t), srv.URL+"/down", hlsRange(0, 1)); err == nil || count("/down") != 3 {
		t.Fatalf("502: %v after %d calls, want failure after 3", err, count("/down"))
	}
}

func hlsRange(off, n int64) hls.ByteRange { return hls.ByteRange{Offset: off, Length: n} }
