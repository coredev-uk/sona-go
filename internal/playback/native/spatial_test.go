//go:build linux || (cgo && (darwin || windows))

package native

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
)

// The fixture's Spatial Audio songs (served by spatialRoute): each plays
// three 10-second segments of the binaural rendition, the first with key 1
// and the rest with key 2, as Apple's 15-second segments change key after
// the first.
//
//	"S1"  has the spatial and atmos traits and both Widevine keys
//	"SN"  has neither trait (a stereo song)
//	"SF"  has the traits but only FairPlay keys
//	"SR"  has the traits; Apple refuses the licence of key 2
//	"SM"  has the traits; its master has no binaural rendition
//	"SP"  has the traits; its rendition's playlist is missing
//	"SE"  has the traits but no enhancedHls master
//	"SX"  is not in the catalogue
var (
	spatialKID1 = bytes.Repeat([]byte{0x11}, 16)
	spatialKID2 = bytes.Repeat([]byte{0x22}, 16)
)

// widevinePSSH is a Widevine 'pssh' box holding kid, as the playlists' data
// URIs do.
func widevinePSSH(kid []byte) []byte {
	data := append([]byte{0x12, 0x10}, kid...) // WidevinePsshData.key_id
	b := make([]byte, 32, 32+len(data))
	binary.BigEndian.PutUint32(b[0:], uint32(32+len(data)))
	copy(b[4:], "pssh")
	binary.BigEndian.PutUint32(b[28:], uint32(len(data)))
	return append(b, data...)
}

// widevineURI is the URI of kid's Widevine EXT-X-KEY, which is also the key
// URI of its licence request.
func widevineURI(kid []byte) string {
	return "data:text/plain;base64," + base64.StdEncoding.EncodeToString(widevinePSSH(kid))
}

// licenceReq is a licence request as the fixture saw it.
type licenceReq struct {
	URI     string
	AdamID  string
	Renewal bool
}

// licence answers a licence request and records it. Apple refuses key 2 of
// "SR".
func (fx *fixture) licence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URI           string `json:"uri"`
		AdamID        string `json:"adamId"`
		UserInitiated bool   `json:"user-initiated"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	fx.mu.Lock()
	fx.lics = append(fx.lics, licenceReq{URI: body.URI, AdamID: body.AdamID, Renewal: !body.UserInitiated})
	fx.mu.Unlock()
	if g := fx.licGate.Load(); g != nil {
		select {
		case <-*g:
		case <-r.Context().Done():
			return
		}
	}
	if body.AdamID == "SR" && body.URI == widevineURI(spatialKID2) {
		_, _ = fmt.Fprint(w, `{"status":-1002,"message":"refused"}`)
		return
	}
	_, _ = fmt.Fprintf(w, `{"status":0,"license":"%s","stkn":"slot-%d"}`, base64.StdEncoding.EncodeToString([]byte("licence")), len(fx.licences()))
}

func (fx *fixture) licences() []licenceReq {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	return append([]licenceReq(nil), fx.lics...)
}

// spatialRoute serves the catalogue, master playlists and rendition
// playlists of the Spatial Audio songs; it reports whether it did.
func (fx *fixture) spatialRoute(w http.ResponseWriter, r *http.Request) bool {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/v1/catalog/gb/songs/"):
		id := strings.TrimPrefix(p, "/v1/catalog/gb/songs/")
		if id == "SX" {
			http.NotFound(w, r)
			return true
		}
		traits, master := `["lossy-stereo"]`, ""
		if id != "SN" && strings.HasPrefix(id, "S") {
			traits = `["atmos","spatial"]`
			if id != "SE" {
				master = "https://aod.test/" + id + "/master.m3u8"
			}
		}
		_, _ = fmt.Fprintf(w, `{"data":[{"id":%q,"type":"songs","attributes":{"audioTraits":%s,"extendedAssetUrls":{"enhancedHls":%q}}}]}`, id, traits, master)
		return true
	case strings.HasPrefix(p, "/S") && strings.HasSuffix(p, "/master.m3u8"):
		id := strings.Split(p, "/")[1]
		_, _ = fmt.Fprint(w, "#EXTM3U\n#EXT-X-VERSION:7\n")
		for _, a := range []string{"audio-stereo-256", "audio-stereo-256-binaural", "audio-HE-stereo-64-binaural", "audio-stereo-256-downmix"} {
			if id == "SM" && strings.HasSuffix(a, "binaural") {
				continue
			}
			_, _ = fmt.Fprintf(w, "#EXT-X-STREAM-INF:BANDWIDTH=256000,CODECS=\"mp4a.40.2\",AUDIO=%q\nspatial/%s.m3u8\n", a, a)
		}
		return true
	case strings.Contains(p, "/spatial/") && strings.HasSuffix(p, ".m3u8"):
		id := strings.Split(p, "/")[1]
		if id == "SP" {
			http.NotFound(w, r)
			return true
		}
		fairplay := func(name string) string {
			return fmt.Sprintf("#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"skd://itunes.apple.com/P000000000/%s\",KEYFORMAT=\"com.apple.streamingkeydelivery\",KEYFORMATVERSIONS=\"1\"\n", name)
		}
		widevine := func(kid []byte) string {
			return fmt.Sprintf("#EXT-X-KEY:METHOD=SAMPLE-AES,URI=%q,KEYFORMAT=%q,KEYFORMATVERSIONS=\"1\"\n", widevineURI(kid), hls.WidevineKeyFormat)
		}
		keys := [3]string{}
		for i, kid := range [][]byte{spatialKID1, spatialKID2, spatialKID2} {
			keys[i] = fairplay([]string{"s1/e1", "c24", "c24"}[i])
			if id != "SF" {
				keys[i] += widevine(kid)
			}
		}
		_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-MAP:URI=\"media.mp4\",BYTERANGE=\"%d@0\"\n", initLen)
		_, _ = fmt.Fprintf(w, "%s#EXTINF:10,\n#EXT-X-BYTERANGE:100@%d\nmedia.mp4\n", keys[0], initLen)
		_, _ = fmt.Fprintf(w, "%s#EXTINF:10,\n#EXT-X-BYTERANGE:100\nmedia.mp4\n", keys[1])
		_, _ = fmt.Fprint(w, "#EXTINF:10,\n#EXT-X-BYTERANGE:100\nmedia.mp4\n#EXT-X-ENDLIST\n")
		return true
	}
	return false
}

// logBuffer collects an engine's log.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// spatialHarness is a harness whose engine has Spatial Audio on and logs
// to the returned buffer.
func spatialHarness(t *testing.T, on bool) (*harness, *logBuffer) {
	t.Helper()
	h := newHarness(t, -1, false)
	buf := &logBuffer{}
	h.e.log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h.e.SetSpatial(on)
	return h, buf
}

// queued is how many items were queued behind the current one (a drop
// is a Next of track 0).
func (f *fakeHost) queued() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.nexts {
		if c.track != 0 {
			n++
		}
	}
	return n
}

func kids(feeds []feedCall) [][]byte {
	out := make([][]byte, len(feeds))
	for i, f := range feeds {
		out[i] = f.kid
	}
	return out
}

func wantKIDs(t *testing.T, feeds []feedCall, want ...[]byte) {
	t.Helper()
	got := kids(feeds)
	if len(got) != len(want) {
		t.Fatalf("%d feeds with key IDs %x, want %x", len(got), got, want)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("feed %d has key ID %x, want %x (all %x)", i, got[i], want[i], got)
		}
	}
}

func TestSpatialRenditionChoice(t *testing.T) {
	for q, want := range map[applemusic.Quality]string{
		applemusic.Quality256: "audio-stereo-256-binaural",
		applemusic.Quality64:  "audio-HE-stereo-64-binaural",
		0:                     "audio-stereo-256-binaural",
	} {
		if got := spatialRendition(q); got != want {
			t.Errorf("quality %d plays %s, want %s", q, got, want)
		}
	}
	for traits, want := range map[string]bool{
		"spatial": true, "atmos": true, "atmos,spatial": true, "lossless,atmos": true,
		"lossy-stereo": false, "lossless,hi-res-lossless": false, "": false,
	} {
		if got := hasSpatialTrait(strings.Split(traits, ",")); got != want {
			t.Errorf("traits %q: spatial %v, want %v", traits, got, want)
		}
	}
}

// With the setting on, a Spatial Audio song plays its binaural rendition:
// one licensed session per Widevine key, each segment fed with its key's
// KID, and the status says so while the item stays the one asked for.
func TestSpatialPlaysTheBinauralRenditionWithEachKey(t *testing.T) {
	h, log := spatialHarness(t, true)
	if err := h.e.Play(ctx(t), "S1", 0); err != nil {
		t.Fatal(err)
	}
	st := h.e.Status()
	if st.State != playback.StatePlaying || st.ItemID != "S1" || !st.Spatial || st.Duration != trimmed {
		t.Fatalf("status %+v (spatial, S1, %v)", st, trimmed)
	}
	f := h.host(0)
	eventually(t, "every segment fed", func() bool { _, feeds, _ := f.snapshot(); return len(feeds) == 3 })
	loads, feeds, sessions := f.snapshot()
	if len(loads) != 1 || !bytes.Equal(loads[0].init, initSeg) || loads[0].dur != trimmed {
		t.Fatalf("loads %+v", loads)
	}
	wantKIDs(t, feeds, spatialKID1, spatialKID2, spatialKID2)
	for i, fc := range feeds {
		if !bytes.Equal(fc.data, segment(i)) {
			t.Errorf("feed %d carries %v", i, fc.data[:4])
		}
	}
	if sessions != 2 || f.certs != 1 {
		t.Fatalf("%d sessions, %d certificates; want one session per key", sessions, f.certs)
	}
	if got := string(f.inits["s1"]) + "|" + string(f.inits["s2"]); got != string(widevinePSSH(spatialKID1))+"|"+string(widevinePSSH(spatialKID2)) {
		t.Errorf("sessions were created from %q", got)
	}
	// The licences are asked together for the catalogue song, each key's
	// own URI.
	got := map[string]string{}
	for _, l := range h.fx.licences() {
		got[l.URI] = l.AdamID
	}
	if want := map[string]string{widevineURI(spatialKID1): "S1", widevineURI(spatialKID2): "S1"}; fmt.Sprint(got) != fmt.Sprint(want) || len(h.fx.licences()) != 2 {
		t.Errorf("licence requests %+v, want %+v", h.fx.licences(), want)
	}
	// The binaural rendition is fetched, not the stereo stream.
	if n := h.fx.requests("/S1/spatial/audio-stereo-256-binaural.m3u8", ""); n != 1 {
		t.Errorf("binaural playlist fetched %d times", n)
	}
	if n := h.fx.requests("/S1/list.m3u8", ""); n != 0 {
		t.Errorf("the stereo playlist was fetched %d times", n)
	}
	if !strings.Contains(log.String(), "rendition=audio-stereo-256-binaural") {
		t.Errorf("the log doesn't name the rendition:\n%s", log)
	}
}

func TestSpatialAtLowQualityPlaysTheHERendition(t *testing.T) {
	h, _ := spatialHarness(t, true)
	h.e.SetQuality(applemusic.Quality64)
	if err := h.e.Play(ctx(t), "S1", 0); err != nil {
		t.Fatal(err)
	}
	if !h.e.Status().Spatial || h.fx.requests("/S1/spatial/audio-HE-stereo-64-binaural.m3u8", "") != 1 ||
		h.fx.requests("/S1/spatial/audio-stereo-256-binaural.m3u8", "") != 0 {
		t.Fatalf("status %+v; HE playlist fetched %d times", h.e.Status(), h.fx.requests("/S1/spatial/audio-HE-stereo-64-binaural.m3u8", ""))
	}
}

// With the setting off nothing about Spatial Audio is asked of Apple.
func TestSpatialOffPlaysStereoAndAsksNothing(t *testing.T) {
	h, _ := spatialHarness(t, false)
	if err := h.e.Play(ctx(t), "S1", 0); err != nil {
		t.Fatal(err)
	}
	if st := h.e.Status(); st.Spatial || st.State != playback.StatePlaying {
		t.Fatalf("status %+v", st)
	}
	if n := h.fx.requests("/v1/catalog/gb/songs/S1", ""); n != 0 {
		t.Errorf("the catalogue was asked %d times", n)
	}
	_, feeds, _ := h.host(0).snapshot()
	for _, fc := range feeds {
		if fc.kid != nil {
			t.Errorf("a stereo segment was fed with a key ID %x", fc.kid)
		}
	}
}

// A song that can't play spatial plays its stereo stream, and the log says
// why (except for a song that simply has no Spatial Audio mix).
func TestSpatialFallsBackToTheStereoStream(t *testing.T) {
	for _, tc := range []struct {
		id    string
		why   string // in the log
		miss  string // Status.SpatialMiss
		quiet bool   // not logged above debug level
	}{
		{id: "SN", miss: "", why: "the song has no Spatial Audio mix", quiet: true},
		{id: "SF", miss: playback.SpatialAppleOnly, why: "segment 0: no Widevine key (FairPlay only)"},
		{id: "SR", miss: playback.SpatialAppleOnly, why: "a licence was refused: key 2 of 2: apple music: licence rejected"},
		{id: "SM", miss: playback.SpatialNoMix, why: "the master has no audio-stereo-256-binaural rendition"},
		{id: "SP", miss: playback.SpatialUnavailable, why: "the rendition's playlist"},
		{id: "SE", miss: playback.SpatialNoMix, why: "the song has no enhancedHls master"},
		{id: "SX", miss: "", why: "the catalogue lookup failed"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			h, log := spatialHarness(t, true)
			if err := h.e.Play(ctx(t), tc.id, 0); err != nil {
				t.Fatalf("a fallback must still play: %v", err)
			}
			st := h.e.Status()
			if st.State != playback.StatePlaying || st.ItemID != tc.id || st.Spatial || st.Duration != trimmed || st.SpatialMiss != tc.miss {
				t.Fatalf("status %+v, want the reason %q", st, tc.miss)
			}
			f := h.host(0)
			eventually(t, "the stereo stream fed", func() bool { _, feeds, _ := f.snapshot(); return len(feeds) == 3 })
			_, feeds, sessions := f.snapshot()
			wantKIDs(t, feeds, nil, nil, nil)
			if sessions != 1 {
				t.Errorf("%d CDM sessions are left; the stereo stream needs one (a refused rendition's must be closed)", sessions)
			}
			if n := h.fx.requests("/"+tc.id+"/list.m3u8", ""); n != 1 {
				t.Errorf("the stereo playlist was fetched %d times", n)
			}
			h.e.mu.Lock()
			held := len(h.e.sessions)
			h.e.mu.Unlock()
			if held != 1 {
				t.Errorf("the engine still tracks %d sessions", held)
			}
			var line string
			for l := range strings.SplitSeq(log.String(), "\n") {
				if strings.Contains(l, "why=") {
					line = l
				}
			}
			switch {
			case !strings.Contains(line, tc.why):
				t.Errorf("the log has no %q:\n%s", tc.why, log)
			case tc.quiet != strings.Contains(line, "level=DEBUG"):
				t.Errorf("wrong level for the reason: %s", line)
			}
		})
	}
}

// A library song is played by its library ID: its catalogue ID, which
// webPlayback answers with, is where the Spatial Audio mix is looked up.
func TestSpatialPlaysALibrarySongsCatalogueMix(t *testing.T) {
	h, _ := spatialHarness(t, true)
	if err := h.e.Play(ctx(t), "i.S1", 0); err != nil {
		t.Fatal(err)
	}
	if st := h.e.Status(); !st.Spatial || st.ItemID != "i.S1" {
		t.Fatalf("status %+v", st)
	}
	for _, l := range h.fx.licences() {
		if l.AdamID != "S1" {
			t.Errorf("licence asked for %q, want the catalogue song", l.AdamID)
		}
	}
}

// The catalogue is asked once for a song, however many times it is
// prepared.
func TestSpatialCachesTheCatalogue(t *testing.T) {
	h, _ := spatialHarness(t, true)
	for range 2 {
		if err := h.e.Play(ctx(t), "S1", 0); err != nil {
			t.Fatal(err)
		}
		h.e.mu.Lock()
		h.e.dropLocked(h.e.cache["S1"])
		h.e.mu.Unlock()
	}
	if n, m := h.fx.requests("/v1/catalog/gb/songs/S1", ""), h.fx.requests("/S1/master.m3u8", ""); n != 1 || m != 1 {
		t.Fatalf("catalogue asked %d times, master fetched %d times", n, m)
	}
	// A failed lookup is not kept.
	for range 2 {
		if err := h.e.Play(ctx(t), "SX", 0); err != nil {
			t.Fatal(err)
		}
		h.e.mu.Lock()
		h.e.dropLocked(h.e.cache["SX"])
		h.e.mu.Unlock()
	}
	if n := h.fx.requests("/v1/catalog/gb/songs/SX", ""); n != 2 {
		t.Errorf("a failed lookup was asked %d times, want 2", n)
	}
}

// Turning Spatial Audio on or off reloads the playing song, paused or
// playing as it was, at its position, in the other mix.
func TestSpatialToggleSwitchesThePlayingSong(t *testing.T) {
	h, _ := spatialHarness(t, false)
	if err := h.e.Play(ctx(t), "S1", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	eventually(t, "played in stereo", func() bool { _, feeds, _ := f.snapshot(); return len(feeds) == 3 })
	if err := h.e.Seek(ctx(t), 12*time.Second); err != nil {
		t.Fatal(err)
	}
	eventually(t, "fed after the seek", func() bool { _, feeds, _ := f.snapshot(); return len(feeds) == 5 })
	if err := h.e.Pause(ctx(t)); err != nil { // the fake host paused at 12.5 s
		t.Fatal(err)
	}
	at := h.e.Status().Position
	if at != 12500*time.Millisecond {
		t.Fatalf("paused at %v", at)
	}
	_, before, _ := f.snapshot()

	h.e.SetSpatial(true)
	eventually(t, "the spatial mix", func() bool { return h.e.Status().Spatial })
	st := h.e.Status()
	if st.State != playback.StatePaused || st.Position != at || st.SpatialMiss != "" {
		t.Fatalf("status %+v, want paused at %v", st, at)
	}
	eventually(t, "the mix fed from the position", func() bool { _, feeds, _ := f.snapshot(); return len(feeds)-len(before) == 2 })
	loads, feeds, _ := f.snapshot()
	if l := loads[len(loads)-1]; len(loads) != 3 || l.start != at || l.dur != trimmed {
		t.Fatalf("loads %+v, want a reload at %v", loads, at)
	}
	wantKIDs(t, feeds[len(before):], spatialKID2, spatialKID2)
	if n := h.fx.requests("/S1/spatial/audio-stereo-256-binaural.m3u8", ""); n != 1 {
		t.Errorf("binaural playlist fetched %d times", n)
	}

	// Playing again, back to stereo.
	if err := h.e.Resume(ctx(t)); err != nil {
		t.Fatal(err)
	}
	_, before, _ = f.snapshot()
	h.e.SetSpatial(false)
	eventually(t, "the stereo mix", func() bool { st := h.e.Status(); return !st.Spatial && st.State == playback.StatePlaying })
	eventually(t, "stereo fed from the position", func() bool { _, feeds, _ := f.snapshot(); return len(feeds)-len(before) == 2 })
	loads, feeds, _ = f.snapshot()
	if l := loads[len(loads)-1]; len(loads) != 4 || l.start < at || l.start > at+5*time.Second {
		t.Fatalf("loads %+v, want a reload at about %v", loads, at)
	}
	wantKIDs(t, feeds[len(before):], nil, nil)
	// The spatial mix's sessions are forgotten, the stereo stream's one is held.
	eventually(t, "the sessions of the mix left are forgotten", func() bool {
		h.e.mu.Lock()
		defer h.e.mu.Unlock()
		return len(h.e.sessions) == 1
	})
}

// A song with no mix to switch to plays on untouched, and says why.
func TestSpatialToggleLeavesASongWithNoMixAlone(t *testing.T) {
	for _, tc := range []struct {
		id, miss string
	}{
		{"SN", ""}, // no Spatial Audio mix at all: nothing to say
		{"SF", playback.SpatialAppleOnly},
		{"SR", playback.SpatialAppleOnly},
		{"SM", playback.SpatialNoMix},
		{"SP", playback.SpatialUnavailable},
	} {
		t.Run(tc.id, func(t *testing.T) {
			h, _ := spatialHarness(t, false)
			if err := h.e.Play(ctx(t), tc.id, 0); err != nil {
				t.Fatal(err)
			}
			f := h.host(0)
			eventually(t, "played", func() bool { _, feeds, _ := f.snapshot(); return len(feeds) == 3 })
			h.e.SetSpatial(true)
			eventually(t, "the switch has looked at the song and ended", func() bool {
				if h.fx.requests("/v1/catalog/gb/songs/"+tc.id, "") == 0 || !h.e.mixMu.TryLock() {
					return false
				}
				h.e.mixMu.Unlock()
				return true
			})
			if got := h.e.Status().SpatialMiss; got != tc.miss {
				t.Fatalf("reason %q, want %q", got, tc.miss)
			}
			if st := h.e.Status(); st.Spatial || st.State != playback.StatePlaying {
				t.Fatalf("status %+v", st)
			}
			if loads, feeds, sessions := f.snapshot(); len(loads) != 1 || len(feeds) != 3 || sessions != 1 {
				t.Fatalf("%d loads, %d feeds, %d sessions: the song was reloaded", len(loads), len(feeds), sessions)
			}
			h.e.SetSpatial(false)
			if st := h.e.Status(); st.SpatialMiss != "" || st.Spatial {
				t.Fatalf("status %+v", st)
			}
			if loads, _, _ := f.snapshot(); len(loads) != 1 {
				t.Fatalf("%d loads", len(loads))
			}
		})
	}
}

// A switch that fails leaves the audio as it was.
func TestSpatialFailedSwitchKeepsTheAudio(t *testing.T) {
	h, log := spatialHarness(t, false)
	if err := h.e.Play(ctx(t), "S1", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	eventually(t, "played", func() bool { _, feeds, _ := f.snapshot(); return len(feeds) == 3 })
	h.fx.webDown.Store(true)
	h.e.SetSpatial(true)
	eventually(t, "the failure is logged", func() bool { return strings.Contains(log.String(), "switching the playing song failed") })
	st := h.e.Status()
	if st.State != playback.StatePlaying || st.Spatial || st.Err != "" {
		t.Fatalf("status %+v: the song must play on", st)
	}
	if loads, _, _ := f.snapshot(); len(loads) != 1 {
		t.Fatalf("%d loads", len(loads))
	}
	// The song still seeks and is the engine's cached media.
	h.fx.webDown.Store(false)
	if err := h.e.Seek(ctx(t), time.Second); err != nil {
		t.Fatal(err)
	}
	// And toggling again gets there.
	h.e.SetSpatial(false)
	h.e.SetSpatial(true)
	eventually(t, "the spatial mix", func() bool { return h.e.Status().Spatial })
}

// The armed next item follows the setting too.
func TestSpatialToggleRearmsTheNextItem(t *testing.T) {
	h, _ := spatialHarness(t, false)
	if err := h.e.Play(ctx(t), "SN", 0); err != nil {
		t.Fatal(err)
	}
	h.e.SetNext(playback.Next{ID: "S1"})
	f := h.host(0)
	eventually(t, "the next item queued", func() bool { return f.queued() == 1 })
	h.e.SetSpatial(true)
	eventually(t, "the next item queued again as its spatial mix", func() bool {
		return h.fx.requests("/S1/spatial/audio-stereo-256-binaural.m3u8", "") == 1 && f.queued() == 2
	})
}

// A seek feeds from the segment that holds the position, each with its
// key: playback can start after the key change, and return before it.
func TestSpatialSeekAcrossTheKeyChange(t *testing.T) {
	h, _ := spatialHarness(t, true)
	if err := h.e.Play(ctx(t), "S1", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	eventually(t, "played from the start", func() bool { _, feeds, _ := f.snapshot(); return len(feeds) == 3 })
	for _, tc := range []struct {
		to   time.Duration
		want [][]byte
	}{
		{25 * time.Second, [][]byte{spatialKID2}},
		{5 * time.Second, [][]byte{spatialKID1, spatialKID2, spatialKID2}},
		{12 * time.Second, [][]byte{spatialKID2, spatialKID2}},
	} {
		_, before, _ := f.snapshot()
		if err := h.e.Seek(ctx(t), tc.to); err != nil {
			t.Fatal(err)
		}
		eventually(t, fmt.Sprintf("fed after a seek to %v", tc.to), func() bool { _, feeds, _ := f.snapshot(); return len(feeds)-len(before) == len(tc.want) })
		_, feeds, _ := f.snapshot()
		wantKIDs(t, feeds[len(before):], tc.want...)
		if st := h.e.Status(); !st.Spatial {
			t.Fatalf("a seek lost the status: %+v", st)
		}
	}
}

// Each key's session is renewed through Apple's licence server with its
// own key and slot token.
func TestSpatialRenewsEverySession(t *testing.T) {
	h, _ := spatialHarness(t, true)
	if err := h.e.Play(ctx(t), "S1", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	for _, sid := range []string{"s1", "s2"} {
		f.emit(cdmhost.Event{Kind: cdmhost.EventMessage, Session: sid, Type: int(widevine.LicenseRenewal), Data: []byte("renew")})
	}
	f.emit(cdmhost.Event{Kind: cdmhost.EventMessage, Session: "s9", Type: int(widevine.LicenseRenewal), Data: []byte("renew")})
	eventually(t, "both renewals sent", func() bool { return len(h.fx.licences()) == 4 })
	time.Sleep(50 * time.Millisecond) // one for the unknown session would land by now
	lics := h.fx.licences()
	if len(lics) != 4 {
		t.Fatalf("%d licence requests: %+v", len(lics), lics)
	}
	renewals := map[string]bool{}
	for _, l := range lics[2:] {
		if !l.Renewal || l.AdamID != "S1" {
			t.Errorf("renewal %+v", l)
		}
		renewals[l.URI] = true
	}
	if !renewals[widevineURI(spatialKID1)] || !renewals[widevineURI(spatialKID2)] {
		t.Errorf("renewals for %v, want one per key", renewals)
	}
	if st := h.e.Status(); st.State != playback.StatePlaying || !st.Spatial {
		t.Fatalf("a renewal disturbed playback: %+v", st)
	}
}

// A new host (a crash, an idle shutdown) holds no sessions: the item is
// licensed again, every key.
func TestSpatialRelicensesOnANewHost(t *testing.T) {
	h, _ := spatialHarness(t, true)
	if err := h.e.Play(ctx(t), "S1", 0); err != nil {
		t.Fatal(err)
	}
	h.host(0).exit(fmt.Errorf("killed"))
	eventually(t, "restarted", func() bool { return h.nhosts() == 2 })
	f := h.host(1)
	eventually(t, "licensed again and fed", func() bool { _, feeds, s := f.snapshot(); return s == 2 && len(feeds) == 3 })
	_, feeds, _ := f.snapshot()
	wantKIDs(t, feeds, spatialKID1, spatialKID2, spatialKID2)
	if n := len(h.fx.licences()); n != 4 {
		t.Errorf("%d licence requests, want 2 per host", n)
	}
	if !h.e.Status().Spatial {
		t.Errorf("status %+v", h.e.Status())
	}
}

// Into and out of a Spatial Audio song, a track change is armed and queued
// like any other (the host joins them gaplessly or crossfades; a 48 kHz
// rendition is resampled like the 44.1 kHz ones), and the status follows
// the item that is heard.
func TestSpatialGaplessAndCrossfadeBetweenSpatialAndStereo(t *testing.T) {
	h, _ := spatialHarness(t, true)
	h.e.SetCrossfade(5 * time.Second)
	if err := h.e.Play(ctx(t), "SN", 0); err != nil {
		t.Fatal(err)
	}
	if h.e.Status().Spatial {
		t.Fatal("a stereo song reports spatial")
	}
	f := h.host(0)
	for i, tc := range []struct {
		next     playback.Next
		fade     time.Duration
		kids     [][]byte
		spatial  bool
		fedCount int
	}{
		{next: playback.Next{ID: "S1", Gapless: true}, fade: 0, kids: [][]byte{spatialKID1, spatialKID2, spatialKID2}, spatial: true, fedCount: 3},
		{next: playback.Next{ID: "SN"}, fade: 5 * time.Second, kids: [][]byte{nil, nil, nil}, spatial: false, fedCount: 3},
		{next: playback.Next{ID: "S1"}, fade: 5 * time.Second, kids: [][]byte{spatialKID1, spatialKID2, spatialKID2}, spatial: true, fedCount: 3},
	} {
		h.e.SetNext(tc.next)
		eventually(t, fmt.Sprintf("%d: next queued", i), func() bool { return len(f.nextCalls()) == i+1 })
		q := f.nextCalls()[i]
		if q.fade != tc.fade || q.dur != trimmed {
			t.Fatalf("%d: queued %+v, want crossfade %v and duration %v", i, q, tc.fade, trimmed)
		}
		eventually(t, fmt.Sprintf("%d: next fed", i), func() bool { return f.fedTrack(q.track) == tc.fedCount })
		f.mu.Lock()
		var fed []feedCall
		for _, c := range f.feeds {
			if c.track == q.track {
				fed = append(fed, c)
			}
		}
		f.mu.Unlock()
		wantKIDs(t, fed, tc.kids...)
		drainEvents(h.e)
		f.emit(cdmhost.Event{Kind: cdmhost.EventAdvance, Track: q.track, Position: 20 * time.Millisecond, At: time.Now()})
		ev := nextEvent(t, h.e)
		if ev.Kind != playback.EventAdvance || ev.Status.ItemID != tc.next.ID || ev.Status.Spatial != tc.spatial {
			t.Fatalf("%d: advance event %+v, want %s spatial=%v", i, ev, tc.next.ID, tc.spatial)
		}
		// The next one is fed to its end (the fake host does not play it
		// out), so the item after it is queued behind it.
		eventually(t, fmt.Sprintf("%d: fed to the end", i), func() bool { h.e.mu.Lock(); defer h.e.mu.Unlock(); return h.e.cur.fed })
	}
}

// A refused or broken rendition of a song that is queued behind the current
// one falls back too: the queued item is the stereo stream.
func TestSpatialFallbackWhenQueued(t *testing.T) {
	h, _ := spatialHarness(t, true)
	if err := h.e.Play(ctx(t), "S1", 0); err != nil {
		t.Fatal(err)
	}
	h.e.SetNext(playback.Next{ID: "SF", Gapless: true})
	f := h.host(0)
	eventually(t, "SF queued", func() bool { return len(f.nextCalls()) == 1 })
	q := f.nextCalls()[0]
	eventually(t, "SF fed", func() bool { return f.fedTrack(q.track) == 3 })
	f.mu.Lock()
	var fed []feedCall
	for _, c := range f.feeds {
		if c.track == q.track {
			fed = append(fed, c)
		}
	}
	f.mu.Unlock()
	wantKIDs(t, fed, nil, nil, nil)
	f.emit(cdmhost.Event{Kind: cdmhost.EventAdvance, Track: q.track, Position: time.Millisecond, At: time.Now()})
	eventually(t, "SF current", func() bool { st := h.e.Status(); return st.ItemID == "SF" && !st.Spatial })
}
