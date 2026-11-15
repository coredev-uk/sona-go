package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// playEngine plays instantly and records what it was asked to play.
type playEngine struct {
	mu     sync.Mutex
	st     playback.Status
	plays  []string
	events chan playback.Event
}

func (e *playEngine) Play(_ context.Context, id string, _ time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.plays = append(e.plays, id)
	e.st = playback.Status{State: playback.StatePlaying, ItemID: id, Duration: time.Minute, Volume: 1, UpdatedAt: time.Now()}
	return nil
}

func (e *playEngine) played() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.plays)
}

func (e *playEngine) Status() playback.Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st
}
func (e *playEngine) Preload(context.Context, string) error     { return nil }
func (e *playEngine) SetNext(playback.Next)                     {}
func (e *playEngine) Pause(context.Context) error               { return nil }
func (e *playEngine) Resume(context.Context) error              { return nil }
func (e *playEngine) Seek(context.Context, time.Duration) error { return nil }
func (e *playEngine) SetVolume(context.Context, float64) error  { return nil }
func (e *playEngine) Stop(context.Context) error                { return nil }
func (e *playEngine) Events() <-chan playback.Event             { return e.events }
func (e *playEngine) Close() error                              { return nil }
func (e *playEngine) Capabilities() playback.Capabilities {
	return playback.Capabilities{Kind: playback.KindCDM, Volume: true}
}

type testDev struct{}

func (testDev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev", Expires: time.Now().Add(time.Hour)}, nil
}
func (testDev) Invalidate() {}

// recordStore holds the stored Music User Token record.
type recordStore struct{ rec store.Record }

func (s *recordStore) Get() (store.Record, error) { return s.rec, nil }
func (s *recordStore) Set(r store.Record) error   { s.rec = r; return nil }
func (s *recordStore) Delete() error              { return nil }
func (s *recordStore) Backend() store.Backend     { return store.BackendFile }

// A restricted account (Apple's itre flag on the stored token, with the
// account's storefront validated against a fixture) makes playback skip
// explicit songs in Go, whatever the UI sends.
func TestRestrictedAccountSkipsExplicitSongs(t *testing.T) {
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	for _, tr := range []struct{ lib, cat, rating string }{
		{"i.clean1", "101", "clean"}, {"i.explicit", "102", "explicit"}, {"i.clean2", "103", ""},
	} {
		if _, err := d.W.ExecContext(ctx, `INSERT INTO tracks (library_id, catalog_id, name, content_rating, duration_ms, updated_at) VALUES (?, ?, ?, ?, 60000, 0)`,
			tr.lib, tr.cat, tr.lib, tr.rating); err != nil {
			t.Fatal(err)
		}
	}
	apple := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/storefront" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"us","type":"storefronts","attributes":{"name":"United States","defaultLanguageTag":"en-US","explicitContentPolicy":"opt-in"}}]}`))
	}))
	t.Cleanup(apple.Close)
	creds := &recordStore{rec: store.Record{Token: "mut", ObtainedAt: time.Now(), Restricted: true}}
	authm, err := auth.NewManager(auth.Config{
		HTTP: apple.Client(), Dev: testDev{}, APIBase: apple.URL,
		Open:     func() (store.Store, error) { return creds, nil },
		Accounts: AccountStore{DB: d},
		Logger:   discard(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authm.Close)
	settings, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	// No Apple API here: AutoPlay would ask it for a station.
	if _, err := settings.Update(func(s *config.Settings) { s.AutoPlay = false }); err != nil {
		t.Fatal(err)
	}
	eng := &playEngine{events: make(chan playback.Event)}
	coord, err := coordinator.New(coordinator.Options{Engine: eng})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	e := &Env{Deps: Deps{Settings: settings, DB: d, Auth: authm, Coord: coord, Gate: NewGate(time.Hour, nil), Emit: &recorder{}, Logger: discard()}}
	s := newPlaybackService(e)
	t.Cleanup(e.close)

	authm.Start()
	for deadline := time.Now().Add(5 * time.Second); authm.Status().State != auth.StateSignedIn; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("not signed in: %+v", authm.Status())
		}
	}
	if !coord.Snapshot().Restricted {
		t.Fatal("coordinator not restricted")
	}

	refs := []TrackRef{{ID: "i.clean1", Source: SourceLibrary}, {ID: "i.explicit", Source: SourceLibrary}, {ID: "i.clean2", Source: SourceLibrary}}
	if err := s.PlayItems(ctx, refs, 1, PlayFrom{Kind: "album", ID: "l.album"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Next(ctx); err != nil {
		t.Fatal(err)
	}
	if got := eng.played(); !slices.Equal(got, []string{"103"}) {
		t.Fatalf("played %v, want only the clean song after the explicit one", got)
	}
	q := s.Queue()
	if !q.Items[1].Explicit || q.Items[0].Explicit {
		t.Fatalf("explicit flags %+v", q.Items)
	}
	if st := s.State(); st.Container == nil || st.Container.Kind != "album" || st.Container.ID != "l.album" {
		t.Fatalf("container %+v", st.Container)
	}
}

// A music video ref resolves through the catalog to the video's playback
// ID, and its queue item says it is a video, by its catalog ID.
func TestPlayMusicVideo(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	ce.signIn(t)
	ctx := t.Context()
	refs := []TrackRef{
		{ID: "i.100", Source: SourceLibrary},
		{ID: "1440841363", Source: SourceCatalog, Kind: TrackKindVideo},
	}
	if err := ce.play.PlayItems(ctx, refs, 1, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
	if got := ce.eng.played(); !slices.Equal(got, []string{"mv.1440841363"}) {
		t.Fatalf("played %v, want the video's playback ID", got)
	}
	q := ce.play.Queue()
	if len(q.Items) != 2 {
		t.Fatalf("queue %+v", q.Items)
	}
	if song := q.Items[0]; song.Video || song.ID != "i.100" {
		t.Fatalf("song item %+v", song)
	}
	v := q.Items[1]
	if !v.Video || v.ID != "1440841363" || v.CatalogID != "1440841363" || v.Source != SourceCatalog ||
		v.Name != "Something" || v.Artist != "The Beatles" || v.ArtistID != "136975" || v.Album != "" ||
		v.DurationMs != 182000 || !v.Explicit {
		t.Fatalf("video item %+v", v)
	}

	err := ce.play.PlayItems(ctx, []TrackRef{{ID: "1440841363", Source: SourceCatalog, Kind: "podcast"}}, 0, PlayFrom{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("unknown kind: %v, want ErrInvalidArgument", err)
	}
	err = ce.play.PlayItems(ctx, []TrackRef{{ID: "1440841363", Source: "station", Kind: TrackKindVideo}}, 0, PlayFrom{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("video from an unknown source: %v, want ErrInvalidArgument", err)
	}
}

// Play next and Move to end from the queue panel change what the media
// keys (MPRIS Next) and the queue view say plays next, and the panel is
// told the queue changed.
func TestQueueMoveReachesMediaKeysAndPanel(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	ce.signIn(t)
	ctx := t.Context()
	for _, n := range []string{"102", "103"} {
		if _, err := ce.e.DB.W.ExecContext(ctx, `INSERT INTO tracks (library_id, catalog_id, name, duration_ms, updated_at) VALUES ('i.'||?, ?, ?, 60000, 0)`, n, n, n); err != nil {
			t.Fatal(err)
		}
	}
	refs := []TrackRef{{ID: "i.100", Source: SourceLibrary}, {ID: "i.101", Source: SourceLibrary}, {ID: "i.102", Source: SourceLibrary}, {ID: "i.103", Source: SourceLibrary}}
	if err := ce.play.PlayItems(ctx, refs, 0, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
	order := func() []string {
		var out []string
		for _, it := range ce.play.Queue().Items {
			out = append(out, it.ID)
		}
		return out
	}
	queueEvents := func() int {
		ce.rec.mu.Lock()
		defer ce.rec.mu.Unlock()
		return slices.Index(ce.rec.events, EventPlaybackQueue) + 1 // 0: never
	}
	v := ce.play.Queue().Version

	if err := ce.play.MoveNext(3); err != nil {
		t.Fatal(err)
	}
	q := ce.play.Queue()
	if got := order(); !slices.Equal(got, []string{"i.100", "i.103", "i.101", "i.102"}) || q.Index != 0 || q.Version == v {
		t.Fatalf("play next: %v index %d version %d→%d", got, q.Index, v, q.Version)
	}
	deadline := time.Now().Add(5 * time.Second)
	for queueEvents() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if queueEvents() == 0 {
		t.Fatal("the panel was not told the queue changed")
	}
	if err := ce.play.MoveToEnd(1); err != nil { // i.103 back to the end
		t.Fatal(err)
	}
	if got := order(); !slices.Equal(got, []string{"i.100", "i.101", "i.102", "i.103"}) {
		t.Fatalf("move to end: %v", got)
	}
	if err := ce.play.MoveNext(2); err != nil { // i.102 first
		t.Fatal(err)
	}
	mp := NewMPRISPlayer(ce.e.Coord, func() {}, func() {}, discard())
	if !mp.Snapshot().CanGoNext {
		t.Fatal("MPRIS cannot go next")
	}
	if err := mp.Next(ctx); err != nil {
		t.Fatal(err)
	}
	if got := ce.eng.played(); got[len(got)-1] != "102" {
		t.Fatalf("the media key's Next played %v, want the moved song (102) last", got)
	}
}
