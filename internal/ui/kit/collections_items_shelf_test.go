package kit

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

func albumItem(i int) services.Item {
	id := strconv.Itoa(i)
	return services.Item{
		Kind: services.ItemAlbum, ID: id, Name: "Album " + id, Subtitle: "Artist " + id,
		Art:   services.Art{Medium: "/art/ka" + id + "/400x400.jpg"},
		Album: &services.Album{ID: id, Name: "Album " + id},
	}
}

func albumItems(n int) []services.Item {
	var items []services.Item
	for i := 1; i <= n; i++ {
		items = append(items, albumItem(i))
	}
	return items
}

func videoItem(i int) services.Item {
	id := strconv.Itoa(i)
	return services.Item{
		Kind: services.ItemMusicVideo, ID: id, Name: "Video " + id, Subtitle: "Artist " + id,
		Art:        services.Art{Medium: "/art/kv" + id + "/400x225.jpg"},
		MusicVideo: &services.MusicVideo{ID: id, Name: "Video " + id},
	}
}

// shelfApp is an item shelf on a page that fits three tiles of 160 a row.
func shelfApp(t *testing.T, title string, items []services.Item, o ItemShelfOpts) (*itemsApp, *ui.Tester, *CoverShelfState) {
	t.Helper()
	st := &CoverShelfState{}
	o.First = true
	a, tt := newItemsApp(t, 3*160+2*24+48, 500, nil, func(c *ui.Context, ctx ItemsCtx) {
		ItemShelf(c, st, title, items, ctx, o)
	})
	a.load(t, tt, 0)
	return a, tt, st
}

func TestItemShelfShowsOneRowOfTilesAndTheShowAllToggleOpensTheRest(t *testing.T) {
	_, tt, st := shelfApp(t, "New Releases", albumItems(8), ItemShelfOpts{})
	for _, n := range []string{"Album 1", "Album 2", "Album 3"} {
		if !tt.HasText(n) {
			t.Errorf("%s missing: %q", n, tt.Texts())
		}
	}
	if tt.HasText("Album 4") {
		t.Errorf("a second row shows: %q", tt.Texts())
	}
	if !tt.HasText("New Releases") {
		t.Errorf("no title: %q", tt.Texts())
	}
	if err := tt.Click("Show all 8"); err != nil {
		t.Fatalf("no \"Show all 8\" toggle: %v (%q)", err, tt.Texts())
	}
	tt.Frame()
	if !st.Open || !tt.HasText("Album 8") || !tt.HasText("Album 4") {
		t.Errorf("open %v, texts %q", st.Open, tt.Texts())
	}
	if err := tt.Click("Show fewer"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if st.Open || tt.HasText("Album 4") {
		t.Errorf("open %v after Show fewer, texts %q", st.Open, tt.Texts())
	}
}

func TestItemShelfWithItsOwnPageLinksSeeAllInsteadOfTheToggle(t *testing.T) {
	a, tt, _ := shelfApp(t, "New Releases", albumItems(8), ItemShelfOpts{More: "/room/12"})
	if _, ok := tt.Find("Show all 8"); ok {
		t.Error("a shelf with its own page has the Show all toggle")
	}
	if err := tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.nav, []string{"/room/12"}) {
		t.Errorf("nav %v", a.rec.nav)
	}
}

func TestItemShelfWithRowsShowsThatManyBeforeShowAll(t *testing.T) {
	_, tt, _ := shelfApp(t, "Albums", albumItems(8), ItemShelfOpts{Rows: 2})
	if !tt.HasText("Album 6") || tt.HasText("Album 7") {
		t.Errorf("two rows of three: %q", tt.Texts())
	}
	if _, ok := tt.Find("Show all 8"); !ok {
		t.Error("no toggle for the rest")
	}
	// Few enough to fit: no toggle.
	_, tt, _ = shelfApp(t, "Albums", albumItems(3), ItemShelfOpts{})
	if _, ok := tt.Find("Show all 3"); ok {
		t.Error("a toggle with nothing to show")
	}
}

func TestItemShelfTilesOpenPlayAndLabelTheRow(t *testing.T) {
	a, tt, _ := shelfApp(t, "", albumItems(2), ItemShelfOpts{})
	// An untitled shelf's row falls back to "Items".
	if _, ok := tt.Find("Items"); !ok {
		t.Errorf("the row is not labelled Items: %q", tt.Texts())
	}
	if err := tt.Click("Play Album 2"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.played, []string{"album:2"}) {
		t.Errorf("played %v", a.rec.played)
	}
	r, _ := tt.Find("Artist 1")
	tt.ClickAt(r.X+100, r.Y-60) // on the cover, clear of its play button
	if !slices.Equal(a.rec.nav, []string{"/album/1"}) || !slices.Equal(a.rec.opened, []string{"album:1"}) {
		t.Errorf("nav %v, opened %v", a.rec.nav, a.rec.opened)
	}
}

func TestItemShelfRowAspectIsVideoOnlyWhenEveryItemIsAVideo(t *testing.T) {
	// 6 items in three columns, two rows when open: the y of two subtitles.
	ys := func(items []services.Item, a, b string) (float32, float32) {
		st := &CoverShelfState{Open: true}
		app, tt := newItemsApp(t, 3*160+2*24+48, 600, nil, func(c *ui.Context, ctx ItemsCtx) {
			ItemShelf(c, st, "Shelf", items, ctx, ItemShelfOpts{First: true})
		})
		app.load(t, tt, 0)
		ra, oka := tt.Find(a)
		rb, okb := tt.Find(b)
		if !oka || !okb {
			t.Fatalf("tiles missing: %q", tt.Texts())
		}
		return ra.Y, rb.Y
	}
	var videos, mixed []services.Item
	for i := 1; i <= 6; i++ {
		videos = append(videos, videoItem(i))
		mixed = append(mixed, videoItem(i))
	}
	mixed[5] = albumItem(6)
	const tile = 160
	approx := func(got, want float32) bool { return got > want-0.01 && got < want+0.01 }

	// All videos: rows of 16:9 frames.
	y1, y4 := ys(videos, "Artist 1", "Artist 4")
	if want := CoverRowHeight(tile, true); !approx(y4-y1, want) {
		t.Errorf("video row pitch %v, want %v", y4-y1, want)
	}
	// All albums: square.
	y1, y4 = ys(albumItems(6), "Artist 1", "Artist 4")
	if want := CoverRowHeight(tile, false); !approx(y4-y1, want) {
		t.Errorf("square row pitch %v, want %v", y4-y1, want)
	}
	// One album among videos: the row stays square, so the album's cover is
	// square and its title sits lower than the videos' (the tile's own 16:9
	// frames stay).
	y4, y6 := ys(mixed, "Artist 4", "Artist 6")
	if want := float32(tile - tile*9/16); !approx(y6-y4, want) {
		t.Errorf("album among videos is %v below its row's videos, want %v (a square cover)", y6-y4, want)
	}
}

func TestItemShelfBuildsNothingForNoItems(t *testing.T) {
	_, tt, _ := shelfApp(t, "Empty", nil, ItemShelfOpts{})
	if tt.HasText("Empty") {
		t.Errorf("an empty shelf shows its title: %q", tt.Texts())
	}
}

// gridApp is an item grid in a scroll container, as pages build one.
type gridApp struct {
	*itemsApp
	st     ItemGridState
	pos    ui.ScrollState
	items  []*services.Item
	ranges [][2]int
}

func newGridApp(t *testing.T, items []*services.Item) (*gridApp, *ui.Tester) {
	t.Helper()
	g := &gridApp{items: items}
	var tt *ui.Tester
	g.itemsApp, tt = newItemsApp(t, 3*160+2*24+76, 700, nil, func(c *ui.Context, ctx ItemsCtx) {
		sc := ui.Scroll(c).TrackScroll(&g.pos).Grow(1).Basis(0)
		sc.Children(func() {
			ItemGrid(c, &g.st, len(g.items), func(i int) *services.Item { return g.items[i] }, ctx, ItemGridOpts{
				Label: "Albums", Pane: &Pane{Pos: &g.pos, Bounds: func() ui.Rect { return sc.Bounds() }},
				OnRange: func(s, e int) { g.ranges = append(g.ranges, [2]int{s, e}) },
			})
		})
	})
	return g, tt
}

func TestItemGridShowsLoadingTilesForItemsStillLoading(t *testing.T) {
	var items []*services.Item
	for i := 1; i <= 6; i++ {
		it := albumItem(i)
		if i == 2 || i == 5 {
			items = append(items, nil)
			continue
		}
		items = append(items, &it)
	}
	g, tt := newGridApp(t, items)
	g.load(t, tt, 0)
	for _, n := range []string{"Album 1", "Album 3", "Album 4", "Album 6"} {
		if !tt.HasText(n) {
			t.Errorf("%s missing: %q", n, tt.Texts())
		}
	}
	if tt.HasText("Album 2") || tt.HasText("Album 5") {
		t.Errorf("loading tiles show text: %q", tt.Texts())
	}
	if err := tt.Click("Play Album 3"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(g.rec.played, []string{"album:3"}) {
		t.Errorf("played %v", g.rec.played)
	}
}

func TestItemGridAspectComesFromTheLoadedItems(t *testing.T) {
	pitch := func(items []*services.Item) (got float32, tile float32) {
		g, tt := newGridApp(t, items)
		g.load(t, tt, 0)
		a, _ := tt.Find("Artist 1")
		b, _ := tt.Find("Artist 4")
		_, tile = CoverColumns(g.st.width, 0)
		return b.Y - a.Y, tile
	}
	var videos, loading []*services.Item
	for i := 1; i <= 6; i++ {
		v := videoItem(i)
		videos = append(videos, &v)
		if i == 5 {
			loading = append(loading, nil)
		} else {
			loading = append(loading, &v)
		}
	}
	if got, tile := pitch(videos); got != CoverRowHeight(tile, true) {
		t.Errorf("videos: pitch %v, want %v", got, CoverRowHeight(tile, true))
	}
	// A tile still loading does not make the grid square.
	if got, tile := pitch(loading); got != CoverRowHeight(tile, true) {
		t.Errorf("videos with one loading: pitch %v, want %v", got, CoverRowHeight(tile, true))
	}
	var squares []*services.Item
	for i := 1; i <= 6; i++ {
		a := albumItem(i)
		squares = append(squares, &a)
	}
	if got, tile := pitch(squares); got < CoverRowHeight(tile, false)-0.01 || got > CoverRowHeight(tile, false)+0.01 {
		t.Errorf("albums: pitch %v, want %v", got, CoverRowHeight(tile, false))
	}
	none := make([]*services.Item, 6)
	g, tt := newGridApp(t, none)
	g.load(t, tt, 0) // nothing loaded: square loading tiles, no panic
}

func TestItemGridIsVirtualisedWhenLong(t *testing.T) {
	var items []*services.Item
	for i := 1; i <= 3000; i++ {
		it := albumItem(i)
		items = append(items, &it)
	}
	g, tt := newGridApp(t, items)
	g.load(t, tt, 0)
	built := 0
	for _, s := range tt.Texts() {
		if strings.HasPrefix(s, "Artist ") {
			built++
		}
	}
	if built == 0 || built > 60 {
		t.Errorf("built %d tiles of 3000", built)
	}
	if len(g.ranges) == 0 || g.ranges[0][0] != 0 {
		t.Errorf("OnRange %v", g.ranges)
	}
}

func askedCover(l *ArtLoader, src string) *cover {
	for k, cv := range l.covers {
		if k.src == src {
			return cv
		}
	}
	return nil
}

func TestItemShelfHasTheLoaderDecodeItsNextRowWithoutBuildingIt(t *testing.T) {
	a, tt, _ := shelfApp(t, "New Releases", albumItems(10), ItemShelfOpts{})
	tt.Frame()
	tt.Frame()
	// Three tiles a row: album 1..3 show, 4..6 are what Show all opens first.
	for i := 1; i <= 6; i++ {
		if askedCover(a.art, "/art/ka"+strconv.Itoa(i)+"/400x400.jpg") == nil {
			t.Errorf("the cover of album %d was not asked for", i)
		}
	}
	if askedCover(a.art, "/art/ka7/400x400.jpg") != nil {
		t.Error("the cover of album 7, two rows on, was asked for")
	}
	if cv := askedCover(a.art, "/art/ka5/400x400.jpg"); cv == nil || cv.rank != rankAhead {
		t.Errorf("album 5, in the next row, is not asked ahead: %v", cv)
	}
	if tt.HasText("Album 4") || !tt.HasText("Album 3") {
		t.Errorf("the next row was built, or the first was not: %q", tt.Texts())
	}
}

func TestItemGridHasTheLoaderDecodeTheRowsPastItsBuiltOnesFromTheItemsOnly(t *testing.T) {
	var items []*services.Item
	for i := 1; i <= VirtualFrom+100; i++ {
		it := albumItem(i)
		items = append(items, &it)
	}
	g, tt := newGridApp(t, items)
	g.load(t, tt, 0)
	tt.Frame()
	built := 0
	for i := 1; i <= len(items); i++ {
		if tt.HasText("Artist " + strconv.Itoa(i)) {
			built = i
		}
	}
	// The rows after the last built one are asked ahead, by cover only.
	ahead := askedCover(g.art, "/art/ka"+strconv.Itoa(built+1)+"/400x400.jpg")
	if built == 0 || ahead == nil || ahead.rank != rankAhead {
		t.Fatalf("built up to %d; the next tile's cover is %v", built, ahead)
	}
	if tt.HasText("Artist " + strconv.Itoa(built+1)) {
		t.Error("the tile past the built rows was built")
	}
}
