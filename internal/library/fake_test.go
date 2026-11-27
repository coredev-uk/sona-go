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
	videos    []map[string]any
	plTracks  map[string][]map[string]any // playlist id → tracks
	ratings   map[string]map[string]int   // type → id → value
	folders   map[string][]map[string]any // folder id (applemusic.PlaylistsRoot at the top) → children

	// credits is the catalogue's artists of a catalogue song ID, served at
	// /v1/catalog/{sf}/songs?ids=...&include=artists as [id, name] pairs.
	credits map[string][][2]string
	// albumCredits is the same for a catalogue album ID, served at
	// /v1/catalog/{sf}/albums?ids=...&include=artists. A compilation is
	// present with no artists ("Various Artists").
	albumCredits map[string][][2]string
	// creditArtwork is the artwork template Apple's include=artists carries
	// for a credited catalogue artist ID (checked 2026-11-27: every included
	// artist has one); an artist not in it is served without artwork.
	creditArtwork map[string]string

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

// preAdded is a pre-added album as Apple's library lists it (live check,
// docs/apple-music-reference.md): no releaseDate of its own, its catalog album
// carrying the date and isPrerelease.
func preAdded(id, name, artist, artistID, added, date string, flagged bool) map[string]any {
	a := album(id, name, artist, artistID, added)
	delete(a["attributes"].(map[string]any), "releaseDate")
	cat := a["relationships"].(map[string]any)["catalog"].(map[string]any)["data"].([]any)[0].(map[string]any)
	cat["attributes"] = map[string]any{"isSingle": false, "releaseDate": date, "isPrerelease": flagged}
	return a
}

// unreleasedSong is a song of a pre-added album that is not out: in the
// library, but with no playParams, no releaseDate and no artwork.
func unreleasedSong(id, name, artist, albumID, artistID string) map[string]any {
	s := song(id, name, artist, "Soon", albumID, artistID)
	a := s["attributes"].(map[string]any)
	delete(a, "playParams")
	delete(a, "artwork")
	return s
}

func artist(id, name string) map[string]any {
	return map[string]any{
		"id": id, "type": "library-artists", "attributes": map[string]any{"name": name},
		"relationships": map[string]any{"catalog": map[string]any{"data": []any{map[string]any{
			"id": "c" + id, "type": "artists", "attributes": map[string]any{"name": name, "artwork": map[string]any{"url": "https://art/" + id + "/{w}x{h}.jpg"}},
		}}}},
	}
}

func musicVideo(id, name, artist string) map[string]any {
	return map[string]any{
		"id": id, "type": "library-music-videos",
		"attributes": map[string]any{
			"name": name, "artistName": artist, "albumName": "Clips", "durationInMillis": 215000,
			"releaseDate": "2021-05-01", "genreNames": []string{"Pop"}, "contentRating": "explicit", "has4K": true,
			"artwork":    map[string]any{"url": "https://art/" + id + "/{w}x{h}bb.jpg", "width": 1920, "height": 1080},
			"playParams": map[string]any{"id": id, "kind": "musicVideo", "isLibrary": true, "catalogId": "c" + strings.TrimPrefix(id, "i.")},
		},
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
	if limit > 1 && q.Get("ids") == "" {
		f.pageReqs[path] = append(f.pageReqs[path], off)
	}
	if path == f.failPath && off == f.failOffset && limit > 1 {
		http.Error(w, `{"errors":[{"title":"injected"}]}`, http.StatusBadRequest)
		return
	}
	for _, kind := range []struct {
		suffix  string
		credits map[string][][2]string
	}{{"/songs", f.credits}, {"/albums", f.albumCredits}} {
		if !strings.HasPrefix(path, "/v1/catalog/") || !strings.HasSuffix(path, kind.suffix) {
			continue
		}
		ids := strings.Split(q.Get("ids"), ",")
		if len(ids) > 100 { // Apple's limit: "Too Many Ids"
			http.Error(w, `{"errors":[{"title":"Too Many Ids","code":"40010"}]}`, http.StatusBadRequest)
			return
		}
		data := []map[string]any{}
		for _, id := range ids {
			cr, ok := kind.credits[id]
			if !ok {
				continue
			}
			artists := []any{}
			for _, a := range cr {
				attrs := map[string]any{"name": a[1]}
				if url, ok := f.creditArtwork[a[0]]; ok {
					attrs["artwork"] = map[string]any{"url": url, "width": 1000, "height": 1000}
				}
				artists = append(artists, map[string]any{"id": a[0], "type": "artists", "attributes": attrs})
			}
			data = append(data, map[string]any{
				"id": id, "type": strings.TrimPrefix(kind.suffix, "/"),
				"relationships": map[string]any{"artists": map[string]any{"data": artists}},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
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
	case path == "/v1/me/library/music-videos":
		items = f.videos
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
	if q.Get("ids") != "" && q.Has("limit") {
		http.Error(w, `{"errors":[{"title":"Invalid Parameter","detail":"Limit may not be supplied on this request"}]}`, http.StatusBadRequest)
		return
	}
	if ids := q.Get("ids"); ids != "" && (path == "/v1/me/library/songs" || path == "/v1/me/library/albums") {
		// A lookup by ID: the items asked for, in the order they were asked.
		data := []map[string]any{}
		for _, id := range strings.Split(ids, ",") {
			for _, it := range items {
				if it["id"] == id {
					data = append(data, it)
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
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
