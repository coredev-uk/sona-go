package video

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// A right-click on the music video's picture or title opens the menu its
// "More" button opens, at the pointer; Shift+F10 and the Menu key open it from
// the keyboard, with the actions beside the title and under it.
func TestHeroOpensTheVideosMenuWhereItsMoreButtonDoes(t *testing.T) {
	for _, w := range []int{1132, 600} {
		t.Run(map[int]string{1132: "caption row", 600: "stacked"}[w], func(t *testing.T) {
			p := newPageRig(t, false, w, 1253)
			p.loaded()
			title, ok := p.tt.Find("Sky Video")
			if !ok {
				t.Fatalf("no title: %q", p.tt.Texts())
			}
			access.CheckMenuWays(t, p.tt.Tester, access.MenuWays{
				More:       "More for Sky Video",
				Point:      func() (float32, float32) { return title.X + 12, title.Y + title.H/2 },
				AlsoPoints: []func() (float32, float32){func() (float32, float32) { return title.X + 12, title.Y - 100 }},
				Focus:      func() { access.FocusByTab(t, p.tt.Tester, "Play Sky Video") },
				Settle:     p.tt.Drain,
			})
		})
	}
}
