package services

import (
	"context"
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
	authm, err := auth.NewManager(auth.Config{
		HTTP: apple.Client(), Dev: testDev{}, APIBase: apple.URL,
		Store:    &recordStore{rec: store.Record{Token: "mut", ObtainedAt: time.Now(), Restricted: true}},
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
	e := &Env{Deps: Deps{Settings: settings, DB: d, Auth: authm, Coord: coord, Emit: &recorder{}, Logger: discard()}}
	s := newPlaybackService(e)
	t.Cleanup(e.close)

	if err := authm.Start(ctx); err != nil {
		t.Fatal(err)
	}
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
