// Package home is the For you page (v0.2.0's features/home): Apple's
// personal shelves for the signed-in account, read from Go's cache at once
// and renewed in the background ("discover:home"), set as a lead (Top
// Picks), the shelves Apple ranks first and an index of the rest (plan.go);
// signed out, offline without a copy yet, or when Home is empty, what Sona
// knows locally (Recently played, Recently added, Your playlists). It owns
// `/` (the nav's "For you" link) and `/for-you/{id}`, one shelf in full.
//
// The shared item components (kit.ItemShelf, kit.EditorialSections,
// kit.ItemGrid…) draw what v0.2.0's shared/items did; this package draws what
// lived in the feature: the lead band, the hero card, the compact item rows
// and the index.
package home

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

const (
	homePath     = "/"
	shelfPattern = "/for-you/{id}"
	shelfPrefix  = "/for-you/"
)

// discoverAPI is what the pages ask of DiscoverService.
type discoverAPI interface {
	Home(ctx context.Context) (services.HomePage, error)
	RefreshHome(ctx context.Context) (services.HomePage, error)
	HomeShelf(ctx context.Context, id string) (services.Section, error)
}

// libraryAPI is what the local shelves ask of LibraryService.
type libraryAPI interface {
	RecentlyPlayed(ctx context.Context, limit, offset int) ([]services.Song, error)
	History(ctx context.Context, days int) (services.ListeningHistory, error)
	Albums(ctx context.Context, sort string, desc bool, albumType string, offset, limit int) (services.AlbumPage, error)
	Playlists(ctx context.Context, sort string, desc bool, offset, limit int) (services.PlaylistPage, error)
}

// Query keys, as v0.2.0's discoverKeys and shared/lib/keys.ts.
var (
	keyHome = store.Key{"discover", "home"}
	// store.KeyLibraryRecent holds Apple's recently played songs; this
	// computer's history is under it so a new song refreshes both.
	keyRecentLocal = store.Key{"library", "recent", "local"}
	keyAdded       = store.Key{"library", "albums", "dateAdded", true, -1}
	keyPlaylists   = store.LibraryPlaylists("lastModified", true, -1)
)

func keyHomeShelf(id string) store.Key { return store.Key{"discover", "home-shelf", id} }

// home is the feature's state.
type home struct {
	env  *module.Env
	disc discoverAPI
	lib  libraryAPI

	// refreshing and refreshErr are the Refresh button's mutation
	// (useMutation's isPending and error).
	refreshing bool
	refreshErr error

	page kit.PageFrame // Home's scroll
	grid shelfPage

	sections map[string]*sectionState
	lead     leadState
	index    map[string]*indexLine
	local    localState
	titles   map[string]string // shelf titles seen, for the window title
}

// New is the home module.
func New(env *module.Env) module.Module {
	h := newHome(env,
		services.Service[services.DiscoverService](env.Core),
		services.Service[services.LibraryService](env.Core))
	return module.Module{
		ID: "home",
		Routes: []module.Route{
			{Pattern: homePath, Title: func(*ui.Route) string { return i18n.T("For you") }, Build: h.homePage},
			{Pattern: shelfPattern, Title: h.shelfTitle, Build: h.shelfPage},
		},
		Nav: []module.NavItem{{
			Section: module.NavPrimary,
			Label:   func() string { return i18n.T("For you") },
			Icon:    "sparkles",
			To:      homePath,
			Order:   10,
			Exact:   true,
		}},
		Init: h.init,
	}
}

func newHome(env *module.Env, disc discoverAPI, lib libraryAPI) *home {
	return &home{
		env: env, disc: disc, lib: lib,
		titles: map[string]string{},
	}
}

// init follows v0.2.0's eventHandlers: a renewed Home replaces the cached
// page without a refetch, and a change of account drops every discover
// query.
func (h *home) init() {
	q := h.env.Queries
	store.On(h.env.Bus, services.EventHome, func(p services.HomePage) { store.Set(q, keyHome, p) })
	store.On(h.env.Bus, services.EventAuthChanged, func(services.AuthStatus) {
		q.Invalidate(store.KeyDiscover)
	})
}

// authState is the sign-in state: known is false until Go has answered.
func (h *home) authState() (st services.AuthStatus, known bool) {
	r := store.UseAuth(h.env.Queries, h.env.Core)
	return r.Data, r.HasData
}

// opens reports whether an item whose page is link can be opened: an item
// with no page opens by its own means (a station plays, a web item opens in
// the browser), one with a page only when a module serves it.
func (h *home) opens(link string) bool {
	return link == "" || h.env.Routed(link)
}

// shelfMore is where a section's "See all" goes: its own page when Apple
// has one ("" for none).
func shelfMore(s services.Section) string {
	if s.SeeAll {
		return shelfPrefix + s.ID
	}
	return ""
}

// open opens an item as a tile does: its page, or what a page-less item
// does.
func (h *home) open(it services.Item, ctx kit.ItemsCtx) {
	link := kit.ItemLink(it)
	if link != "" && !h.opens(link) {
		return
	}
	if ctx.OnOpen != nil {
		ctx.OnOpen(it)
	}
	if link != "" {
		if ctx.Nav != nil {
			ctx.Nav(link)
		}
		return
	}
	kit.OpenWithoutPage(it, ctx)
}

// sectionState is the state of one Home shelf, made when it first shows.
type sectionState struct {
	shelf kit.CoverShelfState
	rows  rowShelf
	hero  heroCard
	ed    kit.EditorialState
	cap   captioned
	// menus is the shelf's item menu (the editorial sections' wide cards
	// and the hero's header menu draw on it).
	menus *kit.ActionMenus
}

func (h *home) section(id string) *sectionState {
	s := kit.Entry(&h.sections, id)
	kit.MenusIn(&s.menus, h.env.Actions, nil)
	return s
}
