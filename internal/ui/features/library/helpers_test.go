package library

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// pageRig is the library's real module (its routes, actions and
// subscriptions) over a uitest Env: the shell's router builds whichever
// page the location names.
type pageRig struct {
	t      *testing.T
	h      *uitest.Harness
	router *ui.Router
	l      *library
	tt     *uitest.Session
}

// newPageRig builds the module over uitest.New(opts...) in a w×h window,
// starting at start (a location such as songsPath). The services are the
// real ones; tests wrap l.lib / l.play before the first frame with the
// before hook when they need to record or fail a call.
func newPageRig(t *testing.T, start string, w, h int, before func(l *library), opts ...uitest.Option) *pageRig {
	t.Helper()
	hn := uitest.New(t, opts...)
	r := &pageRig{t: t, h: hn, router: ui.NewRouter(start)}
	hn.Env.Nav = r.router
	hn.Env.HasRoute = func(string) bool { return false }
	real := services.Service[services.AuthService](hn.Core)
	r.l = newLibrary(hn.Env,
		services.Service[services.LibraryService](hn.Core),
		services.Service[services.PlaybackService](hn.Core), real)
	mod := r.l.module()
	hn.Env.Actions = kit.NewActionRegistry(mod.Actions...)
	hn.Env.Actions.SetPoster(hn.Env.Post)
	hn.Env.Actions.SetNavigator(r.router.Push)
	if before != nil {
		before(r.l)
		r.l.queue.svc = r.l.play // a wrapped PlaybackService is the queue's too
	}
	mod.Init()
	if r.l.auth != real {
		// A fake account: the real manager announces its own state (signed
		// out) once it has read the keyring; the fake announces itself after
		// that, so it is the one the page reads.
		manager := services.Service[services.AuthService](hn.Core)
		for end := time.Now().Add(5 * time.Second); manager.Status().State == "restoring" && time.Now().Before(end); time.Sleep(time.Millisecond) {
		}
		time.Sleep(50 * time.Millisecond)
		hn.Env.Bus.Emit(services.EventAuthChanged, r.l.auth.Status())
	}
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen clock never finishes a transition: each input would cost 20 frames
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				for _, route := range mod.Routes {
					if rt.Match(route.Pattern) {
						route.Build(c, rt)
						return
					}
				}
			})
			// What the shell mounts around the pages.
			kit.OverlayLayer(c)
			mod.Slots.SidebarFooter(c)
		})
	}, w, h)
	return r
}

// emptyTheSyncedLibrary removes everything the seeded (and so synced) library
// lists, as a synced account with nothing in its library: the page reads it
// again.
func emptyTheSyncedLibrary(r *pageRig) {
	r.t.Helper()
	if _, err := r.h.DB.W.ExecContext(r.t.Context(), `DELETE FROM library_items`); err != nil {
		r.t.Fatal(err)
	}
	r.l.env.Queries.Invalidate(keyLibrary)
}
