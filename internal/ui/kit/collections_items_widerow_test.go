package kit

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// wideItem is an album card with a banner and/or a cover on #102030, as the
// React test builds them.
func wideItem(id, wide, cover string) services.Item {
	return services.Item{
		Kind: services.ItemAlbum, ID: id, Name: "Item " + id, Color: "#102030",
		Album: &services.Album{ID: id, Name: "Item " + id},
		Art:   services.Art{Medium: cover, Large: cover},
		Wide:  services.Art{Medium: wide, Large: wide},
	}
}

// countText counts the texts and labels equal to s.
func countText(tt *ui.Tester, s string) int {
	n := 0
	for _, x := range tt.Texts() {
		if x == s {
			n++
		}
	}
	return n
}

func artPath(key string, w, h int) string { return fmt.Sprintf("/art/k%s/%dx%d.jpg", key, w, h) }

var (
	itemColour = color.RGBA{0x10, 0x20, 0x30, 255}
	fakeRed    = color.RGBA{220, 30, 30, 255}
)

// isFrame matches the pixels of an artwork frame: the item colour and the
// fake art's red, closely, so the text's anti-aliasing is not taken for it.
func isFrame(p color.RGBA) bool { return closeTo(p, itemColour) || closeTo(p, fakeRed) }

func closeTo(a, b color.RGBA) bool {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return d(a.R, b.R) <= 2 && d(a.G, b.G) <= 2 && d(a.B, b.B) <= 2
}

// frameOf is the artwork frame of the first card in the picture: the box of
// the item colour and the fake art's red.
func frameOf(tt *ui.Tester) (w, h int) {
	r := box(tt.Image(), isFrame)
	return r.Dx(), r.Dy()
}

// wideApp is a one-column wide row: the page is 240 DIPs wide at the row's
// minimum, so cards are 240 wide.
func wideApp(t *testing.T, fake *fakeArt, items []services.Item, ratio float32, crop bool) (*itemsApp, *ui.Tester, *WideRowState) {
	t.Helper()
	st := &WideRowState{}
	var src artSource
	if fake != nil {
		src = fake
	}
	a, tt := newItemsApp(t, 240+48, 400, src, func(c *ui.Context, ctx ItemsCtx) {
		WideRow(c, st, items, ctx, WideRowOpts{Title: "Row", Min: 240, Ratio: ratio, Crop: crop, First: true})
	})
	return a, tt, st
}

func TestWideRowFramesARowOf4To1BannersAt4To1RequestedAtThatShape(t *testing.T) {
	a, tt, _ := wideApp(t, nil, []services.Item{wideItem("1", artPath("1", 1000, 250), ""), wideItem("2", artPath("2", 1000, 250), "")}, 2, false)
	a.load(t, tt, 1)
	// The art's own edge line takes a pixel of the frame on each side.
	if w, h := frameOf(tt); w < 238 || w > 240 || h < 58 || h > 60 {
		t.Errorf("frame %d×%d, want 240×60", w, h)
	}
	if got := a.fake.requests(); !slices.Equal(got, [][2]int{{240, 60}}) {
		t.Errorf("requests %v, want [240 60]", got)
	}
}

func TestWideRowKeepsTheTallestBannersFrameInAMixedRowAndLetterboxesTheWiderOne(t *testing.T) {
	a, tt, _ := wideApp(t, nil, []services.Item{wideItem("1", artPath("1", 1000, 250), ""), wideItem("2", artPath("2", 1000, 417), "")}, 2, false)
	a.load(t, tt, 1)
	w, h := frameOf(tt)
	if got := float64(w) / float64(h); got < 2.39 || got > 2.41 {
		t.Errorf("frame %d×%d is %.3f, want 2.4", w, h, got)
	}
	// The 4:1 banner is bound by the frame's width, not squeezed or cropped.
	if got := a.fake.requests(); !slices.Equal(got, [][2]int{{240, 60}}) {
		t.Errorf("requests %v, want [240 60]", got)
	}
	// It sits whole in the middle with the item colour above and below.
	img := tt.Image()
	r := box(img, isFrame)
	mid := r.Min.X + r.Dx()/2
	if !closeTo(img.RGBAAt(mid, r.Min.Y+1), itemColour) || !closeTo(img.RGBAAt(mid, r.Min.Y+r.Dy()/2), fakeRed) || !closeTo(img.RGBAAt(mid, r.Max.Y-2), itemColour) {
		t.Errorf("not letterboxed: top %v, middle %v, bottom %v", img.RGBAAt(mid, r.Min.Y+1), img.RGBAAt(mid, r.Min.Y+r.Dy()/2), img.RGBAAt(mid, r.Max.Y-2))
	}
}

func TestWideRowNeverFramesTallerThanItsRatioEvenForASquareBanner(t *testing.T) {
	a, tt, _ := wideApp(t, nil, []services.Item{wideItem("1", artPath("1", 400, 400), "")}, 2.4, false)
	a.load(t, tt, 1)
	if w, h := frameOf(tt); w < 238 || w > 240 || h < 98 || h > 100 {
		t.Errorf("frame %d×%d, want 240×100", w, h)
	}
	if got := a.fake.requests(); !slices.Equal(got, [][2]int{{100, 100}}) {
		t.Errorf("requests %v, want [100 100]", got)
	}
}

func TestWideRowShowsACardWithoutABannerByItsWholeCoverInsideTheFrame(t *testing.T) {
	a, tt, _ := wideApp(t, nil, []services.Item{wideItem("1", "", artPath("1", 1000, 250))}, 2.4, false)
	a.load(t, tt, 1)
	if w, h := frameOf(tt); w < 238 || w > 240 || h < 98 || h > 100 {
		t.Errorf("frame %d×%d, want 240×100", w, h)
	}
	if got := a.fake.requests(); !slices.Equal(got, [][2]int{{240, 60}}) {
		t.Errorf("requests %v, want [240 60]", got)
	}
}

func TestWideRowCropsBannersToACroppingRowsFrameAndKeepsCoversWhole(t *testing.T) {
	for _, c := range []struct {
		name         string
		item         services.Item
		request      [2]int
		fillsFrame   bool
		wantRatio    float64
		cropsToFrame bool
	}{
		// Banners are requested at the frame's shape: Apple crops them to it.
		{"4:1 banner", wideItem("1", artPath("1", 1000, 250), ""), [2]int{240, 137}, true, 1.75, true},
		{"2.4:1 banner", wideItem("2", artPath("2", 1000, 417), ""), [2]int{240, 137}, true, 1.75, true},
		// A cover with no banner stays whole on the item colour.
		{"cover", wideItem("3", "", artPath("3", 400, 400)), [2]int{137, 137}, false, 1.75, false},
	} {
		a, tt, _ := wideApp(t, nil, []services.Item{c.item}, 1.75, true)
		a.load(t, tt, 1)
		w, h := frameOf(tt)
		if got := float64(w) / float64(h); got < c.wantRatio-0.02 || got > c.wantRatio+0.02 {
			t.Errorf("%s: frame %d×%d is %.3f, want %.2f", c.name, w, h, got, c.wantRatio)
		}
		if got := a.fake.requests(); !slices.Equal(got, [][2]int{c.request}) {
			t.Errorf("%s: requests %v, want %v", c.name, got, c.request)
		}
		img := tt.Image()
		r := box(img, isFrame)
		inset := img.RGBAAt(r.Min.X+6, r.Min.Y+r.Dy()/2) // just inside the frame's left edge
		if c.cropsToFrame && !closeTo(inset, fakeRed) {
			t.Errorf("%s: the banner does not fill the frame: %v at its edge", c.name, inset)
		}
		if !c.cropsToFrame && !closeTo(inset, itemColour) {
			t.Errorf("%s: the cover is not whole on the item colour: %v at the frame's edge", c.name, inset)
		}
	}
}

func TestWideRowBannerThatFailsToLoadLeavesTheItemColour(t *testing.T) {
	a, tt, _ := wideApp(t, &fakeArt{fail: 1}, []services.Item{wideItem("1", artPath("1", 1000, 250), "")}, 2, false)
	a.load(t, tt, 1)
	img := tt.Image()
	r := box(img, isFrame)
	if r.Empty() {
		t.Fatal("no frame at all")
	}
	if got := img.RGBAAt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2); !closeTo(got, itemColour) {
		t.Errorf("a failed banner shows %v, want the item colour", got)
	}
	// Not the --hover placeholder with a glyph either: the frame is one colour.
	other := 0
	for y := r.Min.Y + 2; y < r.Max.Y-2; y++ {
		for x := r.Min.X + 2; x < r.Max.X-2; x++ {
			if !closeTo(img.RGBAAt(x, y), itemColour) {
				other++
			}
		}
	}
	if other != 0 {
		t.Errorf("%d pixels of the frame are not the item colour", other)
	}
}

func TestWideRowLaysOutAsManyCardsAsFitAndPagesThroughTheRest(t *testing.T) {
	var items []services.Item
	for i := 1; i <= 5; i++ {
		items = append(items, wideItem(strconv.Itoa(i), artPath(strconv.Itoa(i), 1000, 250), ""))
	}
	st := &WideRowState{}
	a, tt := newItemsApp(t, 3*240+2*24+48, 400, nil, func(c *ui.Context, ctx ItemsCtx) {
		WideRow(c, st, items, ctx, WideRowOpts{Title: "Bricks", Min: 240, Ratio: 2, First: true})
	})
	a.load(t, tt, 0)
	for _, name := range []string{"Item 1", "Item 2", "Item 3"} {
		if !tt.HasText(name) {
			t.Errorf("%s not shown: %q", name, tt.Texts())
		}
	}
	if tt.HasText("Item 4") || !tt.HasText("1 of 2") {
		t.Errorf("first page shows %q", tt.Texts())
	}
	prev, ok := tt.Find("Previous in Bricks")
	if !ok {
		t.Fatalf("no Previous button: %q", tt.Texts())
	}
	_ = prev
	// Disabled at the start: the click does nothing.
	_ = tt.Click("Previous in Bricks")
	if st.Page != 0 {
		t.Errorf("Previous at the start moved to page %d", st.Page)
	}
	if err := tt.Click("Next in Bricks"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if st.Page != 1 || !tt.HasText("2 of 2") || !tt.HasText("Item 4") || !tt.HasText("Item 5") || tt.HasText("Item 1") {
		t.Errorf("page %d shows %q", st.Page, tt.Texts())
	}
	// Disabled at the end.
	_ = tt.Click("Next in Bricks")
	if st.Page != 1 {
		t.Errorf("Next at the end moved to page %d", st.Page)
	}
	if err := tt.Click("Previous in Bricks"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if st.Page != 0 || !tt.HasText("1 of 2") {
		t.Errorf("Previous went to page %d", st.Page)
	}
}

func TestWideRowAnUntitledRowIsFeaturedAndHasAHeadingOnlyWhenPaged(t *testing.T) {
	one := []services.Item{wideItem("1", artPath("1", 1000, 250), "")}
	st := &WideRowState{}
	a, tt := newItemsApp(t, 240+48, 400, nil, func(c *ui.Context, ctx ItemsCtx) {
		WideRow(c, st, one, ctx, WideRowOpts{Min: 240, Ratio: 2, First: true})
	})
	a.load(t, tt, 0)
	if _, ok := tt.Find("Featured"); !ok {
		t.Errorf("the region is not named Featured")
	}
	// The region and its list are named Featured; no heading shows.
	if n := countText(tt, "Featured"); n != 2 || tt.HasText("1 of 1") {
		t.Errorf("a single untitled page shows %d Featured (want the 2 names, no heading): %q", n, tt.Texts())
	}
	// Two cards in one column are two pages: the heading appears.
	two := append(one, wideItem("2", artPath("2", 1000, 250), ""))
	a.build = func(c *ui.Context, ctx ItemsCtx) {
		WideRow(c, st, two, ctx, WideRowOpts{Min: 240, Ratio: 2, First: true})
	}
	tt.Frame()
	tt.Frame()
	if n := countText(tt, "Featured"); n != 3 || !tt.HasText("1 of 2") {
		t.Errorf("a paged untitled row: %q", tt.Texts())
	}
	if _, ok := tt.Find("Next in Featured"); !ok {
		t.Error("no Next in Featured")
	}
}

func TestWideRowCardPlaysOpensAndOffersItsMenu(t *testing.T) {
	web := services.Item{Kind: services.ItemLink, Name: "Concert", URL: "https://example.com/c", Wide: services.Art{Medium: artPath("w", 1000, 250)}}
	items := []services.Item{wideItem("1", artPath("1", 1000, 250), ""), web}
	st := &WideRowState{}
	a, tt := newItemsApp(t, 2*240+24+48, 400, nil, func(c *ui.Context, ctx ItemsCtx) {
		WideRow(c, st, items, ctx, WideRowOpts{Title: "Row", Min: 240, Ratio: 2, First: true})
	})
	a.load(t, tt, 0)
	if _, ok := tt.Find("More for Item 1"); !ok {
		t.Errorf("no menu button: %q", tt.Texts())
	}
	if _, ok := tt.Find("More for Concert"); ok {
		t.Error("a web-only item has a menu")
	}
	// The play button plays without opening.
	if err := tt.Click("Play Item 1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.played, []string{"album:1"}) || len(a.rec.nav) != 0 {
		t.Errorf("played %v, navigated %v", a.rec.played, a.rec.nav)
	}
	if _, ok := tt.Find("Play Concert"); ok {
		t.Error("a web-only item has a play button")
	}
	// The card opens its page, and runs onOpen.
	if err := tt.Click("Item 1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.nav, []string{"/album/1"}) || !slices.Equal(a.rec.opened, []string{"album:1"}) {
		t.Errorf("navigated %v, opened %v", a.rec.nav, a.rec.opened)
	}
	// A card without a page opens in the browser.
	if err := tt.Click("Concert"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.urls, []string{"https://example.com/c"}) {
		t.Errorf("urls %v", a.rec.urls)
	}
}

func TestWideRowPlayButtonShowsOnHoverAtTheBannersBottomLeft(t *testing.T) {
	a, tt, _ := wideApp(t, nil, []services.Item{wideItem("1", artPath("1", 1000, 250), "")}, 2, false)
	a.load(t, tt, 1)
	play, ok := tt.Find("Play Item 1")
	if !ok {
		t.Fatalf("no play button: %q", tt.Texts())
	}
	frame := box(tt.Image(), isFrame)
	if play.X > float32(frame.Min.X)+13 || play.Y+play.H < float32(frame.Max.Y)-13 {
		t.Errorf("play button at %v, want the frame's bottom-left corner %v", play, frame)
	}
	amber := theme.C("play")
	at := func() color.RGBA { return tt.Image().RGBAAt(int(play.X+3), int(play.Y+play.H/2)) }
	want := color.RGBA{amber.R, amber.G, amber.B, 255}
	if closeTo(at(), want) {
		t.Errorf("the play button shows before the card is hovered: %v", at())
	}
	card, _ := tt.Find("Item 1")
	tt.Move(card.X+card.W/2, card.Y+card.H/4)
	tt.Frame()
	tt.Advance(time.Second)
	if got := at(); !near(got, want) {
		t.Errorf("hovering the card does not show the play button: %v, want about %v", got, want)
	}
}
