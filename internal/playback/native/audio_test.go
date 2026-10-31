//go:build linux || (darwin && cgo)

package native

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/equalizer"
	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

// fakeAudio records a fakeHost's volume, equaliser, device and file calls.
type fakeAudio struct {
	volumes   []float64
	eqs       []equalizer.Curve
	devices   []string
	fileLoads []fileLoad
	file      []byte // bytes fed for the loaded file, in order
	chunks    int
	fileEnded bool
	fileDur   time.Duration // reported with EventStarted
	gains     []float64     // Sound Check gain of each Load/LoadFile
	trackGain []bool        // SetTrackGain calls
}

type fileLoad struct {
	track uint64
	size  int64
	start time.Duration
}

func (f *fakeHost) SetEqualizer(_ context.Context, eq equalizer.Curve) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audio.eqs = append(f.audio.eqs, eq)
	return f.alive()
}

func (f *fakeHost) SetOutputDevice(_ context.Context, dev string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audio.devices = append(f.audio.devices, dev)
	return f.alive()
}

func (f *fakeHost) LoadFile(_ context.Context, track uint64, size int64, start time.Duration, gain float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audio.gains = append(f.audio.gains, gain)
	f.audio.fileLoads = append(f.audio.fileLoads, fileLoad{track, size, start})
	f.audio.file, f.audio.fileEnded = nil, false
	f.track, f.start, f.fed, f.started = track, start, false, false
	return f.alive()
}

func (f *fakeHost) FeedFile(_ context.Context, track uint64, off int64, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.alive(); err != nil {
		return err
	}
	if n := len(f.audio.fileLoads); n == 0 || f.audio.fileLoads[n-1].track != track {
		return cdmhost.ErrStale
	}
	if off != int64(len(f.audio.file)) {
		return fmt.Errorf("fake: file bytes at %d, want %d", off, len(f.audio.file))
	}
	if len(data) > fileChunk {
		return fmt.Errorf("fake: %d-byte chunk", len(data))
	}
	f.audio.file = append(f.audio.file, data...)
	f.audio.chunks++
	f.fed = true
	f.maybeStart()
	return nil
}

func (f *fakeHost) SetTrackGain(_ context.Context, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audio.trackGain = append(f.audio.trackGain, on)
	return f.alive()
}

func (f *fakeHost) EndOfFile(_ context.Context, track uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audio.fileEnded = true
	return f.alive()
}

func (f *fakeHost) audioSnapshot() fakeAudio {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.audio
	a.volumes = append([]float64(nil), a.volumes...)
	a.eqs = append([]equalizer.Curve(nil), a.eqs...)
	a.devices = append([]string(nil), a.devices...)
	a.fileLoads = append([]fileLoad(nil), a.fileLoads...)
	a.gains = append([]float64(nil), a.gains...)
	a.trackGain = append([]bool(nil), a.trackGain...)
	a.file = append([]byte(nil), a.file...)
	return a
}

func TestVolumeReachesHostThroughCurve(t *testing.T) {
	h := newHarness(t, time.Minute, false)
	if err := h.e.SetVolume(ctx(t), 0.5); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	if err := h.e.SetVolume(ctx(t), 0.8); err != nil {
		t.Fatal(err)
	}
	got := h.host(0).audioSnapshot().volumes
	if len(got) != 2 || got[0] != 0.125 || fmt.Sprintf("%.3f", got[1]) != "0.512" {
		t.Errorf("host volumes %v, want [0.125 0.512]", got)
	}
	if v := h.e.Status().Volume; v != 0.8 {
		t.Errorf("Status().Volume = %v, want the slider value 0.8", v)
	}
}

func TestEqualizerAndDeviceFollowHosts(t *testing.T) {
	if !gst.DeviceChoice {
		t.Skip("this platform's sink plays to the default output only")
	}
	h := newHarness(t, time.Minute, false)
	eq := equalizer.Curve{Pregain: -6, Gains: [10]float64{6, 4}}
	if err := h.e.SetEqualizer(ctx(t), eq); err != nil {
		t.Fatal(err)
	}
	if err := h.e.SetOutputDevice(ctx(t), "alsa_output.test"); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	a := h.host(0).audioSnapshot()
	if len(a.eqs) != 1 || a.eqs[0] != eq || len(a.devices) != 1 || a.devices[0] != "alsa_output.test" {
		t.Fatalf("new host got eqs %v devices %q", a.eqs, a.devices)
	}
	flat := equalizer.Curve{}
	if err := h.e.SetEqualizer(ctx(t), flat); err != nil {
		t.Fatal(err)
	}
	if err := h.e.SetOutputDevice(ctx(t), ""); err != nil {
		t.Fatal(err)
	}
	a = h.host(0).audioSnapshot()
	if len(a.eqs) != 2 || a.eqs[1] != flat || len(a.devices) != 2 || a.devices[1] != "" {
		t.Errorf("live changes: eqs %v devices %q", a.eqs, a.devices)
	}
}

// uploadServer serves webPlayback for a library upload and its file. With
// cutFirst, the first file response ends after half the body, so the
// download has to resume with a Range request.
type uploadServer struct {
	srv      *httptest.Server
	file     []byte
	cutFirst bool

	mu     sync.Mutex
	gets   int
	ranges []string
	paths  []string
}

func newUploadServer(t *testing.T, file []byte, cutFirst bool) *uploadServer {
	us := &uploadServer{file: file, cutFirst: cutFirst}
	us.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		us.mu.Lock()
		us.paths = append(us.paths, r.URL.Path)
		us.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/webPlayback"):
			_, _ = fmt.Fprint(w, `{"songList":[{"songId":"i.up","assets":[{"flavor":"","URL":"https://files.test/up.m4a"}]}]}`)
		case r.URL.Path == "/up.m4a":
			us.mu.Lock()
			us.gets++
			first := us.gets == 1
			us.ranges = append(us.ranges, r.Header.Get("Range"))
			us.mu.Unlock()
			if first && us.cutFirst {
				w.Header().Set("Content-Length", fmt.Sprint(len(file)))
				_, _ = w.Write(file[:len(file)/2])
				return // short body: the connection closes mid-response
			}
			http.ServeContent(w, r, "up.m4a", time.Time{}, bytes.NewReader(file))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(us.srv.Close)
	return us
}

func newUploadHarness(t *testing.T, us *uploadServer, dur time.Duration) *harness {
	hc := &http.Client{Transport: rewrite{us.srv}}
	api := applemusic.New(hc, dev{}, func() string { return "mut" })
	h := &harness{}
	h.e = New(Options{Client: api, HTTP: hc, IdleTimeout: time.Minute, CDM: CDMPath(testCDM)})
	h.e.retry = []time.Duration{time.Millisecond, time.Millisecond}
	h.e.spawn = func(context.Context, string) (host, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		f := newFake(100 + len(h.hosts))
		f.audio.fileDur = dur
		h.hosts = append(h.hosts, f)
		return f, nil
	}
	t.Cleanup(func() { _ = h.e.Close() })
	return h
}

func randomFile(n int) []byte {
	b := make([]byte, n)
	r := rand.New(rand.NewPCG(1, 2))
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	return b
}

// waitFile waits until the fake host has the whole file.
func waitFile(t *testing.T, f *fakeHost) fakeAudio {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		a := f.audioSnapshot()
		if a.fileEnded {
			return a
		}
		if time.Now().After(deadline) {
			t.Fatalf("file not fully fed: %d bytes", len(a.file))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUploadStreamsFileToHostAndSeeksInPlace(t *testing.T) {
	file := randomFile(3*fileChunk + 1234)
	us := newUploadServer(t, file, false)
	h := newUploadHarness(t, us, 3*time.Minute)
	if err := h.e.Play(ctx(t), "i.up", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	a := waitFile(t, f)
	if len(a.fileLoads) != 1 || a.fileLoads[0].size != int64(len(file)) || a.fileLoads[0].start != 0 {
		t.Fatalf("LoadFile calls %+v, want one of %d bytes at 0", a.fileLoads, len(file))
	}
	if !bytes.Equal(a.file, file) || a.chunks < 4 {
		t.Fatalf("host got %d bytes in %d chunks, want the %d-byte file in ≥4", len(a.file), a.chunks, len(file))
	}
	if d := h.e.Status().Duration; d != 3*time.Minute {
		t.Errorf("Duration = %v, want the host-reported 3m", d)
	}

	if err := h.e.Seek(ctx(t), 90*time.Second); err != nil {
		t.Fatal(err)
	}
	loads, _, _ := f.snapshot()
	last := loads[len(loads)-1]
	if last.init != nil || last.start != 90*time.Second || last.track == a.fileLoads[0].track {
		t.Errorf("seek loaded %+v, want a new generation at 1m30s without init", last)
	}
	if b := f.audioSnapshot(); len(b.fileLoads) != 1 || b.chunks != a.chunks {
		t.Errorf("seek re-sent the file: %d loads, %d chunks (was %d)", len(b.fileLoads), b.chunks, a.chunks)
	}
	us.mu.Lock()
	defer us.mu.Unlock()
	for _, p := range us.paths {
		if p == "/lic" || p == "/cert" || strings.HasSuffix(p, ".m3u8") {
			t.Errorf("upload playback requested %s", p)
		}
	}
	if us.gets != 1 {
		t.Errorf("file fetched %d times, want once", us.gets)
	}
}

// Playing an upload from past its end (a stale resume position) ends at
// once instead of waiting for audio that never comes.
func TestUploadStartPastEndEnds(t *testing.T) {
	us := newUploadServer(t, randomFile(fileChunk), false)
	h := newUploadHarness(t, us, 1400*time.Millisecond)
	t0 := time.Now()
	if err := h.e.Play(ctx(t), "i.up", 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(t0); took > 5*time.Second {
		t.Errorf("Play took %v", took)
	}
	if s := h.e.Status(); s.State != playback.StateEnded {
		t.Errorf("state %s, want ended", s.State)
	}
}

func TestUploadDownloadResumesAfterCut(t *testing.T) {
	file := randomFile(2*fileChunk + 77)
	us := newUploadServer(t, file, true)
	h := newUploadHarness(t, us, time.Minute)
	if err := h.e.Play(ctx(t), "i.up", 0); err != nil {
		t.Fatal(err)
	}
	a := waitFile(t, h.host(0))
	if !bytes.Equal(a.file, file) {
		t.Fatalf("host got %d bytes, want the %d-byte file intact", len(a.file), len(file))
	}
	us.mu.Lock()
	defer us.mu.Unlock()
	if len(us.ranges) != 2 || us.ranges[0] != "" || !strings.HasPrefix(us.ranges[1], "bytes=") {
		t.Errorf("file requests ranges %q, want a full GET then a resume", us.ranges)
	}
}

func TestSoundCheckFollowsSettingAndHosts(t *testing.T) {
	h := newHarness(t, time.Minute, false)
	if err := h.e.SetSoundCheck(ctx(t), true); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	if err := h.e.SetSoundCheck(ctx(t), false); err != nil {
		t.Fatal(err)
	}
	a := h.host(0).audioSnapshot()
	if len(a.trackGain) != 2 || !a.trackGain[0] || a.trackGain[1] {
		t.Errorf("SetTrackGain calls %v, want [true false]", a.trackGain)
	}
	// The fixture's init segment carries no loudness: unity.
	if len(a.gains) != 1 || a.gains[0] != 0 {
		t.Errorf("Load gains %v, want [0]", a.gains)
	}
}

func TestUploadGainFromItsSoundCheckTag(t *testing.T) {
	// An MP3 whose ID3v2.3 tag holds iTunNORM 2000/2000 (−3.01 dB).
	comm := append([]byte{0, 'e', 'n', 'g'}, []byte("iTunNORM\x00 000007D0 000007D0 0 0 0 0 0 0 0 0")...)
	frame := append(append([]byte("COMM"), byte(len(comm)>>24), byte(len(comm)>>16), byte(len(comm)>>8), byte(len(comm)), 0, 0), comm...)
	n := len(frame)
	tag := append([]byte{'I', 'D', '3', 3, 0, 0, byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}, frame...)
	file := append(tag, randomFile(fileChunk)...)
	us := newUploadServer(t, file, false)
	h := newUploadHarness(t, us, time.Minute)
	if err := h.e.Play(ctx(t), "i.up", 0); err != nil {
		t.Fatal(err)
	}
	a := h.host(0).audioSnapshot()
	if len(a.gains) != 1 || fmt.Sprintf("%.4f", a.gains[0]) != "0.7071" {
		t.Errorf("LoadFile gains %v, want [0.7071] (−3.01 dB)", a.gains)
	}
}
