package artist

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
	"github.com/egoist/mygo/ui"
)

// A right-click on the artist's name or portrait opens the menu its "More"
// button opens, at the pointer; Shift+F10 and the Menu key open it from the
// keyboard. The catalogue page and a library artist's page share the head.
func TestHeadOpensTheArtistsMenuWhereItsMoreButtonDoes(t *testing.T) {
	tests := []struct {
		name, artist, focus string
		rig                 func(t *testing.T) *rig
	}{
		{"catalogue", "Radiohead", "Shuffle", func(t *testing.T) *rig { return catalogRig(t, nil) }},
		{"library with a catalogue page", "Mia", "Shuffle", func(t *testing.T) *rig { return libraryRig(t, true) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.rig(t)
			r.shows("More for " + tc.artist)
			r.tt.Drain()
			title, ok := r.tt.Find(tc.artist)
			if !ok {
				t.Fatalf("no name: %q", r.tt.Texts())
			}
			access.CheckMenuWays(t, r.tt.Tester, access.MenuWays{
				More:   "More for " + tc.artist,
				Point:  func() (float32, float32) { return title.X + 12, title.Y + title.H/2 },
				Focus:  func() { access.FocusByTab(t, r.tt.Tester, tc.focus) },
				Settle: r.tt.Drain,
			})
			// The round portrait sits at the trailing edge, where the menu opens
			// back to the left of the pointer: only the menu itself is checked.
			r2 := tc.rig(t)
			r2.shows("More for " + tc.artist)
			r2.tt.Drain()
			r2.tt.RightClickAt(1000, title.Y+60)
			r2.tt.Drain()
			if m, ok := access.Read(t, r2.tt.Tester).OpenMenu(); !ok || m.Label != "More for "+tc.artist {
				t.Errorf("a right-click on the portrait opened %q (%v)", m.Label, ok)
			}
		})
	}
}

// The latest-release card beside the head is another album, not part of the
// artist's header: Shift+F10 and the Menu key with the focus on its link do
// not open the artist's menu.
func TestLatestReleaseCardDoesNotOpenTheArtistsMenu(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("More for Radiohead")
	r.tt.Drain()
	const card = "Hail to the Thief (Live Recordings 2003-2009)"
	access.FocusByTab(t, r.tt.Tester, card)
	for _, k := range []struct {
		name string
		mods ui.Modifiers
		key  ui.Key
	}{{"Shift+F10", ui.Shift, ui.KeyF10}, {"the Menu key", 0, ui.KeyContextMenu}} {
		r.tt.Key(k.mods, k.key)
		r.tt.Drain()
		if m, ok := access.Read(t, r.tt.Tester).OpenMenu(); ok {
			t.Errorf("%s on the release card opened %q", k.name, m.Label)
		}
	}
}
