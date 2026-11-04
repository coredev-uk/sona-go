package kit

import (
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// shelfApp is a page with one cover shelf.
type shelfApp struct {
	items  []CoverItem
	st     CoverShelfState
	opts   CoverShelfOpts
	art    *ArtLoader
	more   int
	opened []string
	title  string
}

func newShelfApp(t *testing.T, n int, w int) (*shelfApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	resetDrag()
	t.Cleanup(resetDrag)
	mt := newMainThread()
	a := &shelfApp{title: "Recently Added"}
	a.art = newArtLoader(&fakeArt{}, mt.post, 2)
	t.Cleanup(a.art.Close)
	for i := range n {
		id := strconv.Itoa(i)
		it := AlbumItem(services.Album{ID: id, Name: "Album " + id})
		a.items = append(a.items, CoverItem{Key: "a" + id, Title: "Album " + id, Subtitle: "Artist " + id, Art: "/art/k" + id + "/x.jpg",
			Item: &it, Open: func() { a.opened = append(a.opened, id) }})
	}
	return a, ui.NewTester(a.view, w, 700)
}

func (a *shelfApp) view(c *ui.Context) {
	theme.Apply(c)
	a.opts.Art = a.art
	ui.Column(c).Padding(24, 32).Children(func() {
		CoverShelf(c, &a.st, a.title, a.items, a.opts)
	})
}

func artists(tt *ui.Tester) int {
	n := 0
	for _, s := range tt.Texts() {
		if strings.HasPrefix(s, "Artist ") {
			n++
		}
	}
	return n
}

func TestCoverShelfShowsOneRowAndTogglesInPlace(t *testing.T) {
	_, tt := newShelfApp(t, 12, 1000)
	tt.Frame()
	tt.Frame()
	// 1000-64 wide: 5 columns of 160+ fit.
	if got := artists(tt); got != 5 {
		t.Fatalf("shown %d tiles, want one row of 5", got)
	}
	if !tt.HasText("Recently Added") || !tt.HasText("Show all") || tt.HasText("See all") {
		t.Errorf("heading controls: %q", tt.Texts())
	}
	if err := tt.Click("Show all"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := artists(tt); got != 12 {
		t.Errorf("Show all shows %d tiles, want 12", got)
	}
	if !tt.HasText("Show fewer") {
		t.Errorf("no Show fewer: %q", tt.Texts())
	}
	_ = tt.Click("Show fewer")
	tt.Frame()
	if got := artists(tt); got != 5 {
		t.Errorf("Show fewer shows %d tiles", got)
	}
}

func TestCoverShelfSeeAllReplacesShowAll(t *testing.T) {
	a, tt := newShelfApp(t, 12, 1000)
	a.opts.More = func() { a.more++ }
	tt.Frame()
	tt.Frame()
	if tt.HasText("Show all") {
		t.Error("a shelf with a See all link also shows Show all")
	}
	if err := tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	if a.more != 1 {
		t.Errorf("See all ran %d times", a.more)
	}
}

func TestCoverShelfShowsNothingForNoItemsAndNoToggleWhenAllFit(t *testing.T) {
	_, tt := newShelfApp(t, 0, 1000)
	tt.Frame()
	if tt.HasText("Recently Added") {
		t.Error("an empty shelf shows its title")
	}
	_, tt2 := newShelfApp(t, 4, 1000)
	tt2.Frame()
	tt2.Frame()
	if tt2.HasText("Show all") {
		t.Error("4 tiles in a row of 5 offer Show all")
	}
	if got := artists(tt2); got != 4 {
		t.Errorf("shown %d", got)
	}
}

func TestCoverRowFitsTheWidthAndReportsColumns(t *testing.T) {
	a, tt := newShelfApp(t, 12, 1000)
	tt.Frame()
	tt.Frame()
	if a.st.cols != 5 {
		t.Fatalf("columns %d", a.st.cols)
	}
	tt.SetSize(520, 700)
	tt.Frame()
	tt.Frame()
	// 520-64 = 456: 2 columns.
	if a.st.cols != 2 || artists(tt) != 2 {
		t.Errorf("columns %d, tiles %d, want 2", a.st.cols, artists(tt))
	}
}

func TestCoverShelfVideoUsesWiderTiles(t *testing.T) {
	a, tt := newShelfApp(t, 6, 1000)
	a.opts.Video = true
	tt.Frame()
	tt.Frame()
	// min 240: 1000-64 = 936 → 3 columns.
	if a.st.cols != 3 {
		t.Errorf("video columns %d, want 3", a.st.cols)
	}
}

func TestCoverTilesNameTheirControls(t *testing.T) {
	a, tt := newShelfApp(t, 3, 1000)
	a.items[0].Badge = "Out 19 Nov"
	a.items[1].PlaysOnOpen = true
	a.items[1].Open = nil
	a.items[2].Explicit = true
	a.items[2].Play = &CoverPlay{Label: "Album 2", Run: func() {}}
	a.opts.Restricted = true
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"Album 0, Out 19 Nov", "Album 1 Play"} {
		if _, ok := tt.Find(want); !ok {
			t.Errorf("no control named %q in %q", want, tt.Texts())
		}
	}
	// A restricted account gets no play button on an explicit tile.
	if _, ok := tt.Find("Play Album 2"); ok {
		t.Error("a withheld tile has a play button")
	}
}
