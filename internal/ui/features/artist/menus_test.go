package artist

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
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
				More:  "More for " + tc.artist,
				Point: func() (float32, float32) { return title.X + 12, title.Y + title.H/2 },
				// The round portrait is at the head's trailing edge.
				AlsoPoints: []func() (float32, float32){func() (float32, float32) { return 1132 - 250, title.Y + 60 }},
				Focus:      func() { access.FocusByTab(t, r.tt.Tester, tc.focus) },
				Settle:     r.tt.Drain,
			})
		})
	}
}
