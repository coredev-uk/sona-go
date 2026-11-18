//go:build dev

package own

import (
	"log/slog"
	"testing"
)

// A development build takes a fake service's address only when it is on this
// machine: a real service's address in the variable is ignored.
func TestDevEndpointsAreLoopbackOnly(t *testing.T) {
	for url, want := range map[string]string{
		"http://127.0.0.1:8123":          "http://127.0.0.1:8123",
		"http://localhost:8123/2.0/":     "http://localhost:8123/2.0/",
		"http://[::1]:8123":              "http://[::1]:8123",
		"https://api.listenbrainz.org":   "",
		"http://192.168.1.5:8123":        "",
		"http://127.0.0.1.evil.example/": "",
		"ftp://127.0.0.1/":               "",
		"not a url":                      "",
		"":                               "",
	} {
		t.Setenv("SONA_DEV_ENDPOINT_LISTENBRAINZ", url)
		if got := devEndpoints(slog.New(slog.DiscardHandler)).ListenBrainz; got != want {
			t.Errorf("%q: got %q, want %q", url, got, want)
		}
	}
}
