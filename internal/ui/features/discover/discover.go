// Package discover is the Discover feature: Apple Music's editorial pages
// (New, rooms, multirooms, groupings, curators) and Charts, and the players
// that make the shared item cards play songs, albums, playlists and music
// videos. Discover is v0.2.0's features/discover: the front page with its
// rotating featured lead, the contents and the bands of shelves, the
// editorial pages, rooms of 100 and charts of 50 that load as they scroll.
package discover

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// The routes.
const (
	pathNew    = "/new"
	pathCharts = "/charts"
)

// discoverAPI is what the pages ask of DiscoverService.
type discoverAPI interface {
	New(ctx context.Context) (services.EditorialPage, error)
	Curator(ctx context.Context, id string) (services.EditorialPage, error)
	Multiroom(ctx context.Context, id string) (services.EditorialPage, error)
	Grouping(ctx context.Context, id string) (services.EditorialPage, error)
	Room(ctx context.Context, id, sort string) (services.RoomPage, error)
	RoomItems(ctx context.Context, id, sort string, offset int) (services.ItemPage, error)
	Charts(ctx context.Context, genre string) (services.ChartsPage, error)
	ChartItems(ctx context.Context, key, chartID, genre string, offset int) (services.ItemPage, error)
}

// tracksAPI is where the songs of an album or a playlist come from (v0.2.0's
// shared/actions/songs.ts tracksOf).
type tracksAPI interface {
	Tracks(ctx context.Context, it services.Item) ([]services.Song, error)
}

// serviceTracks reads an album's tracks from the library or the catalogue and
// a playlist's from PlaylistsService.
type serviceTracks struct {
	lib *services.LibraryService
	cat *services.CatalogService
	pls *services.PlaylistsService
}

func (s serviceTracks) Tracks(ctx context.Context, it services.Item) ([]services.Song, error) {
	switch {
	case it.Album != nil:
		if it.Album.Source == services.SourceLibrary {
			d, err := s.lib.Album(ctx, it.Album.ID)
			return d.Tracks, err
		}
		d, err := s.cat.Album(ctx, it.Album.ID)
		return d.Tracks, err
	case it.Playlist != nil:
		d, err := s.pls.Playlist(ctx, it.Playlist.ID)
		return d.Tracks, err
	}
	return nil, nil
}

// discover is the feature's state.
type discover struct {
	env    *module.Env
	api    discoverAPI
	tracks tracksAPI
	queue  *queueCommands
	// now is the clock the freshness line is read against.
	now func() time.Time

	front  front
	newPg  newPage
	edit   editorialPage
	room   roomPage
	charts chartsPage
	chart  chartPage
}

// New is the Discover module.
func New(env *module.Env) module.Module { return newFor(env).module() }

// newFor is the feature over the services of the Env's Core.
func newFor(env *module.Env) *discover {
	pb := services.Service[services.PlaybackService](env.Core)
	sys := services.Service[services.SystemService](env.Core)
	d := newDiscover(env,
		services.Service[services.DiscoverService](env.Core),
		serviceTracks{
			lib: services.Service[services.LibraryService](env.Core),
			cat: services.Service[services.CatalogService](env.Core),
			pls: services.Service[services.PlaylistsService](env.Core),
		},
		&queueCommands{
			svc: pb, play: env.Play, post: env.Post,
			replaceMode: func() string { return sys.Settings().Queue.ReplaceConfirm },
		})
	d.registerPlayers()
	return d
}

// module is what the shell gets: the routes, the sidebar's two links and the
// subscriptions.
func (d *discover) module() module.Module {
	return module.Module{
		ID: "discover",
		Routes: []module.Route{
			{Pattern: pathNew, Title: func(*ui.Route) string { return i18n.T("Discover") }, Build: d.newPage},
			{Pattern: "/room/{id}", Build: d.roomPage},
			{Pattern: "/curator/{id}", Build: d.curatorPage},
			{Pattern: "/multiroom/{id}", Build: d.multiroomPage},
			{Pattern: "/grouping/{id}", Build: d.groupingPage},
			{Pattern: pathCharts, Title: func(*ui.Route) string { return i18n.T("Charts") }, Build: d.chartsPage},
			{Pattern: "/charts/{chart}", Build: d.chartPage},
		},
		Nav: []module.NavItem{
			{Section: module.NavPrimary, Label: func() string { return i18n.T("Discover") }, Icon: "compass", To: pathNew, Order: 15},
			{Section: module.NavPrimary, Label: func() string { return i18n.T("Charts") }, Icon: "chart-no-axes-column", To: pathCharts, Order: 17},
		},
		Init: d.init,
	}
}

func newDiscover(env *module.Env, api discoverAPI, tracks tracksAPI, queue *queueCommands) *discover {
	d := &discover{env: env, api: api, tracks: tracks, queue: queue, now: time.Now}
	d.front.menus = kit.NewActionMenus(env.Actions, nil)
	d.newPg.menus = kit.NewActionMenus(env.Actions, nil)
	d.edit.menus = kit.NewActionMenus(env.Actions, nil)
	d.room.menus = kit.NewActionMenus(env.Actions, nil)
	d.chart.menus = kit.NewActionMenus(env.Actions, nil)
	d.charts.menus = kit.NewActionMenus(env.Actions, nil)
	return d
}

// init ports v0.2.0's event handling for the keys of discover: a sign-in
// change (the account, its storefront) invalidates every discover query, so
// the pages load again for the new account.
func (d *discover) init() {
	store.On(d.env.Bus, services.EventAuthChanged, func(services.AuthStatus) {
		d.env.Queries.Invalidate(keyDiscover)
	})
}
