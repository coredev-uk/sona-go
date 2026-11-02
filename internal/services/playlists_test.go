package services

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
)

func ref(id string) applemusic.ResourceRef {
	return applemusic.ResourceRef{ID: id, Type: applemusic.TypeLibrarySongs}
}

func TestMoveAndRemoveEntries(t *testing.T) {
	list := []string{"a", "b", "c", "d"}
	for _, c := range []struct {
		from, to int
		want     []string
	}{
		{0, 3, []string{"b", "c", "d", "a"}},
		{3, 0, []string{"d", "a", "b", "c"}},
		{1, 2, []string{"a", "c", "b", "d"}},
		{2, 2, []string{"a", "b", "c", "d"}},
	} {
		got, err := moveEntry(list, c.from, c.to)
		if err != nil || !slices.Equal(got, c.want) {
			t.Fatalf("move %d→%d = %v, %v", c.from, c.to, got, err)
		}
	}
	if !slices.Equal(list, []string{"a", "b", "c", "d"}) {
		t.Fatalf("moveEntry changed its input: %v", list)
	}
	if _, err := moveEntry(list, 0, 4); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("out of range move: %v", err)
	}
	got, err := removeEntries(list, []int{3, 1, 1})
	if err != nil || !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("remove = %v, %v", got, err)
	}
	if _, err := removeEntries(list, []int{4}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("out of range remove: %v", err)
	}
}

// fakePlaylistAPI is amp-api's library playlist track endpoints over one
// mutable playlist.
type fakePlaylistAPI struct {
	mu      sync.Mutex
	entries []map[string]any
	puts    []string // PUT bodies, as sent
}

func entry(id, typ string) map[string]any {
	return map[string]any{"id": id, "type": typ, "attributes": map[string]any{
		"name": "Song " + id, "durationInMillis": 1000, "playParams": map[string]any{"id": id, "catalogId": "c" + id},
	}}
}

func (f *fakePlaylistAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/me/library/playlists/p.x/tracks":
		limit := 100
		if r.URL.Query().Get("limit") == "1" {
			limit = 1
		}
		json.NewEncoder(w).Encode(map[string]any{"data": f.entries[:min(limit, len(f.entries))], "meta": map[string]any{"total": len(f.entries)}})
	case r.Method == http.MethodPut && r.URL.Path == "/v1/me/library/playlists/p.x/tracks":
		b, _ := io.ReadAll(r.Body)
		f.puts = append(f.puts, string(b))
		var body struct {
			Data []applemusic.ResourceRef `json:"data"`
		}
		json.Unmarshal(b, &body)
		f.entries = f.entries[:0:0]
		for _, d := range body.Data {
			f.entries = append(f.entries, entry(d.ID, d.Type))
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/me/library/playlists/p.x":
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"id": "p.x", "type": "library-playlists",
			"attributes": map[string]any{"name": "X", "canEdit": true, "canDelete": true, "lastModifiedDate": "2026-10-30T12:00:00Z"},
		}}})
	default:
		http.NotFound(w, r)
	}
}

func newPlaylistsEnv(t *testing.T, f *fakePlaylistAPI) (*PlaylistsService, *recorder) {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return "mut" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	rec := &recorder{}
	e := &Env{Deps: Deps{
		DB: d, API: api, Library: library.New(d, api), Storefront: func() string { return "gb" }, Emit: rec,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
	}, members: newMembership(d)}
	s := &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}}
	// The playlist as the last sync stored it.
	var songs []applemusic.Song
	for _, en := range f.entries {
		b, _ := json.Marshal(en)
		var so applemusic.Song
		json.Unmarshal(b, &so)
		songs = append(songs, so)
	}
	p := applemusic.Playlist{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
		ID: "p.x", Type: applemusic.TypeLibraryPlaylists,
		Attributes: applemusic.PlaylistAttributes{Name: "X", CanEdit: true, CanDelete: true, LastModifiedDate: "2026-10-30T11:00:00Z"},
	}}
	if err := e.Library.StorePlaylist(t.Context(), p, "", songs); err != nil {
		t.Fatal(err)
	}
	return s, rec
}

func trackIDs(d PlaylistDetail) []string {
	out := make([]string, len(d.Tracks))
	for i, s := range d.Tracks {
		out[i] = s.ID
	}
	return out
}

func TestPlaylistEditPayloads(t *testing.T) {
	f := &fakePlaylistAPI{entries: []map[string]any{
		entry("i.a", "library-songs"), entry("i.b", "library-songs"), entry("i.a", "library-songs"), entry("123", "songs"),
	}}
	s, rec := newPlaylistsEnv(t, f)
	ctx := t.Context()

	// Reorder: the full list goes back in the new order, each entry with
	// its own type; duplicates stay.
	d, err := s.MoveTrack(ctx, "p.x", 3, 0, "123")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"data":[{"id":"123","type":"songs"},{"id":"i.a","type":"library-songs"},{"id":"i.b","type":"library-songs"},{"id":"i.a","type":"library-songs"}]}`
	if got := strings.TrimSpace(f.puts[0]); got != want {
		t.Fatalf("PUT body\n got %s\nwant %s", got, want)
	}
	if ids := trackIDs(d); !slices.Equal(ids, []string{"123", "i.a", "i.b", "i.a"}) {
		t.Fatalf("detail after move = %v", ids)
	}
	if d.Playlist.LastModified != time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("lastModified = %d", d.Playlist.LastModified)
	}

	// Remove one copy of a duplicated song: only that position goes.
	d, err = s.RemoveTracks(ctx, "p.x", []int{3}, []string{"i.a"})
	if err != nil {
		t.Fatal(err)
	}
	want = `{"data":[{"id":"123","type":"songs"},{"id":"i.a","type":"library-songs"},{"id":"i.b","type":"library-songs"}]}`
	if got := strings.TrimSpace(f.puts[1]); got != want {
		t.Fatalf("PUT body\n got %s\nwant %s", got, want)
	}
	// SQLite holds the new order without a sync.
	again, err := s.Playlist(ctx, "p.x")
	if err != nil || !slices.Equal(trackIDs(again), []string{"123", "i.a", "i.b"}) {
		t.Fatalf("stored = %v, %v", trackIDs(again), err)
	}
	if !slices.Contains(rec.events, EventPlaylistsChanged) {
		t.Fatalf("events = %v", rec.events)
	}

	// The playlist changed on another device: the edit is refused, nothing
	// is sent, and the stored tracks follow Apple.
	f.mu.Lock()
	f.entries = []map[string]any{entry("i.b", "library-songs"), entry("123", "songs")}
	f.mu.Unlock()
	if _, err := s.MoveTrack(ctx, "p.x", 2, 0, "i.b"); !errors.Is(err, ErrConflict) || CodeOf(err) != CodeConflict {
		t.Fatalf("stale move: %v", err)
	}
	if len(f.puts) != 2 {
		t.Fatalf("stale move sent a PUT")
	}
	again, _ = s.Playlist(ctx, "p.x")
	if !slices.Equal(trackIDs(again), []string{"i.b", "123"}) {
		t.Fatalf("after conflict = %v", trackIDs(again))
	}
	if _, err := s.MoveTrack(ctx, "l.album", 0, 1, "x"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("non-playlist id: %v", err)
	}
}
