package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
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

const albumJSON = `{"data":[{"id":"7","type":"albums","attributes":{"name":"Big","artistName":"Band","trackCount":2,"artwork":{"url":"https://a/{w}x{h}.jpg"}},
"relationships":{"artists":{"data":[{"id":"55","type":"artists","attributes":{"name":"Band"}}]},
"tracks":{"data":[{"id":"1","type":"songs","attributes":{"name":"One","albumName":"Big","durationInMillis":1000}},{"id":"2","type":"songs","attributes":{"name":"Two","albumName":"Big"}}]}}}]}`

type fixture struct {
	cat   *Catalog
	db    *db.DB
	hits  atomic.Int32
	down  atomic.Bool
	clock time.Time
	sf    string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{clock: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), sf: "gb"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if f.down.Load() {
			http.Error(w, "down", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/v1/catalog/gb/albums/7", "/v1/catalog/us/albums/7":
			w.Write([]byte(albumJSON))
		case "/v1/catalog/gb/songs":
			// Batched lookup: every ID but "404" is known.
			var data []string
			for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
				if id != "404" {
					data = append(data, `{"id":"`+id+`","type":"songs","attributes":{"name":"Song `+id+`","durationInMillis":1000}}`)
				}
			}
			_, _ = w.Write([]byte(`{"data":[` + strings.Join(data, ",") + `]}`))
		case "/v1/catalog/gb/search":
			w.Write([]byte(`{"results":{"song":{"data":[{"id":"1","type":"songs"}]}},"resources":{"songs":{"1":{"id":"1","type":"songs","attributes":{"name":"One"}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	d, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	api := applemusic.New(srv.Client(), staticDev{}, nil)
	api.Base = srv.URL
	api.AMPBase = srv.URL
	f.db = d
	f.cat = New(d, api, func() string { return f.sf }, nil)
	f.cat.now = func() time.Time { return f.clock }
	return f
}

func TestAlbumReadThrough(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for range 3 {
		a, err := f.cat.Album(ctx, "7")
		if err != nil || a.Attributes.Name != "Big" || len(a.Relationships.Tracks.Data) != 2 {
			t.Fatalf("album = %+v, %v", a, err)
		}
	}
	if f.hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 (cached)", f.hits.Load())
	}

	// Normalised catalog rows.
	tr, err := f.db.RQ.GetTrackByCatalogID(ctx, db.NullString("1"))
	if err != nil || tr.Name != "One" || tr.AlbumCatalogID.String != "7" || tr.DurationMs != 1000 || tr.LibraryID.Valid {
		t.Fatalf("track row = %+v, %v", tr, err)
	}
	alb, err := f.db.RQ.GetCatalogAlbum(ctx, db.NullString("7"))
	if err != nil || alb.ArtistCatalogID.String != "55" || alb.ArtworkTemplate != "https://a/{w}x{h}.jpg" {
		t.Fatalf("album row = %+v, %v", alb, err)
	}
	var tracks int
	if err := f.db.R.QueryRowContext(ctx, `SELECT count(*) FROM tracks WHERE album_catalog_id = '7' AND library_id IS NULL`).Scan(&tracks); err != nil || tracks != 2 {
		t.Fatalf("album tracks = %d, %v", tracks, err)
	}
	var artist string
	if err := f.db.R.QueryRowContext(ctx, `SELECT name FROM artists WHERE catalog_id = '55' AND library_id IS NULL`).Scan(&artist); err != nil || artist != "Band" {
		t.Fatalf("artist row = %q, %v", artist, err)
	}

	// Expired: refetched.
	f.clock = f.clock.Add(AlbumTTL + time.Minute)
	if _, err := f.cat.Album(ctx, "7"); err != nil || f.hits.Load() != 2 {
		t.Fatalf("after TTL: hits=%d err=%v", f.hits.Load(), err)
	}

	// Expired and Apple unreachable: the stale copy is served.
	f.clock = f.clock.Add(AlbumTTL + time.Minute)
	f.down.Store(true)
	if a, err := f.cat.Album(ctx, "7"); err != nil || a.ID != "7" {
		t.Fatalf("stale = %+v, %v", a, err)
	}
}

func TestSongsBatchesAndStores(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	got, err := f.cat.Songs(ctx, []string{"1", "2", "404", "1", "3"})
	if err != nil || len(got) != 3 || got["2"].Attributes.Name != "Song 2" {
		t.Fatalf("songs = %v, %v (an unknown ID is left out, a repeated one asked once)", got, err)
	}
	if f.hits.Load() != 1 {
		t.Fatalf("hits = %d, want one batched request", f.hits.Load())
	}

	// Stored as rows, like the single-song lookup, and cached per song.
	tr, err := f.db.RQ.GetTrackByCatalogID(ctx, db.NullString("2"))
	if err != nil || tr.Name != "Song 2" || tr.LibraryID.Valid {
		t.Fatalf("track row = %+v, %v", tr, err)
	}
	if s, err := f.cat.Song(ctx, "3"); err != nil || s.Attributes.Name != "Song 3" {
		t.Fatalf("song = %+v, %v", s, err)
	}
	if _, err := f.cat.Songs(ctx, []string{"3", "2"}); err != nil || f.hits.Load() != 1 {
		t.Fatalf("cached songs asked Apple again: hits=%d err=%v", f.hits.Load(), err)
	}

	// Only what is missing is asked for; the unknown one is asked again (no
	// negative entry), and expired entries are refetched.
	if got, err = f.cat.Songs(ctx, []string{"3", "404", "9"}); err != nil || len(got) != 2 || f.hits.Load() != 2 {
		t.Fatalf("partial = %v, hits=%d, %v", got, f.hits.Load(), err)
	}
	f.clock = f.clock.Add(SongTTL + time.Minute)
	if _, err := f.cat.Songs(ctx, []string{"3"}); err != nil || f.hits.Load() != 3 {
		t.Fatalf("after TTL: hits=%d err=%v", f.hits.Load(), err)
	}

	f.sf = ""
	if _, err := f.cat.Songs(ctx, []string{"1"}); !errors.Is(err, ErrNoStorefront) {
		t.Fatalf("no storefront: %v", err)
	}
}

func TestStorefrontIsPartOfKey(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.cat.Album(ctx, "7")
	f.sf = "us"
	f.cat.Album(ctx, "7")
	if f.hits.Load() != 2 {
		t.Fatalf("hits = %d: storefronts share a cache entry", f.hits.Load())
	}
	f.sf = ""
	if _, err := f.cat.Album(ctx, "7"); !errors.Is(err, ErrNoStorefront) {
		t.Fatalf("err = %v", err)
	}
}

// TestContentLanguageIsPartOfKey: a response asked for in another content
// language (l=) is a separate cache entry, and the default's keeps the
// storefront-only key.
func TestContentLanguageIsPartOfKey(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	lang := ""
	f.cat.api.Language = func() string { return lang }
	for _, l := range []string{"", "fr-CA", "fr-CA", ""} {
		lang = l
		if _, err := f.cat.Album(ctx, "7"); err != nil {
			t.Fatalf("album in %q: %v", l, err)
		}
	}
	if f.hits.Load() != 2 {
		t.Fatalf("hits = %d, want one per language", f.hits.Load())
	}
	for _, key := range []string{"gb|album-page.2|7", "gb@fr-CA|album-page.2|7"} {
		if _, err := f.db.RQ.GetCache(ctx, key); err != nil {
			t.Errorf("cache %q: %v", key, err)
		}
	}
}

func TestNotFoundIsNotCached(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for range 2 {
		if _, err := f.cat.Album(ctx, "404"); !errors.Is(err, applemusic.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
	}
	if f.hits.Load() != 2 {
		t.Fatalf("hits = %d", f.hits.Load())
	}
}

func TestSearchCachedPerQuery(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.cat.Search(ctx, "One", applemusic.SearchOptions{})
	res, err := f.cat.Search(ctx, "  one ", applemusic.SearchOptions{})
	if err != nil || len(res.Groups["song"].Data) != 1 || len(res.Resources.Resolve(res.Groups["song"].Data)) != 1 || f.hits.Load() != 1 {
		t.Fatalf("res=%+v hits=%d err=%v", res, f.hits.Load(), err)
	}
	f.cat.Search(ctx, "one", applemusic.SearchOptions{Group: applemusic.GroupSong, Offset: 25})
	if f.hits.Load() != 2 {
		t.Fatalf("different page served from cache")
	}
}

func TestPrune(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.cat.Search(ctx, "one", applemusic.SearchOptions{})
	f.cat.Album(ctx, "7")
	f.clock = f.clock.Add(SearchTTL + time.Second)
	n, err := f.cat.Prune(ctx)
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, %v", n, err)
	}
}
