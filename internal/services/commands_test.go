package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

type commandEnv struct {
	e        *Env
	srv      *commandServer
	play     *PlaybackService
	eng      *pausingEngine
	rec      *recorder
	apple    *fakeState
	settings *config.Store
	focused  int
	// auth stays restoring until signIn or signOut starts it with creds.
	auth  *auth.Manager
	creds store.Store
}

// signIn starts the account with a stored token and waits until it is
// signed in.
func (ce *commandEnv) signIn(t *testing.T) {
	ce.start(t, &recordStore{rec: store.Record{Token: "mut", ObtainedAt: time.Now()}}, auth.StateSignedIn)
}

// signOut starts the account with nothing stored.
func (ce *commandEnv) signOut(t *testing.T) { ce.start(t, &emptyStore{}, auth.StateSignedOut) }

func (ce *commandEnv) start(t *testing.T, creds store.Store, want auth.State) {
	t.Helper()
	ce.creds = creds
	ce.auth.Start()
	for deadline := time.Now().Add(5 * time.Second); ce.auth.Status().State != want; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("auth = %+v, want %s", ce.auth.Status(), want)
		}
	}
}

// emptyStore holds no token.
type emptyStore struct{ recordStore }

func (*emptyStore) Get() (store.Record, error) { return store.Record{}, store.ErrNotFound }

// pausingEngine is playEngine with a pause that pauses.
type pausingEngine struct{ *playEngine }

func (e *pausingEngine) Pause(context.Context) error  { e.set(playback.StatePaused); return nil }
func (e *pausingEngine) Resume(context.Context) error { e.set(playback.StatePlaying); return nil }

func (e *pausingEngine) set(s playback.State) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.st.State, e.st.UpdatedAt = s, time.Now()
}

// newCommandEnv builds the command server over the services it uses, a
// coordinator with an instant engine, two library songs (i.100 with
// catalog ID 100, i.101) and a fake Apple API: fakeState for ratings,
// plus a catalogue search for "abbey".
func newCommandEnv(t *testing.T, private privatemode.Options) *commandEnv {
	t.Helper()
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	for _, tr := range []struct{ lib, cat, name string }{{"i.100", "100", "Come Together"}, {"i.101", "101", "Something"}} {
		if _, err := d.W.ExecContext(ctx, `INSERT INTO tracks (library_id, catalog_id, name, artist_name, album_name, duration_ms, updated_at) VALUES (?, ?, ?, 'The Beatles', 'Abbey Road', 259000, 0)`,
			tr.lib, tr.cat, tr.name); err != nil {
			t.Fatal(err)
		}
	}
	ce := &commandEnv{apple: &fakeState{}, rec: &recorder{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/catalog/gb/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"results":{
			"top":{"data":[{"id":"1441164426","type":"albums"},{"id":"1441164430","type":"songs"}]},
			"album":{"data":[{"id":"1441164426","type":"albums"},{"id":"2","type":"albums"}]},
			"song":{"data":[{"id":"1441164430","type":"songs"},{"id":"3","type":"songs"}]}},
			"resources":{
			"albums":{"1441164426":{"id":"1441164426","type":"albums","attributes":{"name":"Abbey Road","artistName":"The Beatles","url":"https://music.apple.com/gb/album/abbey-road/1441164426"}},
				"2":{"id":"2","type":"albums","attributes":{"name":"Abbey Road (Super Deluxe)","artistName":"The Beatles"}}},
			"songs":{"1441164430":{"id":"1441164430","type":"songs","attributes":{"name":"Come Together","artistName":"The Beatles","playParams":{"id":"1441164430","kind":"song"}}},
				"3":{"id":"3","type":"songs","attributes":{"name":"Abbey Road Medley","artistName":"Someone","contentRating":"explicit","playParams":{"id":"3","kind":"song"}}}}},
			"meta":{"results":{"order":["top","album","song"]}}}`)
	})
	mux.HandleFunc("/v1/me/storefront", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[{"id":"gb","type":"storefronts","attributes":{"name":"United Kingdom","defaultLanguageTag":"en-GB","explicitContentPolicy":"allowed"}}]}`)
	})
	mux.Handle("/", ce.apple)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	ce.creds = &recordStore{}
	ce.auth, err = auth.NewManager(auth.Config{
		HTTP: srv.Client(), Dev: testDev{}, APIBase: srv.URL,
		Open:     func() (store.Store, error) { return ce.creds, nil },
		Accounts: AccountStore{DB: d}, Logger: discard(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ce.auth.Close)
	authm := ce.auth
	api := applemusic.New(srv.Client(), testDev{}, authm.UserToken)
	api.Base, api.AMPBase = srv.URL, srv.URL
	ce.settings, err = config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ce.settings.Update(func(s *config.Settings) { s.AutoPlay = false }); err != nil {
		t.Fatal(err)
	}
	ce.eng = &pausingEngine{&playEngine{events: make(chan playback.Event)}}
	coord, err := coordinator.New(coordinator.Options{Engine: ce.eng})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	if err := coord.SetVolume(ctx, 1); err != nil { // as main applies the saved volume
		t.Fatal(err)
	}
	sf := func() string { return "gb" }
	ce.e = &Env{Deps: Deps{
		Settings: ce.settings, DB: d, Auth: authm, API: api, Library: library.New(d, api),
		Catalog: catalog.New(d, api, sf, nil), Coord: coord, Engine: ce.eng, Private: privatemode.New(private),
		Storefront: sf, Emit: ce.rec, Logger: discard(), Now: time.Now, Gate: NewGate(time.Hour, discard()),
	}, members: newMembership(d)}
	ce.play = newPlaybackService(ce.e)
	lib := &LibraryService{o: ce.e, m: ce.e.m}
	search := newSearchService(ce.e)
	open := &opener{emit: ce.rec, focus: func() { ce.focused++ }, log: discard()}
	open.markReady()
	ce.srv = newCommandServer(ce.e, open, []application.Service{
		application.NewService(ce.play), application.NewService(lib), application.NewService(search),
	})
	t.Cleanup(ce.e.close)
	return ce
}

// call runs a command with its arguments.
func (ce *commandEnv) call(t *testing.T, cmd string, args ...string) (any, error) {
	t.Helper()
	return ce.srv.Call(t.Context(), cli.Request{Command: cmd, Args: args})
}

func (ce *commandEnv) status(t *testing.T, cmd string, args ...string) cli.Status {
	t.Helper()
	res, err := ce.call(t, cmd, args...)
	if err != nil {
		t.Fatalf("%s %v: %v", cmd, args, err)
	}
	return res.(cli.Status)
}

func code(err error) string {
	var e *cli.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestCommandsPlayback(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	if st := ce.status(t, "status"); !st.Running || st.State != cli.StateIdle || st.Track != nil || !st.Private || st.Repeat != "off" {
		t.Fatalf("idle status = %+v", st)
	}
	if res, _ := ce.call(t, "queue"); !slices.Equal(res.(cli.Queue).Tracks, []cli.QueueTrack{}) || res.(cli.Queue).Index != -1 {
		t.Errorf("empty queue = %+v", res)
	}
	refs := []TrackRef{{ID: "i.100", Source: SourceLibrary}, {ID: "i.101", Source: SourceLibrary}}
	if err := ce.play.PlayItems(t.Context(), refs, 0, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
	st := ce.status(t, "status")
	if st.State != cli.StatePlaying || st.Track == nil || st.Track.ID != "100" || st.Track.Title != "Come Together" ||
		st.Track.Artist != "The Beatles" || st.Track.Album != "Abbey Road" || st.Track.Duration != 60 {
		t.Fatalf("playing status = %+v (track %+v)", st, st.Track)
	}
	if st := ce.status(t, "pause"); st.State != cli.StatePaused {
		t.Errorf("pause → %+v", st)
	}
	if st := ce.status(t, "toggle"); st.State != cli.StatePlaying {
		t.Errorf("toggle → %+v", st)
	}
	if st := ce.status(t, "next"); st.Track == nil || st.Track.ID != "101" {
		t.Errorf("next → %+v", st.Track)
	}
	if got := ce.eng.played(); !slices.Equal(got, []string{"100", "101"}) {
		t.Errorf("engine played %v", got)
	}
	res, err := ce.call(t, "queue")
	if err != nil {
		t.Fatal(err)
	}
	if q := res.(cli.Queue); q.Index != 1 || len(q.Tracks) != 2 || q.Tracks[0].Title != "Come Together" || q.Tracks[1].Title != "Something" {
		t.Errorf("queue = %+v", q)
	}
}

func TestCommandsVolume(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	for _, tc := range []struct {
		args []string
		want int
	}{
		{nil, 100}, {[]string{"70"}, 70}, {[]string{"-25"}, 45}, {[]string{"+80"}, 100}, {[]string{"-200"}, 0}, {[]string{"35"}, 35}, {nil, 35},
	} {
		res, err := ce.call(t, "volume", tc.args...)
		if err != nil || res.(cli.Volume).Volume != tc.want {
			t.Errorf("volume %v = %+v, %v; want %d", tc.args, res, err, tc.want)
		}
	}
	if v := ce.e.Coord.Snapshot().Volume; v != 0.35 {
		t.Errorf("coordinator volume = %v", v)
	}
	ce.e.close() // flushes the volume into the settings, as on quit
	if v := ce.settings.Get().Volume; v != 0.35 {
		t.Errorf("saved volume = %v", v)
	}
}

func TestCommandsPrivate(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	for _, tc := range []struct {
		arg string
		on  bool
	}{{"status", false}, {"on", true}, {"toggle", false}, {"toggle", true}, {"off", false}} {
		res, err := ce.call(t, "private", tc.arg)
		if err != nil || res.(cli.Private) != (cli.Private{On: tc.on, Source: cli.SourceSetting, Saved: tc.on}) {
			t.Errorf("private %s = %+v, %v", tc.arg, res, err)
		}
	}
	forced := newCommandEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	if _, err := forced.call(t, "private", "off"); code(err) != CodePrivateForced {
		t.Errorf("forced off: %v", err)
	}
	dev := newCommandEnv(t, privatemode.Options{Dev: true})
	if res, _ := dev.call(t, "private"); res.(cli.Private) != (cli.Private{On: true, Source: cli.SourceDevelopment}) {
		t.Errorf("development = %+v", res)
	}
}

func TestCommandsLike(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	// While Sona is still reading the stored token (a slow keyring) the
	// account isn't known yet: nothing goes to Apple, and the CLI says so.
	if _, err := ce.call(t, "like", "status"); code(err) != cli.CodeAccountLoading {
		t.Fatalf("like while restoring: %v", err)
	}
	out := newCommandEnv(t, privatemode.Options{})
	out.signOut(t)
	if _, err := out.call(t, "like"); code(err) != CodeNotSignedIn {
		t.Fatalf("like signed out: %v", err)
	}
	if r := slices.DeleteFunc(append(ce.apple.take(), out.apple.take()...), func(r string) bool { return r[:3] == "GET" }); len(r) != 0 {
		t.Fatalf("requests without an account: %v", r)
	}
	ce.signIn(t)
	if _, err := ce.call(t, "like"); code(err) != cli.CodeNothingPlaying {
		t.Fatalf("like with nothing playing: %v", err)
	}
	if err := ce.play.PlayItems(t.Context(), []TrackRef{{ID: "i.100", Source: SourceLibrary}}, 0, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
	ce.apple.take()
	for _, tc := range []struct {
		arg   string
		liked bool
		req   string // the rating request sent to Apple
	}{
		{"status", true, ""}, // Apple's answer: song 100 is a favourite
		{"off", false, "DELETE /v1/me/favorites"},
		{"status", false, ""},
		{"toggle", true, "POST /v1/me/favorites"},
	} {
		res, err := ce.call(t, "like", tc.arg)
		if err != nil {
			t.Fatalf("like %s: %v", tc.arg, err)
		}
		if l := res.(cli.Like); l.Liked != tc.liked || l.Track.ID != "100" || l.Track.Title != "Come Together" {
			t.Errorf("like %s = %+v", tc.arg, l)
		}
		var sent []string
		for _, r := range ce.apple.take() {
			if r[:3] != "GET" {
				sent = append(sent, r)
			}
		}
		if want := slices.DeleteFunc([]string{tc.req}, func(s string) bool { return s == "" }); !slices.Equal(sent, want) {
			t.Errorf("like %s sent %v, want %v", tc.arg, sent, want)
		}
	}
	// The frontend heard each change, as the library ref it shows.
	ce.rec.mu.Lock()
	defer ce.rec.mu.Unlock()
	var states []ItemState
	for i, ev := range ce.rec.events {
		if ev == EventItemState {
			states = append(states, ce.rec.data[i].(ItemState))
		}
	}
	if len(states) != 2 || states[0].ID != "i.100" || states[0].Rating != 0 || states[1].Rating != 1 {
		t.Errorf("item-state events = %+v", states)
	}
}

func TestCommandsOpen(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	res, err := ce.call(t, "open", "https://music.apple.com/gb/album/abbey-road/1441164426?i=1441164430")
	if err != nil || res.(cli.Opened) != (cli.Opened{Kind: "song", ID: "1441164430", Storefront: "gb"}) {
		t.Fatalf("open = %+v, %v", res, err)
	}
	if ce.focused != 1 {
		t.Errorf("window raised %d times", ce.focused)
	}
	ce.rec.mu.Lock()
	var opened []OpenTarget
	for i, ev := range ce.rec.events {
		if ev == EventAppOpen {
			opened = append(opened, ce.rec.data[i].(OpenTarget))
		}
	}
	ce.rec.mu.Unlock()
	if len(opened) != 1 || opened[0] != (OpenTarget{Kind: "song", ID: "1441164430", AlbumID: "1441164426", Storefront: "gb"}) {
		t.Errorf("app:open = %+v", opened)
	}
	if _, err := ce.call(t, "open", "https://example.com/album/1"); code(err) != cli.CodeBadLink {
		t.Errorf("foreign link: %v", err)
	}
}

func TestCommandsSearch(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	res, err := ce.srv.Call(t.Context(), cli.Request{Command: "search", Args: []string{"abbey"}, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	got := res.(cli.SearchResults)
	var names []string
	for _, r := range got.Results {
		names = append(names, r.Kind+":"+r.Name)
	}
	// Grouped by kind in Apple's order, the top results not repeated.
	want := []string{"album:Abbey Road", "album:Abbey Road (Super Deluxe)", "song:Come Together", "song:Abbey Road Medley"}
	if got.Term != "abbey" || !slices.Equal(names, want) {
		t.Errorf("search = %q %v", got.Term, names)
	}
	if r := got.Results[0]; r.URL != "https://music.apple.com/gb/album/abbey-road/1441164426" || r.Subtitle != "The Beatles" || r.ID != "1441164426" {
		t.Errorf("first result = %+v", r)
	}
	if !got.Results[3].Explicit {
		t.Errorf("explicit flag lost: %+v", got.Results[3])
	}
	res, _ = ce.srv.Call(t.Context(), cli.Request{Command: "search", Args: []string{"abbey"}, Kind: "song", Limit: 1})
	if r := res.(cli.SearchResults).Results; len(r) != 1 || r[0].Name != "Come Together" {
		t.Errorf("--type song --limit 1 = %+v", r)
	}
}

// The status stream follows the coordinator and private listening, drops
// position-only changes and ends when Sona closes; commands then report
// that it is shutting down.
func TestCommandsWatch(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	updates, cancel := ce.srv.Watch()
	defer cancel()
	next := func() cli.Status {
		t.Helper()
		select {
		case s, ok := <-updates:
			if !ok {
				t.Fatal("stream ended")
			}
			return s
		case <-time.After(5 * time.Second):
			t.Fatal("no status")
		}
		return cli.Status{}
	}
	if s := next(); s.State != cli.StateIdle {
		t.Fatalf("first = %+v", s)
	}
	if err := ce.play.PlayItems(t.Context(), []TrackRef{{ID: "i.100", Source: SourceLibrary}}, 0, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
	for s := next(); s.State != cli.StatePlaying; s = next() {
	}
	if _, err := ce.call(t, "private", "on"); err != nil {
		t.Fatal(err)
	}
	for s := next(); !s.Private; s = next() {
	}
	ce.e.close()
	for {
		select {
		case _, ok := <-updates:
			if ok {
				continue
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the stream did not end when Sona closed")
		}
		break
	}
	if _, err := ce.call(t, "status"); code(err) != cli.CodeShuttingDown {
		t.Errorf("after close: %v", err)
	}
}
