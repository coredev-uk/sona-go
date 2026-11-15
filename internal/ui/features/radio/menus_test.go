package radio

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// A right-click on the station page's cover or name opens the menu its "More"
// button opens, at the pointer; Shift+F10 and the Menu key open it from the
// keyboard.
func TestStationPageOpensItsMenuWhereItsMoreButtonDoes(t *testing.T) {
	s := liveStation("ra.1", "Apple Music 1")
	rg := stationRig(t, services.StationDetail{Station: s})
	rg.tt.Until("the header", rg.tt.Shows("More for Apple Music 1"))
	title, ok := rg.tt.Find("Apple Music 1")
	if !ok {
		t.Fatalf("no name: %q", rg.tt.Texts())
	}
	access.CheckMenuWays(t, rg.tt.Tester, access.MenuWays{
		More:  "More for Apple Music 1",
		Point: func() (float32, float32) { return title.X + 12, title.Y + title.H/2 },
		// The cover is above the name, in the identity column.
		AlsoPoints: []func() (float32, float32){func() (float32, float32) { return title.X + 12, title.Y - 60 }},
		Focus:      func() { access.FocusByTab(t, rg.tt.Tester, "Play Apple Music 1") },
		Settle:     rg.tt.Drain,
	})
}
