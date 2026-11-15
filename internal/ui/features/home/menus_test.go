package home

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// A right-click on the lead's cover or title, and on the featured card's
// banner or title, opens the menu its "More" button opens, at the pointer;
// Shift+F10 and the Menu key open it from the keyboard. The titles are links,
// which take the pointer themselves.
func TestHomeHeadsOpenTheirMenuWhereTheirMoreButtonDoes(t *testing.T) {
	tests := []struct {
		name, title, play string
		// banner is a point on the picture, from the title's box.
		banner func(x, y float32) (float32, float32)
	}{
		{"lead", "Lead Album", "Play Lead Album", func(x, y float32) (float32, float32) { return x + 12, y - 60 }},
		{"featured card", "Featured Station", "Play Featured Station", func(x, y float32) (float32, float32) { return x + 400, y }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, "/", 1132, 1253, withHome(samplePage()))
			registerItemActions(r)
			r.until("the page", tc.title, "More for "+tc.title)
			title, ok := r.tt.Find(tc.title)
			if !ok {
				t.Fatalf("no title: %q", r.tt.Texts())
			}
			access.CheckMenuWays(t, r.tt.Tester, access.MenuWays{
				More:  "More for " + tc.title,
				Point: func() (float32, float32) { return title.X + 12, title.Y + title.H/2 },
				AlsoPoints: []func() (float32, float32){func() (float32, float32) {
					return tc.banner(title.X, title.Y)
				}},
				Focus:  func() { access.FocusByTab(t, r.tt.Tester, tc.play) },
				Settle: r.tt.Drain,
			})
		})
	}
}
