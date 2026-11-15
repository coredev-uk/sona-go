package player

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// Now Playing's "More" button opens the menu a right-click on its cover or
// song copy opens, at the pointer; Shift+F10 and the Menu key open it from the
// keyboard.
func TestNowPlayingOpensItsMenuWhereItsMoreButtonDoes(t *testing.T) {
	r := newNPRig(t)
	npActions(r)
	r.play(30_000)
	r.open("")
	title, ok := r.tt.Find("Song Title")
	if !ok {
		t.Fatalf("no title: %q", r.tt.Texts())
	}
	cover, ok := r.tt.Find("The Album cover")
	if !ok {
		t.Fatalf("no cover: %q", r.tt.Texts())
	}
	access.CheckMenuWays(t, r.tt.Tester, access.MenuWays{
		More:       "More for Song Title",
		Point:      func() (float32, float32) { return title.X + 12, title.Y + title.H/2 },
		AlsoPoints: []func() (float32, float32){func() (float32, float32) { return cover.X + 12, cover.Y + 12 }},
		Focus:      func() { access.FocusByTab(t, r.tt.Tester, "More for Song Title") },
		Settle:     r.tt.Drain,
	})
}
