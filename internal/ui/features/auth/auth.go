// Package auth is sign-in state: the auth query and its Go event, the top
// bar's account menu and the account dialog it opens, and the palette's way
// to it (v0.2.0's features/auth).
//
// The account views themselves (the sign-in flow, the signed-in account,
// the sign-out confirm, the keyring's notices) are internal/ui/account's,
// shared with the first-run setup; this package owns what is specific to the
// top bar: the avatar and its menu, the dialog that hosts account.Section,
// the "Account settings" command, and keeping the cached sign-in status
// current from "auth:changed" (and the queries a sign-in or sign-out makes
// stale refetched).
//
// The account dialog is open while store.Surfaces says so: every "Sign in"
// action in the app calls Surfaces.OpenAccount without knowing this package.
package auth

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// feature is the module's state. The menu and the dialog share one
// account.Section, so the sign-out confirm the menu asks for and the one the
// dialog's signed-in view asks for are the same (kit.Confirm keeps one).
type feature struct {
	env     *module.Env
	d       account.Deps
	section *account.Section

	// menu is the avatar's menu.
	menu kit.Menu
	// shown is whether the dialog was open in the last frame: it takes the
	// focus when it opens, and tells Surfaces when the user closes it.
	shown bool
	// last is the status the previous "auth:changed" (or Init) saw: what the
	// invalidations compare with. It is not read back from the query: other
	// handlers of the event write the new status there first.
	last *services.AuthStatus
}

// New is the auth feature's module.
func New(env *module.Env) module.Module {
	return newFeature(env, account.NewDeps(env)).module()
}

func newFeature(env *module.Env, d account.Deps) *feature {
	return &feature{env: env, d: d, section: account.New(d)}
}

func (f *feature) module() module.Module {
	return module.Module{
		ID: "auth",
		Shortcuts: []module.Shortcut{{
			ID: "auth.account", Group: "general", Icon: "circle-user",
			Label:    func() string { return i18n.T("Account settings") },
			Keywords: func() string { return i18n.T("account apple music sign in sign out storefront keyring") },
			Run:      f.env.Surfaces.OpenAccount,
		}},
		Slots: module.Slots{TopBarAccount: f.topBar},
		Init:  f.init,
	}
}

// init follows the account: Go's "auth:changed" is the auth query's new
// data, and what the change makes stale is refetched.
func (f *feature) init() {
	st := f.d.API.Status()
	f.last = &st
	store.On(f.env.Bus, services.EventAuthChanged, f.changed)
}

func (f *feature) changed(next services.AuthStatus) {
	store.Set(f.env.Queries, store.KeyAuth, next)
	prev := f.last
	f.last = &next
	// Invalidate with no keys would make every query stale.
	if keys := authInvalidations(prev, next); len(keys) > 0 {
		f.env.Queries.Invalidate(keys...)
	}
}
