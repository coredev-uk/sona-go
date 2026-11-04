// Package song is the song feature: a catalogue song's own page (its
// header, lyrics, credits, music videos and more by its artist) and the
// "View credits" action on song rows. It is v0.2.0's features/song.
package song

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// songPattern is the page's route; id is a catalogue song ID.
const songPattern = "/song/{id}"

// catalogAPI is what the page asks of CatalogService.
type catalogAPI interface {
	SongPage(ctx context.Context, id string) (services.CatalogSong, error)
}

// lyricsAPI is what the lyrics block asks of LyricsService.
type lyricsAPI interface {
	Lyrics(ctx context.Context, songID string) (services.Lyrics, error)
}

// song is the feature's state.
type song struct {
	env     *module.Env
	catalog catalogAPI
	lyrics  lyricsAPI
	menus   *kit.ActionMenus

	page pageState
}

// New is the song module.
func New(env *module.Env) module.Module {
	s := newSong(env,
		services.Service[services.CatalogService](env.Core),
		services.Service[services.LyricsService](env.Core))
	return module.Module{
		ID: "song",
		Routes: []module.Route{{
			Pattern: songPattern,
			Title:   func(*ui.Route) string { return i18n.T("Song") },
			Build:   s.songPage,
		}},
		Actions: []kit.ActionDef{viewCredits},
	}
}

func newSong(env *module.Env, catalog catalogAPI, lyrics lyricsAPI) *song {
	s := &song{env: env, catalog: catalog, lyrics: lyrics}
	if env.Actions != nil {
		s.menus = kit.NewActionMenus(env.Actions, nil)
	}
	return s
}

// viewCredits is "View credits" on song rows: the song's own page, for a
// song with a catalogue ID.
var viewCredits = kit.ActionDef{
	ID:     "song.view-credits",
	Target: kit.KindSong,
	Label:  func(*kit.ActionContext) string { return i18n.T("View credits") },
	Icon:   "info",
	Group:  kit.GroupItem,
	Order:  15,
	Available: func(a *kit.ActionContext) bool {
		return a.Item().Song().CatalogID != ""
	},
	Run: func(a *kit.ActionContext) {
		a.Navigate("/song/" + a.Item().Song().CatalogID)
	},
}

// Query key of the page, as v0.2.0's shared/lib/keys.ts; the lyrics' is
// store.KeyLyrics.
func keySongPage(id string) store.Key { return store.Key{"catalog", "song-page", id} }
