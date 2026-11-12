package auth

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// Query keys an auth change makes stale, as v0.2.0's shared/lib/keys.ts.
var (
	keyDiagnostics = store.Key{"diagnostics"}
)

// authInvalidations is the queries an auth change invalidates (v0.2.0's
// features/auth/invalidations.ts). Signing in or out swaps the library (Go
// wipes or syncs it) and the catalogue's personal fields; a new storefront
// changes every catalogue answer; where the token is kept shows in the
// diagnostics. prev is nil for the first status seen: nothing changed yet.
func authInvalidations(prev *services.AuthStatus, next services.AuthStatus) []store.Key {
	if prev == nil {
		return nil
	}
	var out []store.Key
	wasIn := prev.State == "signed_in"
	isIn := next.State == "signed_in"
	if wasIn != isIn {
		out = append(out, store.KeyLibrary, store.KeyCatalog)
	} else if prev.StorefrontID != next.StorefrontID {
		out = append(out, store.KeyCatalog)
	}
	if wasIn != isIn || prev.Backend != next.Backend {
		out = append(out, keyDiagnostics)
	}
	return out
}
