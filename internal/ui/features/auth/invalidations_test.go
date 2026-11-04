package auth

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// cachedKeys is a cache with one query of each shape, for invalidation-plan
// tests (v0.2.0's shared/testing/query-cache.ts).
var cachedKeys = []store.Key{
	{"auth"},
	{"settings"},
	{"diagnostics"},
	{"library", "counts"},
	{"library", "sync-status"},
	{"library", "recent"},
	{"library", "songs", "title", false, 0},
	{"library", "albums", "dateAdded", true, 2},
	{"library", "playlists", "title", false, 0},
	{"library", "album", "l.abc"},
	{"library", "playlist", "p.abc"},
	{"catalog", "search", "radiohead"},
	{"catalog", "album", "1097861387"},
	{"catalog", "lyrics", "1097862703"},
}

// keyPath is a key as "library/songs/title/false/0".
func keyPath(k store.Key) string {
	parts := make([]string, len(k))
	for i, e := range k {
		parts[i] = fmt.Sprint(e)
	}
	return strings.Join(parts, "/")
}

// staleKeys is which of cached the cache holds stale: a query that is
// stale is fetched when a view next uses it. Every key must hold an int (Set
// it first); the paths come back in cached's order.
func staleKeys(t *testing.T, cl *store.Client, cached []store.Key, log *fetchLog, pump func()) []string {
	t.Helper()
	use := func(k store.Key) store.Result[int] {
		return store.Use(cl, k, func(context.Context) (int, error) {
			log.note(keyPath(k))
			return 2, nil
		}, store.Options{Stale: store.Forever})
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		if pump != nil {
			pump()
		}
		busy := false
		for _, k := range cached {
			if use(k).Fetching {
				busy = true
			}
		}
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the fetches never finished")
		}
	}
	var out []string
	for _, k := range cached {
		if log.has(keyPath(k)) {
			out = append(out, keyPath(k))
		}
	}
	return out
}

// fetchLog is the queries that were fetched. A query already in use is
// refetched at once when it is invalidated, by the fetch it was last used
// with, so one log has to outlive the calls of staleKeys that share a cache.
type fetchLog struct {
	mu sync.Mutex
	m  map[string]bool
}

func (l *fetchLog) note(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string]bool{}
	}
	l.m[path] = true
}

func (l *fetchLog) has(path string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m[path]
}

// staleAfter is which of cached the invalidation list marks stale.
func staleAfter(t *testing.T, invalidate []store.Key, cached []store.Key) []string {
	t.Helper()
	cl := store.NewClient(nil)
	for _, k := range cached {
		store.Set(cl, k, 1)
	}
	cl.Invalidate(invalidate...)
	return staleKeys(t, cl, cached, &fetchLog{}, nil)
}

func TestAuthInvalidations(t *testing.T) {
	signedIn := services.AuthStatus{State: "signed_in", StorefrontID: "gb", Backend: "secret-service"}
	signedOut := services.AuthStatus{State: "signed_out"}
	status := func(s services.AuthStatus) *services.AuthStatus { return &s }

	t.Run("does nothing for the first status", func(t *testing.T) {
		if got := authInvalidations(nil, signedIn); len(got) != 0 {
			t.Errorf("invalidated %v", got)
		}
	})

	t.Run("refreshes library, catalogue and diagnostics when signing in or out", func(t *testing.T) {
		in := authInvalidations(status(signedOut), signedIn)
		stale := staleAfter(t, in, cachedKeys)
		for _, want := range []string{"library/songs/title/false/0", "catalog/lyrics/1097862703", "diagnostics"} {
			if !slices.Contains(stale, want) {
				t.Errorf("%s not stale: %v", want, stale)
			}
		}
		for _, not := range []string{"auth", "settings"} {
			if slices.Contains(stale, not) {
				t.Errorf("%s is stale: %v", not, stale)
			}
		}
		if out := authInvalidations(status(signedIn), signedOut); !slices.EqualFunc(out, in, func(a, b store.Key) bool { return keyPath(a) == keyPath(b) }) {
			t.Errorf("signing out invalidates %v, signing in %v", out, in)
		}
	})

	t.Run("ignores the intermediate sign-in states", func(t *testing.T) {
		if got := authInvalidations(status(signedOut), services.AuthStatus{State: "signing_in"}); len(got) != 0 {
			t.Errorf("signed_out to signing_in invalidated %v", got)
		}
		if got := authInvalidations(status(services.AuthStatus{State: "signing_in"}), services.AuthStatus{State: "validating"}); len(got) != 0 {
			t.Errorf("signing_in to validating invalidated %v", got)
		}
	})

	t.Run("refreshes only the catalogue when the storefront changes", func(t *testing.T) {
		next := signedIn
		next.StorefrontID = "us"
		stale := staleAfter(t, authInvalidations(status(signedIn), next), cachedKeys)
		if len(stale) != 3 {
			t.Fatalf("stale %v, want the 3 catalogue queries", stale)
		}
		for _, k := range stale {
			if !strings.HasPrefix(k, "catalog/") {
				t.Errorf("%s is stale", k)
			}
		}
	})

	t.Run("refreshes diagnostics when the keyring backend changes", func(t *testing.T) {
		next := signedIn
		next.Backend = "file (unencrypted)"
		if got := staleAfter(t, authInvalidations(status(signedIn), next), cachedKeys); !slices.Equal(got, []string{"diagnostics"}) {
			t.Errorf("stale %v, want only diagnostics", got)
		}
	})
}
