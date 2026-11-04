// Package search is the search feature: the top bar's search field with
// suggestions and recent searches, the results page (Apple Music or the
// library), "see all" pages per result group, and listener profiles
// (v0.2.0's features/search).
package search

import (
	"context"
	"log/slog"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// Routes.
const (
	searchPath  = "/search"
	groupPath   = "/search/{group}"
	profilePath = "/profile/{id}"
)

// searchAPI is what the feature asks of SearchService.
type searchAPI interface {
	Catalog(ctx context.Context, term string) (services.SearchResults, error)
	Group(ctx context.Context, term, group string, offset int) (services.SearchGroupPage, error)
	Hints(ctx context.Context, term string) (services.SearchHints, error)
	Browse(ctx context.Context) ([]services.Item, error)
	Library(ctx context.Context, term string, perKind int) (services.LibrarySearch, error)
	Profile(ctx context.Context, id string) (services.ProfileDetail, error)
	Recent(ctx context.Context) ([]services.RecentSearch, error)
	RememberTerm(ctx context.Context, term, scope string) error
	RememberItem(ctx context.Context, it services.RecentItem) error
	ForgetRecent(ctx context.Context, key string) error
	ClearRecent(ctx context.Context) error
}

// playbackAPI is what playing a song from a result asks of PlaybackService.
type playbackAPI interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
}

// authAPI is the sign-in state the results' empty and error states depend on.
type authAPI interface {
	Status() services.AuthStatus
}

// search is the feature's state. The field (field.go) and the pages
// (page.go, group.go, profile.go) keep their own state in their own structs.
type search struct {
	env  *module.Env
	api  searchAPI
	play playbackAPI
	auth authAPI
	log  *slog.Logger

	field *field
	pages pages
}

// New is the search module.
func New(env *module.Env) module.Module {
	s := newSearch(env,
		services.Service[services.SearchService](env.Core),
		services.Service[services.PlaybackService](env.Core),
		services.Service[services.AuthService](env.Core))
	return module.Module{
		ID: "search",
		Routes: []module.Route{
			{Pattern: searchPath, Title: func(*ui.Route) string { return i18n.T("Search") }, Build: s.searchPage},
			{Pattern: groupPath, Title: s.groupTitle, Build: s.groupPage},
			{Pattern: profilePath, Title: func(*ui.Route) string { return i18n.T("Profile") }, Build: s.profilePage},
		},
		Nav: []module.NavItem{{
			Section: module.NavPrimary,
			Label:   func() string { return i18n.T("Search") },
			Icon:    "search",
			To:      searchPath,
			Order:   20,
		}},
		Shortcuts: []module.Shortcut{s.focusShortcut()},
		Slots:     module.Slots{TopBarSearch: s.field.build},
		Init:      s.init,
	}
}

func newSearch(env *module.Env, api searchAPI, play playbackAPI, auth authAPI) *search {
	s := &search{env: env, api: api, play: play, auth: auth, log: env.Log}
	s.field = newField(s)
	return s
}
