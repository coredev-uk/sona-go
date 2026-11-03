package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/db"
)

type staticDev struct{}

func (staticDev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev", Expires: time.Now().Add(time.Hour)}, nil
}
func (staticDev) Invalidate() {}

// fakeApple serves a mutable library in Apple's /v1/me shapes.
type fakeApple struct {
	mu        sync.Mutex
	songs     []map[string]any
	albums    []map[string]any
	artists   []map[string]any
	playlists []map[string]any
	plTracks  map[string][]map[string]any // playlist id → tracks
	ratings   map[string]map[string]int   // type → id → value
	folders   map[string][]map[string]any // folder id (applemusic.PlaylistsRoot at the top) → children

	failPath   string // path answering 400 ...
	failOffset int    // ... for this page offset
	pageReqs   map[string][]int
	requests   []string      // every request's path and query, in order
	block      chan struct{} // when set, requests wait on it
	forbid     bool          // answer 403 to everything
}

func newFakeApple() *fakeApple {
	return &fakeApple{plTracks: map[string][]map[string]any{}, ratings: map[string]map[string]int{}, pageReqs: map[string][]int{}, folders: map[string][]map[string]any{}}
}

func song(id, name, artist, album, albumID, artistID string) map[string]any {
	return map[string]any{
		"id": id, "type": "library-songs",
		"attributes": map[string]any{
			"name": name, "artistName": artist, "albumName": album, "durationInMillis": 200000,
			"trackNumber": 1, "discNumber": 1, "genreNames": []string{"Pop"},
			"artwork":    map[string]any{"url": "https://art/" + id + "/{w}x{h}bb.jpg"},
			"playParams": map[string]any{"id": id, "kind": "song", "isLibrary": true, "catalogId": "c" + strings.TrimPrefix(id, "i.")},
		},
		"relationships": map[string]any{
			"albums":  map[string]any{"data": []any{map[string]any{"id": albumID, "type": "library-albums"}}},
			"artists": map[string]any{"data": []any{map[string]any{"id": artistID, "type": "library-artists"}}},
		},
	}
}

func album(id, name, artist, artistID, added string) map[string]any {
	return map[string]any{
		"id": id, "type": "library-albums",
		"attributes": map[string]any{"name": name, "artistName": artist, "dateAdded": added, "releaseDate": "2020-01-01", "trackCount": 10},
		"relationships": map[string]any{
			"catalog": map[string]any{"data": []any{map[string]any{"id": "c" + id, "type": "albums"}}},
			"artists": map[string]any{"data": []any{map[string]any{"id": artistID, "type": "library-artists"}}},
		},
	}
}

func artist(id, name string) map[string]any {
	return map[string]any{
		"id": id, "type": "library-artists", "attributes": map[string]any{"name": name},
		"relationships": map[string]any{"catalog": map[string]any{"data": []any{map[string]any{
			"id": "c" + id, "type": "artists", "attributes": map[string]any{"name": name, "artwork": map[string]any{"url": "https://art/" + id + "/{w}x{h}.jpg"}},
		}}}},
	}
}

func playlist(id, name, modified string) map[string]any {
	return map[string]any{
		"id": id, "type": "library-playlists",
		"attributes": map[string]any{
			"name": name, "lastModifiedDate": modified, "dateAdded": "2024-01-01T00:00:00Z", "canEdit": true,
			"playParams": map[string]any{"id": id, "kind": "playlist", "isLibrary": true, "globalId": "pl.u-" + id},
		},
	}
}

func (f *fakeApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forbid {
		http.Error(w, `{"errors":[]}`, http.StatusForbidden)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	off, _ := strconv.Atoi(q.Get("offset"))
	path := r.URL.Path
	f.requests = append(f.requests, path+"?"+r.URL.RawQuery)
	if limit > 1 {
		f.pageReqs[path] = append(f.pageReqs[path], off)
	}
	if path == f.failPath && off == f.failOffset && limit > 1 {
		http.Error(w, `{"errors":[{"title":"injected"}]}`, http.StatusBadRequest)
		return
	}
	var items []map[string]any
	switch {
	case path == "/v1/me/library/songs":
		items = f.songs
	case path == "/v1/me/library/albums":
		items = f.albums
	case path == "/v1/me/library/artists":
		items = f.artists
	case path == "/v1/me/library/playlists":
		items = f.playlists
	case strings.HasPrefix(path, "/v1/me/library/playlists/") && strings.HasSuffix(path, "/tracks"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/me/library/playlists/"), "/tracks")
		items = f.plTracks[id]
		if len(items) == 0 {
			http.Error(w, `{"errors":[]}`, http.StatusNotFound)
			return
		}
	case strings.HasPrefix(path, "/v1/me/library/playlist-folders/") && strings.HasSuffix(path, "/children"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/me/library/playlist-folders/"), "/children")
		items = f.folders[id]
		if len(items) == 0 {
			http.Error(w, `{"errors":[]}`, http.StatusNotFound)
			return
		}
	case strings.HasPrefix(path, "/v1/me/ratings/"):
		typ := strings.TrimPrefix(path, "/v1/me/ratings/")
		var data []any
		for _, id := range strings.Split(q.Get("ids"), ",") {
			if v, ok := f.ratings[typ][id]; ok {
				data = append(data, map[string]any{"id": id, "type": "ratings", "attributes": map[string]any{"value": v}})
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
		return
	default:
		http.NotFound(w, r)
		return
	}
	page := []map[string]any{}
	if off < len(items) {
		page = items[off:min(off+limit, len(items))]
	}
	body := map[string]any{"data": page, "meta": map[string]any{"total": len(items)}}
	if off+limit < len(items) {
		body["next"] = fmt.Sprintf("%s?offset=%d", path, off+limit)
	}
	json.NewEncoder(w).Encode(body)
}

// reset forgets the requests seen so far.
func (f *fakeApple) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pageReqs = map[string][]int{}
	f.requests = nil
}

func (f *fakeApple) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *fakeApple) reqs(path string) []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.pageReqs[path]...)
}

type env struct {
	path  string // the database file
	db    *db.DB
	api   *applemusic.Client
	fake  *fakeApple
	sync  *Syncer
	lib   *Library
	steps []Progress
}

func newEnv(t testing.TB) *env {
	t.Helper()
	fake := newFakeApple()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	path := filepath.Join(t.TempDir(), "sona.db")
	d, err := db.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	api := applemusic.New(srv.Client(), staticDev{}, func() string { return "mut" })
	api.Base = srv.URL
	api.AMPBase = srv.URL
	return &env{path: path, db: d, api: api, fake: fake, sync: NewSyncer(d, api, nil, 4), lib: New(d, api)}
}

// run syncs domains as the app does on startup (Incremental, which runs
// fully the first time); runFull forces a full run.
func (e *env) run(t testing.TB, domains ...Domain) error {
	t.Helper()
	return e.runMode(Incremental, domains...)
}

func (e *env) runFull(t testing.TB, domains ...Domain) error {
	t.Helper()
	return e.runMode(Full, domains...)
}

func (e *env) runMode(mode Mode, domains ...Domain) error {
	e.steps = nil
	return e.sync.Sync(context.Background(), mode, func(p Progress) { e.steps = append(e.steps, p) }, domains...)
}
