package kit

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

func linkItem(kind, id, name, url string) services.Item {
	return services.Item{Kind: kind, ID: id, Name: name, URL: url}
}

func exploreItems() []services.Item {
	return []services.Item{
		linkItem(services.ItemLink, "", "Concerts", "https://music.apple.com/concerts"),
		linkItem(services.ItemRoom, "6456176470", "Browse by Genre", ""),
		linkItem(services.ItemCharts, "", "Charts", ""),
		linkItem(services.ItemGrouping, "10023", "Music Videos", ""),
	}
}

func linkListApp(t *testing.T, width int, title string, items []services.Item) (*itemsApp, *ui.Tester) {
	t.Helper()
	st := &LinkListState{}
	a, tt := newItemsApp(t, width, 500, nil, func(c *ui.Context, ctx ItemsCtx) {
		LinkList(c, st, items, ctx, LinkListOpts{Title: title, First: true})
	})
	a.load(t, tt, 0)
	return a, tt
}

func TestLinkListListsEveryDestinationInAppleOrderPagesInSonaAsLinks(t *testing.T) {
	a, tt := linkListApp(t, 600, "More to Explore", exploreItems())
	// Filled down each column: two rows, two columns.
	pos := map[string][2]float32{}
	for _, name := range []string{"Concerts", "Browse by Genre", "Charts", "Music Videos"} {
		r, ok := tt.Find(name)
		if !ok {
			t.Fatalf("no %q: %q", name, tt.Texts())
		}
		pos[name] = [2]float32{r.X, r.Y}
	}
	if pos["Concerts"][0] != pos["Browse by Genre"][0] || pos["Concerts"][1] >= pos["Browse by Genre"][1] {
		t.Errorf("Concerts %v and Browse by Genre %v are not one column", pos["Concerts"], pos["Browse by Genre"])
	}
	if pos["Charts"][0] <= pos["Concerts"][0] || pos["Charts"][1] != pos["Concerts"][1] || pos["Charts"][0] != pos["Music Videos"][0] {
		t.Errorf("Charts %v, Music Videos %v: not a second column", pos["Charts"], pos["Music Videos"])
	}
	// Tab follows the same order.
	var order []string
	for range 4 {
		tt.Key(0, ui.KeyTab)
		for _, name := range []string{"Concerts", "Browse by Genre", "Charts", "Music Videos"} {
			if tt.Focused(name) {
				order = append(order, name)
			}
		}
	}
	if !slices.Equal(order, []string{"Concerts", "Browse by Genre", "Charts", "Music Videos"}) {
		t.Errorf("Tab order %v", order)
	}
	// Sona's pages are links.
	for name, want := range map[string]string{
		"Browse by Genre": "/room/6456176470", "Charts": "/charts", "Music Videos": "/grouping/10023",
	} {
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
		if got := a.rec.nav[len(a.rec.nav)-1]; got != want {
			t.Errorf("%s opened %q, want %q", name, got, want)
		}
	}
	if len(a.rec.urls) != 0 {
		t.Errorf("a Sona page opened the browser: %v", a.rec.urls)
	}
}

func TestLinkListOpensAWebOnlyDestinationInTheBrowserAndSaysSo(t *testing.T) {
	a, tt := linkListApp(t, 600, "More to Explore", exploreItems())
	if _, ok := tt.Find("Opens in your browser"); !ok {
		t.Fatalf("no arrow out: %q", tt.Texts())
	}
	n := 0
	for _, s := range tt.Texts() {
		if s == "Opens in your browser" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d arrows out, want one (Concerts is the only web-only link)", n)
	}
	if err := tt.Click("Concerts"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.urls, []string{"https://music.apple.com/concerts"}) || len(a.rec.nav) != 0 {
		t.Errorf("urls %v, nav %v", a.rec.urls, a.rec.nav)
	}
}

func TestLinkListPlaysAStationWithNoPageNamedByItsTextThenPlayWithNoArrow(t *testing.T) {
	a, tt := linkListApp(t, 600, "Stations", []services.Item{stationItem(false, true)})
	if err := tt.Click("Local FM Play"); err != nil {
		t.Fatalf("no button named \"Local FM Play\": %v (%q)", err, tt.Texts())
	}
	if !slices.Equal(a.rec.played, []string{"station:ra.1"}) || len(a.rec.urls) != 0 {
		t.Errorf("played %v, urls %v", a.rec.played, a.rec.urls)
	}
	if _, ok := tt.Find("Opens in your browser"); ok {
		t.Error("a station that plays has an arrow out")
	}
}

func TestLinkListNamesItsRegionByItsTitleAndAContinuationMoreLinks(t *testing.T) {
	_, tt := linkListApp(t, 600, "More to Explore", exploreItems())
	// The region, its list and the heading.
	if n := countText(tt, "More to Explore"); n != 3 {
		t.Errorf("titled list names itself %d times, want region + list + heading: %q", n, tt.Texts())
	}
	_, tt = linkListApp(t, 600, "", exploreItems())
	if n := countText(tt, "More links"); n != 2 {
		t.Errorf("untitled list names itself %d times, want region + list: %q", n, tt.Texts())
	}
}

func TestLinkListTitledIs40DIPsAboveAnUntitledContinuation24(t *testing.T) {
	gap := func(title string) float32 {
		st1, st2 := &LinkListState{}, &LinkListState{}
		a, tt := newItemsApp(t, 600, 600, nil, func(c *ui.Context, ctx ItemsCtx) {
			LinkList(c, st1, []services.Item{linkItem(services.ItemRoom, "1", "First", "")}, ctx, LinkListOpts{Title: "One", First: true})
			LinkList(c, st2, []services.Item{linkItem(services.ItemRoom, "2", "Second", "")}, ctx, LinkListOpts{Title: title})
		})
		a.load(t, tt, 0)
		r1, _ := tt.Find("First")
		r2, _ := tt.Find("Second")
		return r2.Y - r1.Y
	}
	titled, untitled := gap("Two"), gap("")
	// Titled: 40 above, then the 22 DIP heading and the 12 under it; untitled: 24.
	if got := titled - untitled; got != 40+22+12-24 {
		t.Errorf("titled %v, untitled %v: differ by %v, want 50", titled, untitled, got)
	}
}

func TestLinkListFillsColumnsTopToBottomWithEightDIPGaps(t *testing.T) {
	var items []services.Item
	for _, n := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
		items = append(items, linkItem(services.ItemRoom, n, "Link "+n, ""))
	}
	// 5 columns fit: eight links make two rows of tracks as wide as the genres'.
	_, tt := linkListApp(t, 5*184+4*8+48, "", items)
	pos := func(n string) ui.Rect { r, _ := tt.Find("Link " + n); return r }
	a, b, c := pos("A"), pos("B"), pos("C")
	if a.X != b.X || b.Y <= a.Y {
		t.Errorf("A %v, B %v: not down a column", a, b)
	}
	if c.X <= a.X {
		t.Errorf("C %v is not in the second column", c)
	}
	// Four columns of two: H is at the bottom of the fourth.
	h, g := pos("H"), pos("G")
	if h.X != g.X || h.Y <= g.Y {
		t.Errorf("G %v and H %v", g, h)
	}
}
