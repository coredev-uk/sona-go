package activity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

type dev struct{}

func (dev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev-jwt", Expires: time.Now().Add(time.Hour)}, nil
}
func (dev) Invalidate() {}

type rewrite struct{ srv *httptest.Server }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(r.srv.URL)
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = u.Scheme, u.Host
	return http.DefaultTransport.RoundTrip(req)
}

// apple records play-activity posts; status decides the answer.
type apple struct {
	mu     sync.Mutex
	status int
	events []map[string]any
	posts  int
}

func (a *apple) setStatus(s int) { a.mu.Lock(); a.status = s; a.mu.Unlock() }

func (a *apple) got() []map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]map[string]any(nil), a.events...)
}

func newApple(t *testing.T) (*apple, *applemusic.Client) {
	a := &apple{status: 200}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/storefront":
			io.WriteString(w, `{"data":[{"id":"gb","type":"storefronts"}]}`)
		case "/play":
			var body struct {
				Data []map[string]any `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			a.mu.Lock()
			defer a.mu.Unlock()
			a.posts++
			w.WriteHeader(a.status)
			if a.status < 300 {
				a.events = append(a.events, body.Data...)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return a, applemusic.New(&http.Client{Transport: rewrite{srv}}, dev{}, func() string { return "mut-secret" })
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

var song = coordinator.Item{ID: "1109715168", Title: "Weird Fishes", Duration: 318 * time.Second}

func act(kind coordinator.ActivityKind, pos time.Duration) coordinator.Activity {
	return coordinator.Activity{Kind: kind, Item: song, Position: pos, Duration: song.Duration, At: time.Now()}
}

func TestListenBecomesMusicKitEvents(t *testing.T) {
	a, client := newApple(t)
	r, err := New(Options{Client: client, AppVersion: "0.1"})
	if err != nil {
		t.Fatal(err)
	}
	r.Observe(act(coordinator.ActivityStart, 0))
	seek := act(coordinator.ActivitySeek, 90*time.Second)
	seek.From = 12 * time.Second
	r.Observe(seek)
	end := act(coordinator.ActivityEnd, 100*time.Second)
	end.Reason = coordinator.EndSkipForward
	r.Observe(end)
	r.Observe(end) // a second end without a start is ignored
	eventually(t, "4 events", func() bool { return len(a.got()) == 4 })
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	type ev struct {
		typ, reason      any
		start, endPos    any
		hint, storefront any
	}
	want := []ev{
		{1.0, nil, 0.0, nil, 0.0, "gb"},
		{0.0, 12.0, 0.0, 12000.0, nil, "gb"},     // SCRUB_BEGIN at the old position
		{0.0, 13.0, 12000.0, 90000.0, nil, "gb"}, // SCRUB_END at the new one
		{0.0, 2.0, 90000.0, 100000.0, nil, "gb"}, // skipped forwards
	}
	for i, e := range a.got() {
		got := ev{e["event-type"], e["end-reason-type"], e["start-position-in-milliseconds"], e["end-position-in-milliseconds"], e["event-reason-hint-type"], e["store-front"]}
		if got != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, got, want[i])
		}
		if ids := e["ids"].(map[string]any); ids["subscription-adam-id"] != song.ID {
			t.Errorf("event %d ids %v", i, ids)
		}
		if e["media-duration-in-milliseconds"] != 318000.0 || e["user-token"] != "mut-secret" || e["developer-token"] != "dev-jwt" {
			t.Errorf("event %d: %v", i, e)
		}
		if bv, _ := e["build-version"].(string); !strings.HasPrefix(bv, "Sona/0.1 Linux/0.0 model/Linux ") {
			t.Errorf("build-version %q", bv)
		}
	}
}

func TestLibraryItemUsesCloudID(t *testing.T) {
	a, client := newApple(t)
	r, _ := New(Options{Client: client, Storefront: func() string { return "us" }})
	defer r.Close(context.Background())
	r.Observe(coordinator.Activity{Kind: coordinator.ActivityStart, Item: coordinator.Item{ID: "i.AbC123"}, Duration: time.Minute})
	eventually(t, "event", func() bool { return len(a.got()) == 1 })
	e := a.got()[0]
	if ids := e["ids"].(map[string]any); ids["cloud-id"] != "i.AbC123" || ids["subscription-adam-id"] != nil || e["store-front"] != "us" {
		t.Fatalf("%v", e)
	}
}

func TestUndeliveredEventsSurviveRestart(t *testing.T) {
	a, client := newApple(t)
	a.setStatus(http.StatusServiceUnavailable)
	path := filepath.Join(t.TempDir(), "play-activity.json")
	r, _ := New(Options{Client: client, StatePath: path})
	r.Observe(act(coordinator.ActivityStart, 0))
	exit := act(coordinator.ActivityEnd, 40*time.Second)
	exit.Reason = coordinator.EndExited
	r.Observe(exit)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	if strings.Contains(string(b), "mut-secret") || strings.Contains(string(b), "dev-jwt") || strings.Contains(string(b), "token") {
		t.Fatalf("credentials persisted: %s", b)
	}

	a.setStatus(200)
	r2, _ := New(Options{Client: client, StatePath: path})
	eventually(t, "saved events delivered", func() bool { return len(a.got()) == 2 })
	if got := a.got()[1]; got["end-reason-type"] != 17.0 || got["end-position-in-milliseconds"] != 40000.0 {
		t.Fatalf("exit event %v", got)
	}
	if err := r2.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("state file left behind: %v", err)
	}
}

func TestRetriesTransientFailuresAndDropsRejected(t *testing.T) {
	a, client := newApple(t)
	a.setStatus(http.StatusBadGateway)
	r, _ := newReporter(Options{Client: client}, func(int) time.Duration { return 10 * time.Millisecond })
	defer r.Close(context.Background())
	r.Observe(act(coordinator.ActivityStart, 0))
	eventually(t, "retries", func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.posts >= 2 })
	a.setStatus(200)
	eventually(t, "delivery after retry", func() bool { return len(a.got()) == 1 })

	a.setStatus(http.StatusBadRequest)
	end := act(coordinator.ActivityEnd, time.Second)
	end.Reason = coordinator.EndPaused
	r.Observe(end)
	eventually(t, "rejected batch dropped", func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.queue) == 0
	})
}

// A send in flight when Close starts must finish, not be cancelled and sent
// again: Apple may already have accepted it, and would count the listen twice.
func TestCloseDoesNotResendInFlightBatch(t *testing.T) {
	var (
		mu       sync.Mutex
		received int
	)
	arrived, release := make(chan struct{}, 4), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/play" {
			_, _ = io.WriteString(w, `{"data":[{"id":"gb","type":"storefronts"}]}`)
			return
		}
		var body struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		received += len(body.Data)
		mu.Unlock()
		arrived <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	client := applemusic.New(&http.Client{Transport: rewrite{srv}}, dev{}, func() string { return "mut" })
	r, _ := New(Options{Client: client, Storefront: func() string { return "gb" }})
	r.Observe(act(coordinator.ActivityStart, 0))
	<-arrived

	closed := make(chan error, 1)
	go func() { closed <- r.Close(context.Background()) }()
	time.Sleep(50 * time.Millisecond) // Close is waiting on the send
	close(release)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if received != 1 {
		t.Fatalf("Apple received %d events, want the one start once", received)
	}
}

func TestEventsCarryContainers(t *testing.T) {
	a, client := newApple(t)
	r, err := New(Options{Client: client, AppVersion: "0.1"})
	if err != nil {
		t.Fatal(err)
	}
	mode := coordinator.PlayMode{Shuffle: true, Repeat: coordinator.RepeatAll, AutoPlay: coordinator.AutoPlayOn}
	play := func(id string, c coordinator.Container) {
		it := song
		it.ID, it.Container = id, c
		r.Observe(coordinator.Activity{Kind: coordinator.ActivityStart, Item: it, Duration: it.Duration, At: time.Now(), Mode: mode})
		r.Observe(coordinator.Activity{Kind: coordinator.ActivityEnd, Item: it, Position: time.Minute, Duration: it.Duration, Reason: coordinator.EndSkipForward, At: time.Now(), Mode: mode})
	}
	album := coordinator.Container{Kind: coordinator.ContainerAlbum, ID: "1109714933"}
	play("1", album)
	play("2", album) // same album: no container change
	play("3", coordinator.Container{Kind: coordinator.ContainerAlbum, ID: "l.AbC123"})
	play("4", coordinator.Container{Kind: coordinator.ContainerPlaylist, ID: "pl.u-xyz"})
	play("5", coordinator.Container{Kind: coordinator.ContainerPlaylist, ID: "p.Lib1"})
	play("6", coordinator.Container{Kind: coordinator.ContainerStation, ID: "ra.1234"})
	play("7", coordinator.Container{Kind: coordinator.ContainerStation, ID: "ra.st.5678"})
	play("8", coordinator.Container{})
	eventually(t, "16 events", func() bool { return len(a.got()) == 16 })
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	type want struct {
		typ     float64
		ids     map[string]any
		feature string
		hint    any
	}
	wants := []want{
		{3, map[string]any{"album-adam-id": "1109714933"}, "album", 1.0},
		{3, map[string]any{"album-adam-id": "1109714933"}, "album", 0.0},
		{3, map[string]any{"cloud-album-id": "l.AbC123"}, "album", 1.0},
		{2, map[string]any{"global-playlist-id": "pl.u-xyz"}, "playlist", 1.0},
		{2, map[string]any{"universal-library-id": "p.Lib1"}, "playlist", 1.0},
		{1, map[string]any{"station-id": "ra.1234"}, "station", 1.0},
		{1, map[string]any{"station-id": "ra.st.5678", "station-personalized-id": "5678"}, "station", 1.0},
		{0, nil, "music_kit-integration", 0.0},
	}
	got := a.got()
	for i, w := range wants {
		for j, e := range got[2*i : 2*i+2] { // the start and its end
			ids, _ := e["container-ids"].(map[string]any)
			if e["container-type"] != w.typ || !mapsEqual(ids, w.ids) || e["feature-name"] != w.feature {
				t.Errorf("play %d event %d: type %v ids %v feature %v", i+1, j, e["container-type"], e["container-ids"], e["feature-name"])
			}
			pm, _ := e["play-mode"].(map[string]any)
			if pm["auto-play-mode"] != 2.0 || pm["repeat-play-mode"] != 3.0 || pm["shuffle-play-mode"] != 2.0 {
				t.Errorf("play %d event %d: play-mode %v", i+1, j, e["play-mode"])
			}
		}
		if h := got[2*i]["event-reason-hint-type"]; h != w.hint {
			t.Errorf("play %d: hint %v, want %v", i+1, h, w.hint)
		}
	}
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
