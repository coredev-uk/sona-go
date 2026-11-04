package library

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// seven albums, two of them singles, one explicit.
var sevenAlbums = []fixture.Album{
	{Name: "Zebra Crossing", Artist: "Mia"},
	{Name: "Apple Orchard", Artist: "Zed", Explicit: true},
	{Name: "Lone Wolf", Artist: "Bo", Single: true},
	{Name: "Mango Season", Artist: "Kay"},
	{Name: "Blue Hour", Artist: "Al"},
	{Name: "Single Malt", Artist: "Cy", Single: true},
	{Name: "Paper Planes", Artist: "Di"},
}

type rig struct {
	t      *testing.T
	h      *uitest.Harness
	router *ui.Router
	l      *library
	tt     *uitest.Session
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

func newRig(t *testing.T, albums []fixture.Album, width, height int) *rig {
	t.Helper()
	opts := []uitest.Option{uitest.Albums(albums...)}
	if len(albums) == 0 {
		opts = []uitest.Option{uitest.Empty()}
	}
	h := uitest.New(t, opts...)
	r := &rig{t: t, h: h, router: ui.NewRouter(albumsPath)}
	h.Env.Nav = r.router
	r.l = newLibrary(h.Env, nil, nil)
	r.asked = &recordingAPI{libraryAPI: services.Service[services.LibraryService](h.Core)}
	r.l.lib = r.asked
	r.l.play = services.Service[services.PlaybackService](h.Core)
	seedAuth(h)
	r.l.init()
	r.tt = h.Tester(t, func(c *ui.Context) {
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

// order is the albums' names in the order the page shows them.
func (r *rig) order(names []string) []string {
	var got []string
	for _, w := range r.tt.Texts() {
		// A tile names itself twice: its opener's label and its title.
		if slices.Contains(names, w) && (len(got) == 0 || got[len(got)-1] != w) {
			got = append(got, w)
		}
	}
	return got
}

func names(albums []fixture.Album) []string {
	out := make([]string, len(albums))
	for i, a := range albums {
		out[i] = a.Name
	}
	return out
}

func TestAlbumsGridShowsTheLibrary(t *testing.T) {
	r := newRig(t, sevenAlbums, 1132, 1000)
	r.tt.Until("the albums", r.tt.Shows("Zebra Crossing", "Paper Planes", "Albums", "7 albums"))
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
	r.tt.Until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
	r.tt.Until("a cover drawn", func() bool { return coverPixels(r) })
}

// coverPixels reports whether the last frame has the generated covers'
// colours in it: pixels near the first gradient colour of one of the albums.
func coverPixels(r *rig) bool {
	img := r.tt.Image()
	b := img.Bounds()
	near := make([]int, len(sevenAlbums))
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			for i := range near {
				c0, _, _ := fixture.CoverColors(i)
				if abs(int(cr>>8)-int(c0.R))+abs(int(cg>>8)-int(c0.G))+abs(int(cb>>8)-int(c0.B)) < 40 {
					near[i]++
				}
			}
		}
	}
	for _, n := range near {
		if n > 100 {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func TestSortAndTypeChangeTheQuery(t *testing.T) {
	r := newRig(t, sevenAlbums, 1132, 1000)
	r.tt.Until("the albums", r.tt.Shows("Paper Planes"))

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
	r.tt.Until("title order", func() bool { return slices.Equal(r.order(names(sevenAlbums)), want) })
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
	r.tt.Until("singles", r.tt.Shows("Lone Wolf", "Single Malt", "2 singles & EPs"))
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
	r.tt.Until("all albums", r.tt.Shows("Zebra Crossing", "7 albums"))
}

func TestAnAlbumTileHasNoPlayButton(t *testing.T) {
	r := newRig(t, sevenAlbums, 1132, 1000)
	r.tt.Until("the albums", r.tt.Shows("Mango Season"))
	// v0.2.0's library albums are CoverTiles with no play control.
	if _, ok := r.tt.Find("Play Mango Season"); ok {
		t.Error("an album tile has a play button")
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
	r.tt.Until("the albums", r.tt.Shows("2 albums"))
	seedMore := []fixture.Album{{Name: "Brand New"}}
	// A third album arrives with a sync, which tells the UI when a domain
	// finishes.
	res, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO albums (library_id, catalog_id, name, artist_name, updated_at) VALUES ('l.new', '999', ?, '', 0)`, seedMore[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := res.LastInsertId()
	if _, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO library_items (kind, ref, date_added, generation) VALUES ('album', ?, 99999999999, 1)`, ref); err != nil {
		t.Fatal(err)
	}
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "albums", Done: 3, Total: 3, Finished: true})
	r.tt.Until("the new album", r.tt.Shows("Brand New", "3 albums"))

	// sync:done refreshes everything the library shows.
	if _, err := r.h.DB.W.ExecContext(t.Context(), `DELETE FROM library_items WHERE kind = 'album' AND ref = ?`, ref); err != nil {
		t.Fatal(err)
	}
	r.h.Env.Bus.Emit(services.EventSyncDone, services.SyncDone{OK: true})
	r.tt.Until("the album gone", func() bool { return !r.tt.HasText("Brand New") && r.tt.HasText("2 albums") })
}

func TestAnUnfinishedSyncTickRefetchesNothing(t *testing.T) {
	r := newRig(t, sevenAlbums[:2], 1132, 1000)
	r.tt.Until("the albums", r.tt.Shows("2 albums"))
	res, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO albums (library_id, catalog_id, name, artist_name, updated_at) VALUES ('l.new', '999', 'Brand New', '', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := res.LastInsertId()
	if _, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO library_items (kind, ref, date_added, generation) VALUES ('album', ?, 99999999999, 1)`, ref); err != nil {
		t.Fatal(err)
	}
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "albums", Done: 1, Total: 3})
	for end := time.Now().Add(300 * time.Millisecond); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		r.h.Main.Drain()
		r.tt.Frame()
		if r.tt.HasText("Brand New") {
			t.Fatal("a tick inside a domain refetched the albums")
		}
	}
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "albums", Done: 3, Total: 3, Finished: true})
	r.tt.Until("the new album", r.tt.Shows("Brand New"))
}

func TestAnEmptyLibraryKeepsItsFrameAndSaysWhy(t *testing.T) {
	r := newRig(t, nil, 1132, 800)
	// No account in the keyring: the sign-in text.
	r.tt.Until("the signed-out text", r.tt.Shows("Sign in to see your albums", "Albums"))
	if r.tt.HasText("0 albums") {
		t.Error("an empty library counts its albums")
	}
	// A filter with nothing in it keeps the page and offers all albums.
	r2 := newRig(t, []fixture.Album{{Name: "Only Album", Artist: "X"}}, 1132, 800)
	r2.tt.Until("the album", r2.tt.Shows("Only Album"))
	r2.router.Replace(albumsPath + "?type=singles")
	r2.tt.Until("the filtered-empty state", r2.tt.Shows("No singles or EPs in your library", "Show all albums"))
	if err := r2.tt.Click("Show all albums"); err != nil {
		t.Fatal(err)
	}
	r2.tt.Until("the album again", r2.tt.Shows("Only Album", "1 album"))
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
