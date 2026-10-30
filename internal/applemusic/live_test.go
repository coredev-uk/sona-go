package applemusic_test

// Live contract tests: one real request per Apple endpoint family Sona
// uses, asserting the decoded shapes the app relies on, so an Apple-side
// response change shows up here (weekly in CI, .github/workflows/live.yml)
// before users see it. Opt in with SONA_LIVE=1 and a Music User Token in
// MUSIC_USER_TOKEN (or the repository's .env). Nothing is played: the
// playback family stops at the playlist, certificate and a licence
// exchange with a junk challenge.

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
)

// liveTerm is searched to find the songs the other families use, so the
// tests work in any storefront.
const liveTerm = "Daft Punk"

type live struct {
	api *applemusic.Client
	sf  string
	// From the search: a catalog song with lyrics and its album.
	song, album string
}

var (
	liveOnce sync.Once
	liveEnv  *live
	liveErr  error
)

func readDotEnv(key string) string {
	f, err := os.Open("../../.env")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export ")
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// liveClient signs in once (developer token from the web player, MUT from
// the environment) and finds the fixture IDs. Without SONA_LIVE=1 the test
// is skipped; with it, a missing token fails, so a lost CI secret is loud.
func liveClient(t *testing.T) *live {
	t.Helper()
	if os.Getenv("SONA_LIVE") != "1" {
		t.Skip("live Apple Music tests: set SONA_LIVE=1 and MUSIC_USER_TOKEN")
	}
	liveOnce.Do(func() {
		mut := os.Getenv("MUSIC_USER_TOKEN")
		if mut == "" {
			mut = readDotEnv("MUSIC_USER_TOKEN")
		}
		if mut == "" {
			liveErr = errors.New("SONA_LIVE=1 but MUSIC_USER_TOKEN is not set (environment or .env)")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		api := applemusic.New(nil, devtoken.NewWebPlayer(nil), func() string { return mut })
		sf, err := api.Storefront(ctx)
		if err != nil {
			liveErr = err
			return
		}
		liveEnv = &live{api: api, sf: sf.ID}
	})
	if liveErr != nil {
		t.Fatal(liveErr)
	}
	return liveEnv
}

func liveCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// checkArtwork asserts the artwork template Sona sizes and caches.
func checkArtwork(t *testing.T, what string, a *applemusic.Artwork) {
	t.Helper()
	if a == nil || !strings.Contains(a.URL, "{w}") || !strings.Contains(a.URL, "{h}") || a.Width <= 0 {
		t.Errorf("%s artwork = %+v, want a {w}x{h} template with a size", what, a)
	}
}

func checkSong(t *testing.T, what string, s applemusic.Song) {
	t.Helper()
	a := s.Attributes
	if s.ID == "" || s.Type == "" || a.Name == "" || a.ArtistName == "" || a.DurationInMillis <= 0 {
		t.Errorf("%s: id %q type %q name %q artist %q duration %d", what, s.ID, s.Type, a.Name, a.ArtistName, a.DurationInMillis)
	}
	if a.PlayParams == nil || a.PlayParams.ID == "" || a.PlayParams.Kind == "" {
		t.Errorf("%s: playParams = %+v", what, a.PlayParams)
	}
	checkArtwork(t, what, a.Artwork)
}

// TestLive runs the families in order: the search finds the IDs the
// catalog, lyrics and playback families use.
func TestLive(t *testing.T) {
	l := liveClient(t)

	t.Run("storefront", func(t *testing.T) {
		if len(l.sf) != 2 {
			t.Errorf("storefront id %q, want a two-letter code", l.sf)
		}
	})

	t.Run("search", func(t *testing.T) {
		res, err := applemusic.CatalogSearch(liveCtx(t), l.api, l.sf, liveTerm, applemusic.SearchOptions{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		order := res.Order()
		if len(order) == 0 {
			t.Fatalf("no result groups (groups %v)", len(res.Groups))
		}
		songs := res.Resources.Resolve(res.Groups["song"].Data)
		if len(songs) == 0 {
			t.Fatalf("no songs in group %q; groups %v", "song", order)
		}
		for i, n := range songs {
			s, err := applemusic.As[applemusic.Song](n)
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				checkSong(t, "search song", s)
			}
			if l.song == "" && s.Attributes.HasLyrics && len(s.Relationships.Albums.Data) > 0 {
				l.song, l.album = s.ID, s.Relationships.Albums.Data[0].ID
			}
		}
		if l.song == "" {
			t.Fatal("no search song with lyrics and an album relationship")
		}
	})
	if l.song == "" {
		t.Fatal("search found no fixture song")
	}

	t.Run("catalog", func(t *testing.T) {
		s, err := l.api.Song(liveCtx(t), l.sf, l.song)
		if err != nil {
			t.Fatal(err)
		}
		checkSong(t, "catalog song", s)
		a, err := applemusic.AlbumPage(liveCtx(t), l.api, l.sf, l.album)
		if err != nil {
			t.Fatal(err)
		}
		if a.Attributes.Name == "" || a.Attributes.ArtistName == "" || a.Attributes.TrackCount <= 0 {
			t.Errorf("album attributes = %+v", a.Attributes)
		}
		checkArtwork(t, "album", a.Attributes.Artwork)
		if len(a.Relationships.Tracks.Data) == 0 {
			t.Fatal("album has no tracks relationship")
		}
		checkSong(t, "album track", a.Relationships.Tracks.Data[0])
	})

	t.Run("library", func(t *testing.T) {
		p, err := l.api.LibrarySongsPage(liveCtx(t), 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if p.Meta.Total < len(p.Data) {
			t.Errorf("meta.total %d < %d songs on the page", p.Meta.Total, len(p.Data))
		}
		if len(p.Data) == 0 {
			t.Skip("the account's library is empty")
		}
		// Songs no longer streamable come without playParams; the sync
		// maps the rest to catalog songs through playParams.catalogId
		// (uploads have none).
		playable, mapped := 0, 0
		for _, s := range p.Data {
			if !strings.HasPrefix(s.ID, "i.") || s.Type != "library-songs" || s.Attributes.Name == "" || len(s.Relationships.Albums.Data) == 0 {
				t.Errorf("library song id %q type %q name %q albums %d", s.ID, s.Type, s.Attributes.Name, len(s.Relationships.Albums.Data))
			}
			if pp := s.Attributes.PlayParams; pp != nil {
				if !pp.IsLibrary || pp.ID != s.ID {
					t.Errorf("library song %s playParams = %+v", s.ID, pp)
				}
				playable++
				if s.CatalogID() != "" {
					mapped++
				}
			}
		}
		if playable == 0 || mapped == 0 {
			t.Errorf("of %d library songs, %d have playParams and %d a catalog ID", len(p.Data), playable, mapped)
		}
	})

	t.Run("recommendations", func(t *testing.T) {
		r, err := applemusic.Recommendations(liveCtx(t), l.api, time.Now().Format("-07:00"))
		if err != nil {
			t.Fatal(err)
		}
		shelves := r.Primary()
		if len(shelves) == 0 {
			t.Fatalf("no shelves (data %d, resource types %d)", len(r.Data), len(r.Resources))
		}
		withItems := 0
		for _, sh := range shelves {
			if sh.Type != applemusic.TypePersonalRecommendations {
				t.Errorf("shelf type %q", sh.Type)
			}
			var a applemusic.RecommendationAttributes
			if err := sh.Attrs(&a); err != nil {
				t.Fatal(err)
			}
			if a.Display.Kind == "" {
				t.Errorf("shelf %s has no display kind", sh.ID)
			}
			if len(r.Resources.Resolve(sh.Rel("contents"))) > 0 {
				withItems++
			}
		}
		if withItems == 0 {
			t.Error("no shelf resolves its contents")
		}
	})

	t.Run("lyrics", func(t *testing.T) {
		ly, err := l.api.Lyrics(liveCtx(t), l.sf, l.song, applemusic.LyricsOptions{Syllable: true, Language: "en-GB"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(ly.TTML, "<tt") || len(ly.Lines) == 0 {
			t.Fatalf("lyrics: ttml %d bytes, %d lines", len(ly.TTML), len(ly.Lines))
		}
		switch ly.Timing {
		case "Word", "Line", "None":
		default:
			t.Errorf("timing %q", ly.Timing)
		}
		if ly.Timing != "None" && ly.Lines[0].End <= ly.Lines[0].Begin {
			t.Errorf("first line not timed: %+v", ly.Lines[0])
		}
	})

	t.Run("stations", func(t *testing.T) {
		ss, err := l.api.LiveStations(liveCtx(t), l.sf)
		if err != nil {
			t.Fatal(err)
		}
		if len(ss) == 0 {
			t.Fatal("no live stations")
		}
		st := ss[0]
		if st.Attributes.Name == "" || !st.Attributes.IsLive || st.Attributes.PlayParams == nil || st.Attributes.PlayParams.ID == "" {
			t.Fatalf("live station = %+v", st.Attributes)
		}
		checkArtwork(t, "station", st.Attributes.Artwork)
		asset, err := l.api.RadioAssets(liveCtx(t), *st.Attributes.PlayParams)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(asset.URL, "https://") || (st.Attributes.PlayParams.HasDRM && asset.KeyServerURL == "") {
			t.Errorf("radio asset = %+v", asset)
		}
	})

	t.Run("playback", func(t *testing.T) {
		ctx := liveCtx(t)
		info, err := l.api.WebPlayback(ctx, l.song, applemusic.Quality256)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Protected() || info.SongID != l.song || info.LicenseURL == "" || info.CertURL == "" {
			t.Fatalf("webPlayback = %+v", info)
		}
		cert, err := l.api.WidevineCert(ctx, info.CertURL)
		if err != nil || len(cert) < 64 {
			t.Fatalf("certificate: %d bytes, %v", len(cert), err)
		}

		// The media playlist: fMP4 with an init map and a Widevine key ID.
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, info.PlaylistURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("playlist: %s, %v", resp.Status, err)
		}
		pl, err := hls.Parse(body)
		if err != nil {
			t.Fatal(err)
		}
		if len(pl.Segments) == 0 || !pl.Ended {
			t.Fatalf("playlist: %d segments, ended %v", len(pl.Segments), pl.Ended)
		}
		seg := pl.Segments[0]
		if seg.Map == nil || seg.Key == nil || len(seg.Key.KID) != 16 || seg.Key.Method != "ISO-23001-7" {
			t.Fatalf("first segment = %+v (key %+v)", seg, seg.Key)
		}

		// The licence exchange's request and answer shape: a challenge the
		// CDM never made must come back as a decoded licence status, not an
		// HTTP or decode failure.
		_, err = l.api.AcquireLicense(ctx, applemusic.LicenseRequest{
			URL: info.LicenseURL, Challenge: []byte("sona live contract test: not a CDM challenge"),
			KeyURI: seg.Key.URI, AdamID: info.SongID,
		})
		var le *applemusic.LicenseError
		if !errors.As(err, &le) || le.Status == 0 {
			t.Fatalf("licence with a junk challenge: %v, want a *LicenseError with a status", err)
		}
		t.Logf("junk challenge → licence status %d", le.Status)
	})
}
