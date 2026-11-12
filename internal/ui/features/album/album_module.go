// Package album is the album page: a library or a catalogue album as v0.2.0
// laid it out (a sleeve beside the listing, the head, the songs, Apple's
// sections), and "Go to album" on song rows.
package album

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// albumPath is the page's route.
const albumPath = "/album/{id}"

// libraryAPI is what the page asks of LibraryService.
type libraryAPI interface {
	Album(ctx context.Context, id string) (services.AlbumDetail, error)
}

// catalogAPI is what the page asks of CatalogService.
type catalogAPI interface {
	Album(ctx context.Context, id string) (services.CatalogAlbum, error)
}

// radioAPI is what a "More to hear" tile asks of RadioService.
type radioAPI interface {
	Play(ctx context.Context, id string) error
}

// New is the album module.
func New(env *module.Env) module.Module {
	a := newAlbum(env)
	a.lib = services.Service[services.LibraryService](env.Core)
	a.cat = services.Service[services.CatalogService](env.Core)
	a.radio = services.Service[services.RadioService](env.Core)
	return a.module()
}

// newAlbum is the page's state over an Env; the services are set by New
// (tests set fakes).
func newAlbum(env *module.Env) *album {
	a := &album{env: env, now: time.Now}
	a.menus = kit.NewActionMenus(env.Actions, nil)
	a.pageState = &pageState{}
	if env.Surfaces != nil {
		a.signIn = env.Surfaces.OpenAccount
	}
	return a
}

func (a *album) module() module.Module {
	return module.Module{
		ID: "album",
		Routes: []module.Route{{
			Pattern: albumPath,
			Title:   func(*ui.Route) string { return i18n.T("Album") },
			Build:   a.page,
		}},
		Actions: []kit.ActionDef{{
			ID:     "album.go-to",
			Target: kit.KindSong,
			Label:  func(*kit.ActionContext) string { return i18n.T("Go to album") },
			Icon:   "disc-3",
			Group:  kit.GroupItem,
			Order:  10,
			Available: func(a *kit.ActionContext) bool {
				return a.Item().Song().AlbumID != ""
			},
			Run: func(a *kit.ActionContext) {
				a.Navigate("/album/" + a.Item().Song().AlbumID)
			},
		}},
	}
}
