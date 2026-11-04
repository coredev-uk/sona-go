package library

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
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
	r.l = newLibrary(hn.Env,
		services.Service[services.LibraryService](hn.Core),
		services.Service[services.PlaybackService](hn.Core))
	mod := r.l.module()
	hn.Env.Actions = kit.NewActionRegistry(mod.Actions...)
	hn.Env.Actions.SetPoster(hn.Env.Post)
	hn.Env.Actions.SetNavigator(r.router.Push)
	if before != nil {
		before(r.l)
		// A wrapped service is the queue's and the item states' too.
		hn.Env.Queue = viaPlay(hn, r.l.play)
		hn.Env.ItemStates = kit.NewItemStates(hn.Env.Queries, hn.Env.Post, r.l.lib)
		hn.Env.ItemStates.Follow(hn.Env.Bus)
	}
	mod.Init()
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

// queueVia is the queue's service with PlayItems, UndoReplace and SetShuffle
// going to play, which a test may have wrapped.
type queueVia struct {
	kit.QueueService
	play playbackAPI
}

func (q queueVia) PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	return q.play.PlayItems(ctx, refs, start, from)
}

func (q queueVia) UndoReplace(ctx context.Context) (bool, error) { return q.play.UndoReplace(ctx) }
func (q queueVia) SetShuffle(on bool)                            { q.play.SetShuffle(on) }

// viaPlay is the harness's Env.Queue with play behind its PlaybackService.
func viaPlay(h *uitest.Harness, play playbackAPI) *kit.QueueCommands {
	return kit.NewQueueCommands(kit.QueueDeps{
		Svc:  queueVia{services.Service[services.PlaybackService](h.Core), play},
		Play: h.Env.Play,
		Post: h.Env.Post,
	})
}

// fakeAuth is the account in the state a.Status() names, as the auth module
// would have written it.
func fakeAuth(l *library, a interface{ Status() services.AuthStatus }) {
	store.Set(l.env.Queries, store.KeyAuth, a.Status())
}
