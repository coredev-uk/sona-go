package playlist

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// A right-click on a sidebar row (a playlist, a pin, a folder) opens the menu
// its "More" button opens, at the pointer; Shift+F10 and the Menu key open it
// from the keyboard.
func TestSidebarRowsOpenTheirMenuWhereTheirMoreButtonDoes(t *testing.T) {
	tests := []struct{ name, text, more string }{
		{"playlist", "Road Trip", "More for Road Trip"},
		{"pin", "Alpha", "More for Alpha"},
		{"folder", "Running", "More for the folder Running"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := sbSetup(t, sbLibrary(), sbPins())
			e.shows(tc.text)
			row, ok := e.tt.Find(tc.text)
			if !ok {
				t.Fatalf("no %q: %q", tc.text, e.tt.Texts())
			}
			access.CheckMenuWays(t, e.tt.Tester, access.MenuWays{
				More:   tc.more,
				Point:  func() (float32, float32) { return row.X + row.W/2, row.Y + row.H/2 },
				Focus:  func() { access.FocusByTab(t, e.tt.Tester, tc.text) },
				Settle: e.tt.Drain,
			})
		})
	}
}

// A right-click on the playlist page's cover or title opens the playlist's
// menu, as its "More" button does.
func TestPageHeaderOpensThePlaylistsMenuWhereItsMoreButtonDoes(t *testing.T) {
	r := viewRig(t)
	r.shows("More for Road Trip")
	title, ok := r.tt.Find("Road Trip")
	if !ok {
		t.Fatalf("no title: %q", r.tt.Texts())
	}
	access.CheckMenuWays(t, r.tt.Tester, access.MenuWays{
		More:       "More for Road Trip",
		Point:      func() (float32, float32) { return title.X + 12, title.Y + title.H/2 },
		AlsoPoints: []func() (float32, float32){func() (float32, float32) { return title.X - 60, title.Y + 10 }},
		Focus:      func() { access.FocusByTab(t, r.tt.Tester, "Shuffle") },
		Settle:     r.tt.Drain,
	})
}
