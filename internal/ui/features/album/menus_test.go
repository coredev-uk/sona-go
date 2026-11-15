package album

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// A right-click on the album's cover or title opens the menu its "More"
// button opens, at the pointer; Shift+F10 and the Menu key open it from the
// keyboard, in the side-by-side head and the stacked one.
func TestHeaderOpensTheAlbumsMenuWhereItsMoreButtonDoes(t *testing.T) {
	for _, w := range []int{1132, 620} {
		t.Run(map[int]string{1132: "wide", 620: "stacked"}[w], func(t *testing.T) {
			r := newRig(t, "/album/100", w, 1000)
			r.h.Env.Actions.Register(kit.ActionDef{
				ID: "test.album-action", Target: kit.KindAlbum, Group: kit.GroupItem, Order: 1,
				Label: func(*kit.ActionContext) string { return "Do a thing" }, Icon: "star",
				Run: func(*kit.ActionContext) {},
			})
			r.addCatalog(saltFlats())
			r.shows("More for Salt Flats")
			title, ok := r.tt.Find("Salt Flats")
			if !ok {
				t.Fatalf("no title: %q", r.tt.Texts())
			}
			access.CheckMenuWays(t, r.tt.Tester, access.MenuWays{
				More:  "More for Salt Flats",
				Point: func() (float32, float32) { return title.X + 12, title.Y + title.H/2 },
				// The cover is left of the title.
				AlsoPoints: []func() (float32, float32){func() (float32, float32) { return title.X - 60, title.Y + 10 }},
				Focus:      func() { access.FocusByTab(t, r.tt.Tester, "Shuffle") },
				Settle:     r.tt.Drain,
			})
		})
	}
}
