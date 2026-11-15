package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
)

const mixLibraryStamp = "2019-04-12T22:52:43Z" // what Apple keeps on a mix's library copy

// fakeMixAPI is a personal mix as Apple serves it: the library copy keeps
// its stamp, the catalog copy (include=catalog) moves with each refresh.
type fakeMixAPI struct {
	mu           sync.Mutex
	catalogStamp string
	entries      []map[string]any
	trackGets    int
}

func (f *fakeMixAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/v1/me/library/playlists/p.mix":
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"id": "p.mix", "type": "library-playlists",
			"attributes": map[string]any{
				"name": "New Music Mix", "lastModifiedDate": mixLibraryStamp,
				"playParams": map[string]any{"id": "p.mix", "kind": "playlist", "isLibrary": true, "globalId": "pl.pm-1"},
			},
			"relationships": map[string]any{"catalog": map[string]any{"data": []any{map[string]any{
				"id": "pl.pm-1", "type": "playlists", "attributes": map[string]any{"lastModifiedDate": f.catalogStamp},
			}}}},
		}}})
	case "/v1/me/library/playlists/p.mix/tracks":
		f.trackGets++
		_ = json.NewEncoder(w).Encode(map[string]any{"data": f.entries, "meta": map[string]any{"total": len(f.entries)}})
	default:
		http.NotFound(w, r)
	}
}

// Opening a personal mix asks Apple for its catalog stamp: when it moved
// since the stored one (the library stamp never does), the mix's tracks
// and stamp are stored again and the page is told; unchanged, nothing is
// fetched.
func TestRefreshMixFollowsTheCatalogStamp(t *testing.T) {
	const week1, week2 = "2026-11-06T02:00:00Z", "2026-11-13T02:00:00Z"
	f := &fakeMixAPI{catalogStamp: week1, entries: []map[string]any{entry("i.1", "library-songs")}}
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
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

	// The mix as the last sync stored it: week 1's tracks under week 1's stamp.
	mix := applemusic.Playlist{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
		ID: "p.mix", Type: applemusic.TypeLibraryPlaylists,
		Attributes: applemusic.PlaylistAttributes{
			Name: "New Music Mix", LastModifiedDate: mixLibraryStamp,
			PlayParams: &applemusic.PlayParams{ID: "p.mix", GlobalID: "pl.pm-1"},
		},
	}}
	mix.Relationships.Catalog.Data = []applemusic.Playlist{{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
		ID: "pl.pm-1", Type: applemusic.TypePlaylists, Attributes: applemusic.PlaylistAttributes{LastModifiedDate: week1},
	}}}
	oldSong := applemusic.Song{Resource: applemusic.Resource[applemusic.SongAttributes]{
		ID: "i.1", Type: applemusic.TypeLibrarySongs, Attributes: applemusic.SongAttributes{Name: "Song i.1"},
	}}
	if err := e.Library.StorePlaylist(t.Context(), mix, "", []applemusic.Song{oldSong}); err != nil {
		t.Fatal(err)
	}

	moved, err := s.refreshMix(t.Context(), "p.mix")
	if err != nil || moved {
		t.Fatalf("unchanged mix: moved %v, %v", moved, err)
	}
	if f.trackGets != 0 || len(rec.events) != 0 {
		t.Fatalf("unchanged mix fetched its tracks %d times, emitted %v", f.trackGets, rec.events)
	}

	// The weekly refresh.
	f.mu.Lock()
	f.catalogStamp = week2
	f.entries = []map[string]any{entry("i.2", "library-songs"), entry("i.1", "library-songs")}
	f.mu.Unlock()
	moved, err = s.refreshMix(t.Context(), "p.mix")
	if err != nil || !moved {
		t.Fatalf("regenerated mix: moved %v, %v", moved, err)
	}
	p, songs, err := e.Library.AllPlaylist(t.Context(), "p.mix")
	var got []string
	for _, so := range songs {
		got = append(got, so.LibraryID)
	}
	if err != nil || p.LastModified != week2 || !slices.Equal(got, []string{"i.2", "i.1"}) {
		t.Fatalf("mix after the refresh: Updated %q, tracks %v, %v", p.LastModified, got, err)
	}
	if !slices.Equal(rec.events, []string{EventPlaylistsChanged}) {
		t.Fatalf("events = %v", rec.events)
	}

	trackGets := f.trackGets
	if moved, err = s.refreshMix(t.Context(), "p.mix"); err != nil || moved || f.trackGets != trackGets {
		t.Fatalf("second check: moved %v, %v, %d more track fetches", moved, err, f.trackGets-trackGets)
	}
}
