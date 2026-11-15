package library

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// A right-click on the library artist's portrait or its line opens the menu
// the header's "More" button opens, at the pointer; Shift+F10 and the Menu key
// open it from the keyboard.
func TestArtistHeaderOpensItsMenuWhereItsMoreButtonDoes(t *testing.T) {
	r := newPageRig(t, artistsPath, 1132, 900, nil)
	r.tt.Until("the detail", r.tt.Shows("Artist · 2 albums, 4 songs", "More for Dara Finch"))
	line, ok := r.tt.Find("Artist · 2 albums, 4 songs")
	if !ok {
		t.Fatalf("no line: %q", r.tt.Texts())
	}
	access.CheckMenuWays(t, r.tt.Tester, access.MenuWays{
		More:  "More for Dara Finch",
		Point: func() (float32, float32) { return line.X + 12, line.Y + line.H/2 },
		// The portrait is left of the text.
		AlsoPoints: []func() (float32, float32){func() (float32, float32) { return line.X - 60, line.Y }},
		Focus:      func() { access.FocusByTab(t, r.tt.Tester, "Shuffle") },
		Settle:     r.tt.Drain,
	})
}
