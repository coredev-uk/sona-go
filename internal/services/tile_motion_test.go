package services

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/motion"
)

// Square motion flavours as Apple sends them (trimmed from live responses
// with extend=editorialVideo, 2026-11-01).
const (
	essentialsHLS = "https://mvod.itunes.apple.com/itunes-assets/HLSMusic116/v4/aa/essentials_default.m3u8"
	albumHLS      = "https://mvod.itunes.apple.com/itunes-assets/HLSMusic126/v4/bb/gybr_default.m3u8"
	artistHLS     = "https://mvod.itunes.apple.com/itunes-assets/HLSMusic211/v4/cc/dua_default.m3u8"
	stationHLS    = "https://mvod.itunes.apple.com/itunes-assets/HLSMusic116/v4/dd/your_station_default.m3u8"
)

func squareVideo(key, hls string) string {
	return `"editorialVideo":{"` + key + `":{"video":"` + hls + `"},"motionDetailTall":{"video":"https://mvod.itunes.apple.com/itunes-assets/HLSMusic116/v4/ee/tall.m3u8"}}`
}

var (
	essentialsJSON = `{"id":"pl.72ee689fe3f84ce2b46a6913c2e6f5f1","type":"playlists","attributes":{"name":"Elton John Essentials","curatorName":"Apple Music Pop",` + squareVideo("motionDetailSquare", essentialsHLS) + `}}`
	rocketHourJSON = `{"id":"pl.9192e00c1da741719f7b4816d164031e","type":"playlists","attributes":{"name":"Elton John’s Rocket Hour"}}`
	gybrJSON       = `{"id":"1440863013","type":"albums","attributes":{"name":"Goodbye Yellow Brick Road (2014 Remaster)",` + squareVideo("motionSquareVideo1x1", albumHLS) + `}}`
	duaJSON        = `{"id":"1031397873","type":"artists","attributes":{"name":"Dua Lipa",` + squareVideo("motionArtistSquare1x1", artistHLS) + `}}`
)

// tileMotionAMP is a fake amp-api for the pages whose tiles play motion
// on hover. Like Apple, it leaves the motion artwork out unless the
// request asks for editorialVideo.
func tileMotionAMP() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked := slices.Contains(strings.Split(r.URL.Query().Get("extend"), ","), "editorialVideo")
		body := ""
		switch p := r.URL.Path; p {
		case "/v1/catalog/gb/search":
			body = `{"results":{"playlist":{"data":[{"id":"pl.72ee689fe3f84ce2b46a6913c2e6f5f1","type":"playlists"},{"id":"pl.9192e00c1da741719f7b4816d164031e","type":"playlists"}]},
				"album":{"data":[{"id":"1440863013","type":"albums"}]},"artist":{"data":[{"id":"1031397873","type":"artists"}]}},
				"resources":{"playlists":{"pl.72ee689fe3f84ce2b46a6913c2e6f5f1":` + essentialsJSON + `,"pl.9192e00c1da741719f7b4816d164031e":` + rocketHourJSON + `},
				"albums":{"1440863013":` + gybrJSON + `},"artists":{"1031397873":` + duaJSON + `}},
				"meta":{"results":{"order":["artist","album","playlist"]}}}`
		case "/v1/catalog/gb/artists/54657":
			body = `{"data":[{"id":"54657","type":"artists","attributes":{"name":"Elton John"},"views":{
				"full-albums":{"data":[` + gybrJSON + `]},
				"playlists":{"data":[` + essentialsJSON + `,` + rocketHourJSON + `]},
				"similar-artists":{"data":[` + duaJSON + `]}}}]}`
		case "/v1/catalog/gb/artists/54657/view/playlists":
			body = `{"data":[` + essentialsJSON + `,` + rocketHourJSON + `]}`
		case "/v1/me/recent/radio-stations":
			body = `{"data":[{"id":"ra.u-f6e485aaa343121e1abb70ff757aa95f","type":"stations","attributes":{"name":"Your Station","kind":"user",` + squareVideo("motionSquareVideo1x1", stationHLS) + `}},
				{"id":"ra.a-570372593","type":"stations","attributes":{"name":"Artist Station","kind":"artistOnly"}}]}`
		case "/v1/catalog/gb/charts":
			body = `{"results":{"playlists":[{"chart":"most-played","name":"Top Playlists","data":[{"id":"pl.72ee689fe3f84ce2b46a6913c2e6f5f1","type":"playlists"}]}]},
				"resources":{"playlists":{"pl.72ee689fe3f84ce2b46a6913c2e6f5f1":` + essentialsJSON + `}}}`
		default:
			http.NotFound(w, r)
			return
		}
		if !asked {
			// Apple leaves editorialVideo out unless it is asked for.
			for _, hls := range []string{essentialsHLS, albumHLS, artistHLS, stationHLS} {
				body = strings.ReplaceAll(body, hls, "")
			}
		}
		_, _ = w.Write([]byte(body))
	})
}

type tileMotionEnv struct {
	e  *Env
	mc *motion.Cache
}

func newTileMotionEnv(t *testing.T) tileMotionEnv {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	srv := httptest.NewServer(tileMotionAMP())
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return "" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	mc, err := motion.New(motion.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	sf := func() string { return "gb" }
	gate := NewGate(time.Hour, nil)
	gate.Open()
	e := &Env{
		Deps: Deps{
			DB: d, API: api, Auth: &auth.Manager{}, Catalog: catalog.New(d, api, sf, nil), Gate: gate,
			Storefront: sf, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
		},
		m:       mapper{motion: mc},
		members: newMembership(d),
	}
	return tileMotionEnv{e: e, mc: mc}
}

// Every catalog tile the user can hover carries its square motion
// artwork where Apple has one: search results, the artist page's shelves
// and "See all", the Radio page's own stations and the charts. Items
// without motion stay without.
func TestCatalogTilesCarryMotion(t *testing.T) {
	env := newTileMotionEnv(t)
	ctx := t.Context()
	want := func(what, got, hls string) {
		t.Helper()
		if exp := env.mc.URL(hls); got != exp || !strings.HasPrefix(got, "/motion/") {
			t.Errorf("%s: motion = %q, want %q", what, got, exp)
		}
	}
	none := func(what, got string) {
		t.Helper()
		if got != "" {
			t.Errorf("%s: motion = %q, want none", what, got)
		}
	}

	res, err := newSearchService(env.e).Catalog(ctx, "Elton John Essentials")
	if err != nil {
		t.Fatal(err)
	}
	var found int
	for _, sec := range res.Sections {
		for _, it := range sec.Items {
			switch {
			case it.Playlist != nil && it.Playlist.Name == "Elton John Essentials":
				want("search: playlist", it.Playlist.Motion, essentialsHLS)
			case it.Playlist != nil:
				none("search: playlist without motion", it.Playlist.Motion)
			case it.Album != nil:
				want("search: album", it.Album.Motion, albumHLS)
			case it.Artist != nil:
				want("search: artist", it.Artist.Motion, artistHLS)
			default:
				continue
			}
			found++
		}
	}
	if found != 4 {
		t.Fatalf("search items = %d, want 4: %+v", found, res.Sections)
	}

	cs := newCatalogService(env.e)
	a, err := cs.Artist(ctx, "54657")
	if err != nil {
		t.Fatal(err)
	}
	shelves := map[string]ArtistShelf{}
	for _, sh := range a.Shelves {
		shelves[sh.View] = sh
	}
	if sh := shelves[ViewPlaylists]; len(sh.Playlists) != 2 {
		t.Fatalf("artist playlists = %+v", sh)
	} else {
		want("artist page: Elton John Essentials", sh.Playlists[0].Motion, essentialsHLS)
		none("artist page: Rocket Hour", sh.Playlists[1].Motion)
	}
	if sh := shelves[ViewFullAlbums]; len(sh.Albums) != 1 {
		t.Fatalf("artist albums = %+v", sh)
	} else {
		want("artist page: album", sh.Albums[0].Motion, albumHLS)
	}
	if sh := shelves[ViewSimilarArtists]; len(sh.Artists) != 1 {
		t.Fatalf("similar artists = %+v", sh)
	} else {
		want("artist page: similar artist", sh.Artists[0].Motion, artistHLS)
	}

	v, err := cs.ArtistView(ctx, "54657", ViewPlaylists, 0, 0)
	if err != nil || len(v.Playlists) != 2 {
		t.Fatalf("artist view = %+v, %v", v, err)
	}
	want("artist See all: Elton John Essentials", v.Playlists[0].Motion, essentialsHLS)

	st, err := (&RadioService{e: env.e}).Recent(ctx)
	if err != nil || len(st) != 2 {
		t.Fatalf("recent stations = %+v, %v", st, err)
	}
	want("Your stations: personal station", st[0].Motion, stationHLS)
	none("Your stations: artist station (Apple has none)", st[1].Motion)

	ds := newDiscoverService(env.e)
	t.Cleanup(env.e.close)
	ch, err := ds.ChartItems(ctx, ChartPlaylists, "", "", 0)
	if err != nil || len(ch.Items) != 1 || ch.Items[0].Playlist == nil {
		t.Fatalf("chart = %+v, %v", ch, err)
	}
	want("charts: playlist", ch.Items[0].Playlist.Motion, essentialsHLS)
}
