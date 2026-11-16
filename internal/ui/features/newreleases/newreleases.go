// Package newreleases is the artist tracker's page (Go's ArtistTrackerService,
// off until Settings → General turns it on): the new albums, EPs and singles
// of the artists the user has favourited, newest first, with the releases
// still to come on top. Each row is the release as an album (cover, title,
// artist and kind, its date) with the usual item menu; opening one marks it
// read. The sidebar link carries how many are unread (module.NavItem.Count).
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
// in (the actions below are listed only there).
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
	list releaseList
}

// New is the New releases module.
func New(env *module.Env) module.Module {
	var a api
	if env.Core != nil {
		a = services.Service[services.ArtistTrackerService](env.Core)
	}
	return newNewReleases(env, a).module()
}

func newNewReleases(env *module.Env, a api) *newReleases {
	return &newReleases{env: env, api: a, now: time.Now}
}

func (n *newReleases) module() module.Module {
	return module.Module{
		ID: "newreleases",
		Routes: []module.Route{{
			Pattern: path,
			Title:   func(*ui.Route) string { return i18n.T("New releases") },
			Build:   n.releasesPage,
		}},
		Nav: []module.NavItem{{
			// After Concerts, so the positional Ctrl+1…9 keys of the pages before it stay.
			Section: module.NavPrimary,
			Label:   func() string { return i18n.T("New releases") },
			Icon:    "calendar",
			To:      path,
			Order:   27,
			Count:   n.unread,
		}},
		Actions: n.actions(),
		Init: func() {
			var snapshot func() services.NewReleases
			if n.env.Core != nil {
				svc := services.Service[services.ArtistTrackerService](n.env.Core)
				snapshot = func() services.NewReleases {
					r, _ := svc.Releases(context.Background())
					return r
				}
			}
			store.Mirror[services.NewReleases](n.env.Bus, n.env.Queries, n.env.Post, store.KeyNewReleases, snapshot, services.EventArtistReleasesChanged)
		},
	}
}

// unread is how many releases are unread, for the sidebar link: 0 until the
// list is known.
func (n *newReleases) unread() int {
	r, _ := store.Peek[services.NewReleases](n.env.Queries, store.KeyNewReleases)
	return r.Unread
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
		Label:     func(*kit.ActionContext) string { return i18n.T("Remove from New releases") },
		Available: here,
		Run: func(a *kit.ActionContext) {
			id := a.Item().Album().CatalogID
			n.do(func(s api) error { return s.Dismiss(context.Background(), id) })
		},
	}}
}
