package applemusic

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"testing"
)

func TestContentLanguage(t *testing.T) {
	canada := []string{"en-CA", "fr-CA"}
	for _, tc := range []struct {
		name      string
		preferred []string
		def       string
		supported []string
		want      string
	}{
		{"exact tag the storefront offers", []string{"fr-CA"}, "en-CA", canada, "fr-CA"},
		{"bare language picks the storefront's tag", []string{"fr"}, "en-CA", canada, "fr-CA"},
		{"another region of an offered language", []string{"fr-FR"}, "en-CA", canada, "fr-CA"},
		{"Apple's spelling is kept", []string{"FR-ca"}, "en-CA", canada, "fr-CA"},
		{"the default is sent as nothing", []string{"en-CA"}, "en-CA", canada, ""},
		{"the default's language answers its other regions", []string{"en-US", "fr-CA"}, "en-CA", canada, ""},
		{"the first preferred tag the storefront offers wins", []string{"de-DE", "fr-CA", "en-CA"}, "en-CA", canada, "fr-CA"},
		{"none offered", []string{"de-DE", "ja"}, "en-CA", canada, ""},
		{"unknown storefront languages", []string{"fr-CA"}, "", nil, ""},
		{"no preference", nil, "en-CA", canada, ""},
		{"blank tags are skipped", []string{"", " ", "fr"}, "en-CA", canada, "fr-CA"},
		{"script subtags", []string{"zh-Hant-TW"}, "zh-Hans-CN", []string{"zh-Hans-CN", "zh-Hant-TW", "en-GB"}, "zh-Hant-TW"},
		{"English in a French storefront", []string{"en-US"}, "fr-FR", []string{"fr-FR", "en-GB"}, "en-GB"},
		{"another region of the same script", []string{"zh-Hant-HK"}, "zh-Hans-CN", []string{"zh-Hans-CN", "zh-Hant-TW"}, "zh-Hant-TW"},
		{"the default's script answers its regions", []string{"zh-Hans-SG"}, "zh-Hans-CN", []string{"zh-Hans-CN", "zh-Hant-TW"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContentLanguage(tc.preferred, tc.def, tc.supported); got != tc.want {
				t.Errorf("ContentLanguage(%q, %q, %q) = %q, want %q", tc.preferred, tc.def, tc.supported, got, tc.want)
			}
		})
	}
}

// TestRequestsSendContentLanguage checks which requests carry l=: GETs of
// Apple's API hosts, not playback assets, writes or a URL with its own.
func TestRequestsSendContentLanguage(t *testing.T) {
	var mu sync.Mutex
	got := map[string]string{}
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got[r.Method+" "+r.URL.Path] = r.URL.Query().Get("l")
		mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	c.AMPBase = c.Base + "/amp"
	lang := "fr-CA"
	c.Language = func() string { return lang }
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.Get(ctx, "/v1/catalog/ca/albums/1", nil, nil))
	must(c.Get(ctx, c.AMPBase+"/v1/catalog/ca/stations/2", url.Values{"platform": {"web"}}, nil))
	must(c.Get(ctx, "/v1/catalog/ca/songs/3", url.Values{"l": {"en-CA"}}, nil))
	must(c.Get(ctx, "/v1/catalog/ca/songs/4?l=en-CA", nil, nil))
	must(c.Get(ctx, c.AMPBase+"/v1/play/assets", nil, nil))
	must(c.Do(ctx, Request{Method: http.MethodPost, URL: "/v1/me/library", Query: url.Values{"ids[songs]": {"5"}}}, nil))
	must(c.Get(ctx, "/v1/catalog/ca/songs/7/lyrics", url.Values{"l[lyrics]": {"fr-ca"}}, nil))
	lang = ""
	must(c.Get(ctx, "/v1/catalog/ca/albums/6", nil, nil))

	want := map[string]string{
		"GET /v1/catalog/ca/albums/1":       "fr-CA",
		"GET /amp/v1/catalog/ca/stations/2": "fr-CA",
		"GET /v1/catalog/ca/songs/3":        "en-CA",
		"GET /v1/catalog/ca/songs/4":        "en-CA",
		"GET /amp/v1/play/assets":           "",
		"POST /v1/me/library":               "",
		"GET /v1/catalog/ca/albums/6":       "",
		"GET /v1/catalog/ca/songs/7/lyrics": "",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: l=%q, want %q", k, got[k], v)
		}
	}
}

func TestContentLanguageWithoutHook(t *testing.T) {
	var c *Client
	if got := c.ContentLanguage(); got != "" {
		t.Errorf("nil client: %q", got)
	}
	if got := (&Client{}).ContentLanguage(); got != "" {
		t.Errorf("no hook: %q", got)
	}
}
