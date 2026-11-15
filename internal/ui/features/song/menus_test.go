package song

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// A right-click on the song's cover or title opens the menu its "More" button
// opens, at the pointer; Shift+F10 and the Menu key open it from the
// keyboard, in the spine and the narrow band.
func TestHeadOpensTheSongsMenuWhereItsMoreButtonDoes(t *testing.T) {
	tests := []struct {
		name  string
		width int
		cover func(x, y, h float32) (float32, float32)
	}{
		{"spine", 1132, func(x, y, h float32) (float32, float32) { return x + 12, y - 60 }},
		{"band", 700, func(x, y, h float32) (float32, float32) { return x - 60, y + 10 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, tc.width, 1000, airbag())
			r.open()
			r.tt.Until("the menu button", r.tt.Shows("More for Airbag"))
			title, ok := r.tt.Find("Airbag")
			if !ok {
				t.Fatalf("no title: %q", r.tt.Texts())
			}
			access.CheckMenuWays(t, r.tt.Tester, access.MenuWays{
				More:  "More for Airbag",
				Point: func() (float32, float32) { return title.X + 12, title.Y + title.H/2 },
				AlsoPoints: []func() (float32, float32){func() (float32, float32) {
					return tc.cover(title.X, title.Y, title.H)
				}},
				Focus:  func() { access.FocusByTab(t, r.tt.Tester, "More for Airbag") },
				Settle: r.tt.Drain,
			})
		})
	}
}
