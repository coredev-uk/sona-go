package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	if tracks, _ := f.db.RQ.CatalogAlbumTracks(ctx, db.NullString("7")); len(tracks) != 2 {
		t.Fatalf("album tracks = %d", len(tracks))
	}
	if ar, err := f.db.RQ.GetCatalogArtist(ctx, db.NullString("55")); err != nil || ar.Name != "Band" {
		t.Fatalf("artist row = %+v, %v", ar, err)
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
