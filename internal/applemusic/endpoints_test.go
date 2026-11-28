package applemusic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestRatingsBatchesAndStripsPrefix(t *testing.T) {
	var mu sync.Mutex
	var batches []int
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/ratings/library-songs" {
			t.Errorf("path = %s", r.URL.Path)
		}
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		mu.Lock()
		batches = append(batches, len(ids))
		mu.Unlock()
		var data []string
		for _, id := range ids {
			switch id {
			case "i.5":
				data = append(data, `{"id":"i.5","type":"ratings","attributes":{"value":1}}`)
			case "i.150":
				data = append(data, `{"id":"r.i.150","type":"ratings","attributes":{"value":-1}}`)
			}
		}
		_, _ = fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(data, ","))
	}))
	ids := make([]string, 250)
	for i := range ids {
		ids[i] = fmt.Sprintf("i.%d", i)
	}
	got, err := c.Ratings(context.Background(), TypeLibrarySongs, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["i.5"] != RatingFavorite || got["i.150"] != RatingSuggestLess {
		t.Fatalf("ratings = %v", got)
	}
	if len(batches) != 3 {
		t.Fatalf("batches = %v, want 3 requests of <=100", batches)
	}
	for _, n := range batches {
		if n > 100 {
			t.Fatalf("batch of %d", n)
		}
	}
}

func TestSetAndDeleteRating(t *testing.T) {
	var got []string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, r.Method+" "+r.URL.Path+" "+string(b))
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNotFound) // not rated: still success
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := context.Background()
	if err := c.SetRating(ctx, TypeSongs, "42", RatingFavorite); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteRating(ctx, TypeSongs, "42"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetRating(ctx, TypeSongs, "42", 3); err == nil {
		t.Fatal("invalid rating accepted")
	}
	want := []string{
		`PUT /v1/me/ratings/songs/42 {"attributes":{"value":1},"type":"rating"}`,
		`DELETE /v1/me/ratings/songs/42 `,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s", strings.Join(got, "\n"))
	}
}

func TestAddToLibraryReturnsLibraryIDs(t *testing.T) {
	var q string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/me/library" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		q = r.URL.RawQuery
		// Observed shape (representation=ids).
		_, _ = fmt.Fprint(w, `{"data":[{"id":"l.0EeC6mp","type":"library-albums"}],
			"meta":{"operation":{"albums":{"9":[{"id":"l.0EeC6mp","type":"library-albums"}]},"songs":{"1":[{"id":"i.1","type":"library-songs"}],"2":[]}}}}`)
	}))
	c.AMPBase = c.Base
	got, err := c.AddToLibrary(context.Background(), map[string][]string{TypeSongs: {"1", "2"}, TypeAlbums: {"9"}})
	if err != nil {
		t.Fatal(err)
	}
	if q != "ids%5Balbums%5D=9&ids%5Bsongs%5D=1%2C2&representation=ids" {
		t.Fatalf("query = %s", q)
	}
	want := map[ItemKey]string{{TypeAlbums, "9"}: "l.0EeC6mp", {TypeSongs, "1"}: "i.1"}
	if len(got) != len(want) || got[ItemKey{TypeAlbums, "9"}] != "l.0EeC6mp" || got[ItemKey{TypeSongs, "1"}] != "i.1" {
		t.Fatalf("ids = %v", got)
	}
}

func TestLibraryPlaylistTracksEmpty404(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	tracks, err := c.LibraryPlaylistTracks(context.Background(), "p.1", PageOptions{})
	if err != nil || len(tracks) != 0 {
		t.Fatalf("tracks=%v err=%v", tracks, err)
	}
}

func TestCatalogIDs(t *testing.T) {
	var lib Song
	if err := json.Unmarshal([]byte(`{"id":"i.1","type":"library-songs","attributes":{"playParams":{"catalogId":"99"}}}`), &lib); err != nil {
		t.Fatal(err)
	}
	var cat Song
	if err := json.Unmarshal([]byte(`{"id":"5","type":"songs"}`), &cat); err != nil {
		t.Fatal(err)
	}
	var alb Album
	if err := json.Unmarshal([]byte(`{"id":"l.1","type":"library-albums","relationships":{"catalog":{"data":[{"id":"77","type":"albums"}]}}}`), &alb); err != nil {
		t.Fatal(err)
	}
	var pl Playlist
	if err := json.Unmarshal([]byte(`{"id":"p.1","type":"library-playlists","attributes":{"playParams":{"globalId":"pl.u-1"}}}`), &pl); err != nil {
		t.Fatal(err)
	}
	if lib.CatalogID() != "99" || cat.CatalogID() != "5" || alb.CatalogID() != "77" || pl.CatalogID() != "pl.u-1" {
		t.Fatalf("%q %q %q %q", lib.CatalogID(), cat.CatalogID(), alb.CatalogID(), pl.CatalogID())
	}
}

// A library playlist's track fetches ask for the songs' artists and albums:
// a catalogue song in the playlist has no other way to name them.
func TestLibraryPlaylistTrackFetchesAskForRelationships(t *testing.T) {
	var mu sync.Mutex
	var queries []url.Values
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/library/playlists/p.1/tracks" {
			t.Errorf("path = %s", r.URL.Path)
		}
		mu.Lock()
		queries = append(queries, r.URL.Query())
		mu.Unlock()
		_, _ = w.Write([]byte(`{"data":[],"meta":{"total":0}}`))
	}))
	c.AMPBase = c.Base
	ctx := context.Background()
	if _, err := c.LibraryPlaylistTracks(ctx, "p.1", PageOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LibraryPlaylistEntries(ctx, "p.1"); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 {
		t.Fatalf("requests = %d", len(queries))
	}
	for i, q := range queries {
		if q.Get("include[songs]") != "artists" || q.Get("relate[songs]") != "albums" {
			t.Errorf("request %d: query = %v", i, q)
		}
	}
	if queries[1].Get("platform") != "web" || queries[0].Has("platform") {
		t.Errorf("platform: tracks %q, entries %q", queries[0].Get("platform"), queries[1].Get("platform"))
	}
}
