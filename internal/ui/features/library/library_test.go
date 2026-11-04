package library

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	authstore "github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	ilib "github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// recEngine plays instantly and records what it was asked to play.
type recEngine struct {
	mu     sync.Mutex
	st     playback.Status
	plays  []string
	events chan playback.Event
}

func (e *recEngine) Play(_ context.Context, id string, _ time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.plays = append(e.plays, id)
	e.st = playback.Status{State: playback.StatePlaying, ItemID: id, Duration: time.Minute, Volume: 1, UpdatedAt: time.Now()}
	return nil
}

func (e *recEngine) played() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.plays)
}

func (e *recEngine) Status() playback.Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st
}
func (e *recEngine) Preload(context.Context, string) error     { return nil }
func (e *recEngine) SetNext(playback.Next)                     {}
func (e *recEngine) Pause(context.Context) error               { return nil }
func (e *recEngine) Resume(context.Context) error              { return nil }
func (e *recEngine) Seek(context.Context, time.Duration) error { return nil }
func (e *recEngine) SetVolume(context.Context, float64) error  { return nil }
func (e *recEngine) Stop(context.Context) error                { return nil }
func (e *recEngine) Events() <-chan playback.Event             { return e.events }
func (e *recEngine) Close() error                              { return nil }
func (e *recEngine) Capabilities() playback.Capabilities {
	return playback.Capabilities{Kind: playback.KindCDM, Volume: true}
}

type testDev struct{}

func (testDev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev", Expires: time.Now().Add(time.Hour)}, nil
}
func (testDev) Invalidate() {}

// noAccount is a keyring holding no sign-in.
type noAccount struct{}

func (noAccount) Get() (authstore.Record, error) { return authstore.Record{}, authstore.ErrNotFound }
func (noAccount) Set(authstore.Record) error     { return nil }
func (noAccount) Delete() error                  { return nil }
func (noAccount) Backend() authstore.Backend     { return authstore.BackendFile }

// coverCDN answers every artwork request with a red JPEG.
type coverCDN struct {
	mu   sync.Mutex
	hits int
}

func (c *coverCDN) RoundTrip(*http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	img := image.NewRGBA(image.Rect(0, 0, 600, 600))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 200, 40, 40, 255
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, nil)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/jpeg"}}, Body: nopCloser{&b}, ContentLength: int64(b.Len())}, nil
}

type nopCloser struct{ *bytes.Buffer }

func (nopCloser) Close() error { return nil }

// mainThread is a Poster run where the test says.
type mainThread struct{ q chan func() }

func (m *mainThread) post(fn func()) { m.q <- fn }

func (m *mainThread) drain() {
	for {
		select {
		case fn := <-m.q:
			fn()
		default:
			return
		}
	}
}

// albumRow is one seeded album: the library holds it with two songs.
type albumRow struct {
	name, artist string
	single       bool
	explicit     bool
}

// seedAlbums fills the library: album i was added i days after the epoch,
// so "Recently added" lists the last one first.
func seedAlbums(t *testing.T, d *db.DB, albums []albumRow) {
	t.Helper()
	ctx := t.Context()
	for i, a := range albums {
		lib := fmt.Sprintf("l.a%d", i)
		rating := ""
		if a.explicit {
			rating = "explicit"
		}
		res, err := d.W.ExecContext(ctx, `INSERT INTO albums (library_id, catalog_id, name, artist_name, artwork_template, release_date, track_count, content_rating, is_single, updated_at)
			VALUES (?, ?, ?, ?, 'https://is1-ssl.mzstatic.com/image/thumb/Music/x/{w}x{h}bb.jpg', ?, 2, ?, ?, 0)`,
			lib, fmt.Sprint(100+i), a.name, a.artist, fmt.Sprintf("20%02d-01-01", 10+i), rating, a.single)
		if err != nil {
			t.Fatal(err)
		}
		ref, _ := res.LastInsertId()
		if _, err := d.W.ExecContext(ctx, `INSERT INTO library_items (kind, ref, date_added, generation) VALUES ('album', ?, ?, 1)`, ref, (i+1)*86_400_000); err != nil {
			t.Fatal(err)
		}
		for j := range 2 {
			if _, err := d.W.ExecContext(ctx, `INSERT INTO tracks (library_id, catalog_id, name, artist_name, album_name, album_library_id, album_catalog_id, artwork_template, duration_ms, track_number, disc_number, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, 'https://is1-ssl.mzstatic.com/image/thumb/Music/x/{w}x{h}bb.jpg', 60000, ?, 1, 0)`,
				fmt.Sprintf("i.a%dt%d", i, j), fmt.Sprintf("%d%d", 1000+i, j), fmt.Sprintf("%s song %d", a.name, j+1), a.artist, a.name, lib, fmt.Sprint(100+i), j+1); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// seven albums, two of them singles, one explicit.
var sevenAlbums = []albumRow{
	{name: "Zebra Crossing", artist: "Mia"},
	{name: "Apple Orchard", artist: "Zed", explicit: true},
	{name: "Lone Wolf", artist: "Bo", single: true},
	{name: "Mango Season", artist: "Kay"},
	{name: "Blue Hour", artist: "Al"},
	{name: "Single Malt", artist: "Cy", single: true},
	{name: "Paper Planes", artist: "Di"},
}

type rig struct {
	t      *testing.T
	d      *db.DB
	core   *services.Core
	eng    *recEngine
	cdn    *coverCDN
	mt     *mainThread
	env    *module.Env
	router *ui.Router
	l      *library
	tt     *ui.Tester
	// asked records the albums queries the service answered.
	asked *recordingAPI
}

// recordingAPI passes through to the real LibraryService and notes what was asked.
type recordingAPI struct {
	libraryAPI
	mu    sync.Mutex
	pages []string
}

func (r *recordingAPI) Albums(ctx context.Context, sort string, desc bool, typ string, off, limit int) (services.AlbumPage, error) {
	r.mu.Lock()
	r.pages = append(r.pages, fmt.Sprintf("%s desc=%v type=%q offset=%d", sort, desc, typ, off))
	r.mu.Unlock()
	return r.libraryAPI.Albums(ctx, sort, desc, typ, off, limit)
}

func (r *recordingAPI) asks() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.pages)
}

func newRig(t *testing.T, albums []albumRow, width, height int) *rig {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	dir := t.TempDir()
	d, err := db.Open(t.Context(), filepath.Join(dir, "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	seedAlbums(t, d, albums)

	r := &rig{t: t, d: d, cdn: &coverCDN{}, mt: &mainThread{q: make(chan func(), 256)}}
	art, err := artwork.New(artwork.Options{Dir: filepath.Join(dir, "art"), HTTP: &http.Client{Transport: r.cdn}})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := config.OpenStore(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Update(func(s *config.Settings) { s.AutoPlay = false }); err != nil {
		t.Fatal(err)
	}
	authm, err := auth.NewManager(auth.Config{
		HTTP: http.DefaultClient, Dev: testDev{}, APIBase: "http://127.0.0.1:1",
		Open:     func() (authstore.Store, error) { return noAccount{}, nil },
		Accounts: services.AccountStore{DB: d},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authm.Close)
	r.eng = &recEngine{events: make(chan playback.Event)}
	coord, err := coordinator.New(coordinator.Options{Engine: r.eng})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	bus := store.NewBus(nil)
	bus.SetPoster(r.mt.post)
	r.core = services.New(services.Deps{
		Paths: config.Paths{Config: dir, Data: dir, Cache: dir, Art: dir, State: dir}, Settings: settings, DB: d, Auth: authm,
		Library: ilib.New(d, nil), Art: art, Engine: r.eng, Coord: coord,
		Private: privatemode.New(privatemode.Options{}), Gate: services.NewGate(time.Hour, nil),
		Emit: bus, Windows: noWindows{}, Started: time.Now(),
	})
	t.Cleanup(r.core.Close)
	authm.Start()

	loader := kit.NewArtLoader(art, r.mt.post)
	t.Cleanup(loader.Close)
	r.router = ui.NewRouter(albumsPath)
	r.env = &module.Env{
		Core: r.core, Bus: bus, Post: r.mt.post, Queries: store.NewClient(r.mt.post),
		Play: store.NewPlayback(bus, nil), Art: loader, Nav: r.router,
	}
	r.l = newLibrary(r.env, nil, nil, nil)
	r.asked = &recordingAPI{libraryAPI: services.Service[services.LibraryService](r.core)}
	r.l.lib = r.asked
	r.l.play = services.Service[services.PlaybackService](r.core)
	r.l.auth = services.Service[services.AuthService](r.core)
	r.l.init()
	r.tt = ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(albumsPath) {
					r.l.albumsPage(c, rt)
				}
			})
		})
	}, width, height)
	return r
}

// until runs what was posted and draws frames until ok, waiting for the
// service calls and cover decodes that run on goroutines.
func (r *rig) until(what string, ok func() bool) {
	r.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		r.mt.drain()
		r.tt.Frame()
		if ok() {
			return
		}
	}
	r.t.Fatalf("timed out waiting for %s; texts %q", what, r.tt.Texts())
}

func (r *rig) shows(texts ...string) func() bool {
	return func() bool {
		for _, w := range texts {
			if !r.tt.HasText(w) {
				return false
			}
		}
		return true
	}
}

// order is the albums' names in the order the page shows them.
func (r *rig) order(names []string) []string {
	var got []string
	for _, w := range r.tt.Texts() {
		if slices.Contains(names, w) {
			got = append(got, w)
		}
	}
	return got
}

func names(albums []albumRow) []string {
	out := make([]string, len(albums))
	for i, a := range albums {
		out[i] = a.name
	}
	return out
}

// noWindows is a window control with no window: the services only follow it.
type noWindows struct{ services.Windows }

func (noWindows) WatchMain(func(windowchrome.WindowState)) {}
func (noWindows) WatchNavigate(func(bool))                 {}
func (noWindows) DeveloperTools() bool                     { return false }

func TestAlbumsGridShowsTheLibrary(t *testing.T) {
	r := newRig(t, sevenAlbums, 1132, 1000)
	r.until("the albums", r.shows("Zebra Crossing", "Paper Planes", "Albums", "7 albums"))
	// "Recently added" lists the album added last first.
	want := []string{"Paper Planes", "Single Malt", "Blue Hour", "Mango Season", "Lone Wolf", "Apple Orchard", "Zebra Crossing"}
	if got := r.order(names(sevenAlbums)); !slices.Equal(got, want) {
		t.Errorf("albums shown %q, want %q", got, want)
	}
	for _, w := range []string{"Mia", "Zed", "All", "Singles & EPs", "Album type", "Sort"} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q", w)
		}
	}
	if asks := r.asked.asks(); len(asks) == 0 || asks[0] != `dateAdded desc=true type="" offset=0` {
		t.Errorf("first query %q", asks)
	}
	// The covers are fetched through the artwork cache and drawn.
	r.until("the covers", func() bool { r.cdn.mu.Lock(); defer r.cdn.mu.Unlock(); return r.cdn.hits > 0 })
	r.until("a cover drawn", func() bool { return coverPixels(r) })
}

// coverPixels reports whether the last frame has the covers' red in it.
func coverPixels(r *rig) bool {
	img := r.tt.Image()
	red := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			if cr>>8 > 150 && cg>>8 < 90 && cb>>8 < 90 {
				red++
			}
		}
	}
	return red > 200
}

func TestSortAndTypeChangeTheQuery(t *testing.T) {
	r := newRig(t, sevenAlbums, 1132, 1000)
	r.until("the albums", r.shows("Paper Planes"))

	// Sort: the select's choice goes to the URL and the query.
	if err := r.tt.Click("Sort"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.Click("Title"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != albumsPath+"?desc=false&sort=title" {
		t.Fatalf("after choosing Title the URL is %q", got)
	}
	want := []string{"Apple Orchard", "Blue Hour", "Lone Wolf", "Mango Season", "Paper Planes", "Single Malt", "Zebra Crossing"}
	r.until("title order", func() bool { return slices.Equal(r.order(names(sevenAlbums)), want) })
	if asks := r.asked.asks(); !slices.Contains(asks, `title desc=false type="" offset=0`) {
		t.Errorf("service asks %q", asks)
	}

	// Type: Singles & EPs narrows the list, and the line under the title.
	if err := r.tt.Click("Singles & EPs"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != albumsPath+"?desc=false&sort=title&type=singles" {
		t.Fatalf("after choosing Singles & EPs the URL is %q", got)
	}
	r.until("singles", r.shows("Lone Wolf", "Single Malt", "2 singles & EPs"))
	if r.tt.HasText("Zebra Crossing") {
		t.Error("a standard album shows under Singles & EPs")
	}
	if asks := r.asked.asks(); !slices.Contains(asks, `title desc=false type="singles" offset=0`) {
		t.Errorf("service asks %q", asks)
	}

	// All again.
	if err := r.tt.Click("All"); err != nil {
		t.Fatal(err)
	}
	r.until("all albums", r.shows("Zebra Crossing", "7 albums"))
}

func TestAnAlbumsPlayButtonPlaysTheAlbum(t *testing.T) {
	r := newRig(t, sevenAlbums, 1132, 1000)
	r.until("the albums", r.shows("Mango Season"))
	if err := r.tt.Click("Play Mango Season"); err != nil {
		t.Fatal(err)
	}
	r.until("the engine to play", func() bool { return len(r.eng.played()) > 0 })
	// Mango Season is album 3: its first song, catalogue ID 10030.
	if got := r.eng.played(); got[0] != "10030" {
		t.Errorf("played %v, want the album's first song 10030", got)
	}
	q := services.Service[services.PlaybackService](r.core).Queue()
	if len(q.Items) != 2 || q.Items[0].Name != "Mango Season song 1" {
		t.Errorf("queue %+v", q.Items)
	}
	if st := services.Service[services.PlaybackService](r.core).State(); st.Container == nil || st.Container.Kind != "album" || st.Container.ID != "l.a3" {
		t.Errorf("the queue's container is %+v, want album l.a3", st.Container)
	}
	// Tapping the tile itself goes nowhere: there is no album page yet.
	before := r.router.Location()
	_ = r.tt.Click("Lone Wolf")
	if r.router.Location() != before {
		t.Errorf("a tile navigated to %q", r.router.Location())
	}
}

func TestASyncRefreshesTheGrid(t *testing.T) {
	r := newRig(t, sevenAlbums[:2], 1132, 1000)
	r.until("the albums", r.shows("2 albums"))
	seedMore := []albumRow{{name: "Brand New"}}
	// A third album arrives with a sync, which tells the UI when a domain
	// finishes.
	res, err := r.d.W.ExecContext(t.Context(), `INSERT INTO albums (library_id, catalog_id, name, artist_name, updated_at) VALUES ('l.new', '999', ?, '', 0)`, seedMore[0].name)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := res.LastInsertId()
	if _, err := r.d.W.ExecContext(t.Context(), `INSERT INTO library_items (kind, ref, date_added, generation) VALUES ('album', ?, 99999999999, 1)`, ref); err != nil {
		t.Fatal(err)
	}
	r.env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "albums", Done: 3, Total: 3, Finished: true})
	r.until("the new album", r.shows("Brand New", "3 albums"))

	// sync:done refreshes everything the library shows.
	if _, err := r.d.W.ExecContext(t.Context(), `DELETE FROM library_items WHERE kind = 'album' AND ref = ?`, ref); err != nil {
		t.Fatal(err)
	}
	r.env.Bus.Emit(services.EventSyncDone, services.SyncDone{OK: true})
	r.until("the album gone", func() bool { return !r.tt.HasText("Brand New") && r.tt.HasText("2 albums") })
}

func TestAnUnfinishedSyncTickRefetchesNothing(t *testing.T) {
	r := newRig(t, sevenAlbums[:2], 1132, 1000)
	r.until("the albums", r.shows("2 albums"))
	res, err := r.d.W.ExecContext(t.Context(), `INSERT INTO albums (library_id, catalog_id, name, artist_name, updated_at) VALUES ('l.new', '999', 'Brand New', '', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := res.LastInsertId()
	if _, err := r.d.W.ExecContext(t.Context(), `INSERT INTO library_items (kind, ref, date_added, generation) VALUES ('album', ?, 99999999999, 1)`, ref); err != nil {
		t.Fatal(err)
	}
	r.env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "albums", Done: 1, Total: 3})
	for end := time.Now().Add(300 * time.Millisecond); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		r.mt.drain()
		r.tt.Frame()
		if r.tt.HasText("Brand New") {
			t.Fatal("a tick inside a domain refetched the albums")
		}
	}
	r.env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "albums", Done: 3, Total: 3, Finished: true})
	r.until("the new album", r.shows("Brand New"))
}

func TestAnEmptyLibraryKeepsItsFrameAndSaysWhy(t *testing.T) {
	r := newRig(t, nil, 1132, 800)
	// No account in the keyring: the sign-in text.
	r.until("the signed-out text", r.shows("Sign in to see your albums", "Albums"))
	if r.tt.HasText("0 albums") {
		t.Error("an empty library counts its albums")
	}
	// A filter with nothing in it keeps the page and offers all albums.
	r2 := newRig(t, []albumRow{{name: "Only Album", artist: "X"}}, 1132, 800)
	r2.until("the album", r2.shows("Only Album"))
	r2.router.Replace(albumsPath + "?type=singles")
	r2.until("the filtered-empty state", r2.shows("No singles or EPs in your library", "Show all albums"))
	if err := r2.tt.Click("Show all albums"); err != nil {
		t.Fatal(err)
	}
	r2.until("the album again", r2.shows("Only Album", "1 album"))
}

func TestSyncInvalidations(t *testing.T) {
	if got := syncInvalidations(services.SyncProgress{Domain: "albums"}); got != nil {
		t.Errorf("a tick inside a domain invalidated %v", got)
	}
	got := syncInvalidations(services.SyncProgress{Domain: "albums", Finished: true})
	if !strings.Contains(fmt.Sprint(got), "[library albums]") || !strings.Contains(fmt.Sprint(got), "[library counts]") {
		t.Errorf("albums finished invalidates %v", got)
	}
	if got := syncInvalidations(services.SyncProgress{Domain: "unknown", Finished: true}); len(got) != 3 {
		t.Errorf("an unknown domain invalidates %v, want everything under library and the counts", got)
	}
}
