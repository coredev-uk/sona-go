package kit

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// coverApp is a page with a cover grid in its scroll container, the shape the
// library page has: the tests drive it.
type coverApp struct {
	n      int
	items  func(i int) *CoverItem
	st     CoverState
	pos    ui.ScrollState
	scroll ui.Element
	art    *ArtLoader
	opts   CoverGridOpts
	target func(c *ui.Context) // a drop target built beside the page
	opened []string
	played []string
	ranges [][2]int
	header float32
}

func newCoverApp(t *testing.T, n int, w, h int) (*coverApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	resetDrag()
	t.Cleanup(resetDrag)
	mt := newMainThread()
	a := &coverApp{n: n}
	a.art = newArtLoader(&fakeArt{}, mt.post, 2)
	t.Cleanup(a.art.Close)
	a.items = func(i int) *CoverItem {
		id := strconv.Itoa(i)
		it := AlbumItem(services.Album{ID: id, Name: "Album " + id})
		return &CoverItem{
			Key: "a" + id, Title: "Album " + id, Subtitle: "Artist " + id, Art: "/art/k" + id + "/x.jpg",
			Item: &it,
			Open: func() { a.opened = append(a.opened, id) },
			Play: &CoverPlay{Label: "Album " + id, Run: func() { a.played = append(a.played, id) }},
		}
	}
	a.opts = CoverGridOpts{CoverOpts: CoverOpts{Label: "Albums"}, OnRange: func(s, e int) { a.ranges = append(a.ranges, [2]int{s, e}) }}
	return a, ui.NewTester(a.view, w, h)
}

func (a *coverApp) view(c *ui.Context) {
	theme.Apply(c)
	a.opts.Art = a.art
	ui.Row(c).Fill().Children(func() {
		sc := ui.Scroll(c).TrackScroll(&a.pos).Grow(1).Basis(0).MinWidth(0).FillHeight().Padding(0, 44, 0, 32)
		a.scroll = sc
		sc.Children(func() {
			ui.Column(c).Padding(28, 0, 48, 0).Children(func() {
				ui.Box(c.Key("header")).Height(a.header)
				a.opts.Pane = &Pane{Pos: &a.pos, Bounds: func() ui.Rect { return sc.Bounds() }}
				CoverGrid(c, &a.st, a.n, a.items, a.opts)
			})
		})
		if a.target != nil {
			a.target(c)
		}
	})
}

func collPos(t *testing.T, tt *ui.Tester, label string) (x, y float32) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("no %q in %q", label, tt.Texts())
	}
	return r.X + r.W/2, r.Y + r.H/2
}

func TestCoverColumns(t *testing.T) {
	for _, c := range []struct {
		w, min float32
		cols   int
		tile   float32
	}{
		{0, 160, 1, 160},
		{100, 160, 1, 100},
		{160, 160, 1, 160},
		{343, 160, 1, 343},
		{344, 160, 2, 160},
		{1000, 160, 5, 180.8},
		{1000, 240, 3, 317.333},
		{1000, 0, 5, 180.8},
	} {
		cols, tile := CoverColumns(c.w, c.min)
		if cols != c.cols || tile < c.tile-0.01 || tile > c.tile+0.01 {
			t.Errorf("CoverColumns(%v, %v) = %d, %.3f; want %d, %.3f", c.w, c.min, cols, tile, c.cols, c.tile)
		}
	}
}

func TestCoverGridBuildsOnlyTheRowsShowing(t *testing.T) {
	a, tt := newCoverApp(t, 5000, 1000, 700)
	tt.Frame()
	tt.Frame()
	built := func() int {
		n := 0
		for _, s := range tt.Texts() {
			if strings.HasPrefix(s, "Artist ") {
				n++
			}
		}
		return n
	}
	// 5 columns; rows are 240 tall: 3 showing, plus 3 beyond.
	if got := built(); got != 5*6 {
		t.Errorf("built %d tiles of 5000", got)
	}
	if len(a.ranges) == 0 || a.ranges[0][0] != 0 {
		t.Fatalf("OnRange %v", a.ranges)
	}
	first := a.ranges[len(a.ranges)-1]
	tt.Scroll(500, 300, 0, 9000)
	tt.Frame()
	last := a.ranges[len(a.ranges)-1]
	if last[0] <= first[0] || last[1] <= first[1] {
		t.Errorf("range after scrolling %v, before %v", last, first)
	}
	if got := built(); got > 5*10 {
		t.Errorf("built %d tiles after scrolling", got)
	}
}

func TestCoverGridClicksOpenAndModifiedClicksSelect(t *testing.T) {
	a, tt := newCoverApp(t, 12, 1000, 700)
	tt.Frame()
	x, y := collPos(t, tt, "Artist 3")
	tt.ClickAt(x, y-100)
	if len(a.opened) != 1 || a.opened[0] != "3" {
		t.Fatalf("opened %v", a.opened)
	}
	tt.ClickAtWith(ui.Cmd, x, y-100)
	if len(a.opened) != 1 || !a.st.Sel.Selected("a3") {
		t.Errorf("ctrl+click: opened %v, selected %v", a.opened, a.st.Sel.State().Keys())
	}
	x5, y5 := collPos(t, tt, "Artist 5")
	tt.ClickAtWith(ui.Shift, x5, y5-30)
	if got := a.st.Sel.State().Keys(); len(got) != 3 {
		t.Errorf("shift+click selected %v", got)
	}
	if len(a.opened) != 1 {
		t.Errorf("a modified click opened a tile: %v", a.opened)
	}
	// The play button runs without opening.
	if err := tt.Click("Play Album 7"); err != nil {
		t.Fatal(err)
	}
	if len(a.played) != 1 || a.played[0] != "7" || len(a.opened) != 1 {
		t.Errorf("played %v opened %v", a.played, a.opened)
	}
}

func TestCoverGridArrowsMoveFocusAndShiftExtendsTheSelection(t *testing.T) {
	a, tt := newCoverApp(t, 12, 1000, 700)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Album 0") {
		t.Fatalf("Tab did not focus the first tile")
	}
	tt.Key(0, ui.KeyRight)
	if !tt.Focused("Album 1") {
		t.Errorf("Right did not move to tile 1")
	}
	tt.Key(0, ui.KeyDown)
	if !tt.Focused("Album 6") {
		t.Errorf("Down did not move a row (5 columns)")
	}
	tt.Key(0, ui.KeyLeft)
	tt.Key(ui.Shift, ui.KeyLeft)
	if got := a.st.Sel.State().Keys(); len(got) != 2 {
		t.Errorf("Shift+Left selected %v", got)
	}
	tt.Key(ui.Shift, ui.KeyRight)
	tt.Key(ui.Shift, ui.KeyRight)
	if got := a.st.Sel.State().Keys(); len(got) != 2 || !a.st.Sel.Selected("a5") || !a.st.Sel.Selected("a6") {
		t.Errorf("Shift+Right back past the anchor selected %v", got)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	if got := a.st.Sel.Len(); got != 12 {
		t.Errorf("Ctrl+A selected %d", got)
	}
	tt.Key(0, ui.KeyEscape)
	if a.st.Sel.Len() != 0 {
		t.Errorf("Escape left %d selected", a.st.Sel.Len())
	}
	tt.Key(0, ui.KeyEnter)
	if len(a.opened) != 1 {
		t.Errorf("Enter on a focused tile opened %v", a.opened)
	}
}

// dropApp adds a sidebar of drop targets to the grid page.
type dropApp struct {
	*coverApp
	dropped  []*DragPayload
	hover    []string
	over     bool
	accepts  bool
	describe string
}

func newDropApp(t *testing.T) (*dropApp, *ui.Tester) {
	t.Helper()
	a, tt := newCoverApp(t, 12, 1200, 700)
	d := &dropApp{coverApp: a, accepts: true, describe: "Add to “Road trip”"}
	a.target = func(c *ui.Context) {
		box := ui.Box(c.Key("drop")).Width(200).FillHeight().Padding(12)
		d.over = DropOn(box, "playlist", DropTarget{
			Accepts:  func(p *DragPayload) bool { return d.accepts },
			Describe: func(p *DragPayload) string { return d.describe },
			Hover: func(p *DragPayload, at *DragPoint) {
				if at == nil {
					d.hover = append(d.hover, "left")
				} else if len(d.hover) == 0 || d.hover[len(d.hover)-1] != "over" {
					d.hover = append(d.hover, "over")
				}
			},
			Drop: func(p *DragPayload, at DragPoint) { d.dropped = append(d.dropped, p) },
		})
		box.Children(func() { ui.Text(c, "Road trip") })
	}
	tt.Frame()
	return d, tt
}

func TestDragStartsAfterSixDIPsAndAPressBelowIsAClick(t *testing.T) {
	d, tt := newDropApp(t)
	x, y := collPos(t, tt, "Artist 2")
	y -= 100
	// A press that travels 5 DIPs is a click.
	tt.Press(x, y)
	tt.Move(x+5, y)
	tt.Release(x+5, y)
	if len(d.opened) != 1 {
		t.Fatalf("a 5 DIP press opened %v, want a click", d.opened)
	}
	// 8 DIPs starts a drag, which opens nothing.
	tt.Press(x, y)
	tt.Move(x+4, y)
	tt.Move(x+8, y)
	tt.Move(x+12, y)
	if !DragActive(nil) {
		t.Fatal("no drag after 12 DIPs")
	}
	tt.Release(x+12, y)
	if len(d.opened) != 1 {
		t.Errorf("a drag opened %v", d.opened)
	}
	if DragActive(nil) {
		t.Error("the drag is still active after the release")
	}
}

func TestDragDropsTheTileOnATargetAndTheGhostSaysWhatItDoes(t *testing.T) {
	d, tt := newDropApp(t)
	x, y := collPos(t, tt, "Artist 2")
	y -= 100
	tx, ty := collPos(t, tt, "Road trip")
	tt.Press(x, y)
	tt.Move(x+10, y)
	tt.Move(tx, ty)
	tt.Move(tx+1, ty+1)
	if !d.over {
		t.Error("the target does not know the drag is over it")
	}
	if !tt.HasText("Add to “Road trip”") || !tt.HasText("Album 2") {
		t.Errorf("ghost texts missing: %q", tt.Texts())
	}
	tt.Release(tx+1, ty+1)
	if len(d.dropped) != 1 {
		t.Fatalf("dropped %d times", len(d.dropped))
	}
	p := d.dropped[0]
	if len(p.Items) != 1 || p.Items[0].Name() != "Album 2" || p.Label != "Album 2" {
		t.Errorf("payload %+v", p)
	}
	if got := d.hover; len(got) < 2 || got[0] != "over" || got[len(got)-1] != "left" {
		t.Errorf("hover calls %v", got)
	}
	tt.Frame()
	if tt.HasText("Add to “Road trip”") {
		t.Error("the ghost stayed after the drop")
	}
}

func TestDropIndex(t *testing.T) {
	// Dragging row 1 into gap 3 (between rows 2 and 3) lands it at index 2.
	for _, c := range [][3]int{{1, 3, 2}, {3, 0, 0}, {2, 2, 2}} {
		if got := DropIndex(c[0], c[1]); got != c[2] {
			t.Errorf("DropIndex(%d, %d) = %d, want %d", c[0], c[1], got, c[2])
		}
	}
}

func TestDragNeverStartsFromThePlayButton(t *testing.T) {
	d, tt := newDropApp(t)
	r, _ := tt.Find("Play Album 2")
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Move(x, y)
	tt.Frame()
	tt.Press(x, y)
	tt.Move(x+20, y)
	tt.Move(x+40, y)
	if DragActive(nil) {
		t.Error("a press on the play button started a drag")
	}
	tt.Release(x+40, y)
	_ = d
}

func TestDropFallsBackToTheTargetAroundADecliningOne(t *testing.T) {
	a, tt := newCoverApp(t, 12, 1200, 700)
	var got []string
	a.target = func(c *ui.Context) {
		outer := ui.Column(c.Key("outer")).Width(300).FillHeight().Padding(40)
		DropOn(outer, "queue", DropTarget{
			Describe: func(p *DragPayload) string { return "Add to queue" },
			Drop:     func(p *DragPayload, at DragPoint) { got = append(got, "queue") },
		})
		outer.Children(func() {
			inner := ui.Box(c.Key("inner")).Size(200, 100)
			DropOn(inner, "playlist", DropTarget{
				Accepts: func(p *DragPayload) bool { return false },
				Drop:    func(p *DragPayload, at DragPoint) { got = append(got, "playlist") },
			})
			inner.Children(func() { ui.Text(c, "Declines") })
		})
	}
	tt.Frame()
	tt.Frame()
	x, y := collPos(t, tt, "Artist 2")
	y -= 100
	ix, iy := collPos(t, tt, "Declines")
	tt.Press(x, y)
	tt.Move(x+10, y)
	tt.Move(x+20, y)
	tt.Move(ix, iy)
	tt.Move(ix+1, iy)
	if !tt.HasText("Add to queue") {
		t.Errorf("the caption does not come from the outer target: %q", tt.Texts())
	}
	tt.Release(ix+1, iy)
	if len(got) != 1 || got[0] != "queue" {
		t.Errorf("drop went to %v, want the outer target", got)
	}
}

func TestTheDraggedTileStaysBuiltWhileEdgeScrollingMovesItOutOfView(t *testing.T) {
	a, tt := newCoverApp(t, 5000, 1000, 700)
	tt.Frame()
	x, y := collPos(t, tt, "Artist 7")
	y -= 100
	tt.Press(x, y)
	tt.Move(x+10, y)
	tt.Move(x+20, y)
	if !DragActive(nil) {
		t.Fatal("no drag")
	}
	// Hold the pointer at the pane's bottom edge: it scrolls.
	for range 40 {
		tt.Move(x+20, 695)
		tt.Advance(100 * time.Millisecond)
	}
	if a.pos.Y < 3000 {
		t.Fatalf("the pane did not scroll under the drag: %v", a.pos.Y)
	}
	if !DragActive(nil) {
		t.Error("the drag ended while scrolling")
	}
	if !tt.HasText("Artist 7") {
		t.Error("the dragged tile was dropped from the build when it left the view")
	}
	tt.Release(x+20, 695)
}

func TestTheSameTilesStayAtTheTopWhenTheColumnsChange(t *testing.T) {
	a, tt := newCoverApp(t, 5000, 1000, 700)
	tt.Frame()
	if a.st.Columns() != 5 {
		t.Fatalf("columns %d", a.st.Columns())
	}
	tt.Scroll(400, 300, 0, 4000)
	tt.Frame()
	// The first tile at the top, before.
	top := func() int {
		return int((a.pos.Y-a.st.top)/a.st.rowH) * a.st.cols
	}
	before := top()
	tt.SetSize(700, 700)
	tt.Frame()
	tt.Frame()
	if a.st.Columns() >= 5 {
		t.Fatalf("columns after narrowing: %d", a.st.Columns())
	}
	after := top()
	if d := after - before; d < -a.st.cols || d > a.st.cols {
		t.Errorf("the top tile moved from %d to %d", before, after)
	}
}

func TestCoverGridLoadingTilesAreFlatAndNotSelectable(t *testing.T) {
	a, tt := newCoverApp(t, 10, 1000, 700)
	inner := a.items
	a.items = func(i int) *CoverItem {
		if i%2 == 1 {
			return nil
		}
		return inner(i)
	}
	tt.Frame()
	if tt.HasText("Album 1") {
		t.Error("a loading tile shows text")
	}
	a.st.Sel.Click(1, ClickMods{})
	if a.st.Sel.Len() != 0 {
		t.Error("selected a loading tile")
	}
}

// menuApp gives the cover page an item menu with one action.
func newCoverMenuApp(t *testing.T) (*coverApp, *ui.Tester, *[]string) {
	t.Helper()
	a, tt := newCoverApp(t, 12, 1000, 700)
	var ran []string
	reg := NewActionRegistry(ActionDef{
		ID: "test.open", Target: KindAlbum, Group: GroupItem, Order: 10,
		Label: func(*ActionContext) string { return "Go to album" },
		Run:   func(x *ActionContext) { ran = append(ran, x.Item().Name()) },
	})
	a.opts.Menus = NewActionMenus(reg, &a.st.Sel)
	return a, tt, &ran
}

func TestRightClickOpensTheTileMenuAndSelectsTheTileOutsideTheSelection(t *testing.T) {
	a, tt, ran := newCoverMenuApp(t)
	tt.Frame()
	a.st.Sel.Click(0, ClickMods{})
	x, y := collPos(t, tt, "Artist 3")
	tt.RightClickAt(x, y-100)
	tt.Frame()
	if !tt.HasText("Go to album") {
		t.Fatalf("no menu: %q", tt.Texts())
	}
	if a.st.Sel.Len() != 1 || !a.st.Sel.Selected("a3") {
		t.Errorf("the menu's tile was not selected alone: %v", a.st.Sel.State().Keys())
	}
	if err := tt.Click("Go to album"); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 1 || (*ran)[0] != "Album 3" {
		t.Errorf("ran %v", *ran)
	}
	if len(a.opened) != 0 {
		t.Errorf("the right-click opened %v", a.opened)
	}
}

func TestMenuOnASelectedTileKeepsTheSelectionAndIsHeldOpen(t *testing.T) {
	a, tt, _ := newCoverMenuApp(t)
	tt.Frame()
	x2, y2 := collPos(t, tt, "Artist 2")
	x3, y3 := collPos(t, tt, "Artist 3")
	tt.ClickAtWith(ui.Cmd, x2, y2-100)
	tt.ClickAtWith(ui.Cmd, x3, y3-100)
	tt.RightClickAt(x3, y3-100)
	tt.Frame()
	if a.st.Sel.Len() != 2 {
		t.Errorf("the menu changed the selection: %v", a.st.Sel.State().Keys())
	}
	// A press on the menu is not a press outside the list.
	if a.st.Sel.Hold == nil || !a.st.Sel.Hold() {
		t.Error("the selection is not held while the menu is open")
	}
}

func TestShiftF10OpensTheFocusedTilesMenu(t *testing.T) {
	a, tt, _ := newCoverMenuApp(t)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyRight)
	tt.Key(ui.Shift, ui.KeyF10)
	tt.Frame()
	if !tt.HasText("Go to album") {
		t.Fatalf("Shift+F10 opened no menu: %q", tt.Texts())
	}
	if !a.st.Sel.Selected("a1") || a.st.Sel.Len() != 1 {
		t.Errorf("the focused tile was not selected for its menu: %v", a.st.Sel.State().Keys())
	}
}

func TestEscapeCancelsTheDragWithoutDroppingOrOpening(t *testing.T) {
	d, tt := newDropApp(t)
	x, y := collPos(t, tt, "Artist 2")
	y -= 100
	tx, ty := collPos(t, tt, "Road trip")
	tt.Press(x, y)
	tt.Move(x+10, y)
	tt.Move(x+20, y)
	tt.Move(tx, ty)
	tt.Move(tx+1, ty)
	tt.Key(0, ui.KeyEscape)
	tt.Release(tx+1, ty)
	tt.Frame()
	if len(d.dropped) != 0 || len(d.opened) != 0 {
		t.Errorf("dropped %d, opened %v", len(d.dropped), d.opened)
	}
	if DragActive(nil) {
		t.Error("the drag is still active after Escape")
	}
	if tt.HasText("Add to “Road trip”") {
		t.Error("the ghost stayed after Escape")
	}
}

func TestATileWithoutAnItemDoesNotDrag(t *testing.T) {
	a, tt := newCoverApp(t, 6, 1000, 700)
	inner := a.items
	a.items = func(i int) *CoverItem {
		it := inner(i)
		it.Item = nil // a music video: no target
		return it
	}
	tt.Frame()
	x, y := collPos(t, tt, "Artist 2")
	y -= 100
	tt.Press(x, y)
	tt.Move(x+10, y)
	tt.Move(x+20, y)
	if DragActive(nil) {
		t.Error("a tile with no target started a drag")
	}
	tt.Release(x+20, y)
	a.st.Sel.Click(2, ClickMods{})
	if a.st.Sel.Len() != 0 {
		t.Error("a tile with no target was selected")
	}
}

func TestTheGhostSitsRightAndBelowThePointerAndFlipsNearTheEdge(t *testing.T) {
	for _, c := range []struct{ x, y, w, ww, left, top float32 }{
		{100, 50, 160, 1000, 114, 60},
		{800, 50, 160, 1000, 814, 60},
		{826, 50, 160, 1000, 840, 60}, // ends exactly at the edge: stays
		{830, 50, 160, 1000, 656, 60}, // would pass the edge: left of the pointer
	} {
		left, top := ghostPlace(c.x, c.y, c.w, c.ww)
		if left != c.left || top != c.top {
			t.Errorf("ghostPlace(%v,%v,%v,%v) = %v,%v want %v,%v", c.x, c.y, c.w, c.ww, left, top, c.left, c.top)
		}
	}
}

// A tile's play button and more button are elements of their own beside the
// opener (a button or a link is a leaf on the accessibility tree, so inside
// it they would be unreachable there): they exist without hover and the keyboard
// reaches the play button after the opener, Enter pressing it (the more button
// opens with Shift+F10 on the tile, as before).
func TestCoverTileControlsAreBesideTheOpenerAndTabbable(t *testing.T) {
	a, tt, _ := newCoverMenuApp(t)
	tt.Frame()
	for _, label := range []string{"Play Album 0", "More for Album 0"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("%q is not there while the tile is not hovered", label)
		}
	}
	tt.Key(0, ui.KeyTab)
	for _, want := range []string{"Album 0", "Play Album 0", "Album 1"} {
		if !tt.Focused(want) {
			t.Fatalf("Tab did not reach %q", want)
		}
		tt.Key(0, ui.KeyTab)
	}
	// Back on the play button: Enter presses it.
	tt.Key(ui.Shift, ui.KeyTab)
	tt.Key(ui.Shift, ui.KeyTab)
	if !tt.Focused("Play Album 0") {
		t.Fatalf("Shift+Tab did not return to the play button")
	}
	tt.Key(0, ui.KeyEnter)
	if len(a.played) != 1 || a.played[0] != "0" || len(a.opened) != 0 {
		t.Errorf("played %v opened %v", a.played, a.opened)
	}
}
