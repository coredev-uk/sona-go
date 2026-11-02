// Package applemusic is Sona's Apple Music client. It is the only code that
// talks to Apple: api.music.apple.com and the play.itunes.apple.com playback
// endpoints.
package applemusic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
)

// BaseURL is the Apple Music API root.
const BaseURL = "https://api.music.apple.com"

// UserAgent is sent with every Apple request that sets none: a desktop
// browser, as the web player runs in.
const UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"

// ErrUnauthorized reports a 401/403 from Apple. Errors wrapping it are
// *StatusError values, so callers can tell a 401 (developer token) from a 403
// (Music User Token) with errors.As.
var ErrUnauthorized = errors.New("apple music: unauthorized")

// ErrNotFound reports a 404 from Apple (or an empty data array where a
// resource was expected).
var ErrNotFound = errors.New("apple music: not found")

// StatusError is a non-2xx response from Apple.
type StatusError struct {
	Method     string
	URL        string // without query
	Code       int
	Status     string
	Body       string        // first 512 bytes
	RetryAfter time.Duration // parsed Retry-After header, 0 if absent
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("apple music: %s %s: %s %s", e.Method, e.URL, e.Status, e.Body)
}

// Is makes errors.Is(err, ErrUnauthorized) match 401 and 403, and
// errors.Is(err, ErrNotFound) match 404.
func (e *StatusError) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.Code == http.StatusUnauthorized || e.Code == http.StatusForbidden
	case ErrNotFound:
		return e.Code == http.StatusNotFound
	}
	return false
}

// TokenSource supplies the developer token.
type TokenSource interface {
	Token(ctx context.Context) (devtoken.Token, error)
	Invalidate()
}

// Client issues authenticated Apple Music API requests.
//
// Requests that fail with 429 or a 5xx are retried with exponential backoff
// (honouring Retry-After), and identical concurrent GETs share one HTTP
// request. A Client must not be copied after first use.
type Client struct {
	HTTP      *http.Client
	Dev       TokenSource
	UserToken func() string // returns "" when signed out
	Base      string
	// AMPBase is the web player's API host for AMP requests (default
	// AMPBaseURL); tests point it at a fake.
	AMPBase string
	// Language returns the content language to ask Apple for (the l=
	// parameter, see ContentLanguage): a tag the storefront offers, or ""
	// for the storefront's default. Nil sends none.
	Language func() string

	flight singleflight.Group
	// sleep waits between retries; tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
}

// Retry policy for 429/5xx responses.
const (
	maxRetries    = 4
	retryBase     = 500 * time.Millisecond
	maxRetryWait  = 30 * time.Second
	maxRetryAfter = 60 * time.Second
)

// New returns a Client using the web player header policy.
func New(httpClient *http.Client, dev TokenSource, userToken func() string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{HTTP: httpClient, Dev: dev, UserToken: userToken, Base: BaseURL}
}

// Request describes one authenticated call.
type Request struct {
	Method string      // default GET
	URL    string      // absolute URL, or a path relative to Client.Base
	Query  url.Values  // appended to URL
	Body   any         // JSON-encoded when non-nil
	Header http.Header // extra headers
}

// Get fetches path (e.g. "/v1/me/storefront") with query q into out.
// A 401 refreshes the developer token and retries once.
func (c *Client) Get(ctx context.Context, path string, q url.Values, out any) error {
	return c.Do(ctx, Request{URL: path, Query: q}, out)
}

// Do sends r with the developer token, Music User Token and web player
// origin headers, and JSON-decodes a 2xx body into out (if non-nil; a
// *[]byte receives the raw body). A 401 refreshes the developer token and
// retries once. 429 and 5xx responses are retried up to maxRetries times
// with exponential backoff, waiting at least Retry-After when Apple sends
// it (5xx only for idempotent methods).
//
// Concurrent GETs without extra headers for the same URL are coalesced into
// one request; each caller decodes its own copy of the body. If the request
// was started by a caller whose context is cancelled, the others retry on
// their own.
func (c *Client) Do(ctx context.Context, r Request, out any) error {
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet || r.Body != nil || len(r.Header) > 0 {
		return c.doRetry(ctx, r, out)
	}
	key := c.resolve(r)
	for shared := true; ; shared = false {
		v, err, _ := c.flight.Do(key, func() (any, error) {
			var b []byte
			err := c.doRetry(ctx, r, &b)
			return b, err
		})
		if err != nil {
			if shared && ctx.Err() == nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				// The shared request belonged to a cancelled caller.
				continue
			}
			return err
		}
		return decode(v.([]byte), out)
	}
}

// resolve returns r's absolute URL including the query, with the content
// language (l=) added where it applies (wantsLanguage).
func (c *Client) resolve(r Request) string {
	u := r.URL
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		u = c.Base + u
	}
	q := r.Query
	if lang := c.ContentLanguage(); lang != "" && !namesLanguage(q) && c.wantsLanguage(r.Method, u) {
		q = maps.Clone(q)
		if q == nil {
			q = url.Values{}
		}
		q.Set("l", lang)
	}
	if len(q) > 0 {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + q.Encode()
	}
	return u
}

// ContentLanguage is the l= tag requests send now ("" for none): Language's
// answer. Caches of Apple's responses key on it.
func (c *Client) ContentLanguage() string {
	if c == nil || c.Language == nil {
		return ""
	}
	return c.Language()
}

// wantsLanguage reports whether a request reads content Apple localises:
// a GET from the API hosts (api and amp-api), except playback assets and
// a URL that names its own language already.
func (c *Client) wantsLanguage(method, u string) bool {
	if method != "" && method != http.MethodGet {
		return false
	}
	base, query, _ := strings.Cut(u, "?")
	if vals, err := url.ParseQuery(query); err != nil || namesLanguage(vals) {
		return false
	}
	for _, host := range []string{c.Base, c.ampBase()} {
		path, ok := strings.CutPrefix(base, host)
		if ok && host != "" && strings.HasPrefix(path, "/v1/") {
			return !strings.HasPrefix(path, "/v1/play/")
		}
	}
	return false
}

// namesLanguage reports whether a query chooses its own language: l=, or
// lyrics' l[lyrics]= and l[script]=, which the web player sends without l=.
func namesLanguage(q url.Values) bool {
	for k := range q {
		if k == "l" || strings.HasPrefix(k, "l[") {
			return true
		}
	}
	return false
}

// ContentLanguage picks the l= tag for the preferred languages (the
// Language setting alone, else the system's; most preferred first) among
// the storefront's supported ones: the first preferred tag the storefront
// offers (the tag itself, else a tag of the same language and script), in
// Apple's spelling. "" when the storefront's default (def) already is that
// language, when the storefront offers none of them, or when its languages
// are unknown: Apple then answers in the default.
func ContentLanguage(preferred []string, def string, supported []string) string {
	for _, want := range preferred {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		match := ""
		for _, s := range supported {
			if strings.EqualFold(s, want) {
				match = s
				break
			}
		}
		if match == "" && sameLanguage(def, want) {
			return ""
		}
		if match == "" {
			for _, s := range supported {
				if sameLanguage(s, want) {
					match = s
					break
				}
			}
		}
		if match != "" {
			if strings.EqualFold(match, def) {
				return ""
			}
			return match
		}
	}
	return ""
}

// sameLanguage reports whether two BCP 47 tags name the same language, and
// the same script when both name one ("zh-Hant-TW" and "zh-Hans-CN" do
// not).
func sameLanguage(a, b string) bool {
	la, sa := languageScript(a)
	lb, sb := languageScript(b)
	return la != "" && la == lb && (sa == "" || sb == "" || sa == sb)
}

// languageScript is a tag's language and script subtags, lower case.
func languageScript(tag string) (lang, script string) {
	parts := strings.Split(strings.ToLower(tag), "-")
	if len(parts) > 1 && len(parts[1]) == 4 {
		script = parts[1]
	}
	return parts[0], script
}

func decode(b []byte, out any) error {
	switch out := out.(type) {
	case nil:
		return nil
	case *[]byte:
		*out = bytes.Clone(b)
		return nil
	default:
		if len(b) == 0 {
			return nil
		}
		return json.Unmarshal(b, out)
	}
}

// doRetry runs one logical request: the 401 token refresh and the 429/5xx
// backoff loop.
func (c *Client) doRetry(ctx context.Context, r Request, out any) error {
	refreshed := false
	for attempt := 0; ; attempt++ {
		status, err := c.do(ctx, r, out)
		if err == nil {
			return nil
		}
		if status == http.StatusUnauthorized && !refreshed {
			refreshed = true
			c.Dev.Invalidate()
			continue
		}
		if attempt >= maxRetries || !retryable(status, r.Method) {
			return err
		}
		wait := backoff(attempt)
		var se *StatusError
		if errors.As(err, &se) && se.RetryAfter > 0 {
			wait = max(wait, min(se.RetryAfter, maxRetryAfter))
		}
		if err := c.wait(ctx, wait); err != nil {
			return err
		}
	}
}

func retryable(status int, method string) bool {
	switch {
	case status == http.StatusTooManyRequests:
		return true
	case status >= 500 && status <= 599:
		switch method {
		case "", http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
			return true
		}
	}
	return false
}

// backoff is retryBase·2^attempt with ±25% jitter, capped at maxRetryWait.
func backoff(attempt int) time.Duration {
	d := min(retryBase<<attempt, maxRetryWait)
	return d - d/4 + time.Duration(rand.Int64N(int64(d/2)+1))
}

func (c *Client) wait(ctx context.Context, d time.Duration) error {
	if c.sleep != nil {
		return c.sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// parseRetryAfter reads a Retry-After header (delay seconds or HTTP date).
func parseRetryAfter(v string, now time.Time) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

func (c *Client) do(ctx context.Context, r Request, out any) (int, error) {
	tok, err := c.Dev.Token(ctx)
	if err != nil {
		return 0, err
	}
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	u := c.resolve(r)
	base, _, _ := strings.Cut(u, "?")
	var body io.Reader
	if r.Body != nil {
		b, err := json.Marshal(r.Body)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return 0, err
	}
	for k, v := range r.Header {
		req.Header[k] = v
	}
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+tok.JWT)
	// The web player token is only accepted from an apple.com origin.
	req.Header.Set("Origin", devtoken.WebPlayerOrigin)
	req.Header.Set("Referer", devtoken.WebPlayerOrigin+"/")
	if req.Header.Get("User-Agent") == "" {
		// Apple's edge 403s a share of requests with Go's default agent
		// (seen on the artwork CDN); send the web player's browser agent.
		req.Header.Set("User-Agent", UserAgent)
	}
	if c.UserToken != nil {
		if mut := c.UserToken(); mut != "" {
			req.Header.Set("Media-User-Token", mut)
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return resp.StatusCode, &StatusError{
			Method: method, URL: base, Code: resp.StatusCode, Status: resp.Status, Body: string(b),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	}
	switch out := out.(type) {
	case nil:
		return resp.StatusCode, nil
	case *[]byte:
		*out, err = io.ReadAll(resp.Body)
		return resp.StatusCode, err
	default:
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, err
		}
		if len(b) == 0 {
			return resp.StatusCode, nil
		}
		return resp.StatusCode, json.Unmarshal(b, out)
	}
}

// Resource is the common Apple Music resource envelope.
type Resource[A any] struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Href       string `json:"href"`
	Attributes A      `json:"attributes"`
}

// StorefrontAttributes describes a storefront.
type StorefrontAttributes struct {
	Name                  string   `json:"name"`
	DefaultLanguageTag    string   `json:"defaultLanguageTag"`
	ExplicitContentPolicy string   `json:"explicitContentPolicy"`
	SupportedLanguageTags []string `json:"supportedLanguageTags"`
}

// SongAttributes are the song attributes Sona uses, for catalog songs and
// library songs (which carry a subset).
type SongAttributes struct {
	Name         string `json:"name"`
	ArtistName   string `json:"artistName"`
	AlbumName    string `json:"albumName"`
	ComposerName string `json:"composerName"`
	// Classical works: the work a movement belongs to and the movement's
	// own name (catalog songs; "" for everything else).
	WorkName         string   `json:"workName,omitempty"`
	MovementName     string   `json:"movementName,omitempty"`
	DurationInMillis int64    `json:"durationInMillis"`
	TrackNumber      int      `json:"trackNumber"`
	DiscNumber       int      `json:"discNumber"`
	ReleaseDate      string   `json:"releaseDate"`
	GenreNames       []string `json:"genreNames"`
	ContentRating    string   `json:"contentRating"` // "explicit", "clean" or ""
	HasLyrics        bool     `json:"hasLyrics"`
	// Display badges (labels only: the web player streams AAC).
	AudioTraits          []string `json:"audioTraits"` // lossless, hi-res-lossless, lossy-stereo, atmos, spatial
	IsAppleDigitalMaster bool     `json:"isAppleDigitalMaster"`
	IsMasteredForItunes  bool     `json:"isMasteredForItunes"`
	// HasTimeSyncedLyrics and LyricsExcerpt (song page, extend=lyricsExcerpt).
	HasTimeSyncedLyrics bool        `json:"hasTimeSyncedLyrics"`
	LyricsExcerpt       string      `json:"lyricsExcerpt,omitempty"`
	ISRC                string      `json:"isrc"`
	URL                 string      `json:"url"`
	Artwork             *Artwork    `json:"artwork"`
	PlayParams          *PlayParams `json:"playParams"`
	Previews            []struct {
		URL string `json:"url"`
	} `json:"previews"`
}

// Storefront returns the signed-in user's storefront (validates the MUT).
func (c *Client) Storefront(ctx context.Context) (Resource[StorefrontAttributes], error) {
	var body struct {
		Data []Resource[StorefrontAttributes] `json:"data"`
	}
	if err := c.Get(ctx, "/v1/me/storefront", nil, &body); err != nil {
		return Resource[StorefrontAttributes]{}, err
	}
	if len(body.Data) == 0 {
		return Resource[StorefrontAttributes]{}, errors.New("apple music: empty storefront response")
	}
	return body.Data[0], nil
}

// SearchSongs searches the catalog of storefront sf for songs.
func (c *Client) SearchSongs(ctx context.Context, sf, term string, limit int) ([]Resource[SongAttributes], error) {
	var body struct {
		Results struct {
			Songs struct {
				Data []Resource[SongAttributes] `json:"data"`
			} `json:"songs"`
		} `json:"results"`
	}
	q := url.Values{"term": {term}, "types": {"songs"}, "limit": {fmt.Sprint(limit)}}
	if err := c.Get(ctx, "/v1/catalog/"+url.PathEscape(sf)+"/search", q, &body); err != nil {
		return nil, err
	}
	return body.Results.Songs.Data, nil
}
