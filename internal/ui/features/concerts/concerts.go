// Package concerts is the Concerts feature: the shows near a city the user
// picks (Sona's own list of cities, kept in settings), filtered by date and
// genre, as an agenda by the venue's own calendar days, with ticket links
// that open in the browser. The artist pages' "Upcoming concerts" and
// Home's shelf (ArtistConcerts, ConcertsShelf) come from the same rows and
// queries.
package concerts

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// authAPI is the sign-in state the "Your artists" filter depends on.
type authAPI interface {
	Status() services.AuthStatus
}

// keyAuth is v0.2.0's keys.auth.
var keyAuth = store.Key{"auth"}

// concerts is the feature's state.
type concerts struct {
	*data
	auth authAPI
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
	}
	where locationDialog

	// wasSignedIn is whether the account was signed in at the last auth
	// change: only a change is news.
	wasSignedIn *bool
}

// New is the Concerts module.
func New(env *module.Env) module.Module {
	return newConcerts(env,
		services.Service[services.ConcertsService](env.Core),
		services.Service[services.AuthService](env.Core)).module()
}

func newConcerts(env *module.Env, api concertsAPI, auth authAPI) *concerts {
	return &concerts{data: &data{env: env, api: api}, auth: auth, today: time.Now}
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

// hasAccount: the account states in which the library and personal
// endpoints are usable.
func hasAccount(state string) bool { return state == "signed_in" || state == "validating" }

// init follows the account: the user's own artists are part of the list
// while signed in.
func (s *concerts) init() {
	store.On(s.env.Bus, services.EventAuthChanged, func(st services.AuthStatus) {
		store.Set(s.env.Queries, keyAuth, st)
		now := hasAccount(st.State)
		if s.wasSignedIn != nil && *s.wasSignedIn != now {
			s.env.Queries.Invalidate(keyConcerts)
		}
		s.wasSignedIn = &now
	})
}

// authState is the sign-in state, "" until known.
func (s *concerts) authState() string {
	r := store.Use(s.env.Queries, keyAuth, func(context.Context) (services.AuthStatus, error) {
		return s.auth.Status(), nil
	}, store.Options{Stale: store.Forever})
	return r.Data.State
}

// relativeTime is v0.2.0's relativeTime: "just now", then minutes, hours
// and days ago in short form; stamp is Unix milliseconds.
func relativeTime(stamp int64, now time.Time) string {
	if stamp <= 0 {
		return i18n.T("never")
	}
	s := int64(now.Sub(time.UnixMilli(stamp)).Round(time.Second) / time.Second)
	if s < 45 {
		return i18n.T("just now")
	}
	m := (s + 30) / 60
	if m < 60 {
		return i18n.T("{minutes, plural, one {# min. ago} other {# min. ago}}", "minutes", m)
	}
	h := (m + 30) / 60
	if h < 24 {
		return i18n.T("{hours, plural, one {# hr. ago} other {# hr. ago}}", "hours", h)
	}
	d := (h + 12) / 24
	if d == 1 {
		return i18n.T("yesterday")
	}
	return i18n.T("{days, plural, one {# day ago} other {# days ago}}", "days", d)
}
