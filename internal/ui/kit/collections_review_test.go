package kit

import (
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// builtIndices are the numbers of the "<prefix> N" texts the tester shows.
func builtIndices(tt *ui.Tester, prefix string) []int {
	var out []int
	for _, s := range tt.Texts() {
		if n, ok := strings.CutPrefix(s, prefix); ok {
			if i, err := strconv.Atoi(n); err == nil {
				out = append(out, i)
			}
		}
	}
	return out
}

func TestKeyboardFocusScrollsANonVirtualTrackListIntoView(t *testing.T) {
	_, tt := newCollTrackApp(t, 40, 1000, 400)
	tt.Frame()
	for range 4 {
		tt.Key(0, ui.KeyTab)
	}
	tt.Key(0, ui.KeyEnd)
	tt.Frame()
	if !tt.Focused("Song 39") {
		t.Fatal("End did not focus the last row")
	}
	r, ok := tt.Find("Song 39")
	if !ok || r.Y < 0 || r.Y+r.H > 400 {
		t.Errorf("the focused row is at %+v in a 400 DIP window: it was not scrolled into view", r)
	}
}

func TestOneBigScrollJumpBuildsTheRowsThereWithoutAHover(t *testing.T) {
	a, tt := newCollTrackApp(t, 2000, 1000, 600)
	tt.Frame()
	tt.Frame()
	tt.Move(5, 5) // the pointer is nowhere near a row: no hover change rebuilds
	tt.Frame()
	a.pos.Y = 20000
	tt.Frame()
	got := builtIndices(tt, "Artist ")
	if len(got) == 0 {
		t.Fatalf("nothing built after the jump: %q", tt.Texts())
	}
	lo := got[0]
	for _, i := range got {
		lo = min(lo, i)
	}
	// 20000 DIPs down at 44 DIPs a row is row ~450, 12 rows of overscan above.
	if lo < 420 || lo > 455 {
		t.Errorf("the first row built after the jump is %d, want about 440", lo)
	}
}

func TestOneBigScrollJumpBuildsTheTilesThereWithoutAHover(t *testing.T) {
	a, tt := newCoverApp(t, 5000, 1000, 700)
	tt.Frame()
	tt.Frame()
	tt.Move(5, 5)
	tt.Frame()
	a.pos.Y = 9000
	tt.Frame()
	got := builtIndices(tt, "Album ")
	if len(got) == 0 {
		t.Fatalf("nothing built after the jump: %q", tt.Texts())
	}
	lo := got[0]
	for _, i := range got {
		lo = min(lo, i)
	}
	// 5 columns of 240 DIP rows: 9000 DIPs is row ~37, 3 rows of overscan above.
	if lo < 5*30 || lo > 5*38 {
		t.Errorf("the first tile built after the jump is %d, want about %d", lo, 5*34)
	}
}

func TestTheSameAlbumInTwoShelvesDragsFromTheSecond(t *testing.T) {
	resetDrag()
	t.Cleanup(resetDrag)
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	art := newArtLoader(&fakeArt{}, newMainThread().post, 2)
	t.Cleanup(art.Close)
	var first, second CoverState
	var dropped []*DragPayload
	build := func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Fill().Padding(24).Children(func() {
			for n, st := range []*CoverState{&first, &second} {
				at := func(i int) *CoverItem {
					id := strconv.Itoa(i)
					it := AlbumItem(services.Album{ID: id, Name: "Album " + id})
					// The same key in both shelves.
					return &CoverItem{Key: "a" + id, Title: "Album " + id, Subtitle: "Shelf" + strconv.Itoa(n) + " " + id, Item: &it, Open: func() {}}
				}
				ui.Column(c.Key("shelf" + strconv.Itoa(n))).Children(func() {
					CoverGrid(c, st, 3, at, CoverGridOpts{CoverOpts: CoverOpts{Label: "Shelf", Art: art}})
				})
			}
			box := ui.Box(c.Key("drop")).Width(200).Height(60)
			DropOn(box, "queue", DropTarget{
				Describe: func(*DragPayload) string { return "Add to queue" },
				Drop:     func(p *DragPayload, _ DragPoint) { dropped = append(dropped, p) },
			})
			box.Children(func() { ui.Text(c, "Queue target") })
		})
	}
	tt := ui.NewTester(build, 900, 900)
	tt.Frame()
	x, y := collPos(t, tt, "Shelf1 1")
	y -= 100
	tx, ty := collPos(t, tt, "Queue target")
	tt.Press(x, y)
	tt.Move(x+10, y)
	tt.Move(x+20, y)
	tt.Frame()
	if !DragActive(nil) {
		t.Fatal("the drag from the second shelf did not start (or ended at once)")
	}
	tt.Move(tx, ty)
	tt.Move(tx+1, ty)
	tt.Release(tx+1, ty)
	if len(dropped) != 1 {
		t.Fatalf("dropped %d times", len(dropped))
	}
}

func TestTheDraggedRowOutOfViewReportsOnlyTheWindow(t *testing.T) {
	a, tt := newCollTrackApp(t, 2000, 1000, 600)
	var ranges [][2]int
	a.opts.OnRange = func(s, e int) { ranges = append(ranges, [2]int{s, e}) }
	tt.Frame()
	tt.Frame()
	x, y := collRowPos(t, tt, "Song 3")
	tt.Press(x, y)
	tt.Move(x, y+10)
	tt.Move(x, y+20)
	tt.Frame()
	if !DragActive(nil) {
		t.Fatal("no drag")
	}
	a.pos.Y = 20000
	tt.Frame()
	tt.Frame()
	for _, r := range ranges {
		if r[0] > r[1] {
			t.Errorf("OnRange %v is inverted", r)
		}
	}
	if last := ranges[len(ranges)-1]; last[0] < 300 {
		t.Errorf("OnRange %v includes the dragged row far above the window", last)
	}
}

func TestTheDraggedTileOutOfViewReportsOnlyTheWindow(t *testing.T) {
	d, tt := newDropApp(t)
	d.n = 5000
	tt.Frame()
	d.ranges = nil
	x, y := collPos(t, tt, "Artist 2")
	y -= 100
	tt.Press(x, y)
	tt.Move(x+10, y)
	tt.Move(x+20, y)
	tt.Frame()
	d.pos.Y = 9000
	tt.Frame()
	tt.Frame()
	if len(d.ranges) == 0 {
		t.Fatal("no OnRange")
	}
	if last := d.ranges[len(d.ranges)-1]; last[0] < 100 {
		t.Errorf("OnRange %v includes the dragged tile far above the window", last)
	}
}

func TestModifiedClicksOnATrackLinkSelectTheRowAndPlainOnesOpenIt(t *testing.T) {
	a, tt := newCollTrackApp(t, 6, 1000, 500)
	tt.Frame()
	ax, ay := collPos(t, tt, "Artist 4")
	tt.ClickAtWith(ui.Ctrl, ax, ay)
	if len(a.artists) != 0 {
		t.Errorf("Ctrl+click on the artist link opened %v", a.artists)
	}
	if !a.st.Sel.Selected("s4") {
		t.Errorf("Ctrl+click on the artist link selected %v", a.st.Sel.State().Keys())
	}
	bx, by := collPos(t, tt, "Artist 1")
	tt.ClickAtWith(ui.Shift, bx, by)
	if len(a.artists) != 0 {
		t.Errorf("Shift+click on the artist link opened %v", a.artists)
	}
	if a.st.Sel.Len() < 2 {
		t.Errorf("Shift+click on the artist link selected %v", a.st.Sel.State().Keys())
	}
	a.st.Sel.Reset()
	tt.ClickAt(ax, ay)
	if len(a.artists) != 1 || a.artists[0] != "ar4" {
		t.Errorf("a plain click on the link opened %v", a.artists)
	}
}

func TestAnEmptyMultiDiscTrackListDoesNotPanic(t *testing.T) {
	a, tt := newCollTrackApp(t, 0, 1000, 500)
	a.opts.Discs = true
	tt.Frame()
	tt.Frame()
}

func TestEachShelfOfASectionsPageHasItsOwnBulkMenu(t *testing.T) {
	var ran [][]string
	reg := NewActionRegistry(ActionDef{
		ID: "test.one", Target: KindAlbum, Group: GroupItem, Order: 10,
		Label: func(*ActionContext) string { return "Go to album" },
		Run:   func(*ActionContext) {},
	}, ActionDef{
		ID: "test.many", Target: KindSelection, Kinds: []ItemKind{KindAlbum}, Group: GroupQueue, Order: 10,
		Label: func(*ActionContext) string { return "Queue all" },
		Run: func(x *ActionContext) {
			var names []string
			for _, it := range x.Items {
				names = append(names, it.Name())
			}
			ran = append(ran, names)
		},
	})
	sections := []services.Section{
		sectionOf("a", services.SectionShelf, "First", albumItems(3)...),
		sectionOf("b", services.SectionShelf, "Second", albumItem(11), albumItem(12), albumItem(13)),
	}
	st := &EditorialState{}
	a, tt := newItemsApp(t, 900, 1400, nil, func(c *ui.Context, ctx ItemsCtx) {
		EditorialSections(c, st, sections, ctx, EditorialOpts{First: true})
	})
	a.menus = NewActionMenus(reg, nil) // the page's menu knows no selection
	a.load(t, tt, 0)
	x1, y1 := collPos(t, tt, "Album 11")
	x2, y2 := collPos(t, tt, "Album 12")
	tt.ClickAtWith(ui.Ctrl, x1, y1-100)
	tt.ClickAtWith(ui.Ctrl, x2, y2-100)
	tt.RightClickAt(x2, y2-100)
	tt.Frame()
	if !tt.HasText("Queue all") {
		t.Fatalf("no bulk menu for the second shelf's selection: %q sel %v", tt.Texts(), st.shelves["b"].Row.Sel.State().Keys())
	}
	if err := tt.Click("Queue all"); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || len(ran[0]) != 2 {
		t.Errorf("the bulk action ran over %v, want the two selected albums", ran)
	}
}

func TestTheHighlightedSongIsScrolledToAfterTheListIsLaidOut(t *testing.T) {
	// Content above the list, and the highlight there from the first frame:
	// the list's offset is only known once it is laid out.
	a, tt := newCollTrackApp(t, 2000, 1000, 400, func(a *collTrackApp) {
		a.header = 300
		a.opts.Highlight = "s900"
	})
	tt.Frame()
	tt.Frame()
	tt.Frame()
	tt.Frame()
	r, ok := tt.Find("Song 900")
	if !ok {
		t.Fatalf("the highlighted song was not built (scroll %v)", a.pos.Y)
	}
	if r.Y < 100 || r.Y+r.H > 300 {
		t.Errorf("the highlighted row is at %v in a 400 DIP window, want about the centre (pos %v top %v/%v)", r.Y, a.pos.Y, a.st.top, a.st.drawTop)
	}
}
