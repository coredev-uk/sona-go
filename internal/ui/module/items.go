package module

import (
	"context"

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
		Nav:     func(path string) { e.Nav.Push(path) },
		Players: e.Players,
		Art:     e.Art,
		Menus:   menus,
		Motion:  e.Motion,
	}
	if e.Core != nil {
		ctx.Restricted = services.Service[services.AuthService](e.Core).Status().Restricted
		catalog := services.Service[services.CatalogService](e.Core)
		ctx.OpenURL = func(url string) {
			go func() {
				if err := catalog.OpenInAppleMusic(url); err != nil && e.Log != nil {
					e.Log.Warn("open in Apple Music", "err", err)
				}
			}()
		}
		pb := services.Service[services.PlaybackService](e.Core)
		ctx.Toggle = func() {
			go func() {
				if err := pb.Toggle(context.Background()); err != nil && e.Log != nil {
					e.Log.Warn("toggle playback", "err", err)
				}
			}()
		}
	}
	if e.Play != nil {
		st := e.Play.State()
		cur := st.Current
		ctx.Playing = st.State == "playing" || st.State == "loading"
		ctx.OnAir = st.OnAir
		ctx.Current = func(s services.Song) bool {
			return cur != nil && (cur.ID == s.ID || (s.CatalogID != "" && cur.CatalogID == s.CatalogID))
		}
		ctx.CurrentStation = func(s services.Station) bool {
			return cur != nil && (cur.Live || cur.Broadcast) && (cur.ID == s.ID || cur.CatalogID == s.ID)
		}
	}
	return ctx
}
