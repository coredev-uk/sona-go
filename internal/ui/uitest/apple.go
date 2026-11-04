package uitest

import (
	"net/http"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// AppleHost is the host the fake Apple Music API answers to.
const AppleHost = "api.music.apple.com"

// Apple signs the Core in to a fake Apple Music account (storefront "gb", a
// Music User Token that is always valid) and serves every Apple Music API
// request with h, in process on the harness Network: the responses a test
// gives are JSON shaped like Apple's. Requests the handler does not know
// should answer 404. Without it the Core is signed out and has no API.
func Apple(h http.Handler) Option { return func(o *options) { o.apple = h } }

// serveApple registers the fake API (wrapped with the two requests the
// account's validation makes) on the Network.
func (h *Harness) serveApple(next http.Handler) {
	h.Network.ServeHandler(AppleHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/me/storefront":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"gb","type":"storefronts","attributes":{"name":"United Kingdom","defaultLanguageTag":"en-GB","supportedLanguageTags":["en-GB"]}}]}`))
		case strings.HasPrefix(r.URL.Path, "/v1/storefronts/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"gb","type":"storefronts"}]}`))
		default:
			next.ServeHTTP(w, r)
		}
	}))
}

func appleStorefront(on bool) func() string {
	if !on {
		return nil
	}
	return func() string { return "gb" }
}

// signedIn is a keyring holding a Music User Token.
type signedIn struct{ noAccount }

func (signedIn) Get() (store.Record, error) {
	return store.Record{Token: "uitest-music-user-token", ObtainedAt: Epoch.Add(-time.Hour)}, nil
}
