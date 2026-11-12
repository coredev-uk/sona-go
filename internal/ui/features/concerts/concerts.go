// Package concerts is the Concerts feature: the shows near a city the user
// picks (Sona's own list of cities, kept in settings), filtered by date and
// genre, as an agenda by the venue's own calendar days, with ticket links
// that open in the browser. The artist pages' "Upcoming concerts" and
// Home's shelf (ArtistConcerts, ConcertsShelf) come from the same rows and
// queries.
package concerts

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// concerts is the feature's state.
type concerts struct {
	*data
	// today is the user's calendar day (the date presets and the From
	// field's earliest day count from it); tests move it.
	today func() time.Time

	page struct {
		pos ui.ScrollState
		// more is how many concerts the listing asks for, for the filters it
		// was raised under.
		more struct {
			key   string
			limit int
		}
		// prev is the last listing shown, kept on screen while a changed
		// filter loads.
		prev     *services.ConcertsPage
		from, to dateField
		rov      roving
		// view is the listing grouped for drawing, kept while it is still
		// the listing; jump is the day of the strip chosen, for the frame
		// that scrolls the agenda to it.
		view agendaView
		jump string
		// heard is the listing last read out (filters and whether it was
		// the first): the first listing is the page opening, which
		// announces itself, so it is only noted.
		heard      string
		heardYours bool
		heardSet   bool
	}
	where locationDialog

	// wasSignedIn is whether the account was signed in at the last auth
	// change: only a change is news.
	wasSignedIn *bool
}

// New is the Concerts module.
func New(env *module.Env) module.Module {
	env.ConcertsShelf = func(c *ui.Context, first bool) { ConcertsShelf(c, env, first) }
	env.ArtistConcerts = func(c *ui.Context, artistID string, first bool) { ArtistConcerts(c, env, artistID, first) }
	return newConcerts(env,
		services.Service[services.ConcertsService](env.Core)).module()
}

func newConcerts(env *module.Env, api concertsAPI) *concerts {
	return &concerts{data: &data{env: env, api: api}, today: time.Now}
}

func (s *concerts) module() module.Module {
	return module.Module{
		ID: "concerts",
		Routes: []module.Route{{
			Pattern: concertsPath,
			Title:   func(*ui.Route) string { return i18n.T("Concerts") },
			Build:   s.concertsPage,
		}},
		Nav: []module.NavItem{{
			// After Search, so the positional Ctrl+1…9 keys of the pages before it stay.
			Section: module.NavPrimary,
			Label:   func() string { return i18n.T("Concerts") },
			Icon:    "ticket",
			To:      concertsPath,
			Order:   25,
		}},
		Init: s.init,
	}
}

// init follows the account: the user's own artists are part of the list
// while signed in.
func (s *concerts) init() {
	store.On(s.env.Bus, services.EventAuthChanged, func(st services.AuthStatus) {
		now := account.HasAccount(st.State)
		if s.wasSignedIn != nil && *s.wasSignedIn != now {
			s.env.Queries.Invalidate(keyConcerts)
		}
		s.wasSignedIn = &now
	})
}

// authState is the sign-in state, "" until known.
func (s *concerts) authState() string {
	r := store.UseAuth(s.env.Queries, s.env.Core)
	return r.Data.State
}
