// Package newreleases is the artist tracker's list (Go's ArtistTrackerService,
// off until Settings → General turns it on): the new albums, EPs and singles
// of the artists the user has favourited, newest first, with the releases
// still to come on top. It is a shelf of For You ("New from Artists You Love",
// Env.NewReleasesShelf: the upcoming and unread releases, while there are
// any and the tracker is on) and, behind its "See all", a page of its own at
// /new-releases with every release, read or not. Each row is the release as
// an album (cover, title, artist and kind, its date) with the usual item
// menu; opening one marks it read. The page is also where a summary
// notification leads, and says so when the tracker is off.
// The list mirrors "artisttracker:changed" (store.UseNewReleases).
//
// A release still to come carries the album's "Remind me" action
// (features/reminders): the tracker has no notification of its own for it.
package newreleases

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

// The page's route, and the kind of the ActionContainer its rows are listed
// in, on the shelf and on the page (the actions below are listed only there).
const (
	path          = "/new-releases"
	containerKind = "new-releases"
	// settingsPath opens the Settings section that turns the tracker on.
	settingsPath = "/settings?section=general"
)

// api is what the page asks of ArtistTrackerService; tests replace it.
type api interface {
	MarkRead(ctx context.Context, id string) error
	MarkAllRead(ctx context.Context) error
	Dismiss(ctx context.Context, id string) error
	Clear(ctx context.Context) error
}

// newReleases is the feature's state.
type newReleases struct {
	env *module.Env
	api api
	now func() time.Time

	page struct {
		pos ui.ScrollState
		// width is the scroll container's width in the last frame: the
		// gutters narrow with it.
		width float32
	}
	// list is the page's rows (selection, keyboard navigation, item menu),
	// shelf the For You shelf's.
	list, shelf releaseList
}

// New is the New releases module.
func New(env *module.Env) module.Module {
	var a api
	if env.Core != nil {
		a = services.Service[services.ArtistTrackerService](env.Core)
	}
	n := newNewReleases(env, a)
	env.NewReleasesShelf = n.forYouShelf
	return n.module()
}

func newNewReleases(env *module.Env, a api) *newReleases {
	return &newReleases{env: env, api: a, now: time.Now}
}

func (n *newReleases) module() module.Module {
	return module.Module{
		ID: "newreleases",
		Routes: []module.Route{{
			Pattern: path,
			Title:   func(*ui.Route) string { return shelfTitle() },
			Build:   n.releasesPage,
		}},
		Actions: n.actions(),
		// Init reads what the shelf decides on, the list (SQLite only) and
		// whether the tracker is on, off the main thread, so that both are in
		// the cache by the time For You's page has arrived and sets its shelves.
		Init: func() {
			var snapshot func() services.NewReleases
			if n.env.Core != nil {
				svc := services.Service[services.ArtistTrackerService](n.env.Core)
				snapshot = func() services.NewReleases {
					r, _ := svc.Releases(context.Background())
					return r
				}
				store.Seed(n.env.Queries, n.env.Post, store.KeySettings, services.Service[services.SystemService](n.env.Core).Settings)
			}
			store.Mirror[services.NewReleases](n.env.Bus, n.env.Queries, n.env.Post, store.KeyNewReleases, snapshot, services.EventArtistReleasesChanged)
		},
	}
}

// shelfTitle names the shelf and the page behind its "See all".
func shelfTitle() string { return i18n.T("New from Artists You Love") }

// tracking reports whether the artist tracker is on (Settings → General): the
// shelf is shown only then. The page opens either way (the notification
// centre's link, a desktop notification's), and says the tracker is off and
// how to turn it on.
func (n *newReleases) tracking() bool {
	st := store.UseSettings(n.env.Queries, n.env.Core)
	return st.HasData && st.Data.ArtistTracker.Enabled
}

// do runs a service call off the main thread; the change comes back as
// "artisttracker:changed".
func (n *newReleases) do(f func(a api) error) {
	if n.api == nil {
		return
	}
	go func() {
		if err := f(n.api); err != nil && n.env.Log != nil {
			n.env.Log.Warn("new releases", "err", err)
		}
	}()
}

// actions are the item menu's two entries for a release in this list: "Mark
// as read" while it is unread, and "Remove from New releases" (item group, after
// the library's own actions).
func (n *newReleases) actions() []kit.ActionDef {
	here := func(a *kit.ActionContext) bool { return a.Container != nil && a.Container.Kind == containerKind }
	return []kit.ActionDef{{
		ID: "newreleases.mark-read", Target: kit.KindAlbum, Icon: "check", Group: kit.GroupItem, Order: 80,
		Label: func(*kit.ActionContext) string { return i18n.T("Mark as read") },
		Available: func(a *kit.ActionContext) bool {
			if !here(a) {
				return false
			}
			list, _ := store.Peek[services.NewReleases](n.env.Queries, store.KeyNewReleases)
			for _, r := range list.Items {
				if r.Album.CatalogID == a.Item().Album().CatalogID {
					return !r.Read
				}
			}
			return false
		},
		Run: func(a *kit.ActionContext) {
			id := a.Item().Album().CatalogID
			n.do(func(s api) error { return s.MarkRead(context.Background(), id) })
		},
	}, {
		ID: "newreleases.dismiss", Target: kit.KindAlbum, Icon: "x", Group: kit.GroupItem, Order: 81,
		Label:     func(*kit.ActionContext) string { return i18n.T("Remove from list") },
		Available: here,
		Run: func(a *kit.ActionContext) {
			id := a.Item().Album().CatalogID
			n.do(func(s api) error { return s.Dismiss(context.Background(), id) })
		},
	}}
}
