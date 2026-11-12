package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/app/dbustest"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/mut"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/motion"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

type recorder struct {
	mu     sync.Mutex
	events []string
	data   []any
}

func (r *recorder) Emit(name string, data ...any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, name)
	r.data = append(r.data, data[0])
	return false
}

func TestCodeOf(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want ErrorCode
	}{
		{fmt.Errorf("sign in: %w", appleid.ErrInvalidCredentials), CodeInvalidCredentials},
		{&appleid.ServiceError{Status: 400, Code: "-21669", Message: "Incorrect verification code."}, CodeUnknown},
		{fmt.Errorf("x: %w", appleid.ErrUnsupported2FA), CodeUnsupported2FA},
		{mut.ErrTimeout, CodeTimeout},
		{mut.ErrCanceled, CodeCancelled},
		{fmt.Errorf("validate: %w", auth.ErrMUTRejected), CodeMUTRejected},
		{&applemusic.StatusError{Status: "403 Forbidden", Code: 403}, CodeUnauthorized},
		{&applemusic.StatusError{Status: "404 Not Found", Code: 404}, CodeNotFound},
		{fmt.Errorf("play: %w", applemusic.ErrNotPlayable), CodeNotPlayable},
		{fmt.Errorf("track catalog/1: %w", ErrNotReleased), CodeNotReleased},
		{library.ErrSyncRunning, CodeSyncRunning},
		{&url.Error{Op: "Get", URL: "https://api.music.apple.com", Err: errors.New("dial tcp: no route")}, CodeNetwork},
		{fmt.Errorf("delete: %w", ErrFolderNotEmpty), CodeFolderNotEmpty},
		{fmt.Errorf("%w: %w (3 items)", ErrInvalidArgument, ErrFolderNotEmpty), CodeFolderNotEmpty},
		{ErrNameRequired, CodeNameRequired},
		{ErrNotAppleMusicLink, CodeNotAppleMusicLink},
		{ErrTokenRequired, CodeTokenRequired},
		{fmt.Errorf("copy link: %w", ErrClipboard), CodeClipboard},
		{fmt.Errorf("%w: Last.fm: %w", ErrOpenBrowser, errors.New("xdg-open")), CodeBrowserOpen},
		{auth.ErrBrowserOpen, CodeBrowserOpen},
		{fmt.Errorf("%w: %w", auth.ErrSignOutIncomplete, auth.ErrKeyringTimeout), CodeSignOutIncomplete},
		{auth.ErrStoredUnreadable, CodeStoredUnreadable},
		{auth.ErrKeyringTimeout, CodeKeyring},
		{auth.ErrDevTokenRejected, CodeDevTokenRejected},
		{config.ErrNotExport, CodeSettingsNotExport},
		{config.ErrNewerExport, CodeSettingsNewerExport},
		{fmt.Errorf("%w: theme", config.ErrBadValue), CodeSettingsBadValue},
		{fmt.Errorf("%w: %w", ErrSettingsRead, os.ErrPermission), CodeSettingsRead},
		{fmt.Errorf("%w: %w", ErrSettingsWrite, os.ErrPermission), CodeSettingsWrite},
		{fmt.Errorf("fetch: %w", motion.ErrNoRendition), CodeNoRendition},
		{nil, ""},
		{errors.New("boom"), CodeUnknown},
	} {
		if got := CodeOf(tc.err); got != tc.want {
			t.Errorf("CodeOf(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestGateHoldsRequestsUntilOpen(t *testing.T) {
	var hits sync.WaitGroup
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer srv.Close()

	g := NewGate(time.Hour, nil)
	c := &http.Client{Transport: g.Transport(http.DefaultTransport)}
	var order []string
	var mu sync.Mutex
	g.OnOpen(func() { mu.Lock(); order = append(order, "first"); mu.Unlock() })
	g.OnOpen(func() { mu.Lock(); order = append(order, "second"); mu.Unlock() })

	done := make(chan error, 1)
	hits.Add(1)
	go func() {
		defer hits.Done()
		resp, err := c.Get(srv.URL)
		if err == nil {
			resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-done:
		t.Fatal("request completed before the gate opened")
	case <-time.After(50 * time.Millisecond):
	}

	// A request whose context ends while held fails without reaching the server.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if _, err := c.Do(req); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("held request with expired context: %v", err)
	}

	g.Open()
	g.Open()
	if err := <-done; err != nil {
		t.Fatalf("request after open: %v", err)
	}
	hits.Wait()
	ran := false
	g.OnOpen(func() { ran = true })
	if !ran {
		t.Error("OnOpen after Open did not run immediately")
	}
	if strings.Join(order, ",") != "first,second" {
		t.Errorf("OnOpen order = %v", order)
	}
}

func TestGateFallbackOpens(t *testing.T) {
	g := NewGate(10*time.Millisecond, nil)
	select {
	case <-g.opened:
	case <-time.After(time.Second):
		t.Fatal("gate did not open after the fallback delay")
	}
}

func newMapper(t *testing.T) mapper {
	t.Helper()
	c, err := artwork.New(artwork.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return mapper{art: c}
}

const tmpl = "https://is1-ssl.mzstatic.com/image/thumb/Music/x/{w}x{h}bb.jpg"

func TestMapLibrarySong(t *testing.T) {
	m := newMapper(t)
	s := m.libSong(library.Song{LibraryID: "i.abc", CatalogID: "1440", Name: "A", AlbumLibraryID: "l.1", ArtworkTemplate: tmpl, DurationMS: 1234})
	if s.ID != "i.abc" || s.Source != SourceLibrary || !s.Playable || s.AlbumID != "l.1" || s.DurationMs != 1234 {
		t.Errorf("library song = %+v", s)
	}
	if !strings.HasPrefix(s.Art.Small, "/art/") || !strings.HasSuffix(s.Art.Small, "/96x96.jpg") || !strings.HasSuffix(s.Art.Large, "/1000x1000.jpg") {
		t.Errorf("art = %+v", s.Art)
	}
	upload := m.libSong(library.Song{LibraryID: "i.up", Name: "Upload"})
	if !upload.Playable {
		t.Error("an upload (no catalog ID) is not playable: it plays from its library ID")
	}
	if upload.Art != (Art{}) {
		t.Errorf("no artwork gave %+v", upload.Art)
	}
	// A template outside Apple's CDN is never handed to the UI.
	if a := m.artOf("https://example.com/{w}x{h}.jpg"); a != (Art{}) {
		t.Errorf("foreign artwork host gave %+v", a)
	}
}

// Apple artwork carries its own shape in its URLs (wide editorial banners,
// video frames), except "cc" templates, which Apple crops square.
func TestArtworkKeepsAppleShape(t *testing.T) {
	m := newMapper(t)
	for _, tc := range []struct {
		a            applemusic.Artwork
		small, large string
	}{
		{applemusic.Artwork{URL: "https://is1-ssl.mzstatic.com/image/thumb/F/hero.png/{w}x{h}sr.{f}", Width: 4320, Height: 1800}, "/96x40.jpg", "/1000x417.jpg"},
		{applemusic.Artwork{URL: "https://is1-ssl.mzstatic.com/image/thumb/F/brick.png/{w}x{h}{c}.{f}", Width: 4320, Height: 1080}, "/96x24.jpg", "/1000x250.jpg"},
		{applemusic.Artwork{URL: "https://is1-ssl.mzstatic.com/image/thumb/V/frame.jpg/{w}x{h}mv.{f}", Width: 1920, Height: 1080}, "/96x54.jpg", "/1000x563.jpg"},
		{applemusic.Artwork{URL: "https://is1-ssl.mzstatic.com/image/thumb/F/chart.png/{w}x{h}cc.{f}", Width: 4320, Height: 1080}, "/96x96.jpg", "/1000x1000.jpg"},
		{applemusic.Artwork{URL: tmpl}, "/96x96.jpg", "/1000x1000.jpg"},
	} {
		a := m.artwork(&tc.a)
		if !strings.HasSuffix(a.Small, tc.small) || !strings.HasSuffix(a.Large, tc.large) {
			t.Errorf("%s %dx%d: art = %+v, want …%s and …%s", tc.a.URL, tc.a.Width, tc.a.Height, a, tc.small, tc.large)
		}
	}
	if a := m.artwork(nil); a != (Art{}) {
		t.Errorf("nil artwork gave %+v", a)
	}
}

func TestMapAppleSong(t *testing.T) {
	m := newMapper(t)
	var cat applemusic.Song
	cat.ID, cat.Type = "1440", "songs"
	cat.Attributes.Name = "Song"
	cat.Attributes.ContentRating = "explicit"
	cat.Attributes.GenreNames = []string{"Rock", "Music"}
	cat.Attributes.PlayParams = &applemusic.PlayParams{ID: "1440", Kind: "song"}
	cat.Attributes.Artwork = &applemusic.Artwork{URL: tmpl}
	cat.Relationships.Albums.Data = []applemusic.Album{{Resource: applemusic.Resource[applemusic.AlbumAttributes]{ID: "99"}}}
	s := m.amSong(cat)
	if s.Source != SourceCatalog || s.CatalogID != "1440" || s.LibraryID != "" || !s.Explicit || s.Genre != "Rock" || !s.Playable || s.AlbumID != "99" {
		t.Errorf("catalog song = %+v", s)
	}

	var lib applemusic.Song
	lib.ID, lib.Type = "i.x", "library-songs"
	lib.Attributes.PlayParams = &applemusic.PlayParams{ID: "i.x", IsLibrary: true, CatalogID: "1441"}
	s = m.amSong(lib)
	if s.Source != SourceLibrary || s.LibraryID != "i.x" || s.CatalogID != "1441" || !s.Playable {
		t.Errorf("library song = %+v", s)
	}
	lib.Attributes.PlayParams.CatalogID = ""
	if !m.amSong(lib).Playable {
		t.Error("an uploaded library song (no catalog ID) is not playable")
	}
	var noPlay applemusic.Song
	noPlay.ID, noPlay.Type = "1", "songs"
	if m.amSong(noPlay).Playable {
		t.Error("catalog song without playParams is playable")
	}
}

func TestParseMs(t *testing.T) {
	if parseMs("") != 0 || parseMs("garbage") != 0 {
		t.Error("unparsable timestamps should be 0")
	}
	if got := parseMs("2024-01-02T03:04:05Z"); got != time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC).UnixMilli() {
		t.Errorf("parseMs = %d", got)
	}
	if unixMs(time.Time{}) != 0 {
		t.Error("zero time should be 0")
	}
}

func TestSyncPolicy(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	all := func(success time.Time, mod func(*library.DomainStatus)) []library.DomainStatus {
		var ds []library.DomainStatus
		for _, d := range library.AllDomains {
			s := library.DomainStatus{Domain: d, LastSuccess: success}
			if mod != nil {
				mod(&s)
			}
			ds = append(ds, s)
		}
		return ds
	}
	for _, tc := range []struct {
		name string
		ds   []library.DomainStatus
		want bool
	}{
		{"never synced", nil, true},
		{"fresh", all(now.Add(-time.Hour), nil), false},
		{"stale", all(now.Add(-7*time.Hour), nil), true},
		{"one domain missing", all(now.Add(-time.Hour), nil)[1:], true},
		{"interrupted run", all(now.Add(-time.Hour), func(s *library.DomainStatus) {
			if s.Domain == library.DomainSongs {
				s.InProgress = true
			}
		}), true},
	} {
		st := syncStatus(tc.ds, false)
		if got := needsSync(st, now); got != tc.want {
			t.Errorf("%s: needsSync = %v, want %v (lastSuccess %d)", tc.name, got, tc.want, st.LastSuccess)
		}
	}
	// LastSuccess is the oldest domain's success.
	ds := all(now.Add(-time.Hour), func(s *library.DomainStatus) {
		if s.Domain == library.DomainRatings {
			s.LastSuccess = now.Add(-5 * time.Hour)
		}
	})
	if got := syncStatus(ds, true); got.LastSuccess != now.Add(-5*time.Hour).UnixMilli() || !got.Running {
		t.Errorf("syncStatus = %+v", got)
	}
}

func TestActivationEvents(t *testing.T) {
	rec := &recorder{}
	focused := 0
	o := &opener{emit: rec, focus: func() { focused++ }, log: discard()}

	// Launch arguments arrive before the UI listens: held.
	o.activate(app.Activation{Args: []string{"--verbose", "https://music.apple.com/gb/album/x/1440857781?i=1440857795", "https://example.com/"}}, false)
	if len(rec.events) != 0 || focused != 0 {
		t.Fatalf("emitted before first paint: %v (focused %d)", rec.events, focused)
	}
	o.markReady()
	if len(rec.events) != 1 || rec.events[0] != EventAppOpen {
		t.Fatalf("events after first paint = %v", rec.events)
	}
	if got := rec.data[0].(OpenTarget); got != (OpenTarget{Kind: "song", ID: "1440857795", AlbumID: "1440857781", Storefront: "gb"}) {
		t.Errorf("target = %+v", got)
	}

	// A forwarded launch focuses the window and emits at once.
	o.activate(app.Activation{Args: []string{"music://music.apple.com/us/artist/x/657515"}}, true)
	if focused != 1 || len(rec.events) != 2 || rec.data[1].(OpenTarget).Kind != "artist" {
		t.Errorf("forwarded: focused %d, events %v %v", focused, rec.events, rec.data)
	}
	// Unsupported links are dropped.
	o.activate(app.Activation{Args: []string{"https://music.apple.com/us/room/6449178938"}}, true)
	if len(rec.events) != 2 {
		t.Errorf("unsupported link emitted: %v", rec.data)
	}
}

// queuedCoordinator is a coordinator that has started items[start] on an
// engine that plays instantly, for the snapshots the media controls map.
func queuedCoordinator(t *testing.T, items []coordinator.Item, start int) *coordinator.Coordinator {
	t.Helper()
	coord, err := coordinator.New(coordinator.Options{Engine: &playEngine{events: make(chan playback.Event)}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	if err := coord.SetQueue(t.Context(), items, start, ""); err != nil {
		t.Fatal(err)
	}
	return coord
}

func TestMPRISSnapshot(t *testing.T) {
	item := coordinator.Item{ID: "1440", Title: "T", Duration: 3 * time.Minute}
	coord := queuedCoordinator(t, []coordinator.Item{item, item}, 1)
	if err := coord.SetRepeat(coordinator.RepeatOne); err != nil {
		t.Fatal(err)
	}
	snap := coord.Snapshot()
	snap.State = playback.StateLoading
	m := mprisSnapshot(snap, "file:///a.jpg")
	if m.Status != mpris.StatusPlaying || m.Loop != mpris.LoopTrack || m.Track == nil || m.Track.Length != time.Minute || m.Track.ArtURL != "file:///a.jpg" {
		t.Errorf("loading snapshot = %+v", m)
	}
	if m.CanGoNext || !m.CanGoPrevious || !m.CanSeek {
		t.Errorf("last item without repeat all: next %v previous %v seek %v", m.CanGoNext, m.CanGoPrevious, m.CanSeek)
	}
	if err := coord.SetRepeat(coordinator.RepeatAll); err != nil {
		t.Fatal(err)
	}
	snap = coord.Snapshot()
	snap.State = playback.StateEnded
	m = mprisSnapshot(snap, "")
	if !m.CanGoNext || m.Status != mpris.StatusPaused || m.Loop != mpris.LoopPlaylist {
		t.Errorf("ended snapshot = %+v", m)
	}
	if m := mprisSnapshot(coordinator.Snapshot{State: playback.StateIdle, Index: -1}, ""); m.Status != mpris.StatusStopped || m.Track != nil || m.CanPlay {
		t.Errorf("empty snapshot = %+v", m)
	}
}

// A live station has nothing to skip to, seek in, shuffle or repeat: MPRIS
// reports it, and clients that write the modes anyway are refused.
func TestMPRISLiveStation(t *testing.T) {
	live := coordinator.Item{ID: "ra.1445918457", Title: "KISS"}
	coord := queuedCoordinator(t, []coordinator.Item{live}, 0)
	coord.SetShuffle(true)
	if err := coord.SetRepeat(coordinator.RepeatAll); err != nil {
		t.Fatal(err)
	}
	snap := coord.Snapshot()
	m := mprisSnapshot(snap, "")
	if m.CanGoNext || m.CanGoPrevious || m.CanSeek || !m.CanPause || !m.CanPlay || !m.CanControl {
		t.Errorf("live station: next %v previous %v seek %v pause %v play %v control %v", m.CanGoNext, m.CanGoPrevious, m.CanSeek, m.CanPause, m.CanPlay, m.CanControl)
	}
	if m.Shuffle || m.Loop != mpris.LoopNone {
		t.Errorf("live station reports shuffle %v, loop %q", m.Shuffle, m.Loop)
	}

	p := NewMPRISPlayer(coord, func() {}, func() {}, discard())
	if err := p.SetShuffle(t.Context(), false); !errors.Is(err, coordinator.ErrLiveStation) {
		t.Errorf("SetShuffle on a live station = %v", err)
	}
	if err := p.SetLoopStatus(t.Context(), mpris.LoopNone); !errors.Is(err, coordinator.ErrLiveStation) {
		t.Errorf("SetLoopStatus on a live station = %v", err)
	}
	if got := coord.Snapshot(); !got.Shuffle || got.Repeat != coordinator.RepeatAll {
		t.Errorf("a refused write changed the modes: shuffle %v, repeat %q", got.Shuffle, got.Repeat)
	}
	if err := p.Stop(t.Context()); err != nil {
		t.Errorf("Stop on a live station = %v", err)
	}

	// Songs bring the modes back.
	song := coordinator.Item{ID: "1440", Title: "T", Duration: time.Minute}
	if err := coord.SetQueue(t.Context(), []coordinator.Item{song, song}, 0, ""); err != nil {
		t.Fatal(err)
	}
	m = mprisSnapshot(coord.Snapshot(), "")
	if !m.Shuffle || m.Loop != mpris.LoopPlaylist || !m.CanGoNext || !m.CanSeek {
		t.Errorf("after the station: shuffle %v, loop %q, next %v, seek %v", m.Shuffle, m.Loop, m.CanGoNext, m.CanSeek)
	}
	if err := p.SetShuffle(t.Context(), false); err != nil {
		t.Errorf("SetShuffle on songs = %v", err)
	}
}

// A station of songs keeps Next and the timeline, restarts on Previous, and
// refuses what makes no sense on an endless station like a live one does.
func TestMPRISSongStation(t *testing.T) {
	song := coordinator.Item{ID: "1440", Title: "T", Duration: time.Minute}
	coord := queuedCoordinator(t, []coordinator.Item{song, song}, 0)
	coord.SetShuffle(true)
	snap := coord.Snapshot()
	snap.Station = &coordinator.Station{ID: "ra.1", Name: "Hits"}
	snap.Controls = coordinator.Controls{Previous: true, Next: true, Seek: true}
	m := mprisSnapshot(snap, "")
	if !m.CanGoNext || !m.CanGoPrevious || !m.CanSeek || m.Shuffle || m.Loop != mpris.LoopNone {
		t.Errorf("station of songs: next %v previous %v seek %v shuffle %v loop %q", m.CanGoNext, m.CanGoPrevious, m.CanSeek, m.Shuffle, m.Loop)
	}
	if err := snap.Refusal(func(c coordinator.Controls) bool { return c.Repeat }); !errors.Is(err, coordinator.ErrStation) {
		t.Errorf("repeat refusal = %v", err)
	}
}

func TestMPRISSnapshotOnAir(t *testing.T) {
	coord := queuedCoordinator(t, []coordinator.Item{{ID: "ra.1445918457", Title: "KISS"}}, 0)
	snap := coord.Snapshot()
	if m := mprisSnapshot(snap, ""); m.Track.Title != "KISS" || m.Track.Artist != "" || m.CanSeek {
		t.Errorf("silent broadcaster = %+v", m.Track)
	}
	snap.OnAir = playback.OnAir{Title: "Espresso", Artist: "Sabrina Carpenter"}
	if m := mprisSnapshot(snap, ""); m.Track.Title != "Espresso" || m.Track.Artist != "Sabrina Carpenter" || m.Track.Album != "KISS" {
		t.Errorf("on air with artist = %+v", m.Track)
	}
	snap.OnAir = playback.OnAir{Title: "Sabrina Carpenter - Espresso"}
	if m := mprisSnapshot(snap, ""); m.Track.Title != "Sabrina Carpenter - Espresso" || m.Track.Artist != "KISS" || m.Track.Album != "KISS" {
		t.Errorf("on air without artist = %+v", m.Track)
	}
}

// MPRIS shows the cover of the song on air when the catalog has it, else
// the station's.
func TestMPRISArtTemplate(t *testing.T) {
	item := coordinator.Item{ID: "ra.1", Title: "KISS", Artwork: "https://is1-ssl.mzstatic.com/station/{w}x{h}.jpg"}
	snap := coordinator.Snapshot{State: playback.StatePlaying, Current: &coordinator.Entry{Item: item}}
	if got := mprisArtTemplate(snap); got != item.Artwork {
		t.Errorf("nothing on air: %q", got)
	}
	song := "https://is1-ssl.mzstatic.com/song/{w}x{h}.jpg"
	snap.OnAir = playback.OnAir{Title: "Song", Artist: "Artist", Art: song}
	if got := mprisArtTemplate(snap); got != song {
		t.Errorf("song on air: %q", got)
	}
	// A cover left over from a song the stream no longer names.
	snap.OnAir = playback.OnAir{Art: song}
	if got := mprisArtTemplate(snap); got != item.Artwork {
		t.Errorf("nothing named on air: %q", got)
	}
	if got := mprisArtTemplate(coordinator.Snapshot{}); got != "" {
		t.Errorf("nothing playing: %q", got)
	}
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

// A desktop shell or playerctl reads a live station off the bus: nothing to
// skip to or seek, shuffle and loop off, and writes to either are refused
// without touching the queue.
func TestMPRISLiveStationOverTheBus(t *testing.T) {
	live := coordinator.Item{ID: "ra.1445918457", Title: "KISS"}
	coord := queuedCoordinator(t, []coordinator.Item{live}, 0)
	addr := dbustest.Start(t)
	player := NewMPRISPlayer(coord, func() {}, func() {}, discard())
	srv, err := mpris.New(dbustest.Conn(t, addr), player, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	obj := dbustest.Conn(t, addr).Object(mpris.BusName, mpris.ObjectPath)
	const iface = "org.mpris.MediaPlayer2.Player"
	prop := func(name string) any {
		t.Helper()
		v, err := obj.GetProperty(iface + "." + name)
		if err != nil {
			t.Fatal(err)
		}
		return v.Value()
	}

	for _, name := range []string{"CanGoNext", "CanGoPrevious", "CanSeek", "Shuffle"} {
		if prop(name) != false {
			t.Errorf("%s = %v on a live station, want false", name, prop(name))
		}
	}
	for _, name := range []string{"CanPlay", "CanPause", "CanControl"} {
		if prop(name) != true {
			t.Errorf("%s = %v on a live station, want true", name, prop(name))
		}
	}
	if prop("LoopStatus") != "None" {
		t.Errorf("LoopStatus = %v", prop("LoopStatus"))
	}
	if err := obj.SetProperty(iface+".Shuffle", dbus.MakeVariant(true)); err == nil {
		t.Error("a live station accepted Shuffle")
	}
	if err := obj.SetProperty(iface+".LoopStatus", dbus.MakeVariant("Playlist")); err == nil {
		t.Error("a live station accepted LoopStatus")
	}
	for _, m := range []string{"Next", "Previous"} {
		if err := obj.Call(iface+"."+m, 0).Err; err != nil {
			t.Errorf("%s: %v", m, err)
		}
	}
	if s := coord.Snapshot(); s.Shuffle || s.Repeat != coordinator.RepeatOff || s.Current == nil || s.Current.Item.ID != live.ID {
		t.Errorf("the bus changed the station's queue: shuffle %v, repeat %q, current %+v", s.Shuffle, s.Repeat, s.Current)
	}
}
