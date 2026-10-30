package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/db"
)

// fakeAMP serves trimmed real amp-api responses from testdata/discover and
// records the requests it saw.
type fakeAMP struct {
	t      *testing.T
	mu     sync.Mutex
	seen   []string
	status int // non-zero: fail every request with it
}

func (f *fakeAMP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.seen = append(f.seen, r.Method+" "+r.URL.Path)
	status := f.status
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, `{"errors":[]}`, status)
		return
	}
	if r.URL.Path != "/v1/catalog/gb/genres" && (r.URL.Query().Get("format[resources]") != "map" || r.URL.Query().Get("platform") != "web") {
		f.t.Errorf("%s %s: missing map/platform query: %s", r.Method, r.URL.Path, r.URL.RawQuery)
	}
	file := ""
	switch p := r.URL.Path; {
	case p == "/v1/me/recommendations" && r.Method == http.MethodGet:
		file = "home.json"
	case strings.HasPrefix(p, "/v1/me/recommendations/") && r.Method == http.MethodPost:
		// A refreshed Recently Played shelf: the same shelf, one item fewer.
		w.Write([]byte(`{"data":[{"id":"7-2Tqlz47h9yro","type":"personal-recommendation"}],"resources":{"personal-recommendation":{"7-2Tqlz47h9yro":{"id":"7-2Tqlz47h9yro","type":"personal-recommendation","attributes":{"title":{"stringForDisplay":"Recently Played"},"kind":"recently-played","display":{"kind":"MusicCoverShelf"},"hasSeeAll":true,"nextUpdateDate":"2026-10-30T12:30:00Z"},"relationships":{"contents":{"data":[{"id":"6812476961","type":"albums"}]}}}}}}`))
		return
	case p == "/v1/editorial/gb/groupings":
		file = "new.json"
	case p == "/v1/catalog/gb":
		file = "curator.json"
	case strings.HasPrefix(p, "/v1/editorial/gb/rooms/"):
		file = "room.json"
	case strings.HasPrefix(p, "/v1/editorial/gb/multirooms/"):
		file = "multiroom.json"
	case p == "/v1/catalog/gb/charts":
		file = "charts.json"
	case p == "/v1/catalog/gb/genres":
		w.Write([]byte(`{"data":[{"id":"34","type":"genres","attributes":{"name":"Music"}},{"id":"14","type":"genres","attributes":{"name":"Pop","parentId":"34"}}]}`))
		return
	default:
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(filepath.Join("testdata", "discover", file))
	if err != nil {
		f.t.Fatal(err)
	}
	w.Write(b)
}

func (f *fakeAMP) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

type discoverEnv struct {
	s    *DiscoverService
	amp  *fakeAMP
	now  *time.Time
	gate *Gate
	ev   *recorder
}

func newDiscoverEnv(t *testing.T) discoverEnv {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	amp := &fakeAMP{t: t}
	srv := httptest.NewServer(amp)
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return "mut" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	art, err := artwork.New(artwork.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 10, 59, 0, 0, time.UTC)
	gate := NewGate(time.Hour, nil)
	ev := &recorder{}
	e := &Env{
		Deps: Deps{
			DB: d, API: api, Auth: &auth.Manager{}, Art: art, Gate: gate, Emit: ev,
			Storefront: func() string { return "gb" },
			Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			Now:        func() time.Time { return now },
		},
		m: mapper{art},
	}
	return discoverEnv{s: &DiscoverService{e: e, m: e.m}, amp: amp, now: &now, gate: gate, ev: ev}
}

func titles(secs []Section) []string {
	out := make([]string, len(secs))
	for i, s := range secs {
		out[i] = s.Kind + ":" + s.Title
	}
	return out
}

func TestHomeCacheAndRefresh(t *testing.T) {
	env := newDiscoverEnv(t)
	ctx := t.Context()

	// No cache: Home asks Apple.
	page, err := env.s.Home(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(page.Sections), " | ")
	// Concerts is left out; Replay keeps its playlists but not the web-only
	// Replay card.
	want := "feature:Top Picks for You | shelf:Recently Played | hero: | feature:Replay: Your Top Music"
	if got != want {
		t.Fatalf("sections = %s\nwant       %s", got, want)
	}
	top := page.Sections[0]
	if top.Items[0].Kind != ItemPlaylist || top.Items[0].Caption != "Made for You" || top.Items[0].Playlist == nil {
		t.Errorf("top pick = %+v", top.Items[0])
	}
	if it := top.Items[2]; it.Kind != ItemStation || it.Station == nil || it.Caption != "Mood for You" {
		t.Errorf("station pick = %+v", it)
	}
	if !page.Sections[1].SeeAll {
		t.Error("Recently Played lost its See all")
	}
	if hero := page.Sections[2].Items[0]; hero.Wide.Large == "" || hero.Color == "" {
		t.Errorf("hero card without wide art: %+v", hero)
	}
	for _, it := range page.Sections[3].Items {
		if it.Kind != ItemPlaylist {
			t.Errorf("Replay item %s %q kept", it.Kind, it.Name)
		}
	}
	if n := len(env.amp.requests()); n != 1 {
		t.Fatalf("requests = %v", env.amp.requests())
	}

	// Cached: no request, even with the network closed.
	if _, err := env.s.Home(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(env.amp.requests()); n != 1 {
		t.Fatalf("cached Home asked Apple: %v", env.amp.requests())
	}

	// After first paint, with only Recently Played due, that shelf alone is
	// refreshed and the new page is emitted.
	*env.now = time.Date(2026, 10, 30, 11, 1, 0, 0, time.UTC)
	env.gate.Open()
	env.s.refreshHomeIfDue(ctx) // due by Apple's date, but fetched a minute ago
	if n := len(env.amp.requests()); n != 1 {
		t.Fatalf("refreshed within the minimum life: %v", env.amp.requests())
	}
	*env.now = time.Date(2026, 10, 30, 11, 5, 0, 0, time.UTC)
	env.s.refreshHomeIfDue(ctx)
	reqs := env.amp.requests()
	if len(reqs) != 2 || reqs[1] != "POST /v1/me/recommendations/7-2Tqlz47h9yro" {
		t.Fatalf("requests = %v", reqs)
	}
	if len(env.ev.events) != 1 || env.ev.events[0] != EventHome {
		t.Fatalf("events = %v", env.ev.events)
	}
	page, _ = env.s.Home(ctx) // served from cache; nothing else is due
	if n := len(page.Sections[1].Items); n != 1 {
		t.Errorf("refreshed Recently Played has %d items, want 1", n)
	}

	// When most shelves are due, Home is refetched whole.
	*env.now = time.Date(2026, 10, 30, 13, 0, 0, 0, time.UTC)
	env.s.refreshHomeIfDue(ctx)
	reqs = env.amp.requests()
	if last := reqs[len(reqs)-1]; last != "GET /v1/me/recommendations" {
		t.Errorf("last request = %s", last)
	}
}

func TestHomeSignedOut(t *testing.T) {
	env := newDiscoverEnv(t)
	env.s.e.API.UserToken = func() string { return "" }
	page, err := env.s.Home(t.Context())
	if err != nil || len(page.Sections) != 0 || page.Sections == nil {
		t.Fatalf("signed-out Home = %+v, %v", page, err)
	}
	if n := len(env.amp.requests()); n != 0 {
		t.Errorf("signed-out Home asked Apple: %v", env.amp.requests())
	}
}

func TestHomeExpiry(t *testing.T) {
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	shelf := func(id, next string) string {
		return `"` + id + `":{"id":"` + id + `","type":"personal-recommendation","attributes":{"nextUpdateDate":"` + next + `"}}`
	}
	var r applemusic.MapResponse
	body := `{"data":[{"id":"a","type":"personal-recommendation"},{"id":"b","type":"personal-recommendation"}],"resources":{"personal-recommendation":{` +
		shelf("a", "2026-10-30T10:30:00Z") + "," + shelf("b", "2026-10-30T09:00:00Z") + `}}}`
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	// A date in the past is clamped to a minute ahead.
	if got := homeExpiry(r, now); !got.Equal(now.Add(homeMinLife)) {
		t.Errorf("expiry = %v", got)
	}
	if due := dueShelves(r, now.Add(-time.Hour), now); len(due) != 1 || due[0] != "b" {
		t.Errorf("due = %v", due)
	}
	// Nothing is due within homeMinLife of the fetch, whatever Apple says.
	if due := dueShelves(r, now.Add(-time.Minute), now); len(due) != 0 {
		t.Errorf("due = %v", due)
	}
}

func TestNewSections(t *testing.T) {
	env := newDiscoverEnv(t)
	page, err := env.s.New(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(page.Sections), " | ")
	want := "hero: | tracks:Best New Songs | shelf:New This Week | bricks:New for October | links:More to Explore | shelf:Listen to Live Radio Now"
	if got != want {
		t.Fatalf("sections = %s\nwant       %s", got, want)
	}
	hero := page.Sections[0].Items
	if hero[0].Caption != "Updated playlist" || hero[0].Kind != ItemPlaylist || hero[0].Wide.Large == "" {
		t.Errorf("hero card 0 = %+v", hero[0])
	}
	// 320 cards carry their own art and tagline.
	if hero[1].Kind != ItemCurator || !strings.HasPrefix(hero[1].Subtitle, "Hear Overmono") || hero[1].Caption != "Upcoming livestream" {
		t.Errorf("hero card 1 = %+v", hero[1])
	}
	if s := page.Sections[1]; s.Room != "6828645880" || s.Items[0].Song == nil {
		t.Errorf("tracks = %+v", s)
	}
	bricks := page.Sections[3].Items
	if bricks[0].Kind != ItemCurator || bricks[3].Kind != ItemMultiroom || bricks[3].ID != "1425024236" || bricks[3].Name != "Coming Soon" {
		t.Errorf("bricks = %+v / %+v", bricks[0], bricks[3])
	}
	links := map[string]Item{}
	for _, l := range page.Sections[4].Items {
		links[l.Name] = l
	}
	for name, want := range map[string][2]string{
		"Concerts":        {ItemLink, ""},
		"Browse by Genre": {ItemRoom, "6456176470"},
		"Charts":          {ItemCharts, ""},
		"Music Videos":    {ItemGrouping, "10023"},
	} {
		if l := links[name]; l.Kind != want[0] || l.ID != want[1] {
			t.Errorf("link %s = %s %s, want %v", name, l.Kind, l.ID, want)
		}
	}
	if st := page.Sections[5].Items[0]; st.Kind != ItemStation || st.Station == nil {
		t.Errorf("live station = %+v", st)
	}
}

func TestParseAppleLink(t *testing.T) {
	for _, tc := range []struct{ url, kind, id string }{
		{"https://music.apple.com/gb/curator/the-zane-lowe-show/990050553", ItemCurator, "990050553"},
		{"https://music.apple.com/gb/multi-room/6447561826", ItemMultiroom, "6447561826"},
		{"https://music.apple.com/gb/room/6828645964", ItemRoom, "6828645964"},
		{"https://music.apple.com/gb/album/sessions/6827329958", ItemAlbum, "6827329958"},
		{"https://music.apple.com/gb/playlist/a-list-pop/pl.5ee8333dbe944d9f9151e97d92d1ead9", ItemPlaylist, "pl.5ee8333dbe944d9f9151e97d92d1ead9"},
		{"https://music.apple.com/gb/new/top-charts", ItemCharts, ""},
		{"https://music.apple.com/gb/new/top-charts/songs/14", ItemCharts, "14"},
		{"https://itunes.apple.com/WebObjects/MZStore.woa/wa/viewTop?genreId=34", ItemCharts, ""},
		{"https://itunes.apple.com/WebObjects/MZStore.woa/wa/viewMultiRoom?cc=gb&fcId=1425024236&mt=1", ItemMultiroom, "1425024236"},
		{"https://music.apple.com/WebObjects/MZStore.woa/wa/viewGrouping?cc=gb&id=172080", ItemGrouping, "172080"},
		{"https://itunes.apple.com/gb/collection/decades/id178644?fcId=6456176471&mt=1", ItemRoom, "6456176471"},
		{"https://music.apple.com/concerts", "", ""},
		{"https://replay.music.apple.com/gb", "", ""},
		{"https://example.com/gb/album/x/1", "", ""},
	} {
		kind, id := parseAppleLink(tc.url)
		if kind != tc.kind || id != tc.id {
			t.Errorf("%s → %q %q, want %q %q", tc.url, kind, id, tc.kind, tc.id)
		}
	}
}

func TestSentenceCase(t *testing.T) {
	for in, want := range map[string]string{
		"NEW ALBUM": "New album", "ONLY ON APPLE MUSIC": "Only on Apple Music", "NEW DJ MIX": "New DJ mix",
		"Made for You": "Made for You", "": "",
	} {
		if got := sentenceCase(in); got != want {
			t.Errorf("sentenceCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCuratorPageAndStale(t *testing.T) {
	env := newDiscoverEnv(t)
	ctx := t.Context()
	env.gate.Open()
	p, err := env.s.Curator(ctx, "990050553")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "The Zane Lowe Show" || p.Subtitle != "Radio show with Zane Lowe" || p.Wide.Large == "" || p.Notes == "" {
		t.Errorf("curator = %+v", p)
	}
	if got := strings.Join(titles(p.Sections), " | "); !strings.HasPrefix(got, "shelf:The Interview Series | shelf:Episodes") {
		t.Errorf("sections = %s", got)
	}
	if p.Stale {
		t.Error("fresh page marked stale")
	}

	// Past its lifetime with Apple failing, the cached page is served stale.
	*env.now = env.now.Add(curatorTTL + time.Minute)
	env.amp.status = http.StatusBadRequest
	p, err = env.s.Curator(ctx, "990050553")
	if err != nil || !p.Stale || p.Title != "The Zane Lowe Show" {
		t.Fatalf("stale curator = %+v, %v", p.Title, err)
	}
}

func TestRoomMultiroomAndCharts(t *testing.T) {
	env := newDiscoverEnv(t)
	ctx := t.Context()
	room, err := env.s.Room(ctx, "6828645964", "")
	if err != nil {
		t.Fatal(err)
	}
	if room.Title != "New This Week" || room.Kind != SectionShelf || len(room.Items) != 3 || room.Next != 100 || room.Sort != "featured" || len(room.Sorts) != 3 {
		t.Errorf("room = %+v", room)
	}

	mr, err := env.s.Multiroom(ctx, "1425024236")
	if err != nil {
		t.Fatal(err)
	}
	if mr.Title != "Coming Soon" || mr.Wide.Large == "" || len(mr.Sections) < 2 || mr.Sections[0].Kind != SectionText || mr.Sections[1].Room == "" {
		t.Errorf("multiroom = %s %v", mr.Title, titles(mr.Sections))
	}

	ch, err := env.s.Charts(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, c := range ch.Charts {
		keys = append(keys, c.Key+"/"+c.ChartID)
	}
	if got := strings.Join(keys, " "); got != "songs/ cityCharts/135 dailyGlobalTopCharts/119 playlists/ albums/ music-videos/" {
		t.Errorf("charts = %s", got)
	}
	if s := ch.Charts[0].Items[0]; s.Kind != ItemSong || s.Song == nil || s.Song.AlbumID == "" || ch.Charts[0].Next != 50 {
		t.Errorf("top song = %+v next %d", s, ch.Charts[0].Next)
	}
	if len(ch.Genres) != 2 || ch.Genres[0].ID != "" || ch.Genres[1].Name != "Pop" {
		t.Errorf("genres = %+v", ch.Genres)
	}
	if _, err := env.s.ChartItems(ctx, ChartCities, "", "", 50); err == nil {
		t.Error("city chart page without a chart ID accepted")
	}
}

func TestSlimResources(t *testing.T) {
	var r applemusic.MapResponse
	body := `{"data":[{"id":"1","type":"albums"}],"resources":{"albums":{"1":{"id":"1","type":"albums",
		"attributes":{"name":"A","editorialVideo":{"motionSquare":{}},"editorialArtwork":{"superHeroWide":{"url":"u"},"staticDetailTall":{"url":"t"}}}}}}}`
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	slimResources(r.Resources)
	raw := string(r.Primary()[0].Raw)
	if strings.Contains(raw, "editorialVideo") || strings.Contains(raw, "staticDetailTall") || !strings.Contains(raw, "superHeroWide") || !strings.Contains(raw, `"name":"A"`) {
		t.Errorf("slimmed = %s", raw)
	}
}
