package album

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// TestRealServices runs New over the real services: a library album of the
// seeded fixture library is read by LibraryService, its Play button starts
// the queue through PlaybackService on the fake engine.
func TestRealServices(t *testing.T) {
	h := uitest.New(t)
	env := h.Env
	mod := New(env)
	if mod.ID != "album" || len(mod.Routes) != 1 || mod.Routes[0].Pattern != "/album/{id}" || mod.Init == nil {
		t.Fatalf("module %+v", mod)
	}
	env.Actions = kit.NewActionRegistry(mod.Actions...)
	page, err := services.Service[services.LibraryService](env.Core).Albums(context.Background(), "title", false, "", 0, 1)
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("library albums: %v %v", page, err)
	}
	want := page.Items[0]
	router := ui.NewRouter("/album/" + want.ID)
	env.Nav = router
	env.HasRoute = func(string) bool { return true }
	mod = New(env) // after the actions are in: its menus read them
	mod.Init()
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			router.View(c, func(rt *ui.Route) {
				if rt.Match("/album/{id}") {
					mod.Routes[0].Build(c, rt)
				}
			})
		})
	}, 1132, 1000)
	tt.Until("the album", tt.Shows(want.Name, "Play "+want.Name))
	if !tt.HasText("In library") {
		t.Errorf("a library album is not marked in the library; texts %q", tt.Texts())
	}
	if err := tt.Click("Play " + want.Name); err != nil {
		t.Fatal(err)
	}
	tt.Until("the engine to play", func() bool { return len(h.Engine.Played()) > 0 })
}
