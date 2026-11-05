// Package account is the shared sign-in and account UI: v0.2.0's
// features/auth/Account.tsx (everything but the avatar menu), signin.ts,
// queries.ts and KeyringRetry.tsx. The account dialog (features/auth) and
// the first-run setup's Apple Music step (features/onboarding) both build a
// Section; neither imports the other, so the views live here, like kit.
//
// Passwords, verification codes and tokens live in a field only while the
// view that asked for them is shown, are handed to the service in one call
// and are never logged, put in an error, a query key, a link or the
// clipboard.
package account

import (
	"context"
	"errors"
	"log/slog"
	"runtime"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// API is what the views use of *services.AuthService (tests fake it).
type API interface {
	Status() services.AuthStatus
	SignInWithPassword(ctx context.Context, account, password string) (*services.SecondFactor, error)
	SubmitSecurityCode(ctx context.Context, code string, phoneID int, mode string) error
	SendCodeToPhone(ctx context.Context, phoneID int, mode string) error
	ResendSecurityCode(ctx context.Context) error
	SignIn(ctx context.Context) error
	SignInWithPasskey(ctx context.Context) error
	SignInLink() (string, error)
	CancelSignIn()
	SignInWithToken(ctx context.Context, token string) error
	SignOut(ctx context.Context) error
	Revalidate(ctx context.Context) error
	RetryKeyring()
}

var _ API = (*services.AuthService)(nil)

// Deps is what a Section needs; NewDeps builds it from the Env.
type Deps struct {
	API     API
	Queries *store.Client
	Post    store.Poster
	OpenURL func(string) error
	Copy    func(string) bool
	Log     *slog.Logger
	// OS is the GOOS the keyring notices are worded for (kit.KeyringWords);
	// "" gets the Linux wording.
	OS string
}

// NewDeps is the Deps over the app's AuthService. An Env without services
// (uitest.Bare) has no API: the status query then fails and the views show
// its error.
func NewDeps(env *module.Env) Deps {
	d := Deps{Queries: env.Queries, Post: env.Post, OpenURL: env.OpenURL, Copy: env.Copy, Log: env.Log, OS: runtime.GOOS}
	if env.Core != nil {
		d.API = services.Service[services.AuthService](env.Core)
	}
	return d
}

var errNoAuth = errors.New("account: no auth service")

// UseStatus is useAuth(): the cached status, fetched with API.Status and
// kept current by the auth:changed handler (so it is never stale by age).
func UseStatus(d Deps) store.Result[services.AuthStatus] {
	return store.Use(d.Queries, store.KeyAuth, func(context.Context) (services.AuthStatus, error) {
		if d.API == nil {
			return services.AuthStatus{}, errNoAuth
		}
		return d.API.Status(), nil
	}, store.Options{Stale: store.Forever})
}

// HasAccount reports the account states in which the library and personal
// endpoints are usable: signed_in and validating.
func HasAccount(state string) bool { return state == "signed_in" || state == "validating" }

// IsSignedOut is known to be signed out: not before the status is known (""),
// not while Go still reads the stored sign-in from the keyring (restoring),
// and not when Apple only asked to sign in again (needs_reauth: the library
// is still here).
func IsSignedOut(state string) bool {
	return state != "" && state != "restoring" && state != "needs_reauth" && !HasAccount(state)
}

// NeedsAttention is what the account needs from the user: Apple stopped
// taking the token, the keyring didn't hand the sign-in over, or Apple
// refuses Sona's requests. The avatar carries a dot for these; offline and
// the keyring's own waits are the status banner's.
func NeedsAttention(s services.AuthStatus) bool {
	return s.State == "needs_reauth" || s.State == "error" ||
		(s.State == "signed_out" && s.Keyring == "unavailable")
}
