package module

import (
	"context"
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// ItemsCtx is the context the shared item components (kit.ItemShelf,
// ItemGrid, EditorialSections, StationList, TrackShelf…) take, built over the
// Env (v0.2.0's router, browser, play registry and now-playing context): the
// router, the browser for web-only items, the item-play registry, the
// account's explicit-content restriction, artwork, motion and what plays.
// Build it each frame, on the main thread; menus is the page's item menu (nil:
// none).
func (e *Env) ItemsCtx(menus *kit.ActionMenus) kit.ItemsCtx {
	ctx := kit.ItemsCtx{
		Nav:      func(path string) { e.Nav.Push(path) },
		HasRoute: e.HasRoute,
		Players:  e.Players,
		Art:      e.Art,
		Menus:    menus,
		Motion:   e.Motion,
	}
	if e.Core != nil {
		ctx.Restricted = services.Service[services.AuthService](e.Core).Status().Restricted
		pb := services.Service[services.PlaybackService](e.Core)
		ctx.Toggle = func() {
			go func() {
				if err := pb.Toggle(context.Background()); err != nil && e.Log != nil {
					e.Log.Warn("toggle playback", "err", err)
				}
			}()
		}
	}
	// A web-only item opens in the browser (v0.2.0's openWithoutPage: any
	// item.url).
	if e.OpenURL != nil {
		ctx.OpenURL = func(url string) {
			if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
				return
			}
			go func() {
				if err := e.OpenURL(url); err != nil && e.Log != nil {
					e.Log.Warn("open in the browser", "err", err)
				}
			}()
		}
	}
	if e.Play != nil {
		st := e.Play.State()
		ctx.Current = st.Current
		ctx.Playing = st.State == "playing" || st.State == "loading"
		ctx.OnAir = st.OnAir
	}
	return ctx
}

// Routed reports whether a module serves path: HasRoute, false in a hand-built
// Env. A feature links to a page only when this is true.
func (e *Env) Routed(path string) bool { return e.HasRoute != nil && e.HasRoute(path) }
