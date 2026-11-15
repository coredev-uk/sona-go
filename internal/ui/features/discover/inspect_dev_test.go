//go:build dev

package discover

import (
	"fmt"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/features/inspect"
)

// A development build lists Inspect in a hero card's menu: the card's item
// menu is the registry's, so the inspector's action appears with no code here.
func TestFrontCardMenuOffersInspectInADevelopmentBuild(t *testing.T) {
	api := &fakeAPI{}
	cards := make([]services.Item, 3)
	for i := range cards {
		cards[i] = playlistItem(fmt.Sprint(i+1), fmt.Sprintf("List %d", i+1))
	}
	r := newRig(t, "/new", withAPI(api), wideWindow())
	r.env.Actions.Register(inspect.New(r.env).Actions...)
	newPageWith(api, r.h.Clock.Now(), heroSection(cards))
	r.tt.Until("the front page", r.tt.Shows("Featured"))
	r.d.front.paused = true
	x, y := r.rowCentre(1)
	r.tt.RightClickAt(x, y)
	r.tt.Drain()
	if !r.d.front.menus.IsOpen() || !r.tt.HasText("Inspect") {
		t.Fatalf("a right-click on a hero card offered no Inspect: %q", r.tt.Texts())
	}
}
