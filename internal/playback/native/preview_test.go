//go:build linux || (cgo && (darwin || windows))

package native

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// previewServer serves the catalog songs "A" and "B" with a preview each,
// "NOPREV" without one, and their clips; it records every request and
// whether any carried a Music User Token.
type previewServer struct {
	srv  *httptest.Server
	file []byte

	mu    sync.Mutex
	paths []string
	mut   bool
}

func newPreviewServer(t *testing.T, file []byte) *previewServer {
	ps := &previewServer{file: file}
	ps.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.mu.Lock()
		ps.paths = append(ps.paths, r.URL.Path)
		ps.mut = ps.mut || r.Header.Get("Music-User-Token") != ""
		ps.mu.Unlock()
		switch r.URL.Path {
		case "/v1/catalog/gb/songs/A", "/v1/catalog/gb/songs/B":
			id := r.URL.Path[len("/v1/catalog/gb/songs/"):]
			_, _ = fmt.Fprintf(w, `{"data":[{"id":%q,"type":"songs","attributes":{"name":"Song %s","durationInMillis":245000,"previews":[{"url":"https://audio.test/%s.p.m4a"}]}}]}`, id, id, id)
		case "/v1/catalog/gb/songs/NOPREV":
			_, _ = fmt.Fprint(w, `{"data":[{"id":"NOPREV","type":"songs","attributes":{"name":"No preview","previews":[]}}]}`)
		case "/A.p.m4a", "/B.p.m4a":
			http.ServeContent(w, r, "p.m4a", time.Time{}, bytes.NewReader(file))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ps.srv.Close)
	return ps
}

// requested returns the paths requested so far.
func (ps *previewServer) requested() []string {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return append([]string(nil), ps.paths...)
}

// newPreviewHarness is an engine in preview mode over ps, with no Music
// User Token and a CDM source that has nothing ready (previews must not
// ask it), storefront sf. Its fake hosts report dur for each file.
func newPreviewHarness(t *testing.T, ps *previewServer, sf string, dur time.Duration) *harness {
	hc := &http.Client{Transport: rewrite{ps.srv}}
	api := applemusic.New(hc, dev{}, func() string { return "" })
	h := &harness{cdm: &fakeCDM{err: fmt.Errorf("cdm: not downloaded: %w", playback.ErrNoCDM)}}
	h.e = New(Options{Client: api, HTTP: hc, IdleTimeout: time.Minute, CDM: h.cdm, Preview: true, Storefront: func() string { return sf }})
	h.e.retry = []time.Duration{time.Millisecond}
	h.e.spawn = func(_ context.Context, lib string) (host, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		f := newFake(100 + len(h.hosts))
		f.audio.fileDur = dur
		h.hosts = append(h.hosts, f)
		h.libs = append(h.libs, lib)
		return f, nil
	}
	t.Cleanup(func() { _ = h.e.Close() })
	return h
}

// previewFile is a clip like Apple's: a movie header saying 30.024 s
// first, then the audio (n random bytes).
func previewFile(n int) []byte {
	box := func(typ string, parts ...[]byte) []byte {
		body := bytes.Join(parts, nil)
		return append(binary.BigEndian.AppendUint32(nil, uint32(8+len(body))), append([]byte(typ), body...)...)
	}
	u32 := func(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
	mvhd := box("mvhd", u32(0), u32(0), u32(0), u32(1000), u32(30024), make([]byte, 80))
	return bytes.Join([][]byte{box("ftyp", []byte("M4A "), u32(0)), box("moov", mvhd), box("mdat", randomFile(n))}, nil)
}

// In preview mode a song plays its clip through a host started without a
// CDM: no webPlayback, certificate or licence request, no Music User
// Token, and the CDM source is never asked (none needs downloading). Its
// length is the clip's, not the whole song's that the host's demuxer
// reads from Apple's iTunSMPB tag.
func TestPreviewPlaysWithoutCDMOrLicence(t *testing.T) {
	file := previewFile(fileChunk + 777)
	ps := newPreviewServer(t, file)
	h := newPreviewHarness(t, ps, "gb", 318*time.Second)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	st := h.e.Status()
	if !st.Preview || st.State != playback.StatePlaying || st.ItemID != "A" {
		t.Errorf("status %+v, want A playing as a preview", st)
	}
	a := waitFile(t, h.host(0))
	if len(a.fileLoads) != 1 || a.fileLoads[0].size != int64(len(file)) || !bytes.Equal(a.file, file) {
		t.Fatalf("host got loads %+v and %d bytes, want the %d-byte clip", a.fileLoads, len(a.file), len(file))
	}
	if d := h.e.Status().Duration; d != 30024*time.Millisecond {
		t.Errorf("Duration = %v, want the clip's 30.024s, not the song's", d)
	}
	h.mu.Lock()
	libs := h.libs
	h.mu.Unlock()
	if len(libs) != 1 || libs[0] != "" {
		t.Errorf("host started with CDM %q, want none", libs)
	}
	h.cdm.mu.Lock()
	reports := len(h.cdm.reports)
	h.cdm.mu.Unlock()
	if reports != 0 {
		t.Errorf("the CDM source heard of %d host starts", reports)
	}
	f := h.host(0)
	loads, _, sessions := f.snapshot()
	f.mu.Lock()
	certs := f.certs
	f.mu.Unlock()
	if len(loads) != 0 || sessions != 0 || certs != 0 {
		t.Errorf("host got %d HLS loads, %d CDM sessions, %d certificates", len(loads), sessions, certs)
	}
	for _, p := range ps.requested() {
		if p != "/v1/catalog/gb/songs/A" && p != "/A.p.m4a" {
			t.Errorf("preview playback requested %s", p)
		}
	}
	ps.mu.Lock()
	mut := ps.mut
	ps.mu.Unlock()
	if mut {
		t.Error("a request carried a Music User Token")
	}
	if h.e.Capabilities().Crossfade {
		t.Error("previews claim crossfades")
	}

	// The clip plays out; the next one plays the same way.
	h.host(0).emit(eventEnded(h.e))
	eventually(t, "ended", func() bool { return h.e.Status().State == playback.StateEnded })
	if err := h.e.Play(ctx(t), "B", 0); err != nil {
		t.Fatal(err)
	}
	if st := h.e.Status(); !st.Preview || st.ItemID != "B" || st.State != playback.StatePlaying {
		t.Errorf("next: %+v", st)
	}
	if h.nhosts() != 1 {
		t.Errorf("%d hosts, want the one", h.nhosts())
	}
}

// eventEnded is the host's end of the engine's current track.
func eventEnded(e *Engine) cdmhost.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return cdmhost.Event{Kind: cdmhost.EventEnded, Track: e.cur.tok, At: time.Now()}
}

// Songs without a clip, stations and library uploads have no preview and
// fail as not playable (the coordinator skips them); without a storefront
// nothing can be looked up.
func TestPreviewUnavailable(t *testing.T) {
	ps := newPreviewServer(t, randomFile(1000))
	h := newPreviewHarness(t, ps, "gb", 30*time.Second)
	for _, id := range []string{"NOPREV", "ra.1234", "i.upload"} {
		if err := h.e.Play(ctx(t), id, 0); !errors.Is(err, applemusic.ErrNotPlayable) {
			t.Errorf("Play(%s) = %v, want ErrNotPlayable", id, err)
		}
	}
	for _, p := range ps.requested() {
		if !strings.HasPrefix(p, "/v1/catalog/gb/songs/") {
			t.Errorf("requested %s", p)
		}
	}

	none := newPreviewHarness(t, ps, "", 30*time.Second)
	if err := none.e.Play(ctx(t), "A", 0); err == nil || errors.Is(err, applemusic.ErrNotPlayable) {
		t.Errorf("Play without a storefront = %v", err)
	}
}

// Diagnostics in preview mode probe a host without a CDM, even with none
// downloaded.
func TestPreviewDiagnoseProbesWithoutCDM(t *testing.T) {
	h := newPreviewHarness(t, newPreviewServer(t, nil), "gb", 30*time.Second)
	var got cdmhost.Options
	h.e.probe = func(_ context.Context, o cdmhost.Options) (cdmhost.Info, error) {
		got = o
		return cdmhost.Info{PID: 7, GStreamer: "1.26", Decoder: "avdec_aac", Sink: "fakesink"}, nil
	}
	d, err := h.e.Diagnose(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Clear || got.CDMPath != "" {
		t.Errorf("probe options %+v, want a clear host", got)
	}
	if d.CDMPath != "" || d.Decoder != "avdec_aac" || d.GStreamer != "1.26" {
		t.Errorf("diagnosis %+v", d)
	}
}
