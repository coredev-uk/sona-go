package applemusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func songJSON(i int) map[string]any {
	return map[string]any{"id": strconv.Itoa(1000 + i), "type": "songs", "attributes": map[string]any{"name": fmt.Sprint("Song ", i)}}
}

func TestCatalogPlaylistPagesAllTracks(t *testing.T) {
	const total = 650
	var (
		mu    sync.Mutex
		pages []string
	)
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch r.URL.Path {
		case "/v1/catalog/gb/playlists/pl.1":
			if q.Get("limit[tracks]") != "300" || q.Get("include") != "tracks,curator" || q.Get("views") != "more-by-curator" {
				t.Errorf("playlist query = %s", r.URL.RawQuery)
			}
			var tracks []any
			for i := range 300 {
				tracks = append(tracks, songJSON(i))
			}
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
				"id": "pl.1", "type": "playlists",
				"attributes": map[string]any{"name": "Big", "trackCount": total, "playlistType": "editorial"},
				"relationships": map[string]any{
					"tracks":  map[string]any{"data": tracks, "next": "/v1/catalog/gb/playlists/pl.1/tracks?offset=300"},
					"curator": map[string]any{"data": []any{map[string]any{"id": "976439548", "type": "apple-curators", "attributes": map[string]any{"name": "Apple Music Hits"}}}},
				},
				"views": map[string]any{"more-by-curator": map[string]any{"data": []any{}}},
			}}})
		case "/v1/catalog/gb/playlists/pl.1/tracks":
			off, _ := strconv.Atoi(q.Get("offset"))
			mu.Lock()
			pages = append(pages, q.Get("offset")+"/"+q.Get("limit"))
			mu.Unlock()
			var tracks []any
			for i := off; i < min(off+300, total); i++ {
				tracks = append(tracks, songJSON(i))
			}
			json.NewEncoder(w).Encode(map[string]any{"data": tracks})
		case "/v1/catalog/gb/playlists/pl.1/view/featured-artists":
			// Their own request, asking for their motion artwork.
			if q.Get("extend") != "editorialVideo" || q.Get("fields[artists]") != "name,url,artwork,editorialVideo" {
				t.Errorf("featured artists query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"1","type":"artists","attributes":{"name":"A","editorialVideo":{"motionArtistSquare1x1":{"video":"https://mvod.itunes.apple.com/a.m3u8"}}}}]}`))
		case "/v1/catalog/gb/apple-curators/976439548/playlists":
			if q.Get("extend") != "editorialVideo" {
				t.Errorf("curator playlists query = %s", r.URL.RawQuery)
			}
			// The curator's own list stands in for an empty view, without
			// the playlist itself.
			w.Write([]byte(`{"data":[{"id":"pl.1","type":"playlists","attributes":{"name":"Big"}},{"id":"pl.2","type":"playlists","attributes":{"name":"Other"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	c.AMPBase = c.Base
	p, err := c.CatalogPlaylist(t.Context(), "gb", "pl.1")
	if err != nil {
		t.Fatal(err)
	}
	tracks := p.Playlist.Relationships.Tracks.Data
	if len(tracks) != total || tracks[299].ID != "1299" || tracks[300].ID != "1300" || tracks[total-1].ID != strconv.Itoa(1000+total-1) {
		t.Fatalf("tracks = %d, ends %s", len(tracks), tracks[len(tracks)-1].ID)
	}
	if len(pages) != 2 || !strings.Contains(strings.Join(pages, " "), "300/300") || !strings.Contains(strings.Join(pages, " "), "600/300") {
		t.Fatalf("pages = %v", pages)
	}
	if p.Curator != (Curator{ID: "976439548", Type: "apple-curators", Name: "Apple Music Hits"}) {
		t.Fatalf("curator = %+v", p.Curator)
	}
	if len(p.FeaturedArtists) != 1 || p.FeaturedArtists[0].Attributes.EditorialVideo.Square() == "" || len(p.MoreByCurator) != 1 || p.MoreByCurator[0].ID != "pl.2" {
		t.Fatalf("views = %+v / %+v", p.FeaturedArtists, p.MoreByCurator)
	}
}

func TestPlaylistTreeWalksFoldersSkippingTombstones(t *testing.T) {
	children := map[string]string{
		PlaylistsRoot: `[{"id":"p.f1","type":"library-playlist-folders","attributes":{"name":"Mixes","canDelete":true}},
			{"id":"p.gone","type":"library-playlists","attributes":{"canDelete":false}},
			{"id":"p.a","type":"library-playlists","attributes":{"name":"A","canDelete":true,"tags":["favorited"]}}]`,
		"p.f1": `[{"id":"p.f2","type":"library-playlist-folders","attributes":{"name":"Old","canDelete":true}},
			{"id":"p.b","type":"library-playlists","attributes":{"name":"B","canDelete":true}}]`,
	}
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/me/library/playlist-folders/"), "/children")
		body, ok := children[id]
		if !ok { // empty folders answer 404
			http.Error(w, `{"errors":[]}`, http.StatusNotFound)
			return
		}
		var items []json.RawMessage
		json.Unmarshal([]byte(body), &items)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		page := items[min(off, len(items)):min(off+limit, len(items))]
		json.NewEncoder(w).Encode(map[string]any{"data": page, "meta": map[string]any{"total": len(items)}})
	}))
	c.AMPBase = c.Base
	got, err := c.PlaylistTree(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, e := range got {
		lines = append(lines, fmt.Sprintf("%s<%s@%d", e.ID, e.ParentID, e.Position))
	}
	want := "p.f1<p.playlistsroot@0 p.a<p.playlistsroot@1 p.f2<p.f1@0 p.b<p.f1@1"
	if strings.Join(lines, " ") != want {
		t.Fatalf("tree = %s\nwant   %s", strings.Join(lines, " "), want)
	}
	if got[1].Tags[0] != "favorited" {
		t.Fatalf("tags = %v", got[1].Tags)
	}
}

func TestPlaylistMutationRequests(t *testing.T) {
	type req struct{ method, path, query, body string }
	var reqs []req
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		q := r.URL.Query()
		q.Del("platform")
		reqs = append(reqs, req{r.Method, r.URL.Path, q.Encode(), strings.TrimSpace(string(b))})
		switch {
		case r.URL.Path == "/v1/me/library/pins/p.full":
			http.Error(w, `{"errors":[{"title":"Too many pins"}]}`, http.StatusBadRequest)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/me/library/playlists":
			w.Write([]byte(`{"data":[{"id":"p.new","type":"library-playlists","attributes":{"name":"N","canEdit":true,"canDelete":true}}]}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	c.AMPBase = c.Base
	ctx := t.Context()
	p, err := c.CreateLibraryPlaylist(ctx, PlaylistAttributesInput{Name: "N", Description: "d"}, []ResourceRef{{ID: "1", Type: "songs"}})
	if err != nil || p.ID != "p.new" {
		t.Fatalf("create = %+v, %v", p, err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.ReplacePlaylistTracks(ctx, "p.new", []ResourceRef{{ID: "i.2", Type: "library-songs"}, {ID: "1", Type: "songs"}}))
	must(c.MovePlaylistToFolder(ctx, "p.new", PlaylistsRoot))
	must(c.MovePin(ctx, "p.new", ""))
	must(c.SetPinAction(ctx, "p.new", PinShuffle))
	if err := c.Pin(ctx, "p.full"); !errors.Is(err, ErrPinsFull) {
		t.Fatalf("full pins: %v", err)
	}
	want := []req{
		{"POST", "/v1/me/library/playlists", "", `{"attributes":{"name":"N","description":"d","isPublic":false},"relationships":{"tracks":{"data":[{"id":"1","type":"songs"}]}}}`},
		{"PUT", "/v1/me/library/playlists/p.new/tracks", "", `{"data":[{"id":"i.2","type":"library-songs"},{"id":"1","type":"songs"}]}`},
		{"PUT", "/v1/me/library/playlists/p.new/parent", "", `{"data":[{"id":"p.playlistsroot","type":"library-playlist-folders"}]}`},
		{"POST", "/v1/me/library/pins/p.new", "after=top", ""},
		{"PATCH", "/v1/me/library/pins/p.new", "action=shuffle", ""},
		{"POST", "/v1/me/library/pins/p.full", "", ""},
	}
	if len(reqs) != len(want) {
		t.Fatalf("requests = %+v", reqs)
	}
	for i := range want {
		if reqs[i] != want[i] {
			t.Errorf("request %d\n got %+v\nwant %+v", i, reqs[i], want[i])
		}
	}
}

// Apple's generated playlist cover is one fixed 1200×1200 URL; its template
// has the box open and keeps the signature and colours, so the artwork cache
// asks for the size shown.
func TestGeneratedArtworkTemplateOpensTheBox(t *testing.T) {
	const q = "?c1=F2CC9D&c2=FFA940&c3=B36B00&c4=400000&signature=ff4d&t=c29uYQ%3D%3D&tc=000000&vkey=1"
	a := &Artwork{URL: "https://is1-ssl.mzstatic.com/image/thumb/gen/1200x1200AM.PDCXS11.jpg" + q}
	if got, want := a.Template(), "https://is1-ssl.mzstatic.com/image/thumb/gen/{w}x{h}AM.PDCXS11.jpg"+q; got != want {
		t.Errorf("Template = %q, want %q", got, want)
	}
	plain := &Artwork{URL: "https://is1-ssl.mzstatic.com/image/thumb/Music/x/{w}x{h}bb.jpg"}
	if plain.Template() != plain.URL {
		t.Errorf("an album's template changed: %q", plain.Template())
	}
}
